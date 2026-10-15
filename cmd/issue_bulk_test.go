package cmd

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The fake's issue endpoints are in issue_test.go: open issues 1, 2, 3, 5, 6 and
// closed 4, 7 in acme/web.

func init() {
	addConformanceSamples(map[string][]string{
		"issue bulk close":   {"--project", "acme/web", "--ids", "1,2"},
		"issue bulk reopen":  {"--project", "acme/web", "--ids", "4,7"},
		"issue bulk update":  {"--project", "acme/web", "--ids", "1,2", "--add-labels", "needs-triage"},
		"issue bulk label":   {"--project", "acme/web", "--ids", "2,6", "--add", "needs-triage"},
		"issue bulk assign":  {"erin", "--project", "acme/web", "--ids", "1,3"},
		"issue bulk comment": {"--project", "acme/web", "--ids", "1,2", "--body", "Triaged in the weekly sync."},
	})
}

func TestIssueBulkMatrix(t *testing.T) {
	cases := []matrixCase{
		{Path: "issue bulk close", Args: []string{"--project", "acme/web", "--ids", "1,2"}, Role: "reporter",
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--ids"},
				{[]string{"--project", "acme/web", "--ids", "1,x"}, "E_VALIDATION", "invalid_value", "--ids"},
				{[]string{"--ids", "acme/web#1,acme/other#2"}, "E_USAGE", "conflicting_flags", "--ids"},
				{[]string{"--ids", "1,2"}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"--project", "acme/other", "--ids", "acme/web#1"}, "E_USAGE", "conflicting_flags", "--project"},
			}},
		{Path: "issue bulk reopen", Args: []string{"--project", "acme/web", "--ids", "4,7"}, Role: "reporter",
			Invalid: []invalidCase{{[]string{"--project", "acme/web", "--ids", ","}, "E_USAGE", "missing_parameter", "--ids"}}},
		{Path: "issue bulk update", Args: []string{"--project", "acme/web", "--ids", "1,2", "--add-labels", "needs-triage",
			"--milestone", "v2.0"}, Role: "reporter", PreviewReads: 2,
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web", "--ids", "1"}, "E_USAGE", "missing_parameter", "--title"},
				{[]string{"--project", "acme/web", "--ids", "1", "--title", ""}, "E_VALIDATION", "invalid_value", "--title"},
			}},
		{Path: "issue bulk label", Args: []string{"--project", "acme/web", "--ids", "2,6", "--add", "needs-triage"}, Role: "reporter",
			Invalid: []invalidCase{{[]string{"--project", "acme/web", "--ids", "2"}, "E_USAGE", "missing_parameter", "--add"}}},
		{Path: "issue bulk assign", Args: []string{"erin", "--project", "acme/web", "--ids", "1,3"}, Role: "reporter", PreviewReads: 2,
			Invalid: []invalidCase{{[]string{"--project", "acme/web", "--ids", "1"}, "E_USAGE", "missing_parameter", "--assignee"}}},
		{Path: "issue bulk comment", Args: []string{"--project", "acme/web", "--ids", "1,2", "--body", "Triaged."}, Role: "guest",
			Invalid: []invalidCase{{[]string{"--project", "acme/web", "--ids", "1"}, "E_USAGE", "missing_parameter", "--body"}}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// TestIssueBulkLabelsTheTimeoutIssues is the eval's T06: list the open issues whose
// title mentions a timeout, label them in one batch, one token, every item verified,
// and no other issue touched.
func TestIssueBulkLabelsTheTimeoutIssues(t *testing.T) {
	e := newTestEnv(t)
	list := e.run("issue", "list", "--project", "acme/web", "--query", "timeout", "--in", "title", "--fields", "ref")
	var refs []string
	for _, it := range list.items() {
		refs = append(refs, it["ref"].(string))
	}
	args := []string{"issue", "bulk", "label", "--ids", strings.Join(refs, ","), "--add", "needs-triage"}
	dry := e.run(append(args, "--dry-run")...)
	p := dry.preview()
	if dry.exit != 0 || p["total"] != json.Number("2") || p["dangerous"] != false ||
		!strings.HasPrefix(p["action"].(string), "Label issues in acme/web") {
		t.Fatalf("preview: %s", dry.out)
	}
	r := e.confirm(dry.token(), args...)
	if r.exit != 0 || r.summary()["succeeded"] != json.Number("2") {
		t.Fatalf("label: %s", r.out)
	}
	for _, it := range r.batchItems() {
		res := it["result"].(map[string]any)
		if !hasString(stringsOf(res["labels"]), "needs-triage") || res["description"] != nil {
			t.Fatalf("item result: %v", it)
		}
	}
	e.gl.locked(func() {
		for _, iss := range fakeIssues(e.gl)["acme/web"] {
			if hasString(iss.Labels, "needs-triage") != (iss.IID == 2 || iss.IID == 6) {
				t.Errorf("#%d labels %v", iss.IID, iss.Labels)
			}
		}
	})
	if !hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "data.items[].result.title") {
		t.Fatalf("item titles are untrusted: %s", r.out)
	}
}

// TestIssueBulkBindsTheIssuesAsRefs: numbers with --project and refs are one batch
// (CLI-054); repeated issues count once (CLI-063); the items' issues are read in
// one request at the dry-run and one at the confirm.
func TestIssueBulkBindsTheIssuesAsRefs(t *testing.T) {
	e := newTestEnv(t)
	dry := e.run("issue", "bulk", "close", "--project", "acme/web", "--ids", "2,6", "--ids", "2", "--dry-run")
	if in := dry.preview()["inputs"].(map[string]any); strings.Join(stringsOf(in["ids"]), ",") != "2,6" ||
		in["project"] != "acme/web" || dry.preview()["total"] != json.Number("2") {
		t.Fatalf("inputs: %s", dry.out)
	}
	var keys []string
	for _, tg := range dry.preview()["targets"].([]any) {
		keys = append(keys, tg.(map[string]any)["key"].(string))
	}
	if strings.Join(keys, " ") != "acme/web#2 acme/web#6" {
		t.Fatalf("the targets are the refs: %v", keys)
	}
	if n := len(e.gl.Requests()); n != 1 || !strings.Contains(e.gl.Requests()[0].Query, "iids%5B%5D=2&iids%5B%5D=6") {
		t.Fatalf("one read for the batch, got %d: %+v", n, e.gl.Requests())
	}
	r := e.confirm(dry.token(), "issue", "bulk", "close", "--ids", "acme/web#2,acme/web#6")
	if r.exit != 0 || e.gl.Writes() != 2 || len(e.gl.Requests()) != 4 {
		t.Fatalf("confirm with refs: %s (requests %d)", r.out, len(e.gl.Requests()))
	}
}

// TestIssueBulkIsBlockedWhenAnItemCannotApply: an issue already closed, or one the
// project does not have, blocks the batch at the dry-run, naming the issue (CLI-063).
func TestIssueBulkIsBlockedWhenAnItemCannotApply(t *testing.T) {
	e := newTestEnv(t)
	for _, c := range []struct {
		args []string
		kind string
	}{
		{[]string{"issue", "bulk", "close", "--project", "acme/web", "--ids", "1,4", "--dry-run"}, "already_closed"},
		{[]string{"issue", "bulk", "label", "--project", "acme/web", "--ids", "1,99", "--add", "x", "--dry-run"}, "not_found"},
		{[]string{"issue", "bulk", "assign", "nobody", "--project", "acme/web", "--ids", "1", "--dry-run"}, "not_found"},
	} {
		r := e.run(c.args...)
		blocks, _ := r.preview()["blocked_by"].([]any)
		if r.exit != 0 || r.preview()["can_apply"] != false || r.token() != "" || len(blocks) == 0 ||
			blocks[0].(map[string]any)["kind"] != c.kind {
			t.Fatalf("%v: %s", c.args, r.out)
		}
	}
	if e.gl.Writes() != 0 {
		t.Fatal("a blocked batch sent a write")
	}
}

// TestIssueBulkStopsWhenAsked: --continue-on-error=false stops at the first
// failure and reports the rest as skipped (CLI-063).
func TestIssueBulkStopsWhenAsked(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"issue", "bulk", "comment", "--project", "acme/web", "--ids", "1,2,3", "--body", "Triaged.", "--continue-on-error=false"}
	tok := e.dryRun(args...)
	e.gl.fail(fakeFault{Method: "WRITE", Status: 403, Times: 1})
	r := e.confirm(tok, args...)
	s := r.summary()
	if r.code() != "E_FORBIDDEN" || s["failed"] != json.Number("1") || s["skipped"] != json.Number("2") || e.gl.Writes() != 1 {
		t.Fatalf("stop at the first failure: %s", r.out)
	}
	if r.details()["required"] != "guest" {
		t.Fatalf("a refused comment names the role: %s", r.out)
	}
}

// TestIssueBulkAssignAndUpdate: the assignee given as the argument or --assignee
// is one request, and a bulk update reports each item's milestone and labels.
func TestIssueBulkAssignAndUpdate(t *testing.T) {
	e := newTestEnv(t)
	tok := e.dryRun("issue", "bulk", "assign", "@erin", "--project", "acme/web", "--ids", "3,5")
	r := e.confirm(tok, "issue", "bulk", "assign", "--assignee", "erin", "--project", "acme/web", "--ids", "3,5")
	if r.exit != 0 {
		t.Fatalf("assign: %s", r.out)
	}
	for _, it := range r.batchItems() {
		if obs := it["receipt"].(map[string]any)["observed"].(map[string]any); strings.Join(stringsOf(obs["assignees"]), ",") != "@erin" {
			t.Fatalf("item: %v", it)
		}
	}
	args := []string{"issue", "bulk", "update", "--project", "acme/web", "--ids", "3,5", "--remove-labels", "feature", "--milestone", ""}
	r = e.confirm(e.dryRun(args...), args...)
	if r.exit != 0 {
		t.Fatalf("update: %s", r.out)
	}
	for _, it := range r.batchItems() {
		obs := it["receipt"].(map[string]any)["observed"].(map[string]any)
		if obs["milestone"] != nil || hasString(stringsOf(obs["labels"]), "feature") || itemOutcome(it)["verification"] != "passed" {
			t.Fatalf("item: %v", it)
		}
	}
	req, _ := e.gl.lastWrite()
	if req.Method != http.MethodPut || req.Body["remove_labels"] != "feature" || req.Body["milestone_id"] != float64(0) {
		t.Fatalf("request: %+v", req)
	}
}
