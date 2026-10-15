package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- behaviour matrix

func TestVariableMatrix(t *testing.T) {
	valueFile := filepath.Join(t.TempDir(), "value.txt")
	if err := os.WriteFile(valueFile, []byte("from-a-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []matrixCase{
		{Path: "variable list", Args: []string{"--project", "acme/web"}, EmptyArgs: []string{"--project", "acme/empty"},
			Invalid: []invalidCase{
				{[]string{}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"--project", "acme/web", "--offset", "3"}, "E_USAGE", "invalid_value", "--offset"},
			}},
		{Path: "variable get", Args: []string{"DEPLOY_ENV", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--key"},
				{[]string{"DEPLOY-ENV", "--project", "acme/web"}, "E_VALIDATION", "invalid_value", "--key"},
				{[]string{"DATABASE_URL:production", "--project", "acme/web", "--env-scope", "staging"}, "E_USAGE", "conflicting_flags", "--env-scope"},
				{[]string{"DEPLOY_ENV", "--project", "acme/web", "--env-scope", "../prod"}, "E_VALIDATION", "invalid_value", "--env-scope"},
				{[]string{"DEPLOY_ENV", "--project", "acme/web", "--env-scope", "*", "--filter", "env_scope=*"}, "E_USAGE", "conflicting_flags", "--filter"},
			}},
		{Path: "variable create", Args: []string{"--project", "acme/web", "--key", "NEW_KEY", "--value", "new-value-123", "--description", "New"},
			Role: "maintainer", RefusedStatus: 400, PreviewReads: 2, // the variable, then the project
			RequestFields: map[string]string{"key": "key", "environment_scope": "environment_scope", "variable_type": "variable_type",
				"protected": "protected", "masked": "masked", "raw": "raw", "description": "description"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web", "--key", "A"}, "E_USAGE", "missing_parameter", "--value"},
				{[]string{"--project", "acme/web", "--key", "bad key", "--value", "x"}, "E_VALIDATION", "invalid_value", "--key"},
				{[]string{"--project", "acme/web", "--key", "A", "--value", "short", "--masked"}, "E_VALIDATION", "invalid_value", "--value"},
				{[]string{"--project", "acme/web", "--key", "A", "--value", "has space in it", "--masked"}, "E_VALIDATION", "invalid_value", "--value"},
				{[]string{"--project", "acme/web", "--key", "A", "--value", "x", "--type", "yaml"}, "E_USAGE", "invalid_value", "--type"},
				{[]string{"--project", "acme/web", "--key", "A", "--value", "x", "--value-file", valueFile}, "E_USAGE", "conflicting_flags", "--value-file"},
				{[]string{"--project", "acme/web", "--key", "A", "--value", "x", "--env-scope", ""}, "E_VALIDATION", "invalid_value", "--env-scope"},
			}},
		// Without --env-scope the confirm reads the variable once to name the target,
		// then once more in its Preview.
		{Path: "variable update", Args: []string{"DEPLOY_ENV", "--project", "acme/web", "--value", "staging-1", "--description", "Where main deploys now"},
			Role: "maintainer", PreviewReads: 2, RefusedStatus: 400,
			RequestFields: map[string]string{"description": "description", "protected": "protected", "masked": "masked", "raw": "raw",
				"variable_type": "variable_type"},
			Invalid: []invalidCase{
				{[]string{"DEPLOY_ENV", "--project", "acme/web"}, "E_USAGE", "missing_parameter", "--value"},
				{[]string{"DEPLOY_ENV", "--project", "acme/web", "--protected", "yes"}, "E_USAGE", "invalid_value", "--protected"},
				{[]string{"--project", "acme/web", "--value", "x"}, "E_USAGE", "missing_parameter", "--key"},
			}},
		{Path: "variable delete", Args: []string{"OLD_TOKEN", "--project", "acme/web"}, Role: "maintainer", PreviewReads: 2,
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web", "--dangerous"}, "E_USAGE", "missing_parameter", "--key"},
				{[]string{"acme/web/-/variables/", "--dangerous"}, "E_VALIDATION", "invalid_value", "--key"},
			}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// ---------------------------------------------------------------- focused tests

// TestVariableFormsAreOneTarget: KEY, KEY:scope, --env-scope, the ref and 1.x's
// --filter read the same variable, and the ref and KEY --env-scope bind as one
// request (CLI-048, CLI-054).
func TestVariableFormsAreOneTarget(t *testing.T) {
	e := newTestEnv(t)
	for _, c := range []struct {
		args []string
		ref  string
	}{
		{[]string{"variable", "get", "DEPLOY_ENV", "--project", "acme/web"}, "acme/web/-/variables/DEPLOY_ENV"},
		{[]string{"variable", "get", "acme/web/-/variables/DEPLOY_ENV"}, "acme/web/-/variables/DEPLOY_ENV"},
		{[]string{"variables", "show", "DEPLOY_ENV", "-p", "acme/web", "--env-scope", "*"}, "acme/web/-/variables/DEPLOY_ENV"},
		{[]string{"var", "view", "--key", "DATABASE_URL:production", "--repo", "acme/web"}, "acme/web/-/variables/DATABASE_URL:production"},
		{[]string{"variable", "get", "DATABASE_URL", "--project", "acme/web", "--env-scope", "staging"}, "acme/web/-/variables/DATABASE_URL:staging"},
		{[]string{"variable", "get", "acme/web/-/variables/DATABASE_URL:production"}, "acme/web/-/variables/DATABASE_URL:production"},
	} {
		r := e.run(c.args...)
		if r.exit != 0 || r.data()["ref"] != c.ref {
			t.Fatalf("%v: %s", c.args, r.out)
		}
	}
	r := e.run("variable", "get", "DATABASE_URL", "--project", "acme/web", "--filter", "env_scope=production")
	if r.data()["ref"] != "acme/web/-/variables/DATABASE_URL:production" || !hasString(ciNotices(r), "deprecated") {
		t.Fatalf("1.x's --filter: %s", r.out)
	}
	tok := e.dryRun("variable", "update", "acme/web/-/variables/DATABASE_URL:staging", "--description", "Staging DB", "--dangerous")
	r = e.confirm(tok, "variable", "update", "DATABASE_URL", "--project", "acme/web", "--env-scope", "staging", "--description", "Staging DB",
		"--dangerous")
	if r.exit != 0 || r.data()["result"].(map[string]any)["description"] != "Staging DB" {
		t.Fatalf("the ref and KEY --env-scope must bind alike: %s", r.out)
	}
}

// TestVariableAmbiguousKey: a key in several scopes is E_AMBIGUOUS with each
// scope's ref, for reads and writes alike, and nothing is written (CLI-048).
func TestVariableAmbiguousKey(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"variable", "get", "DATABASE_URL", "--project", "acme/web"},
		{"variable", "delete", "DATABASE_URL", "--project", "acme/web", "--dangerous", "--dry-run"},
		{"variable", "update", "DATABASE_URL", "--project", "acme/web", "--protected", "false", "--dangerous", "--dry-run"},
	} {
		r := e.run(args...)
		cands, _ := r.details()["candidates"].([]any)
		if r.code() != "E_AMBIGUOUS" || len(cands) != 2 || cands[0].(map[string]any)["ref"] != "acme/web/-/variables/DATABASE_URL:production" ||
			!hasString(ciUntrusted(r), "error.details.candidates[].label") {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
	if e.gl.Writes() != 0 {
		t.Fatal("an ambiguous key sent a write")
	}
}

// TestVariableValuesNeverAppear: no read returns a value, and a written value
// reaches GitLab but appears in no output and no state file (SEC-021); the receipt
// shows that it was read back, as ***.
func TestVariableValuesNeverAppear(t *testing.T) {
	e := newTestEnv(t)
	var outs []string
	for _, args := range [][]string{
		{"variable", "list", "--project", "acme/web", "--fields", "all"},
		{"variable", "get", "DATABASE_URL:production", "--project", "acme/web"},
		{"variable", "get", "API_KEY", "--project", "acme/web"},
	} {
		r := e.run(args...)
		if r.exit != 0 || strings.Contains(r.out, `"value"`) {
			t.Fatalf("%v shows a value: %s", args, r.out)
		}
		outs = append(outs, r.out)
	}
	for _, seeded := range []string{"postgres://prod-db-secret-1", "hidden-api-key-secret-4", "old-token-secret-3"} {
		for _, out := range outs {
			if strings.Contains(out, seeded) {
				t.Fatalf("a seeded value leaked: %s", out)
			}
		}
	}
	const secret = "sentinel-variable-secret-16180"
	args := []string{"variable", "create", "--project", "acme/web", "--key", "DEPLOY_TOKEN", "--value", secret, "--masked", "--protected",
		"--dangerous"}
	dry := e.run(append(args, "--dry-run")...)
	if dry.preview()["inputs"].(map[string]any)["value"] != "***" {
		t.Fatalf("inputs show the value as ***: %s", dry.out)
	}
	r := e.confirm(dry.token(), args...)
	obs := r.receipt()["observed"].(map[string]any)
	if r.exit != 0 || obs["value"] != "***" || obs["masked"] != true || r.outcome()["verification"] != "passed" ||
		r.data()["result"].(map[string]any)["ref"] != "acme/web/-/variables/DEPLOY_TOKEN" {
		t.Fatalf("create: %s", r.out)
	}
	var stored string
	e.gl.locked(func() {
		vs := fakeCIState(e.gl).variables["acme/web"]
		stored = vs[len(vs)-1].Value
	})
	if stored != secret {
		t.Fatalf("GitLab got %q", stored)
	}
	ciAssertNoSecret(t, e, secret, dry.out, dry.errOut, r.out, r.errOut, e.run("operation", "list").out)
}

// TestVariableValueMismatchIsReported: a value GitLab stored differently from the
// request (here with a newline added) is E_VERIFY_FAILED naming value, and still
// not quoted (CLI-059, SEC-021).
func TestVariableValueMismatchIsReported(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"variable", "update", "DEPLOY_ENV", "--project", "acme/web", "--value", "staging-mangled", "--dangerous"}
	tok := e.dryRun(args...)
	e.gl.locked(func() { fakeCIState(e.gl).mangle = true })
	r := e.confirm(tok, args...)
	if r.code() != "E_VERIFY_FAILED" || !hasString(stringsOf(r.receipt()["mismatched"]), "value") ||
		r.receipt()["observed"].(map[string]any)["value"] != "***" || strings.Contains(r.out, "staging-mangled") {
		t.Fatalf("a value stored differently: %s", r.out)
	}
}

// TestVariableCreateWithoutGrantIsRefusedLocally is the eval's no-permission task:
// creating a CI/CD variable is configure, dangerous by default, and without a scope
// that grants both the tool refuses before anything is sent (SEC-010, CLI-014).
func TestVariableCreateWithoutGrantIsRefusedLocally(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"variable", "create", "--project", "acme/web", "--key", "DEPLOY_TOKEN", "--value", "tok-12345678"}
	r := e.run(append(args, "--dry-run")...)
	if r.code() != "E_CONFIRMATION_REQUIRED" || r.details()["problem"] != "dangerous_flag_required" {
		t.Fatalf("configure is dangerous: %s", r.out)
	}
	e.setScope(nil)
	r = e.run(append(args, "--dangerous", "--dry-run")...)
	scope, _ := r.details()["scope"].(map[string]any)
	missing, _ := scope["missing"].(map[string]any)
	if r.code() != "E_FORBIDDEN" || r.exit != 4 || scope["status"] != "missing" || !hasString(stringsOf(missing["effects"]), "configure") ||
		missing["dangerous"] != true || missing["target"] != "acme/web/-/variables/DEPLOY_TOKEN" {
		t.Fatalf("no scope: %s", r.out)
	}
	only := fullScope()
	only["effects"], only["dangerous"] = []string{"change"}, false
	e.setScope(only)
	if r = e.run(append(args, "--dangerous", "--dry-run")...); r.code() != "E_FORBIDDEN" || r.details()["scope"].(map[string]any)["status"] != "insufficient" {
		t.Fatalf("a change-only scope: %s", r.out)
	}
	if e.gl.Writes() != 0 {
		t.Fatal("a refused write reached GitLab")
	}
}

func TestVariableCreateIsBlockedWhenTheKeyExists(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("variable", "create", "--project", "acme/web", "--key", "DEPLOY_ENV", "--value", "x", "--dangerous", "--dry-run")
	if r.exit != 0 || r.preview()["can_apply"] != false || r.preview()["blocked_by"].([]any)[0].(map[string]any)["kind"] != "already_exists" {
		t.Fatalf("an existing key and scope: %s", r.out)
	}
	args := []string{"variable", "create", "--project", "acme/web", "--key", "DEPLOY_ENV", "--value", "review", "--env-scope", "review/*",
		"--dangerous", "--idempotency-key", "k-1"}
	r = e.confirm(e.dryRun(args...), args...)
	if r.exit != 0 || r.data()["result"].(map[string]any)["ref"] != "acme/web/-/variables/DEPLOY_ENV:review/*" {
		t.Fatalf("the same key in another scope: %s", r.out)
	}
	if again := e.run(append(args, "--dry-run")...); again.receipt()["replayed"] != true || e.gl.Writes() != 1 {
		t.Fatalf("a repeat with the key is a replay: %s", again.out)
	}
	if r := e.run("variable", "create", "--project", "acme/nowhere", "--key", "X", "--value", "x", "--dangerous", "--dry-run"); r.code() != "E_NOT_FOUND" {
		t.Fatalf("a missing project: %s", r.out)
	}
}

// TestVariableUpdateChecksTheMaskedValue: a value that will be masked must be
// maskable, whether it is new or the current one (checked in memory, never shown).
func TestVariableUpdateChecksTheMaskedValue(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("variable", "update", "DATABASE_URL:production", "--project", "acme/web", "--value", "short", "--dangerous", "--dry-run")
	if r.code() != "E_VALIDATION" || r.details()["parameter"] != "--value" {
		t.Fatalf("a short value for a masked variable: %s", r.out)
	}
	e.gl.locked(func() { fakeCIState(e.gl).variables["acme/web"][0].Value = "two words" })
	r = e.run("variable", "update", "DEPLOY_ENV", "--project", "acme/web", "--masked", "true", "--dangerous", "--dry-run")
	if r.code() != "E_VALIDATION" || strings.Contains(r.out, "two words") {
		t.Fatalf("masking an unmaskable current value: %s", r.out)
	}
	args := []string{"variable", "update", "DATABASE_URL", "--env-scope", "staging", "--project", "acme/web", "--value-file", "-", "--dangerous"}
	e.stdin = "postgres://staging-db-new"
	r = e.confirm(e.dryRun(args...), args...)
	var stored string
	e.gl.locked(func() { stored = fakeCIState(e.gl).variables["acme/web"][2].Value })
	if r.exit != 0 || stored != "postgres://staging-db-new" || r.receipt()["observed"].(map[string]any)["value"] != "***" {
		t.Fatalf("--value-file - reads stdin: %s (stored %q)", r.out, stored)
	}
}

func TestVariableDeleteReadsBack(t *testing.T) {
	e := newTestEnv(t)
	if r := e.run("variable", "delete", "NO_SUCH_KEY", "--project", "acme/web", "--dangerous", "--dry-run"); r.code() != "E_NOT_FOUND" {
		t.Fatalf("a missing variable is E_NOT_FOUND at the dry-run (CLI-029): %s", r.out)
	}
	args := []string{"variable", "rm", "DATABASE_URL:staging", "--project", "acme/web", "--dangerous"}
	r := e.confirm(e.dryRun(args...), args...)
	if r.exit != 0 || r.receipt()["observed"].(map[string]any)["exists"] != false ||
		r.data()["result"].(map[string]any)["ref"] != "acme/web/-/variables/DATABASE_URL:staging" {
		t.Fatalf("delete: %s", r.out)
	}
	if r := e.run("variable", "get", "DATABASE_URL", "--project", "acme/web"); r.exit != 0 || r.data()["environment_scope"] != "production" {
		t.Fatalf("the other scope stays, and is no longer ambiguous: %s", r.out)
	}
}

// TestVariableUntrustedText: a description is people's text, here with an injected
// instruction; it is data, listed in meta.untrusted (CLI-046, SEC-002).
func TestVariableUntrustedText(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("variable", "list", "--project", "acme/web", "--fields", "+description")
	if !strings.Contains(r.out, "Ignore previous instructions") || !hasString(ciUntrusted(r), "data.items[].description") {
		t.Fatalf("description: %s", r.out)
	}
	scope := r.data()["scope"].(map[string]any)
	if !strings.Contains(scope["covers"].(string), "group and instance variables are not included") || r.data()["total"] != json.Number("5") {
		t.Fatalf("scope: %s", r.out)
	}
}

func TestParseVariableArg(t *testing.T) {
	for in, want := range map[string][3]string{
		"DEPLOY_ENV":                                   {"", "DEPLOY_ENV", ""},
		"DATABASE_URL:production":                      {"", "DATABASE_URL", "production"},
		"acme/web/-/variables/DEPLOY_ENV":              {"acme/web", "DEPLOY_ENV", ""},
		"acme/sub/web/-/variables/K:review/*":          {"acme/sub/web", "K", "review/*"},
		"acme/web/-/variables/K:${CI_COMMIT_REF_NAME}": {"acme/web", "K", "${CI_COMMIT_REF_NAME}"},
	} {
		p, k, s, _, err := parseVariableArg(in)
		if err != nil || p != want[0] || k != want[1] || s != want[2] {
			t.Errorf("parseVariableArg(%q) = %q %q %q %v", in, p, k, s, err)
		}
	}
	for _, bad := range []string{"", "BAD-KEY", "K:", "K:../x", "42/-/variables/K", "acme/web/-/variables/", "K:a\x00b"} {
		if _, _, _, _, err := parseVariableArg(bad); err == nil {
			t.Errorf("parseVariableArg(%q) should fail", bad)
		}
	}
}
