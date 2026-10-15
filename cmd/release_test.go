package cmd

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- fake GitLab

func init() {
	addFakeRoutes(
		fakeRoute{"GET /api/v4/projects/{project}/releases", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			p, rp := gl.repoOf(w, r)
			if p == nil {
				return
			}
			out := []map[string]any{}
			for _, rel := range rp.Releases {
				out = append(out, rp.releaseJSON(gl, p.Path, rel))
			}
			fakePage(w, r, out)
		}},
		fakeRoute{"GET /api/v4/projects/{project}/releases/{tag}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			if p, rp, rel := gl.repoRelease(w, r); rel != nil {
				fakeJSON(w, 200, rp.releaseJSON(gl, p.Path, rel))
			}
		}},
		fakeRoute{"POST /api/v4/projects/{project}/releases", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			p, rp := gl.repoOf(w, r)
			if p == nil {
				return
			}
			b := fakeBody(r)
			tag, _ := b["tag_name"].(string)
			for _, rel := range rp.Releases {
				if rel.Tag == tag {
					fakeJSON(w, 409, map[string]any{"message": "Release already exists"})
					return
				}
			}
			if _, ok := rp.Tags[tag]; !ok {
				ref, _ := b["ref"].(string)
				c := rp.resolve(ref)
				if ref == "" || c == nil {
					fakeJSON(w, 422, map[string]any{"message": "Ref is not specified or invalid"})
					return
				}
				rp.Tags[tag] = c.ID
			}
			rel := &fakeRepoRelease{Tag: tag, Author: gl.repoUser(r).Username, ReleasedAt: gl.now()}
			rel.Name, _ = b["name"].(string)
			if rel.Name == "" {
				rel.Name = tag
			}
			rel.Description, _ = b["description"].(string)
			if !gl.repoReleaseMilestones(w, p.Path, b, rel) {
				return
			}
			rp.Releases = append([]*fakeRepoRelease{rel}, rp.Releases...)
			fakeJSON(w, 201, rp.releaseJSON(gl, p.Path, rel))
		}},
		fakeRoute{"PUT /api/v4/projects/{project}/releases/{tag}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			p, rp, rel := gl.repoRelease(w, r)
			if rel == nil {
				return
			}
			b := fakeBody(r)
			if v, ok := b["name"].(string); ok {
				rel.Name = v
			}
			if v, ok := b["description"].(string); ok {
				rel.Description = v
			}
			if !gl.repoReleaseMilestones(w, p.Path, b, rel) {
				return
			}
			fakeJSON(w, 200, rp.releaseJSON(gl, p.Path, rel))
		}},
		fakeRoute{"DELETE /api/v4/projects/{project}/releases/{tag}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			p, rp, rel := gl.repoRelease(w, r)
			if rel == nil {
				return
			}
			kept := rp.Releases[:0]
			for _, x := range rp.Releases {
				if x != rel {
					kept = append(kept, x)
				}
			}
			rp.Releases = kept
			fakeJSON(w, 200, rp.releaseJSON(gl, p.Path, rel))
		}},
	)
	addConformanceSamples(map[string][]string{
		"release create": {"v1.1", "--project", "acme/web", "--name", "Version 1.1", "--description", "Bug fixes"},
		"release update": {"v1.0", "--project", "acme/web", "--name", "Version 1.0.0"},
		"release delete": {"v0.9", "--project", "acme/web"},
	})
}

// repoRelease finds the {tag} release; it answers 404 itself when there is none.
func (gl *fakeGitLab) repoRelease(w http.ResponseWriter, r *http.Request) (*fakeProject, *fakeRepo, *fakeRepoRelease) {
	p, rp := gl.repoOf(w, r)
	if p == nil {
		return nil, nil, nil
	}
	for _, rel := range rp.Releases {
		if rel.Tag == r.PathValue("tag") {
			return p, rp, rel
		}
	}
	fakeJSON(w, 404, map[string]any{"message": "404 Not Found"})
	return nil, nil, nil
}

// repoReleaseMilestones sets the body's milestones on rel; GitLab refuses titles the
// project has no milestone for.
func (gl *fakeGitLab) repoReleaseMilestones(w http.ResponseWriter, project string, b map[string]any, rel *fakeRepoRelease) bool {
	raw, has := b["milestones"].([]any)
	if !has {
		return true
	}
	rel.Milestones = nil
	for _, t := range raw {
		title, _ := t.(string)
		found := false
		for _, m := range fakeMilestones(gl)[project] {
			found = found || m.Title == title
		}
		if !found {
			fakeJSON(w, 400, map[string]any{"message": "Milestone(s) not found: " + title})
			return false
		}
		rel.Milestones = append(rel.Milestones, title)
	}
	return true
}

func (rp *fakeRepo) releaseJSON(gl *fakeGitLab, project string, rel *fakeRepoRelease) map[string]any {
	ms := []map[string]any{}
	for i, t := range rel.Milestones {
		ms = append(ms, map[string]any{"id": 900 + i, "iid": i + 1, "title": t})
	}
	author := map[string]any{"username": rel.Author, "name": rel.Author}
	for _, u := range gl.people {
		if u.Username == rel.Author {
			author["name"] = u.Name
		}
	}
	return map[string]any{"tag_name": rel.Tag, "name": rel.Name, "description": rel.Description, "created_at": rel.ReleasedAt,
		"released_at": rel.ReleasedAt, "upcoming_release": false, "author": author,
		"commit": rp.Commits[rp.Tags[rel.Tag]].json(gl, project, false), "milestones": ms,
		"commit_path": "/" + project + "/-/commit/" + rp.Tags[rel.Tag], "tag_path": "/" + project + "/-/tags/" + rel.Tag,
		"assets": map[string]any{"count": 2, "sources": []any{}},
		"_links": map[string]any{"self": gl.URL + "/" + project + "/-/releases/" + rel.Tag}}
}

// ---------------------------------------------------------------- behaviour matrix

func TestReleaseMatrix(t *testing.T) {
	cases := []matrixCase{
		{Path: "release list", Args: []string{"--project", "acme/web"}, EmptyArgs: []string{"--project", "acme/empty"},
			Invalid: []invalidCase{
				{[]string{}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"--project", "acme/web", "--limit", "0"}, "E_USAGE", "invalid_value", "--limit"},
			}},
		{Path: "release get", Args: []string{"v1.0", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--tag"},
				{[]string{"v1 0", "--project", "acme/web"}, "E_VALIDATION", "invalid_value", "--tag"},
				{[]string{"v1.0"}, "E_USAGE", "missing_parameter", "--project"},
			}},
		{Path: "release create", Args: []string{"v1.1", "--project", "acme/web", "--name", "Version 1.1", "--description", "Bug fixes"},
			Role: "developer", PreviewReads: 2,
			RequestFields: map[string]string{"name": "name", "description": "description", "tag_name": "tag_name"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--tag"},
				{[]string{"v1..1", "--project", "acme/web"}, "E_VALIDATION", "invalid_value", "--tag"},
				{[]string{"v1.1", "--project", "acme/web", "--name", " "}, "E_VALIDATION", "invalid_value", "--name"},
				{[]string{"v1.1", "--project", "acme/web", "--description", "x", "--description-file", "notes.md"}, "E_USAGE", "conflicting_flags", "--description-file"},
			}},
		{Path: "release update", Args: []string{"v1.0", "--project", "acme/web", "--name", "Version 1.0.0", "--description", "Stable."},
			Role: "developer", PreviewReads: 1,
			RequestFields: map[string]string{"name": "name", "description": "description"},
			Invalid: []invalidCase{
				{[]string{"v1.0", "--project", "acme/web"}, "E_USAGE", "missing_parameter", "--name"},
				{[]string{"--project", "acme/web", "--name", "x"}, "E_USAGE", "missing_parameter", "--tag"},
			}},
		{Path: "release delete", Args: []string{"v0.9", "--project", "acme/web"}, Role: "developer", PreviewReads: 1,
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web", "--dangerous"}, "E_USAGE", "missing_parameter", "--tag"},
			}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// ---------------------------------------------------------------- focused tests

// TestReleaseFormsAreOneTarget: the tag, the release ref, the tag's ref and the web
// URLs read the same release and bind as one request (CLI-048, CLI-054).
func TestReleaseFormsAreOneTarget(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"release", "get", "v1.0", "--project", "acme/web"},
		{"release", "get", "acme/web/-/releases/v1.0"},
		{"release", "get", "acme/web/-/tags/v1.0"},
		{"release", "get", e.gl.URL + "/acme/web/-/releases/v1.0"},
		{"release", "get", "--tag", "v1.0", "--project", "1"},
	} {
		r := e.run(args...)
		if r.exit != 0 || r.data()["ref"] != "acme/web/-/releases/v1.0" || r.data()["commit_sha"] != repoSeedSHA(2) {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
	r := e.run("release", "get", "v1.0", "-p", "acme/web")
	d := r.data()
	if d["author"] != "@carol" || repoJSON(d["milestones"]) != `["v1.0"]` || d["asset_count"] != repoNumber(2) ||
		d["description"] != "First stable release." || d["description_truncated"] != false {
		t.Fatalf("release get: %s", r.out)
	}
	untrusted := stringsOf(r.doc["meta"].(map[string]any)["untrusted"])
	for _, p := range []string{"data.description", "data.name", "data.tag_name", "data.ref"} {
		if !hasString(untrusted, p) {
			t.Fatalf("%s is untrusted: %v", p, untrusted)
		}
	}
	tok := e.dryRun("release", "update", "acme/web/-/releases/v1.0", "--name", "One")
	if r := e.confirm(tok, "release", "update", "v1.0", "--project", "acme/web", "--name", "One"); r.exit != 0 {
		t.Fatalf("the tag and the ref must bind alike: %s", r.out)
	}
}

// TestReleaseListCarriesNoNotes: release lists leave the notes out (CLI-043) and are
// ordered newest release first.
func TestReleaseListCarriesNoNotes(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("release", "list", "--project", "acme/web", "--fields", "all")
	if r.exit != 0 || len(r.items()) != 2 || r.items()[0]["tag_name"] != "v1.0" || r.data()["sort"] != "released_at_desc" {
		t.Fatalf("list: %s", r.out)
	}
	if _, has := r.items()[0]["description"]; has {
		t.Fatalf("no notes in a list: %s", r.out)
	}
	if r := e.run("release", "list", "--project", "acme/web", "--fields", "description"); r.details()["problem"] != "invalid_value" {
		t.Fatalf("description is not a list field: %s", r.out)
	}
}

// TestReleaseCreateTagHandling: an existing tag is used as it is; a missing one is
// created from --ref at the commit the preview showed; without --ref the plan is
// blocked; an existing release blocks too.
func TestReleaseCreateTagHandling(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("release", "create", "v1.1", "--project", "acme/web", "--dry-run")
	changes := r.preview()["changes"].([]any)
	if r.exit != 0 || changes[0].(map[string]any)["field"] != "commit_sha" || changes[0].(map[string]any)["after"] != repoSeedSHA(3) {
		t.Fatalf("the existing tag's commit: %s", r.out)
	}
	r = e.run("release", "create", "v2.0", "--project", "acme/web", "--dry-run")
	if r.exit != 0 || r.preview()["can_apply"] != false || r.token() != "" ||
		r.preview()["blocked_by"].([]any)[0].(map[string]any)["kind"] != "tag_missing" {
		t.Fatalf("a missing tag without --ref: %s", r.out)
	}
	r = e.run("release", "create", "v1.0", "--project", "acme/web", "--dry-run")
	if r.preview()["can_apply"] != false || r.preview()["blocked_by"].([]any)[0].(map[string]any)["kind"] != "already_exists" {
		t.Fatalf("an existing release: %s", r.out)
	}
	if r := e.run("release", "create", "v2.0", "--project", "acme/web", "--ref", "nowhere", "--dry-run"); r.code() != "E_NOT_FOUND" ||
		r.details()["parameter"] != "--ref" {
		t.Fatalf("a missing --ref: %s", r.out)
	}
	args := []string{"release", "create", "v2.0", "--project", "acme/web", "--ref", "develop", "--name", "Two", "--idempotency-key", "rel-2"}
	tok := e.dryRun(args...)
	e.repoPush("acme/web", "develop", "x.txt", "moved on") // develop moves on; the tag starts where the preview said
	r = e.confirm(tok, args...)
	if r.code() != "E_CONFLICT" || r.details()["conflict"] != "state_drift" || e.gl.Writes() != 0 {
		t.Fatalf("a --ref that moved after the dry-run is state_drift: %s", r.out)
	}
	r = e.confirm(e.dryRun(args...), args...)
	res := r.data()["result"].(map[string]any)
	if r.exit != 0 || res["ref"] != "acme/web/-/releases/v2.0" || res["commit_sha"] != e.repoBranchHead("acme/web", "develop") ||
		r.outcome()["verification"] != "passed" {
		t.Fatalf("create with a new tag: %s", r.out)
	}
	if again := e.run(append(args, "--dry-run")...); again.receipt()["replayed"] != true || e.gl.Writes() != 1 {
		t.Fatalf("a repeat with the same key is a replay: %s", again.out)
	}
}

// TestReleaseMilestonesAndClearing: milestone titles are checked before anything is
// sent; an empty --milestone or --description clears them (CLI-029); the notes
// from a file bind like the same text given inline (CLI-054).
func TestReleaseMilestonesAndClearing(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("release", "update", "v1.0", "--project", "acme/web", "--milestone", "v9.9", "--dry-run")
	if r.code() != "E_NOT_FOUND" || r.details()["parameter"] != "--milestone" {
		t.Fatalf("an unknown milestone: %s", r.out)
	}
	args := []string{"release", "update", "v1.0", "--project", "acme/web", "--milestone", "v1.1,v1.0"}
	r = e.confirm(e.dryRun(args...), args...)
	if r.exit != 0 || repoJSON(r.receipt()["observed"].(map[string]any)["milestones"]) != `["v1.0","v1.1"]` {
		t.Fatalf("milestones: %s", r.out)
	}
	args = []string{"release", "update", "v1.0", "--project", "acme/web", "--milestone", "", "--description", ""}
	dry := e.run(append(args, "--dry-run")...)
	if !hasString(stringsOf(dry.doc["meta"].(map[string]any)["untrusted"]), "data.preview.changes[].before") {
		t.Fatalf("the current values are untrusted: %s", dry.out)
	}
	r = e.confirm(dry.token(), args...)
	if res := r.data()["result"].(map[string]any); r.exit != 0 || res["description"] != "" || repoJSON(res["milestones"]) != "[]" {
		t.Fatalf("clearing: %s", r.out)
	}
	notes := filepath.Join(e.dir, "notes.md")
	if err := os.WriteFile(notes, []byte("Shiny."), 0o600); err != nil {
		t.Fatal(err)
	}
	tok := e.dryRun("release", "update", "v1.0", "--project", "acme/web", "--description-file", notes)
	if r := e.confirm(tok, "release", "update", "v1.0", "--project", "acme/web", "--description", "Shiny."); r.exit != 0 {
		t.Fatalf("--description-file and the same --description are one request: %s", r.out)
	}
}

// TestReleaseGetLongNotes: notes longer than one response carries are cut, flagged
// and named in a notice; --format raw prints them whole (CLI-042).
func TestReleaseGetLongNotes(t *testing.T) {
	e := newTestEnv(t)
	long := strings.Repeat("A line of release notes.\n", 400)
	e.gl.locked(func() { fakeRepos(e.gl)["acme/web"].Releases[0].Description = long })
	r := e.run("release", "get", "v1.0", "--project", "acme/web")
	notices, _ := r.doc["meta"].(map[string]any)["notices"].([]any)
	if r.exit != 0 || r.data()["description_truncated"] != true || len(r.out) > 8000 || len(notices) != 1 {
		t.Fatalf("long notes: %d %s", len(r.out), r.out[:300])
	}
	if raw := e.run("release", "get", "v1.0", "--project", "acme/web", "--format", "raw"); raw.exit != 0 || raw.out != long {
		t.Fatalf("raw notes: %q", raw.out[:80])
	}
}

// TestReleaseDeleteKeepsTheTag: a missing release is E_NOT_FOUND at the dry-run;
// delete needs --dangerous; the tag survives the release.
func TestReleaseDeleteKeepsTheTag(t *testing.T) {
	e := newTestEnv(t)
	if r := e.run("release", "delete", "v1.1", "--project", "acme/web", "--dangerous", "--dry-run"); r.code() != "E_NOT_FOUND" {
		t.Fatalf("a missing release: %s", r.out)
	}
	if r := e.run("release", "delete", "v0.9", "--project", "acme/web", "--dry-run"); r.details()["problem"] != "dangerous_flag_required" {
		t.Fatalf("delete needs --dangerous: %s", r.out)
	}
	args := []string{"release", "delete", "v0.9", "--project", "acme/web", "--dangerous"}
	r := e.confirm(e.dryRun(args...), args...)
	if r.exit != 0 || r.outcome()["verification"] != "passed" || r.data()["result"].(map[string]any)["deleted"] != true {
		t.Fatalf("delete: %s", r.out)
	}
	if t2 := e.run("repo", "commit", "get", "v0.9", "--project", "acme/web"); t2.exit != 0 || t2.data()["sha"] != repoSeedSHA(1) {
		t.Fatalf("the tag stays: %s", t2.out)
	}
}
