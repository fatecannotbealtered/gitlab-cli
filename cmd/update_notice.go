package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	rootchangelog "github.com/fatecannotbealtered/gitlab-cli"
	"github.com/spf13/cobra"
)

const (
	updateNoticeCacheTTL       = 24 * time.Hour
	updateNoticeRefreshTimeout = 2 * time.Second
	updateNoticeEnvOptOut      = "GITLAB_CLI_NO_UPDATE_CHECK"
)

type updateNotice struct {
	Type               string   `json:"type"`
	Severity           string   `json:"severity"`
	Message            string   `json:"message"`
	CurrentVersion     string   `json:"current_version"`
	LatestVersion      string   `json:"latest_version"`
	UpdateAvailable    bool     `json:"update_available"`
	InstallMethod      string   `json:"install_method,omitempty"`
	RecommendedCommand string   `json:"recommended_command"`
	ReleaseURL         string   `json:"release_url,omitempty"`
	CheckedAt          string   `json:"checked_at"`
	Source             string   `json:"source"`
	NextSteps          []string `json:"next_steps"`
}

type updateNoticeCache struct {
	CheckedAt string         `json:"checked_at"`
	Notices   []updateNotice `json:"notices,omitempty"`
}

func installUpdateNoticeHelp(root *cobra.Command) {
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		if cmd.Long != "" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cmd.Long)
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
		} else if cmd.Short != "" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cmd.Short)
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
		}
		_, _ = fmt.Fprint(cmd.OutOrStdout(), cmd.UsageString())
		printUpdateNoticeHint(cmd.OutOrStdout(), readCachedUpdateNotices())
	})
}

func refreshUpdateNotices(ctx context.Context, source string) []updateNotice {
	if updateNoticeAutoDisabled() {
		return nil
	}
	refreshCtx, cancel := context.WithTimeout(ctx, updateNoticeRefreshTimeout)
	defer cancel()

	release, err := fetchUpdateRelease(refreshCtx, "")
	if err != nil {
		return readCachedUpdateNotices()
	}
	plan, err := buildUpdatePlan(release, version)
	if err != nil {
		return readCachedUpdateNotices()
	}
	notices := updateNoticesFromPlan(plan, source)
	writeUpdateNoticeCache(notices)
	return notices
}

func updateNoticesFromPlan(plan updatePlan, source string) []updateNotice {
	current := normalizeVersion(plan.CurrentVersion)
	latest := normalizeVersion(plan.TargetVersion)
	if !plan.UpdateAvailable {
		return nil
	}
	notice := updateNotice{
		Type:               "update_available",
		Severity:           updateNoticeSeverity(current, latest),
		CurrentVersion:     current,
		LatestVersion:      latest,
		UpdateAvailable:    true,
		InstallMethod:      detectInstallMethod(),
		RecommendedCommand: "gitlab-cli update --dry-run --compact",
		ReleaseURL:         plan.ReleaseURL,
		CheckedAt:          time.Now().UTC().Format(time.RFC3339),
		Source:             source,
		NextSteps: []string{
			"run gitlab-cli update --dry-run --compact",
			"ask the user before confirming the local self-update",
			"after update, run gitlab-cli changelog --since " + current + " --compact",
			"refresh gitlab-cli reference --compact before using new behavior",
		},
	}
	notice.Message = fmt.Sprintf("gitlab-cli %s is available (current %s)", latest, current)
	return []updateNotice{notice}
}

// updateNoticeSeverity grades the update notice from the embedded CHANGELOG delta
// between the running version and the latest (CLI-SPEC §14): "warning" when the
// delta since current contains a security entry OR latest crosses a major version
// boundary; "info" otherwise.
func updateNoticeSeverity(current, latest string) string {
	return severityFromChangelog(rootchangelog.ChangelogMarkdown, current, latest)
}

// severityFromChangelog computes the notice severity from a CHANGELOG body, the
// running version, and the latest version, per the §14 rule. Split out from
// updateNoticeSeverity so the delta logic is testable with synthetic changelogs.
func severityFromChangelog(markdown, current, latest string) string {
	if majorBump(current, latest) || deltaHasSecurityEntry(markdown, current) {
		return "warning"
	}
	return "info"
}

// majorBump reports whether latest's major version is greater than current's.
func majorBump(current, latest string) bool {
	c, cOK := parseSemver(current)
	l, lOK := parseSemver(latest)
	if !cOK || !lOK {
		return false
	}
	return l.nums[0] > c.nums[0]
}

// deltaHasSecurityEntry reports whether any CHANGELOG version newer than current
// carries a non-empty `security` category.
func deltaHasSecurityEntry(markdown, current string) bool {
	entries := filterChangelogSince(parseChangelog(markdown), current)
	for _, entry := range entries {
		if len(entry.Changes["security"]) > 0 {
			return true
		}
	}
	return false
}

// noticesAsAny adapts the cached update notices to []any for the output layer's
// UpdateNoticesProvider hook (which is cmd-agnostic to avoid an import cycle).
// Returns nil when there is nothing to report so meta.notices is omitted.
func noticesAsAny(notices []updateNotice) []any {
	if len(notices) == 0 {
		return nil
	}
	out := make([]any, 0, len(notices))
	for _, n := range notices {
		out = append(out, n)
	}
	return out
}

func readCachedUpdateNotices() []updateNotice {
	if updateNoticeAutoDisabled() {
		return nil
	}
	path, err := updateNoticeCachePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cache updateNoticeCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}
	checkedAt, err := time.Parse(time.RFC3339, cache.CheckedAt)
	if err != nil || time.Since(checkedAt) > updateNoticeCacheTTL {
		return nil
	}
	notices := make([]updateNotice, 0, len(cache.Notices))
	for _, notice := range cache.Notices {
		if notice.Type != "update_available" || !notice.UpdateAvailable {
			continue
		}
		// Version-aware: suppress a stale "update available" notice once the
		// running binary is already at (or past) the cached latest version. This
		// also covers the package-manager update path, which upgrades via the
		// manager without clearing this cache (the new binary takes effect next run).
		if notice.LatestVersion != "" && compareVersions(notice.LatestVersion, version) <= 0 {
			continue
		}
		notice.Source = "cache"
		notices = append(notices, notice)
	}
	return notices
}

func writeUpdateNoticeCache(notices []updateNotice) {
	if updateNoticeAutoDisabled() {
		return
	}
	path, err := updateNoticeCachePath()
	if err != nil {
		return
	}
	if len(notices) == 0 {
		_ = os.Remove(path)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	cache := updateNoticeCache{CheckedAt: checkedAt, Notices: notices}
	for i := range cache.Notices {
		cache.Notices[i].CheckedAt = checkedAt
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}

func updateNoticeCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", err
	}
	return filepath.Join(home, "."+updateBinaryName, "update-check.json"), nil
}

func updateNoticeDisabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(updateNoticeEnvOptOut)))
	return value == "1" || value == "true" || value == "yes"
}

// updateNoticeTestForceEnabled lets the cache-path tests exercise the real
// read/write cache (otherwise auto-disabled under the .test binary). Production
// code never sets it.
var updateNoticeTestForceEnabled bool

func updateNoticeAutoDisabled() bool {
	if updateNoticeTestForceEnabled {
		return updateNoticeDisabled()
	}
	return updateNoticeDisabled() || strings.HasSuffix(os.Args[0], ".test")
}

func printUpdateNoticeHint(w io.Writer, notices []updateNotice) {
	if len(notices) == 0 {
		return
	}
	notice := notices[0]
	_, _ = fmt.Fprintf(w, "\nUpdate available: gitlab-cli %s -> %s. Run: %s\n", notice.CurrentVersion, notice.LatestVersion, notice.RecommendedCommand)
}
