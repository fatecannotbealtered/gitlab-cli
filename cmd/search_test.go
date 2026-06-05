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

func TestSearchHelp_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"search", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"projects", "issues", "mrs", "code", "commits"} {
		if !strings.Contains(out, want) {
			t.Errorf("search --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestSearchProjects_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"name":"found","path_with_namespace":"g/found","web_url":"https://gl.example.com/g/found","visibility":"public"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "projects", "--query", "found", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"found"`) || !strings.Contains(out, `"id"`) {
		t.Errorf("expected JSON with search results, got:\n%s", out)
	}
}

func TestSearchIssues_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":5,"iid":2,"title":"bug fix","state":"opened","web_url":"https://gl.example.com/g/p/-/issues/2","project_id":10}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "issues", "--query", "bug", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"bug fix"`) || !strings.Contains(out, `"iid"`) {
		t.Errorf("expected JSON with issue data, got:\n%s", out)
	}
}

func TestSearchCode_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"search", "code", "--query", "func main"})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", lastExit, ExitBadArgs)
	}
}

func TestSearchCode_MissingQuery(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = searchCodeCmd.Flags().Set("query", "") }()
	lastExit = 0
	_ = searchCodeCmd.Flags().Set("query", "")
	rootCmd.SetArgs([]string{"search", "code", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestSearchCode_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"basename":"main","data":"func main()","path":"main.go","filename":"main.go","ref":"main","startline":1,"project_id":99}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "code", "--query", "func main", "--project", "99", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"main.go"`) || !strings.Contains(out, `"filename"`) {
		t.Errorf("expected JSON with blob data, got:\n%s", out)
	}
}

func TestSearchCommits_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"abc123","short_id":"abc123","title":"fix: bug","author_name":"Alice","created_at":"2024-01-01T00:00:00Z","web_url":"https://gl.example.com/g/p/-/commit/abc123"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "commits", "--query", "fix", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"abc123"`) || !strings.Contains(out, `"fix: bug"`) {
		t.Errorf("expected JSON with commit data, got:\n%s", out)
	}
}

func TestSearchMRs_MissingQuery(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"search", "mrs"})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", lastExit, ExitBadArgs)
	}
}

func TestSearch_Help_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"search", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"projects", "issues", "mrs", "code", "commits"} {
		if !strings.Contains(out, want) {
			t.Errorf("search --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestSearch_Projects_MissingQuery(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	_ = searchProjectsCmd.Flags().Set("query", "")

	rootCmd.SetArgs([]string{"search", "projects"})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", lastExit, ExitBadArgs)
	}
}

func TestSearch_Projects_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"name":"myproj","path_with_namespace":"g/myproj","web_url":"http://x","visibility":"public"}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "projects", "--query", "myproj", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"name": "myproj"`) {
		t.Errorf("expected project in output, got: %s", out)
	}
}

func TestSearch_Issues_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"iid":1,"title":"bug","state":"opened","web_url":"","project_id":1}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "issues", "--query", "bug", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"title": "bug"`) {
		t.Errorf("expected issue in output, got: %s", out)
	}
}

func TestSearch_MRs_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"iid":1,"title":"feat","state":"opened","web_url":"","project_id":1}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "mrs", "--query", "feat", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"title": "feat"`) {
		t.Errorf("expected MR in output, got: %s", out)
	}
}

func TestSearch_Code_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":"abc","filename":"main.go","ref":"main","startline":1,"project_id":1}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "code", "--query", "main", "--project", "foo/bar", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"filename": "main.go"`) {
		t.Errorf("expected code result in output, got: %s", out)
	}
}

func TestSearch_Commits_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":"abc","short_id":"abc123","title":"init","author_name":"dev","created_at":"2024-01-01","web_url":""}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "commits", "--query", "init", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"title": "init"`) {
		t.Errorf("expected commit in output, got: %s", out)
	}
}

func TestSearch_Projects_Fields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"name":"myproj","path_with_namespace":"g/myproj","web_url":"http://x","visibility":"public"}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "projects", "--query", "myproj", "--json", "--fields", "name"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"name"`) {
		t.Errorf("expected name field in output, got: %s", out)
	}
	if strings.Contains(out, `"web_url"`) {
		t.Errorf("web_url should be filtered out, got: %s", out)
	}
}

func TestSearch_Projects_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"name":"myproj","path_with_namespace":"g/myproj","web_url":"http://x","visibility":"public"}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "projects", "--query", "myproj"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "myproj") {
		t.Errorf("expected project name in plain text output:\n%s", out)
	}
}

func TestSearch_Issues_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"iid":1,"title":"bug","state":"opened","web_url":"","project_id":1}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "issues", "--query", "bug"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "bug") {
		t.Errorf("expected issue title in plain text output:\n%s", out)
	}
}

func TestSearch_MRs_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":1,"iid":1,"title":"feat","state":"opened","web_url":"","project_id":1}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "mrs", "--query", "feat"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "feat") {
		t.Errorf("expected MR title in plain text output:\n%s", out)
	}
}

func TestSearch_Commits_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":"abc","short_id":"abc123","title":"init","author_name":"dev","created_at":"2024-01-01","web_url":""}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "commits", "--query", "init"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "init") {
		t.Errorf("expected commit title in plain text output:\n%s", out)
	}
}

func TestSearch_Code_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":"abc","filename":"main.go","ref":"main","startline":1,"project_id":1}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "code", "--query", "main", "--project", "foo/bar"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "main.go") {
		t.Errorf("expected filename in plain text output:\n%s", out)
	}
}

func TestSearch_Projects_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"search", "projects", "--query", "test"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestSearch_Projects_InvalidLimit(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = searchProjectsCmd.Flags().Set("limit", "20") }()
	lastExit = 0
	_ = searchProjectsCmd.Flags().Set("limit", "0")
	rootCmd.SetArgs([]string{"search", "projects", "--query", "test"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestSearch_Projects_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"search", "projects", "--query", "test"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestSearch_Projects_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "projects", "--query", "none"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No projects") {
		t.Errorf("expected No projects in output, got: %s", out)
	}
}

func TestSearch_Issues_MissingQuery(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = searchIssuesCmd.Flags().Set("query", "") }()
	lastExit = 0
	_ = searchIssuesCmd.Flags().Set("query", "")
	rootCmd.SetArgs([]string{"search", "issues"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestSearch_Issues_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"search", "issues", "--query", "bug"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestSearch_Issues_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"search", "issues", "--query", "bug"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestSearch_Issues_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "issues", "--query", "none"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No issues") {
		t.Errorf("expected No issues in output, got: %s", out)
	}
}

func TestSearch_MRs_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"search", "mrs", "--query", "feat"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestSearch_MRs_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"search", "mrs", "--query", "feat"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestSearch_MRs_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "mrs", "--query", "none"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No merge requests") {
		t.Errorf("expected No merge requests in output, got: %s", out)
	}
}

func TestSearch_Code_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"search", "code", "--query", "main", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestSearch_Code_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"search", "code", "--query", "main", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestSearch_Code_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "code", "--query", "none", "--project", "foo/bar"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No code results") {
		t.Errorf("expected No code results in output, got: %s", out)
	}
}

func TestSearch_Commits_MissingQuery(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = searchCommitsCmd.Flags().Set("query", "") }()
	lastExit = 0
	_ = searchCommitsCmd.Flags().Set("query", "")
	rootCmd.SetArgs([]string{"search", "commits"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestSearch_Commits_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"search", "commits", "--query", "fix"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestSearch_Commits_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"search", "commits", "--query", "fix"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestSearch_Commits_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"search", "commits", "--query", "none"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No commits") {
		t.Errorf("expected No commits in output, got: %s", out)
	}
}

func TestSearch_Issues_InvalidLimit(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = searchIssuesCmd.Flags().Set("limit", "20") }()
	lastExit = 0
	_ = searchIssuesCmd.Flags().Set("limit", "0")
	rootCmd.SetArgs([]string{"search", "issues", "--query", "bug"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestSearch_MRs_MissingQuery(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = searchMRsCmd.Flags().Set("query", "") }()
	lastExit = 0
	_ = searchMRsCmd.Flags().Set("query", "")
	rootCmd.SetArgs([]string{"search", "mrs"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestSearch_MRs_InvalidLimit(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = searchMRsCmd.Flags().Set("limit", "20") }()
	lastExit = 0
	_ = searchMRsCmd.Flags().Set("limit", "0")
	rootCmd.SetArgs([]string{"search", "mrs", "--query", "feat"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestSearch_Code_InvalidLimit(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = searchCodeCmd.Flags().Set("limit", "20") }()
	lastExit = 0
	_ = searchCodeCmd.Flags().Set("limit", "0")
	rootCmd.SetArgs([]string{"search", "code", "--query", "main", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestSearch_Commits_InvalidLimit(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = searchCommitsCmd.Flags().Set("limit", "20") }()
	lastExit = 0
	_ = searchCommitsCmd.Flags().Set("limit", "0")
	rootCmd.SetArgs([]string{"search", "commits", "--query", "fix"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}
