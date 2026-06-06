package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestIssueComment_Help_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"issue", "comment", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(nil)

	out := buf.String()
	for _, want := range []string{"add", "list", "delete"} {
		if !strings.Contains(out, want) {
			t.Errorf("issue comment --help missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestIssueComment_Add_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() {
		dryRun = origDR
		jsonMode = origJM
		_ = issueCommentAddCmd.Flags().Set("body-file", "")
	}()
	_ = issueCommentAddCmd.Flags().Set("body-file", "")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"issue", "comment", "add", "1",
			"--project", "group/proj",
			"--body", "hello",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"body": "hello"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestIssueComment_List_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"body":"hello","author":{"username":"alice"},"created_at":"2024-01-01","updated_at":"2024-01-01"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "comment", "list", "--project", "group/proj", "1", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"hello"`, `"alice"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestIssueComment_Delete_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"issue", "comment", "delete", "1",
			"--project", "group/proj",
			"--note-id", "42",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"noteId": 42`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestIssueComment_Add_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":5,"body":"hello","author":{"username":"alice"},"created_at":"2024-01-01"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "comment", "add", "1", "--project", "foo/bar", "--body", "hello", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"hello"`) {
		t.Errorf("expected body in output, got: %s", out)
	}
}

func TestIssueComment_Add_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":5,"body":"hello","author":{"username":"alice"},"created_at":"2024-01-01"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	dryRun = false
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "comment", "add", "1", "--project", "foo/bar", "--body", "hello"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Added") {
		t.Errorf("expected 'Added' in output, got: %s", out)
	}
}

func TestIssueComment_List_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"body":"hello","system":false,"author":{"username":"alice"},"created_at":"2024-01-01"}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "comment", "list", "--project", "group/proj", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "hello") {
		t.Errorf("expected comment body in plain text output, got: %s", out)
	}
}

func TestIssueComment_Delete_JSON(t *testing.T) {
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
		args := []string{"issue", "comment", "delete", "1", "--project", "foo/bar", "--note-id", "5", "--json"}
		args = append(args, confirmArgsForTest(t, "delete comment", map[string]any{"project": "foo/bar", "iid": 1, "noteId": 5})...)
		rootCmd.SetArgs(args)
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
}

func TestIssueComment_Add_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = issueCommentAddCmd.Flags().Set("project", "")
	_ = issueCommentAddCmd.Flags().Set("body", "")
	rootCmd.SetArgs([]string{"issue", "comment", "add", "1", "--body", "hi"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssueComment_Add_MissingBody(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = issueCommentAddCmd.Flags().Set("project", "")
	_ = issueCommentAddCmd.Flags().Set("body", "")
	rootCmd.SetArgs([]string{"issue", "comment", "add", "1", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssueComment_List_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = issueCommentListCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"issue", "comment", "list", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssueComment_Delete_MissingNoteID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = issueCommentDeleteCmd.Flags().Set("note-id", "0")
	rootCmd.SetArgs([]string{"issue", "comment", "delete", "1", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssueComment_Delete_PlainText(t *testing.T) {
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
	setTextFormatForTest(t)
	lastExit = 0
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		args := []string{"issue", "comment", "delete", "1", "--project", "foo/bar", "--note-id", "5"}
		args = append(args, confirmArgsForTest(t, "delete comment", map[string]any{"project": "foo/bar", "iid": 1, "noteId": 5})...)
		rootCmd.SetArgs(args)
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Deleted") {
		t.Errorf("expected 'Deleted' in output, got: %s", out)
	}
}

func TestIssueComment_Add_InvalidIID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	rootCmd.SetArgs([]string{"issue", "comment", "add", "x", "--project", "foo/bar", "--body", "hi"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssueComment_Add_BodyFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + string(os.PathSeparator) + "body.txt"
	if err := os.WriteFile(path, []byte("from file"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":5,"body":"from file","author":{"username":"alice"},"created_at":"2024-01-01"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	dryRun = false
	setTextFormatForTest(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "comment", "add", "1", "--project", "foo/bar", "--body-file", path})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Added") {
		t.Errorf("expected Added in output, got: %s", out)
	}
}

func TestIssueComment_Add_BodyFileError(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	rootCmd.SetArgs([]string{"issue", "comment", "add", "1", "--project", "foo/bar", "--body-file", "/no/such/file.txt"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssueComment_Add_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	origDR := dryRun
	defer func() {
		lastExit = origExit
		dryRun = origDR
		_ = issueCommentAddCmd.Flags().Set("body-file", "")
		_ = issueCommentAddCmd.Flags().Set("body", "")
	}()
	lastExit = 0
	dryRun = false
	_ = issueCommentAddCmd.Flags().Set("body-file", "")
	_ = issueCommentAddCmd.Flags().Set("body", "")
	rootCmd.SetArgs([]string{"issue", "comment", "add", "1", "--project", "foo/bar", "--body", "hi"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestIssueComment_Add_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"issue", "comment", "add", "1", "--project", "foo/bar", "--body", "hi"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestIssueComment_List_InvalidIID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"issue", "comment", "list", "--project", "foo/bar", "x"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssueComment_List_InvalidLimit(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = issueCommentListCmd.Flags().Set("limit", "20") }()
	lastExit = 0
	_ = issueCommentListCmd.Flags().Set("limit", "0")
	rootCmd.SetArgs([]string{"issue", "comment", "list", "--project", "foo/bar", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssueComment_List_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"issue", "comment", "list", "--project", "foo/bar", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestIssueComment_List_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"500"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"issue", "comment", "list", "--project", "foo/bar", "1"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestIssueComment_List_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "comment", "list", "--project", "foo/bar", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No comments") {
		t.Errorf("expected No comments in output, got: %s", out)
	}
}

func TestIssueComment_List_SkipsSystemNotes(t *testing.T) {
	longBody := strings.Repeat("x", 100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":1,"body":"` + longBody + `","system":false,"author":{"username":"alice"},"created_at":"2024-01-01"},
			{"id":2,"body":"system","system":true,"author":{"username":"gitlab"},"created_at":"2024-01-01"}
		]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "comment", "list", "--project", "foo/bar", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "...") {
		t.Errorf("expected truncated body in output, got: %s", out)
	}
	if strings.Contains(out, "system") {
		t.Errorf("system note should be skipped, got: %s", out)
	}
}

func TestIssueComment_List_JSON_SkipsSystem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"id":1,"body":"hello","system":false,"author":{"username":"alice"},"created_at":"2024-01-01"},
			{"id":2,"body":"system","system":true,"author":{"username":"gitlab"},"created_at":"2024-01-01"}
		]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "comment", "list", "--project", "foo/bar", "1", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"hello"`) {
		t.Errorf("expected user comment in JSON, got: %s", out)
	}
	if strings.Contains(out, `"system"`) {
		t.Errorf("system note should be skipped in JSON, got: %s", out)
	}
}

func TestIssueComment_Delete_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR; _ = issueCommentDeleteCmd.Flags().Set("project", "") }()
	lastExit = 0
	dryRun = false
	_ = issueCommentDeleteCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"issue", "comment", "delete", "1", "--note-id", "5"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssueComment_Delete_InvalidIID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	rootCmd.SetArgs([]string{"issue", "comment", "delete", "x", "--project", "foo/bar", "--note-id", "5"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssueComment_Delete_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	args := []string{"issue", "comment", "delete", "1", "--project", "foo/bar", "--note-id", "5"}
	args = append(args, confirmArgsForTest(t, "delete comment", map[string]any{"project": "foo/bar", "iid": 1, "noteId": 5})...)
	rootCmd.SetArgs(args)
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestIssueComment_Delete_ConfirmCancelled(t *testing.T) {
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
	rootCmd.SetArgs([]string{"issue", "comment", "delete", "1", "--project", "foo/bar", "--note-id", "5"})
	_ = rootCmd.Execute()
	if lastExit != ExitCancelled {
		t.Errorf("exit code = %d, want %d", lastExit, ExitCancelled)
	}
}

func TestIssueComment_Delete_APIError(t *testing.T) {
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
	args := []string{"issue", "comment", "delete", "1", "--project", "foo/bar", "--note-id", "5"}
	args = append(args, confirmArgsForTest(t, "delete comment", map[string]any{"project": "foo/bar", "iid": 1, "noteId": 5})...)
	rootCmd.SetArgs(args)
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}
