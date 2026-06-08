package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// test hooks for interactive login
var (
	stdinIsTerminalForAuth = func() bool { return term.IsTerminal(int(syscall.Stdin)) }
	readPasswordForAuth    = func() ([]byte, error) { return term.ReadPassword(int(syscall.Stdin)) }
	authStatusSourceHook   = authStatusSource
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Configure GitLab authentication",
	Long: `Configure GitLab host and Personal Access Token (PAT).

Authentication precedence (highest first):
  1. GITLAB_CLI_HOST / GITLAB_CLI_TOKEN  (isolates this CLI from glab)
  2. GITLAB_HOST     / GITLAB_TOKEN      (compatible with glab and other tooling)
  3. Active profile in ~/.gitlab-cli/profiles.json  (saved by 'gitlab-cli auth login')
  4. ~/.gitlab-cli/config.json           (legacy fallback)`,
}

var authLoginCmd = &cobra.Command{
	Use:   "login",
	Short: "Save GitLab credentials to ~/.gitlab-cli/config.json",
	Long: `Save credentials interactively, or pass --host and --token for non-interactive use.

Credentials are resolved with this precedence (highest first):
  GITLAB_CLI_* > GITLAB_* > active profile > ~/.gitlab-cli/config.json

Examples:
  gitlab-cli auth login
  gitlab-cli auth login --host https://gitlab.example.com --token <PAT>`,
	RunE: runAuthLogin,
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove saved GitLab credentials",
	RunE:  runAuthLogout,
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	RunE:  runAuthStatus,
}

var authProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage named GitLab credential profiles",
}

var authProfileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved profiles",
	RunE:  runAuthProfileList,
}

var authProfileUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Switch active profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if done, err := prepareWrite(cmd, "use profile", map[string]any{"name": args[0]}); done || err != nil {
			return err
		}
		if err := config.UseProfile(args[0]); err != nil {
			return failArg(err.Error())
		}
		if jsonMode {
			output.PrintJSON(map[string]any{"active": args[0], "status": "ok"})
			return nil
		}
		output.Success("Active profile: " + args[0])
		return nil
	},
}

var authProfileRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a saved profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if done, err := prepareWrite(cmd, "remove profile", map[string]any{"name": args[0]}); done || err != nil {
			return err
		}
		if err := config.DeleteProfile(args[0]); err != nil {
			return failArg(err.Error())
		}
		if jsonMode {
			output.PrintJSON(map[string]string{"status": "removed", "name": args[0]})
			return nil
		}
		output.Success("Removed profile: " + args[0])
		return nil
	},
}

var (
	authLoginHostFlag    string
	authLoginTokenFlag   string
	authLoginProfileFlag string
)

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authProfileCmd)
	authProfileCmd.AddCommand(authProfileListCmd)
	authProfileCmd.AddCommand(authProfileUseCmd)
	authProfileCmd.AddCommand(authProfileRemoveCmd)

	authLoginCmd.Flags().StringVar(&authLoginHostFlag, "host", "", "GitLab host URL (e.g. https://gitlab.example.com)")
	authLoginCmd.Flags().StringVar(&authLoginTokenFlag, "token", "", "Personal Access Token (PAT); prefer GITLAB_CLI_TOKEN env var")
	authLoginCmd.Flags().StringVar(&authLoginProfileFlag, "profile", "default", "Profile name to save credentials under")

	markWrite(authLoginCmd)
	markWrite(authLogoutCmd)
	markWrite(authProfileUseCmd)
	markWrite(authProfileRemoveCmd)
}

func runAuthProfileList(_ *cobra.Command, _ []string) error {
	pf, err := config.LoadProfiles()
	if err != nil {
		return failWithCode(err.Error(), ExitNetwork, output.ErrNetwork)
	}
	names := config.ProfileNames(pf)
	if jsonMode {
		items := make([]map[string]any, 0, len(names))
		for _, n := range names {
			c := pf.Profiles[n]
			items = append(items, output.MarkUntrusted(map[string]any{
				"name":   n,
				"host":   c.Host,
				"active": n == pf.Active,
			}, "name", "host"))
		}
		output.PrintJSON(output.NewListEnvelope(items, output.ListMeta{
			Count:   len(items),
			Limit:   len(items),
			HasMore: false,
		}))
		return nil
	}
	output.Info(fmt.Sprintf("Active profile: %s", pf.Active))
	for _, n := range names {
		c := pf.Profiles[n]
		mark := ""
		if n == pf.Active {
			mark = " (active)"
		}
		output.Gray(fmt.Sprintf("  %s: %s%s", n, c.Host, mark))
	}
	return nil
}

func runAuthLogin(cmd *cobra.Command, _ []string) error {
	// Non-interactive mode: both --host and --token provided
	if authLoginHostFlag != "" && authLoginTokenFlag != "" {
		host := strings.TrimSpace(authLoginHostFlag)
		if !strings.HasPrefix(host, "https://") && !strings.HasPrefix(host, "http://") {
			return failArg("host must start with https:// (or http:// for local development)")
		}
		token := strings.TrimSpace(authLoginTokenFlag)
		if token == "" {
			return failArg("token cannot be empty")
		}

		if done, err := prepareWrite(cmd, "save credentials", map[string]any{"host": host}); done || err != nil {
			return err
		}

		cfg := &config.Config{Host: host, Token: token}
		client := api.NewClient(cfg)
		me, err := client.Users.Me(apiCtx())
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if err := config.SetProfile(authLoginProfileFlag, cfg); err != nil {
			return failWithCode("failed to save profile: "+err.Error(), ExitNetwork, output.ErrNetwork)
		}

		if jsonMode {
			output.PrintJSON(output.MarkUntrusted(map[string]any{
				"status":   "ok",
				"username": me.Username,
				"name":     me.Name,
				"host":     host,
				"profile":  authLoginProfileFlag,
			}, "username", "name"))
			return nil
		}
		output.Success(fmt.Sprintf("Logged in as %s (%s)", me.Name, me.Username))
		output.Info("Profile saved: " + authLoginProfileFlag)
		return nil
	}

	if dryRun && authLoginHostFlag != "" {
		host := strings.TrimSpace(authLoginHostFlag)
		if !strings.HasPrefix(host, "https://") && !strings.HasPrefix(host, "http://") {
			return failArg("host must start with https:// (or http:// for local development)")
		}
		if done, err := prepareWrite(cmd, "save credentials", map[string]any{"host": host}); done || err != nil {
			return err
		}
	}

	if jsonMode {
		return failArg("auth login requires --host and --token in json mode; use --format text for interactive login")
	}

	// Interactive mode
	reader := bufio.NewReader(os.Stdin)

	fmt.Println()
	output.Bold("  gitlab-cli Login")
	output.Gray("  ────────────────────────────────────────")
	fmt.Println()

	host := authLoginHostFlag
	if host == "" {
		fmt.Print("  GitLab host (e.g. https://gitlab.example.com): ")
		host, _ = reader.ReadString('\n')
		host = strings.TrimSpace(host)
	}
	if !strings.HasPrefix(host, "https://") && !strings.HasPrefix(host, "http://") {
		return failArg("host must start with https:// (or http:// for local development)")
	}

	token := authLoginTokenFlag
	if token == "" {
		fmt.Print("  Personal Access Token (PAT) [scope: api]: ")
		var tokenBytes []byte
		var err error
		if stdinIsTerminalForAuth() {
			tokenBytes, err = readPasswordForAuth()
			fmt.Println()
			if err != nil {
				return failWithCode("failed to read token: "+err.Error(), ExitNetwork, output.ErrNetwork)
			}
		} else {
			line, _ := reader.ReadString('\n')
			tokenBytes = []byte(strings.TrimSpace(line))
		}
		token = strings.TrimSpace(string(tokenBytes))
	}
	if token == "" {
		return failArg("token cannot be empty")
	}

	output.Gray("  Verifying credentials...")
	if done, err := prepareWrite(cmd, "save credentials", map[string]any{"host": host}); done || err != nil {
		return err
	}
	cfg := &config.Config{Host: host, Token: token}
	client := api.NewClient(cfg)
	me, err := client.Users.Me(apiCtx())
	if err != nil {
		return handleAPIError(err, jsonMode)
	}

	if err := config.SetProfile(authLoginProfileFlag, cfg); err != nil {
		return failWithCode("failed to save profile: "+err.Error(), ExitNetwork, output.ErrNetwork)
	}

	fmt.Println()
	output.Success(fmt.Sprintf("Logged in as %s (%s)", me.Name, me.Username))
	output.Info("Profile saved: " + authLoginProfileFlag)
	fmt.Println()
	output.Gray("  Try: gitlab-cli doctor")
	fmt.Println()
	return nil
}

func runAuthLogout(cmd *cobra.Command, _ []string) error {
	if done, err := prepareWrite(cmd, "delete credentials", map[string]any{"path": config.FilePath()}); done || err != nil {
		return err
	}
	if err := config.ClearStoredCredentials(); err != nil {
		return failWithCode("failed to remove credentials: "+err.Error(), ExitNetwork, output.ErrNetwork)
	}
	if jsonMode {
		output.PrintJSON(map[string]string{"status": "logged_out"})
		return nil
	}
	output.Success("Logged out. Config removed.")
	return nil
}

// authStatusSource reports the highest-precedence credential source, matching config.Load().
func authStatusSource() (string, error) {
	if os.Getenv("GITLAB_CLI_HOST") != "" || os.Getenv("GITLAB_CLI_TOKEN") != "" {
		return "env-cli", nil
	}
	if os.Getenv("GITLAB_HOST") != "" || os.Getenv("GITLAB_TOKEN") != "" {
		return "env", nil
	}
	pf, err := config.LoadProfiles()
	if err != nil {
		return "", err
	}
	if pf.Active != "" {
		if c, ok := pf.Profiles[pf.Active]; ok && c != nil && c.Host != "" && strings.TrimSpace(c.Token) != "" {
			return "profile", nil
		}
	}
	fileCfg, exists, err := config.LoadStoredConfig()
	if err != nil {
		return "", err
	}
	if !exists {
		return "none", nil
	}
	if fileCfg.Host != "" && strings.TrimSpace(fileCfg.Token) != "" {
		return "file", nil
	}
	return "none", nil
}

func runAuthStatus(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return failWithCode("reading config: "+err.Error(), ExitNetwork, output.ErrNetwork)
	}

	type statusResult struct {
		Configured bool   `json:"configured"`
		Host       string `json:"host,omitempty"`
		TokenSet   bool   `json:"tokenSet"`
		Source     string `json:"source"` // "env-cli" / "env" / "profile" / "file" / "none"
	}

	source, err := authStatusSourceHook()
	if err != nil {
		return failWithCode("reading config: "+err.Error(), ExitNetwork, output.ErrNetwork)
	}

	result := statusResult{
		Configured: cfg.Host != "" && cfg.Token != "",
		Host:       cfg.Host,
		TokenSet:   strings.TrimSpace(cfg.Token) != "",
		Source:     source,
	}

	if jsonMode {
		output.PrintJSON(result)
		if !result.Configured {
			setExitCode(ExitAuth)
			return ErrSilent
		}
		return nil
	}

	fmt.Println()
	output.Bold("  gitlab-cli Auth Status")
	output.Gray("  ────────────────────────────────────────")
	fmt.Println()
	if result.Configured {
		output.Success(fmt.Sprintf("Configured (host=%s, source=%s)", result.Host, result.Source))
	} else {
		output.Warn("Not configured. Run 'gitlab-cli auth login' or set GITLAB_HOST and GITLAB_TOKEN.")
		setExitCode(ExitAuth)
		return ErrSilent
	}
	return nil
}
