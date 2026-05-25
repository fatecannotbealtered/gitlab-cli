package cmd

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestIsTerminal_Nil(t *testing.T) {
	if isTerminal(nil) {
		t.Fatal("nil file should not be terminal")
	}
}

func TestIsTerminal_Pipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(r) {
		t.Fatal("pipe should not be terminal")
	}
}

func TestIsTerminal_ClosedFile(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	_ = r.Close()
	if isTerminal(r) {
		t.Fatal("closed file should not be terminal")
	}
}

func TestRequireConfirm_MatchingToken(t *testing.T) {
	origConfirm := confirmFlag
	origForce := forceMode
	defer func() {
		confirmFlag = origConfirm
		forceMode = origForce
	}()
	confirmFlag = "yes"
	forceMode = false

	if err := requireConfirm(&cobra.Command{}, "proceed", "yes"); err != nil {
		t.Fatalf("requireConfirm() = %v", err)
	}
}

func TestRequireConfirm_WrongToken_NonTTY(t *testing.T) {
	withNonInteractiveStdin(t)
	origConfirm := confirmFlag
	origForce := forceMode
	origExit := lastExit
	defer func() {
		confirmFlag = origConfirm
		forceMode = origForce
		lastExit = origExit
	}()
	confirmFlag = "nope"
	forceMode = false
	lastExit = 0

	err := requireConfirm(&cobra.Command{}, "delete branch", "feat/x")
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("requireConfirm() = %v", err)
	}
	if lastExit != ExitCancelled {
		t.Fatalf("exit=%d want=%d", lastExit, ExitCancelled)
	}
}

func TestRequireConfirm_ForceAllowed(t *testing.T) {
	t.Setenv("GITLAB_CLI_AGENT_SAFE", "0")
	origConfirm := confirmFlag
	origForce := forceMode
	defer func() {
		confirmFlag = origConfirm
		forceMode = origForce
	}()
	confirmFlag = ""
	forceMode = true

	if err := requireConfirm(&cobra.Command{}, "delete", "secret"); err != nil {
		t.Fatalf("requireConfirm(force) = %v", err)
	}
}

func TestRequireConfirm_ForceBlockedInAgentSafe(t *testing.T) {
	withAgentSafe(t)
	origConfirm := confirmFlag
	origForce := forceMode
	origExit := lastExit
	defer func() {
		confirmFlag = origConfirm
		forceMode = origForce
		lastExit = origExit
	}()
	confirmFlag = ""
	forceMode = true
	lastExit = 0

	err := requireConfirm(&cobra.Command{}, "delete", "secret")
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("requireConfirm() = %v", err)
	}
	if lastExit != ExitBadArgs {
		t.Fatalf("exit=%d want=%d", lastExit, ExitBadArgs)
	}
}

func TestRequireConfirm_TTY_ReadError(t *testing.T) {
	candidates := []string{"/dev/null"}
	if runtime.GOOS == "windows" {
		candidates = append([]string{"NUL", "CONIN$"}, candidates...)
	}

	var f *os.File
	for _, name := range candidates {
		file, err := os.Open(name)
		if err != nil {
			continue
		}
		if !isTerminal(file) {
			file.Close()
			continue
		}
		f = file
		break
	}
	if f == nil {
		t.Skip("no character device available for TTY read-error test")
	}
	defer f.Close()

	origStdin := os.Stdin
	origConfirm := confirmFlag
	origForce := forceMode
	origExit := lastExit
	defer func() {
		os.Stdin = origStdin
		confirmFlag = origConfirm
		forceMode = origForce
		lastExit = origExit
	}()
	os.Stdin = f
	confirmFlag = ""
	forceMode = false
	lastExit = 0

	err := requireConfirm(&cobra.Command{}, "delete branch", "feat/x")
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("requireConfirm() = %v", err)
	}
	if lastExit != ExitCancelled {
		t.Fatalf("exit=%d want=%d", lastExit, ExitCancelled)
	}
}

func TestRequireConfirm_NonTTY_ExitCancelled(t *testing.T) {
	t.Setenv("GITLAB_CLI_AGENT_SAFE", "0")
	withNonInteractiveStdin(t)
	resetRootPersistentFlags(t)

	origExit := lastExit
	origJM := jsonMode
	defer func() {
		lastExit = origExit
		jsonMode = origJM
		resetRootPersistentFlags(t)
	}()
	lastExit = 0
	jsonMode = true

	stderr := captureStderr(t, func() {
		rootCmd.SetArgs([]string{"issue", "close", "5", "--project", "group/proj", "--json"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitCancelled {
		t.Errorf("exit code = %d, want %d (ExitCancelled)", lastExit, ExitCancelled)
	}
	for _, want := range []string{`"errorCode": "CANCELLED"`, "confirmation required", `--confirm`} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q, got:\n%s", want, stderr)
		}
	}
}

func TestRequireConfirm_ConfirmToken_AllowsExecution(t *testing.T) {
	t.Setenv("GITLAB_CLI_AGENT_SAFE", "0")
	resetRootPersistentFlags(t)

	deleted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/repository/branches/") {
			deleted = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	origDR := dryRun
	origJM := jsonMode
	origExit := lastExit
	defer func() {
		dryRun = origDR
		jsonMode = origJM
		lastExit = origExit
		resetRootPersistentFlags(t)
	}()
	dryRun = false
	lastExit = 0
	jsonMode = true

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"repo", "branch", "delete",
			"--project", "group/proj", "--name", "feat/x",
			"--confirm", "feat/x", "--json",
		})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("exit code = %d, want %d", lastExit, ExitOK)
	}
	if !deleted {
		t.Error("expected DeleteBranch API call when --confirm matches branch name")
	}
}

func TestDryRun_Delete_NoForceRequired(t *testing.T) {
	t.Setenv("GITLAB_CLI_AGENT_SAFE", "0")
	resetRootPersistentFlags(t)

	origDR := dryRun
	origJM := jsonMode
	origExit := lastExit
	defer func() {
		dryRun = origDR
		jsonMode = origJM
		lastExit = origExit
		resetRootPersistentFlags(t)
	}()
	lastExit = 0
	jsonMode = true

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"release", "delete",
			"--project", "group/proj", "--tag", "v1.0.0",
			"--dry-run", "--json",
		})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("exit code = %d, want %d", lastExit, ExitOK)
	}
	for _, want := range []string{`"dryRun": true`, `"action": "release delete"`, `"tag": "v1.0.0"`} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q, got:\n%s", want, out)
		}
	}
}

func TestRequireConfirm_HookTTY_Accepted(t *testing.T) {
	origIn := readConfirmInputFunc
	origTTY := stdinIsTerminalFunc
	defer func() {
		readConfirmInputFunc = origIn
		stdinIsTerminalFunc = origTTY
	}()
	stdinIsTerminalFunc = func(*os.File) bool { return true }
	readConfirmInputFunc = func() (string, error) { return "secret", nil }

	if err := requireConfirm(&cobra.Command{}, "delete branch", "secret"); err != nil {
		t.Fatalf("requireConfirm() = %v", err)
	}
}

func TestRequireConfirm_HookTTY_Rejected(t *testing.T) {
	origIn := readConfirmInputFunc
	origTTY := stdinIsTerminalFunc
	origExit := lastExit
	defer func() {
		readConfirmInputFunc = origIn
		stdinIsTerminalFunc = origTTY
		lastExit = origExit
	}()
	stdinIsTerminalFunc = func(*os.File) bool { return true }
	readConfirmInputFunc = func() (string, error) { return "wrong", nil }
	lastExit = 0

	if err := requireConfirm(&cobra.Command{}, "delete branch", "secret"); err == nil {
		t.Fatal("expected rejection")
	}
	if lastExit != ExitCancelled {
		t.Fatalf("exit=%d want=%d", lastExit, ExitCancelled)
	}
}

func TestRequireConfirm_HookTTY_ReadError(t *testing.T) {
	origIn := readConfirmInputFunc
	origTTY := stdinIsTerminalFunc
	origExit := lastExit
	defer func() {
		readConfirmInputFunc = origIn
		stdinIsTerminalFunc = origTTY
		lastExit = origExit
	}()
	stdinIsTerminalFunc = func(*os.File) bool { return true }
	readConfirmInputFunc = func() (string, error) { return "", io.EOF }
	lastExit = 0

	if err := requireConfirm(&cobra.Command{}, "delete branch", "secret"); err == nil {
		t.Fatal("expected read error")
	}
	if lastExit != ExitCancelled {
		t.Fatalf("exit=%d want=%d", lastExit, ExitCancelled)
	}
}
