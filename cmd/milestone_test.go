package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestMilestoneCmd_Help(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"milestone", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"list", "get", "create", "update", "close"} {
		if !strings.Contains(out, want) {
			t.Errorf("milestone --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestMilestoneCreate_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = true
	jsonMode = true

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"milestone", "create", "--project", "42", "--title", "v1.0", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"action": "create milestone"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestMilestoneClose_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = true
	jsonMode = true

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"milestone", "close", "--project", "42", "--milestone-id", "5", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"action": "close milestone"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestMilestone_Help_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"milestone", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"list", "get", "create", "update", "close"} {
		if !strings.Contains(out, want) {
			t.Errorf("milestone --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestMilestone_List_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"milestone", "list"})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", lastExit, ExitBadArgs)
	}
}

func TestMilestone_List_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"iid":1,"title":"v1","state":"active","due_date":"","start_date":"","web_url":""}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"milestone", "list", "--project", "foo/bar", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"title": "v1"`) {
		t.Errorf("expected milestone in output, got: %s", out)
	}
}

func TestMilestone_Get_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"iid":1,"title":"v1","state":"active","due_date":"","start_date":"","web_url":""}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"milestone", "get", "--project", "foo/bar", "--milestone-id", "1", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"id": 1`) {
		t.Errorf("expected id in output, got: %s", out)
	}
}

func TestMilestone_Update_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"milestone", "update",
			"--dry-run", "--json",
			"--project", "foo/bar",
			"--milestone-id", "1",
			"--title", "v2",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"action": "update milestone"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestMilestone_Close_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"milestone", "close",
			"--dry-run", "--json",
			"--project", "foo/bar",
			"--milestone-id", "1",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"action": "close milestone"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestMilestone_Create_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/milestones") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":1,"iid":1,"title":"v1","state":"active","created_at":"2024-01-01","updated_at":"2024-01-01"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"milestone", "create", "--project", "foo/bar", "--title", "v1", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"title": "v1"`) {
		t.Errorf("expected milestone in output, got: %s", out)
	}
}

func TestMilestone_Update_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"iid":1,"title":"v2","state":"active","created_at":"2024-01-01","updated_at":"2024-01-01"}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"milestone", "update", "--project", "foo/bar", "--milestone-id", "1", "--title", "v2", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"title": "v2"`) {
		t.Errorf("expected updated milestone in output, got: %s", out)
	}
}

func TestMilestone_Close_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"iid":1,"title":"v1","state":"closed","created_at":"2024-01-01","updated_at":"2024-01-01"}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"milestone", "close", "--project", "foo/bar", "--milestone-id", "1", "--json", "--force"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"closed"`) {
		t.Errorf("expected closed state in output, got: %s", out)
	}
}

func TestMilestone_List_Fields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"iid":1,"title":"v1","state":"active","due_date":"","start_date":"","web_url":""}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"milestone", "list", "--project", "foo/bar", "--json", "--fields", "title"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"title"`) {
		t.Errorf("expected title field in output, got: %s", out)
	}
	if strings.Contains(out, `"state"`) {
		t.Errorf("state field should be filtered out, got: %s", out)
	}
}

func TestMilestone_List_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"iid":1,"title":"v1","state":"active","due_date":"","start_date":"","web_url":""}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"milestone", "list", "--project", "foo/bar"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "v1") {
		t.Errorf("expected milestone title in plain text output:\n%s", out)
	}
}

func TestMilestone_Get_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"iid":1,"title":"v1","state":"active","due_date":"","start_date":"","web_url":""}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"milestone", "get", "--project", "foo/bar", "--milestone-id", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "v1") {
		t.Errorf("expected milestone title in plain text output:\n%s", out)
	}
}

func TestMilestone_Create_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":1,"iid":1,"title":"v1","state":"active","created_at":"2024-01-01","updated_at":"2024-01-01"}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	dryRun = false
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"milestone", "create", "--project", "foo/bar", "--title", "v1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Created") {
		t.Errorf("expected 'Created' in output, got: %s", out)
	}
}

func TestMilestone_Update_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"iid":1,"title":"v2","state":"active","created_at":"2024-01-01","updated_at":"2024-01-01"}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	dryRun = false
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"milestone", "update", "--project", "foo/bar", "--milestone-id", "1", "--title", "v2"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Updated") {
		t.Errorf("expected 'Updated' in output, got: %s", out)
	}
}

func TestMilestone_List_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "bad-token")
	origExit := lastExit
	origJM := jsonMode
	defer func() { lastExit = origExit; jsonMode = origJM }()
	lastExit = 0
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	rootCmd.SetArgs([]string{"milestone", "list", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestMilestone_List_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"milestone", "list", "--project", "foo/bar"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No milestones") {
		t.Errorf("expected 'No milestones' in output, got: %s", out)
	}
}

func TestMilestone_List_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"milestone", "list", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMilestone_List_InvalidLimit(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = milestoneListCmd.Flags().Set("limit", "20") }()
	lastExit = 0
	_ = milestoneListCmd.Flags().Set("limit", "0")
	rootCmd.SetArgs([]string{"milestone", "list", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMilestone_Get_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = milestoneGetCmd.Flags().Set("project", "") }()
	lastExit = 0
	_ = milestoneGetCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"milestone", "get", "--milestone-id", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMilestone_Get_MissingMilestoneID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = milestoneGetCmd.Flags().Set("milestone-id", "0") }()
	lastExit = 0
	_ = milestoneGetCmd.Flags().Set("milestone-id", "0")
	rootCmd.SetArgs([]string{"milestone", "get", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMilestone_Get_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"milestone", "get", "--project", "foo/bar", "--milestone-id", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMilestone_Get_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"milestone", "get", "--project", "foo/bar", "--milestone-id", "1"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestMilestone_Get_PlainTextFull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"iid":1,"title":"v1","state":"active","start_date":"2024-01-01","due_date":"2024-06-01","description":"release","web_url":""}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"milestone", "get", "--project", "foo/bar", "--milestone-id", "1"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{"v1", "2024-01-01", "2024-06-01", "release"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestMilestone_Create_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR; _ = milestoneCreateCmd.Flags().Set("project", "") }()
	lastExit = 0
	dryRun = false
	_ = milestoneCreateCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"milestone", "create", "--title", "v1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMilestone_Create_MissingTitle(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR; _ = milestoneCreateCmd.Flags().Set("title", "") }()
	lastExit = 0
	dryRun = false
	_ = milestoneCreateCmd.Flags().Set("title", "")
	rootCmd.SetArgs([]string{"milestone", "create", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMilestone_Create_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	rootCmd.SetArgs([]string{"milestone", "create", "--project", "foo/bar", "--title", "v1"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMilestone_Create_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"500"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origExit := lastExit
	defer func() { dryRun = origDR; lastExit = origExit }()
	dryRun = false
	lastExit = 0
	rootCmd.SetArgs([]string{"milestone", "create", "--project", "foo/bar", "--title", "v1"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestMilestone_Update_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR; _ = milestoneUpdateCmd.Flags().Set("project", "") }()
	lastExit = 0
	dryRun = false
	_ = milestoneUpdateCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"milestone", "update", "--milestone-id", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMilestone_Update_MissingMilestoneID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR; _ = milestoneUpdateCmd.Flags().Set("milestone-id", "0") }()
	lastExit = 0
	dryRun = false
	_ = milestoneUpdateCmd.Flags().Set("milestone-id", "0")
	rootCmd.SetArgs([]string{"milestone", "update", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMilestone_Update_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	rootCmd.SetArgs([]string{"milestone", "update", "--project", "foo/bar", "--milestone-id", "1", "--title", "v2"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMilestone_Update_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"500"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origExit := lastExit
	defer func() { dryRun = origDR; lastExit = origExit }()
	dryRun = false
	lastExit = 0
	rootCmd.SetArgs([]string{"milestone", "update", "--project", "foo/bar", "--milestone-id", "1", "--title", "v2"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestMilestone_Close_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR; _ = milestoneCloseCmd.Flags().Set("project", "") }()
	lastExit = 0
	dryRun = false
	_ = milestoneCloseCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"milestone", "close", "--milestone-id", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMilestone_Close_MissingMilestoneID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR; _ = milestoneCloseCmd.Flags().Set("milestone-id", "0") }()
	lastExit = 0
	dryRun = false
	_ = milestoneCloseCmd.Flags().Set("milestone-id", "0")
	rootCmd.SetArgs([]string{"milestone", "close", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMilestone_Close_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	rootCmd.SetArgs([]string{"milestone", "close", "--project", "foo/bar", "--milestone-id", "1", "--force"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMilestone_Close_ConfirmCancelled(t *testing.T) {
	t.Setenv("GITLAB_CLI_AGENT_SAFE", "0")
	withNonInteractiveStdin(t)
	resetRootPersistentFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR; resetRootPersistentFlags(t) }()
	lastExit = 0
	dryRun = false
	rootCmd.SetArgs([]string{"milestone", "close", "--project", "foo/bar", "--milestone-id", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitCancelled {
		t.Errorf("exit code = %d, want %d", lastExit, ExitCancelled)
	}
}

func TestMilestone_Close_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"500"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origExit := lastExit
	defer func() { dryRun = origDR; lastExit = origExit }()
	dryRun = false
	lastExit = 0
	rootCmd.SetArgs([]string{"milestone", "close", "--project", "foo/bar", "--milestone-id", "1", "--force"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestMilestone_Close_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":1,"iid":1,"title":"v1","state":"closed","created_at":"2024-01-01","updated_at":"2024-01-01"}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	dryRun = false
	jsonMode = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"milestone", "close", "--project", "foo/bar", "--milestone-id", "1", "--force"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Closed") {
		t.Errorf("expected Closed in output, got: %s", out)
	}
}
