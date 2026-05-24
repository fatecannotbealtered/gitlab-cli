package cmd

import (
	"fmt"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check configuration and connectivity",
	RunE:  runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(_ *cobra.Command, _ []string) error {
	type doctorResult struct {
		ConfigExists bool   `json:"configExists"`
		AuthValid    bool   `json:"authValid"`
		LatencyMs    int64  `json:"latencyMs"`
		Host         string `json:"host,omitempty"`
		Username     string `json:"username,omitempty"`
		Name         string `json:"name,omitempty"`
		Error        string `json:"error,omitempty"`
	}

	result := doctorResult{}

	cfg, err := config.Load()
	if err != nil {
		result.Error = err.Error()
		if jsonMode {
			output.PrintJSON(result)
		} else {
			output.Error("Reading config: " + err.Error())
		}
		setExitCode(ExitAuth)
		return ErrSilent
	}

	if cfg.Host == "" || cfg.Token == "" {
		result.ConfigExists = false
		result.Error = "not configured: run 'gitlab-cli auth login' or set GITLAB_HOST and GITLAB_TOKEN"
		if jsonMode {
			output.PrintJSON(result)
		} else {
			fmt.Println()
			output.Bold("  gitlab-cli Doctor")
			output.Gray("  ────────────────────────────────────────")
			fmt.Println()
			output.Error(result.Error)
			fmt.Println()
		}
		setExitCode(ExitAuth)
		return ErrSilent
	}
	result.ConfigExists = true
	result.Host = cfg.Host

	client := api.NewClient(cfg)
	start := time.Now()
	me, err := client.Users.Me(apiCtx())
	latency := time.Since(start).Milliseconds()
	result.LatencyMs = latency

	if err != nil {
		result.AuthValid = false
		result.Error = err.Error()
		if jsonMode {
			output.PrintJSON(result)
		} else {
			fmt.Println()
			output.Bold("  gitlab-cli Doctor")
			output.Gray("  ────────────────────────────────────────")
			fmt.Println()
			output.Error("Connection failed: " + err.Error())
			fmt.Println()
		}
		// Map status code to exit code via handleAPIError-style logic.
		var apiErr *api.APIError
		if asAPI(err, &apiErr) {
			setExitCode(exitCodeForStatus(apiErr.StatusCode))
		} else {
			setExitCode(ExitNetwork)
		}
		return ErrSilent
	}

	result.AuthValid = true
	result.Username = me.Username
	result.Name = me.Name

	if jsonMode {
		output.PrintJSON(result)
		return nil
	}

	fmt.Println()
	output.Bold("  gitlab-cli Doctor")
	output.Gray("  ────────────────────────────────────────")
	fmt.Println()
	output.Success("Config found")
	output.Success("PAT valid (Bearer authentication)")
	output.Success(fmt.Sprintf("Connected to %s", cfg.Host))
	output.Success(fmt.Sprintf("Authenticated as %s (%s)", me.Name, me.Username))
	output.Gray(fmt.Sprintf("  Latency: %dms", latency))
	fmt.Println()
	return nil
}

// asAPI is a small helper to keep doctor.go decoupled from errors.As call site sprawl.
func asAPI(err error, target **api.APIError) bool {
	type unwrapper interface{ Unwrap() error }
	for cur := err; cur != nil; {
		if v, ok := cur.(*api.APIError); ok {
			*target = v
			return true
		}
		u, ok := cur.(unwrapper)
		if !ok {
			return false
		}
		cur = u.Unwrap()
	}
	return false
}
