package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
)

// testEnv is one test's world: a fake GitLab with its seed data, a private state
// directory, a scope granting every write, an empty keyring, and the only
// environment the App sees (vars; nothing comes from the process environment).
// Every run validates its envelope and checks the exit code and retryable against
// contract.json (VER-002, VER-004 "exit code").
type testEnv struct {
	t     *testing.T
	gl    *fakeGitLab
	dir   string
	vars  map[string]string
	stdin string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	testKeyring.reset()
	gl := newFakeGitLab(t)
	dir := t.TempDir()
	e := &testEnv{t: t, gl: gl, dir: dir, vars: baseVars(gl.URL, filepath.Join(dir, "state"))}
	e.setScope(fullScope())
	return e
}

// baseVars is the environment of a test App: the fake GitLab, carol's token, a
// short timeout, the coverage recorder.
func baseVars(host, stateDir string) map[string]string {
	return map[string]string{
		"GITLAB_CLI_HOST": host, "GITLAB_CLI_TOKEN": testToken, "GITLAB_CLI_TIMEOUT": "5s",
		"GITLAB_CLI_STATE_DIR": stateDir, "GITLAB_CLI_COVERAGE_FILE": coverageFile,
	}
}

// newTestApp is NewApp for tests: the environment from vars only, the release
// source on releaseAPI, no Skill directories.
func newTestApp(vars map[string]string) *aicli.App {
	app := NewApp(func(k string) string { return vars[k] })
	app.Update.APIBase = releaseAPI.URL
	app.SkillDirs = []string{}
	return app
}

// fullScope grants every effect the commands declare, dangerous included, on every
// target (SEC-010).
func fullScope() map[string]any {
	return map[string]any{"scope_id": "scp_tests", "tool": "gitlab-cli", "granted_by": "gitlab-cli tests",
		"granted_at": "2026-10-12T00:00:00Z", "expires_at": nil, "effects": []string{"change", "configure"}, "dangerous": true}
}

// setScope writes the scope file; nil removes the grant.
func (e *testEnv) setScope(scope map[string]any) {
	e.t.Helper()
	if scope == nil {
		e.vars["GITLAB_CLI_SCOPE_FILE"] = filepath.Join(e.dir, "no-scope.json")
		return
	}
	b, _ := json.Marshal(scope)
	path := filepath.Join(e.dir, "scope.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		e.t.Fatal(err)
	}
	e.vars["GITLAB_CLI_SCOPE_FILE"] = path
}

// writeSettings writes the settings file the App reads (<state dir>/settings.json).
func (e *testEnv) writeSettings(content string) string {
	e.t.Helper()
	path := filepath.Join(e.vars["GITLAB_CLI_STATE_DIR"], "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		e.t.Fatal(err)
	}
	return path
}

// cliResult is one run.
type cliResult struct {
	exit        int
	out, errOut string
	doc         map[string]any
}

func (r cliResult) data() map[string]any {
	d, _ := r.doc["data"].(map[string]any)
	if d == nil {
		d = map[string]any{}
	}
	return d
}

func (r cliResult) errObj() map[string]any {
	e, _ := r.doc["error"].(map[string]any)
	if e == nil {
		e = map[string]any{}
	}
	return e
}

func (r cliResult) details() map[string]any {
	d, _ := r.errObj()["details"].(map[string]any)
	if d == nil {
		d = map[string]any{}
	}
	return d
}

func (r cliResult) code() string {
	c, _ := r.errObj()["code"].(string)
	return c
}

func (r cliResult) token() string {
	if t, ok := r.data()["confirm_token"].(string); ok {
		return t
	}
	t, _ := r.details()["confirm_token"].(string)
	return t
}

func (r cliResult) items() []map[string]any {
	var out []map[string]any
	list, _ := r.data()["items"].([]any)
	for _, it := range list {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// batchItems are the per-item results of a batch (CLI-063), in data or in
// error.details.
func (r cliResult) batchItems() []map[string]any {
	list, _ := r.data()["items"].([]any)
	if list == nil {
		list, _ = r.details()["items"].([]any)
	}
	var out []map[string]any
	for _, x := range list {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// summary is a batch's summary, in data or in error.details.
func (r cliResult) summary() map[string]any {
	s, _ := r.data()["summary"].(map[string]any)
	if s == nil {
		s, _ = r.details()["summary"].(map[string]any)
	}
	return s
}

// itemOutcome is a batch item's receipt outcome.
func itemOutcome(it map[string]any) map[string]any {
	rc, _ := it["receipt"].(map[string]any)
	o, _ := rc["outcome"].(map[string]any)
	return o
}

// listOf is a JSON array (nil when v is none).
func listOf(v any) []any {
	l, _ := v.([]any)
	return l
}

// receipt is the receipt of a confirmed write, in data or in error.details.
func (r cliResult) receipt() map[string]any {
	if rc, ok := r.data()["receipt"].(map[string]any); ok {
		return rc
	}
	rc, _ := r.details()["receipt"].(map[string]any)
	if rc == nil {
		rc = map[string]any{}
	}
	return rc
}

func (r cliResult) outcome() map[string]any {
	o, _ := r.receipt()["outcome"].(map[string]any)
	if o == nil {
		o = map[string]any{}
	}
	return o
}

func (r cliResult) preview() map[string]any {
	if p, ok := r.data()["preview"].(map[string]any); ok {
		return p
	}
	p, _ := r.details()["preview"].(map[string]any)
	if p == nil {
		p = map[string]any{}
	}
	return p
}

// app builds the App of the next run.
func (e *testEnv) app() *aicli.App { return newTestApp(e.vars) }

// run executes one command line and checks its output: one envelope, or an NDJSON
// stream (CLI-006) whose every line is checked and whose summary line becomes doc.
func (e *testEnv) run(args ...string) cliResult {
	e.t.Helper()
	app := e.app()
	var out, errb bytes.Buffer
	app.Stdout, app.Stderr = &out, &errb
	app.Stdin = strings.NewReader(e.stdin)
	r := cliResult{exit: app.Execute(args), out: out.String(), errOut: errb.String()}
	if !strings.HasPrefix(strings.TrimSpace(r.out), "{") {
		return r
	}
	doc := r.out
	if isStream(out.Bytes()) {
		if err := aicli.ValidateStream(out.Bytes()); err != nil {
			e.t.Errorf("%v: invalid stream: %v\n%s", args, err, r.out)
		}
		lines := strings.Split(strings.TrimSpace(r.out), "\n")
		doc = lines[len(lines)-1]
	} else if err := aicli.ValidateEnvelope(out.Bytes()); err != nil {
		e.t.Errorf("%v: invalid envelope: %v\n%s", args, err, r.out)
	}
	dec := json.NewDecoder(strings.NewReader(doc))
	dec.UseNumber()
	_ = dec.Decode(&r.doc)
	checkExitCode(e.t, args, r)
	return r
}

// isStream tells NDJSON (one-line documents, more than one) from one envelope, as
// the conformance suite does.
func isStream(raw []byte) bool {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var first json.RawMessage
	if dec.Decode(&first) != nil {
		return false
	}
	rest := bytes.TrimSpace(raw[dec.InputOffset():])
	return len(rest) > 0 && rest[0] == '{' && !bytes.Contains(first, []byte("\n"))
}

// dryRun runs args with --dry-run and returns the confirm token.
func (e *testEnv) dryRun(args ...string) string {
	e.t.Helper()
	r := e.run(append(append([]string{}, args...), "--dry-run")...)
	if r.exit != 0 || r.token() == "" {
		e.t.Fatalf("dry-run %v: exit %d %s", args, r.exit, r.out)
	}
	return r.token()
}

// confirm runs args with --confirm token.
func (e *testEnv) confirm(token string, args ...string) cliResult {
	e.t.Helper()
	return e.run(append(append([]string{}, args...), "--confirm", token)...)
}

// contractCodes is contract.json's error code table: code -> exit, retryable.
var contractCodes = sync.OnceValue(func() map[string]struct {
	Exit      int  `json:"exit"`
	Retryable bool `json:"retryable"`
} {
	raw, err := os.ReadFile(filepath.Join("..", "contract", "contract.json"))
	if err != nil {
		panic(err)
	}
	var c struct {
		ErrorCodes struct {
			Core map[string]struct {
				Exit      int  `json:"exit"`
				Retryable bool `json:"retryable"`
			} `json:"core"`
		} `json:"error_codes"`
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		panic(err)
	}
	return c.ErrorCodes.Core
})

// checkExitCode asserts that an envelope's exit code and retryable are the ones
// contract.json gives its code (CLI-010).
func checkExitCode(t *testing.T, args []string, r cliResult) {
	t.Helper()
	if ok, _ := r.doc["ok"].(bool); ok {
		if r.exit != 0 {
			t.Errorf("%v: ok envelope with exit %d", args, r.exit)
		}
		return
	}
	spec, known := contractCodes()[r.code()]
	if !known {
		t.Errorf("%v: code %q is not in contract.json", args, r.code())
		return
	}
	if r.exit != spec.Exit {
		t.Errorf("%v: %s exits %d, contract.json says %d", args, r.code(), r.exit, spec.Exit)
	}
	if retry, _ := r.errObj()["retryable"].(bool); retry != spec.Retryable {
		t.Errorf("%v: %s retryable %v, contract.json says %v", args, r.code(), retry, spec.Retryable)
	}
}

// stringsOf converts a JSON array to strings.
func stringsOf(x any) []string {
	list, _ := x.([]any)
	var out []string
	for _, v := range list {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func hasString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
