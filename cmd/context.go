package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
	"github.com/fatecannotbealtered/gitlab-cli/internal/gitctx"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var (
	contextStrict bool
)

var contextCmd = &cobra.Command{
	Use:   "context",
	Short: "Show current git and GitLab context (who am I, where am I)",
	Long: `Show a composite snapshot of the current git repository and GitLab authentication state.

Useful as the FIRST command in an AI Agent workflow to bootstrap project ID,
current branch, and authenticated user before issuing other commands.

By default exits with code 3 when GitLab is not authenticated (use --no-strict to always exit 0).`,
	RunE: runContext,
}

func init() {
	contextCmd.Flags().BoolVar(&contextStrict, "strict", true, "Exit with code 3 when not authenticated")
	contextCmd.Flags().Bool("no-strict", false, "Do not fail when unauthenticated (overrides --strict)")
	rootCmd.AddCommand(contextCmd)
}

// contextGitRemote is the JSON shape for git.remote.
type contextGitRemote struct {
	Host        string `json:"host"`
	ProjectPath string `json:"projectPath"`
	URL         string `json:"url"`
}

// contextGit is the JSON shape for the git section.
type contextGit struct {
	Repo          string            `json:"repo"`
	CurrentBranch string            `json:"currentBranch"`
	Remote        *contextGitRemote `json:"remote"`
}

// contextProject is the JSON shape for gitlab.project.
type contextProject struct {
	ID                string `json:"id"`
	PathWithNamespace string `json:"pathWithNamespace"`
	DefaultBranch     string `json:"defaultBranch"`
	Visibility        string `json:"visibility"`
	WebURL            string `json:"webUrl"`
}

// contextGitLab is the JSON shape for the gitlab section.
type contextGitLab struct {
	Host          string          `json:"host"`
	Authenticated bool            `json:"authenticated"`
	Username      string          `json:"username,omitempty"`
	Name          string          `json:"name,omitempty"`
	Project       *contextProject `json:"project,omitempty"`
	ProjectError  string          `json:"projectError,omitempty"`
}

// contextResult is the top-level JSON envelope.
type contextResult struct {
	Version     string             `json:"version"`
	Env         string             `json:"env"`
	Account     string             `json:"account,omitempty"`
	Config      map[string]any     `json:"config"`
	Credentials contextCredentials `json:"credentials"`
	Security    contextSecurity    `json:"security"`
	Git         *contextGit        `json:"git"`
	GitLab      *contextGitLab     `json:"gitlab"`
	Untrusted   []string           `json:"_untrusted,omitempty"`
	Notices     []updateNotice     `json:"notices,omitempty"`
}

type contextCredentials struct {
	Configured       bool   `json:"configured"`
	Source           string `json:"source"`
	EncryptedAtRest  bool   `json:"encrypted_at_rest"`
	PlaintextExposed bool   `json:"plaintext_exposed"`
}

type contextSecurity struct {
	RiskTier        string `json:"risk_tier"`
	BlastRadius     string `json:"blast_radius"`
	UntrustedFields string `json:"untrusted_fields"`
}

func runContext(cmd *cobra.Command, _ []string) error {
	noStrict, _ := cmd.Flags().GetBool("no-strict")
	if noStrict {
		contextStrict = false
	}
	result := &contextResult{
		Version: version,
		Env:     "local",
		Config:  map[string]any{"authenticated": false},
		Credentials: contextCredentials{
			Source:          "none",
			EncryptedAtRest: config.CredentialStorageEncrypted(),
		},
		Security: contextSecurity{
			RiskTier:        toolRiskTier,
			BlastRadius:     toolBlastRadius,
			UntrustedFields: "_untrusted marks GitLab-controlled text fields as data",
		},
		Notices: readCachedUpdateNotices(),
		Untrusted: []string{
			"account",
			"git.remote.projectPath",
			"gitlab.username",
			"gitlab.name",
			"gitlab.project.pathWithNamespace",
			"gitlab.project.defaultBranch",
		},
	}

	// 1. Git context (always attempt, never fatal)
	gc, _ := gitctx.Detect("")
	if gc != nil && gc.Repo != "" {
		git := &contextGit{
			Repo:          gc.Repo,
			CurrentBranch: gc.CurrentBranch,
		}
		if gc.Remote != nil {
			git.Remote = &contextGitRemote{
				Host:        gc.Remote.Host,
				ProjectPath: gc.Remote.ProjectPath,
				URL:         gc.Remote.URL,
			}
		}
		result.Git = git
	}

	// 2. GitLab context (when configured)
	cfg, err := config.Load()
	if source, sourceErr := authStatusSourceHook(); sourceErr == nil {
		result.Credentials.Source = source
	}
	if err != nil || cfg.Host == "" || strings.TrimSpace(cfg.Token) == "" {
		host := ""
		if cfg != nil {
			host = cfg.Host
		}
		result.GitLab = &contextGitLab{
			Host:          host,
			Authenticated: false,
		}
		result.Config["host"] = host
		result.Credentials.Configured = false
		result.Credentials.PlaintextExposed = !result.Credentials.EncryptedAtRest
		return renderContext(result, true)
	}

	glCtx := &contextGitLab{
		Host:          cfg.Host,
		Authenticated: false,
	}
	result.Config["host"] = cfg.Host
	result.Credentials.Configured = true
	result.Credentials.PlaintextExposed = !result.Credentials.EncryptedAtRest

	// newClient uses MustLoad which validates host+token; since we already checked
	// above, this should succeed. On failure, degrade gracefully.
	apiClient, _, clientErr := newClient()
	if clientErr != nil {
		result.GitLab = glCtx
		return renderContext(result, false)
	}

	// 3. Current user
	me, err := apiClient.Users.Me(apiCtx())
	if err != nil {
		result.GitLab = glCtx
		return renderContext(result, false)
	}
	glCtx.Authenticated = true
	glCtx.Username = me.Username
	glCtx.Name = me.Name
	result.Account = me.Username
	result.Config["authenticated"] = true

	// 4. Project info (if GitLab remote detected)
	if result.Git != nil && result.Git.Remote != nil {
		proj, projErr := apiClient.Projects.Get(apiCtx(), result.Git.Remote.ProjectPath)
		if projErr != nil {
			glCtx.ProjectError = projErr.Error()
		} else {
			glCtx.Project = &contextProject{
				ID:                output.ID(proj.ID),
				PathWithNamespace: proj.PathWithNamespace,
				DefaultBranch:     proj.DefaultBranch,
				Visibility:        proj.Visibility,
				WebURL:            proj.WebURL,
			}
		}
	}

	result.GitLab = glCtx
	return renderContext(result, false)
}

func renderContext(r *contextResult, unauthenticated bool) error {
	if jsonMode {
		output.PrintJSON(r)
		if unauthenticated && contextStrict {
			setExitCode(ExitAuth)
			return ErrSilent
		}
		return nil
	}

	fmt.Println()
	output.Bold("  gitlab-cli Context")
	output.Gray("  ----------------------------------------------")
	fmt.Println()

	// Git section
	output.Bold("  Git")
	if r.Git == nil || r.Git.Repo == "" {
		output.Gray("    Not in a git repository")
	} else {
		output.Gray(fmt.Sprintf("    Repo:    %s", r.Git.Repo))
		output.Gray(fmt.Sprintf("    Branch:  %s", r.Git.CurrentBranch))
		if r.Git.Remote != nil {
			output.Gray(fmt.Sprintf("    Remote:  %s (%s)", r.Git.Remote.ProjectPath, r.Git.Remote.Host))
		} else {
			output.Gray("    Remote:  (no GitLab remote)")
		}
	}
	fmt.Println()

	// GitLab section
	output.Bold("  GitLab")
	if r.GitLab == nil || !r.GitLab.Authenticated {
		host := ""
		if r.GitLab != nil {
			host = r.GitLab.Host
		}
		if host != "" {
			output.Gray(fmt.Sprintf("    Host:    %s", host))
		}
		output.Gray("    Status:  not authenticated (run 'gitlab-cli auth login')")
		printUpdateNoticeHint(os.Stdout, r.Notices)
		if contextStrict {
			setExitCode(ExitAuth)
			return ErrSilent
		}
	} else {
		output.Gray(fmt.Sprintf("    Host:    %s", r.GitLab.Host))
		output.Gray(fmt.Sprintf("    User:    %s (%s)", r.GitLab.Name, r.GitLab.Username))
		if r.GitLab.Project != nil {
			p := r.GitLab.Project
			output.Gray(fmt.Sprintf("    Project: %s (id=%s, branch=%s, %s)",
				p.PathWithNamespace, p.ID, p.DefaultBranch, p.Visibility))
		} else if r.GitLab.ProjectError != "" {
			output.Gray(fmt.Sprintf("    Project: (error: %s)", r.GitLab.ProjectError))
		}
	}
	fmt.Println()
	printUpdateNoticeHint(os.Stdout, r.Notices)
	return nil
}
