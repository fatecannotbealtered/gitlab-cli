package output

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestPrintJSON_Compact(t *testing.T) {
	orig := Compact
	defer func() { Compact = orig }()

	Compact = false
	var indented bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	PrintJSON(map[string]any{"a": 1})
	w.Close()
	os.Stdout = old
	_, _ = indented.ReadFrom(r)
	if !strings.Contains(indented.String(), "\n") {
		t.Error("expected indented JSON to contain newlines")
	}

	Compact = true
	var compact bytes.Buffer
	r2, w2, _ := os.Pipe()
	os.Stdout = w2
	PrintJSON(map[string]any{"a": 1})
	w2.Close()
	os.Stdout = old
	_, _ = compact.ReadFrom(r2)
	out := strings.TrimSpace(compact.String())
	if strings.Contains(out, "\n") {
		t.Errorf("expected single-line compact JSON, got: %q", out)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}
