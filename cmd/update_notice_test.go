package cmd

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

// failingRoundTripper fails the test if any HTTP request is attempted; the
// meta.notices piggyback path must read only the local cache (CLI-SPEC §14).
type failingRoundTripper struct{ t *testing.T }

func (f failingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	f.t.Fatalf("unexpected network call to %s; meta.notices must be cache-only", req.URL)
	return nil, nil
}

// TestMetaNotices_FromCacheNoNetwork asserts that when the local cache holds an
// available-update notice, it surfaces on an arbitrary (non-update) command's
// meta.notices, that the path makes no network call, and that an empty/expired
// cache omits meta.notices entirely.
func TestMetaNotices_FromCacheNoNetwork(t *testing.T) {
	resetRootPersistentFlags(t)
	isolateConfigHome(t)
	t.Setenv("GITLAB_NO_AUDIT", "1")
	t.Setenv(updateNoticeEnvOptOut, "")

	// Exercise the real read/write cache (otherwise auto-disabled under .test).
	prevForce := updateNoticeTestForceEnabled
	updateNoticeTestForceEnabled = true
	t.Cleanup(func() { updateNoticeTestForceEnabled = prevForce })

	// Any network attempt from the meta path fails the test.
	prevHTTP := updateHTTPClient
	updateHTTPClient = &http.Client{Transport: failingRoundTripper{t}}
	t.Cleanup(func() { updateHTTPClient = prevHTTP })

	// Empty cache -> meta.notices absent on an arbitrary command (changelog).
	out := runChangelog(t, "changelog", "--json", "--compact")
	if got := metaNoticesOf(t, out); got != nil {
		t.Fatalf("empty cache should omit meta.notices, got %v\n%s", got, out)
	}

	// Seed the cache with an available-update notice.
	writeUpdateNoticeCache([]updateNotice{{
		Type:            "update_available",
		Severity:        "warning",
		CurrentVersion:  "1.0.0",
		LatestVersion:   "2.0.0",
		UpdateAvailable: true,
	}})

	out = runChangelog(t, "changelog", "--json", "--compact")
	notices := metaNoticesOf(t, out)
	if len(notices) != 1 {
		t.Fatalf("expected 1 cached notice in meta, got %d\n%s", len(notices), out)
	}
	if notices[0]["severity"] != "warning" || notices[0]["type"] != "update_available" {
		t.Fatalf("cached notice carried wrong fields: %v", notices[0])
	}
}

func TestUpdateNoticeAutoDisabledDetectsWindowsGoTestBinary(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	origForce := updateNoticeTestForceEnabled
	updateNoticeTestForceEnabled = false
	t.Cleanup(func() { updateNoticeTestForceEnabled = origForce })
	os.Args = []string{`C:\Users\me\AppData\Local\Temp\cmd.test.exe`}
	t.Setenv(updateNoticeEnvOptOut, "")

	if !updateNoticeAutoDisabled() {
		t.Fatal("Windows Go test binary must not write the real update notice cache")
	}
}

// metaNoticesOf extracts data.meta.notices from a JSON envelope string.
func metaNoticesOf(t *testing.T, out string) []map[string]any {
	t.Helper()
	var env struct {
		Meta struct {
			Notices []map[string]any `json:"notices"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid envelope: %v\n%s", err, out)
	}
	return env.Meta.Notices
}

// TestUpdateNoticeSeverity_FromChangelogDelta covers the §14 severity rule:
// a security entry in the delta -> warning; a major bump -> warning; a plain
// patch with no security entry -> info.
func TestUpdateNoticeSeverity_FromChangelogDelta(t *testing.T) {
	changelog := `# Changelog

## [1.0.2] - 2026-06-22
### Security
- fix token leak

## [1.0.1] - 2026-06-21
### Fixed
- minor fix

## [1.0.0] - 2026-06-20
### Added
- initial
`
	cases := []struct {
		name            string
		current, latest string
		md              string
		want            string
	}{
		{"security entry -> warning", "1.0.1", "1.0.2", changelog, "warning"},
		{"major bump -> warning", "1.0.1", "2.0.0", "# Changelog\n", "warning"},
		{"plain patch -> info", "1.0.0", "1.0.1", "# Changelog\n\n## [1.0.1] - 2026-06-21\n### Fixed\n- minor fix\n", "info"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := severityFromChangelog(tc.md, tc.current, tc.latest); got != tc.want {
				t.Fatalf("severityFromChangelog(%s->%s) = %q, want %q", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}

// TestUpdateNoticesFromPlan_SeverityGraded asserts updateNoticesFromPlan stores
// the computed severity (not a hardcoded "info") in the cache view, and that
// noticesAsAny adapts the result for the output hook.
func TestUpdateNoticesFromPlan_SeverityGraded(t *testing.T) {
	notices := updateNoticesFromPlan(updatePlan{
		CurrentVersion:  "1.0.0",
		TargetVersion:   "2.0.0",
		UpdateAvailable: true,
	}, "test")
	if len(notices) != 1 {
		t.Fatalf("expected 1 notice, got %d", len(notices))
	}
	if notices[0].Severity != "warning" {
		t.Fatalf("major bump should grade warning, got %q", notices[0].Severity)
	}
	if got := noticesAsAny(notices); len(got) != 1 {
		t.Fatalf("noticesAsAny should yield 1 entry, got %d", len(got))
	}
	if got := noticesAsAny(nil); got != nil {
		t.Fatalf("noticesAsAny(nil) should be nil, got %v", got)
	}
}
