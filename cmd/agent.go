package cmd

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// agentSafeMode is on by default (v1 targets AI Agents). Set GITLAB_CLI_AGENT_SAFE=0 to disable.
func agentSafeMode() bool {
	v := strings.TrimSpace(os.Getenv("GITLAB_CLI_AGENT_SAFE"))
	if v == "" {
		return true
	}
	return v != "0" && strings.ToLower(v) != "false"
}

func allowForce() error {
	if !agentSafeMode() {
		return nil
	}
	if strings.TrimSpace(os.Getenv("GITLAB_CLI_ALLOW_FORCE")) == "1" {
		return nil
	}
	return failArg("--force is disabled in agent-safe mode; set GITLAB_CLI_ALLOW_FORCE=1 when the user explicitly authorizes")
}

func allowShowValues() error {
	if !agentSafeMode() {
		return nil
	}
	if strings.TrimSpace(os.Getenv("GITLAB_CLI_ALLOW_SHOW_VALUES")) == "1" {
		return nil
	}
	return failArg("--show-values is disabled in agent-safe mode; set GITLAB_CLI_ALLOW_SHOW_VALUES=1 when the user explicitly needs secret values")
}

func checkShowValues(cmd *cobra.Command) error {
	showValues, _ := cmd.Flags().GetBool("show-values")
	if showValues {
		return allowShowValues()
	}
	return nil
}
