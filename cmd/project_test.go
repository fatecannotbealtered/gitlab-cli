package cmd

import (
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- fake GitLab

// fakeProjectMeta is what the fake knows about a project beyond its ID and path
// (gl.projects, shared). The shared GET /projects/{project} answers with these
// fields too (fakeProjectDetails), as do the list, search and create routes below.
type fakeProjectMeta struct {
	Name, Description, Visibility, DefaultBranch string
	Archived                                     bool
	Created, Activity                            string
	Stars                                        int
	Members                                      map[string]fakeMembership // direct members by username
}

// fakeMembership is one membership: an access level and an optional expiry date.
type fakeMembership struct {
	Level   int
	Expires string
}

// fakeNamespace is a group or a user's namespace.
type fakeNamespace struct {
	ID               int
	Path, Name, Kind string
}

func fakeProjectMetas(gl *fakeGitLab) map[string]*fakeProjectMeta {
	return gl.state["projectMeta"].(map[string]*fakeProjectMeta)
}

func fakeGroupMembers(gl *fakeGitLab) map[string]map[string]fakeMembership {
	return gl.state["groupMembers"].(map[string]map[string]fakeMembership)
}

func fakeNamespaces(gl *fakeGitLab) []*fakeNamespace {
	return gl.state["namespaces"].([]*fakeNamespace)
}

// fakeMetaOf is a project's metadata; a project another group added has a default.
func fakeMetaOf(gl *fakeGitLab, path string) *fakeProjectMeta {
	if m, ok := fakeProjectMetas(gl)[path]; ok {
		return m
	}
	return &fakeProjectMeta{Name: path[strings.LastIndex(path, "/")+1:], Visibility: "private", Members: map[string]fakeMembership{}}
}

// fakeMembersOf is a project's members by username: direct ones, and with all the
// members of its parent groups too, each at their highest level (GitLab's
// /members/all).
func fakeMembersOf(gl *fakeGitLab, path string, all bool) map[string]fakeMembership {
	out := map[string]fakeMembership{}
	for u, m := range fakeMetaOf(gl, path).Members {
		out[u] = m
	}
	if all {
		segs := strings.Split(path, "/")
		for i := 1; i < len(segs); i++ {
			for u, m := range fakeGroupMembers(gl)[strings.Join(segs[:i], "/")] {
				if cur, ok := out[u]; !ok || m.Level > cur.Level {
					out[u] = m
				}
			}
		}
	}
	return out
}

func (gl *fakeGitLab) me(r *http.Request) fakeUser {
	return gl.users[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
}

func fakeProjectJSON(gl *fakeGitLab, p *fakeProject, simple bool) map[string]any {
	m := fakeMetaOf(gl, p.Path)
	i := strings.LastIndex(p.Path, "/")
	var nsNames []string
	segs := strings.Split(p.Path[:i], "/")
	for k := range segs {
		name := segs[k]
		for _, n := range fakeNamespaces(gl) {
			if n.Path == strings.Join(segs[:k+1], "/") {
				name = n.Name
			}
		}
		nsNames = append(nsNames, name)
	}
	out := map[string]any{"id": p.ID, "name": m.Name, "name_with_namespace": strings.Join(append(nsNames, m.Name), " / "),
		"path": p.Path[i+1:], "path_with_namespace": p.Path, "description": m.Description, "default_branch": m.DefaultBranch,
		"web_url": gl.URL + "/" + p.Path, "ssh_url_to_repo": "git@gitlab.example.com:" + p.Path + ".git",
		"http_url_to_repo": gl.URL + "/" + p.Path + ".git", "star_count": m.Stars, "forks_count": 0,
		"created_at": m.Created, "last_activity_at": m.Activity, "empty_repo": fakeRepoIsEmpty(gl, p.Path)}
	if !simple {
		out["visibility"], out["archived"] = m.Visibility, m.Archived
	}
	return out
}

func init() {
	fakeProjectDetails = func(gl *fakeGitLab, p *fakeProject) map[string]any { return fakeProjectJSON(gl, p, false) }
	addFakeSeed(func(gl *fakeGitLab) {
		// Two projects named payments, as in the eval's ambiguous-project task (CLI-048).
		gl.projects = append(gl.projects, &fakeProject{ID: gl.id(), Path: "fin/payments-service"},
			&fakeProject{ID: gl.id(), Path: "legacy/payments"})
		gl.state["projectMeta"] = map[string]*fakeProjectMeta{
			"acme/web": {Name: "Web", Description: "The storefront; it calls the payments service", Visibility: "private",
				DefaultBranch: "main", Created: "2026-01-10T08:00:00.000Z", Activity: "2026-10-11T09:30:00.000+02:00", Stars: 3,
				Members: map[string]fakeMembership{"carol": {Level: 40}, "dave": {Level: 20}}},
			"acme/empty": {Name: "Empty", Visibility: "internal", Created: "2026-02-01T08:00:00.000Z",
				Activity: "2026-02-01T08:00:00.000Z", Members: map[string]fakeMembership{}},
			// Every member comes through the group fin: the project 1.2.17 reported empty (CLI-041).
			"fin/payments-service": {Name: "Payments Service", Description: "Card payments", Visibility: "internal",
				DefaultBranch: "main", Created: "2026-03-01T08:00:00.000Z", Activity: "2026-10-01T12:00:00.000Z",
				Members: map[string]fakeMembership{}},
			"legacy/payments": {Name: "payments", Description: "The old payments monolith", Visibility: "private",
				DefaultBranch: "master", Archived: true, Created: "2025-05-01T08:00:00.000Z", Activity: "2025-12-24T08:00:00.000Z",
				Members: map[string]fakeMembership{"erin": {Level: 30}}},
		}
		gl.state["groupMembers"] = map[string]map[string]fakeMembership{
			"acme": {"dave": {Level: 40}, "erin": {Level: 30, Expires: "2026-12-31"}},
			"fin":  {"dave": {Level: 30}, "erin": {Level: 50}},
		}
		gl.state["namespaces"] = []*fakeNamespace{
			{ID: 501, Path: "acme", Name: "Acme", Kind: "group"}, {ID: 502, Path: "acme/platform", Name: "Platform", Kind: "group"},
			{ID: 503, Path: "fin", Name: "Finance", Kind: "group"}, {ID: 504, Path: "legacy", Name: "Legacy", Kind: "group"},
			{ID: 507, Path: "carol", Name: "Carol Danvers", Kind: "user"}, {ID: 508, Path: "dave", Name: "Dave Lister", Kind: "user"},
		}
	})
	addFakeRoutes(
		fakeRoute{"GET /api/v4/projects", fakeProjectList},
		fakeRoute{"POST /api/v4/projects", fakeProjectCreate},
		fakeRoute{"GET /api/v4/projects/{project}/members/all", fakeProjectMembers(true)},
		fakeRoute{"GET /api/v4/projects/{project}/members", fakeProjectMembers(false)},
		fakeRoute{"GET /api/v4/namespaces/{id}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			key := r.PathValue("id")
			for _, n := range fakeNamespaces(gl) {
				if strconv.Itoa(n.ID) == key || strings.EqualFold(n.Path, key) {
					fakeJSON(w, 200, map[string]any{"id": n.ID, "name": n.Name, "path": n.Path[strings.LastIndex(n.Path, "/")+1:],
						"kind": n.Kind, "full_path": n.Path})
					return
				}
			}
			fakeJSON(w, 404, map[string]any{"message": "404 Namespace Not Found"})
		}},
	)
	addConformanceSamples(map[string][]string{
		"project create": {"--name", "New App", "--namespace", "acme"},
	})
}

// fakeProjectList answers GET /projects like GitLab: search matches the path, name
// or description; membership, owned and visibility filter; order_by path or
// created_at (the default, newest first).
func fakeProjectList(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	me := gl.me(r)
	search := strings.ToLower(q.Get("search"))
	var ps []*fakeProject
	for _, p := range gl.projects {
		m := fakeMetaOf(gl, p.Path)
		access := fakeMembersOf(gl, p.Path, true)[me.Username].Level
		switch {
		case search != "" && !strings.Contains(strings.ToLower(p.Path[strings.LastIndex(p.Path, "/")+1:]+"\n"+m.Name+"\n"+m.Description), search):
		case q.Get("visibility") != "" && q.Get("visibility") != m.Visibility:
		case q.Get("membership") == "true" && access == 0:
		case q.Get("owned") == "true" && !strings.HasPrefix(p.Path, me.Username+"/") && access < 50:
		default:
			ps = append(ps, p)
		}
	}
	sort.SliceStable(ps, func(i, j int) bool {
		if q.Get("order_by") == "path" {
			return ps[i].Path[strings.LastIndex(ps[i].Path, "/")+1:] < ps[j].Path[strings.LastIndex(ps[j].Path, "/")+1:]
		}
		return fakeMetaOf(gl, ps[i].Path).Created > fakeMetaOf(gl, ps[j].Path).Created
	})
	out := make([]map[string]any, 0, len(ps))
	for _, p := range ps {
		out = append(out, fakeProjectJSON(gl, p, q.Get("simple") == "true"))
	}
	fakePage(w, r, out)
}

// fakeProjectCreate answers POST /projects: the account's own namespace unless
// namespace_id names another; a taken path is 400, as GitLab answers.
func fakeProjectCreate(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	me := gl.me(r)
	b := fakeBody(r)
	name, _ := b["name"].(string)
	path, _ := b["path"].(string)
	if name == "" || path == "" {
		fakeJSON(w, 400, map[string]any{"message": "name, path are missing"})
		return
	}
	ns := me.Username
	if id, ok := b["namespace_id"].(float64); ok && id != 0 {
		ns = ""
		for _, n := range fakeNamespaces(gl) {
			if n.ID == int(id) {
				ns = n.Path
			}
		}
		if ns == "" {
			fakeJSON(w, 404, map[string]any{"message": "404 Namespace Not Found"})
			return
		}
	}
	full := ns + "/" + path
	for _, p := range gl.projects {
		if strings.EqualFold(p.Path, full) {
			fakeJSON(w, 400, map[string]any{"message": map[string]any{"path": []string{"has already been taken"}}})
			return
		}
	}
	p := &fakeProject{ID: gl.id(), Path: full}
	gl.projects = append(gl.projects, p)
	visibility, _ := b["visibility"].(string)
	if visibility == "" {
		visibility = "private"
	}
	description, _ := b["description"].(string)
	now := gl.now()
	fakeProjectMetas(gl)[full] = &fakeProjectMeta{Name: name, Description: description, Visibility: visibility, Created: now,
		Activity: now, Members: map[string]fakeMembership{me.Username: {Level: 50}}}
	fakeJSON(w, 201, fakeProjectJSON(gl, p, false))
}

// fakeProjectMembers answers GET /projects/{project}/members (direct) and
// /members/all (effective), with GitLab's query and user_ids[] filters.
func fakeProjectMembers(all bool) func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	return func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
		p := gl.project(w, r)
		if p == nil {
			return
		}
		members := fakeMembersOf(gl, p.Path, all)
		q := r.URL.Query()
		query, ids := strings.ToLower(q.Get("query")), q["user_ids[]"]
		var out []map[string]any
		for _, u := range gl.people {
			m, ok := members[u.Username]
			switch {
			case !ok:
			case query != "" && !strings.Contains(strings.ToLower(u.Username+" "+u.Name), query):
			case len(ids) > 0 && !slices.Contains(ids, strconv.Itoa(u.ID)):
			default:
				var expires any
				if m.Expires != "" {
					expires = m.Expires
				}
				out = append(out, map[string]any{"id": u.ID, "username": u.Username, "name": u.Name, "state": "active",
					"access_level": m.Level, "expires_at": expires, "web_url": gl.URL + "/" + u.Username})
			}
		}
		fakePage(w, r, out)
	}
}

// ---------------------------------------------------------------- behaviour matrix

func TestProjectMatrix(t *testing.T) {
	cases := []matrixCase{
		{Path: "project list", EmptyArgs: []string{"--query", "zzz-none"},
			Invalid: []invalidCase{
				{[]string{"--limit", "0"}, "E_USAGE", "invalid_value", "--limit"},
				{[]string{"--visibility", "secret"}, "E_USAGE", "invalid_value", "--visibility"},
				{[]string{"--query", "web", "--search", "api"}, "E_USAGE", "conflicting_flags", "--search"},
			}},
		{Path: "project get", Args: []string{"acme/web"},
			Invalid: []invalidCase{
				{[]string{}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"acme/../web"}, "E_USAGE", "invalid_value", "--project"},
			}},
		{Path: "project members", Args: []string{"acme/web"}, EmptyArgs: []string{"acme/web", "--query", "zzz-none"},
			Invalid: []invalidCase{
				{[]string{}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"acme/web", "--offset", "3"}, "E_USAGE", "invalid_value", "--offset"},
			}},
		{Path: "project create", Args: []string{"--name", "New App", "--namespace", "acme", "--description", "A new app",
			"--visibility", "internal"},
			Role: "maintainer", PreviewReads: 2,
			RequestFields: map[string]string{"name": "name", "description": "description", "visibility": "visibility"},
			Invalid: []invalidCase{
				{[]string{"--namespace", "acme"}, "E_USAGE", "missing_parameter", "--name"},
				{[]string{"--name", " ", "--namespace", "acme"}, "E_VALIDATION", "invalid_value", "--name"},
				{[]string{"--name", "x", "--namespace", "acme", "--path", "-bad"}, "E_VALIDATION", "invalid_value", "--path"},
				{[]string{"--name", "!!!", "--namespace", "acme"}, "E_VALIDATION", "invalid_value", "--path"},
				{[]string{"--name", "x", "--namespace", "acme/../web"}, "E_VALIDATION", "invalid_value", "--namespace"},
				{[]string{"--name", "x", "--namespace", "acme", "--visibility", "secret"}, "E_USAGE", "invalid_value", "--visibility"},
				{[]string{"--name", "x", "--namespace", "acme", "--namespace-id", "501"}, "E_USAGE", "conflicting_flags", "--namespace-id"},
				{[]string{"--name", "x", "--namespace-id", "0"}, "E_VALIDATION", "invalid_value", "--namespace-id"},
			}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// ---------------------------------------------------------------- focused tests

// TestProjectFormsAreOneProject: the path, the web URL, the numeric ID and the name
// read the same project (CLI-048).
func TestProjectFormsAreOneProject(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"project", "get", "acme/web"},
		{"project", "get", "https://gitlab.example.com/acme/web/-/merge_requests/5"},
		{"project", "get", "1"},
		{"project", "get", "web"},
		{"project", "get", "--project", "Web"},
		{"projects", "view", "acme/web"},
	} {
		r := e.run(args...)
		if r.exit != 0 || r.data()["ref"] != "acme/web" || r.data()["namespace"] != "acme" || r.data()["name"] != "Web" ||
			r.data()["visibility"] != "private" || r.data()["default_branch"] != "main" {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
	// "payments service" has both words of one project's name.
	if r := e.run("project", "get", "payments service"); r.exit != 0 || r.data()["ref"] != "fin/payments-service" {
		t.Fatalf("a two-word name: %s", r.out)
	}
}

// TestProjectNameMatchingSeveralIsAmbiguous is the eval's "the payments project"
// (CLI-048): two projects have payments in their name, so every command that takes
// a project answers E_AMBIGUOUS with both refs, never a silent pick. acme/web only
// mentions payments in its description and is no candidate.
func TestProjectNameMatchingSeveralIsAmbiguous(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"project", "get", "payments"},
		{"project", "members", "payments"},
		{"member", "payments"},
		{"search", "issues", "--query", "login", "--project", "Payments"},
	} {
		r := e.run(args...)
		if r.code() != "E_AMBIGUOUS" || r.exit != 2 || r.details()["parameter"] != "--project" || r.details()["dispatch_state"] != "not_started" {
			t.Fatalf("%v: %s", args, r.out)
		}
		var refs []string
		for _, c := range r.details()["candidates"].([]any) {
			refs = append(refs, c.(map[string]any)["ref"].(string))
		}
		if !slices.Equal(refs, []string{"fin/payments-service", "legacy/payments"}) {
			t.Fatalf("%v: candidates %v", args, refs)
		}
		if !hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "error.details.candidates[].label") {
			t.Fatalf("%v: candidate labels are untrusted: %s", args, r.out)
		}
		if strings.Contains(r.errObj()["message"].(string), "payments") {
			t.Fatalf("%v: the message echoes the input (CLI-017): %s", args, r.out)
		}
	}
	if r := e.run("project", "get", "nothing-like-this"); r.code() != "E_NOT_FOUND" || r.details()["parameter"] != "--project" ||
		!strings.Contains(r.details()["next_action"].(string), "project list --query nothing-like-this") {
		t.Fatalf("an unknown name: %s", r.out)
	}
	// A name lookup reads GitLab's search with the name, limited fields, by path.
	var lookup fakeRequest
	for _, req := range e.gl.Requests() {
		if req.Path == "/api/v4/projects" {
			lookup = req
		}
	}
	if !strings.Contains(lookup.Query, "simple=true") || !strings.Contains(lookup.Query, "per_page=100") {
		t.Fatalf("name lookup query: %s", lookup.Query)
	}
}

// TestProjectNameLookupWithTooManyHits: when more hits remain than the lookup reads,
// one match proves nothing, so the agent is asked for the path; several are
// candidates, flagged as not all of them.
func TestProjectNameLookupWithTooManyHits(t *testing.T) {
	e := newTestEnv(t)
	e.gl.locked(func() {
		add := func(path, name string) {
			e.gl.projects = append(e.gl.projects, &fakeProject{ID: e.gl.id(), Path: path})
			fakeProjectMetas(e.gl)[path] = &fakeProjectMeta{Name: name, Description: "mentions storefront",
				Created: "2024-01-01T00:00:00.000Z", Members: map[string]fakeMembership{}}
		}
		add("shop/a-storefront", "Storefront") // the one name match, on the first page
		for i := 0; i < 300; i++ {
			add("bulk/repo-"+strconv.Itoa(i), "repo "+strconv.Itoa(i))
		}
	})
	r := e.run("project", "get", "storefront")
	if r.code() != "E_USAGE" || r.details()["parameter"] != "--project" || !strings.Contains(r.details()["next_action"].(string), "--membership") {
		t.Fatalf("one match among too many hits: %s", r.out)
	}
	if n := len(e.gl.Requests()); n != 3 {
		t.Fatalf("the lookup reads at most 3 pages, read %d", n)
	}
	r = e.run("project", "get", "repo")
	if r.code() != "E_AMBIGUOUS" || r.details()["more_candidates"] != true || len(r.details()["candidates"].([]any)) != 10 ||
		!strings.Contains(r.details()["next_action"].(string), "project list --query repo") {
		t.Fatalf("many matches: %s", r.out)
	}
}

// TestProjectMembersAreEffective (CLI-041): members inherited from groups are
// listed, each with where its access comes from, and the scope says the list is
// effective. fin/payments-service has no direct member at all: 1.2.17 listed nothing.
func TestProjectMembersAreEffective(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("project", "members", "fin/payments-service")
	scope, _ := r.data()["scope"].(map[string]any)
	if r.exit != 0 || len(r.items()) != 2 || scope["membership"] != "effective: direct and inherited from groups" ||
		scope["project"] != "fin/payments-service" || !strings.Contains(scope["covers"].(string), "shared with") {
		t.Fatalf("members through the group: %s", r.out)
	}
	if _, has := r.data()["sort"]; has {
		t.Fatalf("GitLab documents no order for this list: %s", r.out)
	}
	got := map[string]string{}
	for _, it := range r.items() {
		got[it["ref"].(string)] = it["access"].(string) + "/" + it["membership"].(string)
	}
	if got["@dave"] != "developer/inherited" || got["@erin"] != "owner/inherited" {
		t.Fatalf("members: %v", got)
	}
	// acme/web: carol is a direct maintainer; dave is a direct reporter but a
	// maintainer through acme, so the access shown is inherited; erin's expires.
	r = e.run("project", "members", "acme/web", "--fields", "all")
	got = map[string]string{}
	for _, it := range r.items() {
		got[it["username"].(string)] = it["access"].(string) + "/" + it["membership"].(string) + "/" + it["access_level"].(json.Number).String()
		if it["username"] == "erin" && it["expires_on"] != "2026-12-31" {
			t.Fatalf("expires_at: %v", it)
		}
	}
	if got["carol"] != "maintainer/direct/40" || got["dave"] != "maintainer/inherited/40" || got["erin"] != "developer/inherited/30" {
		t.Fatalf("members of acme/web: %v", got)
	}
	reqs := e.gl.Requests()
	if last := reqs[len(reqs)-1]; !strings.HasSuffix(last.Path, "/members") || !strings.Contains(last.Query, "user_ids%5B%5D=7") {
		t.Fatalf("the direct lookup names the listed users: %+v", last)
	}
	if !hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "data.items[].name") {
		t.Fatalf("member names are untrusted: %s", r.out)
	}
}

// TestProjectMembershipCostsARequestOnlyWhenShown: membership needs the direct
// members, read only when the output shows the field.
func TestProjectMembershipCostsARequestOnlyWhenShown(t *testing.T) {
	for fields, reads := range map[string]int{"": 2, "ref,access": 1, "+web_url": 2, "ref,membership": 2, "all": 2} {
		e := newTestEnv(t)
		args := []string{"project", "members", "acme/web"}
		if fields != "" {
			args = append(args, "--fields", fields)
		}
		if r := e.run(args...); r.exit != 0 || len(e.gl.Requests()) != reads {
			t.Fatalf("--fields %q: %d requests, want %d: %s", fields, len(e.gl.Requests()), reads, r.out)
		}
	}
}

// TestMemberGuessesReachProjectMembers: the fleet's measured guesses for project
// members (CLI-022): member and members at the root, with or without list, and
// project member.
func TestMemberGuessesReachProjectMembers(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"member", "acme/web"},
		{"members", "acme/web"},
		{"member", "list", "--project", "acme/web"},
		{"members", "list", "-p", "acme/web"},
		{"project", "member", "acme/web"},
		{"projects", "members", "--repo", "acme/web"},
	} {
		if r := e.run(args...); r.exit != 0 || r.data()["scope"].(map[string]any)["membership"] == nil || len(r.items()) != 3 {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
	for _, args := range [][]string{{"projects", "ls"}, {"project", "show", "acme/web"}} {
		if r := e.run(args...); r.exit != 0 {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
	if r := e.run("project", "new", "--name", "Guess", "--namespace", "acme", "--dry-run"); r.exit != 0 || r.token() == "" {
		t.Fatalf("project new: %s", r.out)
	}
}

// TestProjectGroupVerbGuessesAreAnswered: list and search stand in for each other
// (CLI-022): the guess is answered with the right command, flags kept.
func TestProjectGroupVerbGuessesAreAnswered(t *testing.T) {
	e := newTestEnv(t)
	for _, c := range []struct {
		args       []string
		suggestion string
	}{
		{[]string{"project", "search", "--query", "payments"}, "gitlab-cli project list"},
		{[]string{"user", "list", "--query", "carol"}, "gitlab-cli user search"},
		{[]string{"users", "find", "--query", "carol"}, "gitlab-cli user search"},
	} {
		r := e.run(c.args...)
		if r.details()["problem"] != "unknown_command" || stringsOf(r.details()["did_you_mean"])[0] != c.suggestion ||
			!strings.Contains(r.details()["next_action"].(string), "--query") {
			t.Fatalf("%v: %s", c.args, r.out)
		}
	}
}

func TestProjectListScopeFiltersAndDeprecatedSearch(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("project", "list")
	var refs []string
	for _, it := range r.items() {
		refs = append(refs, it["ref"].(string))
	}
	if r.exit != 0 || r.data()["sort"] != "created_at_desc" || !slices.Equal(refs, []string{"fin/payments-service", "acme/empty", "acme/web", "legacy/payments"}) {
		t.Fatalf("every project, newest first: %v %s", refs, r.out)
	}
	if req := e.gl.Requests()[0]; !strings.Contains(req.Query, "order_by=created_at") || !strings.Contains(req.Query, "sort=desc") {
		t.Fatalf("the stated order is asked for: %s", req.Query)
	}
	if covers := r.data()["scope"].(map[string]any)["covers"].(string); !strings.Contains(covers, "archived") {
		t.Fatalf("scope.covers: %s", covers)
	}
	if it := r.items()[3]; it["archived"] != true || it["visibility"] != "private" || it["last_activity_at"] != "2025-12-24T08:00:00Z" {
		t.Fatalf("legacy/payments: %v", it)
	}
	if it := r.items()[2]; it["ref"] != "acme/web" || it["last_activity_at"] != "2026-10-11T07:30:00Z" {
		t.Fatalf("times are UTC: %v", it)
	}
	r = e.run("project", "list", "--membership")
	if len(r.items()) != 1 || r.items()[0]["ref"] != "acme/web" || r.data()["scope"].(map[string]any)["membership"] != true {
		t.Fatalf("--membership: %s", r.out)
	}
	r = e.run("project", "list", "--query", "payments", "--visibility", "internal")
	scope := r.data()["scope"].(map[string]any)
	if len(r.items()) != 1 || scope["query"] != "payments" || scope["visibility"] != "internal" {
		t.Fatalf("--query and --visibility: %s", r.out)
	}
	r = e.run("project", "list", "--search", "payments")
	notices, _ := r.doc["meta"].(map[string]any)["notices"].([]any)
	if r.exit != 0 || len(r.items()) != 3 || len(notices) == 0 || notices[0].(map[string]any)["type"] != "deprecated" {
		t.Fatalf("--search still works, with a deprecated notice: %s", r.out)
	}
	if r = e.run("project", "list", "--owned"); r.exit != 0 || len(r.items()) != 0 || !strings.Contains(r.data()["scope"].(map[string]any)["covers"].(string), "owns") {
		t.Fatalf("--owned: %s", r.out)
	}
}

// TestProjectCreateInTheAccountsNamespace: without --namespace the project goes to
// the account's own namespace, whose path (the username) is read before the gate to
// build the target key; the created project's ref is observed.
func TestProjectCreateInTheAccountsNamespace(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"project", "create", "--name", "Team Notes"}
	dry := e.run(append(args, "--dry-run")...)
	p := dry.preview()
	if dry.exit != 0 || p["target"].(map[string]any)["key"] != "carol/team-notes" || p["action"] != "Create project carol/team-notes" {
		t.Fatalf("dry-run: %s", dry.out)
	}
	r := e.confirm(dry.token(), args...)
	res := r.data()["result"].(map[string]any)
	if r.exit != 0 || res["ref"] != "carol/team-notes" || res["visibility"] != "private" || r.outcome()["verification"] != "passed" {
		t.Fatalf("confirm: %s", r.out)
	}
	if obs := r.receipt()["observed"].(map[string]any); obs["ref"] != "carol/team-notes" || obs["name"] != "Team Notes" {
		t.Fatalf("observed: %v", obs)
	}
	req, _ := e.gl.lastWrite()
	if req.Body["path"] != "team-notes" || req.Body["namespace_id"] != nil {
		t.Fatalf("the path is always sent, no namespace for the account's own: %v", req.Body)
	}
	if r = e.run("project", "list", "--owned"); len(r.items()) != 1 || r.items()[0]["ref"] != "carol/team-notes" {
		t.Fatalf("the new project is owned: %s", r.out)
	}
	if r = e.run("project", "get", "carol/team-notes"); r.exit != 0 {
		t.Fatalf("the read-back command: %s", r.out)
	}
}

func TestProjectCreateChecksPathAndNamespace(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("project", "create", "--name", "Web", "--namespace", "acme", "--dry-run")
	if r.exit != 0 || r.preview()["can_apply"] != false || r.token() != "" ||
		r.preview()["blocked_by"].([]any)[0].(map[string]any)["kind"] != "already_exists" {
		t.Fatalf("an existing path blocks the create: %s", r.out)
	}
	r = e.run("project", "create", "--name", "X", "--namespace", "nowhere", "--dry-run")
	if r.code() != "E_NOT_FOUND" || !strings.Contains(r.details()["next_action"].(string), "--namespace") {
		t.Fatalf("a missing namespace: %s", r.out)
	}
	// A URL and the path are one namespace; a subgroup works; the path is given.
	args := []string{"project", "create", "--name", "Payments API v2!", "--namespace", "https://gitlab.example.com/acme/platform"}
	tok := e.dryRun(args...)
	r = e.confirm(tok, "project", "create", "--name", "Payments API v2!", "--namespace", "acme/platform")
	if r.exit != 0 || r.data()["result"].(map[string]any)["ref"] != "acme/platform/payments-api-v2" {
		t.Fatalf("the namespace URL binds as its path: %s", r.out)
	}
	if req, _ := e.gl.lastWrite(); req.Body["namespace_id"] != float64(502) {
		t.Fatalf("namespace_id: %v", req.Body)
	}
	if r = e.run("project", "create", "--name", "Docs", "--path", "handbook", "--namespace", "acme", "--dry-run"); r.preview()["target"].(map[string]any)["key"] != "acme/handbook" {
		t.Fatalf("--path: %s", r.out)
	}
}

// TestProjectCreateDeprecatedNamespaceID: 1.x's --namespace-id keeps working, read
// once to find the namespace's path for the target key (CLI-081).
func TestProjectCreateDeprecatedNamespaceID(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"project", "create", "--name", "Ledger", "--namespace-id", "503"}
	r := e.confirm(e.dryRun(args...), args...)
	notices, _ := r.doc["meta"].(map[string]any)["notices"].([]any)
	if r.exit != 0 || r.data()["result"].(map[string]any)["ref"] != "fin/ledger" || len(notices) == 0 ||
		notices[0].(map[string]any)["type"] != "deprecated" {
		t.Fatalf("--namespace-id: %s", r.out)
	}
	if r = e.run("project", "create", "--name", "X", "--namespace-id", "999", "--dry-run"); r.code() != "E_NOT_FOUND" {
		t.Fatalf("an unknown namespace ID: %s", r.out)
	}
}

// TestProjectCreateReplaysWithTheKey: the same create with the same key is answered
// with the first receipt at the dry-run (CLI-062, P4); without it, the project that
// now exists blocks the plan.
func TestProjectCreateReplaysWithTheKey(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"project", "create", "--name", "Ops", "--namespace", "acme", "--idempotency-key", "proj-ops-1"}
	e.confirm(e.dryRun(args...), args...)
	again := e.run(append(args, "--dry-run")...)
	if again.exit != 0 || again.receipt()["replayed"] != true || again.token() != "" || e.gl.Writes() != 1 {
		t.Fatalf("replay: %s (writes %d)", again.out, e.gl.Writes())
	}
	plain := e.run("project", "create", "--name", "Ops", "--namespace", "acme", "--dry-run")
	if plain.preview()["can_apply"] != false || plain.token() != "" {
		t.Fatalf("a second create without the key: %s", plain.out)
	}
}

// TestProjectCreateForbiddenNamesTheGroupRole: a 403 on the create names the role
// (CLI-014) and speaks of the group, not of a project that does not exist yet.
func TestProjectCreateForbiddenNamesTheGroupRole(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"project", "create", "--name", "X", "--namespace", "acme"}
	tok := e.dryRun(args...)
	e.gl.fail(fakeFault{Method: "WRITE", Status: 403, Times: 1})
	r := e.confirm(tok, args...)
	if r.code() != "E_FORBIDDEN" || r.details()["required"] != "maintainer" || !strings.Contains(r.details()["next_action"].(string), "group") {
		t.Fatalf("403: %s", r.out)
	}
}

func TestProjectSlugPathAndNamespace(t *testing.T) {
	for name, want := range map[string]string{"Payments API v2!": "payments-api-v2", "my_app.v2": "my-app-v2", "  Notes ": "notes",
		"Café Menu": "caf-menu", "!!!": "", "支付": ""} {
		if got := projectSlug(name); got != want {
			t.Errorf("projectSlug(%q) = %q, want %q", name, got, want)
		}
	}
	for path, ok := range map[string]bool{"web": true, "my-app.v2": true, "A_b": true, "-bad": false, "bad-": false, "a--b": false,
		"a..b": false, "app.git": false, "feed.atom": false, "": false, "a b": false} {
		if projectPathOK(path) != ok {
			t.Errorf("projectPathOK(%q) = %v", path, !ok)
		}
	}
	for raw, want := range map[string]string{"acme": "acme", "acme/platform/": "acme/platform", " https://h/acme/platform ": "acme/platform",
		"https://h/groups/acme/-/issues": "acme", "acme/../x": "", "https://h": "", "a b": ""} {
		if got, _ := projectNamespacePath(raw); got != want {
			t.Errorf("projectNamespacePath(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestProjectNameMatches(t *testing.T) {
	cases := []struct {
		name  string
		names []string
		want  bool
	}{
		{"payments", []string{"Payments Service", "payments-service"}, true},
		{"payments", []string{"payments"}, true},
		{"Payments Service", []string{"payments-service"}, true},
		{"service payments", []string{"payments-service"}, false},
		{"pay", []string{"Payments Service", "payments-service"}, false},
		{"payments", []string{"repayments"}, false},
		{"dave", []string{"Dave Lister", "dlister"}, true},
		{"", []string{"x"}, false},
		{"--", []string{"x"}, false},
	}
	for _, c := range cases {
		if got := projectNameMatches(c.name, c.names...); got != c.want {
			t.Errorf("projectNameMatches(%q, %v) = %v", c.name, c.names, got)
		}
	}
	if projectIsName("acme/web") || projectIsName("42") || projectIsName("") || !projectIsName("payments service") {
		t.Error("projectIsName")
	}
	if got := projectClip("abcdef", 4); got != "abc…" || projectClip("abc", 4) != "abc" {
		t.Errorf("projectClip = %q", got)
	}
}

// TestProjectCommandMapLines pins the project group's lines of the Skill's command
// map (SKL-002), in registration order, between ci and auth.
func TestProjectCommandMapLines(t *testing.T) {
	got := newTestApp(map[string]string{}).CommandMap()
	lines := []string{
		"- `project`: list, get, create, members\n",
		"- `user`: me, get, search\n",
		"- `search`: projects, issues, mrs, code, commits\n",
		"- `auth`: ",
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
