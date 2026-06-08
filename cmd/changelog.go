package cmd

import (
	"fmt"
	"regexp"
	"strings"

	rootchangelog "github.com/fatecannotbealtered/gitlab-cli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var changelogSince string

var changelogCmd = &cobra.Command{
	Use:   "changelog",
	Short: "Show version changes from CHANGELOG.md",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		entries := parseChangelog(rootchangelog.ChangelogMarkdown)
		if changelogSince != "" {
			entries = filterChangelogSince(entries, changelogSince)
		}
		result := changelogResult{
			CurrentVersion: cleanVersion(rootCmd.Version),
			Since:          cleanVersion(changelogSince),
			Entries:        entries,
		}
		if jsonMode {
			output.PrintJSON(result)
			return nil
		}
		printChangelogText(cmd, result)
		return nil
	},
}

func init() {
	changelogCmd.Flags().StringVar(&changelogSince, "since", "", "Only include versions newer than this SemVer version")
	rootCmd.AddCommand(changelogCmd)
}

type changelogResult struct {
	CurrentVersion string           `json:"current_version"`
	Since          string           `json:"since,omitempty"`
	Entries        []changelogEntry `json:"entries"`
}

type changelogEntry struct {
	Version string              `json:"version"`
	Date    string              `json:"date,omitempty"`
	Changes map[string][]string `json:"changes"`
}

var changelogHeadingRe = regexp.MustCompile(`^## \[([^\]]+)\](?: - ([0-9]{4}-[0-9]{2}-[0-9]{2}))?`)

func parseChangelog(markdown string) []changelogEntry {
	var entries []changelogEntry
	var current *changelogEntry
	category := ""
	for _, line := range strings.Split(markdown, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := changelogHeadingRe.FindStringSubmatch(line); m != nil {
			if current != nil {
				entries = append(entries, *current)
			}
			current = &changelogEntry{
				Version: cleanVersion(m[1]),
				Changes: map[string][]string{
					"added":      {},
					"changed":    {},
					"fixed":      {},
					"deprecated": {},
					"removed":    {},
					"security":   {},
				},
			}
			if len(m) > 2 {
				current.Date = m[2]
			}
			category = ""
			continue
		}
		if current == nil {
			continue
		}
		if strings.HasPrefix(line, "### ") {
			category = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "### ")))
			if _, ok := current.Changes[category]; !ok {
				current.Changes[category] = []string{}
			}
			continue
		}
		if category == "" || !strings.HasPrefix(strings.TrimSpace(line), "- ") {
			continue
		}
		item := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- "))
		if item != "" {
			current.Changes[category] = append(current.Changes[category], item)
		}
	}
	if current != nil {
		entries = append(entries, *current)
	}
	for i := range entries {
		for _, cat := range []string{"added", "changed", "fixed", "deprecated", "removed", "security"} {
			if entries[i].Changes[cat] == nil {
				entries[i].Changes[cat] = []string{}
			}
		}
	}
	return entries
}

func filterChangelogSince(entries []changelogEntry, since string) []changelogEntry {
	since = cleanVersion(since)
	out := make([]changelogEntry, 0, len(entries))
	for _, entry := range entries {
		if strings.EqualFold(entry.Version, "unreleased") {
			continue
		}
		if semverGreater(entry.Version, since) {
			out = append(out, entry)
		}
	}
	return out
}

func semverGreater(a, b string) bool {
	return compareVersions(cleanVersion(a), cleanVersion(b)) > 0
}

func cleanVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	return v
}

func printChangelogText(cmd *cobra.Command, result changelogResult) {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "gitlab-cli changelog (current %s)\n\n", result.CurrentVersion)
	for _, entry := range result.Entries {
		if entry.Date != "" {
			_, _ = fmt.Fprintf(out, "## %s - %s\n\n", entry.Version, entry.Date)
		} else {
			_, _ = fmt.Fprintf(out, "## %s\n\n", entry.Version)
		}
		for _, cat := range []string{"added", "changed", "fixed", "deprecated", "removed", "security"} {
			items := entry.Changes[cat]
			if len(items) == 0 {
				continue
			}
			_, _ = fmt.Fprintf(out, "### %s\n", strings.Title(cat))
			for _, item := range items {
				_, _ = fmt.Fprintf(out, "- %s\n", item)
			}
			_, _ = fmt.Fprintln(out)
		}
	}
}
