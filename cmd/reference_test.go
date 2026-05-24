package cmd

import (
	"encoding/json"
	"testing"
)

func TestReference_JSON_HasAgentMetadata(t *testing.T) {
	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = true
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"reference", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("reference: %v", err)
		}
	})
	var tree refTree
	if err := json.Unmarshal([]byte(out), &tree); err != nil {
		t.Fatalf("unmarshal: %v\noutput:\n%s", err, out)
	}
	if tree.Version == "" {
		t.Error("expected version in reference tree")
	}
	mrDelete := findRefCommandByPath(tree.Commands, "mr comment delete")
	if mrDelete == nil {
		t.Fatal("mr comment delete not found in reference tree")
	}
	if !mrDelete.Write {
		t.Error("mr comment delete should be write=true")
	}
	if !mrDelete.RequiresConfirmation {
		t.Error("mr comment delete should require confirmation")
	}
	if len(mrDelete.PositionalArgs) != 1 || mrDelete.PositionalArgs[0] != "iid" {
		t.Errorf("positionalArgs = %v, want [iid]", mrDelete.PositionalArgs)
	}
	mrDiff := findRefCommandByPath(tree.Commands, "mr diff")
	if mrDiff == nil {
		t.Fatal("mr diff not found")
	}
	if mrDiff.OutputType != "text" {
		t.Errorf("mr diff outputType = %q, want text", mrDiff.OutputType)
	}
}

func findRefCommandByPath(nodes []refCommand, path string) *refCommand {
	for i := range nodes {
		n := &nodes[i]
		if n.Path == path {
			return n
		}
		if found := findRefCommandByPath(n.Commands, path); found != nil {
			return found
		}
	}
	return nil
}
