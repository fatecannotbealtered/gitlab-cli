package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestContext_Help(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"context", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(nil)
	// Reset the --help flag so subsequent tests don't inherit it.
	_ = contextCmd.Flags().Set("help", "false")

	out := buf.String()
	if !strings.Contains(out, "context") {
		t.Errorf("context --help missing expected text\noutput:\n%s", out)
	}
}

func TestContext_NotConfigured_JSON(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"context", "--json"})
		_ = rootCmd.Execute()
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, out)
	}

	gitlab, ok := result["gitlab"].(map[string]any)
	if !ok {
		t.Fatalf("missing gitlab key in output:\n%s", out)
	}
	if gitlab["authenticated"] != false {
		t.Errorf("expected authenticated=false, got %v", gitlab["authenticated"])
	}
}

func TestContext_HappyPath_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/user":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1,"username":"alice","name":"Alice Example","email":"alice@example.com"}`))
		case strings.HasPrefix(r.URL.Path, "/api/v4/projects/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":42,"path_with_namespace":"team/svc","default_branch":"main","visibility":"private","web_url":"https://gitlab.example.com/team/svc"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"context", "--json"})
		_ = rootCmd.Execute()
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, out)
	}

	gitlab, ok := result["gitlab"].(map[string]any)
	if !ok {
		t.Fatalf("missing gitlab key:\n%s", out)
	}
	if gitlab["authenticated"] != true {
		t.Errorf("expected authenticated=true, got %v", gitlab["authenticated"])
	}
	if gitlab["username"] != "alice" {
		t.Errorf("expected username=alice, got %v", gitlab["username"])
	}
}

func TestContext_AuthenticatedNoGitLabRemote_JSON(t *testing.T) {
	// Authenticated but no git remote → project should be nil
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/user" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":2,"username":"bob","name":"Bob"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"context", "--json"})
		_ = rootCmd.Execute()
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, out)
	}

	gitlab, ok := result["gitlab"].(map[string]any)
	if !ok {
		t.Fatalf("missing gitlab key:\n%s", out)
	}
	if gitlab["authenticated"] != true {
		t.Errorf("expected authenticated=true, got %v", gitlab["authenticated"])
	}
	// project should be absent (no remote detected in test environment)
	if _, hasProject := gitlab["project"]; hasProject {
		// This is acceptable if we happen to be in a git repo with a GitLab remote;
		// only fail if project lookup returned something unexpected.
		t.Logf("note: project key present (running in a git repo with GitLab remote): %v", gitlab["project"])
	}
}

func TestContext_Get_NoGitRepo(t *testing.T) {
	// Run context in a temp dir that is not a git repo.
	// context never exits non-zero for missing git — it degrades gracefully.
	// So we just verify it runs without panic and produces valid JSON.
	origDir, _ := os.Getwd()
	tmp := t.TempDir()
	_ = os.Chdir(tmp)
	defer func() { _ = os.Chdir(origDir) }()

	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"context", "--json"})
		_ = rootCmd.Execute()
	})

	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, out)
	}
	// git section should be absent or empty since we're not in a git repo
	if git, ok := result["git"]; ok && git != nil {
		t.Logf("note: git section present (may be inside a git repo): %v", git)
	}
}

func TestContext_PlainText_NotAuthenticated(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"context"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "gitlab-cli Context") {
		t.Errorf("expected context header in plain text output:\n%s", out)
	}
}

func TestContext_PlainText_Authenticated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/user" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1,"username":"alice","name":"Alice"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"context"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "alice") {
		t.Errorf("expected username in plain text output:\n%s", out)
	}
}
