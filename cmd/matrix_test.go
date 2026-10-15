package cmd

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The behaviour matrix (VER-004): every command at experimental maturity or above
// runs each row below against the fake GitLab, as a subtest named after the row. A
// row that does not apply is skipped with a reason that starts "not applicable:".
// A group describes each of its commands with one matrixCase and calls runMatrix;
// Rows replaces a row with a command-specific test.
//
// A batch command (Meta.BatchFlag: reference lists E_PARTIAL for it) answers with
// one receipt per item and a summary (CLI-063), so its success, auth, gate and
// apply-failure rows are the batch rows at the end of this file.
var matrixRows = []string{"success", "invalid input", "auth and permission", "upstream failure", "empty",
	"pagination", "exit code", "no side effect", "gate", "apply failure"}

// matrixCase describes one command.
type matrixCase struct {
	Path string   // the command path, "label update"
	Args []string // a valid invocation after the path (a write: without gate flags)
	// Setup, when set, prepares the test world of every row (a scope that grants an
	// artifact or deliver effect, a file the command reads) and returns the
	// invocation to use instead of Args.
	Setup func(t *testing.T, e *testEnv) []string
	// Invalid are invocations rejected before any request reaches GitLab.
	Invalid []invalidCase
	// EmptyArgs replaces Args in the empty row of a list (a project with no items).
	EmptyArgs []string
	// Role is details.required on a 403 from the write itself (CLI-014).
	Role string
	// RequestFields maps preview change fields to the JSON body fields Apply sends:
	// the gate row checks each change's after against the request GitLab received.
	RequestFields map[string]string
	// PreviewReads is the number of GETs the confirm makes before its first write
	// (default 1); the "failure before sending" row lets them through.
	PreviewReads int
	// RefusedStatus is the status GitLab refuses the write with (default 422).
	RefusedStatus int
	// OneRequest marks a batch whose items go out in one request (a native bulk
	// endpoint, CLI-063): every item shares that request's outcome.
	OneRequest bool
	// Skip maps a row to the reason it does not apply.
	Skip map[string]string
	// Rows replaces rows with command-specific tests.
	Rows map[string]func(t *testing.T)
}

// invalidCase is one invocation that must fail before anything is sent.
type invalidCase struct {
	Args               []string // after the command path
	Code               string   // E_USAGE or E_VALIDATION
	Problem, Parameter string
}

// commandInfo is what reference says about a command.
type commandInfo struct {
	write, dangerous, batch bool
	shape                   string
	fields, defaults        []string
}

// matrixWorld builds a row's test world: a fresh testEnv, prepared by
// matrixCase.Setup, and the command line for it (with --dangerous when the
// command needs it).
type matrixWorld func(t *testing.T) (*testEnv, func(extra ...string) []string)

func referenceOf(t *testing.T, path string) commandInfo {
	t.Helper()
	e := newTestEnv(t)
	r := e.run("reference", "--command", path)
	if r.exit != 0 {
		t.Fatalf("reference --command %q: %s", path, r.out)
	}
	cmd, _ := r.data()["command"].(map[string]any)
	info := commandInfo{write: cmd["type"] == "write", dangerous: cmd["dangerous"] == true,
		batch: hasString(stringsOf(cmd["error_codes"]), "E_PARTIAL")}
	schemas, _ := r.data()["schemas"].(map[string]any)
	if s, ok := schemas[cmd["output"].(string)].(map[string]any); ok {
		info.shape, _ = s["shape"].(string)
		info.fields, info.defaults = stringsOf(s["fields"]), stringsOf(s["default_fields"])
	}
	return info
}

// runMatrix runs the behaviour matrix of one command.
func runMatrix(t *testing.T, m matrixCase) {
	info := referenceOf(t, m.Path)
	words := strings.Fields(m.Path)
	world := func(t *testing.T) (*testEnv, func(...string) []string) {
		e := newTestEnv(t)
		args := m.Args
		if m.Setup != nil {
			args = m.Setup(t, e)
		}
		return e, func(extra ...string) []string {
			out := append(append(append([]string{}, words...), args...), extra...)
			if info.dangerous {
				out = append(out, "--dangerous")
			}
			return out
		}
	}
	batch := info.write && info.batch
	for _, row := range matrixRows {
		t.Run(row, func(t *testing.T) {
			if fn, ok := m.Rows[row]; ok {
				fn(t)
				return
			}
			if reason, ok := m.Skip[row]; ok {
				t.Skip("not applicable: " + reason)
			}
			switch row {
			case "success":
				if batch {
					matrixBatchSuccess(t, info, world)
				} else {
					matrixSuccess(t, info, world)
				}
			case "invalid input":
				if len(m.Invalid) == 0 {
					t.Skip("not applicable: the command takes no parameters")
				}
				e, _ := world(t)
				for _, c := range m.Invalid {
					r := e.run(append(append([]string{}, words...), c.Args...)...)
					if r.code() != c.Code || r.details()["problem"] != c.Problem || r.details()["parameter"] != c.Parameter {
						t.Errorf("%v: want %s %s %s, got %s", c.Args, c.Code, c.Problem, c.Parameter, r.out)
					}
				}
				if n := len(e.gl.Requests()); n != 0 {
					t.Errorf("invalid input sent %d request(s) to GitLab", n)
				}
			case "auth and permission":
				if batch {
					matrixBatchAuth(t, m, world)
				} else {
					matrixAuth(t, m, info, world)
				}
			case "upstream failure":
				matrixUpstream(t, info, world)
			case "empty":
				if info.shape != "page" {
					t.Skip("not applicable: not a list")
				}
				e, _ := world(t)
				r := e.run(append(append([]string{}, words...), m.EmptyArgs...)...)
				if r.exit != 0 || len(r.items()) != 0 || r.data()["count"] != json.Number("0") || r.data()["has_more"] != false {
					t.Fatalf("empty list: %s", r.out)
				}
				if scope, _ := r.data()["scope"].(map[string]any); len(scope) == 0 {
					t.Fatalf("an empty list says what it covered (scope): %s", r.out)
				}
			case "pagination":
				if info.shape != "page" {
					t.Skip("not applicable: not a list")
				}
				matrixPagination(t, world)
			case "exit code":
				// run checks every envelope against contract.json; here one success
				// and one retryable failure.
				e, line := world(t)
				step := line()
				if info.write {
					step = line("--dry-run")
				}
				if r := e.run(step...); r.exit != 0 {
					t.Errorf("success: %s", r.out)
				}
				e.gl.fail(fakeFault{Status: 503, Header: map[string]string{"Retry-After": "11"}})
				if r := e.run(step...); r.code() != "E_SERVER" || r.exit != 7 {
					t.Errorf("a 503 (not retried: Retry-After over 10 s): %s", r.out)
				}
			case "no side effect":
				if info.write {
					t.Skip("not applicable: a write")
				}
				e, line := world(t)
				before := testKeyring.Writes()
				if r := e.run(line()...); r.exit != 0 {
					t.Fatalf("%s", r.out)
				}
				if e.gl.Writes() != 0 || testKeyring.Writes() != before {
					t.Fatalf("a read sent %d write(s) to GitLab and %d to the keyring", e.gl.Writes(), testKeyring.Writes()-before)
				}
			case "gate":
				if !info.write {
					t.Skip("not applicable: a read")
				}
				if batch {
					matrixBatchGate(t, m, world)
				} else {
					matrixGate(t, m, world)
				}
			case "apply failure":
				if !info.write {
					t.Skip("not applicable: a read")
				}
				if batch {
					matrixBatchApplyFailure(t, m, info, world)
				} else {
					matrixApplyFailure(t, m, world)
				}
			}
		})
	}
}

func matrixSuccess(t *testing.T, info commandInfo, world matrixWorld) {
	e, line := world(t)
	if info.write {
		tok := e.dryRun(line()...)
		r := e.confirm(tok, line()...)
		if r.exit != 0 {
			t.Fatalf("confirm: %s", r.out)
		}
		if v := r.outcome()["verification"]; v != "passed" {
			t.Errorf("verification %v; observed %v", v, r.receipt()["observed"])
		}
		if res, ok := r.data()["result"].(map[string]any); ok && len(info.fields) > 0 {
			checkKeys(t, "result", res, info.fields)
		}
		return
	}
	r := e.run(line()...)
	if r.exit != 0 {
		t.Fatalf("%s", r.out)
	}
	switch info.shape {
	case "page":
		items := r.items()
		if len(items) == 0 {
			t.Fatalf("the success row needs items: %s", r.out)
		}
		for _, it := range items {
			checkKeys(t, "item", it, info.fields)
			for _, f := range info.defaults {
				if _, ok := it[f]; !ok {
					t.Errorf("default field %q missing: %v", f, it)
				}
			}
		}
		for _, it := range e.run(line("--fields", "ref")...).items() {
			if len(it) != 1 || it["ref"] == nil {
				t.Errorf("--fields ref must project items to ref: %v", it)
			}
		}
	default:
		checkKeys(t, "data", r.data(), info.fields)
		field := "ref"
		if !hasString(info.fields, "ref") && len(info.fields) > 0 {
			field = info.fields[0] // a command whose output is no resource (auth status)
		}
		if p := e.run(line("--fields", field)...).data(); len(p) != 1 || p[field] == nil {
			t.Errorf("--fields %s must project to %s: %v", field, field, p)
		}
	}
}

// checkKeys asserts that obj only has keys of its schema.
func checkKeys(t *testing.T, what string, obj map[string]any, fields []string) {
	t.Helper()
	if len(fields) == 0 {
		return
	}
	for k := range obj {
		if !hasString(fields, k) {
			t.Errorf("%s key %q is not a field of the output schema %v", what, k, fields)
		}
	}
}

func matrixAuth(t *testing.T, m matrixCase, info commandInfo, world matrixWorld) {
	step := func(line func(...string) []string) []string {
		if info.write {
			return line("--dry-run")
		}
		return line()
	}
	e, line := world(t)
	e.vars["GITLAB_CLI_TOKEN"] = ""
	if r := e.run(step(line)...); r.code() != "E_AUTH" || len(e.gl.Requests()) != 0 {
		t.Errorf("no token: want E_AUTH and no request, got %s (%d requests)", r.out, len(e.gl.Requests()))
	}
	e, line = world(t)
	e.vars["GITLAB_CLI_TOKEN"] = "glpat-revoked"
	if r := e.run(step(line)...); r.code() != "E_AUTH" || r.details()["http_status"] != json.Number("401") {
		t.Errorf("a rejected token: want E_AUTH from a 401, got %s", r.out)
	}
	e, line = world(t)
	if !info.write {
		e.gl.fail(fakeFault{Method: http.MethodGet, Status: 403})
		if r := e.run(step(line)...); r.code() != "E_FORBIDDEN" || r.details()["dispatch_state"] != "not_started" {
			t.Errorf("a 403: want E_FORBIDDEN, got %s", r.out)
		}
		return
	}
	tok := e.dryRun(line()...)
	e.gl.fail(fakeFault{Method: "WRITE", Status: 403, Times: 1})
	r := e.confirm(tok, line()...)
	if r.code() != "E_FORBIDDEN" || r.details()["dispatch_state"] != "dispatched" || r.outcome()["change"] != "not_applied" {
		t.Errorf("a 403 on the write: want E_FORBIDDEN dispatched not_applied, got %s", r.out)
	}
	if m.Role != "" && r.details()["required"] != m.Role {
		t.Errorf("a 403 on the write names the role (CLI-014): want required %q, got %v", m.Role, r.details()["required"])
	}
}

func matrixUpstream(t *testing.T, info commandInfo, world matrixWorld) {
	cases := []struct {
		name  string
		fault *fakeFault
		setup func(e *testEnv)
		code  string
	}{
		{"5xx", &fakeFault{Method: http.MethodGet, Status: 500}, nil, "E_SERVER"},
		{"429", &fakeFault{Method: http.MethodGet, Status: 429, Header: map[string]string{"Retry-After": "11"}}, nil, "E_RATE_LIMITED"},
		{"timeout", &fakeFault{Method: http.MethodGet, Delay: 2 * time.Second}, func(e *testEnv) { e.vars["GITLAB_CLI_TIMEOUT"] = "200ms" }, "E_TIMEOUT"},
		{"refused", nil, func(e *testEnv) { e.gl.down() }, "E_NETWORK"},
	}
	for _, c := range cases {
		e, line := world(t)
		step := line()
		if info.write {
			step = line("--dry-run")
		}
		if c.fault != nil {
			e.gl.fail(*c.fault)
		}
		if c.setup != nil {
			c.setup(e)
		}
		r := e.run(step...)
		if r.code() != c.code || r.details()["dispatch_state"] != "not_started" {
			t.Errorf("%s: want %s not_started, got %s", c.name, c.code, r.out)
		}
		if c.name == "429" && r.details()["retry_after_ms"] != json.Number("11000") {
			t.Errorf("429: retry_after_ms from Retry-After: %s", r.out)
		}
		if e.gl.Writes() != 0 {
			t.Errorf("%s: a failed read or preview sent a write", c.name)
		}
	}
}

func matrixPagination(t *testing.T, world matrixWorld) {
	e, line := world(t)
	first := e.run(line("--limit", "1")...)
	if first.exit != 0 || first.data()["has_more"] != true || first.data()["next_offset"] != json.Number("1") || len(first.items()) != 1 {
		t.Fatalf("first page of one: %s", first.out)
	}
	offset, seen := "1", map[any]bool{first.items()[0]["ref"]: true}
	for i := 0; i < 50; i++ {
		page := e.run(line("--limit", "1", "--offset", offset)...)
		if page.exit != 0 || len(page.items()) != 1 {
			t.Fatalf("page at %s: %s", offset, page.out)
		}
		ref := page.items()[0]["ref"]
		if seen[ref] {
			t.Fatalf("page at %s repeats %v", offset, ref)
		}
		seen[ref] = true
		if page.data()["has_more"] == false {
			if page.data()["next_offset"] != nil {
				t.Fatalf("the last page has no next_offset: %s", page.out)
			}
			return
		}
		offset = string(page.data()["next_offset"].(json.Number))
	}
	t.Fatal("the list never ended")
}

func matrixGate(t *testing.T, m matrixCase, world matrixWorld) {
	e, line := world(t)
	dry := e.run(line("--dry-run")...)
	tok := dry.token()
	if tok == "" {
		t.Fatalf("dry-run: %s", dry.out)
	}
	r := e.confirm(tok, line()...)
	if r.exit != 0 {
		t.Fatalf("confirm: %s", r.out)
	}
	if obs, _ := r.receipt()["observed"].(map[string]any); len(obs) == 0 {
		t.Errorf("observed must come from a read-back: %s", r.out)
	}
	checkRequestFields(t, e, m, listOf(dry.preview()["changes"]))
}

// checkRequestFields compares the preview's changes with the last write GitLab
// received, for the fields RequestFields maps (the gate row).
func checkRequestFields(t *testing.T, e *testEnv, m matrixCase, changes []any) {
	t.Helper()
	if len(m.RequestFields) == 0 {
		return
	}
	req, ok := e.gl.lastWrite()
	if !ok {
		t.Fatal("no write reached GitLab")
	}
	for _, raw := range changes {
		ch, _ := raw.(map[string]any)
		field, _ := ch["field"].(string)
		bodyField, mapped := m.RequestFields[field]
		after, has := ch["after"]
		if !mapped || !has {
			continue
		}
		if got := req.Body[bodyField]; !reflect.DeepEqual(normalizeJSON(got), normalizeJSON(after)) {
			t.Errorf("preview %s after %v, but Apply sent %s=%v", field, after, bodyField, got)
		}
	}
}

// normalizeJSON makes numbers comparable across json.Number and float64.
func normalizeJSON(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

func matrixApplyFailure(t *testing.T, m matrixCase, world matrixWorld) {
	reads := m.PreviewReads
	if reads == 0 {
		reads = 1
	}
	t.Run("not sent", func(t *testing.T) {
		e, line := world(t)
		tok := e.dryRun(line()...)
		e.gl.failAfterReads(reads)
		r := e.confirm(tok, line()...)
		if r.code() != "E_NETWORK" || r.details()["dispatch_state"] != "not_started" || r.outcome()["change"] != "not_applied" {
			t.Fatalf("want E_NETWORK not_started not_applied, got %s", r.out)
		}
		if e.gl.Writes() != 0 {
			t.Fatal("a write reached GitLab")
		}
	})
	t.Run("refused", func(t *testing.T) {
		status := m.RefusedStatus
		if status == 0 {
			status = 422
		}
		e, line := world(t)
		tok := e.dryRun(line()...)
		e.gl.fail(fakeFault{Method: "WRITE", Status: status, Body: `{"message":{"name":["is invalid"]}}`, Times: 1})
		r := e.confirm(tok, line()...)
		if r.details()["dispatch_state"] != "dispatched" || r.outcome()["change"] != "not_applied" || r.outcome()["execution"] != "failed" {
			t.Fatalf("want dispatched, failed, not_applied, got %s", r.out)
		}
		if !hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "error.details.upstream_message") {
			t.Errorf("GitLab's text is untrusted: %s", r.out)
		}
		if e.gl.Writes() != 1 {
			t.Fatalf("sent %d writes, want 1", e.gl.Writes())
		}
	})
	t.Run("unknown", func(t *testing.T) {
		e, line := world(t)
		tok := e.dryRun(line()...)
		e.gl.fail(fakeFault{Method: "WRITE", Status: 502, Times: 1})
		r := e.confirm(tok, line()...)
		id, _ := r.details()["operation_id"].(string)
		if r.code() != "E_OUTCOME_UNKNOWN" || r.exit != 6 || id == "" {
			t.Fatalf("a 5xx after sending is an unknown outcome: %s", r.out)
		}
		if show := e.run("operation", "show", "--id", id); show.data()["status"] != "unknown" {
			t.Fatalf("operation show: %s", show.out)
		}
		if again := e.run(line("--dry-run")...); again.details()["conflict"] != "unresolved_operation" {
			t.Fatalf("the target stays blocked until resolved: %s", again.out)
		}
		if e.gl.Writes() != 1 {
			t.Fatalf("the write was sent %d times, want once", e.gl.Writes())
		}
	})
}

// ---------------------------------------------------------------- batch rows

// matrixBatchSuccess: one preview and one token for every target; at confirm every
// item is applied and verified, and the summary counts them (CLI-063).
func matrixBatchSuccess(t *testing.T, info commandInfo, world matrixWorld) {
	e, line := world(t)
	dry := e.run(line("--dry-run")...)
	p := dry.preview()
	total := jsonInt(p["total"])
	if dry.exit != 0 || dry.token() == "" || total == 0 || len(listOf(p["targets"])) != total || len(listOf(p["items"])) != total {
		t.Fatalf("dry-run: one preview naming every target: %s", dry.out)
	}
	r := e.confirm(dry.token(), line()...)
	items := r.batchItems()
	if r.exit != 0 || len(items) != total || jsonInt(r.summary()["succeeded"]) != total {
		t.Fatalf("confirm: %s", r.out)
	}
	for _, it := range items {
		if it["ok"] != true || itemOutcome(it)["verification"] != "passed" {
			t.Errorf("item %v: %v", it["target"], it["receipt"])
		}
		if res, ok := it["result"].(map[string]any); ok {
			checkKeys(t, "result", res, info.fields)
		}
	}
}

// matrixBatchAuth: no token and a rejected token send nothing; a 403 on the writes
// applies nothing, item by item, and names the role.
func matrixBatchAuth(t *testing.T, m matrixCase, world matrixWorld) {
	e, line := world(t)
	e.vars["GITLAB_CLI_TOKEN"] = ""
	if r := e.run(line("--dry-run")...); r.code() != "E_AUTH" || len(e.gl.Requests()) != 0 {
		t.Errorf("no token: want E_AUTH and no request, got %s", r.out)
	}
	e, line = world(t)
	e.vars["GITLAB_CLI_TOKEN"] = "glpat-revoked"
	if r := e.run(line("--dry-run")...); r.code() != "E_AUTH" || r.details()["http_status"] != json.Number("401") {
		t.Errorf("a rejected token: want E_AUTH from a 401, got %s", r.out)
	}
	e, line = world(t)
	tok := e.dryRun(line()...)
	e.gl.fail(fakeFault{Method: "WRITE", Status: 403})
	r := e.confirm(tok, line()...)
	if r.code() != "E_FORBIDDEN" || r.details()["dispatch_state"] != "dispatched" || jsonInt(r.summary()["succeeded"]) != 0 {
		t.Errorf("a 403 on the writes: want E_FORBIDDEN dispatched, nothing applied, got %s", r.out)
	}
	if m.Role != "" && r.details()["required"] != m.Role {
		t.Errorf("a 403 names the role (CLI-014): want required %q, got %v", m.Role, r.details()["required"])
	}
	for _, it := range r.batchItems() {
		if it["ok"] == true || itemOutcome(it)["change"] == "applied" {
			t.Errorf("item %v: %v", it["target"], it)
		}
	}
}

// matrixBatchGate: every planned item can apply and has changes; every item's
// receipt observed a read-back; the one token is used once.
func matrixBatchGate(t *testing.T, m matrixCase, world matrixWorld) {
	e, line := world(t)
	dry := e.run(line("--dry-run")...)
	items := listOf(dry.preview()["items"])
	if dry.token() == "" || len(items) == 0 {
		t.Fatalf("dry-run: %s", dry.out)
	}
	for _, raw := range items {
		it, _ := raw.(map[string]any)
		if it["can_apply"] != true || len(listOf(it["changes"])) == 0 {
			t.Errorf("preview item: %v", it)
		}
	}
	r := e.confirm(dry.token(), line()...)
	if r.exit != 0 {
		t.Fatalf("confirm: %s", r.out)
	}
	for _, it := range r.batchItems() {
		rc, _ := it["receipt"].(map[string]any)
		if obs, _ := rc["observed"].(map[string]any); len(obs) == 0 {
			t.Errorf("item %v: observed must come from a read-back", it["target"])
		}
	}
	last, _ := items[len(items)-1].(map[string]any)
	checkRequestFields(t, e, m, listOf(last["changes"]))
	if again := e.confirm(dry.token(), line()...); again.details()["conflict"] != "token_used" {
		t.Errorf("one token for the whole batch, used once: %s", again.out)
	}
}

// matrixBatchApplyFailure: nothing sent when GitLab is gone before the first write;
// a refused item fails alone (E_PARTIAL), or stops a dangerous batch, or fails
// every item of a one-request batch; a lost answer is an unknown outcome that
// blocks its target.
func matrixBatchApplyFailure(t *testing.T, m matrixCase, info commandInfo, world matrixWorld) {
	reads := m.PreviewReads
	if reads == 0 {
		reads = 1
	}
	status := m.RefusedStatus
	if status == 0 {
		status = 422
	}
	refusal := fakeFault{Method: "WRITE", Status: status, Body: `{"message":{"base":["is invalid"]}}`, Times: 1}
	t.Run("not sent", func(t *testing.T) {
		e, line := world(t)
		tok := e.dryRun(line()...)
		e.gl.failAfterReads(reads)
		r := e.confirm(tok, line()...)
		if r.code() != "E_NETWORK" || r.details()["dispatch_state"] != "not_started" || e.gl.Writes() != 0 {
			t.Fatalf("want E_NETWORK not_started and nothing sent, got %s", r.out)
		}
	})
	t.Run("refused", func(t *testing.T) {
		e, line := world(t)
		tok := e.dryRun(line()...)
		e.gl.fail(refusal)
		r := e.confirm(tok, line()...)
		items := r.batchItems()
		if len(items) == 0 || items[0]["ok"] != false || itemOutcome(items[0])["change"] != "not_applied" ||
			itemOutcome(items[0])["execution"] != "failed" {
			t.Fatalf("the refused item: %s", r.out)
		}
		first, _ := items[0]["error"].(map[string]any)
		want := "E_PARTIAL" // the other items went on
		if info.dangerous || m.OneRequest {
			want, _ = first["code"].(string) // a dangerous batch stops; one request fails every item
			if e.gl.Writes() != 1 {
				t.Errorf("sent %d writes, want 1", e.gl.Writes())
			}
		}
		if r.code() != want || r.details()["dispatch_state"] != "dispatched" {
			t.Fatalf("want %s dispatched: %s", want, r.out)
		}
	})
	t.Run("partial", func(t *testing.T) {
		if m.OneRequest {
			t.Skip("not applicable: one request applies every item or none")
		}
		e, line := world(t)
		tok := e.dryRun(line("--continue-on-error")...)
		e.gl.fail(fakeFault{Method: "WRITE", Times: 1}) // the first write goes through
		e.gl.fail(refusal)
		r := e.confirm(tok, line("--continue-on-error")...)
		items := r.batchItems()
		if r.code() != "E_PARTIAL" || len(items) < 2 || items[0]["ok"] != true || items[1]["ok"] != false ||
			jsonInt(r.summary()["failed"]) != 1 {
			t.Fatalf("one item refused: want E_PARTIAL, got %s", r.out)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		e, line := world(t)
		tok := e.dryRun(line()...)
		e.gl.fail(fakeFault{Method: "WRITE", Status: 502, Times: 1})
		r := e.confirm(tok, line()...)
		id, _ := r.details()["operation_id"].(string)
		if r.code() != "E_OUTCOME_UNKNOWN" || r.exit != 6 || !strings.HasPrefix(id, "opb_") || jsonInt(r.summary()["unknown"]) == 0 {
			t.Fatalf("a 5xx after sending is an unknown outcome: %s", r.out)
		}
		if again := e.run(line("--dry-run")...); again.details()["conflict"] != "unresolved_operation" {
			t.Fatalf("the unknown item's target stays blocked: %s", again.out)
		}
	})
}

// jsonInt is a JSON number as an int (0 when absent).
func jsonInt(v any) int {
	n, _ := v.(json.Number)
	i, _ := strconv.Atoi(string(n))
	return i
}
