package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func init() {
	addConformanceSamples(map[string][]string{
		"mr comment add":    {"5", "--project", "acme/web", "--body", "Looks good to me."},
		"mr comment delete": {"5", "--project", "acme/web", "--note-id", "9001"},
	})
}

func TestMRCommentMatrix(t *testing.T) {
	cases := []matrixCase{
		{Path: "mr comment list", Args: []string{"5", "--project", "acme/web"}, EmptyArgs: []string{"7", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--mr"},
				{[]string{"5", "--project", "acme/web", "--limit", "0"}, "E_USAGE", "invalid_value", "--limit"},
			}},
		{Path: "mr comment add", Args: []string{"5", "--project", "acme/web", "--body", "Looks good to me."}, Role: "guest",
			RequestFields: map[string]string{"body": "body"},
			Invalid: []invalidCase{
				{[]string{"5", "--project", "acme/web"}, "E_USAGE", "missing_parameter", "--body"},
				{[]string{"5", "--project", "acme/web", "--body", " \n"}, "E_VALIDATION", "invalid_value", "--body"},
				{[]string{"--project", "acme/web", "--body", "x"}, "E_USAGE", "missing_parameter", "--mr"},
				{[]string{"5", "--project", "acme/web", "--body", "x", "--body-file", "b.md"}, "E_USAGE", "conflicting_flags", "--body-file"},
			}},
		{Path: "mr comment delete", Args: []string{"5", "--project", "acme/web", "--note-id", "9001"}, Role: "maintainer",
			Invalid: []invalidCase{
				{[]string{"5", "--project", "acme/web", "--dangerous"}, "E_USAGE", "missing_parameter", "--note-id"},
				{[]string{"5", "--project", "acme/web", "--note-id", "x", "--dangerous"}, "E_VALIDATION", "invalid_value", "--note-id"},
				{[]string{"acme/web!5/notes/9001", "--note-id", "9002", "--dangerous"}, "E_USAGE", "conflicting_flags", "--note-id"},
			}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// TestMRCommentListLeavesOutSystemNotes: comments people wrote, oldest first, with
// their refs; GitLab's system notes are left out and scope says so (CLI-041).
func TestMRCommentListLeavesOutSystemNotes(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("mr", "comment", "list", "acme/web!5")
	if r.exit != 0 || len(r.items()) != 4 || r.items()[0]["ref"] != "acme/web!5/notes/9001" || r.items()[0]["author"] != "@carol" ||
		r.data()["sort"] != "created_at_asc" {
		t.Fatalf("comments: %s", r.out)
	}
	r = e.run("mr", "notes", "ls", "6", "-p", "acme/web") // the fleet's guesses (CLI-022)
	scope, _ := r.data()["scope"].(map[string]any)
	if r.exit != 0 || len(r.items()) != 1 || r.items()[0]["body"] != "Is this still flaky?" || r.data()["total"] != nil ||
		!strings.Contains(scope["covers"].(string), "system notes") {
		t.Fatalf("system notes left out: %s", r.out)
	}
	if !hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "data.items[].body") {
		t.Fatalf("bodies are untrusted: %s", r.out)
	}
}

// TestMRCommentAdd: the body from stdin binds like the literal (CLI-054), the new
// comment's ref is observed, and a quick action is blocked.
func TestMRCommentAdd(t *testing.T) {
	e := newTestEnv(t)
	e.stdin = "Please add a test.\n"
	tok := e.dryRun("mr", "comment", "add", "acme/web!5", "--body-file", "-")
	r := e.confirm(tok, "mr", "comment", "add", "acme/web!5", "--body-file", "-")
	res, _ := r.data()["result"].(map[string]any)
	ref, _ := res["ref"].(string)
	if r.exit != 0 || !strings.HasPrefix(ref, "acme/web!5/notes/") || res["body"] != "Please add a test." ||
		r.receipt()["observed"].(map[string]any)["ref"] != ref || r.outcome()["verification"] != "passed" {
		t.Fatalf("add: %s", r.out)
	}
	if req, _ := e.gl.lastWrite(); req.Body["body"] != "Please add a test." {
		t.Fatalf("request: %+v", req.Body)
	}
	e.stdin = ""
	r = e.run("mr", "comment", "add", "acme/web!5", "--body", "LGTM\n/merge", "--dry-run")
	if r.exit != 0 || r.token() != "" || r.preview()["blocked_by"].([]any)[0].(map[string]any)["kind"] != "quick_action" {
		t.Fatalf("a quick action: %s", r.out)
	}
	r = e.run("mr", "comment", "add", "acme/web!55", "--body", "x", "--dry-run")
	if r.code() != "E_NOT_FOUND" {
		t.Fatalf("a missing merge request: %s", r.out)
	}
}

// TestMRCommentDeleteRefsAndReadBack: the comment's ref, its URL and the merge
// request with --note-id are one request; a missing comment is E_NOT_FOUND at the
// dry-run (CLI-029); a system note is blocked; the delete is read back.
func TestMRCommentDeleteRefsAndReadBack(t *testing.T) {
	e := newTestEnv(t)
	if r := e.run("mr", "comment", "delete", "acme/web!5/notes/42", "--dangerous", "--dry-run"); r.code() != "E_NOT_FOUND" || r.exit != 3 {
		t.Fatalf("a missing comment: %s", r.out)
	}
	if r := e.run("mr", "comment", "delete", "acme/web!6/notes/9005", "--dangerous", "--dry-run"); r.token() != "" ||
		r.preview()["blocked_by"].([]any)[0].(map[string]any)["kind"] != "system_note" {
		t.Fatalf("a system note: %s", r.out)
	}
	dry := e.run("mr", "comment", "rm", "https://gitlab.example.com/acme/web/-/merge_requests/5#note_9001", "--dangerous", "--dry-run")
	if dry.preview()["target"].(map[string]any)["key"] != "acme/web!5/notes/9001" ||
		!strings.Contains(dry.out, `"before":"Looks good overall."`) {
		t.Fatalf("preview: %s", dry.out)
	}
	r := e.confirm(dry.token(), "mr", "comment", "delete", "5", "--project", "acme/web", "--note-id", "9001", "--dangerous")
	if r.exit != 0 || r.receipt()["observed"].(map[string]any)["exists"] != false || r.data()["result"].(map[string]any)["deleted"] != true {
		t.Fatalf("delete: %s", r.out)
	}
	if after := e.run("mr", "comment", "list", "acme/web!5"); len(after.items()) != 3 || after.data()["count"] != json.Number("3") {
		t.Fatalf("after the delete: %s", after.out)
	}
}
