package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fake's issue and comment endpoints are in issue_test.go. Issue #1 has two
// comments (8101 by carol, 8102 by dave), #2 a system note (8201) and erin's 8202,
// whose text carries an injected instruction; #3 has none.

func init() {
	addConformanceSamples(map[string][]string{
		"issue comment create": {"1", "--project", "acme/web", "--body", "Reproduced on staging."},
		"issue comment delete": {"1", "--project", "acme/web", "--note-id", "8102"},
	})
}

func TestIssueCommentMatrix(t *testing.T) {
	cases := []matrixCase{
		{Path: "issue comment list", Args: []string{"1", "--project", "acme/web"}, EmptyArgs: []string{"3", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--issue"},
				{[]string{"acme/web#1/notes/x"}, "E_VALIDATION", "invalid_value", "--issue"},
				{[]string{"1", "--project", "acme/web", "--limit", "0"}, "E_USAGE", "invalid_value", "--limit"},
			}},
		{Path: "issue comment create", Args: []string{"1", "--project", "acme/web", "--body", "Reproduced on staging."},
			Role: "guest", RequestFields: map[string]string{"body": "body"},
			Invalid: []invalidCase{
				{[]string{"1", "--project", "acme/web"}, "E_USAGE", "missing_parameter", "--body"},
				{[]string{"1", "--project", "acme/web", "--body", " "}, "E_USAGE", "missing_parameter", "--body"},
				{[]string{"1", "--project", "acme/web", "--body", "x", "--body-file", "x.md"}, "E_USAGE", "conflicting_flags", "--body-file"},
				{[]string{"--project", "acme/web", "--body", "x"}, "E_USAGE", "missing_parameter", "--issue"},
			}},
		{Path: "issue comment delete", Args: []string{"1", "--project", "acme/web", "--note-id", "8102"}, Role: "maintainer",
			Invalid: []invalidCase{
				{[]string{"1", "--project", "acme/web"}, "E_USAGE", "missing_parameter", "--note-id"},
				{[]string{"acme/web#1/notes/8101", "--note-id", "8102"}, "E_USAGE", "conflicting_flags", "--note-id"},
				{[]string{"1", "--project", "acme/web", "--note-id", "-3"}, "E_VALIDATION", "invalid_value", "--note-id"},
				{[]string{"https://h/acme/web/-/issues/1#note_x"}, "E_VALIDATION", "invalid_value", "--issue"},
			}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// TestIssueCommentFormsAreOneTarget: a comment's ref, its web URL, and the issue
// with --note-id name one comment and bind as one request (CLI-048, CLI-054).
func TestIssueCommentFormsAreOneTarget(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("issue", "comment", "list", "acme/web#1")
	if r.exit != 0 || len(r.items()) != 2 || r.items()[0]["ref"] != "acme/web#1/notes/8102" || r.items()[0]["author"] != "@dave" {
		t.Fatalf("list: %s", r.out)
	}
	for _, form := range [][]string{
		{"acme/web#1/notes/8102"},
		{e.gl.URL + "/acme/web/-/issues/1#note_8102"},
		{"1", "--project", "acme/web", "--note-id", "8102"},
		{"acme/web#1", "--note-id", "8102"},
	} {
		dry := e.run(append(append([]string{"issue", "comment", "delete"}, form...), "--dangerous", "--dry-run")...)
		in, _ := dry.preview()["inputs"].(map[string]any)
		if dry.exit != 0 || in["issue"] != "1" || in["project"] != "acme/web" || in["note_id"] != "8102" ||
			dry.preview()["target"].(map[string]any)["key"] != "acme/web#1/notes/8102" {
			t.Fatalf("%v: %s", form, dry.out)
		}
	}
	tok := e.dryRun("issue", "comment", "delete", "acme/web#1/notes/8102", "--dangerous")
	r = e.confirm(tok, "issue", "comment", "delete", "1", "--project", "acme/web", "--note-id", "8102", "--dangerous")
	if r.exit != 0 || r.receipt()["observed"].(map[string]any)["exists"] != false || r.outcome()["verification"] != "passed" {
		t.Fatalf("the ref and --note-id must bind alike: %s", r.out)
	}
	if res := r.data()["result"].(map[string]any); res["ref"] != "acme/web#1/notes/8102" || res["deleted"] != true {
		t.Fatalf("result: %v", res)
	}
}

// TestIssueCommentListLeavesOutSystemNotes: system notes are not comments; the
// page says so in scope and reports no total, since GitLab's counts them (CLI-041).
// Comment bodies are untrusted: the injected instruction is data (CLI-046, SEC-002).
func TestIssueCommentListLeavesOutSystemNotes(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("issue", "comment", "list", "2", "--project", "acme/web")
	if r.exit != 0 || len(r.items()) != 1 || r.items()[0]["ref"] != "acme/web#2/notes/8202" || r.data()["total"] != nil {
		t.Fatalf("system notes left out: %s", r.out)
	}
	if !strings.Contains(r.data()["scope"].(map[string]any)["covers"].(string), "system notes") || r.data()["sort"] != "created_at_desc" {
		t.Fatalf("scope and sort: %s", r.out)
	}
	if !hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "data.items[].body") {
		t.Fatalf("bodies are untrusted: %s", r.out)
	}
	if e.gl.Writes() != 0 {
		t.Fatal("reading an instruction must not act on it")
	}
	if r := e.run("issue", "comment", "list", "acme/web#99"); r.code() != "E_NOT_FOUND" {
		t.Fatalf("a missing issue: %s", r.out)
	}
}

// TestIssueCommentBodyFileBindsItsContent: --body-file and the same text given
// with --body are one request (CLI-054); a file's trailing line break is not sent.
func TestIssueCommentBodyFileBindsItsContent(t *testing.T) {
	e := newTestEnv(t)
	path := filepath.Join(e.dir, "comment.md")
	if err := os.WriteFile(path, []byte("Fixed in !5."), 0o600); err != nil {
		t.Fatal(err)
	}
	tok := e.dryRun("issue", "comment", "create", "acme/web#3", "--body", "Fixed in !5.")
	r := e.confirm(tok, "issue", "comment", "create", "acme/web#3", "--body-file", path)
	res, _ := r.data()["result"].(map[string]any)
	if r.exit != 0 || res["body"] != "Fixed in !5." || res["issue"] != "acme/web#3" || res["author"] != "@carol" {
		t.Fatalf("a comment from a file: %s", r.out)
	}
	if obs := r.receipt()["observed"].(map[string]any); obs["ref"] != res["ref"] || r.outcome()["verification"] != "passed" {
		t.Fatalf("the new comment's ref is observed (CLI-059): %s", r.out)
	}
	if err := os.WriteFile(path, []byte("Fixed in !6.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.stdin = "Fixed in !6.\n"
	args := []string{"issue", "comment", "create", "acme/web#3", "--body-file", "-"}
	r = e.confirm(e.dryRun(args...), args...)
	if r.exit != 0 || r.data()["result"].(map[string]any)["body"] != "Fixed in !6." {
		t.Fatalf("a comment from stdin: %s", r.out)
	}
}

// TestIssueCommentLostReply: GitLab's reply to the comment is lost. The outcome
// stays unknown, the issue is blocked until the agent looks it up and resolves the
// operation, and nothing is sent twice (CLI-015, CLI-060; the eval's T13).
func TestIssueCommentLostReply(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"issue", "comment", "create", "acme/web#3", "--body", "Drafting the design now."}
	tok := e.dryRun(args...)
	e.gl.fail(fakeFault{Method: "WRITE", Reset: true, Times: 1})
	r := e.confirm(tok, args...)
	id, _ := r.details()["operation_id"].(string)
	if r.code() != "E_OUTCOME_UNKNOWN" || id == "" || !strings.Contains(r.details()["next_action"].(string), "issue comment list acme/web#3") {
		t.Fatalf("a lost reply: %s", r.out)
	}
	if again := e.run(append(args, "--dry-run")...); again.details()["conflict"] != "unresolved_operation" {
		t.Fatalf("the issue is blocked until the outcome is resolved: %s", again.out)
	}
	if list := e.run("issue", "comment", "list", "acme/web#3"); len(list.items()) != 0 {
		t.Fatalf("the fake dropped the request: %s", list.out)
	}
	if res := e.run("operation", "resolve", "--id", id, "--outcome", "not_applied", "--evidence", "issue comment list shows no comment"); res.exit != 0 {
		t.Fatalf("resolve: %s", res.out)
	}
	r = e.confirm(e.dryRun(args...), args...)
	if r.exit != 0 || e.gl.Writes() != 2 {
		t.Fatalf("after the lookup the comment is sent once more: %s (writes %d)", r.out, e.gl.Writes())
	}
}

func TestIssueCommentDeleteChecksTheComment(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("issue", "comment", "delete", "acme/web#1/notes/8101", "--dry-run")
	if r.code() != "E_CONFIRMATION_REQUIRED" || r.details()["problem"] != "dangerous_flag_required" {
		t.Fatalf("delete needs --dangerous (CLI-056): %s", r.out)
	}
	r = e.run("issue", "comment", "delete", "acme/web#2/notes/8201", "--dangerous", "--dry-run")
	if r.exit != 0 || r.preview()["can_apply"] != false || r.token() != "" {
		t.Fatalf("a system note cannot be deleted: %s", r.out)
	}
	r = e.run("issue", "comment", "delete", "acme/web#1/notes/999", "--dangerous", "--dry-run")
	if r.code() != "E_NOT_FOUND" || !strings.Contains(r.details()["next_action"].(string), "issue comment list acme/web#1") {
		t.Fatalf("a missing comment is E_NOT_FOUND at the dry-run (CLI-029): %s", r.out)
	}
	r = e.run("issue", "comment", "delete", "acme/web#1/notes/8101", "--dangerous", "--dry-run")
	ch := r.preview()["changes"].([]any)[0].(map[string]any)
	if ch["before"] != "Reproduced on Safari 17." || !strings.Contains(ch["note"].(string), "@carol") ||
		!hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "data.preview.changes[].before") {
		t.Fatalf("the preview shows the comment as untrusted text: %s", r.out)
	}
	if r.preview()["base"].(map[string]any)["source"] != "updated_at" {
		t.Fatalf("an edited comment is drift: %s", r.out)
	}
}
