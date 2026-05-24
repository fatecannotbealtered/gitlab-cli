package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestUserHelp_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"user", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"me", "search", "get"} {
		if !strings.Contains(out, want) {
			t.Errorf("user --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestUserMe_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"username":"alice","name":"Alice","email":"alice@example.com","state":"active","web_url":"https://gitlab.example.com/alice"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"user", "me", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"alice"`) || !strings.Contains(out, `"id"`) {
		t.Errorf("expected JSON with user data, got:\n%s", out)
	}
}

func TestUserSearch_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":2,"username":"bob","name":"Bob","state":"active"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"user", "search", "--query", "bob", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"bob"`) {
		t.Errorf("expected JSON with user data, got:\n%s", out)
	}
}

func TestUserSearch_MissingQuery(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	// Reset the query flag to empty to simulate missing --query
	_ = userSearchCmd.Flags().Set("query", "")

	rootCmd.SetArgs([]string{"user", "search"})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", lastExit, ExitBadArgs)
	}
}

func TestUserGet_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":3,"username":"carol","name":"Carol","state":"active"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"user", "get", "carol", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"carol"`) {
		t.Errorf("expected JSON with user data, got:\n%s", out)
	}
}

func TestUser_Help_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"user", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"get", "me"} {
		if !strings.Contains(out, want) {
			t.Errorf("user --help missing subcommand %q\noutput:\n%s", want, out)
		}
	}
}

func TestUser_Me_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"username":"alice","name":"Alice","email":"a@b.com","state":"active","web_url":"http://x"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"user", "me", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"username": "alice"`) {
		t.Errorf("expected JSON with username alice, got:\n%s", out)
	}
}

func TestUser_Get_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"username":"alice","name":"Alice","email":"a@b.com","state":"active","web_url":"http://x"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"user", "get", "alice", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"alice"`) {
		t.Errorf("expected JSON with alice, got:\n%s", out)
	}
}

func TestUser_List_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"username":"alice","name":"Alice","state":"active"},{"id":2,"username":"bob","name":"Bob","state":"active"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"user", "search", "--query", "a", "--json"})
		_ = rootCmd.Execute()
	})
	if out == "" || !strings.Contains(out, `"username"`) {
		t.Errorf("expected non-empty user list with username key, got:\n%s", out)
	}
}

func TestUser_Me_Fields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"username":"alice","name":"Alice","email":"a@b.com","state":"active","web_url":"http://x"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"user", "me", "--json", "--fields", "username,email"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"username"`) || !strings.Contains(out, `"email"`) {
		t.Errorf("expected username and email fields, got:\n%s", out)
	}
	if strings.Contains(out, `"name"`) || strings.Contains(out, `"state"`) {
		t.Errorf("unexpected fields in filtered output, got:\n%s", out)
	}
}

func TestUser_Me_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"username":"alice","name":"Alice","email":"a@b.com","state":"active","web_url":"http://x"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"user", "me"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "alice") {
		t.Errorf("expected username in plain text output:\n%s", out)
	}
}

func TestUser_List_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"username":"alice","name":"Alice","state":"active"}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"user", "search", "--query", "alice"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "alice") {
		t.Errorf("expected username in plain text output:\n%s", out)
	}
}

func TestUser_Me_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"user", "me"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestUser_Search_Empty(t *testing.T) {
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
		rootCmd.SetArgs([]string{"user", "search", "--query", "nobody"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No users") {
		t.Errorf("expected 'No users' in output, got: %s", out)
	}
}
