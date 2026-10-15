package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- behaviour matrix

func TestJobMatrix(t *testing.T) {
	cases := []matrixCase{
		{Path: "job get", Args: []string{"302", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--job"},
				{[]string{"acme/web/-/pipelines/204"}, "E_VALIDATION", "invalid_value", "--job"},
				{[]string{"302", "--project", "acme/web", "--wait", "soon"}, "E_USAGE", "invalid_value", "--wait"},
			}},
		{Path: "job log", Args: []string{"302", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--job"},
				{[]string{"302", "--project", "acme/web", "--tail", "-1"}, "E_USAGE", "invalid_value", "--tail"},
				{[]string{"302", "--project", "acme/web", "--max-bytes", "-5"}, "E_USAGE", "invalid_value", "--max-bytes"},
				{[]string{"302", "--project", "acme/web", "--grep", "(unclosed"}, "E_VALIDATION", "invalid_value", "--grep"},
				{[]string{"302", "--project", "acme/web", "--follow", "--max-bytes", "10"}, "E_USAGE", "conflicting_flags", "--max-bytes"},
				{[]string{"302", "--project", "acme/web", "--follow", "--wait", "5s", "--timeout", "5"}, "E_USAGE", "conflicting_flags", "--timeout"},
			}},
		{Path: "job retry", Args: []string{"302", "--project", "acme/web"}, Role: "developer",
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--job"},
			}},
		{Path: "job play", Args: []string{"304", "--project", "acme/web"}, Role: "developer",
			Invalid: []invalidCase{
				{[]string{"304", "--project", "acme/web", "--variable", "=x"}, "E_VALIDATION", "invalid_value", "--variable"},
			}},
		{Path: "job cancel", Args: []string{"305", "--project", "acme/web"}, Role: "developer",
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--job"},
			}},
		{Path: "job artifacts", Args: []string{"301", "--project", "acme/web", "--output", "artifacts.zip"},
			Invalid: []invalidCase{
				{[]string{"301", "--project", "acme/web"}, "E_USAGE", "missing_parameter", "--output"},
				{[]string{"--project", "acme/web", "--output", "a.zip"}, "E_USAGE", "missing_parameter", "--job"},
				{[]string{"x1", "--project", "acme/web", "--output", "a.zip"}, "E_VALIDATION", "invalid_value", "--job"},
			},
			// Every row's world grants artifact files in its own directory; the write
			// rows check the file, not an upstream write, so they are the job's own.
			Setup: func(t *testing.T, e *testEnv) []string {
				return []string{"301", "--project", "acme/web", "--output", filepath.Join(ciGrantArtifacts(t, e), "a.zip")}
			},
			Rows: jobArtifactsRows()},
		{Path: "job wait", Args: []string{"302", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--job"},
			}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// jobArtifactsRows are the write rows of the matrix for job artifacts, a local
// write: its scope must grant artifact and an outputs directory, and nothing is
// written upstream, so the rows check the file instead.
func jobArtifactsRows() map[string]func(t *testing.T) {
	args := func(out string, extra ...string) []string {
		return append([]string{"job", "artifacts", "301", "--project", "acme/web", "--output", out}, extra...)
	}
	noFile := func(t *testing.T, dir string) {
		t.Helper()
		if entries, _ := os.ReadDir(dir); len(entries) != 0 {
			t.Fatalf("a file was left in %s: %v", dir, entries[0].Name())
		}
	}
	return map[string]func(t *testing.T){
		"success": func(t *testing.T) {
			e, dir := ciArtifactEnv(t)
			out := filepath.Join(dir, "a.zip")
			r := e.confirm(e.dryRun(args(out)...), args(out)...)
			sum := sha256.Sum256([]byte(ciArtifacts301))
			res, _ := r.data()["result"].(map[string]any)
			if r.exit != 0 || r.outcome()["verification"] != "passed" || r.outcome()["persistence"] != "saved" ||
				r.outcome()["change"] != "not_applicable" || res["sha256"] != hex.EncodeToString(sum[:]) || res["path"] != out ||
				res["bytes"] != json.Number(strconv.Itoa(len(ciArtifacts301))) || res["ref"] != "acme/web/-/jobs/301" {
				t.Fatalf("download: %s", r.out)
			}
			checkKeys(t, "result", res, []string{"ref", "path", "bytes", "sha256"})
			if b, err := os.ReadFile(out); err != nil || string(b) != ciArtifacts301 {
				t.Fatalf("the file: %q %v", b, err)
			}
			if e.gl.Writes() != 0 {
				t.Fatal("a download wrote to GitLab")
			}
		},
		"auth and permission": func(t *testing.T) {
			e, dir := ciArtifactEnv(t)
			e.vars["GITLAB_CLI_TOKEN"] = ""
			if r := e.run(args(filepath.Join(dir, "a.zip"), "--dry-run")...); r.code() != "E_AUTH" || len(e.gl.Requests()) != 0 {
				t.Errorf("no token: %s", r.out)
			}
			e, dir = ciArtifactEnv(t)
			e.vars["GITLAB_CLI_TOKEN"] = "glpat-revoked"
			if r := e.run(args(filepath.Join(dir, "a.zip"), "--dry-run")...); r.code() != "E_AUTH" || r.details()["http_status"] != json.Number("401") {
				t.Errorf("a rejected token: %s", r.out)
			}
			e, dir = ciArtifactEnv(t)
			out := filepath.Join(dir, "a.zip")
			tok := e.dryRun(args(out)...)
			e.gl.fail(fakeFault{Method: http.MethodGet, Path: "/artifacts", Status: 403, Times: 1})
			r := e.confirm(tok, args(out)...)
			if r.code() != "E_FORBIDDEN" || r.details()["dispatch_state"] != "not_started" || r.details()["required"] != "reporter" ||
				r.outcome()["persistence"] != "unchanged" {
				t.Errorf("a 403 on the download: %s", r.out)
			}
			noFile(t, dir)
		},
		"gate": func(t *testing.T) {
			e, dir := ciArtifactEnv(t)
			out := filepath.Join(dir, "a.zip")
			dry := e.run(args(out, "--dry-run")...)
			p := dry.preview()
			if dry.token() == "" || p["target"].(map[string]any)["key"] != "acme/web/-/jobs/301" || stringsOf(p["outputs"])[0] != out ||
				dry.preview()["inputs"].(map[string]any)["output"] != out {
				t.Fatalf("the preview names the job and the file: %s", dry.out)
			}
			if r := e.confirm(dry.token(), args(out)...); r.receipt()["observed"].(map[string]any)["path"] != out {
				t.Fatalf("observed: %s", r.out)
			}
			// The scope must grant artifact, and outputs must cover the new file (SEC-012).
			e = newTestEnv(t)
			r := e.run(args(filepath.Join(dir, "b.zip"), "--dry-run")...)
			scope, _ := r.details()["scope"].(map[string]any)
			missing, _ := scope["missing"].(map[string]any)
			if r.code() != "E_FORBIDDEN" || !hasString(stringsOf(missing["effects"]), "artifact") || len(stringsOf(missing["outputs"])) != 1 {
				t.Fatalf("no artifact grant: %s", r.out)
			}
			e, _ = ciArtifactEnv(t)
			if r := e.run(args(filepath.Join(t.TempDir(), "c.zip"), "--dry-run")...); r.code() != "E_FORBIDDEN" {
				t.Fatalf("a file outside outputs: %s", r.out)
			}
		},
		"apply failure": func(t *testing.T) {
			t.Run("not sent", func(t *testing.T) {
				e, dir := ciArtifactEnv(t)
				out := filepath.Join(dir, "a.zip")
				tok := e.dryRun(args(out)...)
				e.gl.fail(fakeFault{Method: http.MethodGet, Path: "/artifacts", Status: 500})
				r := e.confirm(tok, args(out)...)
				if r.code() != "E_SERVER" || r.details()["dispatch_state"] != "not_started" || r.outcome()["persistence"] != "unchanged" {
					t.Fatalf("a failed download saves nothing: %s", r.out)
				}
				noFile(t, dir)
			})
			t.Run("file appeared", func(t *testing.T) {
				// A file that appeared since the dry-run is never overwritten: the command
				// only creates files, so the gate refuses it (artifact without persist,
				// Q9), with roots granted or not, and the file is kept.
				e, dir := ciArtifactEnv(t)
				out := filepath.Join(dir, "a.zip")
				tok := e.dryRun(args(out)...)
				if err := os.WriteFile(out, []byte("mine"), 0o600); err != nil {
					t.Fatal(err)
				}
				r := e.confirm(tok, args(out)...)
				path, _ := r.details()["path"].(string)
				if b, _ := os.ReadFile(out); r.code() != "E_VALIDATION" || !strings.HasSuffix(path, "a.zip") || string(b) != "mine" {
					t.Fatalf("a file that appeared is never overwritten: %s", r.out)
				}
				scopeFile := fullScope()
				scopeFile["effects"], scopeFile["outputs"], scopeFile["roots"] = []string{"artifact"}, []string{dir}, []string{dir}
				e.setScope(scopeFile)
				if r = e.run(args(out, "--dry-run")...); r.code() != "E_VALIDATION" {
					t.Fatalf("an existing file, roots granted: %s", r.out)
				}
			})
		},
	}
}

// ---------------------------------------------------------------- focused tests

func TestJobFormsAreOneTarget(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"job", "get", "302", "--project", "acme/web"},
		{"job", "get", "acme/web/-/jobs/302"},
		{"job", "show", e.gl.URL + "/acme/web/-/jobs/302/raw"},
		{"jobs", "view", "--job", "302", "-R", "acme/web"},
	} {
		r := e.run(args...)
		d := r.data()
		if r.exit != 0 || d["ref"] != "acme/web/-/jobs/302" || d["pipeline"] != "acme/web/-/pipelines/204" || d["user"] != "@carol" ||
			d["failure_reason"] != "script_failure" || d["duration_ms"] != json.Number("61500") {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
	r := e.run("job", "get", "301", "--project", "acme/web")
	arts, _ := r.data()["artifacts"].([]any)
	if len(arts) != 2 || arts[1].(map[string]any)["file_type"] != "archive" || r.data()["artifacts_expire_at"] != "2026-10-12T08:59:00Z" {
		t.Fatalf("artifacts are a summary of kinds and sizes: %s", r.out)
	}
	tok := e.dryRun("job", "retry", "acme/web/-/jobs/302")
	if r := e.confirm(tok, "job", "retry", "302", "--project", "acme/web"); r.exit != 0 {
		t.Fatalf("the ID and the ref must bind alike: %s", r.out)
	}
}

// TestJobLogSelections: --tail, --grep and --max-bytes choose from the end, and say
// what they left out; --format raw passes the trace's bytes through (CLI-005).
func TestJobLogSelections(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("job", "log", "302", "--project", "acme/web", "--tail", "3")
	if r.data()["log"] != "FAIL\tacme/web/checkout\t0.031s\nFAIL\nERROR: Job failed: exit code 1" || r.data()["lines"] != json.Number("3") ||
		r.data()["truncated"] != true || !strings.Contains(r.data()["truncated_reason"].(string), "--tail 3") {
		t.Fatalf("--tail 3: %s", r.out)
	}
	if r.data()["total_lines"] != json.Number(strconv.Itoa(strings.Count(ciTrace302, "\n"))) ||
		r.data()["total_bytes"] != json.Number(strconv.Itoa(len(ciTrace302))) {
		t.Fatalf("totals count the whole trace: %s", r.out)
	}
	r = e.run("job", "log", "302", "--project", "acme/web", "--grep", "FAIL|ERROR")
	if r.data()["matched_lines"] != json.Number("4") || r.data()["truncated"] != false || !strings.HasPrefix(r.data()["log"].(string), "--- FAIL") {
		t.Fatalf("--grep: %s", r.out)
	}
	r = e.run("job", "log", "302", "--project", "acme/web", "--max-bytes", "20")
	if log := r.data()["log"].(string); len(log) > 20 || !strings.HasSuffix(log, "exit code 1") || r.data()["truncated"] != true {
		t.Fatalf("--max-bytes: %s", r.out)
	}
	raw := e.run("job", "log", "302", "--project", "acme/web", "--format", "raw", "--tail", "0")
	if raw.exit != 0 || raw.out != strings.ReplaceAll(ciTrace302, "\r\n", "\n") || raw.doc != nil {
		t.Fatalf("--format raw --tail 0 is the trace as is: %q", raw.out)
	}
	raw = e.run("job", "log", "302", "--project", "acme/web", "--format", "raw", "--tail", "1")
	if raw.out != "\x1b[0;m\n" {
		t.Fatalf("raw --tail 1 is the last line's bytes: %q", raw.out)
	}
	if r := e.run("job", "log", "302", "--project", "acme/web", "--format", "raw", "--fields", "log"); r.code() != "E_USAGE" {
		t.Fatalf("--fields with raw: %s", r.out)
	}
	if r := e.run("job", "get", "302", "--project", "acme/web", "--format", "raw"); r.details()["problem"] != "unsupported_format" {
		t.Fatalf("raw is declared on job log only: %s", r.out)
	}
}

// TestJobLogFitsTheBudget: the last 100 lines of a long log do not fit one JSON
// response; the answer keeps the end that does and says so (CLI-042).
func TestJobLogFitsTheBudget(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("job", "log", "303", "--project", "acme/web") // run validates the envelope, its size included
	lines, _ := strconv.Atoi(string(r.data()["lines"].(json.Number)))
	if r.exit != 0 || lines >= 100 || lines < 30 || r.data()["truncated"] != true ||
		!strings.Contains(r.data()["truncated_reason"].(string), "--format raw") || !strings.HasSuffix(r.data()["log"].(string), "lint: 400 issues") {
		t.Fatalf("a long log: %d lines, %s", lines, r.out[:min(len(r.out), 600)])
	}
	raw := e.run("job", "log", "303", "--project", "acme/web", "--format", "raw")
	if strings.Count(raw.out, "\n") != 100 {
		t.Fatalf("raw keeps --tail's 100 lines: %d", strings.Count(raw.out, "\n"))
	}
	one := strings.Repeat("x", 9000)
	kept := jsonTail(one, 100)
	if len(kept) != 100 {
		t.Fatalf("one line longer than the budget keeps its end: %d", len(kept))
	}
}

func TestJobLogText(t *testing.T) {
	for in, want := range map[string]string{
		"section_start:1760000000:step_script\r\x1b[0K\x1b[0K\x1b[36;1mExecuting\x1b[0;m": "Executing",
		"section_end:1760000009:step_script\r\x1b[0K":                                     "",
		"plain\ttext\r":                          "plain\ttext",
		"10%\r50%\r100%":                         "100%",
		"\x1b]0;title\x07after":                  "after",
		"bell\x07 and nul\x00":                   "bell and nul",
		"invalid \xff utf-8":                     "invalid � utf-8",
		"section_start:1:name[collapsed=true]\r": "",
	} {
		if got := jobLogText([]byte(in)); got != want {
			t.Errorf("jobLogText(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestJobLogFollowStreams: --follow streams the end of the log and each new line as
// NDJSON items, then a summary that says whether the job finished (CLI-006); a
// GitLab that ignores Range repeats nothing.
func TestJobLogFollowStreams(t *testing.T) {
	ciFast(t)
	for _, ignoreRange := range []bool{false, true} {
		e := newTestEnv(t)
		e.gl.locked(func() {
			ci := fakeCIState(e.gl)
			ci.ignoreRange = ignoreRange
			ci.jobs["acme/web"][4].Grow = []string{"Compiling 3/3\nLink", "ing\n", "Done\n"}
		})
		r := e.run("job", "log", "305", "--project", "acme/web", "--follow", "--wait", "5s")
		out, exit := r.out, r.exit
		if exit != 0 {
			t.Fatalf("exit %d: %s", exit, out)
		}
		docs := strings.Split(strings.TrimSpace(out), "\n")
		var lines []string
		for _, d := range docs[:len(docs)-1] {
			var item struct {
				Type string `json:"type"`
				Data struct {
					Line string `json:"line"`
				} `json:"data"`
				Meta struct {
					Untrusted []string `json:"untrusted"`
				} `json:"meta"`
			}
			_ = json.Unmarshal([]byte(d), &item)
			if item.Type != "item" || !hasString(item.Meta.Untrusted, "data.line") {
				t.Fatalf("an item line: %s", d)
			}
			lines = append(lines, item.Data.Line)
		}
		if strings.Join(lines, "|") != "Compiling 1/3|Compiling 2/3|Compiling 3/3|Linking|Done" {
			t.Fatalf("range ignored %v: the lines %q", ignoreRange, lines)
		}
		var summary map[string]any
		_ = json.Unmarshal([]byte(docs[len(docs)-1]), &summary)
		data, _ := summary["data"].(map[string]any)
		if summary["type"] != "summary" || data["finished"] != true || data["status"] != "success" || data["count"] != float64(5) ||
			data["has_more"] != false {
			t.Fatalf("summary: %s", docs[len(docs)-1])
		}
	}
	e := newTestEnv(t)
	out := e.run("job", "log", "305", "--project", "acme/web", "--follow", "--wait", "50ms", "--tail", "1").out
	docs := strings.Split(strings.TrimSpace(out), "\n")
	var summary map[string]any
	_ = json.Unmarshal([]byte(docs[len(docs)-1]), &summary)
	data, _ := summary["data"].(map[string]any)
	if len(docs) != 2 || data["finished"] != false || data["has_more"] != true || data["continuation_reason"] == nil {
		t.Fatalf("a wait that ends first: %s", out)
	}
	// GitLab gone after the stream started: the stream ends with an error summary
	// (CLI-006), and the exit code is the summary's.
	e = newTestEnv(t)
	e.gl.failAfterReads(2)
	r := e.run("job", "log", "305", "--project", "acme/web", "--follow", "--wait", "5s")
	out, exit := r.out, r.exit
	docs = strings.Split(strings.TrimSpace(out), "\n")
	if exit != 7 || len(docs) != 3 || !strings.Contains(docs[2], `"type":"summary"`) || !strings.Contains(docs[2], `"E_NETWORK"`) {
		t.Fatalf("a failed read: exit %d %s", exit, out)
	}
}

// TestJobLogWaitsForTheEnd: --wait without --follow waits for the job, then shows
// the end of its log, in one call (CLI-049).
func TestJobLogWaitsForTheEnd(t *testing.T) {
	ciFast(t)
	e := newTestEnv(t)
	e.gl.locked(func() { fakeCIState(e.gl).jobs["acme/web"][4].FinishAfter = 2 })
	r := e.run("job", "log", "305", "--project", "acme/web", "--wait", "5s")
	if r.exit != 0 || r.data()["finished"] != true || r.data()["status"] != "success" || r.data()["log"] != "Compiling 1/3\nCompiling 2/3" {
		t.Fatalf("job log --wait: %s", r.out)
	}
	e.gl.locked(func() { fakeCIState(e.gl).jobs["acme/web"][4].FinishAfter = 2 })
	r = e.run("job", "wait", "305", "--project", "acme/web")
	if r.exit != 0 || !hasString(ciNotices(r), "deprecated") {
		t.Fatalf("the deprecated job wait: %s", r.out)
	}
}

func TestJobActionsAreBlocked(t *testing.T) {
	e := newTestEnv(t)
	for _, c := range []struct {
		args []string
		kind string
	}{
		{[]string{"job", "retry", "304", "--project", "acme/web"}, "manual_job"},
		{[]string{"job", "retry", "305", "--project", "acme/web"}, "not_finished"},
		{[]string{"job", "play", "302", "--project", "acme/web"}, "not_manual"},
		{[]string{"job", "play", "305", "--project", "acme/web"}, "not_manual"},
		{[]string{"job", "cancel", "301", "--project", "acme/web"}, "not_cancelable"},
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

// TestJobRetryReturnsTheNewJob: GitLab retries a job as a new one; the result is
// that job, and its ref is observed for operation show (CLI-059).
func TestJobRetryReturnsTheNewJob(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"job", "retry", "302", "--project", "acme/web"}
	r := e.confirm(e.dryRun(args...), args...)
	if r.exit != 0 || r.data()["result"].(map[string]any)["ref"] != "acme/web/-/jobs/308" ||
		r.receipt()["observed"].(map[string]any)["ref"] != "acme/web/-/jobs/308" || r.receipt()["target"].(map[string]any)["key"] != "acme/web/-/jobs/302" {
		t.Fatalf("retry: %s", r.out)
	}
}

// TestJobPlayKeepsVariableValuesSecret: job variables reach GitLab, their values no
// output or state file (SEC-021).
func TestJobPlayKeepsVariableValuesSecret(t *testing.T) {
	e := newTestEnv(t)
	const secret = "sentinel-job-secret-27182"
	args := []string{"job", "play", "304", "--project", "acme/web", "--variable", "TOKEN=" + secret}
	dry := e.run(append(args, "--dry-run")...)
	r := e.confirm(dry.token(), args...)
	if r.exit != 0 || r.data()["result"].(map[string]any)["status"] != "pending" {
		t.Fatalf("play: %s", r.out)
	}
	var sent []any
	e.gl.locked(func() { sent = fakeCIState(e.gl).jobs["acme/web"][3].PlayVariables })
	if len(sent) != 1 || sent[0].(map[string]any)["value"] != secret {
		t.Fatalf("GitLab got the variable: %v", sent)
	}
	ciAssertNoSecret(t, e, secret, dry.out, dry.errOut, r.out, r.errOut)
}

// TestJobArtifactsNeverOverwrites: an existing path gets no token (the gate's
// E_VALIDATION for a command that only creates files, Q9, or the plan's
// is_directory block), and the plan is blocked when the job has no archive or the
// directory is missing; nothing is written either way.
func TestJobArtifactsNeverOverwrites(t *testing.T) {
	e, dir := ciArtifactEnv(t)
	taken := filepath.Join(dir, "taken.zip")
	if err := os.WriteFile(taken, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if r := e.run("job", "artifacts", "301", "--project", "acme/web", "--output", taken, "--dry-run"); r.code() != "E_VALIDATION" {
		t.Fatalf("an existing file: %s", r.out)
	}
	if r := e.run("job", "artifacts", "301", "--project", "acme/web", "--output", dir, "--dry-run"); r.token() != "" {
		t.Fatalf("a directory: %s", r.out)
	}
	if b, _ := os.ReadFile(taken); string(b) != "mine" {
		t.Fatal("the existing file changed")
	}
	for _, c := range []struct {
		job, out, kind string
	}{
		{"302", filepath.Join(dir, "new.zip"), "no_artifacts"},
		{"301", filepath.Join(dir, "missing", "a.zip"), "no_directory"},
	} {
		r := e.run("job", "artifacts", c.job, "--project", "acme/web", "--output", c.out, "--dry-run")
		if r.exit != 0 || r.preview()["can_apply"] != false || r.preview()["blocked_by"].([]any)[0].(map[string]any)["kind"] != c.kind {
			t.Fatalf("%s %s: %s", c.job, c.out, r.out)
		}
	}
	// A relative --output binds as the absolute path the preview shows (CLI-054).
	t.Chdir(dir)
	r := e.run("job", "artifacts", "301", "--project", "acme/web", "--output", "rel.zip", "--dry-run")
	if got := stringsOf(r.preview()["outputs"]); r.token() == "" || len(got) != 1 || !filepath.IsAbs(got[0]) {
		t.Fatalf("a relative path: %s", r.out)
	}
	if e.gl.Writes() != 0 {
		t.Fatal("a download wrote to GitLab")
	}
}
