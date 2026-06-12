package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func runChangelog(t *testing.T, args ...string) string {
	t.Helper()
	jsonMode = true
	return captureStdout(t, func() {
		rootCmd.SetArgs(args)
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("changelog: %v", err)
		}
	})
}

func TestChangelog_JSON(t *testing.T) {
	out := runChangelog(t, "changelog", "--json")
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			CurrentVersion string `json:"current_version"`
			Entries        []struct {
				Version string `json:"version"`
			} `json:"entries"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid envelope: %v\n%s", err, out)
	}
	if !env.OK {
		t.Fatalf("ok=false: %s", out)
	}
	if len(env.Data.Entries) == 0 {
		t.Fatal("changelog should expose at least one version entry")
	}
}

func TestChangelog_Since(t *testing.T) {
	// --since with the newest released version returns only strictly newer entries.
	out := runChangelog(t, "changelog", "--json", "--since", "0.0.0")
	if !strings.Contains(out, `"entries"`) {
		t.Fatalf("expected entries field, got: %s", out)
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Since   string           `json:"since"`
			Entries []map[string]any `json:"entries"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("invalid envelope: %v", err)
	}
	if !env.OK {
		t.Fatalf("ok=false: %s", out)
	}
	if len(env.Data.Entries) == 0 {
		t.Fatal("--since 0.0.0 should include every released entry")
	}
}
