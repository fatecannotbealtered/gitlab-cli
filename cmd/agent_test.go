package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestAgentSafeMode_DefaultOn(t *testing.T) {
	t.Setenv("GITLAB_CLI_AGENT_SAFE", "")
	if !agentSafeMode() {
		t.Fatal("agentSafeMode() should default to true")
	}
}

func TestAgentSafeMode_Disabled(t *testing.T) {
	for _, v := range []string{"0", "false", "FALSE"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("GITLAB_CLI_AGENT_SAFE", v)
			if agentSafeMode() {
				t.Fatalf("agentSafeMode(%q) should be false", v)
			}
		})
	}
}

func TestAgentSafeMode_Enabled(t *testing.T) {
	t.Setenv("GITLAB_CLI_AGENT_SAFE", "1")
	if !agentSafeMode() {
		t.Fatal("agentSafeMode(1) should be true")
	}
}

func TestAllowForce_AgentSafeDisabled(t *testing.T) {
	t.Setenv("GITLAB_CLI_AGENT_SAFE", "0")
	if err := allowForce(); err != nil {
		t.Fatalf("allowForce() = %v", err)
	}
}

func TestAllowForce_AllowedByEnv(t *testing.T) {
	withAgentSafe(t)
	t.Setenv("GITLAB_CLI_ALLOW_FORCE", "1")
	if err := allowForce(); err != nil {
		t.Fatalf("allowForce() = %v", err)
	}
}

func TestAllowForce_RejectedInAgentSafe(t *testing.T) {
	withAgentSafe(t)
	origJM := jsonMode
	origExit := lastExit
	defer func() {
		jsonMode = origJM
		lastExit = origExit
	}()
	jsonMode = true
	lastExit = 0

	err := allowForce()
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("allowForce() = %v", err)
	}
	if lastExit != ExitBadArgs {
		t.Fatalf("exit=%d want=%d", lastExit, ExitBadArgs)
	}
}

func TestAllowShowValues_AgentSafeDisabled(t *testing.T) {
	t.Setenv("GITLAB_CLI_AGENT_SAFE", "0")
	if err := allowShowValues(); err != nil {
		t.Fatalf("allowShowValues() = %v", err)
	}
}

func TestAllowShowValues_AllowedByEnv(t *testing.T) {
	withAgentSafe(t)
	t.Setenv("GITLAB_CLI_ALLOW_SHOW_VALUES", "1")
	if err := allowShowValues(); err != nil {
		t.Fatalf("allowShowValues() = %v", err)
	}
}

func TestAllowShowValues_RejectedInAgentSafe(t *testing.T) {
	withAgentSafe(t)
	origJM := jsonMode
	origExit := lastExit
	defer func() {
		jsonMode = origJM
		lastExit = origExit
	}()
	jsonMode = true
	lastExit = 0

	err := allowShowValues()
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("allowShowValues() = %v", err)
	}
	if lastExit != ExitBadArgs {
		t.Fatalf("exit=%d want=%d", lastExit, ExitBadArgs)
	}
}

func TestCheckShowValues_NoFlag(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("show-values", false, "")
	if err := checkShowValues(cmd); err != nil {
		t.Fatalf("checkShowValues() = %v", err)
	}
}

func TestAgentSafe_ShowValuesRejected(t *testing.T) {
	withAgentSafe(t)
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
		rootCmd.SetArgs([]string{
			"variable", "list",
			"--project", "group/proj",
			"--show-values", "--json",
		})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
	if !strings.Contains(stderr, "show-values") {
		t.Errorf("stderr missing show-values rejection:\n%s", stderr)
	}
}

func TestAgentSafe_ForceRejected(t *testing.T) {
	withAgentSafe(t)
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
		rootCmd.SetArgs([]string{
			"repo", "branch", "delete",
			"--project", "group/proj", "--name", "feat/x",
			"--force", "--json",
		})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", lastExit, ExitBadArgs)
	}
	for _, want := range []string{`"code": "E_VALIDATION"`, "--force", "--dry-run", "--confirm"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q, got:\n%s", want, stderr)
		}
	}
}

func TestAgentSafe_DryRunDelete_NoForceRequired(t *testing.T) {
	withAgentSafe(t)
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
			"repo", "branch", "delete",
			"--project", "group/proj", "--name", "feat/x",
			"--dry-run", "--dangerous", "--json",
		})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Errorf("exit code = %d, want %d", lastExit, ExitOK)
	}
	for _, want := range []string{`"confirm_token"`, `"action": "repo branch delete"`} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout missing %q, got:\n%s", want, out)
		}
	}
}
