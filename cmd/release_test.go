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

func TestReleaseCmd_Help(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"release", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"list", "get", "create", "update", "delete"} {
		if !strings.Contains(out, want) {
			t.Errorf("release --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestReleaseList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0","name":"Release 1.0","released_at":"2024-01-01T00:00:00Z"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = true

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"release", "list", "--project", "1", "--json"})
		_ = rootCmd.Execute()
	})
	// ReleaseToMap uses camelCase keys
	if !strings.Contains(out, `"tagName"`) || !strings.Contains(out, "v1.0") {
		t.Errorf("expected JSON with tagName, got:\n%s", out)
	}
}

func TestReleaseGet_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v1.0","name":"Release 1.0","description":"First"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = true

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"release", "get", "--project", "1", "--tag", "v1.0", "--json"})
		_ = rootCmd.Execute()
	})
	// ToFlatRelease uses FlatRelease struct JSON tags (camelCase)
	if !strings.Contains(out, "v1.0") {
		t.Errorf("expected JSON with v1.0, got:\n%s", out)
	}
}

func TestReleaseCreate_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = true
	jsonMode = true

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"release", "create",
			"--project", "1", "--tag", "v2.0", "--name", "Release 2.0",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"action": "release create"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestReleaseUpdate_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = true
	jsonMode = true

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"release", "update",
			"--project", "1", "--tag", "v1.0", "--name", "Updated",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"action": "release update"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestReleaseDelete_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = true
	jsonMode = true

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"release", "delete",
			"--project", "1", "--tag", "v1.0",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"action": "release delete"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestReleaseCreate_MissingRequired(t *testing.T) {
	lastExit = 0
	defer func() { lastExit = 0 }()

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	// Pass --name "" explicitly to override any stale cobra flag value from previous tests.
	rootCmd.SetArgs([]string{"release", "create", "--project", "1", "--tag", "v1.0", "--name", ""})
	_ = rootCmd.Execute()
	if LastExitCode() != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", LastExitCode(), ExitBadArgs)
	}
	lastExit = 0
}

func TestParseMilestones(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"v1.0", []string{"v1.0"}},
		{"v1.0,v2.0", []string{"v1.0", "v2.0"}},
		{"v1.0, v2.0 , v3.0", []string{"v1.0", "v2.0", "v3.0"}},
		{" , ", nil},
		{",,", nil},
	}
	for _, tc := range cases {
		got := parseMilestones(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("parseMilestones(%q) = %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("parseMilestones(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

func TestRelease_Help_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"release", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"list", "get", "create", "update", "delete"} {
		if !strings.Contains(out, want) {
			t.Errorf("release --help missing subcommand %q\noutput:\n%s", want, out)
		}
	}
}

func TestRelease_List_MissingProject(t *testing.T) {
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	_ = releaseListCmd.Flags().Set("project", "")

	rootCmd.SetArgs([]string{"release", "list"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", lastExit, ExitBadArgs)
	}
}

func TestRelease_List_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0","name":"Release 1.0","description":"desc","created_at":"2024-01-01","released_at":"2024-01-01","commit":{"id":"abc"},"assets":{"count":0}}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"release", "list", "--project", "group/proj", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"tagName": "v1.0"`) {
		t.Errorf("expected JSON with tagName v1.0, got:\n%s", out)
	}
}

func TestRelease_Get_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.0","name":"Release 1.0","description":"desc","created_at":"2024-01-01","released_at":"2024-01-01"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"release", "get", "--project", "group/proj", "--tag", "v1.0", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "v1.0") {
		t.Errorf("expected JSON with v1.0, got:\n%s", out)
	}
}

func TestRelease_Create_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"release", "create",
			"--project", "group/proj", "--tag", "v2.0", "--name", "Release 2.0",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"action": "release create"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestRelease_Update_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"release", "update",
			"--project", "group/proj", "--tag", "v1.0", "--name", "Updated",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"action": "release update"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestRelease_Delete_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"release", "delete",
			"--project", "group/proj", "--tag", "v1.0",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"dryRun": true`, `"action": "release delete"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestRelease_Create_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/releases") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"tag_name":"v1.0","name":"Release 1.0","description":"desc","created_at":"2024-01-01","released_at":"2024-01-01","commit":{"id":"abc"},"assets":{"count":0}}`))
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
		rootCmd.SetArgs([]string{"release", "create", "--project", "foo/bar", "--tag", "v1.0", "--name", "Release 1.0", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"tagName": "v1.0"`) {
		t.Errorf("expected tagName in output, got: %s", out)
	}
}

func TestRelease_Update_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.0","name":"Updated","description":"desc","created_at":"2024-01-01","released_at":"2024-01-01"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"release", "update", "--project", "foo/bar", "--tag", "v1.0", "--name", "Updated", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"tagName": "v1.0"`) {
		t.Errorf("expected tagName in output, got: %s", out)
	}
}

func TestRelease_Delete_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"tag_name":"v1.0","name":"Release 1.0"}`))
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
		rootCmd.SetArgs([]string{"release", "delete", "--project", "foo/bar", "--tag", "v1.0", "--json", "--force"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
}

func TestRelease_List_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0","name":"Release 1.0","description":"desc","created_at":"2024-01-01","released_at":"2024-01-01"}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"release", "list", "--project", "group/proj"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "v1.0") {
		t.Errorf("expected tag name in plain text output:\n%s", out)
	}
}

func TestRelease_Get_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.0","name":"Release 1.0","description":"desc","created_at":"2024-01-01","released_at":"2024-01-01"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"release", "get", "--project", "group/proj", "--tag", "v1.0"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "v1.0") {
		t.Errorf("expected tag name in plain text output:\n%s", out)
	}
}

func TestRelease_Create_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"tag_name":"v1.0","name":"Release 1.0","description":"desc","created_at":"2024-01-01","released_at":"2024-01-01"}`))
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
		rootCmd.SetArgs([]string{"release", "create", "--project", "foo/bar", "--tag", "v1.0", "--name", "Release 1.0"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Created") {
		t.Errorf("expected 'Created' in output, got: %s", out)
	}
}

func TestRelease_Update_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.0","name":"Updated","description":"desc","created_at":"2024-01-01","released_at":"2024-01-01"}`))
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
		rootCmd.SetArgs([]string{"release", "update", "--project", "foo/bar", "--tag", "v1.0", "--name", "Updated"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Updated") {
		t.Errorf("expected 'Updated' in output, got: %s", out)
	}
}

func TestRelease_List_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"release", "list", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestRelease_List_Empty(t *testing.T) {
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
		rootCmd.SetArgs([]string{"release", "list", "--project", "foo/bar"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No releases") {
		t.Errorf("expected 'No releases' in output, got: %s", out)
	}
}

func releaseNoAuth(t *testing.T) {
	t.Helper()
	resetReleaseCmdFlags(t)
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")
}

func resetReleaseCmdFlags(t *testing.T) {
	t.Helper()
	resetRootPersistentFlags(t)
	for _, kv := range []struct {
		cmd   *cobra.Command
		name  string
		value string
	}{
		{releaseListCmd, "project", ""},
		{releaseListCmd, "limit", "20"},
		{releaseGetCmd, "project", ""},
		{releaseGetCmd, "tag", ""},
		{releaseCreateCmd, "project", ""},
		{releaseCreateCmd, "tag", ""},
		{releaseCreateCmd, "name", ""},
		{releaseUpdateCmd, "project", ""},
		{releaseUpdateCmd, "tag", ""},
		{releaseDeleteCmd, "project", ""},
		{releaseDeleteCmd, "tag", ""},
	} {
		if err := kv.cmd.Flags().Set(kv.name, kv.value); err != nil {
			t.Fatalf("reset release flag %s.%s: %v", kv.cmd.Name(), kv.name, err)
		}
	}
}

func TestRelease_List_NewClientError(t *testing.T) {
	releaseNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"release", "list", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestRelease_List_InvalidLimit(t *testing.T) {
	resetReleaseCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"release", "list", "--project", "g/p", "--limit", "0"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRelease_List_WithAuthor_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0","name":"Release 1.0","released_at":"2024-01-01","author":{"username":"alice"}}]`))
	}))
	defer srv.Close()
	resetReleaseCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"release", "list", "--project", "g/p"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "alice") {
		t.Errorf("expected author in output, got:\n%s", out)
	}
}

func TestRelease_Get_NewClientError(t *testing.T) {
	releaseNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"release", "get", "--project", "g/p", "--tag", "v1.0"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestRelease_Get_MissingTag(t *testing.T) {
	resetReleaseCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"release", "get", "--project", "g/p", "--tag", ""})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRelease_Get_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	resetReleaseCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"release", "get", "--project", "g/p", "--tag", "v1.0"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestRelease_Get_WithAuthor_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.0","name":"Release 1.0","description":"desc","released_at":"2024-01-01","author":{"username":"alice"}}`))
	}))
	defer srv.Close()
	resetReleaseCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"release", "get", "--project", "g/p", "--tag", "v1.0"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "alice") {
		t.Errorf("expected author in output, got:\n%s", out)
	}
}

func TestRelease_Create_NewClientError(t *testing.T) {
	releaseNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"release", "create", "--project", "g/p", "--tag", "v1.0", "--name", "R1"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestRelease_Create_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"400 Bad Request"}`))
	}))
	defer srv.Close()
	resetReleaseCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"release", "create", "--project", "g/p", "--tag", "v1.0", "--name", "R1"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestRelease_Update_NewClientError(t *testing.T) {
	releaseNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"release", "update", "--project", "g/p", "--tag", "v1.0", "--name", "Updated"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestRelease_Update_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"400 Bad Request"}`))
	}))
	defer srv.Close()
	resetReleaseCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"release", "update", "--project", "g/p", "--tag", "v1.0", "--name", "Updated"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestRelease_Delete_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	resetReleaseCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"release", "delete", "--project", "g/p", "--tag", "v1.0", "--force"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Deleted release") {
		t.Errorf("expected Deleted release in output, got:\n%s", out)
	}
}

func TestRelease_Delete_NewClientError(t *testing.T) {
	releaseNoAuth(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"release", "delete", "--project", "g/p", "--tag", "v1.0", "--force"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestRelease_Delete_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	resetReleaseCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"release", "delete", "--project", "g/p", "--tag", "v1.0", "--force"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit, got %d", lastExit)
	}
}

func TestRelease_Update_MissingTag(t *testing.T) {
	resetReleaseCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"release", "update", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRelease_Delete_MissingTag(t *testing.T) {
	resetReleaseCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"release", "delete", "--project", "g/p"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestRelease_Delete_ConfirmRejected(t *testing.T) {
	withNonInteractiveStdin(t)
	resetReleaseCmdFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"release", "delete", "--project", "g/p", "--tag", "v1.0", "--confirm", "wrong"})
	_ = rootCmd.Execute()
	if lastExit != ExitCancelled {
		t.Errorf("exit = %d, want %d", lastExit, ExitCancelled)
	}
}
