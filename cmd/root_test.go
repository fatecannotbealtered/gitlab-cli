package cmd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
	"github.com/spf13/cobra"
)

func TestExecute_Smoke(t *testing.T) {
	resetRootPersistentFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"--help"})
	if err := Execute(); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
}

func TestExecuteContext_Smoke(t *testing.T) {
	resetRootPersistentFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"--version"})
	if err := ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() = %v", err)
	}
}

func TestApiCtx_ActiveCmdWithContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), testCtxKey{}, "v")
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	activeCmd = cmd
	defer func() { activeCmd = nil }()

	if got := apiCtx(); got != ctx {
		t.Fatal("apiCtx should return active command context")
	}
}

func TestApiCtx_ActiveCmdNilContext(t *testing.T) {
	cmd := &cobra.Command{}
	activeCmd = cmd
	defer func() { activeCmd = nil }()

	got := apiCtx()
	if got == nil {
		t.Fatal("apiCtx should not return nil")
	}
}

func TestApiCtx_NoActiveCmd(t *testing.T) {
	activeCmd = nil
	got := apiCtx()
	if got == nil {
		t.Fatal("apiCtx should fall back to background context")
	}
}

type testCtxKey struct{}

func TestHandleAPIError_AllStatusCodes(t *testing.T) {
	statuses := map[int]int{
		401: ExitAuth,
		403: ExitForbidden,
		404: ExitNotFound,
		429: ExitRateLimit,
		500: ExitNetwork,
		418: ExitBadArgs,
	}
	for status, wantExit := range statuses {
		for _, jsonMode := range []bool{false, true} {
			lastExit = 0
			apiErr := &api.APIError{StatusCode: status, ErrorMessages: []string{"boom"}}
			var err error
			if jsonMode {
				err = captureHandleAPIErrorJSON(t, apiErr)
			} else {
				err = captureHandleAPIErrorPlain(t, apiErr)
			}
			if !errors.Is(err, ErrSilent) {
				t.Fatalf("status %d json=%v: expected ErrSilent, got %v", status, jsonMode, err)
			}
			if lastExit != wantExit {
				t.Fatalf("status %d json=%v: exit=%d want=%d", status, jsonMode, lastExit, wantExit)
			}
		}
	}
}

func captureHandleAPIErrorJSON(t *testing.T, apiErr error) error {
	t.Helper()
	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = true
	return handleAPIError(apiErr, true)
}

func captureHandleAPIErrorPlain(t *testing.T, apiErr error) error {
	t.Helper()
	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	setTextFormatForTest(t)
	stderr := captureStderr(t, func() {
		_ = handleAPIError(apiErr, false)
	})
	if !strings.Contains(stderr, "GitLab API error") && !strings.Contains(stderr, "boom") {
		t.Fatalf("plain handleAPIError stderr = %q", stderr)
	}
	return ErrSilent
}

func TestHandleAPIError_NonAPIError(t *testing.T) {
	lastExit = 0
	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = true

	err := handleAPIError(errors.New("connection reset"), true)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("expected ErrSilent, got %v", err)
	}
	if lastExit != ExitNetwork {
		t.Fatalf("exit=%d want=%d", lastExit, ExitNetwork)
	}
}

func TestHandleAPIError_NonAPIError_PlainText(t *testing.T) {
	lastExit = 0
	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	setTextFormatForTest(t)

	stderr := captureStderr(t, func() {
		err := handleAPIError(errors.New("connection reset"), false)
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("expected ErrSilent, got %v", err)
		}
	})
	if !strings.Contains(stderr, "connection reset") {
		t.Fatalf("stderr = %q", stderr)
	}
	if lastExit != ExitNetwork {
		t.Fatalf("exit=%d want=%d", lastExit, ExitNetwork)
	}
}

func TestDryRunOutput_JSONNilDetail(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() {
		dryRun = origDR
		jsonMode = origJM
	}()
	dryRun = true
	jsonMode = true

	out := captureStdout(t, func() {
		if !dryRunOutput("noop", nil) {
			t.Fatal("dryRunOutput should return true when dry-run is set")
		}
	})
	for _, want := range []string{`"action": "noop"`, `"dryRun": true`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
}

func TestMarkConfirm_InitializesAnnotations(t *testing.T) {
	cmd := &cobra.Command{}
	markConfirm(cmd)
	if cmd.Annotations["confirm"] != "true" {
		t.Fatalf("confirm annotation = %q", cmd.Annotations["confirm"])
	}
}

func TestMarkRiskLevel_InitializesAnnotations(t *testing.T) {
	cmd := &cobra.Command{}
	markRiskLevel(cmd, "critical")
	if cmd.Annotations["riskLevel"] != "critical" {
		t.Fatalf("riskLevel annotation = %q", cmd.Annotations["riskLevel"])
	}
}

func TestNewClient_NotLoggedIn_JSON(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")

	origJM := jsonMode
	origExit := lastExit
	defer func() {
		jsonMode = origJM
		lastExit = origExit
	}()
	jsonMode = true
	lastExit = 0

	_, _, err := newClient()
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("newClient() = %v, want ErrSilent", err)
	}
	if lastExit != ExitAuth {
		t.Fatalf("exit=%d want=%d", lastExit, ExitAuth)
	}
}

func TestNewClient_NotLoggedIn_PlainText(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")

	origJM := jsonMode
	origExit := lastExit
	defer func() {
		jsonMode = origJM
		lastExit = origExit
	}()
	setTextFormatForTest(t)
	lastExit = 0

	stderr := captureStderr(t, func() {
		_, _, err := newClient()
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("newClient() = %v", err)
		}
	})
	if !strings.Contains(stderr, "not logged in") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestNewClient_CorruptConfig(t *testing.T) {
	dir := isolateConfigHome(t)
	cfgDir := filepath.Join(dir, ".gitlab-cli")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")

	origJM := jsonMode
	origExit := lastExit
	defer func() {
		jsonMode = origJM
		lastExit = origExit
	}()
	jsonMode = true
	lastExit = 0

	_, _, err := newClient()
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("newClient() = %v", err)
	}
	if lastExit != ExitAuth {
		t.Fatalf("exit=%d want=%d", lastExit, ExitAuth)
	}
}

func TestNewClient_Success(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	client, cfg, err := newClient()
	if err != nil {
		t.Fatalf("newClient() = %v", err)
	}
	if client == nil || cfg == nil {
		t.Fatal("expected client and config")
	}
	if cfg.Host != "https://gitlab.example.com" {
		t.Fatalf("host = %q", cfg.Host)
	}
}

func TestResolveUserID_Me(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/user" {
			_, _ = w.Write([]byte(`{"id":7,"username":"me"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	client := api.NewClient(cfg)

	id, err := resolveUserID(client, "me")
	if err != nil {
		t.Fatalf("resolveUserID(me) = %v", err)
	}
	if id != 7 {
		t.Fatalf("id=%d want=7", id)
	}
}

func TestResolveUserID_ByUsername(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/users") {
			_, _ = w.Write([]byte(`[{"id":42,"username":"alice"}]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	client := api.NewClient(mustTestConfig(t))

	id, err := resolveUserID(client, "alice")
	if err != nil {
		t.Fatalf("resolveUserID(alice) = %v", err)
	}
	if id != 42 {
		t.Fatalf("id=%d want=42", id)
	}
}

func TestResolveUserID_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/users") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	client := api.NewClient(mustTestConfig(t))

	origJM := jsonMode
	origExit := lastExit
	defer func() {
		jsonMode = origJM
		lastExit = origExit
	}()
	jsonMode = true
	lastExit = 0

	_, err := resolveUserID(client, "ghost")
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("resolveUserID(ghost) = %v", err)
	}
	if lastExit != ExitNotFound {
		t.Fatalf("exit=%d want=%d", lastExit, ExitNotFound)
	}
}

func TestResolveUserID_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	client := api.NewClient(mustTestConfig(t))

	origJM := jsonMode
	origExit := lastExit
	defer func() {
		jsonMode = origJM
		lastExit = origExit
	}()
	jsonMode = true
	lastExit = 0

	_, err := resolveUserID(client, "me")
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("resolveUserID(me) = %v", err)
	}
	if lastExit != ExitAuth {
		t.Fatalf("exit=%d want=%d", lastExit, ExitAuth)
	}
}

func TestResolveUserID_UsernameAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/users") {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	client := api.NewClient(mustTestConfig(t))

	origJM := jsonMode
	origExit := lastExit
	defer func() {
		jsonMode = origJM
		lastExit = origExit
	}()
	jsonMode = true
	lastExit = 0

	_, err := resolveUserID(client, "alice")
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("resolveUserID(alice) = %v", err)
	}
	if lastExit != ExitForbidden {
		t.Fatalf("exit=%d want=%d", lastExit, ExitForbidden)
	}
}

func TestResolveUserID_UsesCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	activeCmd = cmd
	defer func() { activeCmd = nil }()

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	client := api.NewClient(mustTestConfig(t))

	origJM := jsonMode
	origExit := lastExit
	defer func() {
		jsonMode = origJM
		lastExit = origExit
	}()
	jsonMode = true
	lastExit = 0

	_, err := resolveUserID(client, "me")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestValidateOutputPath_Empty(t *testing.T) {
	if err := validateOutputPath(""); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestGetFieldsFlag_NilCommand(t *testing.T) {
	if got := getFieldsFlag(nil); got != nil {
		t.Fatalf("getFieldsFlag(nil) = %v, want nil", got)
	}
}

func TestGetFieldsFlag_FiltersEmptyParts(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("fields", "", "")
	_ = cmd.Flags().Set("fields", "title,, state , ,iid")

	got := getFieldsFlag(cmd)
	want := "title|state|iid"
	if strings.Join(got, "|") != want {
		t.Fatalf("getFieldsFlag() = %v, want [%s]", got, want)
	}
}

func mustTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
