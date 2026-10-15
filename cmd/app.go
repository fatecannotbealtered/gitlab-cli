// Package cmd is gitlab-cli's command tree on the aicli library (ai-native-cli-spec
// 2.0). NewApp builds a fresh App and tree for every run; each command group
// registers itself from one register function in its own file.
package cmd

import (
	"context"
	"os"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"

	gitlabcli "github.com/fatecannotbealtered/gitlab-cli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
)

// version is set at build time:
// -ldflags "-X github.com/fatecannotbealtered/gitlab-cli/cmd.version=<version>".
var version = "dev"

// appVersion is the version this binary reports: the ldflags value, else the module
// version of a `go install` build, else "dev". aicli reports a development build as
// the next patch after the newest changelog release (1.2.17 gives 1.2.18-dev, P14),
// so reference, changelog and the conformance suite see a semantic version.
func appVersion() string {
	if version != "dev" && version != "" {
		return strings.TrimPrefix(version, "v")
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return "dev"
}

// gitlabSettings are the values gitlab-cli reads with App.Setting (CLI-100): a
// flag where one is declared, then GITLAB_CLI_<NAME>, then the active profile of
// settings.json, then the default. The token is a secret: it never comes from the
// settings file; without GITLAB_CLI_TOKEN it comes from the OS keyring entry of the
// active profile, written by `auth login` (SEC-040).
func gitlabSettings() []aicli.Setting {
	return []aicli.Setting{
		{Name: "host", Default: "https://gitlab.com", Description: "GitLab URL: https://, or http:// for localhost only"},
		{Name: "token", Secret: true, Description: "Personal access token; GITLAB_CLI_TOKEN only (the settings file refuses it), else the OS keyring entry of the profile"},
		{Name: "timeout", Default: "30s", Description: "Timeout of each GitLab API request, at most 30s (CLI-110)"},
	}
}

// NewApp builds a fresh App and command tree. env is how the App reads the
// environment: main passes os.Getenv, tests a map, the conformance suite its own
// overlay. Read the environment only through app.Env.
func NewApp(env func(string) string) *aicli.App {
	if env == nil {
		env = os.Getenv
	}
	root := &cobra.Command{Use: "gitlab-cli",
		Short: "GitLab for AI agents: merge requests, issues, pipelines, repositories, labels and milestones"}
	app := &aicli.App{
		Name: "gitlab-cli", Version: appVersion(), RiskTier: "T2", EnvPrefix: "GITLAB_CLI",
		// The whole CHANGELOG.md: without --since, changelog serves the newest entries
		// that fit the size budget (P3).
		Root: root, Changelog: gitlabcli.ChangelogMarkdown, Env: env,
		Settings: gitlabSettings(),
		// The built-in update (CLI-070): the release workflow of this repository signs
		// checksums.txt; npm, pnpm and yarn installs run the package manager. No
		// GoPackage: the module path has no /v2 suffix, so `go install ...@v2` cannot
		// resolve (see docs/migration-2.0/findings.md).
		Update: &aicli.UpdateConfig{Repo: "fatecannotbealtered/gitlab-cli", NPMPackage: "@fateforge/gitlab-cli"},
		// The fleet's measured guesses (CLI-022). Groups add their own from their
		// register function (app.AddShortcut for Shortcuts), never by editing this file.
		Shortcuts:    map[string][]string{},
		ShortFlags:   map[string]string{"p": "project", "R": "project"},
		FlagAliases:  map[string]string{"repo": "project"},
		FlagSynonyms: map[string][]string{"all": {"limit"}},
		VerbSynonyms: map[string][]string{},
	}
	app.Identity = func(context.Context) (aicli.Identity, error) { return gitlabIdentity(app) }
	app.DoctorChecks = func(ctx context.Context) []aicli.Check { return gitlabDoctorChecks(ctx, app) }
	app.ContextExtra = func(context.Context) map[string]any { return gitlabContextExtra() }
	registerCommands(app)
	return app
}

// registerCommands registers every command group in a fixed order (the order of
// the Skill's command map). Each group lives in its own files; a group not yet
// migrated to 2.0 has an empty register function in cmd/pending_<group>.go.
func registerCommands(app *aicli.App) {
	registerLabel(app)
	registerMilestone(app)
	registerMR(app)
	registerIssue(app)
	registerRepo(app)
	registerCI(app)
	registerProject(app)
	registerAuth(app)
}

// addGroup adds a noun to the root and registers its summary (App.Group).
func addGroup(app *aicli.App, use, summary string, aliases ...string) *cobra.Command {
	g := &cobra.Command{Use: use, Aliases: aliases}
	app.Root.AddCommand(g)
	app.Group(g, summary)
	return g
}
