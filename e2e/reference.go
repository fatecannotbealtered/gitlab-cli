//go:build integration

package e2e

import (
	"encoding/json"
	"os/exec"
	"strings"
)

type refCommand struct {
	Name     string       `json:"name"`
	Commands []refCommand `json:"commands,omitempty"`
}

type refTree struct {
	Commands []refCommand `json:"commands"`
}

// LeafPaths returns all leaf command paths (e.g. "issue list") from reference --json.
func LeafPaths() ([]string, error) {
	cmd := exec.Command("go", "run", "./cmd/gitlab-cli", "reference", "--json")
	cmd.Dir = RepoRoot()
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var tree refTree
	if err := json.Unmarshal(out, &tree); err != nil {
		return nil, err
	}
	var leaves []string
	var walk func(refCommand)
	walk = func(c refCommand) {
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
