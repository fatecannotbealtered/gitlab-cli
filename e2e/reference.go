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

// refEnvelope matches the CLI output contract: the command tree lives under
// `data`, never at the top level (CLI-SPEC §4).
type refEnvelope struct {
	Data refTree `json:"data"`
}

// LeafPaths returns all leaf command paths (e.g. "issue list") from reference --json.
func LeafPaths() ([]string, error) {
	cmd := exec.Command("go", "run", "./cmd/gitlab-cli", "reference", "--json")
	cmd.Dir = RepoRoot()
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var env refEnvelope
	if err := json.Unmarshal(out, &env); err != nil {
		return nil, err
	}
	tree := env.Data
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
