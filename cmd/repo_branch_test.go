package cmd

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------- behaviour matrix

func TestRepoBranchMatrix(t *testing.T) {
	cases := []matrixCase{
		{Path: "repo branch list", Args: []string{"--project", "acme/web"}, EmptyArgs: []string{"--project", "acme/empty"},
			Invalid: []invalidCase{
				{[]string{}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"--project", "acme/web", "--limit", "101"}, "E_USAGE", "invalid_value", "--limit"},
				{[]string{"--project", "acme/web", "--query", "a", "--search", "b"}, "E_USAGE", "conflicting_flags", "--search"},
			}},
		{Path: "repo branch get", Args: []string{"main", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--name"},
				{[]string{"a..b", "--project", "acme/web"}, "E_VALIDATION", "invalid_value", "--name"},
				{[]string{"main"}, "E_USAGE", "missing_parameter", "--project"},
			}},
		{Path: "repo branch create", Args: []string{"feature-x", "--project", "acme/web", "--ref", "main"},
			Role: "developer", PreviewReads: 2,
			Invalid: []invalidCase{
				{[]string{"feature-x", "--project", "acme/web"}, "E_USAGE", "missing_parameter", "--ref"},
				{[]string{"bad name", "--project", "acme/web", "--ref", "main"}, "E_VALIDATION", "invalid_value", "--name"},
				{[]string{"x", "--project", "acme/web", "--ref", "a b"}, "E_VALIDATION", "invalid_value", "--ref"},
			}},
		{Path: "repo branch delete", Args: []string{"old-experiment", "--project", "acme/web"}, Role: "developer",
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web", "--dangerous"}, "E_USAGE", "missing_parameter", "--name"},
				{[]string{"x.lock", "--project", "acme/web", "--dangerous"}, "E_VALIDATION", "invalid_value", "--name"},
			}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// ---------------------------------------------------------------- focused tests

// TestRepoBranchFormsAreOneTarget: the name, the ref, both web URLs and --branch read
// the same branch and bind as one request (CLI-048, CLI-054).
func TestRepoBranchFormsAreOneTarget(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"repo", "branch", "get", "release/1.0", "--project", "acme/web"},
		{"repo", "branch", "get", "acme/web/-/branches/release/1.0"},
		{"repo", "branch", "get", e.gl.URL + "/acme/web/-/tree/release/1.0"},
		{"repo", "branch", "get", e.gl.URL + "/acme/web/-/branches/release%2F1.0"},
		{"branch", "get", "--branch", "release/1.0", "--project", "1"},
	} {
		r := e.run(args...)
		if r.exit != 0 || r.data()["ref"] != "acme/web/-/branches/release/1.0" || r.data()["protected"] != true ||
			r.data()["commit_sha"] != repoSeedSHA(2) {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
	tok := e.dryRun("branch", "create", "acme/web/-/branches/topic", "--ref", "main")
	if r := e.confirm(tok, "repo", "branch", "create", "--name", "topic", "--project", "acme/web", "--ref", "main"); r.exit != 0 {
		t.Fatalf("the ref and the name must bind alike: %s", r.out)
	}
	if r := e.run("repo", "branch", "get", "nope", "--project", "acme/web"); r.code() != "E_NOT_FOUND" ||
		!strings.Contains(r.details()["next_action"].(string), "repo branch list") {
		t.Fatalf("a missing branch: %s", r.out)
	}
}

// TestRepoBranchListQueryAndScope: --query filters on GitLab's side and is echoed in
// scope; 1.x's --search still works, with a deprecated notice (CLI-081).
func TestRepoBranchListQueryAndScope(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("repo", "branch", "list", "--project", "acme/web", "--query", "^feat")
	if r.exit != 0 || len(r.items()) != 1 || r.items()[0]["name"] != "feature-login" || r.data()["sort"] != "name_asc" ||
		r.data()["scope"].(map[string]any)["query"] != "^feat" {
		t.Fatalf("--query: %s", r.out)
	}
	if r.items()[0]["merged"] != true || r.items()[0]["committed_date"] != "2026-06-15T08:00:00Z" {
		t.Fatalf("merged and the last commit's date (UTC): %v", r.items()[0])
	}
	r = e.run("repo", "branch", "list", "--project", "acme/web", "--search", "old")
	notices, _ := r.doc["meta"].(map[string]any)["notices"].([]any)
	if r.exit != 0 || len(r.items()) != 1 || len(notices) != 1 || notices[0].(map[string]any)["type"] != "deprecated" {
		t.Fatalf("--search: %s", r.out)
	}
	if !hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "data.items[].name") {
		t.Fatalf("branch names are untrusted: %s", r.out)
	}
}

// TestRepoBranchCreateStartsWherePreviewed: the branch starts at the commit the preview
// resolved --ref to; a name that exists blocks the plan; a missing --ref is
// E_NOT_FOUND naming it; a repeat with the same key is replayed (CLI-062).
func TestRepoBranchCreateStartsWherePreviewed(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("repo", "branch", "create", "develop", "--project", "acme/web", "--ref", "main", "--dry-run")
	if r.preview()["can_apply"] != false || r.token() != "" {
		t.Fatalf("an existing branch blocks: %s", r.out)
	}
	r = e.run("repo", "branch", "create", "x", "--project", "acme/web", "--ref", "nowhere", "--dry-run")
	if r.code() != "E_NOT_FOUND" || r.details()["parameter"] != "--ref" {
		t.Fatalf("a missing --ref: %s", r.out)
	}
	if r := e.run("repo", "branch", "create", "x", "--project", "acme/nowhere", "--ref", "main", "--dry-run"); r.code() != "E_NOT_FOUND" ||
		!strings.Contains(r.details()["next_action"].(string), "--project") {
		t.Fatalf("a missing project: %s", r.out)
	}
	args := []string{"repo", "branch", "create", "topic", "--project", "acme/web", "--ref", "v1.0", "--idempotency-key", "br-1"}
	r = e.confirm(e.dryRun(args...), args...)
	res := r.data()["result"].(map[string]any)
	if r.exit != 0 || res["ref"] != "acme/web/-/branches/topic" || res["commit_sha"] != repoSeedSHA(2) || r.outcome()["verification"] != "passed" {
		t.Fatalf("create: %s", r.out)
	}
	if req, _ := e.gl.lastWrite(); !strings.Contains(req.Query, "ref="+repoSeedSHA(2)) {
		t.Fatalf("the resolved SHA is sent, not the name: %s", req.Query)
	}
	if again := e.run(append(args, "--dry-run")...); again.receipt()["replayed"] != true || again.token() != "" || e.gl.Writes() != 1 {
		t.Fatalf("replay: %s", again.out)
	}
}

// TestRepoBranchDeleteOldExperiment is the eval's dangerous-action task: `branch delete
// old-experiment` through the root shortcut, the name as a positional, --dangerous in
// both steps (CLI-056), read back as gone.
func TestRepoBranchDeleteOldExperiment(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("branch", "delete", "old-experiment", "--project", "acme/web", "--dry-run")
	if r.code() != "E_CONFIRMATION_REQUIRED" || r.details()["problem"] != "dangerous_flag_required" {
		t.Fatalf("without --dangerous: %s", r.out)
	}
	args := []string{"branch", "delete", "old-experiment", "--project", "acme/web", "--dangerous"}
	dry := e.run(append(args, "--dry-run")...)
	p := dry.preview()
	if dry.exit != 0 || p["target"].(map[string]any)["key"] != "acme/web/-/branches/old-experiment" ||
		p["base"].(map[string]any)["version"] != repoSeedSHA(1) || !strings.Contains(repoJSON(p["changes"]), "already merged") {
		t.Fatalf("dry-run: %s", dry.out)
	}
	if r := e.confirm(dry.token(), "branch", "delete", "old-experiment", "--project", "acme/web"); r.details()["problem"] != "dangerous_flag_required" {
		t.Fatalf("the confirm needs --dangerous too: %s", r.out)
	}
	r = e.confirm(dry.token(), args...)
	if r.exit != 0 || r.receipt()["observed"].(map[string]any)["exists"] != false || r.outcome()["verification"] != "passed" ||
		r.data()["result"].(map[string]any)["deleted"] != true {
		t.Fatalf("confirm: %s", r.out)
	}
	if e.repoBranchHead("acme/web", "old-experiment") != "" {
		t.Fatal("the branch is still there")
	}
	if r := e.run(append(args, "--dry-run")...); r.code() != "E_NOT_FOUND" || r.exit != 3 {
		t.Fatalf("a missing branch is E_NOT_FOUND at the dry-run (CLI-029): %s", r.out)
	}
}

// TestRepoBranchDeleteRefusals: GitLab refuses to delete the default branch and
// protected ones, so the plan is blocked; a push after the dry-run is state_drift,
// so work pushed since the plan is never deleted unseen.
func TestRepoBranchDeleteRefusals(t *testing.T) {
	e := newTestEnv(t)
	for name, kind := range map[string]string{"main": "default_branch", "release/1.0": "protected_branch"} {
		r := e.run("repo", "branch", "delete", name, "--project", "acme/web", "--dangerous", "--dry-run")
		if r.exit != 0 || r.preview()["can_apply"] != false || r.token() != "" || !strings.Contains(repoJSON(r.preview()["blocked_by"]), kind) {
			t.Fatalf("%s: %s", name, r.out)
		}
	}
	args := []string{"repo", "branch", "delete", "feature-login", "--project", "acme/web", "--dangerous"}
	tok := e.dryRun(args...)
	e.repoPush("acme/web", "feature-login", "late.txt", "pushed after the plan")
	r := e.confirm(tok, args...)
	if r.code() != "E_CONFLICT" || r.details()["conflict"] != "state_drift" || e.gl.Writes() != 0 {
		t.Fatalf("a push after the dry-run: %s", r.out)
	}
	if e.repoBranchHead("acme/web", "feature-login") == "" {
		t.Fatal("the branch was deleted")
	}
}
