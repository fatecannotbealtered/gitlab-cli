package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestMRComment_Add_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":10,"body":"LGTM","author":{"username":"alice"},"created_at":"2024-01-01"}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "comment", "add", "--project", "foo/bar", "1", "--body", "LGTM", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"LGTM"`) {
		t.Errorf("expected body in output, got: %s", out)
	}
}

func TestMRComment_Add_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":10,"body":"LGTM","author":{"username":"alice"},"created_at":"2024-01-01"}`)
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
		rootCmd.SetArgs([]string{"mr", "comment", "add", "--project", "foo/bar", "1", "--body", "LGTM"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Added") {
		t.Errorf("expected 'Added' in output, got: %s", out)
	}
}

func TestMRComment_List_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"body":"hello","system":false,"author":{"username":"alice"},"created_at":"2024-01-01"},{"id":2,"body":"system note","system":true,"author":{"username":"gitlab"},"created_at":"2024-01-01"}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "comment", "list", "--project", "foo/bar", "1", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"hello"`) {
		t.Errorf("expected comment body in output, got: %s", out)
	}
}

func TestMRComment_List_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"body":"hello","system":false,"author":{"username":"alice"},"created_at":"2024-01-01"}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "comment", "list", "--project", "foo/bar", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "hello") {
		t.Errorf("expected comment body in plain text output, got: %s", out)
	}
}

func TestMRComment_Delete_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	origExit := lastExit
	defer func() { dryRun = origDR; jsonMode = origJM; lastExit = origExit }()
	dryRun = true
	jsonMode = true
	lastExit = 0

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"mr", "comment", "delete",
			"--project", "foo/bar", "1", "--note-id", "10",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Errorf("expected exit 0 for dry-run, got %d", lastExit)
	}
	for _, want := range []string{`"dryRun": true`, `"action": "delete mr comment"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestMRComment_Delete_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	origExit := lastExit
	defer func() { dryRun = origDR; jsonMode = origJM; lastExit = origExit }()
	dryRun = false
	lastExit = 0
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "comment", "delete", "--project", "foo/bar", "1", "--note-id", "10", "--json", "--force"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
}

func TestMRComment_Delete_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	origExit := lastExit
	defer func() {
		dryRun = origDR
		jsonMode = origJM
		lastExit = origExit
		_ = rootCmd.PersistentFlags().Set("json", "false")
	}()
	dryRun = false
	jsonMode = false
	lastExit = 0
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "comment", "delete", "--project", "foo/bar", "1", "--note-id", "10", "--force"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Deleted") {
		t.Errorf("expected 'Deleted' in output, got: %s", out)
	}
}

func TestMRComment_Add_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	// Reset flags to avoid stale values
	_ = mrCommentAddCmd.Flags().Set("body", "")
	_ = mrCommentAddCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"mr", "comment", "add", "1", "--body", "hi"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMRComment_Add_MissingBody(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	// Reset flags to avoid stale values
	_ = mrCommentAddCmd.Flags().Set("body", "")
	rootCmd.SetArgs([]string{"mr", "comment", "add", "--project", "foo/bar", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMRComment_List_MissingIID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "comment", "list", "--project", "foo/bar"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error when IID positional arg is missing")
	}
}

func TestMRComment_List_Empty(t *testing.T) {
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
		rootCmd.SetArgs([]string{"mr", "comment", "list", "--project", "foo/bar", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No comments") {
		t.Errorf("expected 'No comments' in output, got: %s", out)
	}
}

func TestMRComment_List_LongBody(t *testing.T) {
	longBody := strings.Repeat("x", 100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `[{"id":1,"body":%q,"system":false,"author":{"username":"alice"},"created_at":"2024-01-01"}]`, longBody)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "comment", "list", "--project", "foo/bar", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "...") {
		t.Errorf("expected truncated body with '...' in output, got: %s", out)
	}
}

func TestMRComment_List_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"mr", "comment", "list", "--project", "foo/bar", "1"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestMRComment_Delete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	origJM := jsonMode
	defer func() { lastExit = origExit; dryRun = origDR; jsonMode = origJM }()
	lastExit = 0
	dryRun = false
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	rootCmd.SetArgs([]string{"mr", "comment", "delete", "--project", "foo/bar", "1", "--note-id", "999", "--force"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func resetMRCommentCmdFlags(t *testing.T) {
	t.Helper()
	resetRootPersistentFlags(t)
	for _, kv := range []struct {
		cmd   *cobra.Command
		name  string
		value string
	}{
		{mrCommentAddCmd, "project", ""},
		{mrCommentAddCmd, "body", ""},
		{mrCommentAddCmd, "body-file", ""},
		{mrCommentListCmd, "project", ""},
		{mrCommentListCmd, "limit", "20"},
		{mrCommentDeleteCmd, "project", ""},
		{mrCommentDeleteCmd, "note-id", "0"},
	} {
		if err := kv.cmd.Flags().Set(kv.name, kv.value); err != nil {
			t.Fatalf("reset mr comment flag %s.%s: %v", kv.cmd.Name(), kv.name, err)
		}
	}
}

func TestMRComment_Add_InvalidIID(t *testing.T) {
	resetMRCommentCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "comment", "add", "--project", "g/p", "abc", "--body", "hi"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMRComment_Add_BodyFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":10,"body":"from file","author":{"username":"alice"},"created_at":"2024-01-01"}`)
	}))
	defer srv.Close()
	resetMRCommentCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	bodyFile := t.TempDir() + string(os.PathSeparator) + "body.txt"
	if err := os.WriteFile(bodyFile, []byte("from file"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"mr", "comment", "add", "--project", "g/p", "1", "--body-file", bodyFile})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Added") {
		t.Errorf("expected Added in output, got: %s", out)
	}
}

func TestMRComment_Add_BodyFileError(t *testing.T) {
	resetMRCommentCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "comment", "add", "--project", "g/p", "1", "--body-file", t.TempDir() + string(os.PathSeparator) + "missing.txt"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMRComment_Add_NewClientError(t *testing.T) {
	resetMRCommentCmdFlags(t)
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "comment", "add", "--project", "g/p", "1", "--body", "hi"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMRComment_Add_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"400 Bad Request"}`))
	}))
	defer srv.Close()
	resetMRCommentCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "comment", "add", "--project", "g/p", "1", "--body", "hi"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestMRComment_List_MissingProject(t *testing.T) {
	resetMRCommentCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = mrCommentListCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"mr", "comment", "list", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMRComment_List_InvalidIID(t *testing.T) {
	resetMRCommentCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "comment", "list", "--project", "g/p", "abc"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMRComment_List_InvalidLimit(t *testing.T) {
	resetMRCommentCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "comment", "list", "--project", "g/p", "1", "--limit", "0"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMRComment_List_NewClientError(t *testing.T) {
	resetMRCommentCmdFlags(t)
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "comment", "list", "--project", "g/p", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMRComment_List_SkipsSystemNotes_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"body":"hello","system":false,"author":{"username":"alice"},"created_at":"2024-01-01"},{"id":2,"body":"system","system":true,"author":{"username":"gitlab"},"created_at":"2024-01-01"}]`)
	}))
	defer srv.Close()
	resetMRCommentCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"mr", "comment", "list", "--project", "g/p", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "hello") || strings.Contains(out, "system") {
		t.Errorf("expected user comment only, got:\n%s", out)
	}
}

func TestMRComment_Delete_MissingProject(t *testing.T) {
	resetMRCommentCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = mrCommentDeleteCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"mr", "comment", "delete", "1", "--note-id", "10"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMRComment_Delete_InvalidIID(t *testing.T) {
	resetMRCommentCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "comment", "delete", "--project", "g/p", "abc", "--note-id", "10"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMRComment_Delete_MissingNoteID(t *testing.T) {
	resetMRCommentCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = mrCommentDeleteCmd.Flags().Set("note-id", "0")
	rootCmd.SetArgs([]string{"mr", "comment", "delete", "--project", "g/p", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMRComment_Delete_ConfirmRejected(t *testing.T) {
	withNonInteractiveStdin(t)
	resetMRCommentCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "comment", "delete", "--project", "g/p", "1", "--note-id", "10", "--confirm", "wrong"})
	_ = rootCmd.Execute()
	if lastExit != ExitCancelled {
		t.Errorf("exit = %d, want %d", lastExit, ExitCancelled)
	}
}

func TestMRComment_Delete_NewClientError(t *testing.T) {
	resetMRCommentCmdFlags(t)
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "comment", "delete", "--project", "g/p", "1", "--note-id", "10", "--force"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}
