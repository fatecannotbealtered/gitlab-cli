//go:build integration

package e2e_test

import (
	"strings"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/e2e"
)

func TestAllCommands_EveryLeaf(t *testing.T) {
	e2e.RequireEnv(t)
	if sharedFX == nil {
		t.Fatal("bootstrap fixture missing")
	}
	leaves, err := e2e.LeafPaths()
	if err != nil {
		t.Fatalf("reference: %v", err)
	}
	cases := e2e.BuildCommandCases(sharedFX)
	byPath := make(map[string]e2e.CommandCase, len(cases))
	for _, c := range cases {
		byPath[e2e.NormalizeLeafPath(c.Path)] = c
	}
	for _, leaf := range leaves {
		norm := e2e.NormalizeLeafPath(leaf)
		if _, ok := byPath[norm]; !ok {
			t.Errorf("integration table missing leaf command %q (normalized %q)", leaf, norm)
		}
	}
	if len(cases) != len(leaves) {
		t.Fatalf("integration cases=%d reference leaves=%d", len(cases), len(leaves))
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.Path, func(t *testing.T) {
			if tc.SkipIf != nil {
				if msg := tc.SkipIf(sharedFX); msg != "" {
					t.Skip(msg)
				}
			}
			var opt e2e.RunOptions
			if tc.WorkDir != nil {
				opt.WorkDir = tc.WorkDir(sharedFX)
			}
			if tc.ExtraEnv != nil {
				opt.ExtraEnv = tc.ExtraEnv(sharedFX)
			}
			args := tc.Args(sharedFX)
			stdout, stderr, code := e2e.RunCLI(t, args, opt)
			want := tc.WantExit
			if want == 0 && code != 0 {
				t.Fatalf("exit=%d\nargs: %s\nstdout:\n%s\nstderr:\n%s", code, strings.Join(args, " "), stdout, stderr)
			}
			if want != 0 && code != want {
				t.Fatalf("exit=%d want=%d\nargs: %s\nstdout:\n%s\nstderr:\n%s", code, want, strings.Join(args, " "), stdout, stderr)
			}
		})
	}
}
