package main

import (
	"os"
	"runtime"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/cmd"
)

func TestRunSuccess(t *testing.T) {
	if code := run([]string{"gitlab-cli", "--version"}); code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
}

func TestRunHelp(t *testing.T) {
	if code := run([]string{"gitlab-cli", "--help"}); code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
}

func TestRunSilentError(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	code := run([]string{"gitlab-cli", "mr", "list"})
	if code != cmd.ExitBadArgs {
		t.Fatalf("run() = %d, want %d", code, cmd.ExitBadArgs)
	}
}

func TestRunRegularError(t *testing.T) {
	code := run([]string{"gitlab-cli", "not-a-real-command"})
	if code != cmd.ExitBadArgs {
		t.Fatalf("run() = %d, want %d", code, cmd.ExitBadArgs)
	}
}

func TestMainCallsRun(t *testing.T) {
	var got int
	orig := osExit
	osExit = func(code int) { got = code }
	defer func() { osExit = orig }()

	os.Args = []string{"gitlab-cli", "--version"}
	main()
	if got != 0 {
		t.Fatalf("main() exit = %d, want 0", got)
	}
}

func TestRunSilentExitCode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
	code := run([]string{"gitlab-cli", "context", "--json"})
	if code != cmd.ExitAuth {
		t.Fatalf("run() = %d, want %d", code, cmd.ExitAuth)
	}
}
