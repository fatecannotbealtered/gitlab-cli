package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

func TestIssue_Help_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"issue", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"list", "get", "create", "update", "close", "reopen", "assign", "label"} {
		if !strings.Contains(out, want) {
			t.Errorf("issue --help missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestIssue_List_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"issue", "list"})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", lastExit, ExitBadArgs)
	}
}

func TestIssue_List_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"iid":1,"title":"bug","state":"opened","author":{"username":"alice"},"web_url":"http://x","created_at":"2024-01-01","updated_at":"2024-01-01"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "list", "--project", "foo/bar", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"bug"`, `"opened"`, `"alice"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestIssue_Get_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"bug","state":"opened","author":{"username":"alice"},"web_url":"http://x","created_at":"2024-01-01","updated_at":"2024-01-01"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "get", "--project", "foo/bar", "1", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"bug"`) {
		t.Errorf("expected title in output:\n%s", out)
	}
}

func TestIssue_Create_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "create", "--project", "foo/bar", "--title", "new bug", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"action"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestIssue_Update_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "update", "1", "--project", "foo/bar", "--title", "fixed", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"iid": 1`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestIssue_Close_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "close", "2", "--project", "foo/bar", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"iid": 2`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestIssue_Reopen_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "reopen", "3", "--project", "foo/bar", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"iid": 3`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestIssue_Assign_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "assign", "4", "alice", "--project", "foo/bar", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"iid": 4`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestIssue_Label_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "label", "5", "--project", "foo/bar", "--add", "bug", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"iid": 5`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestIssue_List_Fields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"iid":1,"title":"bug","state":"opened","author":{"username":"alice"},"web_url":"http://x","created_at":"2024-01-01","updated_at":"2024-01-01"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "list", "--project", "foo/bar", "--json", "--fields", "title,state"})
		_ = rootCmd.Execute()
	})
	// Reset fields flag to avoid leaking into subsequent tests.
	_ = issueListCmd.Flags().Set("fields", "")

	if !strings.Contains(out, `"title"`) || !strings.Contains(out, `"state"`) {
		t.Errorf("expected title and state fields, got:\n%s", out)
	}
	if strings.Contains(out, `"author"`) {
		t.Errorf("author field should be filtered out, got:\n%s", out)
	}
}

func TestIssueCmd_Help(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"issue", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"list", "get", "create", "update", "close", "reopen", "assign", "label", "comment"} {
		if !strings.Contains(out, want) {
			t.Errorf("issue --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestIssueList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"iid":1,"title":"Bug","state":"opened"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "list", "--project", "42", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"iid"`) {
		t.Errorf("expected JSON with iid, got: %s", out)
	}
}

func TestIssueCreate_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = true
	jsonMode = true

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "create", "--project", "42", "--title", "Test Issue", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"action": "create issue"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestIssueClose_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = true
	jsonMode = true

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "close", "5", "--project", "42", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"action": "close issue"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestIssueCommentCmd_Help(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"issue", "comment", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"add", "list", "delete"} {
		if !strings.Contains(out, want) {
			t.Errorf("issue comment --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestIssueCommentAdd_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() {
		dryRun = origDR
		jsonMode = origJM
		_ = issueCommentAddCmd.Flags().Set("body-file", "")
	}()
	dryRun = true
	jsonMode = true
	_ = issueCommentAddCmd.Flags().Set("body-file", "")

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "comment", "add", "3", "--project", "42", "--body", "hello", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"action": "add comment"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestIssue_Create_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/users"):
			_, _ = fmt.Fprint(w, `[{"id":42,"username":"alice"}]`)
		default:
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, `{"id":1,"iid":1,"title":"bug","state":"opened","author":{"username":"alice"},"web_url":"http://x","created_at":"2024-01-01","updated_at":"2024-01-01"}`)
		}
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "create", "--project", "foo/bar", "--title", "bug", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"title": "bug"`) {
		t.Errorf("expected title in output, got: %s", out)
	}
}

func TestIssue_Update_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":1,"iid":1,"title":"fixed","state":"opened","author":{"username":"alice"},"web_url":"http://x","created_at":"2024-01-01","updated_at":"2024-01-01"}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "update", "1", "--project", "foo/bar", "--title", "fixed", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"title": "fixed"`) {
		t.Errorf("expected updated title in output, got: %s", out)
	}
}

func TestIssue_Close_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":1,"iid":1,"title":"bug","state":"closed","author":{"username":"alice"},"web_url":"http://x","created_at":"2024-01-01","updated_at":"2024-01-01"}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "close", "1", "--project", "foo/bar", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"closed"`) {
		t.Errorf("expected closed state in output, got: %s", out)
	}
}

func TestIssue_Reopen_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":1,"iid":1,"title":"bug","state":"opened","author":{"username":"alice"},"web_url":"http://x","created_at":"2024-01-01","updated_at":"2024-01-01"}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "reopen", "1", "--project", "foo/bar", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"opened"`) {
		t.Errorf("expected opened state in output, got: %s", out)
	}
}

func TestIssue_Get_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"bug","state":"opened","author":{"username":"alice"},"web_url":"http://x","description":"a bug"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "get", "--project", "foo/bar", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "bug") {
		t.Errorf("expected issue title in plain text output:\n%s", out)
	}
}

func TestIssue_List_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"iid":1,"title":"bug","state":"opened","author":{"username":"alice"},"assignee":{"username":"bob"}}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "list", "--project", "foo/bar"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "bug") {
		t.Errorf("expected issue title in plain text output:\n%s", out)
	}
}

func TestIssue_Create_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"bug","state":"opened","author":{"username":"alice"},"web_url":"http://x","created_at":"2024-01-01","updated_at":"2024-01-01"}`))
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
		rootCmd.SetArgs([]string{"issue", "create", "--project", "foo/bar", "--title", "bug"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Created") || !strings.Contains(out, "bug") {
		t.Errorf("expected 'Created' and title in output, got: %s", out)
	}
}

func TestIssue_Update_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"fixed","state":"opened","author":{"username":"alice"},"web_url":"http://x","created_at":"2024-01-01","updated_at":"2024-01-01"}`))
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
		rootCmd.SetArgs([]string{"issue", "update", "1", "--project", "foo/bar", "--title", "fixed"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Updated") {
		t.Errorf("expected 'Updated' in output, got: %s", out)
	}
}

func TestIssue_Close_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"bug","state":"closed","author":{"username":"alice"},"web_url":"http://x","created_at":"2024-01-01","updated_at":"2024-01-01"}`))
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
		rootCmd.SetArgs([]string{"issue", "close", "1", "--project", "foo/bar"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Closed") {
		t.Errorf("expected 'Closed' in output, got: %s", out)
	}
}

func TestIssue_Reopen_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"bug","state":"opened","author":{"username":"alice"},"web_url":"http://x","created_at":"2024-01-01","updated_at":"2024-01-01"}`))
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
		rootCmd.SetArgs([]string{"issue", "reopen", "1", "--project", "foo/bar"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Reopened") {
		t.Errorf("expected 'Reopened' in output, got: %s", out)
	}
}

func TestIssue_Assign_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/users") {
			_, _ = w.Write([]byte(`[{"id":42,"username":"alice","name":"Alice","state":"active"}]`))
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"bug","state":"opened","author":{"username":"alice"},"web_url":"http://x","created_at":"2024-01-01","updated_at":"2024-01-01"}`))
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
		rootCmd.SetArgs([]string{"issue", "assign", "1", "alice", "--project", "foo/bar"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Assigned") {
		t.Errorf("expected 'Assigned' in output, got: %s", out)
	}
}

func TestIssue_Label_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"bug","state":"opened","labels":["bug"],"author":{"username":"alice"},"web_url":"http://x","created_at":"2024-01-01","updated_at":"2024-01-01"}`))
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
		rootCmd.SetArgs([]string{"issue", "label", "1", "--project", "foo/bar", "--add", "bug"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Updated") {
		t.Errorf("expected 'Updated' in output, got: %s", out)
	}
}

func TestIssue_Assign_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/users") {
			_, _ = w.Write([]byte(`[{"id":42,"username":"alice","name":"Alice","state":"active"}]`))
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"bug","state":"opened","author":{"username":"alice"},"web_url":"http://x","created_at":"2024-01-01","updated_at":"2024-01-01"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "assign", "1", "alice", "--project", "foo/bar", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"iid"`) {
		t.Errorf("expected iid in output, got: %s", out)
	}
}

func TestIssue_Label_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"bug","state":"opened","labels":["bug"],"author":{"username":"alice"},"web_url":"http://x","created_at":"2024-01-01","updated_at":"2024-01-01"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "label", "1", "--project", "foo/bar", "--add", "bug", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"iid"`) {
		t.Errorf("expected iid in output, got: %s", out)
	}
}

func TestIssue_List_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"issue", "list", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestIssue_Get_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origJM := jsonMode
	defer func() { lastExit = origExit; jsonMode = origJM }()
	lastExit = 0
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	rootCmd.SetArgs([]string{"issue", "get", "--project", "foo/bar", "999"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestIssue_List_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "list", "--project", "foo/bar"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No issues") {
		t.Errorf("expected 'No issues' in output, got: %s", out)
	}
}

func TestIssue_Close_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = issueCloseCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"issue", "close", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssue_Reopen_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = issueReopenCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"issue", "reopen", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssue_Update_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = issueUpdateCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"issue", "update", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestReadBodyFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + string(os.PathSeparator) + "body.txt"
	if err := os.WriteFile(path, []byte("hello\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readBodyFile(path)
	if err != nil {
		t.Fatalf("readBodyFile: %v", err)
	}
	if got != "hello" {
		t.Fatalf("readBodyFile = %q, want %q", got, "hello")
	}

	_, err = readBodyFile(dir + string(os.PathSeparator) + "missing.txt")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestUsernameOf(t *testing.T) {
	if usernameOf(nil) != "" {
		t.Fatalf("usernameOf(nil) = %q, want empty", usernameOf(nil))
	}
	if usernameOf(&api.User{Username: "alice"}) != "alice" {
		t.Fatalf("usernameOf = %q, want alice", usernameOf(&api.User{Username: "alice"}))
	}
}

func TestToFlatIssue(t *testing.T) {
	iss := &api.Issue{
		IID:      3,
		Title:    "bug",
		State:    "opened",
		Labels:   []string{"a", "b"},
		WebURL:   "http://x",
		Author:   &api.User{Username: "alice"},
		Assignee: &api.User{Username: "bob"},
		Milestone: &struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
		}{ID: 1, Title: "M1"},
	}
	flat := toFlatIssue(iss)
	if flat.Author != "alice" || flat.Assignee != "bob" || flat.Milestone != "M1" {
		t.Fatalf("toFlatIssue = %+v", flat)
	}
	if flat.Labels != "a,b" {
		t.Fatalf("labels = %q, want a,b", flat.Labels)
	}

	minimal := toFlatIssue(&api.Issue{IID: 1, Title: "x", State: "opened"})
	if minimal.Author != "" || minimal.Assignee != "" || minimal.Milestone != "" {
		t.Fatalf("expected empty optional fields, got %+v", minimal)
	}
}

func TestPrintIssueSummary(t *testing.T) {
	iss := &api.Issue{
		IID:         5,
		Title:       "summary test",
		State:       "opened",
		Labels:      []string{"bug", "urgent"},
		WebURL:      "http://example/issue/5",
		Description: "details here",
		Author:      &api.User{Username: "alice"},
		Assignee:    &api.User{Username: "bob"},
		Milestone: &struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
		}{Title: "v1.0"},
	}
	out := captureStdout(t, func() {
		printIssueSummary(iss)
	})
	for _, want := range []string{
		"Issue #5",
		"summary test",
		"alice",
		"bob",
		"bug, urgent",
		"v1.0",
		"http://example/issue/5",
		"details here",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("printIssueSummary missing %q in:\n%s", want, out)
		}
	}
}

func testIssueMissingAuth(t *testing.T, args ...string) {
	t.Helper()
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(args)
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d (ExitAuth)", lastExit, ExitAuth)
	}
}

func TestIssue_List_MissingAuth(t *testing.T) {
	testIssueMissingAuth(t, "issue", "list", "--project", "foo/bar")
}

func TestIssue_List_InvalidLimit(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = issueListCmd.Flags().Set("limit", "20") }()
	lastExit = 0
	_ = issueListCmd.Flags().Set("limit", "0")
	rootCmd.SetArgs([]string{"issue", "list", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssue_Get_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = issueGetCmd.Flags().Set("project", "") }()
	lastExit = 0
	_ = issueGetCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"issue", "get", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssue_Get_InvalidIID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"issue", "get", "--project", "foo/bar", "abc"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssue_Get_MissingAuth(t *testing.T) {
	testIssueMissingAuth(t, "issue", "get", "--project", "foo/bar", "1")
}

func TestIssue_Create_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = issueCreateCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"issue", "create", "--title", "bug"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssue_Create_MissingTitle(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR; _ = issueCreateCmd.Flags().Set("title", "") }()
	lastExit = 0
	dryRun = false
	_ = issueCreateCmd.Flags().Set("title", "")
	rootCmd.SetArgs([]string{"issue", "create", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssue_Create_MissingAuth(t *testing.T) {
	testIssueMissingAuth(t, "issue", "create", "--project", "foo/bar", "--title", "bug")
}

func TestIssue_Create_WithAssignee(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/users") {
			_, _ = w.Write([]byte(`[{"id":42,"username":"alice"}]`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"bug","state":"opened","web_url":"http://x"}`))
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
		rootCmd.SetArgs([]string{"issue", "create", "--project", "foo/bar", "--title", "bug", "--assignee", "alice"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Created") {
		t.Errorf("expected Created in output, got: %s", out)
	}
}

func TestIssue_Create_AssigneeNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origExit := lastExit
	defer func() { dryRun = origDR; lastExit = origExit }()
	dryRun = false
	lastExit = 0
	rootCmd.SetArgs([]string{"issue", "create", "--project", "foo/bar", "--title", "bug", "--assignee", "ghost"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit when assignee not found, got %d", lastExit)
	}
}

func TestIssue_Create_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"500"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origExit := lastExit
	defer func() {
		dryRun = origDR
		lastExit = origExit
		_ = issueCreateCmd.Flags().Set("assignee", "")
	}()
	dryRun = false
	lastExit = 0
	_ = issueCreateCmd.Flags().Set("assignee", "")
	rootCmd.SetArgs([]string{"issue", "create", "--project", "foo/bar", "--title", "bug"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestIssue_Create_APIError_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"500"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origExit := lastExit
	origJM := jsonMode
	defer func() {
		dryRun = origDR
		lastExit = origExit
		jsonMode = origJM
		_ = issueCreateCmd.Flags().Set("assignee", "")
	}()
	dryRun = false
	lastExit = 0
	jsonMode = true
	_ = issueCreateCmd.Flags().Set("assignee", "")
	rootCmd.SetArgs([]string{"issue", "create", "--project", "foo/bar", "--title", "bug", "--json"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestIssue_Update_APIError_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"500"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origExit := lastExit
	origJM := jsonMode
	defer func() { dryRun = origDR; lastExit = origExit; jsonMode = origJM }()
	dryRun = false
	lastExit = 0
	jsonMode = true
	rootCmd.SetArgs([]string{"issue", "update", "1", "--project", "foo/bar", "--title", "x", "--json"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestIssue_Close_ConfirmCancelled(t *testing.T) {
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
	rootCmd.SetArgs([]string{"issue", "close", "1", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitCancelled {
		t.Errorf("exit code = %d, want %d", lastExit, ExitCancelled)
	}
}

func TestIssue_Update_InvalidIID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	rootCmd.SetArgs([]string{"issue", "update", "x", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssue_Update_MissingAuth(t *testing.T) {
	testIssueMissingAuth(t, "issue", "update", "1", "--project", "foo/bar", "--title", "x")
}

func TestIssue_Update_WithAssignee(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/users") {
			_, _ = w.Write([]byte(`[{"id":42,"username":"alice"}]`))
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"bug","state":"opened","web_url":"http://x"}`))
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
		rootCmd.SetArgs([]string{"issue", "update", "1", "--project", "foo/bar", "--assignee", "alice"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Updated") {
		t.Errorf("expected Updated in output, got: %s", out)
	}
}

func TestIssue_Update_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"issue", "update", "1", "--project", "foo/bar", "--title", "x"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestIssue_Close_InvalidIID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	rootCmd.SetArgs([]string{"issue", "close", "x", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssue_Close_MissingAuth(t *testing.T) {
	testIssueMissingAuth(t, "issue", "close", "1", "--project", "foo/bar", "--force")
}

func TestIssue_Close_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"issue", "close", "1", "--project", "foo/bar", "--force"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestIssue_Reopen_InvalidIID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	rootCmd.SetArgs([]string{"issue", "reopen", "x", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssue_Reopen_MissingAuth(t *testing.T) {
	testIssueMissingAuth(t, "issue", "reopen", "1", "--project", "foo/bar")
}

func TestIssue_Reopen_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"issue", "reopen", "1", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestIssue_Assign_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR; _ = issueAssignCmd.Flags().Set("project", "") }()
	lastExit = 0
	dryRun = false
	_ = issueAssignCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"issue", "assign", "1", "alice"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssue_Assign_InvalidIID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	rootCmd.SetArgs([]string{"issue", "assign", "x", "alice", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssue_Assign_MissingAuth(t *testing.T) {
	testIssueMissingAuth(t, "issue", "assign", "1", "alice", "--project", "foo/bar")
}

func TestIssue_Assign_UserNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origExit := lastExit
	defer func() { dryRun = origDR; lastExit = origExit }()
	dryRun = false
	lastExit = 0
	rootCmd.SetArgs([]string{"issue", "assign", "1", "ghost", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit when user not found, got %d", lastExit)
	}
}

func TestIssue_Assign_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/users") {
			_, _ = w.Write([]byte(`[{"id":42,"username":"alice"}]`))
			return
		}
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
	rootCmd.SetArgs([]string{"issue", "assign", "1", "alice", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestIssue_Label_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR; _ = issueLabelCmd.Flags().Set("project", "") }()
	lastExit = 0
	dryRun = false
	_ = issueLabelCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"issue", "label", "1", "--add", "bug"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssue_Label_InvalidIID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	rootCmd.SetArgs([]string{"issue", "label", "x", "--project", "foo/bar", "--add", "bug"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssue_Label_MissingAddRemove(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() {
		lastExit = origExit
		dryRun = origDR
		_ = issueLabelCmd.Flags().Set("add", "")
		_ = issueLabelCmd.Flags().Set("remove", "")
	}()
	lastExit = 0
	dryRun = false
	_ = issueLabelCmd.Flags().Set("add", "")
	_ = issueLabelCmd.Flags().Set("remove", "")
	rootCmd.SetArgs([]string{"issue", "label", "1", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestIssue_Label_MissingAuth(t *testing.T) {
	testIssueMissingAuth(t, "issue", "label", "1", "--project", "foo/bar", "--add", "bug")
}

func TestIssue_Label_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"issue", "label", "1", "--project", "foo/bar", "--add", "bug"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}
