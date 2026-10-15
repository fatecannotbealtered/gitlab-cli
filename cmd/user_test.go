package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
)

// The user commands run against the fake's shared users (carol, dave, erin) and
// /user, /users endpoints (fakegitlab_test.go).

func TestUserMatrix(t *testing.T) {
	cases := []matrixCase{
		{Path: "user me"},
		{Path: "user get", Args: []string{"@dave"},
			Invalid: []invalidCase{
				{[]string{}, "E_USAGE", "missing_parameter", "--user"},
				{[]string{"https://gitlab.example.com/acme/web"}, "E_VALIDATION", "invalid_value", "--user"},
			}},
		{Path: "user search", Args: []string{"--query", "er"}, EmptyArgs: []string{"--query", "zzz-none"},
			Invalid: []invalidCase{
				{[]string{}, "E_USAGE", "missing_parameter", "--query"},
				{[]string{"--query", " "}, "E_VALIDATION", "invalid_value", "--query"},
				{[]string{"--query", "er", "--limit", "500"}, "E_USAGE", "invalid_value", "--limit"},
			}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// TestUserFormsAreOneUser: @carol, carol, Carol, me, the profile URL and the display
// name read the same user, whose ref is @carol (CLI-048).
func TestUserFormsAreOneUser(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"user", "get", "@carol"},
		{"user", "get", "carol"},
		{"user", "get", "Carol"},
		{"user", "get", "me"},
		{"user", "get", "--user", e.gl.URL + "/carol"},
		{"users", "view", "https://gitlab.example.com/users/carol"},
		{"user", "show", "Carol Danvers"},
		{"user", "me"},
	} {
		r := e.run(args...)
		if r.exit != 0 || r.data()["ref"] != "@carol" || r.data()["id"] != "7" || r.data()["name"] != "Carol Danvers" {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
	e.vars["GITLAB_CLI_TOKEN"] = daveToken
	if r := e.run("user", "me"); r.data()["ref"] != "@dave" {
		t.Fatalf("user me is the token's account: %s", r.out)
	}
}

// TestUserGetNotFound: an @username names a username only; a plain word that is no
// username is looked up as a display name before E_NOT_FOUND.
func TestUserGetNotFound(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("user", "get", "@nobody")
	if r.code() != "E_NOT_FOUND" || r.details()["parameter"] != "--user" || len(e.gl.Requests()) != 1 {
		t.Fatalf("@nobody: %s (%d requests)", r.out, len(e.gl.Requests()))
	}
	r = e.run("user", "get", "Nobody Here")
	if r.code() != "E_NOT_FOUND" || !strings.Contains(r.details()["next_action"].(string), `user search --query "Nobody Here"`) {
		t.Fatalf("an unknown name: %s", r.out)
	}
	if last := e.gl.Requests()[len(e.gl.Requests())-1]; !strings.Contains(last.Query, "search=Nobody+Here") {
		t.Fatalf("the name lookup searches: %+v", last)
	}
}

// TestUserNameMatchingSeveralIsAmbiguous: a display name two users share is
// E_AMBIGUOUS with both @usernames; Smithers is not a Smith.
func TestUserNameMatchingSeveralIsAmbiguous(t *testing.T) {
	more := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("username") != "" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		if more {
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[{"id":71,"username":"csmith","name":"Carol Smith"},{"id":73,"username":"asmithers","name":"Alan Smithers"}]`))
			return
		}
		_, _ = w.Write([]byte(`[{"id":71,"username":"csmith","name":"Carol Smith"},{"id":72,"username":"dsmith","name":"Dana Smith"},` +
			`{"id":73,"username":"asmithers","name":"Alan Smithers"}]`))
	}))
	defer srv.Close()
	e := newTestEnv(t)
	c, err := clientFor(e.app(), srv.URL, testToken)
	if err != nil {
		t.Fatal(err)
	}
	_, err = userTarget(context.Background(), c, "smith")
	ae, ok := err.(*aicli.Error)
	if !ok || ae.Code != "E_AMBIGUOUS" || ae.Details["parameter"] != "--user" {
		t.Fatalf("two Smiths: %v", err)
	}
	var refs []string
	for _, cand := range ae.Details["candidates"].([]any) {
		refs = append(refs, cand.(map[string]any)["ref"].(string))
	}
	if !slices.Equal(refs, []string{"@csmith", "@dsmith"}) {
		t.Fatalf("candidates: %v", refs)
	}
	more = true
	if _, err = userTarget(context.Background(), c, "smith"); err == nil || err.(*aicli.Error).Code != "E_USAGE" {
		t.Fatalf("one match with more pages: %v", err)
	}
}

func TestUserArg(t *testing.T) {
	for raw, want := range map[string]string{"@carol": "carol", " carol ": "carol", "https://h/carol": "carol", //nolint:gocritic // the spaces are the input under test
		"https://h/users/carol/": "carol", "https://gitlab.example.com/gitlab/carol": "carol"} {
		name, _, err := userArg("https://gitlab.example.com/gitlab", raw)
		if err != nil || name != want {
			t.Errorf("userArg(%q) = %q, %v", raw, name, err)
		}
	}
	if _, exact, _ := userArg("https://h", "carol"); exact {
		t.Error("a bare word is not exact")
	}
	for _, bad := range []string{"https://h/acme/web", "https://h/", "https://h/a%20b"} {
		if _, _, err := userArg("https://h", bad); err == nil {
			t.Errorf("userArg(%q) should fail", bad)
		}
	}
}

// TestUserSearchStatesRelevance: user search keeps GitLab's order, exact matches
// first (CLI-029), filters active users on GitLab's side, and marks names untrusted.
func TestUserSearchStatesRelevance(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("user", "search", "--query", "er", "--active")
	scope := r.data()["scope"].(map[string]any)
	if r.exit != 0 || r.data()["sort"] != "relevance" || scope["query"] != "er" || scope["state"] != "active" || len(r.items()) != 3 {
		t.Fatalf("user search: %s", r.out)
	}
	if req := e.gl.Requests()[0]; !strings.Contains(req.Query, "active=true") || !strings.Contains(req.Query, "search=er") {
		t.Fatalf("query: %s", req.Query)
	}
	if it := r.items()[0]; it["ref"] != "@carol" || it["state"] != "active" || len(it) != 4 {
		t.Fatalf("default fields: %v", it)
	}
	if !hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "data.items[].name") {
		t.Fatalf("names are untrusted: %s", r.out)
	}
}
