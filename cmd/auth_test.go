package cmd

import (
	"bytes"
	"strings"
	"testing"
)

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
	if !strings.Contains(out, `"dryRun": true`) {
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
	if !strings.Contains(out, `"dryRun": true`) {
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
	jsonMode = false

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
