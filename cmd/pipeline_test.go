package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- behaviour matrix

func TestPipelineMatrix(t *testing.T) {
	cases := []matrixCase{
		{Path: "pipeline list", Args: []string{"--project", "acme/web"}, EmptyArgs: []string{"--project", "acme/empty"},
			Invalid: []invalidCase{
				{[]string{}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"--project", "acme/web", "--state", "broken"}, "E_USAGE", "invalid_value", "--state"},
				{[]string{"--project", "acme/web", "--limit", "101"}, "E_USAGE", "invalid_value", "--limit"},
				{[]string{"--project", "acme/web", "--status", "failed", "--state", "failed"}, "E_USAGE", "conflicting_flags", "--status"},
			}},
		{Path: "pipeline get", Args: []string{"204", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--pipeline"},
				{[]string{"204"}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"pipe-1", "--project", "acme/web"}, "E_VALIDATION", "invalid_value", "--pipeline"},
				{[]string{"acme/web/-/jobs/302"}, "E_VALIDATION", "invalid_value", "--pipeline"},
				{[]string{"204", "--project", "acme/web", "--wait", "5m"}, "E_USAGE", "invalid_value", "--wait"},
				{[]string{"acme/web/-/pipelines/204", "--project", "acme/other"}, "E_USAGE", "conflicting_flags", "--project"},
			}},
		{Path: "pipeline current", Args: []string{"--project", "acme/web", "--ref", "main"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/../web", "--ref", "main"}, "E_USAGE", "invalid_value", "--project"},
			}},
		{Path: "pipeline create", Args: []string{"--project", "acme/web", "--ref", "main"},
			Role: "developer", RefusedStatus: 400, RequestFields: map[string]string{"ref_name": "ref"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--ref"},
				{[]string{"--project", "acme/web", "--ref", " "}, "E_VALIDATION", "invalid_value", "--ref"},
				{[]string{"--project", "acme/web", "--ref", "main", "--variable", "NO_VALUE"}, "E_VALIDATION", "invalid_value", "--variable"},
				{[]string{"--project", "acme/web", "--ref", "main", "--variable", "A=1", "--variable", "A=2"}, "E_VALIDATION", "invalid_value", "--variable"},
			}},
		{Path: "pipeline retry", Args: []string{"202", "--project", "acme/web"}, Role: "developer",
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--pipeline"},
			}},
		{Path: "pipeline cancel", Args: []string{"203", "--project", "acme/web"}, Role: "developer",
			Invalid: []invalidCase{
				{[]string{"0", "--project", "acme/web"}, "E_VALIDATION", "invalid_value", "--pipeline"},
			}},
		{Path: "pipeline jobs", Args: []string{"204", "--project", "acme/web"},
			EmptyArgs: []string{"204", "--project", "acme/web", "--state", "running"},
			Invalid: []invalidCase{
				{[]string{"204", "--project", "acme/web", "--state", "broken"}, "E_USAGE", "invalid_value", "--state"},
				{[]string{"204", "--project", "acme/web", "--scope", "failed", "--state", "failed"}, "E_USAGE", "conflicting_flags", "--scope"},
				// 1.x's --scope is --state (Meta.RenamedFlags), so the refusal names --state.
				{[]string{"204", "--project", "acme/web", "--scope", "failed,nope"}, "E_USAGE", "invalid_value", "--state"},
			}},
		{Path: "pipeline wait", Args: []string{"204", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--pipeline"},
			}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// ---------------------------------------------------------------- focused tests

// TestPipelineFormsAreOneTarget: the ID, #ID, the ref and the web URL read the same
// pipeline and bind as one request (CLI-048, CLI-054).
func TestPipelineFormsAreOneTarget(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"pipeline", "get", "204", "--project", "acme/web"},
		{"pipeline", "get", "#204", "-p", "acme/web"},
		{"pipeline", "get", "acme/web/-/pipelines/204"},
		{"pipeline", "get", e.gl.URL + "/acme/web/-/pipelines/204/failures"},
		{"pipelines", "view", "--pipeline", "204", "--repo", "acme/web"},
		{"ci", "show", "204", "--project", "1"},
	} {
		r := e.run(args...)
		if r.exit != 0 || r.data()["ref"] != "acme/web/-/pipelines/204" || r.data()["id"] != "204" {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
	tok := e.dryRun("pipeline", "retry", "acme/web/-/pipelines/202")
	if r := e.confirm(tok, "pipeline", "retry", "202", "--project", "acme/web"); r.exit != 0 {
		t.Fatalf("the ID and the ref must bind alike: %s", r.out)
	}
}

// TestPipelineGetShowsTheDetail: UTC times, milliseconds, the user's ref and the
// untrusted fields (CLI-007, CLI-046).
func TestPipelineGetShowsTheDetail(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("pipeline", "get", "204", "--project", "acme/web")
	d := r.data()
	if d["status"] != "failed" || d["finished"] != true || d["ref_name"] != "main" || d["user"] != "@carol" || d["name"] != "Nightly" ||
		d["created_at"] != "2026-10-12T08:31:00Z" || d["duration_ms"] != json.Number("480000") || d["coverage"] != json.Number("81.3") {
		t.Fatalf("pipeline get: %s", r.out)
	}
	for _, path := range []string{"data.ref_name", "data.name", "data.yaml_errors"} {
		if !hasString(ciUntrusted(r), path) {
			t.Errorf("%s is untrusted: %v", path, ciUntrusted(r))
		}
	}
	if r := e.run("pipeline", "get", "999", "--project", "acme/web"); r.code() != "E_NOT_FOUND" ||
		!strings.Contains(r.details()["next_action"].(string), "pipeline list --project acme/web") {
		t.Fatalf("a missing pipeline: %s", r.out)
	}
}

// TestPipelineEvalLatestFailedOnMain answers the 2026-10 eval question in three
// calls: the latest failed pipeline on main, its failed jobs with their reasons, and
// the end of the failed job's log, cleaned of terminal codes.
func TestPipelineEvalLatestFailedOnMain(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("pipeline", "list", "--project", "acme/web", "--ref", "main", "--state", "failed", "--limit", "1")
	if r.exit != 0 || len(r.items()) != 1 || r.items()[0]["ref"] != "acme/web/-/pipelines/204" || r.data()["has_more"] != true ||
		r.data()["total"] != json.Number("2") || r.data()["sort"] != "id_desc" {
		t.Fatalf("the latest failed pipeline on main: %s", r.out)
	}
	if scope := r.data()["scope"].(map[string]any); scope["state"] != "failed" || scope["ref_name"] != "main" {
		t.Fatalf("scope: %v", scope)
	}
	r = e.run("pipeline", "jobs", "acme/web/-/pipelines/204", "--state", "failed")
	if r.exit != 0 || len(r.items()) != 2 {
		t.Fatalf("its failed jobs: %s", r.out)
	}
	test, lint := r.items()[0], r.items()[1]
	if test["ref"] != "acme/web/-/jobs/302" || test["failure_reason"] != "script_failure" || test["allow_failure"] != false ||
		lint["allow_failure"] != true {
		t.Fatalf("failure_reason and allow_failure tell the real failure: %v %v", test, lint)
	}
	if scope := r.data()["scope"].(map[string]any); scope["pipeline"] != "acme/web/-/pipelines/204" ||
		!strings.Contains(scope["covers"].(string), "latest attempt") {
		t.Fatalf("scope: %v", scope)
	}
	r = e.run("job", "log", test["ref"].(string))
	log, _ := r.data()["log"].(string)
	if r.exit != 0 || !strings.Contains(log, "--- FAIL: TestCheckout (0.02s)") || !strings.HasSuffix(log, "ERROR: Job failed: exit code 1") ||
		strings.ContainsAny(log, "\x1b\r") || strings.Contains(log, "section_") || !strings.Contains(log, "Downloading 100%\n") {
		t.Fatalf("the failed job's log: %s", r.out)
	}
	if !hasString(ciUntrusted(r), "data.log") || r.data()["truncated"] != false || r.data()["finished"] != true {
		t.Fatalf("log untrusted, complete: %s", r.out)
	}
	if n := len(e.gl.Requests()); n != 4 {
		t.Errorf("three calls make four requests (list, jobs, job and trace), not %d", n)
	}
}

func TestPipelineListFiltersAndDeprecatedStatus(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("pipeline", "list", "--project", "acme/web", "--status", "running")
	if r.exit != 0 || len(r.items()) != 1 || r.items()[0]["ref"] != "acme/web/-/pipelines/203" || !hasString(ciNotices(r), "deprecated") {
		t.Fatalf("--status is a deprecated --state: %s", r.out)
	}
	r = e.run("pipeline", "list", "--project", "acme/web", "--username", "@dave")
	if len(r.items()) != 1 || r.data()["scope"].(map[string]any)["username"] != "dave" {
		t.Fatalf("--username @dave: %s", r.out)
	}
	r = e.run("pipeline", "list", "--project", "acme/web", "--username", "me")
	if len(r.items()) != 2 || r.data()["scope"].(map[string]any)["username"] != "carol" {
		t.Fatalf("--username me is this account: %s", r.out)
	}
	r = e.run("pipeline", "list", "--project", "acme/web", "--username", "nobody")
	if r.exit != 0 || len(r.items()) != 0 {
		t.Fatalf("an unknown user filters to an empty page (CLI-029): %s", r.out)
	}
	for _, it := range e.run("pipeline", "list", "--project", "acme/web").items() {
		if _, has := it["duration_ms"]; has {
			t.Fatalf("list items carry the default fields only: %v", it)
		}
	}
}

// TestPipelineGetWaits: --wait reads again until the pipeline finishes, or tells
// the agent that the wait ended first; the deprecated wait does the same with its
// 1.x flags (CLI-049, CLI-081).
func TestPipelineGetWaits(t *testing.T) {
	ciFast(t)
	e := newTestEnv(t)
	e.gl.locked(func() { fakeCIState(e.gl).pipelines["acme/web"][2].FinishAfter = 3 })
	r := e.run("pipeline", "get", "203", "--project", "acme/web", "--wait", "5s")
	if r.exit != 0 || r.data()["status"] != "success" || r.data()["finished"] != true || len(ciNotices(r)) != 0 {
		t.Fatalf("--wait until it finishes: %s", r.out)
	}
	if n := len(e.gl.Requests()); n != 3 {
		t.Fatalf("three reads, got %d", n)
	}
	e.gl.locked(func() { fakeCIState(e.gl).pipelines["acme/web"][2].Status = "running" })
	r = e.run("pipeline", "get", "203", "--project", "acme/web", "--wait", "50ms")
	if r.exit != 0 || r.data()["status"] != "running" || !hasString(ciNotices(r), "wait_ended") {
		t.Fatalf("a wait that ends first: %s", r.out)
	}
	e.gl.locked(func() { fakeCIState(e.gl).pipelines["acme/web"][2].FinishAfter = 2 })
	r = e.run("pipeline", "wait", "203", "--project", "acme/web", "--timeout", "600")
	if r.exit != 0 || r.data()["status"] != "success" || !hasString(ciNotices(r), "deprecated") || !hasString(ciNotices(r), "wait_lowered") {
		t.Fatalf("the deprecated wait: %s", r.out)
	}
}

func TestPipelineRetryAndCancelAreBlocked(t *testing.T) {
	e := newTestEnv(t)
	for _, c := range []struct {
		args []string
		kind string
	}{
		{[]string{"pipeline", "retry", "201", "--project", "acme/web"}, "not_retryable"},
		{[]string{"pipeline", "retry", "203", "--project", "acme/web"}, "not_retryable"},
		{[]string{"pipeline", "cancel", "204", "--project", "acme/web"}, "not_cancelable"},
	} {
		r := e.run(append(c.args, "--dry-run")...)
		p := r.preview()
		if r.exit != 0 || p["can_apply"] != false || r.token() != "" || p["blocked_by"].([]any)[0].(map[string]any)["kind"] != c.kind {
			t.Fatalf("%v: %s", c.args, r.out)
		}
	}
	if e.gl.Writes() != 0 {
		t.Fatal("a blocked plan sent a write")
	}
}

// TestPipelineCancelRefusesDrift: the base version is the status, so a pipeline
// that finished between dry-run and confirm is not canceled (CLI-057).
func TestPipelineCancelRefusesDrift(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"pipeline", "cancel", "203", "--project", "acme/web"}
	dry := e.run(append(args, "--dry-run")...)
	if base := dry.preview()["base"].(map[string]any); base["source"] != "status" || base["version"] != "running" {
		t.Fatalf("base: %s", dry.out)
	}
	e.gl.locked(func() { fakeCIState(e.gl).pipelines["acme/web"][2].Status = "success" })
	r := e.confirm(dry.token(), args...)
	if r.code() != "E_CONFLICT" || r.details()["conflict"] != "state_drift" || e.gl.Writes() != 0 {
		t.Fatalf("drift: %s", r.out)
	}
}

// TestPipelineRetryVerifiesTheStatus: GitLab's answer must show the pipeline running
// again; one that still says failed is E_VERIFY_FAILED, not a success (CLI-059).
func TestPipelineRetryVerifiesTheStatus(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"pipeline", "retry", "204", "--project", "acme/web"}
	r := e.confirm(e.dryRun(args...), args...)
	if r.exit != 0 || r.outcome()["verification"] != "passed" || r.receipt()["observed"].(map[string]any)["status"] != "running" {
		t.Fatalf("retry: %s", r.out)
	}
	e.gl.locked(func() {
		p := fakeCIState(e.gl).pipelines["acme/web"][1]
		p.RetryKeepsStatus = true
	})
	args = []string{"pipeline", "retry", "202", "--project", "acme/web"}
	r = e.confirm(e.dryRun(args...), args...)
	if r.code() != "E_VERIFY_FAILED" || !hasString(stringsOf(r.receipt()["mismatched"]), "status") || r.outcome()["change"] != "applied" {
		t.Fatalf("a retry GitLab did not start: %s", r.out)
	}
}

// TestPipelineCreateKeepsVariableValuesSecret: pipeline variables reach GitLab, but
// their values appear in no output and no state file, in clear or as a plain
// SHA-256 (SEC-021); the preview names the keys.
func TestPipelineCreateKeepsVariableValuesSecret(t *testing.T) {
	e := newTestEnv(t)
	const secret = "sentinel-pipeline-secret-31415"
	args := []string{"pipeline", "create", "--project", "acme/web", "--ref", "main", "--variable", "DEPLOY_TOKEN=" + secret,
		"--variable", "DRY=1"}
	dry := e.run(append(args, "--dry-run")...)
	changes := dry.preview()["changes"].([]any)
	if vars := changes[1].(map[string]any); vars["field"] != "variables" || strings.Join(stringsOf(vars["after"]), ",") != "DEPLOY_TOKEN,DRY" {
		t.Fatalf("the preview names the keys: %s", dry.out)
	}
	if dry.preview()["inputs"].(map[string]any)["variable"] != "***" {
		t.Fatalf("inputs show the variables as ***: %s", dry.out)
	}
	r := e.confirm(dry.token(), args...)
	if r.exit != 0 || r.data()["result"].(map[string]any)["ref"] != "acme/web/-/pipelines/205" {
		t.Fatalf("create: %s", r.out)
	}
	var sent []any
	e.gl.locked(func() { sent = fakeCIState(e.gl).pipelines["acme/web"][4].Variables })
	if len(sent) != 2 || sent[0].(map[string]any)["value"] != secret {
		t.Fatalf("GitLab got the variables: %v", sent)
	}
	ciAssertNoSecret(t, e, secret, dry.out, dry.errOut, r.out, r.errOut, e.run("operation", "list").out)
}

// ciAssertNoSecret fails when secret, or its plain SHA-256, is in any of the outputs
// or in a file of the state directory (SEC-021).
func ciAssertNoSecret(t *testing.T, e *testEnv, secret string, outputs ...string) {
	t.Helper()
	sum := sha256.Sum256([]byte(secret))
	marks := []string{secret, hex.EncodeToString(sum[:])}
	for _, out := range outputs {
		for _, m := range marks {
			if strings.Contains(out, m) {
				t.Fatalf("an output holds the secret or its SHA-256: %s", out)
			}
		}
	}
	_ = filepath.WalkDir(e.vars["GITLAB_CLI_STATE_DIR"], func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(p)
		for _, m := range marks {
			if strings.Contains(string(b), m) {
				t.Errorf("%s holds the secret or its SHA-256", filepath.Base(p))
			}
		}
		return nil
	})
}

func TestPipelineCreateRefusedRef(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"pipeline", "create", "--project", "acme/web", "--ref", "no-such-branch"}
	r := e.confirm(e.dryRun(args...), args...)
	if r.code() != "E_VALIDATION" || r.details()["dispatch_state"] != "dispatched" || r.outcome()["change"] != "not_applied" ||
		!hasString(ciUntrusted(r), "error.details.upstream_message") {
		t.Fatalf("GitLab refuses an unknown ref: %s", r.out)
	}
	if r := e.run("pipeline", "create", "--project", "acme/nowhere", "--ref", "main", "--dry-run"); r.code() != "E_NOT_FOUND" {
		t.Fatalf("a missing project: %s", r.out)
	}
}

// TestPipelineCurrent: the latest pipeline of a branch; without flags, the current
// directory's GitLab remote, which must be on the configured host.
func TestPipelineCurrent(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("pipeline", "current", "--project", "acme/web", "--ref", "main")
	if r.exit != 0 || r.data()["ref"] != "acme/web/-/pipelines/204" || r.data()["user"] != "@carol" {
		t.Fatalf("the latest pipeline of main: %s", r.out)
	}
	r = e.run("pipeline", "current", "--project", "acme/web", "--ref", "feature/none")
	if r.code() != "E_NOT_FOUND" || !strings.Contains(r.details()["next_action"].(string), "pipeline create --project acme/web --ref feature/none") {
		t.Fatalf("a branch without pipelines: %s", r.out)
	}
	// This test's directory is a checkout whose remote, if any, is not on the fake's
	// host: the tool refuses to guess the project, and asks GitLab nothing.
	before := len(e.gl.Requests())
	r = e.run("pipeline", "current", "--ref", "main")
	if r.code() != "E_USAGE" || r.details()["parameter"] != "--project" || len(e.gl.Requests()) != before {
		t.Fatalf("a remote on another host: %s", r.out)
	}
}
