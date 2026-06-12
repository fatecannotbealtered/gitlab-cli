//go:build integration

package e2e_test

import (
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/e2e"
)

// TestLeafPathsParsesEnvelope guards the guard: LeafPaths must enumerate a
// plausible number of leaves from the envelope-wrapped reference output.
// Catches the parser silently reading the wrong JSON level (0 leaves).
func TestLeafPathsParsesEnvelope(t *testing.T) {
	leaves, err := e2e.LeafPaths()
	if err != nil {
		t.Fatalf("LeafPaths: %v", err)
	}
	if len(leaves) < 80 {
		t.Fatalf("LeafPaths returned %d leaves; expected the full command tree (>=80). Parser is reading the wrong envelope level.", len(leaves))
	}
	t.Logf("leaves=%d first=%q last=%q", len(leaves), leaves[0], leaves[len(leaves)-1])
}
