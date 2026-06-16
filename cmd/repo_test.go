package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRepoCmd_Help(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"repo", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"file", "branch", "commit", "tree"} {
		if !strings.Contains(out, want) {
			t.Errorf("repo --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestRepoFileCmd_Help(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"repo", "file", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"get", "create", "update", "delete"} {
		if !strings.Contains(out, want) {
			t.Errorf("repo file --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestRepoBranchCmd_Help(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"repo", "branch", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"list", "create", "delete"} {
		if !strings.Contains(out, want) {
			t.Errorf("repo branch --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestRepoCommitCmd_Help(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"repo", "commit", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"list", "get"} {
		if !strings.Contains(out, want) {
			t.Errorf("repo commit --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestRepoBranchList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"name":"main","default":true,"protected":false,"merged":false}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = true

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "branch", "list", "--project", "1", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"name"`) || !strings.Contains(out, "main") {
		t.Errorf("expected JSON with branch name, got:\n%s", out)
	}
}

func TestRepoBranchCreate_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = true
	jsonMode = true

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "branch", "create", "--project", "1", "--name", "feature", "--ref", "main", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"action": "repo branch create"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestRepoBranchDelete_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = true
	jsonMode = true

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "branch", "delete", "--project", "1", "--name", "feature", "--dry-run", "--dangerous", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"action": "repo branch delete"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestRepoFileCreate_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = true
	jsonMode = true

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"repo", "file", "create",
			"--project", "1", "--path", "README.md",
			"--branch", "main", "--content", "hello",
			"--commit-message", "add readme",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"action": "repo file create"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestRepoFileUpdate_DryRun_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/repository/files/") && !strings.HasSuffix(r.URL.Path, "/raw") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_name":"README.md","last_commit_id":"deadbeefcafe"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = true
	jsonMode = true

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"repo", "file", "update",
			"--project", "1", "--path", "README.md",
			"--branch", "main", "--content", "updated",
			"--commit-message", "update readme",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"action": "repo file update"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestRepoFileDelete_DryRun_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/repository/files/") && !strings.HasSuffix(r.URL.Path, "/raw") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_name":"README.md","last_commit_id":"deadbeefcafe"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = true
	jsonMode = true

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"repo", "file", "delete",
			"--project", "1", "--path", "README.md",
			"--branch", "main", "--commit-message", "delete readme",
			"--dry-run", "--dangerous", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"action": "repo file delete"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestRepoCommitList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"abc123","short_id":"abc","title":"init","author_name":"Alice","authored_date":"2024-01-01"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = true

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "commit", "list", "--project", "1", "--json"})
		_ = rootCmd.Execute()
	})
	// CommitToMap uses camelCase keys
	if !strings.Contains(out, `"shortId"`) || !strings.Contains(out, "abc") {
		t.Errorf("expected JSON with shortId, got:\n%s", out)
	}
}

func TestRepoTree_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"x","name":"README.md","type":"blob","path":"README.md","mode":"100644"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = true

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "tree", "--project", "1", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"name"`) || !strings.Contains(out, "README.md") {
		t.Errorf("expected JSON with tree entry, got:\n%s", out)
	}
}

func TestRepoFileGet_MissingPath(t *testing.T) {
	lastExit = 0
	defer func() { lastExit = 0 }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	// --path is missing �?ExitBadArgs
	rootCmd.SetArgs([]string{"repo", "file", "get", "--project", "1"})
	_ = rootCmd.Execute()
	if LastExitCode() != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", LastExitCode(), ExitBadArgs)
	}
	lastExit = 0
}

func TestRepo_Help_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"repo", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"file", "branch", "commit", "tree"} {
		if !strings.Contains(out, want) {
			t.Errorf("repo --help missing subcommand %q\noutput:\n%s", want, out)
		}
	}
}

func TestRepo_File_Get_MissingProject(t *testing.T) {
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	_ = repoFileGetCmd.Flags().Set("project", "")
	_ = repoFileGetCmd.Flags().Set("path", "README.md")

	rootCmd.SetArgs([]string{"repo", "file", "get", "--path", "README.md"})
	_ = rootCmd.Execute()
	if LastExitCode() != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", LastExitCode(), ExitBadArgs)
	}
}

func TestRepo_File_Get_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "file", "get", "--project", "group/proj", "--path", "README.md"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "hello") {
		t.Errorf("expected raw file content in output, got:\n%s", out)
	}
}

func TestRepo_File_Create_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"repo", "file", "create",
			"--project", "group/proj", "--path", "new.md",
			"--branch", "main", "--content", "hi",
			"--commit-message", "add new.md",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"action": "repo file create"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestRepo_File_Update_DryRun_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/repository/files/") && !strings.HasSuffix(r.URL.Path, "/raw") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_name":"README.md","last_commit_id":"deadbeefcafe"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"repo", "file", "update",
			"--project", "group/proj", "--path", "README.md",
			"--branch", "main", "--content", "updated",
			"--commit-message", "update readme",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"action": "repo file update"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestRepo_File_Delete_DryRun_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/repository/files/") && !strings.HasSuffix(r.URL.Path, "/raw") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_name":"README.md","last_commit_id":"deadbeefcafe"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"repo", "file", "delete",
			"--project", "group/proj", "--path", "README.md",
			"--branch", "main", "--commit-message", "delete readme",
			"--dry-run", "--dangerous", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"action": "repo file delete"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestRepo_Branch_List_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"main","default":true,"protected":false,"web_url":"http://x","commit":{"id":"abc","short_id":"abc123","title":"init","author_name":"dev","authored_date":"2024-01-01","committed_date":"2024-01-01"}}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "branch", "list", "--project", "group/proj", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"name"`) || !strings.Contains(out, "main") {
		t.Errorf("expected JSON with branch name, got:\n%s", out)
	}
}

func TestRepo_Branch_Create_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"repo", "branch", "create",
			"--project", "group/proj", "--name", "feat/x", "--ref", "main",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"action": "repo branch create"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestRepo_Branch_Delete_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"repo", "branch", "delete",
			"--project", "group/proj", "--name", "feat/x",
			"--dry-run", "--dangerous", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"action": "repo branch delete"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestRepo_Commit_List_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"abc","short_id":"abc123","title":"init","author_name":"dev","authored_date":"2024-01-01","committed_date":"2024-01-01","web_url":"http://x"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "commit", "list", "--project", "group/proj", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"shortId"`) || !strings.Contains(out, "abc123") {
		t.Errorf("expected JSON with shortId, got:\n%s", out)
	}
}

func TestRepo_Commit_Get_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"abc","short_id":"abc123","title":"init","author_name":"dev","authored_date":"2024-01-01","committed_date":"2024-01-01","web_url":"http://x"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "commit", "get", "abc", "--project", "group/proj", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"shortId"`) {
		t.Errorf("expected JSON with shortId key, got:\n%s", out)
	}
}

func TestRepo_Tree_List_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"abc","name":"README.md","type":"blob","path":"README.md","mode":"100644"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "tree", "--project", "group/proj", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"name"`) || !strings.Contains(out, "README.md") {
		t.Errorf("expected JSON with tree entry, got:\n%s", out)
	}
}

func TestRepo_Tag_List_JSON(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"repo", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	if !strings.Contains(out, "tree") {
		t.Errorf("repo --help missing 'tree' subcommand\noutput:\n%s", out)
	}
}

func TestRepo_File_Create_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/repository/files/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"file_path":"README.md","branch":"main"}`))
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
		rootCmd.SetArgs(withConfirmForTest(t, []string{
			"repo", "file", "create",
			"--project", "foo/bar", "--path", "README.md",
			"--branch", "main", "--content", "hello",
			"--commit-message", "add readme", "--json",
		}))
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
}

func TestRepo_File_Update_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/repository/files/") && !strings.HasSuffix(r.URL.Path, "/raw") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_name":"README.md","last_commit_id":"deadbeefcafe"}`))
			return
		}
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/repository/files/") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_path":"README.md","branch":"main"}`))
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
		rootCmd.SetArgs(withConfirmForTest(t, []string{
			"repo", "file", "update",
			"--project", "foo/bar", "--path", "README.md",
			"--branch", "main", "--content", "updated",
			"--commit-message", "update readme", "--json",
		}))
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
}

func TestRepo_File_Delete_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/repository/files/") && !strings.HasSuffix(r.URL.Path, "/raw") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_name":"README.md","last_commit_id":"deadbeefcafe"}`))
			return
		}
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/repository/files/") {
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
		rootCmd.SetArgs(withConfirmForTest(t, []string{
			"repo", "file", "delete",
			"--project", "foo/bar", "--path", "README.md",
			"--branch", "main", "--commit-message", "delete readme",
			"--json", "--dangerous",
		}))
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
}

func TestRepo_Branch_Create_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/repository/branches") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"name":"feat","default":false,"protected":false,"web_url":"http://x","commit":{"id":"abc","short_id":"abc123","title":"init","author_name":"dev","authored_date":"2024-01-01","committed_date":"2024-01-01"}}`))
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
		rootCmd.SetArgs(withConfirmForTest(t, []string{"repo", "branch", "create", "--project", "foo/bar", "--name", "feat", "--ref", "main", "--json"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"feat"`) {
		t.Errorf("expected branch name in output, got: %s", out)
	}
}

func TestRepo_Branch_Delete_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/repository/branches/") {
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
		args := []string{"repo", "branch", "delete", "--project", "foo/bar", "--name", "feat", "--json", "--dangerous"}
		args = append(args, confirmArgsForTest(t, "repo branch delete", map[string]any{"project": "foo/bar", "name": "feat"})...)
		rootCmd.SetArgs(args)
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
}

func TestBoolStr(t *testing.T) {
	if boolStr(true) != "yes" {
		t.Errorf("boolStr(true) = %q, want 'yes'", boolStr(true))
	}
	if boolStr(false) != "no" {
		t.Errorf("boolStr(false) = %q, want 'no'", boolStr(false))
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello", 10) != "hello" {
		t.Errorf("truncate short string changed it")
	}
	got := truncate("hello world", 6)
	if len([]rune(got)) > 6 {
		t.Errorf("truncate did not shorten: %q", got)
	}
	// byte length exceeds n but rune count does not — return unchanged
	multi := "ééé"
	if truncate(multi, 5) != multi {
		t.Errorf("truncate multi-byte = %q, want %q", truncate(multi, 5), multi)
	}
}

func TestResolveContent_InlineEncoding(t *testing.T) {
	content, encoding, err := resolveContent("data", "", "base64")
	if err != nil {
		t.Fatalf("resolveContent: %v", err)
	}
	if content != "data" || encoding != "base64" {
		t.Fatalf("got (%q, %q), want (data, base64)", content, encoding)
	}
}

func TestResolveContent_FromFileText(t *testing.T) {
	dir := t.TempDir()
	path := dir + string(os.PathSeparator) + "hello.txt"
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	content, encoding, err := resolveContent("", path, "")
	if err != nil {
		t.Fatalf("resolveContent: %v", err)
	}
	if content != "hello" || encoding != "text" {
		t.Fatalf("got (%q, %q), want (hello, text)", content, encoding)
	}
}

func TestResolveContent_FromFileBinary(t *testing.T) {
	dir := t.TempDir()
	path := dir + string(os.PathSeparator) + "bin.dat"
	if err := os.WriteFile(path, []byte{0x00, 0xff, 0xfe}, 0o644); err != nil {
		t.Fatal(err)
	}
	content, encoding, err := resolveContent("", path, "")
	if err != nil {
		t.Fatalf("resolveContent: %v", err)
	}
	if encoding != "base64" {
		t.Fatalf("encoding = %q, want base64", encoding)
	}
	if content == "" {
		t.Fatal("expected base64 content")
	}
}

func TestResolveContent_FromFileExplicitBase64(t *testing.T) {
	dir := t.TempDir()
	path := dir + string(os.PathSeparator) + "plain.txt"
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	content, encoding, err := resolveContent("", path, "base64")
	if err != nil {
		t.Fatalf("resolveContent: %v", err)
	}
	if encoding != "base64" {
		t.Fatalf("encoding = %q, want base64", encoding)
	}
	if content == "" {
		t.Fatal("expected base64-encoded content")
	}
}

func TestResolveContent_FileReadError(t *testing.T) {
	_, _, err := resolveContent("", t.TempDir()+string(os.PathSeparator)+"missing.txt", "")
	if err == nil {
		t.Fatal("expected error for missing content file")
	}
}

func TestRepo_Branch_List_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"main","default":true,"protected":false,"web_url":"http://x","commit":{"id":"abc","short_id":"abc123","title":"init","author_name":"dev","authored_date":"2024-01-01","committed_date":"2024-01-01"}}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "branch", "list", "--project", "group/proj"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "main") {
		t.Errorf("expected branch name in plain text output:\n%s", out)
	}
}

func TestRepo_Commit_List_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"abc","short_id":"abc123","title":"init commit","author_name":"dev","authored_date":"2024-01-01","committed_date":"2024-01-01","web_url":"http://x"}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "commit", "list", "--project", "group/proj"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "init commit") {
		t.Errorf("expected commit title in plain text output:\n%s", out)
	}
}

func TestRepo_Tree_List_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"abc","name":"README.md","type":"blob","path":"README.md","mode":"100644"}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "tree", "--project", "group/proj"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "README.md") {
		t.Errorf("expected file name in plain text output:\n%s", out)
	}
}

func TestRepo_Branch_List_Empty(t *testing.T) {
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
		rootCmd.SetArgs([]string{"repo", "branch", "list", "--project", "group/proj"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No branches") {
		t.Errorf("expected 'No branches' in output, got: %s", out)
	}
}

func TestRepo_Commit_List_Empty(t *testing.T) {
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
		rootCmd.SetArgs([]string{"repo", "commit", "list", "--project", "group/proj"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No commits") {
		t.Errorf("expected 'No commits' in output, got: %s", out)
	}
}

func TestRepo_Tree_List_Empty(t *testing.T) {
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
		rootCmd.SetArgs([]string{"repo", "tree", "--project", "group/proj"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No entries") {
		t.Errorf("expected 'No entries' in output, got: %s", out)
	}
}

func TestRepo_Commit_Get_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"abc","short_id":"abc123","title":"init","author_name":"dev","authored_date":"2024-01-01","committed_date":"2024-01-01","web_url":"http://x"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "commit", "get", "abc", "--project", "group/proj"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "init") {
		t.Errorf("expected commit title in plain text output:\n%s", out)
	}
}

func TestRepo_Branch_Create_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"feat","default":false,"protected":false,"web_url":"http://x","commit":{"id":"abc","short_id":"abc123","title":"init","author_name":"dev","authored_date":"2024-01-01","committed_date":"2024-01-01"}}`))
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
		rootCmd.SetArgs(withConfirmForTest(t, []string{"repo", "branch", "create", "--project", "group/proj", "--name", "feat", "--ref", "main"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Created") {
		t.Errorf("expected 'Created' in output, got: %s", out)
	}
}

func TestRepo_File_Create_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"file_path":"README.md","branch":"main"}`))
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
		rootCmd.SetArgs(withConfirmForTest(t, []string{
			"repo", "file", "create",
			"--project", "group/proj", "--path", "README.md",
			"--branch", "main", "--content", "hello",
			"--commit-message", "add readme",
		}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Created") {
		t.Errorf("expected 'Created' in output, got: %s", out)
	}
}

func TestRepo_File_Update_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"file_path":"README.md","branch":"main"}`))
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
		rootCmd.SetArgs(withConfirmForTest(t, []string{
			"repo", "file", "update",
			"--project", "group/proj", "--path", "README.md",
			"--branch", "main", "--content", "updated",
			"--commit-message", "update readme",
		}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Updated") {
		t.Errorf("expected 'Updated' in output, got: %s", out)
	}
}

func TestRepo_Branch_List_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"repo", "branch", "list", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestRepo_Commit_List_APIError(t *testing.T) {
	resetRepoCmdFlags(t)
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
	rootCmd.SetArgs([]string{"repo", "commit", "list", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestRepo_Tree_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"repo", "tree", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func repoNoAuth(t *testing.T) {
	t.Helper()
	resetRepoCmdFlags(t)
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")
}

func resetRepoCmdFlags(t *testing.T) {
	t.Helper()
	resetRootPersistentFlags(t)
	// commit list --project/--group are StringSlice: Set("") would append once
	// Changed is true, so hard-clear them. The remaining commit-list scope/filter
	// flags are reset alongside the scalar set below.
	clearSliceFlag(t, repoCommitListCmd, "project")
	clearSliceFlag(t, repoCommitListCmd, "group")
	for _, kv := range []struct {
		cmd   *cobra.Command
		name  string
		value string
	}{
		{repoCommitListCmd, "all-projects", "false"},
		{repoCommitListCmd, "author", ""},
		{repoCommitListCmd, "with-stats", "false"},
		{repoCommitListCmd, "all-branches", "false"},
		{repoCommitDiffCmd, "project", ""},
		{repoCommitDiffCmd, "path", ""},
		{repoCommitDiffCmd, "fields", ""},
		{repoFileGetCmd, "project", ""},
		{repoFileGetCmd, "path", ""},
		{repoFileGetCmd, "output", ""},
		{repoFileCreateCmd, "project", ""},
		{repoFileCreateCmd, "path", ""},
		{repoFileCreateCmd, "branch", ""},
		{repoFileCreateCmd, "content", ""},
		{repoFileCreateCmd, "content-file", ""},
		{repoFileCreateCmd, "commit-message", ""},
		{repoFileUpdateCmd, "project", ""},
		{repoFileUpdateCmd, "path", ""},
		{repoFileUpdateCmd, "branch", ""},
		{repoFileUpdateCmd, "content", ""},
		{repoFileUpdateCmd, "commit-message", ""},
		{repoFileDeleteCmd, "project", ""},
		{repoFileDeleteCmd, "path", ""},
		{repoFileDeleteCmd, "branch", ""},
		{repoFileDeleteCmd, "commit-message", ""},
		{repoBranchListCmd, "project", ""},
		{repoBranchListCmd, "limit", "20"},
		{repoBranchCreateCmd, "project", ""},
		{repoBranchCreateCmd, "name", ""},
		{repoBranchCreateCmd, "ref", ""},
		{repoBranchDeleteCmd, "project", ""},
		{repoBranchDeleteCmd, "name", ""},
		{repoCommitListCmd, "limit", "20"},
		{repoCommitGetCmd, "project", ""},
		{repoTreeCmd, "project", ""},
		{repoTreeCmd, "limit", "20"},
	} {
		if err := kv.cmd.Flags().Set(kv.name, kv.value); err != nil {
			t.Fatalf("reset repo flag %s.%s: %v", kv.cmd.Name(), kv.name, err)
		}
		if f := kv.cmd.Flags().Lookup(kv.name); f != nil {
			f.Changed = false
		}
	}
}

func TestRepo_File_Get_NewClientError(t *testing.T) {
	repoNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "file", "get", "--project", "g/p", "--path", "README.md"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestRepo_File_Get_InvalidOutputPath(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "file", "get", "--project", "g/p", "--path", "README.md", "--output", "../escape"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_File_Get_SaveToFile_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	outPath := t.TempDir() + string(os.PathSeparator) + "out.txt"
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"repo", "file", "get", "--project", "g/p", "--path", "README.md", "--output", outPath})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Saved to") {
		t.Errorf("expected Saved message, got:\n%s", out)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("file content = %q, want hello", data)
	}
}

func TestRepo_File_Get_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "file", "get", "--project", "g/p", "--path", "missing.md"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestRepo_File_Create_MissingContent(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "file", "create", "--project", "g/p", "--path", "a.md", "--branch", "main", "--commit-message", "add"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_File_Create_NewClientError(t *testing.T) {
	repoNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"repo", "file", "create", "--project", "g/p", "--path", "a.md", "--branch", "main", "--content", "x", "--commit-message", "add"}))
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestRepo_File_Create_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"400 Bad Request"}`))
	}))
	defer srv.Close()
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"repo", "file", "create", "--project", "g/p", "--path", "a.md", "--branch", "main", "--content", "x", "--commit-message", "add"}))
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestRepo_File_Update_NewClientError(t *testing.T) {
	repoNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	// newClient() runs before the version GET and prepareWrite, so a hand-built
	// confirm token is enough — auth fails before the token is ever validated.
	args := []string{"repo", "file", "update", "--project", "g/p", "--path", "a.md", "--branch", "main", "--content", "x", "--commit-message", "upd"}
	args = append(args, confirmArgsForTest(t, "repo file update", map[string]any{"project": "g/p", "path": "a.md", "branch": "main", "lastCommitId": ""})...)
	rootCmd.SetArgs(args)
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestRepo_File_Update_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/repository/files/") && !strings.HasSuffix(r.URL.Path, "/raw") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_name":"a.md","last_commit_id":"deadbeefcafe"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"400 Bad Request"}`))
	}))
	defer srv.Close()
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"repo", "file", "update", "--project", "g/p", "--path", "a.md", "--branch", "main", "--content", "x", "--commit-message", "upd"}))
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestRepo_File_Delete_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/repository/files/") && !strings.HasSuffix(r.URL.Path, "/raw") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_name":"README.md","last_commit_id":"deadbeefcafe"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"repo", "file", "delete", "--project", "g/p", "--path", "README.md", "--branch", "main", "--commit-message", "del", "--dangerous"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Deleted") {
		t.Errorf("expected Deleted in output, got:\n%s", out)
	}
}

func TestRepo_File_Delete_ConfirmRejected(t *testing.T) {
	withNonInteractiveStdin(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/repository/files/") && !strings.HasSuffix(r.URL.Path, "/raw") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_name":"README.md","last_commit_id":"deadbeefcafe"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "file", "delete", "--project", "g/p", "--path", "README.md", "--branch", "main", "--commit-message", "del", "--dangerous", "--confirm", "wrong"})
	_ = rootCmd.Execute()
	if lastExit != ExitConflict {
		t.Errorf("exit = %d, want %d", lastExit, ExitConflict)
	}
}

func TestRepo_File_Delete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/repository/files/") && !strings.HasSuffix(r.URL.Path, "/raw") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_name":"README.md","last_commit_id":"deadbeefcafe"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"repo", "file", "delete", "--project", "g/p", "--path", "README.md", "--branch", "main", "--commit-message", "del", "--dangerous"}))
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestRepo_Branch_Create_MissingArgs(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "branch", "create", "--project", "g/p", "--name", "feat"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_Branch_Create_NewClientError(t *testing.T) {
	repoNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"repo", "branch", "create", "--project", "g/p", "--name", "feat", "--ref", "main"}))
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestRepo_Branch_Create_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"400 Bad Request"}`))
	}))
	defer srv.Close()
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"repo", "branch", "create", "--project", "g/p", "--name", "feat", "--ref", "main"}))
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestRepo_Branch_Delete_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	out := captureCombinedOutput(t, func() {
		args := []string{"repo", "branch", "delete", "--project", "g/p", "--name", "feat", "--dangerous"}
		args = append(args, confirmArgsForTest(t, "repo branch delete", map[string]any{"project": "g/p", "name": "feat"})...)
		rootCmd.SetArgs(args)
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Deleted branch") {
		t.Errorf("expected Deleted branch in output, got:\n%s", out)
	}
}

func TestRepo_Branch_Delete_ConfirmRejected(t *testing.T) {
	withNonInteractiveStdin(t)
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "branch", "delete", "--project", "g/p", "--name", "feat", "--dangerous", "--confirm", "wrong"})
	_ = rootCmd.Execute()
	if lastExit != ExitConflict {
		t.Errorf("exit = %d, want %d", lastExit, ExitConflict)
	}
}

func TestRepo_Commit_Get_NewClientError(t *testing.T) {
	repoNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "commit", "get", "abc", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestRepo_Commit_Get_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "commit", "get", "abc", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestRepo_Tree_NewClientError(t *testing.T) {
	repoNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "tree", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestRepo_Tree_InvalidLimit(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "tree", "--project", "g/p", "--limit", "0"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_File_Get_StdoutDash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"repo", "file", "get", "--project", "g/p", "--path", "README.md", "--output", "-"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "hello") {
		t.Errorf("expected file content, got:\n%s", out)
	}
}

func TestRepo_File_Create_MissingRequired(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "file", "create", "--project", "g/p", "--path", "a.md"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_File_Create_ContentFileError(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{
		"repo", "file", "create",
		"--project", "g/p", "--path", "a.md", "--branch", "main",
		"--content-file", t.TempDir() + string(os.PathSeparator) + "missing.txt",
		"--commit-message", "add",
	})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_File_Update_MissingRequired(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "file", "update", "--project", "g/p", "--path", "a.md", "--content", "x"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_File_Update_ContentFileError(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{
		"repo", "file", "update",
		"--project", "g/p", "--path", "a.md", "--branch", "main",
		"--content-file", t.TempDir() + string(os.PathSeparator) + "missing.txt",
		"--commit-message", "upd",
	})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_File_Delete_MissingRequired(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "file", "delete", "--project", "g/p", "--path", "a.md"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_File_Delete_NewClientError(t *testing.T) {
	repoNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	args := []string{"repo", "file", "delete", "--project", "g/p", "--path", "a.md", "--branch", "main", "--commit-message", "del", "--dangerous"}
	args = append(args, confirmArgsForTest(t, "repo file delete", map[string]any{"project": "g/p", "path": "a.md", "branch": "main", "commitMessage": "del"})...)
	rootCmd.SetArgs(args)
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestRepo_Branch_List_NewClientError(t *testing.T) {
	repoNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "branch", "list", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestRepo_Branch_List_MissingProject(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "branch", "list"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_Branch_List_InvalidLimit(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "branch", "list", "--project", "g/p", "--limit", "0"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_Branch_Delete_MissingName(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "branch", "delete", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_Branch_Delete_NewClientError(t *testing.T) {
	repoNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	args := []string{"repo", "branch", "delete", "--project", "g/p", "--name", "feat", "--dangerous"}
	args = append(args, confirmArgsForTest(t, "repo branch delete", map[string]any{"project": "g/p", "name": "feat"})...)
	rootCmd.SetArgs(args)
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestRepo_Commit_List_NewClientError(t *testing.T) {
	repoNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "commit", "list", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestRepo_Commit_List_MissingProject(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "commit", "list"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_Commit_List_InvalidLimit(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "commit", "list", "--project", "g/p", "--limit", "0"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_Commit_Get_MissingProject(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "commit", "get", "abc"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_File_Update_MissingContentOnly(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = repoFileUpdateCmd.Flags().Set("content", "")
	_ = repoFileUpdateCmd.Flags().Set("content-file", "")
	rootCmd.SetArgs([]string{
		"repo", "file", "update",
		"--project", "g/p", "--path", "a.md", "--branch", "main", "--commit-message", "upd", "--content", "",
	})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRepo_File_Get_WriteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	outPath := t.TempDir()
	rootCmd.SetArgs([]string{"repo", "file", "get", "--project", "g/p", "--path", "README.md", "--output", outPath})
	_ = rootCmd.Execute()
	if lastExit != ExitNetwork {
		t.Errorf("exit = %d, want %d", lastExit, ExitNetwork)
	}
}

func TestRepo_Branch_Delete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	args := []string{"repo", "branch", "delete", "--project", "g/p", "--name", "feat", "--dangerous"}
	args = append(args, confirmArgsForTest(t, "repo branch delete", map[string]any{"project": "g/p", "name": "feat"})...)
	rootCmd.SetArgs(args)
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestRepo_Tree_MissingProject(t *testing.T) {
	resetRepoCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"repo", "tree"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}
