package cmd

import (
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
)

// The conformance suite (VER-001): G00-G10 on every write command, R01-R05 once.
// Each group registers its samples from an init function in its own _test.go file:
//
//	func init() {
//		addConformanceSamples(map[string][]string{
//			"label update": {"bug", "--project", "acme/web", "--color", "#ee0701"},
//		})
//	}
//
// A sample is one valid invocation after the command path, without gate flags
// (--dry-run, --confirm, --dangerous), that the fake GitLab's seed data accepts.

var (
	conformanceSamples = map[string][]string{}
	// conformanceResets restore state a sample needs outside the fake GitLab (the
	// keyring entry `auth logout` removes); Harness.Reset runs them after the fake's.
	conformanceResets []func(gl *fakeGitLab)
	// conformanceDestinations are the destinations deliver samples send to (SEC-012).
	conformanceDestinations []string
)

// addConformanceSamples registers write samples (Harness.Samples).
func addConformanceSamples(samples map[string][]string) {
	for path, args := range samples {
		conformanceSamples[path] = args
	}
}

// addConformanceReset registers a function Harness.Reset runs.
func addConformanceReset(fn func(gl *fakeGitLab)) { conformanceResets = append(conformanceResets, fn) }

// conformanceHarness connects the suite to gitlab-cli and the fake GitLab. gl is nil
// for CheckCoverage, which only builds Apps.
func conformanceHarness(gl *fakeGitLab) aicli.Harness {
	host := "http://127.0.0.1:1"
	if gl != nil {
		host = gl.URL
	}
	// outputs grants new files only, roots existing ones (SEC-011, C1): the suite runs
	// each sample several times, so an artifact sample's second run overwrites the
	// file its first run wrote, and the directory is in both.
	scope := map[string]any{"scope_id": "scp_conformance", "tool": "gitlab-cli", "granted_by": "gitlab-cli conformance",
		"granted_at": "2026-10-12T00:00:00Z", "expires_at": nil,
		"effects": []string{"artifact", "change", "configure", "deliver", "persist"}, "dangerous": true,
		"outputs": []string{conformanceOutputDir}, "roots": []string{conformanceOutputDir}}
	if len(conformanceDestinations) > 0 {
		scope["destinations"] = conformanceDestinations
	}
	return aicli.Harness{
		NewApp: func(env map[string]string) *aicli.App {
			vars := baseVars(host, "")
			delete(vars, "GITLAB_CLI_STATE_DIR")
			for k, v := range env {
				vars[k] = v
			}
			return newTestApp(vars)
		},
		// A write reaches GitLab, or the keyring for auth login and logout, which
		// write no upstream. G04 accepts their receipts anyway (P7); counting the
		// keyring keeps "a refusal writes nothing" checked for them.
		Writes: func() int {
			n := testKeyring.Writes()
			if gl != nil {
				n += gl.Writes()
			}
			return n
		},
		Reset: func() {
			testKeyring.reset()
			if gl != nil {
				gl.Reset()
			}
			for _, fn := range conformanceResets {
				fn(gl)
			}
		},
		Samples: conformanceSamples,
		Scope:   scope,
	}
}

func TestConformance(t *testing.T) {
	testKeyring.reset()
	aicli.RunConformance(t, conformanceHarness(newFakeGitLab(t)))
}
