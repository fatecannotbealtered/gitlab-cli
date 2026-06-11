package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

func TestDoctor_Help(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"doctor", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(nil)
	// Reset the help flag so it doesn't persist into subsequent tests.
	if f := doctorCmd.Flags().Lookup("help"); f != nil {
		_ = f.Value.Set("false")
	}
	out := buf.String()
	if !strings.Contains(out, "doctor") {
		t.Errorf("doctor --help missing expected text\noutput:\n%s", out)
	}
}

func TestDoctor_NoConfig(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"doctor"})
	_ = rootCmd.Execute()

	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit when not configured, got %d", lastExit)
	}
}

func TestDoctor_WithMockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/user":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1,"username":"alice","name":"Alice"}`))
		case "/api/v4/version":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"16.0","revision":"abc"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"doctor"})
	_ = rootCmd.Execute()

	if lastExit != ExitOK {
		t.Errorf("expected exit 0 with valid mock server, got %d", lastExit)
	}
}

func TestDoctor_NoConfig_JSON(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")
	origJM := jsonMode
	origExit := lastExit
	defer func() { jsonMode = origJM; lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"doctor", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"configExists"`) {
		t.Errorf("expected configExists in JSON output, got: %s", out)
	}
}

func TestDoctor_WithMockServer_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"username":"alice","name":"Alice"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	origExit := lastExit
	defer func() { jsonMode = origJM; lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"doctor", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"authValid": true`) {
		t.Errorf("expected authValid true in JSON output, got: %s", out)
	}
	if !strings.Contains(out, `"release_readiness"`) || !strings.Contains(out, `"pass"`) {
		t.Errorf("expected release_readiness pass in JSON output, got: %s", out)
	}
}

func TestDoctor_AuthFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "bad-token")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"doctor"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for auth failure, got %d", lastExit)
	}
}

func TestDoctor_AuthFailed_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "bad-token")
	resetRootPersistentFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"doctor", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitAuth {
		t.Errorf("expected exit %d, got %d", ExitAuth, lastExit)
	}
	if !strings.Contains(out, `"authValid": false`) {
		t.Errorf("expected authValid false in JSON, got: %s", out)
	}
}

func TestDoctor_AuthFailed_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "bad-token")
	resetRootPersistentFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"doctor"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitForbidden {
		t.Errorf("expected exit %d, got %d", ExitForbidden, lastExit)
	}
	if !strings.Contains(out, "Connection failed") {
		t.Errorf("expected connection failed message, got:\n%s", out)
	}
}

func TestDoctor_NetworkError(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "http://127.0.0.1:1")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetRootPersistentFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"doctor", "--json"})
	_ = rootCmd.Execute()
	if lastExit != ExitNetwork {
		t.Errorf("expected exit %d, got %d", ExitNetwork, lastExit)
	}
}

func TestDoctor_ConfigLoadError(t *testing.T) {
	home := isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")
	writeGitLabCLIFile(t, home, "profiles.json", `{bad`)
	resetRootPersistentFlags(t)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"doctor", "--json"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitAuth {
		t.Errorf("expected exit %d, got %d", ExitAuth, lastExit)
	}
	if !strings.Contains(out, `"error"`) {
		t.Errorf("expected error in JSON, got:\n%s", out)
	}
}

func TestDoctor_ConfigLoadError_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	home := isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")
	writeGitLabCLIFile(t, home, "profiles.json", `{bad`)
	resetRootPersistentFlags(t)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"doctor"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitAuth {
		t.Errorf("expected exit %d, got %d", ExitAuth, lastExit)
	}
	if !strings.Contains(out, "Reading config") {
		t.Errorf("expected config read error, got:\n%s", out)
	}
}

func TestDoctor_NoConfig_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")
	resetRootPersistentFlags(t)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"doctor"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitAuth {
		t.Errorf("expected exit %d, got %d", ExitAuth, lastExit)
	}
	if !strings.Contains(out, "not configured") {
		t.Errorf("expected not configured error, got:\n%s", out)
	}
}

func TestDoctor_Success_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"username":"alice","name":"Alice"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	resetRootPersistentFlags(t)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"doctor"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
	for _, want := range []string{"Config found", "PAT valid", "Authenticated as Alice", "Latency:"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestAsAPI_Direct(t *testing.T) {
	var target *api.APIError
	err := &api.APIError{StatusCode: 401, ErrorMessages: []string{"unauthorized"}}
	if !asAPI(err, &target) {
		t.Fatal("expected asAPI to match direct APIError")
	}
	if target.StatusCode != 401 {
		t.Errorf("status = %d, want 401", target.StatusCode)
	}
}

func TestAsAPI_Wrapped(t *testing.T) {
	var target *api.APIError
	inner := &api.APIError{StatusCode: 403}
	wrapped := fmt.Errorf("outer: %w", inner)
	if !asAPI(wrapped, &target) {
		t.Fatal("expected asAPI to match wrapped APIError")
	}
	if target.StatusCode != 403 {
		t.Errorf("status = %d, want 403", target.StatusCode)
	}
}

type stopErr struct{ msg string }

func (e stopErr) Error() string { return e.msg }

func TestAsAPI_UnwrapStopsWithoutAPIError(t *testing.T) {
	var target *api.APIError
	if asAPI(stopErr{msg: "no unwrap"}, &target) {
		t.Fatal("expected false when error does not unwrap")
	}
}

func TestAsAPI_NoMatch(t *testing.T) {
	var target *api.APIError
	if asAPI(fmt.Errorf("plain"), &target) {
		t.Fatal("expected false for non-API error")
	}
	if asAPI(nil, &target) {
		t.Fatal("expected false for nil error")
	}
}
