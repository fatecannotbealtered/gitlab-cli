package cmd

import (
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
)

// TestBuiltInsDescribeTheTool: help, --version, reference, context, doctor and
// changelog come from aicli, fed by NewApp's hooks (CLI-030 to CLI-037).
func TestBuiltInsDescribeTheTool(t *testing.T) {
	e := newTestEnv(t)
	help := e.run("--help")
	if help.exit != 0 || !strings.Contains(help.out, "label create") || !strings.Contains(help.out, "--confirm <token>") {
		t.Fatalf("help: %s", help.out)
	}
	if v := e.run("--version"); v.data()["tool"] != "gitlab-cli" || v.data()["spec_version"] != "2.0.0" {
		t.Fatalf("--version: %s", v.out)
	}
	if idx := e.run("reference", "--index"); idx.exit != 0 || len(idx.out) > 8000 {
		t.Fatalf("reference --index: %d characters", len(idx.out))
	}
	ctx := e.run("context")
	d := ctx.data()
	if ctx.exit != 0 || d["account"] != envAccount || d["host"] != e.gl.URL ||
		d["credentials"].(map[string]any)["source"] != "env" || d["credentials"].(map[string]any)["configured"] != true {
		t.Fatalf("context: %s", ctx.out)
	}
	if len(e.gl.Requests()) != 0 {
		t.Fatal("context made a network call (CLI-035)")
	}
	doc := e.run("doctor")
	checks := map[string]map[string]any{}
	for _, c := range doc.data()["checks"].([]any) {
		m := c.(map[string]any)
		checks[m["check"].(string)] = m
	}
	if checks["credentials"]["status"] != "pass" || !strings.Contains(checks["credentials"]["message"].(string), "carol") ||
		checks["connectivity"]["status"] != "pass" || checks["settings"]["status"] != "pass" {
		t.Fatalf("doctor: %s", doc.out)
	}
	if cl := e.run("changelog", "--since", "1.2.16"); cl.exit != 0 || cl.data()["count"] == nil {
		t.Fatalf("changelog: %s", cl.out)
	}
}

// TestDoctorSeparatesNetworkFromCredentials: a rejected token fails credentials;
// an unreachable GitLab fails only connectivity (CLI-036).
func TestDoctorSeparatesNetworkFromCredentials(t *testing.T) {
	check := func(r cliResult, name string) map[string]any {
		for _, c := range r.data()["checks"].([]any) {
			if m := c.(map[string]any); m["check"] == name {
				return m
			}
		}
		return nil
	}
	e := newTestEnv(t)
	e.vars["GITLAB_CLI_TOKEN"] = "glpat-revoked"
	r := e.run("doctor")
	if check(r, "credentials")["status"] != "fail" || check(r, "connectivity")["status"] != "pass" {
		t.Fatalf("a rejected token: %s", r.out)
	}
	e.vars["GITLAB_CLI_TOKEN"] = ""
	r = e.run("doctor")
	if c := check(r, "credentials"); c["status"] != "fail" || !strings.Contains(c["fix"].(string), "auth login") {
		t.Fatalf("no token: %s", r.out)
	}
	e.vars["GITLAB_CLI_TOKEN"] = testToken
	e.gl.down()
	r = e.run("doctor")
	if r.exit != 0 || check(r, "connectivity")["status"] != "fail" || check(r, "credentials")["status"] != "pass" {
		t.Fatalf("GitLab down: %s", r.out)
	}
}

// TestSettingsResolveFromEnvProfileAndDefault: the host comes from GITLAB_CLI_HOST,
// then the active profile, then https://gitlab.com (CLI-100), and a settings file
// with a token in it is refused (SEC-040).
func TestSettingsResolveFromEnvProfileAndDefault(t *testing.T) {
	e := newTestEnv(t)
	e.vars["GITLAB_CLI_HOST"] = ""
	if ctx := e.run("context"); ctx.data()["host"] != "https://gitlab.com" {
		t.Fatalf("default host: %s", ctx.out)
	}
	path := e.writeSettings(`{"default_profile": "work", "profiles": {"work": {"host": "` + e.gl.URL + `", "timeout": "10s"}, "home": {"host": "https://gitlab.com"}}}`)
	if r := e.run("label", "list", "--project", "acme/web"); r.exit != 0 {
		t.Fatalf("the work profile's host: %s", r.out)
	}
	if ctx := e.run("context", "--profile", "home"); ctx.data()["host"] != "https://gitlab.com" {
		t.Fatalf("--profile home: %s", ctx.out)
	}
	e.vars["GITLAB_CLI_HOST"] = "https://gitlab.env.example"
	if ctx := e.run("context"); ctx.data()["host"] != "https://gitlab.env.example" {
		t.Fatalf("GITLAB_CLI_HOST wins: %s", ctx.out)
	}
	e.vars["GITLAB_CLI_HOST"] = ""
	if err := os.WriteFile(path, []byte(`{"profiles": {"work": {"host": "https://h", "token": "glpat-x"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r := e.run("label", "list", "--project", "acme/web")
	if r.code() != "E_CONFIG" || !strings.Contains(r.errObj()["message"].(string), "secret") {
		t.Fatalf("a token in settings.json: %s", r.out)
	}
}

// TestHostAndTimeoutAreChecked: a token is never sent over plain http to a remote
// host, and the timeout is at most 30 s (CLI-110).
func TestHostAndTimeoutAreChecked(t *testing.T) {
	e := newTestEnv(t)
	for _, host := range []string{"http://gitlab.example.com", "https://user:pw@gitlab.example.com", "gitlab.example.com"} {
		e.vars["GITLAB_CLI_HOST"] = host
		if r := e.run("label", "list", "--project", "acme/web"); r.code() != "E_CONFIG" || strings.Contains(r.out, "pw@") {
			t.Fatalf("host %q: %s", host, r.out)
		}
	}
	e.vars["GITLAB_CLI_HOST"] = e.gl.URL
	for _, timeout := range []string{"1m", "0s", "soon"} {
		e.vars["GITLAB_CLI_TIMEOUT"] = timeout
		if r := e.run("label", "list", "--project", "acme/web"); r.code() != "E_CONFIG" {
			t.Fatalf("timeout %q: %s", timeout, r.out)
		}
	}
	if len(e.gl.Requests()) != 0 {
		t.Fatal("a refused configuration sent requests")
	}
}

// TestPrivateCAFile: TLS is always verified; GITLAB_CLI_CA_FILE adds a company CA
// (CLI-110), and an untrusted certificate says so.
func TestPrivateCAFile(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fakeJSON(w, 200, map[string]any{"id": 7, "username": "carol", "name": "Carol"})
	}))
	defer srv.Close()
	e := newTestEnv(t)
	e.vars["GITLAB_CLI_HOST"] = srv.URL
	r := e.run("auth", "status")
	if r.code() != "E_NETWORK" || !strings.Contains(r.details()["next_action"].(string), "GITLAB_CLI_CA_FILE") {
		t.Fatalf("an untrusted certificate: %s", r.out)
	}
	if d := e.run("doctor"); !strings.Contains(d.out, "not trusted") {
		t.Fatalf("doctor names the certificate: %s", d.out)
	}
	ca := filepath.Join(e.dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	e.vars["GITLAB_CLI_CA_FILE"] = ca
	if r = e.run("auth", "status"); r.exit != 0 || r.data()["account"] != "carol" {
		t.Fatalf("with the CA file: %s", r.out)
	}
	e.vars["GITLAB_CLI_CA_FILE"] = filepath.Join(e.dir, "missing.pem")
	if r = e.run("auth", "status"); r.code() != "E_CONFIG" {
		t.Fatalf("a missing CA file: %s", r.out)
	}
}

// TestProxyComesFromTheEnvironment: HTTPS_PROXY, HTTP_PROXY and NO_PROXY, read
// through the App's environment by aicli's transport (CLI-110, Q2).
func TestProxyComesFromTheEnvironment(t *testing.T) {
	app := newTestApp(map[string]string{"HTTPS_PROXY": "http://proxy.corp:3128", "no_proxy": ".internal.example"})
	tr, e := app.HTTPTransport()
	if e != nil {
		t.Fatal(e)
	}
	proxy := tr.Proxy
	for target, want := range map[string]string{
		"https://gitlab.example.com/api/v4/user": "http://proxy.corp:3128",
		"https://git.internal.example/api/v4":    "",
	} {
		req := &http.Request{URL: mustURL(t, target)}
		got, err := proxy(req)
		if err != nil || (want == "" && got != nil) || (want != "" && (got == nil || got.String() != want)) {
			t.Errorf("%s: proxy %v %v, want %q", target, got, err, want)
		}
	}
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// TestRequestsNameTheToolAndRetryReadsOnly: the User-Agent is
// gitlab-cli/<version> aicli/<spec>; a read is retried after a 503, a write never
// (CLI-110).
func TestRequestsNameTheToolAndRetryReadsOnly(t *testing.T) {
	e := newTestEnv(t)
	e.gl.fail(fakeFault{Method: http.MethodGet, Status: 503, Times: 1})
	if r := e.run("label", "list", "--project", "acme/web"); r.exit != 0 {
		t.Fatalf("a read retried after a 503: %s", r.out)
	}
	reqs := e.gl.Requests()
	if len(reqs) != 2 || !strings.HasPrefix(reqs[0].UserAgent, "gitlab-cli/") || !strings.HasSuffix(reqs[0].UserAgent, " aicli/2.0.0") {
		t.Fatalf("requests: %+v", reqs)
	}
}

// TestDebugLinesGoToStderrWithoutSecrets: GITLAB_CLI_DEBUG prints request lines on
// stderr, never the token, and leaves stdout alone (CLI-102).
func TestDebugLinesGoToStderrWithoutSecrets(t *testing.T) {
	e := newTestEnv(t)
	plain := e.run("label", "list", "--project", "acme/web")
	e.vars["GITLAB_CLI_DEBUG"] = "1"
	r := e.run("label", "list", "--project", "acme/web")
	if !strings.Contains(r.errOut, "debug: GET /api/v4/projects/acme%2Fweb/labels") || strings.Contains(r.errOut, testToken) {
		t.Fatalf("debug lines: %s", r.errOut)
	}
	if r.exit != plain.exit || len(r.items()) != len(plain.items()) {
		t.Fatal("debug changed the output")
	}
}

// TestProjectNamesWorkEverywhere: every command that takes --project takes a
// project's name too (projectFlag, CLI-048). "payments" names two projects, so it
// is E_AMBIGUOUS with both refs in every group, before anything is planned; "web"
// names one, which then works as its path does.
func TestProjectNamesWorkEverywhere(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"label", "list", "--project", "payments"},
		{"milestone", "list", "--project", "payments"},
		{"mr", "list", "--project", "payments"},
		{"issue", "list", "--project", "payments"},
		{"pipeline", "list", "--project", "payments"},
		{"variable", "list", "--project", "payments"},
		{"repo", "branch", "list", "--project", "payments"},
		{"release", "list", "--project", "payments"},
		{"project", "members", "--project", "payments"},
		{"label", "create", "--project", "payments", "--name", "x", "--color", "red", "--dry-run"},
	} {
		r := e.run(args...)
		if r.code() != "E_AMBIGUOUS" || r.details()["parameter"] != "--project" || len(listOf(r.details()["candidates"])) != 2 {
			t.Errorf("%v: %s", args, r.out)
		}
	}
	if e.gl.Writes() != 0 {
		t.Fatal("an ambiguous project planned or sent a write")
	}
	r := e.run("label", "list", "--project", "web")
	if scope, _ := r.data()["scope"].(map[string]any); r.exit != 0 || scope["project"] != "acme/web" {
		t.Fatalf("one match is that project: %s", r.out)
	}
}

// TestConfirmIsBoundToTheCredentials: the profile and every setting, the token from
// GITLAB_CLI_TOKEN included (as a keyed digest), are bound into the confirm token
// (CLI-054, B1), so a confirm under another token or profile sends nothing.
func TestConfirmIsBoundToTheCredentials(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"label", "update", "bug", "--project", "acme/web", "--color", "red"}
	dry := e.run(append(args, "--dry-run")...)
	if set, _ := dry.preview()["inputs"].(map[string]any)["setting"].(map[string]any); set["token"] != "***" || set["host"] != e.gl.URL {
		t.Fatalf("preview.inputs.setting: %s", dry.out)
	}
	e.vars["GITLAB_CLI_TOKEN"] = daveToken
	r := e.confirm(dry.token(), args...)
	if r.details()["conflict"] != "token_mismatch" || r.details()["parameter"] != "setting.token" || e.gl.Writes() != 0 {
		t.Fatalf("another token at confirm: %s", r.out)
	}
	e.vars["GITLAB_CLI_TOKEN"] = testToken
	e.writeSettings(`{"profiles": {"work": {"host": "` + e.gl.URL + `"}}}`)
	r = e.confirm(dry.token(), append(args, "--profile", "work")...)
	if r.details()["conflict"] != "token_mismatch" || e.gl.Writes() != 0 {
		t.Fatalf("another profile at confirm: %s", r.out)
	}
}

// TestCommandMapLines pins this group's lines of the Skill's command map (SKL-002):
// the verbs in registration order, and the groups in registerCommands order. Each
// migrated group checks its own line the same way, in its own test file.
func TestCommandMapLines(t *testing.T) {
	got := newTestApp(map[string]string{}).CommandMap()
	lines := []string{
		"- `label`: list, get, create, update, delete (dangerous)\n",
		"- `milestone`: list, get, create, update, close\n",
		"- `auth`: login (dangerous), status, logout (dangerous)\n",
	}
	last := -1
	for _, line := range lines {
		i := strings.Index(got, line)
		if i < 0 || i < last {
			t.Fatalf("command map:\n%s\nwant, in this order:\n%s", got, strings.Join(lines, ""))
		}
		last = i
	}
}

// TestUpdateIsTheBuiltIn: update is aicli's, configured with gitlab-cli's
// repository and npm package (CLI-070, VER-004).
func TestUpdateIsTheBuiltIn(t *testing.T) {
	var paths []string
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		fakeJSON(w, 200, []map[string]any{{"tag_name": "v2.1.0", "draft": false, "prerelease": false, "body": ""}})
	}))
	defer gh.Close()
	e := newTestEnv(t)
	app := e.app()
	app.Version = "2.0.0"
	app.Update.APIBase = gh.URL
	app.Update.InstallMethod = func(string) string { return "npm" }
	var out strings.Builder
	app.Stdout = &out
	if code := app.Execute([]string{"update", "--check"}); code != 0 {
		t.Fatalf("update --check: %s", out.String())
	}
	if !strings.Contains(out.String(), `@fateforge/gitlab-cli@2.1.0`) || len(paths) == 0 ||
		!strings.HasPrefix(paths[0], "/repos/fatecannotbealtered/gitlab-cli/") {
		t.Fatalf("update --check: %s %v", out.String(), paths)
	}
	// A development build (no -ldflags version) reports the patch after the newest
	// release in CHANGELOG.md, 1.2.18-dev after 1.2.17 (P14), and refuses to update
	// at discover, before any request.
	entries, _ := e.run("changelog").data()["entries"].([]any)
	if len(entries) == 0 {
		t.Fatal("changelog lists no release")
	}
	newest, err := aicli.ParseVersion(entries[0].(map[string]any)["version"].(string))
	if err != nil {
		t.Fatal(err)
	}
	dev := fmt.Sprintf("%d.%d.%d-dev", newest.Major, newest.Minor, newest.Patch+1)
	r := e.run("update", "--check")
	if r.code() != "E_UNSUPPORTED" || r.details()["stage"] != "discover" || r.details()["current_version"] != dev {
		t.Fatalf("a development build reports %s: %s", dev, r.out)
	}
	version = "2.0.0" // a release build, as -ldflags sets it
	defer func() { version = "dev" }()
	r = e.run("update", "--target-version", "1.0.12&calc")
	if r.code() != "E_USAGE" || r.details()["parameter"] != "--target-version" {
		t.Fatalf("a crafted target version: %s", r.out)
	}
}

// TestAgentGuesses: the fleet's measured guesses are accepted or answered with an
// exact did_you_mean (CLI-022). The verb guesses view, show, ls, add, new, edit, rm
// and remove are leaf aliases of every group (per parent, P1); an unknown verb is
// answered with the corrected command line, flags included (P11).
func TestAgentGuesses(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"labels", "list", "-R", "acme/web"},
		{"label", "get", "bug", "--repo", "acme/web"},
		{"label", "view", "bug", "--project", "acme/web"},
		{"labels", "ls", "-R", "acme/web"},
		{"milestones", "list", "-p", "acme/web"},
		{"milestone", "show", "3", "--project", "acme/web"},
	} {
		if r := e.run(args...); r.exit != 0 {
			t.Fatalf("%v should be accepted: %s", args, r.out)
		}
	}
	for _, args := range [][]string{
		{"milestone", "add", "--title", "v9", "--project", "acme/web", "--dry-run"},
		{"label", "rm", "bug", "--project", "acme/web", "--dangerous", "--dry-run"},
	} {
		if r := e.run(args...); r.exit != 0 || r.token() == "" {
			t.Fatalf("%v should be accepted: %s", args, r.out)
		}
	}
	for _, c := range []struct {
		args       []string
		suggestion string
		next       string
	}{
		{[]string{"label", "gte", "bug", "--project", "acme/web"}, "gitlab-cli label get", "`gitlab-cli label get bug --project acme/web`"},
		{[]string{"milestone", "clsoe", "3", "--project", "acme/web"}, "gitlab-cli milestone close", "`gitlab-cli milestone close 3 --project acme/web`"},
	} {
		r := e.run(c.args...)
		if r.details()["problem"] != "unknown_command" || stringsOf(r.details()["did_you_mean"])[0] != c.suggestion ||
			!strings.Contains(r.details()["next_action"].(string), c.next) {
			t.Fatalf("%v: %s", c.args, r.out)
		}
	}
	r := e.run("label", "list", "--project", "acme/web", "--all")
	if r.details()["problem"] != "unknown_flag" || stringsOf(r.details()["did_you_mean"])[0] != "--limit" {
		t.Fatalf("--all is answered with --limit: %s", r.out)
	}
	r = e.run("label", "list", "--project", "acme/web", "--per-page", "5")
	if stringsOf(r.details()["did_you_mean"])[0] != "--limit" {
		t.Fatalf("--per-page is answered with --limit: %s", r.out)
	}
	r = e.run("label", "list", "--project", "acme/web", "--search", "bug")
	if stringsOf(r.details()["did_you_mean"])[0] != "--query" {
		t.Fatalf("--search is answered with --query: %s", r.out)
	}
}

func TestParseProjectAndRefs(t *testing.T) {
	for in, want := range map[string]string{
		"acme/web": "acme/web", "acme/web/": "acme/web", "/acme/sub/web.git": "acme/sub/web",
		"https://gitlab.example.com/acme/web/-/issues/3": "acme/web", "https://h/acme/web": "acme/web",
	} {
		if got, numeric, err := parseProject(in); err != nil || numeric || got != want {
			t.Errorf("parseProject(%q) = %q %v %v", in, got, numeric, err)
		}
	}
	if p, numeric, err := parseProject(" 42 "); err != nil || !numeric || p != "42" {
		t.Errorf("numeric: %q %v %v", p, numeric, err)
	}
	for _, bad := range []string{"", "web", "acme/../web", "acme/./web", "acme/web.", "acme/w eb", "https://h/web"} {
		if _, _, err := parseProject(bad); err == nil {
			t.Errorf("parseProject(%q) should fail", bad)
		}
	}
	type ref struct {
		project string
		iid     int
	}
	for in, want := range map[string]ref{
		mrRef("acme/web", 5): {"acme/web", 5}, "!5": {"", 5}, "5": {"", 5},
		"https://h/acme/web/-/merge_requests/5/diffs": {"acme/web", 5},
	} {
		if p, iid, err := parseIIDRef(in, "!", "/-/merge_requests/"); err != nil || p != want.project || iid != want.iid {
			t.Errorf("parseIIDRef(%q) = %q %d %v", in, p, iid, err)
		}
	}
	if p, iid, err := parseIIDRef(issueRef("acme/web", 12), "#", "/-/issues/"); err != nil || p != "acme/web" || iid != 12 {
		t.Errorf("issue ref: %q %d %v", p, iid, err)
	}
	for _, bad := range []string{"", "x", "acme/web!x", "!0", "https://h/acme/web/-/issues/3"} {
		if _, _, err := parseIIDRef(bad, "!", "/-/merge_requests/"); err == nil {
			t.Errorf("parseIIDRef(%q) should fail", bad)
		}
	}
	for in, want := range map[string]string{"bug": "bug", "acme/web/-/labels/bug": "acme/web/-/labels/bug",
		"feature request": `"feature request"`, `a"b`: `"a\"b"`, "": `""`} {
		if got := shellArg(in); got != want {
			t.Errorf("shellArg(%q) = %s, want %s", in, got, want)
		}
	}
}
