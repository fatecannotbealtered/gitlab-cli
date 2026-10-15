package cmd

import (
	"strings"
	"testing"
)

func init() {
	addConformanceSamples(map[string][]string{
		"mr discussion reply":     {"5", "--project", "acme/web", "--discussion-id", "d5b", "--body", "Fixed in the last commit."},
		"mr discussion create":    {"5", "--project", "acme/web", "--new-path", "src/client.go", "--new-line", "42", "--body", "This can be nil."},
		"mr discussion resolve":   {"5", "--project", "acme/web", "--discussion-id", "d5b"},
		"mr discussion unresolve": {"5", "--project", "acme/web", "--discussion-id", "d5c"},
	})
}

func TestMRDiscussionMatrix(t *testing.T) {
	cases := []matrixCase{
		{Path: "mr discussion list", Args: []string{"5", "--project", "acme/web"}, EmptyArgs: []string{"7", "--project", "acme/web"},
			Invalid: []invalidCase{{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--mr"}}},
		{Path: "mr discussion get", Args: []string{"acme/web!5/discussions/d5b"},
			Invalid: []invalidCase{
				{[]string{"5", "--project", "acme/web"}, "E_USAGE", "missing_parameter", "--discussion-id"},
				{[]string{"5", "--project", "acme/web", "--discussion-id", "a/b"}, "E_VALIDATION", "invalid_value", "--discussion-id"},
				{[]string{"acme/web!5/discussions/d5b", "--discussion-id", "d5c"}, "E_USAGE", "conflicting_flags", "--discussion-id"},
			}},
		{Path: "mr discussion reply", Args: []string{"5", "--project", "acme/web", "--discussion-id", "d5b", "--body", "Fixed."},
			Role: "guest", RequestFields: map[string]string{"body": "body"},
			Invalid: []invalidCase{
				{[]string{"acme/web!5/discussions/d5b"}, "E_USAGE", "missing_parameter", "--body"},
				{[]string{"acme/web!5", "--body", "x"}, "E_USAGE", "missing_parameter", "--discussion-id"},
			}},
		{Path: "mr discussion create", Args: []string{"5", "--project", "acme/web", "--new-path", "src/client.go", "--new-line", "42",
			"--body", "This can be nil."}, Role: "guest", RequestFields: map[string]string{"body": "body"},
			Invalid: []invalidCase{
				{[]string{"acme/web!5", "--new-line", "1", "--body", "x"}, "E_USAGE", "missing_parameter", "--new-path"},
				{[]string{"acme/web!5", "--new-path", "a.go", "--body", "x"}, "E_USAGE", "missing_parameter", "--new-line"},
				{[]string{"acme/web!5", "--new-path", "a.go", "--new-line", "-2", "--body", "x"}, "E_VALIDATION", "invalid_value", "--new-line"},
				{[]string{"acme/web!5", "--new-path", "a.go", "--new-line", "2"}, "E_USAGE", "missing_parameter", "--body"},
			}},
		{Path: "mr discussion resolve", Args: []string{"acme/web!5/discussions/d5b"}, Role: "developer",
			Invalid: []invalidCase{{[]string{"acme/web!5"}, "E_USAGE", "missing_parameter", "--discussion-id"}}},
		{Path: "mr discussion unresolve", Args: []string{"acme/web!5/discussions/d5c"}, Role: "developer",
			Invalid: []invalidCase{{[]string{"acme/web!5/discussions/"}, "E_USAGE", "missing_parameter", "--discussion-id"}}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// TestMRDiscussionListIsSummaries: bounded summaries (CLI-043): who opened the
// thread, an excerpt, where it is anchored, resolved or not; no notes. Threads of
// system notes are left out.
func TestMRDiscussionListIsSummaries(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("mr", "discussion", "list", "acme/web!5", "--fields", "all")
	items := r.items()
	if r.exit != 0 || len(items) != 3 {
		t.Fatalf("threads: %s", r.out)
	}
	open, done := items[1], items[2]
	if open["ref"] != "acme/web!5/discussions/d5b" || open["resolvable"] != true || open["resolved"] != false || open["author"] != "@erin" ||
		open["path"] != "src/client.go" || open["line"] == nil || open["note_count"] == nil || open["notes"] != nil ||
		open["updated_at"] != "2026-10-04T09:00:00Z" {
		t.Fatalf("an open thread: %v", open)
	}
	if done["resolved"] != true || items[0]["resolvable"] != false || items[0]["individual_note"] != true {
		t.Fatalf("a resolved thread and a comment: %v %v", done, items[0])
	}
	r = e.run("mr", "threads", "ls", "acme/web!6")
	if r.exit != 0 || len(r.items()) != 1 || !strings.Contains(r.data()["scope"].(map[string]any)["covers"].(string), "system notes") {
		t.Fatalf("system threads left out: %s", r.out)
	}
	e.gl.locked(func() {
		fakeMRByIID(e.gl, 5).Discussions[0].Notes[0].Body = strings.Repeat("long ", 100)
	})
	r = e.run("mr", "discussion", "list", "acme/web!5")
	if ex, _ := r.items()[0]["excerpt"].(string); len([]rune(ex)) != 201 || !strings.HasSuffix(ex, "…") {
		t.Fatalf("the excerpt is bounded: %q", ex)
	}
}

func TestMRDiscussionGetHasTheThread(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("mr", "discussion", "get", "5", "--project", "acme/web", "--discussion-id", "d5b")
	notes, _ := r.data()["notes"].([]any)
	if r.exit != 0 || len(notes) != 2 || notes[1].(map[string]any)["ref"] != "acme/web!5/notes/9003" ||
		notes[1].(map[string]any)["body"] != "Good catch, fixing." {
		t.Fatalf("thread: %s", r.out)
	}
	if u := stringsOf(r.doc["meta"].(map[string]any)["untrusted"]); !hasString(u, "data.notes[].body") || !hasString(u, "data.excerpt") {
		t.Fatalf("note bodies are untrusted: %v", u)
	}
	if r = e.run("mr", "discussion", "get", "acme/web!5#discussion_nope"); r.code() != "E_NOT_FOUND" ||
		!strings.Contains(r.details()["next_action"].(string), "mr discussion list acme/web!5") {
		t.Fatalf("a missing thread: %s", r.out)
	}
}

// TestMRDiscussionReplyAndResolve: the thread's ref and --discussion-id bind as one
// request (CLI-054); resolve and unresolve are blocked when moot, and the 1.x
// resolve --unresolve still works, with a deprecated notice (CLI-081).
func TestMRDiscussionReplyAndResolve(t *testing.T) {
	e := newTestEnv(t)
	tok := e.dryRun("mr", "discussion", "reply", "acme/web!5/discussions/d5b", "--body", "Done.")
	r := e.confirm(tok, "mr", "discussion", "reply", "5", "--project", "acme/web", "--discussion-id", "d5b", "--body", "Done.")
	if res, _ := r.data()["result"].(map[string]any); r.exit != 0 || !strings.HasPrefix(res["ref"].(string), "acme/web!5/notes/") {
		t.Fatalf("reply: %s", r.out)
	}
	for _, c := range []struct{ verb, ref, kind string }{
		{"resolve", "acme/web!5/discussions/d5c", "already_resolved"},
		{"unresolve", "acme/web!5/discussions/d5b", "not_resolved"},
		{"resolve", "acme/web!5/discussions/d5a", "not_resolvable"},
	} {
		r := e.run("mr", "discussion", c.verb, c.ref, "--dry-run")
		if r.exit != 0 || r.token() != "" || r.preview()["blocked_by"].([]any)[0].(map[string]any)["kind"] != c.kind {
			t.Fatalf("%s %s: %s", c.verb, c.ref, r.out)
		}
	}
	args := []string{"mr", "discussion", "resolve", "acme/web!5/discussions/d5b"}
	if r = e.confirm(e.dryRun(args...), args...); r.exit != 0 || r.receipt()["observed"].(map[string]any)["resolved"] != true {
		t.Fatalf("resolve: %s", r.out)
	}
	args = []string{"mr", "discussion", "resolve", "acme/web!5/discussions/d5b", "--unresolve"}
	r = e.confirm(e.dryRun(args...), args...)
	notices, _ := r.doc["meta"].(map[string]any)["notices"].([]any)
	if r.exit != 0 || r.data()["result"].(map[string]any)["resolved"] != false || len(notices) == 0 {
		t.Fatalf("resolve --unresolve: %s", r.out)
	}
}

// TestMRDiscussionCreateAnchorsToTheDiff: the diff SHAs come from the merge
// request; a push between the dry-run and the confirm is state_drift (CLI-057); a
// merge request whose diff is not computed yet is blocked.
func TestMRDiscussionCreateAnchorsToTheDiff(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"mr", "discussion", "create", "acme/web!5", "--new-path", "src/retry.go", "--new-line", "2", "--body", "Name it backoff?"}
	r := e.confirm(e.dryRun(args...), args...)
	res, _ := r.data()["result"].(map[string]any)
	if r.exit != 0 || res["path"] != "src/retry.go" || res["resolvable"] != true || r.outcome()["verification"] != "passed" {
		t.Fatalf("create: %s", r.out)
	}
	req, _ := e.gl.lastWrite()
	if pos, _ := req.Body["position"].(map[string]any); pos["head_sha"] != "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a" || pos["old_path"] != "src/retry.go" {
		t.Fatalf("position: %+v", req.Body)
	}
	tok := e.dryRun(args...)
	e.gl.locked(func() { fakeMRByIID(e.gl, 5).SHA = "5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c5c" })
	if r = e.confirm(tok, args...); r.code() != "E_CONFLICT" || r.details()["conflict"] != "state_drift" {
		t.Fatalf("a push after the dry-run: %s", r.out)
	}
	e.gl.locked(func() { fakeMRByIID(e.gl, 6).BaseSHA = "" })
	r = e.run("mr", "discussion", "create", "acme/web!6", "--new-path", "a.go", "--new-line", "1", "--body", "x", "--dry-run")
	if r.exit != 0 || r.token() != "" || r.preview()["blocked_by"].([]any)[0].(map[string]any)["kind"] != "diff_not_ready" {
		t.Fatalf("no diff yet: %s", r.out)
	}
}
