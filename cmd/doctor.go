package cmd

import (
	"fmt"
	"os"
	"strings"
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
	type doctorCheck struct {
		Check  string  `json:"check"`
		Status string  `json:"status"`
		Fix    *string `json:"fix"`
	}
	type doctorResult struct {
		Checks          []doctorCheck  `json:"checks"`
		Notices         []updateNotice `json:"notices,omitempty"`
		Version         string         `json:"version"`
		SkillMinVersion string         `json:"skill_min_version"`
		RiskTier        string         `json:"risk_tier"`
		ConfigExists    bool           `json:"configExists"`
		AuthValid       bool           `json:"authValid"`
		LatencyMs       int64          `json:"latencyMs"`
		Host            string         `json:"host,omitempty"`
		Username        string         `json:"username,omitempty"`
		Name            string         `json:"name,omitempty"`
		Error           string         `json:"error,omitempty"`
	}

	result := doctorResult{
		Version:         version,
		SkillMinVersion: skillMinVersion,
		RiskTier:        toolRiskTier,
		Notices:         refreshUpdateNotices(apiCtx(), "doctor"),
	}
	check := func(name, status, fix string) {
		var fixPtr *string
		if fix != "" {
			fixPtr = &fix
		}
		result.Checks = append(result.Checks, doctorCheck{Check: name, Status: status, Fix: fixPtr})
	}

	if currentVersionMeetsSkillMin() {
		check("version", "pass", "")
	} else {
		check("version", "fail", "npm install -g @fatecannotbealtered-/gitlab-cli@latest && gitlab-cli changelog --since "+version)
	}

	cfg, err := config.Load()
	if err != nil {
		result.Error = err.Error()
		check("config", "fail", "run gitlab-cli auth login or fix the config file")
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
		check("config", "fail", "run gitlab-cli auth login or set GITLAB_HOST and GITLAB_TOKEN")
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
	check("config", "pass", "")

	client := api.NewClient(cfg)
	start := time.Now()
	me, err := client.Users.Me(apiCtx())
	latency := time.Since(start).Milliseconds()
	result.LatencyMs = latency

	if err != nil {
		result.AuthValid = false
		result.Error = err.Error()
		var apiErr *api.APIError
		if asAPI(err, &apiErr) && (apiErr.StatusCode == 401 || apiErr.StatusCode == 403) {
			check("network", "pass", "")
			check("auth", "fail", "check PAT scope and project membership")
		} else {
			check("network", "fail", "set HTTP_PROXY or check VPN/connectivity")
			check("auth", "fail", "retry after network connectivity is restored")
		}
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
	check("network", "pass", "")
	check("auth", "pass", "")

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
	printUpdateNoticeHint(os.Stdout, result.Notices)
	fmt.Println()
	return nil
}

func currentVersionMeetsSkillMin() bool {
	current := cleanVersion(version)
	if current == "" || current == "dev" || strings.Contains(current, "devel") {
		return true
	}
	return !semverGreater(skillMinVersion, current)
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
