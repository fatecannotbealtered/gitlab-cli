package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
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

func TestFilterUsers(t *testing.T) {
	users := []api.User{
		{Username: "alice", State: "active"},
		{Username: "bob", State: "blocked"},
		{Username: "carol", State: "active"},
	}

	all := filterUsers(users, false, 0)
	if len(all) != 3 {
		t.Fatalf("filterUsers(all) len = %d, want 3", len(all))
	}

	active := filterUsers(users, true, 0)
	if len(active) != 2 {
		t.Fatalf("filterUsers(active) len = %d, want 2", len(active))
	}

	limited := filterUsers(users, false, 2)
	if len(limited) != 2 {
		t.Fatalf("filterUsers(limit=2) len = %d, want 2", len(limited))
	}

	activeLimited := filterUsers(users, true, 1)
	if len(activeLimited) != 1 || activeLimited[0].Username != "alice" {
		t.Fatalf("filterUsers(active,1) = %+v, want [alice]", activeLimited)
	}
}

func TestUser_Me_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"user", "me"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestUser_Search_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"user", "search", "--query", "alice"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestUser_Search_InvalidLimit(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit; _ = userSearchCmd.Flags().Set("limit", "20") }()
	lastExit = 0
	_ = userSearchCmd.Flags().Set("limit", "0")
	rootCmd.SetArgs([]string{"user", "search", "--query", "alice"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestUser_Search_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"user", "search", "--query", "alice"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestUser_Get_MissingAuth(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"user", "get", "alice"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit code = %d, want %d", lastExit, ExitAuth)
	}
}

func TestUser_Get_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"user", "get", "ghost"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for user not found, got %d", lastExit)
	}
}

func TestUser_Get_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"user", "get", "alice"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestUser_Get_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"username":"alice","name":"Alice","email":"a@b.com","state":"active","web_url":"http://x"}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"user", "get", "alice"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{"alice", "Alice", "a@b.com", "active"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestToFlatUser(t *testing.T) {
	flat := toFlatUser(&api.User{ID: 1, Username: "alice", Name: "Alice", Email: "a@b.com", State: "active", WebURL: "http://x"})
	if flat.Username != "alice" || flat.Email != "a@b.com" {
		t.Fatalf("toFlatUser = %+v", flat)
	}
}
