package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVariable_Help_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"variable", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(nil)

	out := buf.String()
	for _, want := range []string{"list", "get", "create", "update", "delete"} {
		if !strings.Contains(out, want) {
			t.Errorf("variable --help missing subcommand %q\noutput:\n%s", want, out)
		}
	}
}

func TestVariable_List_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"variable", "list"})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", lastExit, ExitBadArgs)
	}
}

func TestVariable_Get_MissingKey(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"variable", "get", "--project", "group/proj"})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", lastExit, ExitBadArgs)
	}
}

func TestVariable_List_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"key":"FOO","value":"secret","variable_type":"env_var","protected":false,"masked":false,"environment_scope":"*"}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"variable", "list", "--project", "foo/bar", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"key": "FOO"`) {
		t.Errorf("expected key in output, got: %s", out)
	}
	if strings.Contains(out, `"value"`) {
		t.Errorf("value should be redacted by default, got: %s", out)
	}
}

func TestVariable_List_ShowValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"key":"FOO","value":"secret","variable_type":"env_var","protected":false,"masked":false,"environment_scope":"*"}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() {
		jsonMode = origJM
		_ = variableCmd.PersistentFlags().Set("show-values", "false")
	}()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"variable", "list", "--project", "foo/bar", "--json", "--show-values"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"value"`) {
		t.Errorf("expected value field with --show-values, got: %s", out)
	}
}

func TestVariable_Get_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"key":"FOO","value":"secret","variable_type":"env_var","protected":false,"masked":false,"environment_scope":"*"}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"variable", "get", "--project", "foo/bar", "--key", "FOO", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"key": "FOO"`) {
		t.Errorf("expected key in output, got: %s", out)
	}
	if strings.Contains(out, `"value"`) {
		t.Errorf("value should be redacted by default, got: %s", out)
	}
}

func TestVariable_Create_DryRun_JSON(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"variable", "create",
			"--dry-run", "--json",
			"--project", "group/proj",
			"--key", "MY_SECRET",
			"--value", "supersecret",
		})
		_ = rootCmd.Execute()
	})

	for _, want := range []string{`"dryRun": true`, `"action"`, `"key": "MY_SECRET"`} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run JSON missing %q\noutput:\n%s", want, out)
		}
	}
	if strings.Contains(out, "supersecret") {
		t.Errorf("secret value should not appear in dry-run output:\n%s", out)
	}
}

func TestVariable_Update_DryRun_JSON(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"variable", "update",
			"--dry-run", "--json",
			"--project", "group/proj",
			"--key", "MY_SECRET",
			"--value", "newval",
		})
		_ = rootCmd.Execute()
	})

	for _, want := range []string{`"dryRun": true`, `"key": "MY_SECRET"`} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run JSON missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestVariable_Delete_DryRun_JSON(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"variable", "delete",
			"--dry-run", "--json",
			"--project", "group/proj",
			"--key", "MY_SECRET",
			"--force",
		})
		_ = rootCmd.Execute()
	})

	for _, want := range []string{`"dryRun": true`, `"key": "MY_SECRET"`} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run JSON missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestVariable_Create_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/variables") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"key":"FOO","value":"secret","variable_type":"env_var","protected":false,"masked":false,"environment_scope":"*"}`)
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
		rootCmd.SetArgs([]string{"variable", "create", "--project", "foo/bar", "--key", "FOO", "--value", "secret", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"key": "FOO"`) {
		t.Errorf("expected key in output, got: %s", out)
	}
	if strings.Contains(out, `"value"`) {
		t.Errorf("value should be redacted by default, got: %s", out)
	}
}

func TestVariable_Update_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"key":"FOO","value":"newval","variable_type":"env_var","protected":false,"masked":false,"environment_scope":"*"}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"variable", "update", "--project", "foo/bar", "--key", "FOO", "--value", "newval", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"key": "FOO"`) {
		t.Errorf("expected key in output, got: %s", out)
	}
}

func TestVariable_Delete_JSON(t *testing.T) {
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
		rootCmd.SetArgs([]string{"variable", "delete", "--project", "foo/bar", "--key", "FOO", "--json", "--force"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
}

func TestParseEnvScopeFilter(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"env_scope=production", "production"},
		{"production", "production"},
		{"", ""},
	}
	for _, tc := range cases {
		cmd := variableGetCmd
		_ = cmd.Flags().Set("filter", tc.input)
		got := parseEnvScopeFilter(cmd)
		if got != tc.want {
			t.Errorf("parseEnvScopeFilter(%q) = %q, want %q", tc.input, got, tc.want)
		}
		_ = cmd.Flags().Set("filter", "")
	}
}

func TestVariable_List_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"key":"FOO","value":"secret","variable_type":"env_var","protected":false,"masked":false,"environment_scope":"*"}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"variable", "list", "--project", "foo/bar"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "FOO") {
		t.Errorf("expected key name in plain text output:\n%s", out)
	}
}

func TestVariable_Create_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"key":"FOO","value":"secret","variable_type":"env_var","protected":false,"masked":false,"environment_scope":"*"}`)
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
		rootCmd.SetArgs([]string{"variable", "create", "--project", "foo/bar", "--key", "FOO", "--value", "secret"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "FOO") {
		t.Errorf("expected key name in plain text output:\n%s", out)
	}
}

func TestVariable_Get_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"key":"FOO","value":"secret","variable_type":"env_var","protected":false,"masked":false,"environment_scope":"*"}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"variable", "get", "--project", "foo/bar", "--key", "FOO"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "FOO") {
		t.Errorf("expected key in plain text output:\n%s", out)
	}
}

func TestVariable_Update_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"key":"FOO","value":"newval","variable_type":"env_var","protected":false,"masked":false,"environment_scope":"*"}`)
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
		rootCmd.SetArgs([]string{"variable", "update", "--project", "foo/bar", "--key", "FOO", "--value", "newval"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "FOO") {
		t.Errorf("expected key in plain text output:\n%s", out)
	}
}

func TestVariable_List_Empty(t *testing.T) {
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
		rootCmd.SetArgs([]string{"variable", "list", "--project", "foo/bar"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No variables") {
		t.Errorf("expected 'No variables' in output, got: %s", out)
	}
}

func TestVariable_List_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"variable", "list", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}
