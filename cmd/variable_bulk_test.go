package cmd

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- behaviour matrix

func TestVariableBulkMatrix(t *testing.T) {
	file := filepath.Join(t.TempDir(), "ci.env")
	if err := os.WriteFile(file, []byte("# imported\nBULK_ONE=alpha-secret-1\nexport DEPLOY_ENV=\"from bulk\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(t.TempDir(), "empty.env")
	if err := os.WriteFile(empty, []byte("# nothing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A batch (CLI-063): the shared matrix's batch rows check every item; the
	// command is dangerous, so it stops at its first failure. Its reads before the
	// first write: the two variables and the project.
	runMatrix(t, matrixCase{Path: "variable bulk-import", Args: []string{"--project", "acme/web", "--file", file}, PreviewReads: 3,
		Invalid: []invalidCase{
			{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--file"},
			{[]string{"--project", "acme/web", "--file", empty}, "E_USAGE", "missing_parameter", "--file"},
			{[]string{"--project", "acme/web", "--file", filepath.Join(t.TempDir(), "missing.env")}, "E_USAGE", "invalid_value", "--file"},
			{[]string{"--project", "acme/web", "--file", file, "--env-scope", "a/../b"}, "E_VALIDATION", "invalid_value", "--env-scope"},
		}})
}

// TestVariableBulkImportPlansAndKeepsValuesSecret: one plan per variable (a create
// or an update), the file's content bound as a keyed digest, the requests the plans
// named, and no value in any output (SEC-021).
func TestVariableBulkImportPlansAndKeepsValuesSecret(t *testing.T) {
	file := filepath.Join(t.TempDir(), "ci.env")
	if err := os.WriteFile(file, []byte("# imported\nBULK_ONE=alpha-secret-1\nexport DEPLOY_ENV=\"from bulk\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"variable", "bulk-import", "--project", "acme/web", "--file", file, "--dangerous"}
	e := newTestEnv(t)
	dry := e.run(append(append([]string{}, args...), "--dry-run")...)
	p := dry.preview()
	// The file's content binds under the name it fills, as a keyed digest (SEC-021).
	if dry.token() == "" || ciTargetKeys(p) != "acme/web/-/variables/BULK_ONE acme/web/-/variables/DEPLOY_ENV" ||
		p["inputs"].(map[string]any)["variables"] != "***" {
		t.Fatalf("the batch preview: %s", dry.out)
	}
	pitems, _ := p["items"].([]any)
	if len(pitems) != 2 || !strings.HasPrefix(pitems[0].(map[string]any)["changes"].([]any)[0].(map[string]any)["field"].(string), "environment_scope") {
		t.Fatalf("item plans: %s", dry.out)
	}
	r := e.confirm(dry.token(), args...)
	if r.exit != 0 || len(r.batchItems()) != 2 {
		t.Fatalf("confirm: %s", r.out)
	}
	for _, it := range r.batchItems() {
		if rc, _ := it["receipt"].(map[string]any); rc["observed"].(map[string]any)["value"] != "***" {
			t.Fatalf("item %v: %s", it["target"], r.out)
		}
	}
	var writes []fakeRequest
	for _, q := range e.gl.Requests() {
		if q.Method != http.MethodGet {
			writes = append(writes, q)
		}
	}
	if len(writes) != 2 || writes[0].Method != http.MethodPost || writes[0].Body["key"] != "BULK_ONE" ||
		writes[1].Method != http.MethodPut || writes[1].Body["value"] != "from bulk" {
		t.Fatalf("the requests the plans named: %+v", writes)
	}
	var vals []string
	e.gl.locked(func() {
		for _, v := range fakeCIState(e.gl).variables["acme/web"] {
			if v.Key == "BULK_ONE" || v.Key == "DEPLOY_ENV" {
				vals = append(vals, v.Key+"="+v.Value)
			}
		}
	})
	if strings.Join(vals, ",") != "DEPLOY_ENV=from bulk,BULK_ONE=alpha-secret-1" {
		t.Fatalf("GitLab's variables: %v", vals)
	}
	ciAssertNoSecret(t, e, "alpha-secret-1", dry.out, r.out, r.errOut)
}

// ciTargetKeys is a batch preview's target keys, space-separated.
func ciTargetKeys(preview map[string]any) string {
	list, _ := preview["targets"].([]any)
	var keys []string
	for _, t := range list {
		if m, ok := t.(map[string]any); ok {
			keys = append(keys, m["key"].(string))
		}
	}
	return strings.Join(keys, " ")
}

// ---------------------------------------------------------------- focused tests

func TestParseVariableImport(t *testing.T) {
	vars, err := parseVariableImport("ci.env", []byte("# c\r\n\r\nexport A=1\r\nB = 'two words' \nC=\"x=y\"\nD=\n"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, v := range vars {
		got = append(got, v.key+"="+v.value)
	}
	if strings.Join(got, "|") != "A=1|B=two words|C=x=y|D=" {
		t.Fatalf("dotenv: %q", got)
	}
	vars, err = parseVariableImport("vars.json", []byte(`{"Z": "last", "A": "first"}`))
	if err != nil || len(vars) != 2 || vars[0].key != "Z" || vars[1].value != "first" {
		t.Fatalf("JSON keeps the file's order: %+v %v", vars, err)
	}
	for _, c := range []struct{ path, content string }{
		{"a.env", "GOOD=1\nnot a pair\n"},
		{"a.env", "BAD-KEY=secret-value\n"},
		{"a.env", "A=secret-1\nA=secret-2\n"},
		{"a.json", `{"A": 1}`},
		{"a.json", `["A"]`},
		{"a.json", `{"A": "x"`},
		{"a.env", "# only comments\n"},
	} {
		_, err := parseVariableImport(c.path, []byte(c.content))
		if err == nil {
			t.Errorf("%q should fail", c.content)
			continue
		}
		for _, leak := range []string{"secret-value", "secret-1", "secret-2", "BAD-KEY"} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("the error quotes the file: %v", err)
			}
		}
	}
}

// TestVariableBulkImportRefusesDrift: a key created between the dry-run and the
// confirm turns a planned create into an update; the confirm refuses (CLI-057).
func TestVariableBulkImportRefusesDrift(t *testing.T) {
	e := newTestEnv(t)
	e.stdin = "BULK_TWO=beta-secret-2\n"
	args := []string{"variable", "bulk-import", "--project", "acme/web", "--file", "-", "--dangerous"}
	tok := e.dryRun(args...)
	e.gl.locked(func() {
		ci := fakeCIState(e.gl)
		ci.variables["acme/web"] = append(ci.variables["acme/web"], &fakeVariable{Key: "BULK_TWO", Value: "other", Type: "env_var", Scope: "*"})
	})
	r := e.confirm(tok, args...)
	if r.code() != "E_CONFLICT" || r.details()["conflict"] != "state_drift" || e.gl.Writes() != 0 {
		t.Fatalf("drift: %s", r.out)
	}
}

// TestVariableBulkImportScopeAndJSON: every variable goes to --env-scope; a JSON file
// works like a .env one.
func TestVariableBulkImportScopeAndJSON(t *testing.T) {
	e := newTestEnv(t)
	file := filepath.Join(t.TempDir(), "vars.json")
	if err := os.WriteFile(file, []byte(`{"DATABASE_URL": "postgres://review", "NEW_ONE": "n-1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"variable", "bulk-import", "--project", "acme/web", "--file", file, "--env-scope", "production", "--dangerous"}
	dry := e.run(append(args, "--dry-run")...)
	if ciTargetKeys(dry.preview()) != "acme/web/-/variables/DATABASE_URL:production acme/web/-/variables/NEW_ONE:production" {
		t.Fatalf("targets: %s", dry.out)
	}
	r := e.confirm(dry.token(), args...)
	if r.exit != 0 {
		t.Fatalf("import: %s", r.out)
	}
	var prod string
	e.gl.locked(func() { prod = fakeCIState(e.gl).variables["acme/web"][1].Value })
	if prod != "postgres://review" {
		t.Fatalf("the production DATABASE_URL was updated: %q", prod)
	}
}
