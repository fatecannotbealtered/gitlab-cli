package cmd

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
)

func mockUserServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/user" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1,"username":"alice","name":"Alice"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func clearAuthEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")
}

func TestAuth_Help_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"auth", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(nil)
	out := buf.String()
	for _, want := range []string{"login", "logout", "status"} {
		if !strings.Contains(out, want) {
			t.Errorf("auth --help missing %q\noutput:\n%s", want, out)
		}
	}
}

func TestAuth_Login_DryRun(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"auth", "login",
			"--host", "https://gitlab.example.com",
			"--token", "mytoken",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
	if !strings.Contains(out, `"confirm_token"`) {
		t.Errorf("expected dry-run JSON, got:\n%s", out)
	}
}

func TestAuth_Logout_DryRun(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"auth", "logout", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
	if !strings.Contains(out, `"confirm_token"`) {
		t.Errorf("expected dry-run JSON, got:\n%s", out)
	}
}

func TestAuth_Status_NoConfig(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"auth", "status", "--json"})
	_ = rootCmd.Execute()

	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit when not configured, got %d", lastExit)
	}
}

func TestAuth_Status_ProfileOnly(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")

	if err := config.SetProfile("default", &config.Config{
		Host:  "https://gitlab.example.com",
		Token: "profiletoken",
	}); err != nil {
		t.Fatalf("SetProfile: %v", err)
	}

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"auth", "status", "--json"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0 when profile configured, got %d", lastExit)
	}
	if !strings.Contains(out, `"source": "profile"`) {
		t.Errorf("expected source=profile, got:\n%s", out)
	}
	if !strings.Contains(out, `"configured": true`) {
		t.Errorf("expected configured=true, got:\n%s", out)
	}
}

func TestAuth_Status_JSON_Configured(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "mytoken")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"auth", "status", "--json"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0 when configured, got %d", lastExit)
	}
	if !strings.Contains(out, `"configured"`) {
		t.Errorf("expected configured field in JSON, got:\n%s", out)
	}
}

func TestAuth_Logout_PlainText_DryRun(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	setTextFormatForTest(t)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"auth", "logout", "--dry-run"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
	if !strings.Contains(out, "dry-run") {
		t.Errorf("expected dry-run in output:\n%s", out)
	}
}

func TestAuth_ProfileList_JSON(t *testing.T) {
	home := isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)

	writeGitLabCLIFile(t, home, "profiles.json", `{
  "active": "work",
  "profiles": {
    "default": {"host": "https://gitlab.example.com", "token": "t1"},
    "work": {"host": "https://work.example.com", "token": "t2"}
  }
}`)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"auth", "profile", "list", "--json"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
	if !strings.Contains(out, `"active": "work"`) || !strings.Contains(out, `"name": "work"`) {
		t.Errorf("expected profile list JSON, got:\n%s", out)
	}
}

func TestAuth_ProfileList_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	home := isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)

	writeGitLabCLIFile(t, home, "profiles.json", `{
  "active": "default",
  "profiles": {
    "default": {"host": "https://gitlab.example.com", "token": "t1"}
  }
}`)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"auth", "profile", "list"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
	if !strings.Contains(out, "Active profile: default") || !strings.Contains(out, "(active)") {
		t.Errorf("expected active profile marker, got:\n%s", out)
	}
}

func TestAuth_ProfileList_LoadError(t *testing.T) {
	home := isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)
	writeGitLabCLIFile(t, home, "profiles.json", `{invalid`)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"auth", "profile", "list", "--json"})
	_ = rootCmd.Execute()

	if lastExit != ExitNetwork {
		t.Errorf("expected exit %d, got %d", ExitNetwork, lastExit)
	}
}

func TestAuth_Login_NonInteractive_Success(t *testing.T) {
	setTextFormatForTest(t)
	srv := mockUserServer(t)
	defer srv.Close()

	isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"auth", "login",
			"--host", srv.URL,
			"--token", "secret",
			"--profile", "work",
		})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
	if !strings.Contains(out, "Logged in as Alice") || !strings.Contains(out, "work") {
		t.Errorf("expected success output, got:\n%s", out)
	}
	pf, err := config.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles: %v", err)
	}
	if pf.Active != "work" || pf.Profiles["work"].Token != "secret" {
		t.Fatalf("profile not saved: %+v", pf)
	}
}

func TestAuth_Login_NonInteractive_JSON(t *testing.T) {
	srv := mockUserServer(t)
	defer srv.Close()

	isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"auth", "login",
			"--host", srv.URL,
			"--token", "secret",
			"--json",
		})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
	if !strings.Contains(out, `"username": "alice"`) || !strings.Contains(out, `"status": "ok"`) {
		t.Errorf("expected login JSON, got:\n%s", out)
	}
}

func TestAuth_Login_NonInteractive_InvalidHost(t *testing.T) {
	resetAuthLoginFlags(t)
	clearAuthEnv(t)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"auth", "login", "--host", "gitlab.example.com", "--token", "tok"})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("expected exit %d, got %d", ExitBadArgs, lastExit)
	}
}

func TestAuth_Login_NonInteractive_EmptyToken(t *testing.T) {
	resetAuthLoginFlags(t)
	clearAuthEnv(t)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"auth", "login", "--host", "https://gitlab.example.com", "--token", "   "})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("expected exit %d, got %d", ExitBadArgs, lastExit)
	}
}

func TestAuth_Login_NonInteractive_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	defer srv.Close()

	resetAuthLoginFlags(t)
	clearAuthEnv(t)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"auth", "login", "--host", srv.URL, "--token", "bad", "--json"})
	_ = rootCmd.Execute()

	if lastExit != ExitAuth {
		t.Errorf("expected exit %d, got %d", ExitAuth, lastExit)
	}
}

func TestAuth_Login_NonInteractive_SaveError(t *testing.T) {
	srv := mockUserServer(t)
	defer srv.Close()

	home := isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)
	blockProfilesRemove(t, home)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"auth", "login", "--host", srv.URL, "--token", "secret", "--json"})
	_ = rootCmd.Execute()

	if lastExit != ExitNetwork {
		t.Errorf("expected exit %d, got %d", ExitNetwork, lastExit)
	}
}

func TestAuth_Login_Interactive_Success(t *testing.T) {
	srv := mockUserServer(t)
	defer srv.Close()

	isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)
	setTextFormatForTest(t)
	withStdinInput(t, "my-token\n")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"auth", "login", "--host", srv.URL})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
	if !strings.Contains(out, "Logged in as Alice") || !strings.Contains(out, "Try: gitlab-cli doctor") {
		t.Errorf("expected interactive success output, got:\n%s", out)
	}
}

func TestAuth_Login_Interactive_ReadHostFromStdin(t *testing.T) {
	srv := mockUserServer(t)
	defer srv.Close()

	isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)
	setTextFormatForTest(t)
	withStdinInput(t, srv.URL+"\nsecret\n")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"auth", "login"})
	_ = rootCmd.Execute()

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
}

func TestAuth_Login_Interactive_InvalidHost(t *testing.T) {
	resetAuthLoginFlags(t)
	clearAuthEnv(t)
	setTextFormatForTest(t)
	withStdinInput(t, "bad-host\n")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"auth", "login"})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("expected exit %d, got %d", ExitBadArgs, lastExit)
	}
}

func TestAuth_Login_Interactive_EmptyToken(t *testing.T) {
	resetAuthLoginFlags(t)
	clearAuthEnv(t)
	setTextFormatForTest(t)
	withStdinInput(t, "\n")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"auth", "login", "--host", "https://gitlab.example.com"})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("expected exit %d, got %d", ExitBadArgs, lastExit)
	}
}

func TestAuth_Login_Interactive_DryRun(t *testing.T) {
	resetAuthLoginFlags(t)
	clearAuthEnv(t)
	withStdinInput(t, "secret\n")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"auth", "login", "--host", "https://gitlab.example.com", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
	if !strings.Contains(out, `"confirm_token"`) {
		t.Errorf("expected dry-run JSON, got:\n%s", out)
	}
}

func TestAuth_Login_Interactive_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"403 Forbidden"}`))
	}))
	defer srv.Close()

	resetAuthLoginFlags(t)
	clearAuthEnv(t)
	setTextFormatForTest(t)
	withStdinInput(t, "secret\n")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"auth", "login", "--host", srv.URL})
	_ = rootCmd.Execute()

	if lastExit != ExitForbidden {
		t.Errorf("expected exit %d, got %d", ExitForbidden, lastExit)
	}
}

func TestAuth_Login_Interactive_SaveError(t *testing.T) {
	srv := mockUserServer(t)
	defer srv.Close()

	home := isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)
	setTextFormatForTest(t)
	blockProfilesRemove(t, home)
	withStdinInput(t, "secret\n")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"auth", "login", "--host", srv.URL})
	_ = rootCmd.Execute()

	if lastExit != ExitNetwork {
		t.Errorf("expected exit %d, got %d", ExitNetwork, lastExit)
	}
}

func TestAuth_Logout_Success_JSON(t *testing.T) {
	home := isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)
	writeGitLabCLIFile(t, home, "config.json", `{"host":"https://gitlab.example.com","token":"tok"}`)
	writeGitLabCLIFile(t, home, "profiles.json", `{"active":"default","profiles":{"default":{"host":"https://gitlab.example.com","token":"tok"}}}`)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"auth", "logout", "--json"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
	if !strings.Contains(out, `"status": "logged_out"`) {
		t.Errorf("expected logged_out JSON, got:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, ".gitlab-cli", "config.json")); !os.IsNotExist(err) {
		t.Errorf("expected config removed, stat err=%v", err)
	}
}

func TestAuth_Logout_Success_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	home := isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)
	writeGitLabCLIFile(t, home, "config.json", `{"host":"https://gitlab.example.com","token":"tok"}`)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"auth", "logout"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
	if !strings.Contains(out, "Logged out") {
		t.Errorf("expected logout message, got:\n%s", out)
	}
}

func TestAuth_Logout_RemoveError(t *testing.T) {
	home := isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)
	blockConfigRemove(t, home)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"auth", "logout", "--json"})
	_ = rootCmd.Execute()

	if lastExit != ExitNetwork {
		t.Errorf("expected exit %d, got %d", ExitNetwork, lastExit)
	}
}

func TestAuthStatusSource(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T, home string)
		env     map[string]string
		want    string
		wantErr bool
	}{
		{
			name: "env-cli",
			env:  map[string]string{"GITLAB_CLI_HOST": "https://a.example.com"},
			want: "env-cli",
		},
		{
			name: "env",
			env:  map[string]string{"GITLAB_HOST": "https://a.example.com"},
			want: "env",
		},
		{
			name: "profile",
			setup: func(t *testing.T, home string) {
				writeGitLabCLIFile(t, home, "profiles.json", `{
  "active": "default",
  "profiles": {"default": {"host": "https://gitlab.example.com", "token": "tok"}}
}`)
			},
			want: "profile",
		},
		{
			name: "profile active but empty token falls through",
			setup: func(t *testing.T, home string) {
				writeGitLabCLIFile(t, home, "profiles.json", `{
  "active": "default",
  "profiles": {"default": {"host": "https://gitlab.example.com", "token": ""}}
}`)
			},
			want: "none",
		},
		{
			name: "profile active but missing entry falls through",
			setup: func(t *testing.T, home string) {
				writeGitLabCLIFile(t, home, "profiles.json", `{
  "active": "ghost",
  "profiles": {"default": {"host": "https://gitlab.example.com", "token": "tok"}}
}`)
			},
			want: "none",
		},
		{
			name: "file empty credentials treated as none",
			setup: func(t *testing.T, home string) {
				writeGitLabCLIFile(t, home, "config.json", `{"host":"https://legacy.example.com","token":""}`)
			},
			want: "none",
		},
		{
			name: "file",
			setup: func(t *testing.T, home string) {
				writeGitLabCLIFile(t, home, "config.json", `{"host":"https://legacy.example.com","token":"legacy"}`)
			},
			want: "file",
		},
		{
			name: "none",
			want: "none",
		},
		{
			name: "profiles load error",
			setup: func(t *testing.T, home string) {
				writeGitLabCLIFile(t, home, "profiles.json", `{bad`)
			},
			wantErr: true,
		},
		{
			name: "config read error",
			setup: func(t *testing.T, home string) {
				dir := filepath.Join(home, ".gitlab-cli", "config.json")
				if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatalf("mkdir config.json: %v", err)
				}
			},
			wantErr: true,
		},
		{
			name: "config parse error",
			setup: func(t *testing.T, home string) {
				writeGitLabCLIFile(t, home, "config.json", `{bad`)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := isolateConfigHome(t)
			clearAuthEnv(t)
			for _, k := range []string{"GITLAB_CLI_HOST", "GITLAB_CLI_TOKEN", "GITLAB_HOST", "GITLAB_TOKEN"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if tt.setup != nil {
				tt.setup(t, home)
			}

			got, err := authStatusSource()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("authStatusSource: %v", err)
			}
			if got != tt.want {
				t.Errorf("authStatusSource = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAuth_Status_JSON_NotConfigured(t *testing.T) {
	isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"auth", "status", "--json"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitAuth {
		t.Errorf("expected exit %d, got %d", ExitAuth, lastExit)
	}
	if !strings.Contains(out, `"configured": false`) {
		t.Errorf("expected configured=false JSON, got:\n%s", out)
	}
}

func TestAuth_Status_LoadError_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	home := isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)
	writeGitLabCLIFile(t, home, "profiles.json", `{bad`)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"auth", "status"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitNetwork {
		t.Errorf("expected exit %d, got %d", ExitNetwork, lastExit)
	}
	if !strings.Contains(strings.ToLower(out), "reading config") {
		t.Errorf("expected config read error, got:\n%s", out)
	}
}

func TestAuth_Status_LoadError(t *testing.T) {
	home := isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)
	writeGitLabCLIFile(t, home, "profiles.json", `{bad`)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"auth", "status", "--json"})
	_ = rootCmd.Execute()

	if lastExit != ExitNetwork {
		t.Errorf("expected exit %d, got %d", ExitNetwork, lastExit)
	}
}

func TestAuth_Status_PlainText_Configured(t *testing.T) {
	setTextFormatForTest(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetAuthLoginFlags(t)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"auth", "status"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
	if !strings.Contains(out, "Configured") || !strings.Contains(out, "env-cli") {
		t.Errorf("expected configured status, got:\n%s", out)
	}
}

func TestAuth_Status_PlainText_NotConfigured(t *testing.T) {
	setTextFormatForTest(t)
	isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"auth", "status"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitAuth {
		t.Errorf("expected exit %d, got %d", ExitAuth, lastExit)
	}
	if !strings.Contains(out, "Not configured") {
		t.Errorf("expected not configured warning, got:\n%s", out)
	}
}

func TestAuth_Status_EnvSource(t *testing.T) {
	isolateConfigHome(t)
	clearAuthEnv(t)
	t.Setenv("GITLAB_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_TOKEN", "tok")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"auth", "status", "--json"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
	if !strings.Contains(out, `"source": "env"`) {
		t.Errorf("expected source=env, got:\n%s", out)
	}
}

func TestAuth_Status_AuthStatusSourceError(t *testing.T) {
	home := isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)
	writeGitLabCLIFile(t, home, "profiles.json", `{
  "active": "default",
  "profiles": {"default": {"host": "https://gitlab.example.com", "token": "tok"}}
}`)
	cfgDir := filepath.Join(home, ".gitlab-cli", "config.json")
	if err := os.MkdirAll(filepath.Dir(cfgDir), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"auth", "status", "--json"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitNetwork {
		t.Errorf("expected exit %d, got %d", ExitNetwork, lastExit)
	}
	if !strings.Contains(out, `"error"`) && !strings.Contains(strings.ToLower(out), "reading config") {
		t.Errorf("expected config error, got:\n%s", out)
	}
}

func TestAuth_Status_FileSource(t *testing.T) {
	home := isolateConfigHome(t)
	clearAuthEnv(t)
	writeGitLabCLIFile(t, home, "config.json", `{"host":"https://legacy.example.com","token":"legacy"}`)

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"auth", "status", "--json"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("expected exit 0, got %d", lastExit)
	}
	if !strings.Contains(out, `"source": "file"`) {
		t.Errorf("expected source=file, got:\n%s", out)
	}
}

func TestAuth_Login_NonInteractive_PlainText(t *testing.T) {
	srv := mockUserServer(t)
	defer srv.Close()
	isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)

	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	setTextFormatForTest(t)

	captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"auth", "login", "--host", srv.URL, "--token", "secret-token"})
		_ = rootCmd.Execute()
	})
}

func TestAuth_Login_Interactive_NonTTY_TokenLine(t *testing.T) {
	srv := mockUserServer(t)
	defer srv.Close()
	isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)
	setTextFormatForTest(t)

	origTTY := stdinIsTerminalForAuth
	defer func() { stdinIsTerminalForAuth = origTTY }()
	stdinIsTerminalForAuth = func() bool { return false }

	withStdinInput(t, "line-token\n")

	captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"auth", "login", "--host", srv.URL})
		_ = rootCmd.Execute()
	})

	pf, err := config.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles: %v", err)
	}
	if pf.Profiles["default"].Token != "line-token" {
		t.Fatalf("token = %q, want line-token", pf.Profiles["default"].Token)
	}
}

func TestAuth_Login_Interactive_PromptHost(t *testing.T) {
	srv := mockUserServer(t)
	defer srv.Close()
	isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)
	setTextFormatForTest(t)

	origTTY := stdinIsTerminalForAuth
	defer func() { stdinIsTerminalForAuth = origTTY }()
	stdinIsTerminalForAuth = func() bool { return false }

	withStdinInput(t, srv.URL+"\nsecret-token\n")

	captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"auth", "login"})
		_ = rootCmd.Execute()
	})
}

func TestAuth_Login_ReadPasswordErrorHook(t *testing.T) {
	srv := mockUserServer(t)
	defer srv.Close()
	resetAuthLoginFlags(t)
	setTextFormatForTest(t)

	origTTY := stdinIsTerminalForAuth
	origRead := readPasswordForAuth
	origExit := lastExit
	defer func() {
		stdinIsTerminalForAuth = origTTY
		readPasswordForAuth = origRead
		lastExit = origExit
	}()
	stdinIsTerminalForAuth = func() bool { return true }
	readPasswordForAuth = func() ([]byte, error) { return nil, errors.New("read failed") }
	lastExit = 0

	rootCmd.SetArgs([]string{"auth", "login", "--host", srv.URL})
	_ = rootCmd.Execute()
	if lastExit != ExitNetwork {
		t.Fatalf("exit=%d want=%d", lastExit, ExitNetwork)
	}
}

func TestAuth_Status_PlainText_WithHostUnconfigured(t *testing.T) {
	home := isolateConfigHome(t)
	clearAuthEnv(t)
	writeGitLabCLIFile(t, home, "config.json", `{"host":"https://partial.example.com","token":""}`)

	origJM := jsonMode
	origExit := lastExit
	defer func() {
		jsonMode = origJM
		lastExit = origExit
	}()
	setTextFormatForTest(t)
	lastExit = 0

	captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"auth", "status"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitAuth {
		t.Fatalf("exit=%d want=%d", lastExit, ExitAuth)
	}
}

func TestAuth_ProfileUse_PlainText(t *testing.T) {
	home := isolateConfigHome(t)
	writeGitLabCLIFile(t, home, "profiles.json", `{
  "active": "default",
  "profiles": {
    "default": {"host": "https://a.example.com", "token": "t1"},
    "work": {"host": "https://b.example.com", "token": "t2"}
  }
}`)
	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	setTextFormatForTest(t)

	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"auth", "profile", "use", "work"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Active profile: work") {
		t.Fatalf("expected plain success, got:\n%s", out)
	}
}

func TestAuth_ProfileRemove_PlainText(t *testing.T) {
	home := isolateConfigHome(t)
	writeGitLabCLIFile(t, home, "profiles.json", `{
  "active": "default",
  "profiles": {
    "default": {"host": "https://a.example.com", "token": "t1"},
    "work": {"host": "https://b.example.com", "token": "t2"}
  }
}`)
	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	setTextFormatForTest(t)

	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"auth", "profile", "remove", "work"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Removed profile: work") {
		t.Fatalf("expected plain success, got:\n%s", out)
	}
}

func TestAuth_Status_AuthStatusSourceHookError(t *testing.T) {
	isolateConfigHome(t)
	clearAuthEnv(t)
	origHook := authStatusSourceHook
	origExit := lastExit
	defer func() {
		authStatusSourceHook = origHook
		lastExit = origExit
	}()
	authStatusSourceHook = func() (string, error) { return "", errors.New("source boom") }
	lastExit = 0

	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"auth", "status"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitNetwork {
		t.Fatalf("exit=%d want=%d", lastExit, ExitNetwork)
	}
	if !strings.Contains(strings.ToLower(out), "source boom") {
		t.Fatalf("expected source error, got:\n%s", out)
	}
}

func TestAuth_DefaultTerminalHook(t *testing.T) {
	_ = stdinIsTerminalForAuth()
}
