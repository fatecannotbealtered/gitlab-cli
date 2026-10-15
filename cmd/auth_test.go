package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func init() {
	addConformanceSamples(map[string][]string{
		// The host is GITLAB_CLI_HOST (the fake); the token is the fake's second account.
		"auth login":  {"--token", daveToken},
		"auth logout": {},
	})
	// auth logout removes the keyring entry of profile "default": restore it.
	addConformanceReset(func(gl *fakeGitLab) {
		host := "http://127.0.0.1:1"
		if gl != nil {
			host = gl.URL
		}
		_ = writeCredential("", storedCredential{Host: host, Token: daveToken, Username: "dave"})
	})
}

// storeTestCredential puts a token in the test keyring for a profile.
func storeTestCredential(t *testing.T, profile, host, token, user string) {
	t.Helper()
	if err := writeCredential(profile, storedCredential{Host: host, Token: token, Username: user}); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------- behaviour matrix

func TestAuthMatrix(t *testing.T) {
	notUpstream := "logout changes only the local keyring; GitLab is not contacted"
	cases := []matrixCase{
		{Path: "auth login", Args: []string{"--token", daveToken},
			Invalid: []invalidCase{
				{[]string{"--host", "ftp://gitlab.example.com", "--token", "x"}, "E_VALIDATION", "invalid_value", "--host"},
				{[]string{"--host", "http://gitlab.example.com", "--token", "x"}, "E_VALIDATION", "invalid_value", "--host"},
				{[]string{"--token", "a", "--token-file", "f"}, "E_USAGE", "conflicting_flags", "--token-file"},
				{[]string{"--token", "  "}, "E_VALIDATION", "invalid_value", "--token"},
			},
			Rows: map[string]func(t *testing.T){
				"auth and permission": testAuthLoginCredentials,
				"apply failure":       testAuthLoginApplyFailure,
			}},
		{Path: "auth status"},
		{Path: "auth logout",
			Skip: map[string]string{"auth and permission": notUpstream, "upstream failure": notUpstream},
			Rows: map[string]func(t *testing.T){
				"success":       testAuthLogoutSuccess,
				"exit code":     testAuthLogoutSuccess,
				"gate":          testAuthLogoutSuccess,
				"apply failure": testAuthLogoutApplyFailure,
			}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// testAuthLoginCredentials: login checks the new token with GitLab before it is
// stored; without a token it asks the user, never the agent.
func testAuthLoginCredentials(t *testing.T) {
	e := newTestEnv(t)
	e.vars["GITLAB_CLI_TOKEN"] = ""
	r := e.run("auth", "login", "--dangerous", "--dry-run")
	if r.code() != "E_USAGE" || r.details()["parameter"] != "--token-file" || !strings.Contains(r.details()["next_action"].(string), "own terminal") {
		t.Fatalf("no token: %s", r.out)
	}
	r = e.run("auth", "login", "--token", "glpat-revoked", "--dangerous", "--dry-run")
	if r.code() != "E_AUTH" || len(e.gl.Requests()) != 1 {
		t.Fatalf("a token GitLab rejects is not stored: %s", r.out)
	}
	e.gl.fail(fakeFault{Path: "/user", Status: 403})
	if r = e.run("auth", "login", "--token", daveToken, "--dangerous", "--dry-run"); r.code() != "E_FORBIDDEN" {
		t.Fatalf("a 403 from GitLab: %s", r.out)
	}
}

// testAuthLoginApplyFailure: the keyring goes first, so a refused keyring leaves
// settings.json alone, and a settings.json that cannot be written puts the keyring
// entry back; there is no upstream write to refuse or lose.
func testAuthLoginApplyFailure(t *testing.T) {
	t.Run("not sent", func(t *testing.T) {
		e := newTestEnv(t)
		args := []string{"auth", "login", "--token", daveToken, "--dangerous"}
		tok := e.dryRun(args...)
		testKeyring.failSet = errors.New("no secret service")
		r := e.confirm(tok, args...)
		if r.code() != "E_IO" || r.details()["dispatch_state"] != "not_started" || r.outcome()["change"] != "not_applied" {
			t.Fatalf("a refused keyring: %s", r.out)
		}
		if _, err := os.Stat(filepath.Join(e.vars["GITLAB_CLI_STATE_DIR"], "settings.json")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("settings.json must not be written: %v", err)
		}
	})
	t.Run("settings not written", func(t *testing.T) {
		e := newTestEnv(t)
		e.vars["GITLAB_CLI_SETTINGS_FILE"] = filepath.Join(e.dir, "missing", "settings.json")
		storeTestCredential(t, "", e.gl.URL, testToken, "carol")
		before := testKeyring.raw(keyringAccount(""))
		args := []string{"auth", "login", "--host", e.gl.URL, "--token", daveToken, "--dangerous"}
		r := e.confirm(e.dryRun(args...), args...)
		if r.code() != "E_IO" || r.details()["dispatch_state"] != "not_started" || testKeyring.raw(keyringAccount("")) != before {
			t.Fatalf("an unwritable settings.json: %s", r.out)
		}
	})
	t.Run("refused", func(t *testing.T) { t.Skip("not applicable: login writes no upstream request GitLab could refuse") })
	t.Run("unknown", func(t *testing.T) {
		t.Skip("not applicable: the keyring write either succeeds or fails; only a failed keyring restore is unknown")
	})
}

func testAuthLogoutSuccess(t *testing.T) {
	e := newTestEnv(t)
	storeTestCredential(t, "", e.gl.URL, daveToken, "dave")
	dry := e.run("auth", "logout", "--dangerous", "--dry-run")
	if ch := dry.preview()["changes"].([]any)[0].(map[string]any); ch["field"] != "token_stored" || ch["before"] != true {
		t.Fatalf("preview: %s", dry.out)
	}
	r := e.confirm(dry.token(), "auth", "logout", "--dangerous")
	if r.exit != 0 || r.outcome()["verification"] != "passed" || r.receipt()["observed"].(map[string]any)["token_stored"] != false {
		t.Fatalf("logout: %s", r.out)
	}
	if res := r.data()["result"].(map[string]any); res["profile"] != "default" || res["host"] != e.gl.URL {
		t.Fatalf("result: %v", res)
	}
	if testKeyring.raw(keyringAccount("")) != "" {
		t.Fatal("the token is still in the keyring")
	}
	if e.gl.Writes() != 0 || len(e.gl.Requests()) != 0 {
		t.Fatal("logout contacted GitLab")
	}
}

func testAuthLogoutApplyFailure(t *testing.T) {
	t.Run("not sent", func(t *testing.T) {
		e := newTestEnv(t)
		storeTestCredential(t, "", e.gl.URL, daveToken, "dave")
		tok := e.dryRun("auth", "logout", "--dangerous")
		testKeyring.failSet = errors.New("keyring locked")
		r := e.confirm(tok, "auth", "logout", "--dangerous")
		if r.code() != "E_IO" || r.details()["dispatch_state"] != "not_started" || r.outcome()["change"] != "not_applied" {
			t.Fatalf("a locked keyring: %s", r.out)
		}
	})
	t.Run("refused", func(t *testing.T) { t.Skip("not applicable: logout writes no upstream request") })
	t.Run("unknown", func(t *testing.T) { t.Skip("not applicable: a keyring delete either succeeds or fails") })
}

// ---------------------------------------------------------------- focused tests

// TestAuthLoginStoresTheTokenInTheKeyring: the documented path (--token-file -):
// the token goes to the keyring, the host to settings.json, and later commands use
// them; the token never reaches an output, the settings file or the state directory
// (SEC-021, SEC-040).
func TestAuthLoginStoresTheTokenInTheKeyring(t *testing.T) {
	e := newTestEnv(t)
	e.vars["GITLAB_CLI_TOKEN"] = ""
	e.stdin = daveToken + "\n"
	args := []string{"auth", "login", "--host", e.gl.URL + "/", "--token-file", "-", "--dangerous"}
	dry := e.run(append(args, "--dry-run")...)
	r := e.confirm(dry.token(), args...)
	if r.exit != 0 || r.outcome()["verification"] != "passed" {
		t.Fatalf("login: %s", r.out)
	}
	res := r.data()["result"].(map[string]any)
	if res["account"] != "dave" || res["profile"] != "default" || res["host"] != e.gl.URL || res["credentials_source"] != "keyring" {
		t.Fatalf("result: %v", res)
	}
	if inputs := dry.preview()["inputs"].(map[string]any); inputs["token"] != "***" {
		t.Fatalf("the token is shown as ***: %v", inputs)
	}
	settings, err := os.ReadFile(res["settings_file"].(string))
	if err != nil {
		t.Fatal(err)
	}
	// App.SetProfileSettings writes the host, and the first profile becomes the
	// default (Q3).
	var doc map[string]any
	if json.Unmarshal(settings, &doc) != nil || doc["default_profile"] != "default" ||
		doc["profiles"].(map[string]any)["default"].(map[string]any)["host"] != e.gl.URL {
		t.Fatalf("settings.json: %s", settings)
	}
	var stored storedCredential
	if err := json.Unmarshal([]byte(testKeyring.raw("profile:default")), &stored); err != nil || stored.Token != daveToken ||
		stored.Username != "dave" || stored.Host != e.gl.URL {
		t.Fatalf("keyring: %+v %v", stored, err)
	}
	// Later commands sign in from the keyring and report it without a network call.
	ctx := e.run("context")
	if prof := ctx.data()["profile"].(map[string]any); ctx.data()["account"] != "dave" ||
		ctx.data()["credentials"].(map[string]any)["source"] != "keyring" || prof["name"] != "default" || prof["source"] != "config" {
		t.Fatalf("context: %s", ctx.out)
	}
	if st := e.run("auth", "status"); st.data()["account"] != "dave" || st.data()["credentials_source"] != "keyring" {
		t.Fatalf("auth status: %s", st.out)
	}
	assertSecretAbsent(t, e, daveToken, dry, r)
}

// assertSecretAbsent searches outputs and the state directory for a secret and its
// unsalted SHA-256 (SEC-021).
func assertSecretAbsent(t *testing.T, e *testEnv, secret string, runs ...cliResult) {
	t.Helper()
	sum := sha256.Sum256([]byte(secret))
	needles := []string{secret, hex.EncodeToString(sum[:])}
	for _, r := range runs {
		for _, n := range needles {
			if strings.Contains(r.out+r.errOut, n) {
				t.Fatalf("the token (or its hash) appeared in an output: %s", r.out)
			}
		}
	}
	_ = filepath.Walk(e.vars["GITLAB_CLI_STATE_DIR"], func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			b, _ := os.ReadFile(p)
			for _, n := range needles {
				if strings.Contains(string(b), n) {
					t.Errorf("%s holds the token (or its hash)", p)
				}
			}
		}
		return nil
	})
}

// TestAuthLoginProfiles: login writes the profile it is asked for, refuses to
// guess between several, and a new profile needs --host (CLI-101); login and logout
// run under a profile settings.json lacks (Meta.AllowUnknownProfile), other commands stop up
// front (D4).
func TestAuthLoginProfiles(t *testing.T) {
	e := newTestEnv(t)
	e.writeSettings(`{"profiles": {"work": {"host": "` + e.gl.URL + `"}, "home": {"host": "https://gitlab.com"}}}`)
	e.vars["GITLAB_CLI_HOST"] = ""
	r := e.run("auth", "login", "--token", daveToken, "--dangerous", "--dry-run")
	if r.code() != "E_USAGE" || r.details()["parameter"] != "--profile" {
		t.Fatalf("several profiles and none active: %s", r.out)
	}
	if r = e.run("auth", "login", "--profile", "lab", "--token", daveToken, "--dangerous", "--dry-run"); r.details()["parameter"] != "--host" {
		t.Fatalf("a new profile needs --host: %s", r.out)
	}
	args := []string{"auth", "login", "--profile", "work", "--token", daveToken, "--dangerous"}
	if r = e.confirm(e.dryRun(args...), args...); r.exit != 0 || r.data()["result"].(map[string]any)["profile"] != "work" {
		t.Fatalf("login to work: %s", r.out)
	}
	if testKeyring.raw("profile:work") == "" || testKeyring.raw("profile:home") != "" {
		t.Fatal("only work's entry is written")
	}
	// The token of work is used with --profile work, not with home.
	e.vars["GITLAB_CLI_TOKEN"] = ""
	if r = e.run("label", "list", "--project", "acme/web", "--profile", "work"); r.exit != 0 {
		t.Fatalf("work signs in from the keyring: %s", r.out)
	}
	if r = e.run("label", "list", "--project", "acme/web", "--profile", "home"); r.code() != "E_AUTH" {
		t.Fatalf("home has no token: %s", r.out)
	}
	sent := len(e.gl.Requests())
	if r = e.run("label", "list", "--project", "acme/web", "--profile", "nope"); r.code() != "E_CONFIG" ||
		r.details()["profile"] != "nope" || strings.Contains(r.errObj()["message"].(string), "nope") || len(e.gl.Requests()) != sent {
		t.Fatalf("an unknown profile: %s", r.out)
	}
	// Two profiles and no default_profile: a keyring token is used only for a named
	// profile.
	if r = e.run("label", "list", "--project", "acme/web"); r.code() != "E_USAGE" || r.details()["parameter"] != "--profile" {
		t.Fatalf("no active profile: %s", r.out)
	}
	// A new profile is created by its first login (aicli's gate resolves the
	// identity under a profile settings.json does not have yet).
	args = []string{"auth", "login", "--profile", "lab", "--host", e.gl.URL, "--token", daveToken, "--dangerous"}
	if r = e.confirm(e.dryRun(args...), args...); r.exit != 0 {
		t.Fatalf("login creating profile lab: %s", r.out)
	}
	if r = e.run("auth", "status", "--profile", "lab"); r.data()["account"] != "dave" || r.data()["profile"] != "lab" {
		t.Fatalf("lab signs in: %s", r.out)
	}
	settings, _ := os.ReadFile(filepath.Join(e.vars["GITLAB_CLI_STATE_DIR"], "settings.json"))
	var doc map[string]any
	// Only a file's first profile becomes its default (Q3, RC2-1): this file had two
	// profiles and no default_profile, a person's choice that lab's login keeps, so a
	// command without --profile still has to name one.
	if json.Unmarshal(settings, &doc) != nil || len(doc["profiles"].(map[string]any)) != 3 {
		t.Fatalf("settings.json keeps the other profiles: %s", settings)
	}
	if _, set := doc["default_profile"]; set {
		t.Fatalf("login of a new profile set default_profile: %s", settings)
	}
	if r = e.run("label", "list", "--project", "acme/web"); r.code() != "E_USAGE" || r.details()["parameter"] != "--profile" {
		t.Fatalf("still no active profile after lab's login: %s", r.out)
	}
	// A profile removed from settings.json by hand: logout still removes its token.
	storeTestCredential(t, "gone", e.gl.URL, daveToken, "dave")
	args = []string{"auth", "logout", "--profile", "gone", "--dangerous"}
	if r = e.confirm(e.dryRun(args...), args...); r.exit != 0 || testKeyring.raw("profile:gone") != "" {
		t.Fatalf("logout of a profile settings.json lacks: %s", r.out)
	}
}

// TestAuthTokenStaysWithItsHost: a keyring token is sent only to the host it was
// stored for, whatever GITLAB_CLI_HOST says.
func TestAuthTokenStaysWithItsHost(t *testing.T) {
	e := newTestEnv(t)
	e.vars["GITLAB_CLI_TOKEN"] = ""
	storeTestCredential(t, "", "https://gitlab.other.example", testToken, "carol")
	r := e.run("label", "list", "--project", "acme/web")
	if r.code() != "E_AUTH" || !strings.Contains(r.errObj()["message"].(string), "another GitLab host") || len(e.gl.Requests()) != 0 {
		t.Fatalf("a token for another host: %s", r.out)
	}
	ctx := e.run("context")
	if ctx.data()["credentials"].(map[string]any)["configured"] != false {
		t.Fatalf("context: %s", ctx.out)
	}
}

func TestAuthStatusReportsTheToken(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("auth", "status")
	d := r.data()
	if r.exit != 0 || d["account"] != "carol" || d["user_id"] != "7" || d["credentials_source"] != "env" ||
		d["expires_at"] != "2027-01-31T00:00:00Z" || d["token_name"] != "agent token" || stringsOf(d["scopes"])[0] != "api" {
		t.Fatalf("auth status: %s", r.out)
	}
	e.gl.fail(fakeFault{Path: "personal_access_tokens", Status: 404})
	if r = e.run("auth", "status"); r.exit != 0 || r.data()["expires_at"] != nil || r.data()["token_name"] != nil {
		t.Fatalf("a token GitLab has no record of (OAuth, job token): %s", r.out)
	}
}

func TestAuthLogoutWithoutAStoredToken(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("auth", "logout", "--dangerous", "--dry-run")
	if r.code() != "E_NOT_FOUND" || !strings.Contains(r.details()["next_action"].(string), "GITLAB_CLI_TOKEN") {
		t.Fatalf("nothing stored, an env token in use: %s", r.out)
	}
	r = e.run("auth", "logout", "--dry-run")
	if r.details()["problem"] != "dangerous_flag_required" {
		t.Fatalf("logout is configure, so dangerous: %s", r.out)
	}
}
