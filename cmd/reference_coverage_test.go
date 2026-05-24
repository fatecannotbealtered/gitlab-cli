package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestUnit_EveryLeafCommandHasTest ensures each CLI leaf command is exercised in cmd/*_test.go.
func TestUnit_EveryLeafCommandHasTest(t *testing.T) {
	leaves, err := leafPathsFromReference()
	if err != nil {
		t.Fatalf("reference: %v", err)
	}
	sources, err := readCmdTestSources(t)
	if err != nil {
		t.Fatalf("read tests: %v", err)
	}
	for _, leaf := range leaves {
		if !leafCoveredInTests(leaf, sources) {
			t.Errorf("no unit test invokes leaf command %q (rootCmd.SetArgs)", leaf)
		}
	}
}

type refJSONCommand struct {
	Name     string           `json:"name"`
	Commands []refJSONCommand `json:"commands,omitempty"`
}

type refJSONTree struct {
	Commands []refJSONCommand `json:"commands"`
}

func leafPathsFromReference() ([]string, error) {
	repo := moduleRoot()
	cmd := exec.Command("go", "run", "./cmd/gitlab-cli", "reference", "--json")
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var tree refJSONTree
	if err := json.Unmarshal(out, &tree); err != nil {
		return nil, err
	}
	var leaves []string
	var walk func(refJSONCommand)
	walk = func(c refJSONCommand) {
		if len(c.Commands) == 0 {
			leaves = append(leaves, strings.TrimPrefix(strings.TrimSpace(c.Name), "gitlab-cli "))
			return
		}
		for _, sub := range c.Commands {
			walk(sub)
		}
	}
	for _, c := range tree.Commands {
		walk(c)
	}
	return leaves, nil
}

func readCmdTestSources(t *testing.T) (string, error) {
	repo := moduleRoot()
	matches, err := filepath.Glob(filepath.Join(repo, "cmd", "*_test.go"))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, p := range matches {
		data, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func leafCoveredInTests(leaf, sources string) bool {
	parts := strings.Fields(leaf)
	if len(parts) == 0 {
		return false
	}
	// Match longest command prefix in SetArgs; positional placeholders (e.g. <iid>) are omitted in tests.
	for n := len(parts); n >= 1; n-- {
		needle := `"` + strings.Join(parts[:n], `", "`) + `"`
		if strings.Contains(sources, needle) {
			return true
		}
	}
	return false
}

func moduleRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}
