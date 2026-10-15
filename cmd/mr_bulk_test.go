package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func init() {
	addConformanceSamples(map[string][]string{
		"mr bulk merge":   {"--project", "acme/web", "--ids", "5,6"},
		"mr bulk approve": {"--project", "acme/web", "--ids", "5,8"},
		"mr bulk close":   {"--project", "acme/web", "--ids", "5,6"},
		"mr bulk update":  {"--project", "acme/web", "--ids", "5,6", "--add-labels", "needs-review"},
	})
}

func TestMRBulkMatrix(t *testing.T) {
	// The batch rows of the shared matrix check every item's receipt and the
	// summary; bulk merge is dangerous, so it stops at its first failure (CLI-063).
	cases := []matrixCase{
		{Path: "mr bulk merge", Args: []string{"--project", "acme/web", "--ids", "5,6", "--squash"}, Role: "developer", PreviewReads: 2,
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web", "--dangerous"}, "E_USAGE", "missing_parameter", "--ids"},
				{[]string{"--ids", "5", "--dangerous"}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"--project", "acme/web", "--ids", "5,x", "--dangerous"}, "E_VALIDATION", "invalid_value", "--ids"},
				{[]string{"--project", "acme/web", "--ids", "acme/api!5", "--dangerous"}, "E_USAGE", "conflicting_flags", "--ids"},
			}},
		{Path: "mr bulk approve", Args: []string{"--project", "acme/web", "--ids", "5,8"}, Role: "developer", PreviewReads: 6,
			Invalid: []invalidCase{{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--ids"}}},
		{Path: "mr bulk close", Args: []string{"--project", "acme/web", "--ids", "5,6"}, Role: "developer", PreviewReads: 2,
			Invalid: []invalidCase{{[]string{"--project", "acme/web", "--ids", ","}, "E_USAGE", "missing_parameter", "--ids"}}},
		{Path: "mr bulk update", Args: []string{"--project", "acme/web", "--ids", "5,6", "--title", "Q4 cleanup", "--add-labels", "q4"},
			Role: "developer", PreviewReads: 2, RequestFields: map[string]string{"title": "title"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web", "--ids", "5"}, "E_USAGE", "missing_parameter", "--title"},
				{[]string{"--project", "acme/web", "--ids", "5", "--assignee", "carol"}, "E_USAGE", "unknown_flag", "--assignee"},
			}},
	}
	for _, tc := range cases {
		t.Run(tc.Path, func(t *testing.T) { runMatrix(t, tc) })
	}
}

// TestMRBulkPreviewListsEveryTarget: one preview for all merge requests, repeats
// dropped, refs and iids bound alike; one item that cannot apply (closed, missing)
// blocks the whole batch, named in blocked_by (CLI-063).
func TestMRBulkPreviewListsEveryTarget(t *testing.T) {
	e := newTestEnv(t)
	dry := e.run("mr", "bulk", "close", "--project", "acme/web", "--ids", "5,!5,acme/web!6", "--dry-run")
	targets, _ := dry.preview()["targets"].([]any)
	if dry.exit != 0 || len(targets) != 2 || dry.preview()["total"] != json.Number("2") ||
		dry.preview()["action"] != "Close the merge request (2 targets)" {
		t.Fatalf("preview: %s", dry.out)
	}
	if r := e.confirm(dry.token(), "mr", "bulk", "close", "--project", "acme/web", "--ids", "5,6"); r.exit != 0 {
		t.Fatalf("5,!5,acme/web!6 and 5,6 are one request: %s", r.out)
	}
	r := e.run("mr", "bulk", "close", "--project", "acme/web", "--ids", "8,7,55", "--dry-run")
	blocked, _ := r.preview()["blocked_by"].([]any)
	if r.exit != 0 || r.token() != "" || len(blocked) != 2 ||
		!strings.HasPrefix(blocked[0].(map[string]any)["message"].(string), "acme/web!7: ") ||
		blocked[1].(map[string]any)["kind"] != "not_found" {
		t.Fatalf("blocked items: %s", r.out)
	}
}

// TestMRBulkMergeStopsAtTheFirstFailure: a dangerous batch stops after a failed
// item unless --continue-on-error; nothing is rolled back (CLI-063).
func TestMRBulkMergeStopsAtTheFirstFailure(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"mr", "bulk", "merge", "--project", "acme/web", "--ids", "5,6", "--dangerous"}
	tok := e.dryRun(args...)
	e.gl.fail(fakeFault{Method: http.MethodPut, Path: "merge_requests/5/merge", Status: 405, Times: 1})
	r := e.confirm(tok, args...)
	if s := r.summary(); r.exit == 0 || s["failed"] != json.Number("1") || s["skipped"] != json.Number("1") {
		t.Fatalf("stop after the first failure: %s", r.out)
	}
	if e.gl.Writes() != 1 {
		t.Fatalf("one write was sent, got %d", e.gl.Writes())
	}
	args = append(args, "--continue-on-error")
	tok = e.dryRun(args...)
	e.gl.fail(fakeFault{Method: http.MethodPut, Path: "merge_requests/5/merge", Status: 405, Times: 1})
	r = e.confirm(tok, args...)
	if s := r.summary(); r.code() != "E_PARTIAL" || s["succeeded"] != json.Number("1") || s["failed"] != json.Number("1") {
		t.Fatalf("--continue-on-error: %s", r.out)
	}
	var state string
	e.gl.locked(func() { state = fakeMRByIID(e.gl, 6).State })
	if state != "merged" {
		t.Fatalf("!6 merged, got %s", state)
	}
}

func TestMRBulkUpdateAndApprove(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"mr", "bulk", "update", "--project", "acme/web", "--ids", "5,6", "--add-labels", "q4", "--remove-labels", "bug"}
	r := e.confirm(e.dryRun(args...), args...)
	items := r.batchItems()
	if r.exit != 0 || len(items) != 2 || items[1]["result"].(map[string]any)["labels"].([]any)[0] != "q4" {
		t.Fatalf("bulk update: %s", r.out)
	}
	args = []string{"mr", "bulk", "approve", "--project", "acme/web", "--ids", "5,8"}
	r = e.confirm(e.dryRun(args...), args...)
	if r.exit != 0 || r.batchItems()[1]["result"].(map[string]any)["approved"] != true {
		t.Fatalf("bulk approve: %s", r.out)
	}
	// carol approved both: a second batch has nothing to do.
	if dry := e.run(append(args, "--dry-run")...); dry.token() != "" {
		t.Fatalf("already approved: %s", dry.out)
	}
}
