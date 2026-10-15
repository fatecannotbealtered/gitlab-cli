package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- behaviour matrix

// repoCommitCreateArgs is a valid commit create: two actions, so two targets.
var repoCommitCreateArgs = []string{"--project", "acme/web", "--branch", "main", "--message", "Sync docs",
	"--action", "create:path=docs/a.md;content=hello", "--action", "update:path=README.md;content=new readme"}

func TestRepoCommitMatrix(t *testing.T) {
	sha2 := repoSeedSHA(2)
	cases := []matrixCase{
		{Path: "repo commit list", Args: []string{"--project", "acme/web", "--with-stats"}, EmptyArgs: []string{"--project", "acme/empty"},
			Invalid: []invalidCase{
				{[]string{}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"--project", "acme/web", "--group", "acme"}, "E_USAGE", "conflicting_flags", "--group"},
				{[]string{"--all-projects"}, "E_USAGE", "missing_parameter", "--author"},
				{[]string{"--project", "acme/web", "--since", "yesterday"}, "E_VALIDATION", "invalid_value", "--since"},
				{[]string{"--project", "acme/web", "--since", "2026-07-01", "--until", "2026-06-01"}, "E_VALIDATION", "invalid_value", "--until"},
				{[]string{"--project", "acme/web,acme/empty", "--offset", "20"}, "E_USAGE", "invalid_value", "--offset"},
				{[]string{"--project", "acme/web", "--ref", "x", "--ref-name", "y"}, "E_USAGE", "conflicting_flags", "--ref-name"},
			}},
		{Path: "repo commit get", Args: []string{"main", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--sha"},
				{[]string{"a b", "--project", "acme/web"}, "E_VALIDATION", "invalid_value", "--sha"},
				{[]string{"main"}, "E_USAGE", "missing_parameter", "--project"},
			}},
		{Path: "repo commit diff", Args: []string{sha2, "--project", "acme/web"},
			EmptyArgs: []string{sha2, "--project", "acme/web", "--path", "nope.md"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--sha"},
				{[]string{sha2, "--project", "acme/web", "--limit", "0"}, "E_USAGE", "invalid_value", "--limit"},
				{[]string{sha2, "--project", "acme/web", "--offset", "1", "--format", "raw"}, "E_USAGE", "conflicting_flags", "--offset"},
				{[]string{sha2, "--project", "acme/web", "--path", "../x"}, "E_VALIDATION", "invalid_value", "--path"},
			}},
		{Path: "repo commit create", Args: repoCommitCreateArgs,
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web", "--branch", "main", "--message", "m"}, "E_USAGE", "missing_parameter", "--action"},
				{[]string{"--project", "acme/web", "--branch", "main", "--message", "m", "--action", "rename:path=a"}, "E_USAGE", "invalid_value", "--action"},
				{[]string{"--project", "acme/web", "--branch", "main", "--message", "m", "--action", "create:path=a.md"}, "E_USAGE", "invalid_value", "--action"},
				{[]string{"--project", "acme/web", "--branch", "main", "--message", "m", "--action", "move:path=b.md"}, "E_USAGE", "invalid_value", "--action"},
				{[]string{"--project", "acme/web", "--branch", "main", "--message", "m", "--action", "delete:path=a.md",
					"--action", "create:path=a.md;content=x"}, "E_USAGE", "invalid_value", "--action"},
				{[]string{"--project", "acme/web", "--branch", "main", "--message", " ", "--action", "delete:path=a.md"}, "E_VALIDATION", "invalid_value", "--message"},
			},
			// One commit carries every action (REPO-L2): the shared batch rows with
			// OneRequest; its reads before the commit are the branch and the two files.
			OneRequest: true, PreviewReads: 3},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

func repoCommitCreateLine(extra ...string) []string {
	return append(append([]string{"repo", "commit", "create"}, repoCommitCreateArgs...), extra...)
}

// TestRepoCommitCreateIsOneCommit: the batch's items go out as one commit (one
// request with every action, updates carrying last_commit_id), the branch moves to
// it, every item reports it, and the actions bind their content's digest.
func TestRepoCommitCreateIsOneCommit(t *testing.T) {
	e := newTestEnv(t)
	dry := e.run(repoCommitCreateLine("--dry-run")...)
	if inputs := dry.preview()["inputs"].(map[string]any); !strings.Contains(repoJSON(inputs["action"]), "content_sha256=") {
		t.Fatalf("the actions bind their content's digest: %v", inputs["action"])
	}
	r := e.confirm(dry.token(), repoCommitCreateLine()...)
	if r.exit != 0 || len(r.items()) != 2 {
		t.Fatalf("confirm: %s", r.out)
	}
	sha := ""
	for _, it := range r.items() {
		res := it["result"].(map[string]any)
		checkKeys(t, "result", res, []string{"ref", "path", "action", "commit_ref", "commit_sha"})
		if res["commit_sha"] == nil || (sha != "" && res["commit_sha"] != sha) {
			t.Fatalf("one commit for every item: %v", r.items())
		}
		sha = res["commit_sha"].(string)
	}
	if e.repoBranchHead("acme/web", "main") != sha || e.gl.Writes() != 1 {
		t.Fatalf("main moves to the one new commit (writes %d)", e.gl.Writes())
	}
	req, _ := e.gl.lastWrite()
	actions, _ := req.Body["actions"].([]any)
	if len(actions) != 2 || actions[1].(map[string]any)["last_commit_id"] != repoSeedSHA(2) {
		t.Fatalf("one request with every action, updates carrying last_commit_id: %v", req.Body)
	}
}

// ---------------------------------------------------------------- focused tests

// TestRepoCommitListFiltersAndScope: the filters reach GitLab and the scope echoes them,
// times as absolute UTC (CLI-028); line counts, default fields, are read whenever
// the response shows them (aicli.WantsField, Q17) and not when --fields leaves them
// out.
func TestRepoCommitListFiltersAndScope(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("repo", "commit", "list", "--project", "acme/web", "--author", "alice")
	if r.exit != 0 || len(r.items()) != 2 || r.items()[0]["ref"] != "acme/web/-/commit/"+repoSeedSHA(3) || r.data()["sort"] != "committed_date_desc" {
		t.Fatalf("--author: %s", r.out)
	}
	if r.items()[0]["additions"] == nil || r.items()[0]["deletions"] == nil {
		t.Fatalf("the default fields hold the line counts: %s", r.out)
	}
	before := len(e.gl.Requests())
	if r = e.run("repo", "commit", "list", "--project", "acme/web", "--fields", "ref,title"); r.exit != 0 {
		t.Fatalf("--fields ref,title: %s", r.out)
	}
	if reqs := e.gl.Requests(); len(reqs) <= before || strings.Contains(reqs[len(reqs)-1].Query, "with_stats=true") {
		t.Fatalf("no line counts read when the response leaves them out: %+v", reqs[len(reqs)-1])
	}
	r = e.run("repo", "commit", "list", "--project", "acme/web", "--since", "2026-06-10", "--path", "docs")
	scope := r.data()["scope"].(map[string]any)
	if r.exit != 0 || len(r.items()) != 1 || scope["since"] != "2026-06-10T00:00:00Z" || scope["path"] != "docs" || scope["project"] != "acme/web" {
		t.Fatalf("--since and --path: %s", r.out)
	}
	r = e.run("repo", "commit", "list", "--project", "acme/web", "--since", "36500d")
	if since, _ := r.data()["scope"].(map[string]any)["since"].(string); r.exit != 0 || !strings.HasSuffix(since, "Z") || len(r.items()) != 3 {
		t.Fatalf("a relative --since is echoed absolute: %s", r.out)
	}
	r = e.run("repo", "commit", "list", "--project", "acme/web", "--fields", "+total")
	if r.items()[0]["total"] == nil || r.items()[0]["additions"] == nil {
		t.Fatalf("--fields with a line count asks GitLab for them: %s", r.out)
	}
	r = e.run("repo", "commit", "list", "--project", "acme/web", "--ref-name", "develop")
	notices, _ := r.doc["meta"].(map[string]any)["notices"].([]any)
	if r.exit != 0 || len(r.items()) != 2 || len(notices) != 1 || r.data()["scope"].(map[string]any)["ref_name"] != "develop" {
		t.Fatalf("--ref-name: %s", r.out)
	}
	if _, has := r.items()[0]["message"]; has {
		t.Fatalf("lists carry no message body (CLI-043): %s", r.out)
	}
	if r := e.run("repo", "commit", "list", "--project", "acme/web", "--fields", "message"); r.details()["parameter"] != "--fields" {
		t.Fatalf("message is not a list field: %s", r.out)
	}
}

// TestRepoCommitListFanOut: several projects or a group are read concurrently, merged
// newest first and cut to --limit; a project that cannot be read is named in scope;
// a list that cannot be paged says how to continue (CLI-041).
func TestRepoCommitListFanOut(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("repo", "commit", "list", "--project", "acme/web,acme/empty")
	scope := r.data()["scope"].(map[string]any)
	if r.exit != 0 || len(r.items()) != 3 || scope["projects_scanned"] != repoNumber(2) || repoJSON(scope["projects"]) != `["acme/web","acme/empty"]` ||
		r.data()["total"] != repoNumber(3) || r.data()["has_more"] != false {
		t.Fatalf("two projects: %s", r.out)
	}
	r = e.run("repo", "commit", "list", "--project", "acme/web", "--project", "acme/nowhere", "--limit", "2")
	scope = r.data()["scope"].(map[string]any)
	if r.exit != 0 || len(r.items()) != 2 || r.data()["has_more"] != true || r.data()["continuation_reason"] == nil ||
		repoJSON(scope["projects_failed"]) != `[{"code":"E_NOT_FOUND","project":"acme/nowhere"}]` {
		t.Fatalf("a project that cannot be read: %s", r.out)
	}
	r = e.run("repo", "commit", "list", "--group", "acme", "--author", "bob")
	if r.exit != 0 || len(r.items()) != 1 || r.items()[0]["title"] != "Add login" || repoJSON(r.data()["scope"].(map[string]any)["group"]) != `["acme"]` {
		t.Fatalf("--group: %s", r.out)
	}
	if r := e.run("repo", "commit", "list", "--project", "acme/nowhere,acme/gone"); r.code() != "E_NOT_FOUND" {
		t.Fatalf("no project could be read: %s", r.out)
	}
	if r := e.run("repo", "commit", "list", "--group", "nope"); r.code() != "E_NOT_FOUND" {
		t.Fatalf("a missing group: %s", r.out)
	}
	if e.gl.Writes() != 0 {
		t.Fatal("a read wrote")
	}
}

// TestRepoCommitGetForms: a SHA, its prefix, the ref, GitLab's acme/web@sha, the web URL
// and a branch name read the same commit; the message comes whole up to a size, then
// cut, flagged and printed whole with --format raw (CLI-042).
func TestRepoCommitGetForms(t *testing.T) {
	e := newTestEnv(t)
	sha := repoSeedSHA(3)
	for _, args := range [][]string{
		{"repo", "commit", "get", sha, "--project", "acme/web"},
		{"repo", "commit", "get", sha[:8], "--project", "acme/web"},
		{"repo", "commit", "get", "acme/web/-/commit/" + sha},
		{"repo", "commit", "get", "acme/web@" + sha[:10]},
		{"repo", "commit", "get", e.gl.URL + "/acme/web/-/commit/" + sha + "/diffs"},
		{"commit", "get", "main", "-p", "acme/web"},
	} {
		r := e.run(args...)
		d := r.data()
		if r.exit != 0 || d["sha"] != sha || d["ref"] != "acme/web/-/commit/"+sha || d["message"] != "Fix typo in guide\n\nThe title said Guide." ||
			d["message_truncated"] != false || d["additions"] == nil || repoJSON(d["parent_shas"]) != `["`+repoSeedSHA(2)+`"]` {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
	long := "Squash\n\n" + strings.Repeat("One line of the squashed history.\n", 300)
	e.gl.locked(func() { fakeRepos(e.gl)["acme/web"].Commits[sha].Message = long })
	r := e.run("repo", "commit", "get", sha, "--project", "acme/web")
	if r.exit != 0 || r.data()["message_truncated"] != true || len(r.out) > 8000 {
		t.Fatalf("a long message: %d", len(r.out))
	}
	if raw := e.run("repo", "commit", "get", sha, "--project", "acme/web", "--format", "raw"); raw.out != long {
		t.Fatal("raw prints the whole message")
	}
	if r := e.run("repo", "commit", "get", "deadbeef", "--project", "acme/web"); r.code() != "E_NOT_FOUND" {
		t.Fatalf("a missing commit: %s", r.out)
	}
}

// TestRepoCommitDiffPages: a diff comes as per-file entries with line counts, a page
// holding what fits the size budget (CLI-042, CLI-043); --path keeps one file;
// --format raw prints a unified diff; a deleted file has no ref.
func TestRepoCommitDiffPages(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("repo", "commit", "diff", repoSeedSHA(2), "--project", "acme/web")
	if r.exit != 0 || len(r.items()) != 2 || r.data()["total"] != repoNumber(2) || r.data()["scope"].(map[string]any)["commit"] != repoSeedSHA(2) {
		t.Fatalf("diff: %s", r.out)
	}
	login := r.items()[0]
	if login["path"] != "src/login.go" || login["change"] != "added" || login["additions"] != repoNumber(3) ||
		login["ref"] != "acme/web/-/blob/"+repoSeedSHA(2)+"/src/login.go" {
		t.Fatalf("an added file: %v", login)
	}
	if r := e.run("repo", "commit", "diff", "main", "--project", "acme/web", "--path", "docs/guide.md"); len(r.items()) != 1 ||
		r.items()[0]["deletions"] != repoNumber(1) || r.data()["scope"].(map[string]any)["commit"] != repoSeedSHA(3) {
		t.Fatalf("--path and a branch name: %s", r.out)
	}
	raw := e.run("repo", "commit", "diff", repoSeedSHA(2), "--project", "acme/web", "--format", "raw")
	if !strings.Contains(raw.out, "diff --git a/src/login.go b/src/login.go\n--- /dev/null\n+++ b/src/login.go\n@@") {
		t.Fatalf("raw: %s", raw.out)
	}
	// Three files with long patches: each page holds what fits.
	big := strings.Repeat("a line of a long file\n", 150)
	e.gl.locked(func() {
		rp := fakeRepos(e.gl)["acme/web"]
		c := rp.commit(fakeRepoSHA("big"), rp.Branches["main"].Head, "Big change", "Erin Hannon", e.gl.now(), func(f map[string][]byte) []fakeRepoDiff {
			return []fakeRepoDiff{fakeRepoAdd(f, "big/a.txt", []byte(big)), fakeRepoAdd(f, "big/b.txt", []byte(big)), fakeRepoRemove(f, "docs/guide.md")}
		})
		rp.Branches["main"].Head = c.ID
	})
	seen, offset := 0, ""
	for page := 0; page < 5; page++ {
		args := []string{"repo", "commit", "diff", fakeRepoSHA("big"), "--project", "acme/web"}
		if offset != "" {
			args = append(args, "--offset", offset)
		}
		r := e.run(args...)
		if r.exit != 0 || len(r.out) > 8000 || len(r.items()) == 0 {
			t.Fatalf("page %d: %d characters: %.300s", page, len(r.out), r.out)
		}
		seen += len(r.items())
		if last := r.items()[len(r.items())-1]; last["change"] == "deleted" && last["ref"] != nil {
			t.Fatalf("a deleted file has no ref: %v", last)
		}
		if r.data()["has_more"] == false {
			break
		}
		offset = string(r.data()["next_offset"].(json.Number))
	}
	if seen != 3 {
		t.Fatalf("every file once, saw %d", seen)
	}
}

// TestRepoCommitCreateActions: creates, updates, deletes and moves land as one commit;
// each touched file is a target, so a move is two; the read-back comes from the
// commit's diff (CLI-063). Deleting a file makes the plan dangerous (Q13).
func TestRepoCommitCreateActions(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"commit", "create", "-p", "acme/web", "--branch", "develop", "--message", "Reorganise",
		"--action", "create:path=docs/new.md;content=new",
		"--action", "update:path=README.md;content=# Web\n",
		"--action", "delete:path=docs/guide.md",
		"--action", "move:path=src/main.go;previous_path=src/app.go"}
	if r := e.run(append(args, "--dry-run")...); r.details()["problem"] != "dangerous_flag_required" || r.token() != "" {
		t.Fatalf("a commit that deletes a file is dangerous: %s", r.out)
	}
	args = append(args, "--dangerous")
	dry := e.run(append(args, "--dry-run")...)
	if dry.exit != 0 || dry.preview()["total"] != repoNumber(5) || dry.preview()["dangerous"] != true {
		t.Fatalf("five targets: %s", dry.out)
	}
	r := e.confirm(dry.token(), args...)
	if r.exit != 0 || r.data()["summary"].(map[string]any)["succeeded"] != repoNumber(5) {
		t.Fatalf("confirm: %s", r.out)
	}
	for _, it := range r.items() {
		if it["receipt"].(map[string]any)["outcome"].(map[string]any)["verification"] != "passed" {
			t.Fatalf("item: %v", it)
		}
	}
	if _, ok := e.repoFileAt("acme/web", "develop", "src/main.go"); !ok {
		t.Fatal("the move landed")
	}
	if _, ok := e.repoFileAt("acme/web", "develop", "src/app.go"); ok {
		t.Fatal("the moved file is gone from its old path")
	}
	if got, _ := e.repoFileAt("acme/web", "develop", "README.md"); got != "# Web\n" {
		t.Fatalf("update: %q", got)
	}
}

// TestRepoCommitCreateBindsContent: content_file binds by its bytes like the same
// content inline; a file changed after the dry-run is token_mismatch naming
// --action (CLI-054).
func TestRepoCommitCreateBindsContent(t *testing.T) {
	e := newTestEnv(t)
	file := filepath.Join(e.dir, "a.md")
	if err := os.WriteFile(file, []byte("from a file"), 0o600); err != nil {
		t.Fatal(err)
	}
	withFile := []string{"repo", "commit", "create", "-p", "acme/web", "--branch", "main", "--message", "m", "--action", "create:path=docs/f.md;content_file=" + file}
	inline := []string{"repo", "commit", "create", "-p", "acme/web", "--branch", "main", "--message", "m", "--action", "create:path=docs/f.md;content=from a file"}
	tok := e.dryRun(withFile...)
	if err := os.WriteFile(file, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := e.confirm(tok, withFile...); r.details()["conflict"] != "token_mismatch" || r.details()["parameter"] != "--action" || e.gl.Writes() != 0 {
		t.Fatalf("a changed content_file: %s", r.out)
	}
	if err := os.WriteFile(file, []byte("from a file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := e.confirm(e.dryRun(withFile...), inline...); r.exit != 0 {
		t.Fatalf("content_file and the same inline content are one request: %s", r.out)
	}
	if got, _ := e.repoFileAt("acme/web", "main", "docs/f.md"); got != "from a file" {
		t.Fatalf("content: %q", got)
	}
}

// TestRepoCommitCreatePlans: a create of an existing file and an action on a missing one
// block the plan; a new branch needs --start-branch; a file changed after the
// dry-run is state_drift; GitLab's 1000-action cap is checked first.
func TestRepoCommitCreatePlans(t *testing.T) {
	e := newTestEnv(t)
	line := func(extra ...string) []string {
		return append([]string{"repo", "commit", "create", "-p", "acme/web", "--message", "m"}, extra...)
	}
	r := e.run(line("--branch", "main", "--action", "create:path=README.md;content=x", "--action", "delete:path=nope.md", "--dry-run")...)
	if r.exit != 0 || r.preview()["can_apply"] != false || r.token() != "" || !strings.Contains(repoJSON(r.preview()["blocked_by"]), "already_exists") ||
		!strings.Contains(repoJSON(r.preview()["blocked_by"]), "not_found") {
		t.Fatalf("blocked items: %s", r.out)
	}
	r = e.run(line("--branch", "topic", "--action", "create:path=t.md;content=t", "--dry-run")...)
	if r.preview()["can_apply"] != false || !strings.Contains(repoJSON(r.preview()["blocked_by"]), "start-branch") {
		t.Fatalf("a missing branch: %s", r.out)
	}
	args := line("--branch", "topic", "--start-branch", "develop", "--action", "create:path=t.md;content=t")
	if r := e.confirm(e.dryRun(args...), args...); r.exit != 0 || e.repoBranchHead("acme/web", "topic") == "" {
		t.Fatalf("--start-branch: %s", r.out)
	}
	args = line("--branch", "main", "--action", "update:path=docs/guide.md;content=mine")
	tok := e.dryRun(args...)
	e.repoPush("acme/web", "main", "docs/guide.md", "theirs")
	if r := e.confirm(tok, args...); r.details()["conflict"] != "state_drift" {
		t.Fatalf("a file changed after the dry-run: %s", r.out)
	}
	many := line("--branch", "main", "--dry-run")
	for i := 0; i <= repoCommitActionsMax; i++ {
		many = append(many, "--action", fmt.Sprintf("create:path=f%d.txt;content=x", i))
	}
	before := len(e.gl.Requests())
	if r := e.run(many...); r.details()["problem"] != "input_too_large" || len(e.gl.Requests()) != before {
		t.Fatalf("over GitLab's cap, refused before any request: %.300s", r.out)
	}
}
