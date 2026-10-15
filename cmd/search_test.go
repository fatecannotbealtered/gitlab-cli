package cmd

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// ---------------------------------------------------------------- fake GitLab

// fakeSearchHits is what the fake's search finds in one project.
type fakeSearchHits struct {
	Issues  []fakeIssueHit
	MRs     []fakeMRHit
	Blobs   []fakeBlobHit
	Commits []fakeCommitHit
}

type fakeIssueHit struct {
	IID                   int
	Title, State, Author  string
	Labels                []string
	Created, Updated      string
	NoReference, NoWebURL bool // left out of the answer
}

type fakeMRHit struct {
	IID                                  int
	Title, State, Author, Source, Target string
	Draft                                bool
	Labels                               []string
	Created, Updated                     string
}

type fakeBlobHit struct {
	Path, Ref string
	Line      int
	Data      string
}

type fakeCommitHit struct {
	SHA, Title, Author, Created string
	NoWebURL                    bool // left out of the answer: the tool reads the project ID
}

func fakeSearchState(gl *fakeGitLab) map[string]*fakeSearchHits {
	return gl.state["searchHits"].(map[string]*fakeSearchHits)
}

func init() {
	addFakeSeed(func(gl *fakeGitLab) {
		gl.state["advancedSearch"] = false // global commit and code search refused, as on basic search
		gl.state["searchHits"] = map[string]*fakeSearchHits{
			"acme/web": {
				Issues: []fakeIssueHit{
					{IID: 1, Title: "Login fails after a password reset", State: "opened", Author: "carol", Labels: []string{"bug"},
						Created: "2026-09-01T10:00:00.000Z", Updated: "2026-10-01T10:00:00.000Z"},
					{IID: 2, Title: "Login page is slow", State: "closed", Author: "dave", Labels: []string{"performance"},
						Created: "2026-09-05T10:00:00.000Z", Updated: "2026-09-20T10:00:00.000Z"},
					{IID: 3, Title: "Add dark mode", State: "opened", Author: "erin", Created: "2026-09-07T10:00:00.000Z",
						Updated: "2026-09-07T10:00:00.000Z"},
				},
				MRs: []fakeMRHit{
					{IID: 1, Title: "Fix login redirect", State: "merged", Author: "carol", Source: "fix-login", Target: "main",
						Labels: []string{"bug"}, Created: "2026-09-02T10:00:00.000Z", Updated: "2026-09-03T10:00:00.000Z"},
					{IID: 2, Title: "Draft: login rate limit", State: "opened", Author: "dave", Source: "login-limit", Target: "main",
						Draft: true, Created: "2026-09-06T10:00:00.000Z", Updated: "2026-09-08T10:00:00.000Z"},
				},
				Blobs: []fakeBlobHit{
					{Path: "src/login.go", Ref: "main", Line: 12, Data: "func Login(w http.ResponseWriter, r *http.Request) {\n"},
					{Path: "src/login_test.go", Ref: "main", Line: 30, Data: "func TestLogin(t *testing.T) {\n"},
					{Path: "README.md", Ref: "main", Line: 3, Data: "Run the login service with make run.\n"},
				},
				Commits: []fakeCommitHit{
					{SHA: "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678", Title: "fix(login): keep the redirect", Author: "Carol Danvers",
						Created: "2026-09-02T10:00:00.000Z"},
					{SHA: "b2c3d4e5f60718293a4b5c6d7e8f901234567890", Title: "feat(login): rate limit attempts", Author: "Dave Lister",
						Created: "2026-09-06T10:00:00.000Z"},
				},
			},
			"fin/payments-service": {
				Issues: []fakeIssueHit{{IID: 1, Title: "Login to the payments dashboard fails", State: "opened", Author: "dave",
					Labels: []string{"bug"}, Created: "2026-10-02T10:00:00.000Z", Updated: "2026-10-02T10:00:00.000Z"}},
				Commits: []fakeCommitHit{{SHA: "c3d4e5f60718293a4b5c6d7e8f90123456789012", Title: "fix: login timeout",
					Author: "Erin Hannon", Created: "2026-10-03T10:00:00.000Z", NoWebURL: true}},
			},
		}
	})
	addFakeRoutes(
		fakeRoute{"GET /api/v4/search", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) { fakeSearch(gl, w, r, "") }},
		fakeRoute{"GET /api/v4/projects/{project}/search", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			if p := gl.project(w, r); p != nil {
				fakeSearch(gl, w, r, p.Path)
			}
		}},
	)
}

// fakeSearch answers GitLab's search API like basic search: issues and merge
// requests newest first, commits and code across every project refused unless the
// test turned advanced search on.
func fakeSearch(gl *fakeGitLab, w http.ResponseWriter, r *http.Request, project string) {
	q := r.URL.Query()
	scope, query := q.Get("scope"), strings.ToLower(q.Get("search"))
	advanced, _ := gl.state["advancedSearch"].(bool)
	idOf := map[string]int{}
	for _, p := range gl.projects {
		idOf[p.Path] = p.ID
	}
	paths := []string{project}
	if project == "" {
		paths = sortedBy(fakeSearchState(gl))
	}
	if project == "" && !advanced && (scope == "commits" || scope == "blobs") {
		fakeJSON(w, 400, map[string]any{"message": "400 Bad request - Scope not supported without Elasticsearch!"})
		return
	}
	user := func(name string) map[string]any { return map[string]any{"username": name, "name": name} }
	var out []map[string]any
	var created []string
	for _, path := range paths {
		hits := fakeSearchState(gl)[path]
		if hits == nil {
			hits = &fakeSearchHits{}
		}
		switch scope {
		case "projects":
			if project != "" {
				fakeJSON(w, 400, map[string]any{"message": "400 Bad request - scope does not have a valid value"})
				return
			}
		case "issues":
			for _, i := range hits.Issues {
				if strings.Contains(strings.ToLower(i.Title), query) {
					hit := map[string]any{"id": 9000 + i.IID, "iid": i.IID, "project_id": idOf[path], "title": i.Title, "state": i.State,
						"labels": i.Labels, "author": user(i.Author), "created_at": i.Created, "updated_at": i.Updated,
						"web_url": gl.URL + "/" + path + "/-/issues/" + strconv.Itoa(i.IID), "references": map[string]any{"full": path + "#" + strconv.Itoa(i.IID)}}
					if i.NoReference {
						delete(hit, "references")
					}
					if i.NoWebURL {
						delete(hit, "web_url")
					}
					out, created = append(out, hit), append(created, i.Created)
				}
			}
		case "merge_requests":
			for _, m := range hits.MRs {
				if strings.Contains(strings.ToLower(m.Title), query) {
					out = append(out, map[string]any{"id": 8000 + m.IID, "iid": m.IID, "project_id": idOf[path], "title": m.Title,
						"state": m.State, "draft": m.Draft, "source_branch": m.Source, "target_branch": m.Target, "labels": m.Labels,
						"author": user(m.Author), "created_at": m.Created, "updated_at": m.Updated,
						"web_url": gl.URL + "/" + path + "/-/merge_requests/" + strconv.Itoa(m.IID), "references": map[string]any{"full": path + "!" + strconv.Itoa(m.IID)}})
					created = append(created, m.Created)
				}
			}
		case "blobs":
			for _, b := range hits.Blobs {
				if strings.Contains(strings.ToLower(b.Data+b.Path), query) {
					out = append(out, map[string]any{"basename": strings.TrimSuffix(b.Path, ".go"), "data": b.Data, "path": b.Path,
						"filename": b.Path, "ref": b.Ref, "startline": b.Line, "project_id": idOf[path]})
				}
			}
		case "commits":
			for _, c := range hits.Commits {
				if strings.Contains(strings.ToLower(c.Title), query) {
					hit := map[string]any{"id": c.SHA, "short_id": c.SHA[:8], "title": c.Title, "author_name": c.Author,
						"created_at": c.Created, "web_url": gl.URL + "/" + path + "/-/commit/" + c.SHA, "project_id": idOf[path]}
					if c.NoWebURL {
						delete(hit, "web_url")
					}
					out = append(out, hit)
				}
			}
		default:
			fakeJSON(w, 400, map[string]any{"message": "400 Bad request - scope does not have a valid value"})
			return
		}
	}
	if scope == "projects" {
		for _, p := range gl.projects {
			m := fakeMetaOf(gl, p.Path)
			if strings.Contains(strings.ToLower(p.Path+"\n"+m.Name+"\n"+m.Description), query) {
				out = append(out, fakeProjectJSON(gl, p, true))
			}
		}
	}
	if len(created) == len(out) && len(out) > 1 { // basic search: newest first
		idx := make([]int, len(out))
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(a, b int) bool { return created[idx[a]] > created[idx[b]] })
		sorted := make([]map[string]any, len(out))
		for i, k := range idx {
			sorted[i] = out[k]
		}
		out = sorted
	}
	fakePage(w, r, out)
}

// ---------------------------------------------------------------- behaviour matrix

func TestSearchMatrix(t *testing.T) {
	inProject := func(query string) []string { return []string{"--query", query, "--project", "acme/web"} }
	cases := []matrixCase{
		{Path: "search projects", Args: []string{"--query", "payments"}, EmptyArgs: []string{"--query", "zzz-none"},
			Invalid: []invalidCase{
				{[]string{}, "E_USAGE", "missing_parameter", "--query"},
				{[]string{"--query", " "}, "E_VALIDATION", "invalid_value", "--query"},
			}},
		{Path: "search issues", Args: inProject("login"), EmptyArgs: inProject("zzz-none"),
			Invalid: []invalidCase{
				{[]string{}, "E_USAGE", "missing_parameter", "--query"},
				{[]string{"--query", "login", "--project", "acme/../web"}, "E_USAGE", "invalid_value", "--project"},
			}},
		{Path: "search mrs", Args: inProject("login"), EmptyArgs: inProject("zzz-none"),
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--query"},
				{[]string{"--query", "login", "--limit", "0"}, "E_USAGE", "invalid_value", "--limit"},
			}},
		{Path: "search code", Args: inProject("login"), EmptyArgs: inProject("zzz-none"),
			Invalid: []invalidCase{
				{[]string{"--query", "login"}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--query"},
			}},
		{Path: "search commits", Args: inProject("login"), EmptyArgs: inProject("zzz-none"),
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--query"},
				{[]string{"--query", " ", "--project", "acme/web"}, "E_VALIDATION", "invalid_value", "--query"},
			}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// ---------------------------------------------------------------- focused tests

// TestSearchStatesItsOrderAndScope (CLI-029, CLI-041): every search says its order
// is GitLab's own (upstream, CLI-029) and what that can be, and what it covered.
func TestSearchStatesItsOrderAndScope(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("search", "issues", "--query", "login", "--project", "acme/web")
	scope := r.data()["scope"].(map[string]any)
	if r.exit != 0 || r.data()["sort"] != "upstream" || scope["ranking"] != searchRanking || scope["project"] != "acme/web" ||
		scope["query"] != "login" || len(r.items()) != 2 {
		t.Fatalf("search issues: %s", r.out)
	}
	if it := r.items()[0]; it["ref"] != "acme/web#2" || it["state"] != "closed" || len(it) != 5 {
		t.Fatalf("first item (newest, as basic search answers): %v", it)
	}
	if !hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "data.items[].title") {
		t.Fatalf("titles are untrusted: %s", r.out)
	}
	r = e.run("search", "issues", "--query", "login", "--fields", "all")
	scope = r.data()["scope"].(map[string]any)
	var refs []string
	for _, it := range r.items() {
		refs = append(refs, it["ref"].(string))
	}
	if r.exit != 0 || scope["covers"] != "every project this account can see" || scope["project"] != nil ||
		strings.Join(refs, ",") != "fin/payments-service#1,acme/web#2,acme/web#1" {
		t.Fatalf("every project: %v %s", refs, r.out)
	}
	if it := r.items()[2]; it["author"] != "@carol" || stringsOf(it["labels"])[0] != "bug" || it["id"] != "9001" || it["iid"] != "1" {
		t.Fatalf("issue fields: %v", it)
	}
	for _, args := range [][]string{
		{"search", "projects", "--query", "payments"},
		{"search", "mrs", "--query", "login", "--project", "acme/web"},
		{"search", "code", "--query", "login", "--project", "acme/web"},
		{"search", "commits", "--query", "login", "--project", "acme/web"},
	} {
		r = e.run(args...)
		if r.exit != 0 || r.data()["sort"] != "upstream" || r.data()["scope"].(map[string]any)["ranking"] != searchRanking {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
}

// TestSearchProjectAcceptsEveryForm: --project takes a path, URL, numeric ID or
// name, as every project command does (CLI-048).
func TestSearchProjectAcceptsEveryForm(t *testing.T) {
	e := newTestEnv(t)
	for _, project := range []string{"acme/web", "https://gitlab.example.com/acme/web", "1", "web"} {
		r := e.run("search", "mrs", "--query", "login", "--project", project)
		if r.exit != 0 || len(r.items()) != 2 || r.items()[0]["ref"] != "acme/web!2" || r.data()["scope"].(map[string]any)["project"] != "acme/web" {
			t.Fatalf("--project %s: %s", project, r.out)
		}
	}
	r := e.run("search", "mrs", "--query", "login", "--project", "acme/web", "--fields", "ref,draft,source_branch")
	if it := r.items()[0]; it["draft"] != true || it["source_branch"] != "login-limit" || len(it) != 3 {
		t.Fatalf("merge request fields: %v", it)
	}
	r = e.run("search", "issues", "--query", "login", "--project", "acme/nowhere")
	if r.code() != "E_NOT_FOUND" || !strings.Contains(r.details()["next_action"].(string), "--project") {
		t.Fatalf("a missing project: %s", r.out)
	}
}

// TestSearchCommitsAcrossProjects: GitLab's basic search refuses commits across
// every project, and the error points at --project; with advanced search the refs
// come from each commit's web URL, or from its project ID when GitLab sent no URL.
func TestSearchCommitsAcrossProjects(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("search", "commits", "--query", "login")
	if r.code() != "E_VALIDATION" || !strings.Contains(r.details()["next_action"].(string), "--project") {
		t.Fatalf("basic search: %s", r.out)
	}
	e.gl.locked(func() { e.gl.state["advancedSearch"] = true })
	r = e.run("search", "commits", "--query", "login", "--fields", "ref,short_id,web_url")
	got := map[string]any{}
	for _, it := range r.items() {
		got[it["ref"].(string)] = it["web_url"]
	}
	if r.exit != 0 || len(got) != 3 || got["acme/web/-/commit/a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"] == nil {
		t.Fatalf("advanced search: %s", r.out)
	}
	if url, ok := got["fin/payments-service/-/commit/c3d4e5f60718293a4b5c6d7e8f90123456789012"]; !ok || url != nil {
		t.Fatalf("a commit without a web URL is placed by its project ID: %v", got)
	}
	if r = e.run("search", "code", "--query", "login", "--project", "fin/payments-service"); r.exit != 0 || len(r.items()) != 0 {
		t.Fatalf("code search in a project with no match: %s", r.out)
	}
}

// TestSearchCodeRefsAndClippedData: a code hit's ref mirrors its web path, and its
// matched lines are cut to searchDataChars, as the scope says (CLI-042).
func TestSearchCodeRefsAndClippedData(t *testing.T) {
	e := newTestEnv(t)
	e.gl.locked(func() {
		hits := fakeSearchState(e.gl)["acme/web"]
		hits.Blobs = append(hits.Blobs, fakeBlobHit{Path: "docs/long.md", Ref: "main", Line: 1, Data: "login " + strings.Repeat("é", 500)})
	})
	r := e.run("search", "code", "--query", "login", "--project", "acme/web")
	if r.exit != 0 || len(r.items()) != 4 || !strings.Contains(r.data()["scope"].(map[string]any)["data"].(string), "200") {
		t.Fatalf("search code: %s", r.out)
	}
	byRef := map[string]map[string]any{}
	for _, it := range r.items() {
		byRef[it["ref"].(string)] = it
	}
	if it := byRef["acme/web/-/blob/main/src/login.go#L12"]; it == nil || it["path"] != "src/login.go" || it["git_ref"] != "main" ||
		it["start_line"] != json.Number("12") {
		t.Fatalf("a code hit: %v", byRef)
	}
	long := byRef["acme/web/-/blob/main/docs/long.md#L1"]["data"].(string)
	if utf8.RuneCountInString(long) != searchDataChars || !strings.HasSuffix(long, "…") {
		t.Fatalf("data is clipped to %d characters: %d", searchDataChars, utf8.RuneCountInString(long))
	}
	if !hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "data.items[].data") {
		t.Fatalf("file content is untrusted: %s", r.out)
	}
}

// TestSearchCodeFitsTheBudget: with default flags a page of code hits stays within
// 8,000 characters (CLI-042), even with long, quote-heavy matched lines.
func TestSearchCodeFitsTheBudget(t *testing.T) {
	e := newTestEnv(t)
	e.gl.locked(func() {
		hits := fakeSearchState(e.gl)["acme/web"]
		for i := 0; i < 30; i++ {
			hits.Blobs = append(hits.Blobs, fakeBlobHit{Path: "internal/handlers/authentication/login_handler_" + strconv.Itoa(i) + ".go",
				Ref: "main", Line: 100 + i, Data: strings.Repeat(`log("login \"failed\"")`+"\n", 40)})
		}
	})
	r := e.run("search", "code", "--query", "login", "--project", "acme/web")
	if r.exit != 0 || len(r.items()) != searchCodeLimit || r.data()["has_more"] != true || utf8.RuneCountInString(r.out) > 8000 {
		t.Fatalf("a default page of code hits is %d characters: %s", utf8.RuneCountInString(r.out), r.out)
	}
}

// TestSearchGuesses: singular and spelled-out nouns reach the search commands
// (CLI-022).
func TestSearchGuesses(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"search", "project", "--query", "payments"},
		{"search", "issue", "--query", "login", "-p", "acme/web"},
		{"search", "mr", "--query", "login", "--repo", "acme/web"},
		{"search", "merge-requests", "--query", "login", "--project", "acme/web"},
		{"search", "blobs", "--query", "login", "--project", "acme/web"},
		{"search", "commit", "--query", "login", "--project", "acme/web"},
	} {
		if r := e.run(args...); r.exit != 0 || len(r.items()) == 0 {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
	r := e.run("search", "projects", "--query", "payments", "--fields", "+description")
	if it := r.items()[0]; len(it) != 5 || it["description"] == nil {
		t.Fatalf("search projects: short defaults, +description adds it: %v", it)
	}
	if r = e.run("search", "projects", "--query", "payments", "--fields", "visibility"); r.code() != "E_USAGE" {
		t.Fatalf("GitLab's search sends no visibility, so search projects has no such field: %s", r.out)
	}
}

// TestSearchRefsFromWhatGitLabSent: an issue or merge request found across projects
// takes its project from GitLab's full reference, else from its web URL (the host's
// relative URL root left out), else keeps the URL as its ref.
func TestSearchRefsFromWhatGitLabSent(t *testing.T) {
	host := "https://gitlab.example.com/gitlab"
	cases := []struct {
		project, full, webURL, want string
	}{
		{"acme/web", "other/x#9", host + "/other/x/-/issues/9", "acme/web#9"},
		{"", "acme/web#9", "", "acme/web#9"},
		{"", "#9", host + "/acme/sub/web/-/issues/9", "acme/sub/web#9"},
		{"", "", "https://elsewhere.example.com/acme/web/-/work_items/9", "acme/web#9"},
		{"", "acme/web#8", host + "/acme/web/-/issues/9", "acme/web#9"},
		{"", "", "", ""},
		{"", "", "https://gitlab.example.com/gitlab/-/issues/9", "https://gitlab.example.com/gitlab/-/issues/9"},
	}
	for _, c := range cases {
		if got := searchIIDRef(host, c.project, c.full, c.webURL, "#", "/-/issues/", 9); got != c.want {
			t.Errorf("searchIIDRef(%q, %q, %q) = %q, want %q", c.project, c.full, c.webURL, got, c.want)
		}
	}
}
