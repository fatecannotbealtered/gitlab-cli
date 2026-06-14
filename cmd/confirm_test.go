package cmd

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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

	payload := map[string]any{"id": "yes"}
	token, _ := newConfirmToken("proceed", payload)
	confirmFlag = token

	if err := requireConfirm(&cobra.Command{}, "proceed", payload); err != nil {
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
	if lastExit != ExitConflict {
		t.Fatalf("exit=%d want=%d", lastExit, ExitConflict)
	}
}

func TestRequireConfirm_ForceDoesNotBypassToken(t *testing.T) {
	t.Setenv("GITLAB_CLI_AGENT_SAFE", "0")
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
		t.Fatalf("requireConfirm(force) = %v", err)
	}
	if lastExit != ExitConfirm {
		t.Fatalf("exit=%d want=%d", lastExit, ExitConfirm)
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
	if lastExit != ExitConfirm {
		t.Fatalf("exit=%d want=%d", lastExit, ExitConfirm)
	}
}

func TestRequireConfirm_MissingToken(t *testing.T) {
	origConfirm := confirmFlag
	origForce := forceMode
	origExit := lastExit
	defer func() {
		confirmFlag = origConfirm
		forceMode = origForce
		lastExit = origExit
	}()
	confirmFlag = ""
	forceMode = false
	lastExit = 0

	err := requireConfirm(&cobra.Command{}, "delete branch", "feat/x")
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("requireConfirm() = %v", err)
	}
	if lastExit != ExitConfirm {
		t.Fatalf("exit=%d want=%d", lastExit, ExitConfirm)
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

	stderr := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"issue", "close", "5", "--project", "group/proj", "--json"})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitConfirm {
		t.Errorf("exit code = %d, want %d (ExitConfirm)", lastExit, ExitConfirm)
	}
	for _, want := range []string{`"code": "E_CONFIRMATION_REQUIRED"`, "confirmation required", `--confirm`} {
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
	payload := map[string]any{"project": "group/proj", "name": "feat/x"}
	token, _ := newConfirmToken("repo branch delete", payload)

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"repo", "branch", "delete",
			"--project", "group/proj", "--name", "feat/x",
			"--dangerous", "--confirm", token, "--json",
		})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("exit code = %d, want %d", lastExit, ExitOK)
	}
	if !deleted {
		t.Error("expected DeleteBranch API call when --confirm token matches operation")
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
			"--dry-run", "--dangerous", "--json",
		})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("exit code = %d, want %d", lastExit, ExitOK)
	}
	for _, want := range []string{`"confirm_token"`, `"action": "release delete"`, `"tag": "v1.0.0"`} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q, got:\n%s", want, out)
		}
	}
}

func TestRequireConfirm_GeneratedTokenAccepted(t *testing.T) {
	origConfirm := confirmFlag
	defer func() { confirmFlag = origConfirm }()
	payload := map[string]any{"name": "secret"}
	token, _ := newConfirmToken("delete branch", payload)
	confirmFlag = token

	if err := requireConfirm(&cobra.Command{}, "delete branch", payload); err != nil {
		t.Fatalf("requireConfirm() = %v", err)
	}
}

func TestRequireConfirm_TokenPayloadMismatch(t *testing.T) {
	origExit := lastExit
	origConfirm := confirmFlag
	defer func() {
		confirmFlag = origConfirm
		lastExit = origExit
	}()
	token, _ := newConfirmToken("delete branch", map[string]any{"name": "secret"})
	confirmFlag = token
	lastExit = 0

	if err := requireConfirm(&cobra.Command{}, "delete branch", map[string]any{"name": "other"}); err == nil {
		t.Fatal("expected rejection")
	}
	if lastExit != ExitConflict {
		t.Fatalf("exit=%d want=%d", lastExit, ExitConflict)
	}
}

func TestRequireConfirm_ExpiredToken(t *testing.T) {
	origExit := lastExit
	origConfirm := confirmFlag
	origNow := confirmNow
	defer func() {
		confirmFlag = origConfirm
		confirmNow = origNow
		lastExit = origExit
	}()
	payload := map[string]any{"name": "secret"}
	token, expires := newConfirmToken("delete branch", payload)
	confirmFlag = token
	confirmNow = func() time.Time { return expires.Add(time.Second) }
	lastExit = 0

	if err := requireConfirm(&cobra.Command{}, "delete branch", payload); err == nil {
		t.Fatal("expected expired token error")
	}
	if lastExit != ExitConflict {
		t.Fatalf("exit=%d want=%d", lastExit, ExitConflict)
	}
}
