package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func mrCloseConfirmArgsForTest(t *testing.T, project string, iid int) []string {
	t.Helper()
	return confirmArgsForTest(t, "close mr", map[string]any{"project": project, "iid": iid})
}

func mrMergeConfirmArgsForTest(t *testing.T, project string, iid int) []string {
	t.Helper()
	return confirmArgsForTest(t, "merge mr", map[string]any{
		"project":                  project,
		"iid":                      iid,
		"squash":                   false,
		"shouldRemoveSourceBranch": false,
		"mergeCommitMessage":       "",
		"sha":                      "",
	})
}

func TestMR_Help_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"mr", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(nil)

	out := buf.String()
	for _, want := range []string{
		"list", "get", "current", "create", "update",
		"merge", "close", "reopen", "approve", "unapprove", "diff", "comment",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("mr --help missing subcommand %q\noutput:\n%s", want, out)
		}
	}
}

func TestMR_Comment_Help_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"mr", "comment", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(nil)

	out := buf.String()
	for _, want := range []string{"add", "list", "delete"} {
		if !strings.Contains(out, want) {
			t.Errorf("mr comment --help missing subcommand %q\noutput:\n%s", want, out)
		}
	}
}

func TestMR_Create_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"mr", "create",
			"--dry-run", "--json",
			"--project", "group/proj",
			"--title", "my feature",
			"--source-branch", "feat/x",
		})
		_ = rootCmd.Execute()
	})

	for _, want := range []string{`"confirm_token"`, `"action"`, `"project"`} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run JSON missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestMR_List_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"mr", "list"})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", lastExit, ExitBadArgs)
	}
}

func TestMR_Get_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"mr", "get", "1"})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", lastExit, ExitBadArgs)
	}
}

func TestMR_Close_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"mr", "close", "5",
			"--dry-run", "--json",
			"--project", "group/proj",
		})
		_ = rootCmd.Execute()
	})

	for _, want := range []string{`"confirm_token"`, `"iid": "5"`} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run JSON missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestMR_Merge_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"mr", "merge", "7",
			"--dry-run", "--json",
			"--project", "group/proj",
		})
		_ = rootCmd.Execute()
	})

	for _, want := range []string{`"confirm_token"`, `"iid": "7"`} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run JSON missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestMR_CommentAdd_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"mr", "comment", "add",
			"--dry-run", "--json",
			"--project", "group/proj",
			"3",
			"--body", "LGTM",
		})
		_ = rootCmd.Execute()
	})

	for _, want := range []string{`"confirm_token"`, `"body": "LGTM"`} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run JSON missing %q\noutput:\n%s", want, out)
		}
	}
}

// ── new tests ──────────────────────────────────────────────────────────────────

func TestMR_List_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"iid":1,"title":"feat","state":"opened","source_branch":"feat","target_branch":"main","web_url":"http://x","author":{"username":"alice"}}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "list", "--project", "group/proj", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"feat"`, `"opened"`, `"alice"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestMR_Get_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"feat","state":"opened","source_branch":"feat","target_branch":"main","web_url":"http://x","author":{"username":"alice"}}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "get", "--project", "group/proj", "1", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"webUrl"`) {
		t.Errorf("expected camelCase webUrl key, got:\n%s", out)
	}
}

func TestMR_Update_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "update", "1", "--project", "group/proj", "--title", "new title", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"iid": "1"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestMR_Approve_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "approve", "1", "--project", "group/proj", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"iid": "1"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestMR_Unapprove_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "unapprove", "1", "--project", "group/proj", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"iid": "1"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestMR_Reopen_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "reopen", "1", "--project", "group/proj", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"iid": "1"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestMR_Create_AssigneeNotFound(t *testing.T) {
	mrCreateCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/users"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/merge_requests"):
			mrCreateCalled = true
			http.Error(w, "unexpected MR create", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
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
		_ = mrCreateCmd.Flags().Set("assignee", "")
	}()
	dryRun = false
	jsonMode = true
	lastExit = 0

	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{
			"mr", "create",
			"--project", "foo/bar",
			"--title", "feat",
			"--source-branch", "feat",
			"--assignee", "ghost",
			"--json",
		}))
		_ = rootCmd.Execute()
	})

	if lastExit != ExitNotFound {
		t.Errorf("exit code = %d, want %d (ExitNotFound)", lastExit, ExitNotFound)
	}
	if !strings.Contains(out, `"E_NOT_FOUND"`) {
		t.Errorf("expected E_NOT_FOUND error code in JSON output, got:\n%s", out)
	}
	if !strings.Contains(out, "ghost") || !strings.Contains(out, "not found") {
		t.Errorf("expected user not found message in output, got:\n%s", out)
	}
	if mrCreateCalled {
		t.Error("MR create API should not be called when assignee does not exist")
	}
}

func TestMR_Create_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/users"):
			_, _ = w.Write([]byte(`{"id":42,"username":"alice"}`))
		default:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"feat","state":"opened","source_branch":"feat","target_branch":"main","web_url":"http://x","author":{"username":"alice"},"created_at":"2024-01-01","updated_at":"2024-01-01"}`))
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
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "create", "--project", "foo/bar", "--title", "feat", "--source-branch", "feat", "--json"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"title": "feat"`) {
		t.Errorf("expected title in output, got: %s", out)
	}
}

func TestMR_Update_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"updated","state":"opened","source_branch":"feat","target_branch":"main","web_url":"http://x","author":{"username":"alice"},"created_at":"2024-01-01","updated_at":"2024-01-01"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "update", "1", "--project", "foo/bar", "--title", "updated", "--json"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"title": "updated"`) {
		t.Errorf("expected updated title in output, got: %s", out)
	}
}

func TestMR_Close_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"feat","state":"closed","source_branch":"feat","target_branch":"main","web_url":"http://x","author":{"username":"alice"},"created_at":"2024-01-01","updated_at":"2024-01-01"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		args := []string{"mr", "close", "1", "--project", "foo/bar", "--json"}
		args = append(args, mrCloseConfirmArgsForTest(t, "foo/bar", 1)...)
		rootCmd.SetArgs(args)
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"closed"`) {
		t.Errorf("expected closed state in output, got: %s", out)
	}
}

func TestMR_Approve_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/approve") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"approved":true}`))
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
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "approve", "1", "--project", "foo/bar", "--json"}))
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
}

func TestMR_Get_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"feat","state":"opened","source_branch":"feat","target_branch":"main","web_url":"http://x","author":{"username":"alice"}}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "get", "--project", "group/proj", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "feat") {
		t.Errorf("expected MR title in plain text output:\n%s", out)
	}
}

func TestMR_List_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"iid":1,"title":"feat","state":"opened","source_branch":"feat","target_branch":"main","author":{"username":"alice"}}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "list", "--project", "group/proj"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "feat") {
		t.Errorf("expected MR title in plain text output:\n%s", out)
	}
}

func TestMR_Merge_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"feat","state":"merged","source_branch":"feat","target_branch":"main","web_url":"http://x","author":{"username":"alice"}}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		args := []string{"mr", "merge", "1", "--project", "foo/bar", "--json"}
		args = append(args, mrMergeConfirmArgsForTest(t, "foo/bar", 1)...)
		rootCmd.SetArgs(args)
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"merged"`) {
		t.Errorf("expected merged state in output, got: %s", out)
	}
}

func TestMR_Reopen_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"feat","state":"opened","source_branch":"feat","target_branch":"main","web_url":"http://x","author":{"username":"alice"}}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "reopen", "1", "--project", "foo/bar", "--json"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"opened"`) {
		t.Errorf("expected opened state in output, got: %s", out)
	}
}

func TestMR_Create_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/users"):
			_, _ = w.Write([]byte(`{"id":42,"username":"alice"}`))
		default:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"feat","state":"opened","source_branch":"feat","target_branch":"main","web_url":"http://x","author":{"username":"alice"},"created_at":"2024-01-01","updated_at":"2024-01-01"}`))
		}
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
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "create", "--project", "foo/bar", "--title", "feat", "--source-branch", "feat"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Created") {
		t.Errorf("expected 'Created' in output, got: %s", out)
	}
}

func TestMR_Update_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"updated","state":"opened","source_branch":"feat","target_branch":"main","web_url":"http://x","author":{"username":"alice"},"created_at":"2024-01-01","updated_at":"2024-01-01"}`))
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
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "update", "1", "--project", "foo/bar", "--title", "updated"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Updated") {
		t.Errorf("expected 'Updated' in output, got: %s", out)
	}
}

func TestMR_Close_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"feat","state":"closed","source_branch":"feat","target_branch":"main","web_url":"http://x","author":{"username":"alice"}}`))
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
		args := []string{"mr", "close", "1", "--project", "foo/bar"}
		args = append(args, mrCloseConfirmArgsForTest(t, "foo/bar", 1)...)
		rootCmd.SetArgs(args)
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Closed") {
		t.Errorf("expected 'Closed' in output, got: %s", out)
	}
}

func TestMR_Merge_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"feat","state":"merged","source_branch":"feat","target_branch":"main","web_url":"http://x","author":{"username":"alice"}}`))
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
		args := []string{"mr", "merge", "1", "--project", "foo/bar"}
		args = append(args, mrMergeConfirmArgsForTest(t, "foo/bar", 1)...)
		rootCmd.SetArgs(args)
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Merged") {
		t.Errorf("expected 'Merged' in output, got: %s", out)
	}
}

func TestMR_Reopen_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"feat","state":"opened","source_branch":"feat","target_branch":"main","web_url":"http://x","author":{"username":"alice"}}`))
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
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "reopen", "1", "--project", "foo/bar"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Reopened") {
		t.Errorf("expected 'Reopened' in output, got: %s", out)
	}
}

func TestMR_Approve_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/approve") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"approved":true}`))
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
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "approve", "1", "--project", "foo/bar"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Approved") {
		t.Errorf("expected 'Approved' in output, got: %s", out)
	}
}

func TestMR_Unapprove_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/unapprove") {
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
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "unapprove", "1", "--project", "foo/bar"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "approval") {
		t.Errorf("expected 'approval' in output, got: %s", out)
	}
}

func TestMR_Diff_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("diff --git a/file.go b/file.go\n+added line\n"))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "diff", "1", "--project", "foo/bar"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "diff") {
		t.Errorf("expected diff content in output, got: %s", out)
	}
}

func TestMR_List_Empty(t *testing.T) {
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
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "list", "--project", "foo/bar"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No merge requests") {
		t.Errorf("expected 'No merge requests' in output, got: %s", out)
	}
}

func TestMR_Current_NotInGitRepo(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	// Run from a temp dir that's not a git repo
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()
	rootCmd.SetArgs([]string{"mr", "current"})
	_ = rootCmd.Execute()
	// Should fail with ExitNotFound since not in a git repo
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit when not in git repo, got %d", lastExit)
	}
}

func TestMR_Create_Auto_NotInGitRepo(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()
	rootCmd.SetArgs([]string{"mr", "create", "--auto"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit when not in git repo, got %d", lastExit)
	}
}

func TestMR_List_APIError(t *testing.T) {
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
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	rootCmd.SetArgs([]string{"mr", "list", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestMR_Get_APIError(t *testing.T) {
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
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	rootCmd.SetArgs([]string{"mr", "get", "--project", "foo/bar", "999"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestMR_Merge_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = mrMergeCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"mr", "merge", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Reopen_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = mrReopenCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"mr", "reopen", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Approve_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = mrApproveCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"mr", "approve", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Unapprove_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = mrUnapproveCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"mr", "unapprove", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Diff_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = mrDiffCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"mr", "diff", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Close_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = mrCloseCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"mr", "close", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Update_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = mrUpdateCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"mr", "update", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Merge_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"message":"405 Method Not Allowed"}`))
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
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	rootCmd.SetArgs([]string{"mr", "merge", "1", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestGitCommitSubject_InRepo(t *testing.T) {
	subject := gitCommitSubject()
	if subject == "" {
		t.Fatal("expected non-empty commit subject in git repo")
	}
}

func TestGitCommitSubject_NoGitRepo(t *testing.T) {
	dir := t.TempDir()
	defer mrChdir(t, dir)()
	if got := gitCommitSubject(); got != "" {
		t.Fatalf("gitCommitSubject outside repo = %q, want empty", got)
	}
}

func TestMR_List_All_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"iid":1,"title":"feat","state":"opened","source_branch":"feat","target_branch":"main","web_url":"http://x"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "list", "--project", "group/proj", "--all", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"all": true`) {
		t.Errorf("expected all=true in list envelope, got:\n%s", out)
	}
}

func mrRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}

func mrInitRepo(t *testing.T, remoteURL, branch string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	mrRunGit(t, dir, "init", "--initial-branch=main")
	mrRunGit(t, dir, "config", "user.email", "test@example.com")
	mrRunGit(t, dir, "config", "user.name", "Test")
	mrRunGit(t, dir, "config", "commit.gpgsign", "false")
	mrRunGit(t, dir, "commit", "--allow-empty", "-m", "init")
	if branch != "" && branch != "main" {
		mrRunGit(t, dir, "checkout", "-b", branch)
	}
	if remoteURL != "" {
		mrRunGit(t, dir, "remote", "add", "origin", remoteURL)
	}
	return dir
}

func mrChdir(t *testing.T, dir string) func() {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	return func() { _ = os.Chdir(orig) }
}

func mrNoAuth(t *testing.T) {
	t.Helper()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")
}

func resetMRCmdFlags(t *testing.T) {
	t.Helper()
	for _, kv := range []struct {
		cmd   *cobra.Command
		name  string
		value string
	}{
		{mrListCmd, "project", ""},
		{mrListCmd, "limit", "20"},
		{mrListCmd, "all", "false"},
		{mrGetCmd, "project", ""},
		{mrCreateCmd, "project", ""},
		{mrCreateCmd, "title", ""},
		{mrCreateCmd, "source-branch", ""},
		{mrCreateCmd, "target-branch", "main"},
		{mrCreateCmd, "auto", "false"},
		{mrCreateCmd, "find-existing", "false"},
		{mrCreateCmd, "assignee", ""},
		{mrUpdateCmd, "project", ""},
		{mrUpdateCmd, "assignee", ""},
		{mrMergeCmd, "project", ""},
		{mrCloseCmd, "project", ""},
		{mrReopenCmd, "project", ""},
		{mrApproveCmd, "project", ""},
		{mrUnapproveCmd, "project", ""},
		{mrDiffCmd, "project", ""},
	} {
		if err := kv.cmd.Flags().Set(kv.name, kv.value); err != nil {
			t.Fatalf("reset mr flag %s.%s: %v", kv.cmd.Name(), kv.name, err)
		}
	}
}

func TestMR_List_NewClientError(t *testing.T) {
	mrNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "list", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMR_List_InvalidLimit(t *testing.T) {
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "list", "--project", "g/p", "--limit", "0"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_List_PaginationHint_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"iid":1,"title":"a","state":"opened","source_branch":"a","target_branch":"main"},{"id":2,"iid":2,"title":"b","state":"opened","source_branch":"b","target_branch":"main"}]`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "list", "--project", "g/p", "--limit", "1", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"hasMore": true`) {
		t.Errorf("expected hasMore in list envelope, got:\n%s", out)
	}
}

func TestMR_Get_NewClientError(t *testing.T) {
	mrNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "get", "--project", "g/p", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMR_Get_InvalidIID(t *testing.T) {
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "get", "--project", "g/p", "abc"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Current_Success_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"iid":3,"title":"feat","state":"opened","source_branch":"feat/x","target_branch":"main","web_url":"http://x","author":{"username":"alice"}}]`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	repo := mrInitRepo(t, srv.URL+"/group/proj.git", "feat/x")
	defer mrChdir(t, repo)()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "current", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"iid": "3"`) {
		t.Errorf("expected MR JSON, got:\n%s", out)
	}
}

func TestMR_Current_Success_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"iid":3,"title":"feat","state":"opened","source_branch":"feat/x","target_branch":"main","web_url":"http://x","author":{"username":"alice"}}]`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	repo := mrInitRepo(t, srv.URL+"/group/proj.git", "feat/x")
	defer mrChdir(t, repo)()
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"mr", "current"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "feat") {
		t.Errorf("expected MR detail in output, got:\n%s", out)
	}
}

func TestMR_Current_NoOpenMR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	repo := mrInitRepo(t, srv.URL+"/group/proj.git", "feat/x")
	defer mrChdir(t, repo)()
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"mr", "current"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitNotFound {
		t.Errorf("exit = %d, want %d", lastExit, ExitNotFound)
	}
	if !strings.Contains(out, `"E_NOT_FOUND"`) || !strings.Contains(out, "no open MR") {
		t.Errorf("expected no MR JSON error, got:\n%s", out)
	}
}

func TestMR_Current_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	repo := mrInitRepo(t, srv.URL+"/group/proj.git", "feat/x")
	defer mrChdir(t, repo)()
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "current"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestMR_Create_Auto_Success(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"init","state":"opened","source_branch":"feat/x","target_branch":"main","web_url":"http://x"}`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	repo := mrInitRepo(t, srv.URL+"/group/proj.git", "feat/x")
	defer mrChdir(t, repo)()
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "create", "--auto"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Created") {
		t.Errorf("expected Created in output, got:\n%s", out)
	}
}

func TestMR_Create_MissingProject(t *testing.T) {
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	rootCmd.SetArgs([]string{"mr", "create", "--title", "t", "--source-branch", "b"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Create_MissingTitle(t *testing.T) {
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "create", "--project", "g/p", "--source-branch", "b"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Create_MissingSourceBranch(t *testing.T) {
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "create", "--project", "g/p", "--title", "t"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Create_FindExisting_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"iid":5,"title":"existing","state":"opened","source_branch":"feat","target_branch":"main","web_url":"http://x"}]`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{
			"mr", "create", "--find-existing", "--json",
			"--project", "g/p", "--title", "t", "--source-branch", "feat",
		}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"existing": true`) {
		t.Errorf("expected existing flag in JSON, got:\n%s", out)
	}
}

func TestMR_Create_FindExisting_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"iid":5,"title":"existing","state":"opened","source_branch":"feat","target_branch":"main","web_url":"http://x"}]`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{
			"mr", "create", "--find-existing",
			"--project", "g/p", "--title", "t", "--source-branch", "feat",
		}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Existing MR !5") {
		t.Errorf("expected existing MR message, got:\n%s", out)
	}
}

func TestMR_Create_WithAssignee(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/users") {
			_, _ = w.Write([]byte(`[{"id":42,"username":"alice"}]`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"feat","state":"opened","source_branch":"feat","target_branch":"main","web_url":"http://x"}`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{
			"mr", "create",
			"--project", "g/p", "--title", "feat", "--source-branch", "feat", "--assignee", "alice",
		}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Created") {
		t.Errorf("expected Created in output, got:\n%s", out)
	}
}

func TestMR_Create_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"400 Bad Request"}`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "create", "--project", "g/p", "--title", "t", "--source-branch", "b"}))
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestMR_Update_NewClientError(t *testing.T) {
	mrNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "update", "1", "--project", "g/p", "--title", "x"}))
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMR_Update_WithAssignee(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/users") {
			_, _ = w.Write([]byte(`[{"id":42,"username":"alice"}]`))
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"updated","state":"opened","source_branch":"feat","target_branch":"main","web_url":"http://x"}`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "update", "1", "--project", "g/p", "--assignee", "alice"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Updated") {
		t.Errorf("expected Updated in output, got:\n%s", out)
	}
}

func TestMR_Update_AssigneeAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "update", "1", "--project", "g/p", "--assignee", "ghost"}))
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestMR_Update_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"400 Bad Request"}`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "update", "1", "--project", "g/p", "--title", "x"}))
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestMR_Merge_WithConfirm(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"feat","state":"merged","source_branch":"feat","target_branch":"main","web_url":"http://x"}`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	out := captureCombinedOutput(t, func() {
		args := []string{"mr", "merge", "1", "--project", "g/p"}
		args = append(args, mrMergeConfirmArgsForTest(t, "g/p", 1)...)
		rootCmd.SetArgs(args)
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Merged") {
		t.Errorf("expected Merged in output, got:\n%s", out)
	}
}

func TestMR_Merge_NewClientError(t *testing.T) {
	mrNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	args := []string{"mr", "merge", "1", "--project", "g/p"}
	args = append(args, mrMergeConfirmArgsForTest(t, "g/p", 1)...)
	rootCmd.SetArgs(args)
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMR_Close_WithConfirm(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"title":"feat","state":"closed","source_branch":"feat","target_branch":"main","web_url":"http://x"}`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	out := captureCombinedOutput(t, func() {
		args := []string{"mr", "close", "1", "--project", "g/p"}
		args = append(args, mrCloseConfirmArgsForTest(t, "g/p", 1)...)
		rootCmd.SetArgs(args)
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Closed") {
		t.Errorf("expected Closed in output, got:\n%s", out)
	}
}

func TestMR_Close_NewClientError(t *testing.T) {
	mrNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	args := []string{"mr", "close", "1", "--project", "g/p"}
	args = append(args, mrCloseConfirmArgsForTest(t, "g/p", 1)...)
	rootCmd.SetArgs(args)
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMR_Close_APIError_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"400 Bad Request"}`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	args := []string{"mr", "close", "1", "--project", "g/p", "--json"}
	args = append(args, mrCloseConfirmArgsForTest(t, "g/p", 1)...)
	rootCmd.SetArgs(args)
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestMR_Reopen_NewClientError(t *testing.T) {
	mrNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "reopen", "1", "--project", "g/p"}))
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMR_Reopen_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"400 Bad Request"}`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "reopen", "1", "--project", "g/p"}))
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestMR_Approve_NewClientError(t *testing.T) {
	mrNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "approve", "1", "--project", "g/p"}))
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMR_Approve_APIError_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "approve", "1", "--project", "g/p", "--json"}))
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestMR_Unapprove_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "unapprove", "1", "--project", "g/p", "--json"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"approved": false`) {
		t.Errorf("expected approved:false in JSON, got:\n%s", out)
	}
}

func TestMR_Unapprove_NewClientError(t *testing.T) {
	mrNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "unapprove", "1", "--project", "g/p"}))
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMR_Unapprove_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "unapprove", "1", "--project", "g/p"}))
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestMR_Diff_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("diff --git a/x b/x\n"))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "diff", "1", "--project", "g/p", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"diff"`) {
		t.Errorf("expected diff JSON, got:\n%s", out)
	}
}

func TestMR_Diff_Quiet(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("diff --git a/x b/x\n"))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "diff", "1", "--project", "g/p", "--quiet"})
		_ = rootCmd.Execute()
	})
	if strings.Contains(out, "diff --git") {
		t.Errorf("expected no diff output in quiet mode, got:\n%s", out)
	}
}

func TestMR_Diff_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "diff", "1", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestMR_Get_InvalidIID_Merge(t *testing.T) {
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "merge", "abc", "--project", "g/p", "--confirm", "abc"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Current_NewClientError(t *testing.T) {
	mrNoAuth(t)
	repo := mrInitRepo(t, "https://gitlab.example.com/group/proj.git", "feat/x")
	defer mrChdir(t, repo)()
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "current"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMR_Create_NewClientError(t *testing.T) {
	mrNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "create", "--project", "g/p", "--title", "t", "--source-branch", "b"}))
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestMR_Create_FindExisting_ListError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"400 Bad Request"}`))
	}))
	defer srv.Close()
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "create", "--find-existing", "--project", "g/p", "--title", "t", "--source-branch", "b"}))
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestMR_Update_InvalidIID(t *testing.T) {
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "update", "abc", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Merge_ConfirmRejected(t *testing.T) {
	withNonInteractiveStdin(t)
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "merge", "1", "--project", "g/p", "--confirm", "wrong"})
	_ = rootCmd.Execute()
	if lastExit != ExitConflict {
		t.Errorf("exit = %d, want %d", lastExit, ExitConflict)
	}
}

func TestMR_Close_InvalidIID(t *testing.T) {
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "close", "abc", "--project", "g/p", "--confirm", "abc"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Close_ConfirmRejected(t *testing.T) {
	withNonInteractiveStdin(t)
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "close", "1", "--project", "g/p", "--confirm", "wrong"})
	_ = rootCmd.Execute()
	if lastExit != ExitConflict {
		t.Errorf("exit = %d, want %d", lastExit, ExitConflict)
	}
}

func TestMR_Reopen_InvalidIID(t *testing.T) {
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "reopen", "abc", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Approve_InvalidIID(t *testing.T) {
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "approve", "abc", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Unapprove_InvalidIID(t *testing.T) {
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "unapprove", "abc", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Diff_InvalidIID(t *testing.T) {
	resetRootPersistentFlags(t)
	resetMRCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "diff", "abc", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMR_Diff_NewClientError(t *testing.T) {
	mrNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"mr", "diff", "1", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}
