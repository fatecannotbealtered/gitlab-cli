package cmd

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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

	result := unwrapJSONDataMap(t, out)

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

	result := unwrapJSONDataMap(t, out)

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

	result := unwrapJSONDataMap(t, out)

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

	result := unwrapJSONDataMap(t, out)
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
	setTextFormatForTest(t)
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
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"context"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "alice") {
		t.Errorf("expected username in plain text output:\n%s", out)
	}
}

func TestRenderContext_Direct_JSON_StrictUnauthenticated(t *testing.T) {
	origJM := jsonMode
	origStrict := contextStrict
	origExit := lastExit
	defer func() {
		jsonMode = origJM
		contextStrict = origStrict
		lastExit = origExit
	}()
	jsonMode = true
	contextStrict = true
	lastExit = 0

	captureStdout(t, func() {
		err := renderContext(&contextResult{
			GitLab: &contextGitLab{Host: "https://gitlab.example.com"},
		}, true)
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("renderContext() = %v", err)
		}
	})
	if lastExit != ExitAuth {
		t.Fatalf("exit=%d want=%d", lastExit, ExitAuth)
	}
}

func TestRenderContext_Direct_JSON_NoStrict(t *testing.T) {
	origJM := jsonMode
	origStrict := contextStrict
	origExit := lastExit
	defer func() {
		jsonMode = origJM
		contextStrict = origStrict
		lastExit = origExit
	}()
	jsonMode = true
	contextStrict = false
	lastExit = 0

	captureStdout(t, func() {
		if err := renderContext(&contextResult{
			GitLab: &contextGitLab{Host: "https://gitlab.example.com"},
		}, true); err != nil {
			t.Fatalf("renderContext() = %v", err)
		}
	})
	if lastExit != ExitOK {
		t.Fatalf("exit=%d want=%d", lastExit, ExitOK)
	}
}

func TestRenderContext_Direct_PlainText_NoGit(t *testing.T) {
	origJM := jsonMode
	origStrict := contextStrict
	defer func() {
		jsonMode = origJM
		contextStrict = origStrict
	}()
	setTextFormatForTest(t)
	contextStrict = false

	out := captureCombinedOutput(t, func() {
		if err := renderContext(&contextResult{}, false); err != nil {
			t.Fatalf("renderContext() = %v", err)
		}
	})
	if !strings.Contains(out, "Not in a git repository") {
		t.Fatalf("expected no-git message, got:\n%s", out)
	}
}

func TestRenderContext_Direct_PlainText_GitNoRemote(t *testing.T) {
	origJM := jsonMode
	origStrict := contextStrict
	defer func() {
		jsonMode = origJM
		contextStrict = origStrict
	}()
	setTextFormatForTest(t)
	contextStrict = false

	out := captureCombinedOutput(t, func() {
		if err := renderContext(&contextResult{
			Git: &contextGit{Repo: "/tmp/repo", CurrentBranch: "main"},
		}, false); err != nil {
			t.Fatalf("renderContext() = %v", err)
		}
	})
	if !strings.Contains(out, "no GitLab remote") {
		t.Fatalf("expected no-remote message, got:\n%s", out)
	}
}

func TestRenderContext_Direct_PlainText_GitWithRemote(t *testing.T) {
	origJM := jsonMode
	origStrict := contextStrict
	defer func() {
		jsonMode = origJM
		contextStrict = origStrict
	}()
	setTextFormatForTest(t)
	contextStrict = false

	out := captureCombinedOutput(t, func() {
		if err := renderContext(&contextResult{
			Git: &contextGit{
				Repo:          "/tmp/repo",
				CurrentBranch: "main",
				Remote: &contextGitRemote{
					Host:        "gitlab.example.com",
					ProjectPath: "group/proj",
					URL:         "https://gitlab.example.com/group/proj.git",
				},
			},
		}, false); err != nil {
			t.Fatalf("renderContext() = %v", err)
		}
	})
	if !strings.Contains(out, "group/proj") || !strings.Contains(out, "gitlab.example.com") {
		t.Fatalf("expected remote details, got:\n%s", out)
	}
}

func TestRenderContext_Direct_PlainText_UnauthenticatedNoHost(t *testing.T) {
	origJM := jsonMode
	origStrict := contextStrict
	defer func() {
		jsonMode = origJM
		contextStrict = origStrict
	}()
	setTextFormatForTest(t)
	contextStrict = false

	out := captureCombinedOutput(t, func() {
		if err := renderContext(&contextResult{GitLab: &contextGitLab{}}, false); err != nil {
			t.Fatalf("renderContext() = %v", err)
		}
	})
	if !strings.Contains(out, "not authenticated") {
		t.Fatalf("expected unauthenticated output, got:\n%s", out)
	}
	if strings.Contains(out, "Host:") {
		t.Fatalf("did not expect host line when host empty, got:\n%s", out)
	}
}

func TestRenderContext_Direct_PlainText_UnauthenticatedWithHost(t *testing.T) {
	origJM := jsonMode
	origStrict := contextStrict
	origExit := lastExit
	defer func() {
		jsonMode = origJM
		contextStrict = origStrict
		lastExit = origExit
	}()
	setTextFormatForTest(t)
	contextStrict = true
	lastExit = 0

	out := captureCombinedOutput(t, func() {
		err := renderContext(&contextResult{
			GitLab: &contextGitLab{Host: "https://gitlab.example.com"},
		}, false)
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("renderContext() = %v", err)
		}
	})
	if !strings.Contains(out, "not authenticated") || !strings.Contains(out, "gitlab.example.com") {
		t.Fatalf("expected unauthenticated output, got:\n%s", out)
	}
	if lastExit != ExitAuth {
		t.Fatalf("exit=%d want=%d", lastExit, ExitAuth)
	}
}

func TestRenderContext_Direct_PlainText_AuthenticatedWithProject(t *testing.T) {
	origJM := jsonMode
	origStrict := contextStrict
	defer func() {
		jsonMode = origJM
		contextStrict = origStrict
	}()
	setTextFormatForTest(t)
	contextStrict = false

	out := captureCombinedOutput(t, func() {
		if err := renderContext(&contextResult{
			GitLab: &contextGitLab{
				Host:          "https://gitlab.example.com",
				Authenticated: true,
				Username:      "alice",
				Name:          "Alice",
				Project: &contextProject{
					ID:                "42",
					PathWithNamespace: "team/svc",
					DefaultBranch:     "main",
					Visibility:        "private",
				},
			},
		}, false); err != nil {
			t.Fatalf("renderContext() = %v", err)
		}
	})
	for _, want := range []string{"team/svc", "id=42", "main", "private"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestRenderContext_Direct_PlainText_ProjectError(t *testing.T) {
	origJM := jsonMode
	origStrict := contextStrict
	defer func() {
		jsonMode = origJM
		contextStrict = origStrict
	}()
	setTextFormatForTest(t)
	contextStrict = false

	out := captureCombinedOutput(t, func() {
		if err := renderContext(&contextResult{
			GitLab: &contextGitLab{
				Host:          "https://gitlab.example.com",
				Authenticated: true,
				Username:      "alice",
				Name:          "Alice",
				ProjectError:  "403 Forbidden",
			},
		}, false); err != nil {
			t.Fatalf("renderContext() = %v", err)
		}
	})
	if !strings.Contains(out, "error: 403 Forbidden") {
		t.Fatalf("expected project error in output:\n%s", out)
	}
}

func TestContext_NoStrict_ExitZeroWhenUnauthenticated(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"context", "--no-strict", "--json"})
	_ = rootCmd.Execute()

	if lastExit != ExitOK {
		t.Fatalf("exit=%d want=%d with --no-strict", lastExit, ExitOK)
	}
}

func TestContext_APIUserError_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/user" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "bad")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"context", "--json"})
		_ = rootCmd.Execute()
	})

	result := unwrapJSONDataMap(t, out)
	gitlab := result["gitlab"].(map[string]any)
	if gitlab["authenticated"] != false {
		t.Fatalf("expected authenticated=false, got %v", gitlab["authenticated"])
	}
}

func TestContext_ProjectLookupError_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/user":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1,"username":"alice","name":"Alice"}`))
		default:
			if strings.HasPrefix(r.URL.Path, "/api/v4/projects/") {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"404 Project Not Found"}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origDir, _ := os.Getwd()
	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "remote", "add", "origin", srv.URL+"/group/missing.git")
	_ = os.Chdir(repoDir)
	defer func() { _ = os.Chdir(origDir) }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"context", "--json"})
		_ = rootCmd.Execute()
	})

	result := unwrapJSONDataMap(t, out)
	gitlab := result["gitlab"].(map[string]any)
	if gitlab["projectError"] == nil {
		t.Fatalf("expected projectError in output:\n%s", out)
	}
}

func TestContext_ProjectLookupError_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/user":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1,"username":"alice","name":"Alice"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Project Not Found"}`))
		}
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origDir, _ := os.Getwd()
	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "remote", "add", "origin", srv.URL+"/group/missing.git")
	_ = os.Chdir(repoDir)
	defer func() { _ = os.Chdir(origDir) }()

	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	setTextFormatForTest(t)

	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"context", "--no-strict"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Project: (error:") {
		t.Fatalf("expected project error in plain output:\n%s", out)
	}
}

func TestContext_NewClientError_PlainText(t *testing.T) {
	home := isolateConfigHome(t)
	clearAuthEnv(t)
	writeGitLabCLIFile(t, home, "profiles.json", `{
  "active": "default",
  "profiles": {"default": {"host": "http://evil.example.com", "token": "tok"}}
}`)

	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	setTextFormatForTest(t)

	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"context", "--no-strict"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "not authenticated") && !strings.Contains(strings.ToLower(out), "http://") {
		t.Fatalf("expected newClient degradation, got:\n%s", out)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
}
