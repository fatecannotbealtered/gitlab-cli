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

func TestProjectHelp_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"project", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"list", "get", "members"} {
		if !strings.Contains(out, want) {
			t.Errorf("project --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestProjectList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"name":"myproject","path_with_namespace":"group/myproject","visibility":"private","web_url":"https://gitlab.example.com/group/myproject","default_branch":"main"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"project", "list", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"id"`) || !strings.Contains(out, `"myproject"`) {
		t.Errorf("expected JSON with project data, got:\n%s", out)
	}
}

func TestProjectGet_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":42,"name":"myproject","path_with_namespace":"group/myproject","visibility":"public","web_url":"https://gitlab.example.com/group/myproject","default_branch":"main"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"project", "get", "group/myproject", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"id"`) || !strings.Contains(out, `"public"`) {
		t.Errorf("expected JSON with project data, got:\n%s", out)
	}
}

func TestProjectMembers_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":10,"username":"alice","name":"Alice","state":"active","access_level":40}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"project", "members", "42", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"alice"`) || !strings.Contains(out, `"accessLevel"`) {
		t.Errorf("expected JSON with member data, got:\n%s", out)
	}
}

func TestProjectList_MissingAuth(t *testing.T) {
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	// Unset env vars so config.MustLoad fails
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")

	rootCmd.SetArgs([]string{"project", "list"})
	_ = rootCmd.Execute()

	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d (ExitAuth)", lastExit, ExitAuth)
	}
}

func TestProject_Help_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"project", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"list", "get", "members"} {
		if !strings.Contains(out, want) {
			t.Errorf("project --help missing subcommand %q\noutput:\n%s", want, out)
		}
	}
}

func TestProject_List_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"name":"myproj","path_with_namespace":"g/myproj","web_url":"http://x","default_branch":"main","visibility":"private","star_count":0,"forks_count":0}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"project", "list", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"name": "myproj"`) {
		t.Errorf("expected JSON with name myproj, got:\n%s", out)
	}
}

func TestProject_Get_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"name":"myproj","path_with_namespace":"g/myproj","web_url":"http://x","default_branch":"main","visibility":"private"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"project", "get", "g/myproj", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"pathWithNamespace"`) {
		t.Errorf("expected JSON with pathWithNamespace key, got:\n%s", out)
	}
}

func TestProject_Members_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"username":"alice","name":"Alice","access_level":40,"state":"active"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"project", "members", "1", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"accessLevel": "40"`) {
		t.Errorf("expected JSON with accessLevel 40, got:\n%s", out)
	}
}

func TestProject_List_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"name":"myproj","path_with_namespace":"g/myproj","web_url":"http://x","default_branch":"main","visibility":"private"}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"project", "list"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "myproj") {
		t.Errorf("expected project name in plain text output:\n%s", out)
	}
}

func TestProject_Get_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"name":"myproj","path_with_namespace":"g/myproj","web_url":"http://x","default_branch":"main","visibility":"private"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"project", "get", "g/myproj"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "myproj") {
		t.Errorf("expected project name in plain text output:\n%s", out)
	}
}

func TestProject_Members_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"username":"alice","name":"Alice","access_level":40,"state":"active"}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"project", "members", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "alice") {
		t.Errorf("expected username in plain text output:\n%s", out)
	}
}

func TestProject_List_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"project", "list"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestProject_List_Empty(t *testing.T) {
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
		rootCmd.SetArgs([]string{"project", "list"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No projects") {
		t.Errorf("expected 'No projects' in output, got: %s", out)
	}
}

func TestProject_Members_Empty(t *testing.T) {
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
		rootCmd.SetArgs([]string{"project", "members", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No members") {
		t.Errorf("expected 'No members' in output, got: %s", out)
	}
}

func projectNoAuth(t *testing.T) {
	t.Helper()
	resetProjectCmdFlags(t)
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")
}

func resetProjectCmdFlags(t *testing.T) {
	t.Helper()
	resetRootPersistentFlags(t)
	for _, kv := range []struct {
		cmd   *cobra.Command
		name  string
		value string
	}{
		{projectListCmd, "limit", "20"},
		{projectMembersCmd, "limit", "20"},
	} {
		if err := kv.cmd.Flags().Set(kv.name, kv.value); err != nil {
			t.Fatalf("reset project flag %s.%s: %v", kv.cmd.Name(), kv.name, err)
		}
	}
}

func TestProject_List_InvalidLimit(t *testing.T) {
	resetProjectCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"project", "list", "--limit", "0"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestProject_Get_NewClientError(t *testing.T) {
	projectNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"project", "get", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestProject_Get_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	resetProjectCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"project", "get", "g/missing"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestProject_Members_NewClientError(t *testing.T) {
	projectNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"project", "members", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestProject_Members_InvalidLimit(t *testing.T) {
	resetProjectCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"project", "members", "1", "--limit", "0"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestProject_Members_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	resetProjectCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"project", "members", "999"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}
