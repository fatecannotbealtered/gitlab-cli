package cmd

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- fake GitLab

// fakeMilestone is a milestone as the fake keeps it.
type fakeMilestone struct {
	ID, IID                   int
	Title, Description, State string
	StartDate, DueDate        string
	UpdatedAt                 string
}

// fakeMilestones is the milestone state of the fake: milestones by project path,
// oldest first.
func fakeMilestones(gl *fakeGitLab) map[string][]*fakeMilestone {
	return gl.state["milestones"].(map[string][]*fakeMilestone)
}

func init() {
	addFakeSeed(func(gl *fakeGitLab) {
		gl.state["milestones"] = map[string][]*fakeMilestone{
			"acme/web": {
				{ID: gl.id(), IID: 1, Title: "v1.0", State: "closed", DueDate: "2026-06-30", UpdatedAt: gl.now()},
				{ID: gl.id(), IID: 2, Title: "v1.1", Description: "Bug fixes", State: "active", StartDate: "2026-09-01",
					DueDate: "2026-10-31", UpdatedAt: gl.now()},
				{ID: gl.id(), IID: 3, Title: "v2.0", Description: "The next major", State: "active", UpdatedAt: gl.now()},
			},
			"acme/empty": {},
		}
	})
	addFakeRoutes(
		fakeRoute{"GET /api/v4/projects/{project}/milestones", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			p := gl.project(w, r)
			if p == nil {
				return
			}
			q := r.URL.Query()
			var out []map[string]any
			ms := fakeMilestones(gl)[p.Path]
			for i := len(ms) - 1; i >= 0; i-- { // newest first, like GitLab (id_desc)
				m := ms[i]
				switch {
				case q.Get("state") != "" && q.Get("state") != m.State,
					q.Get("title") != "" && q.Get("title") != m.Title,
					q.Get("search") != "" && !strings.Contains(strings.ToLower(m.Title+" "+m.Description), strings.ToLower(q.Get("search"))),
					len(q["iids[]"]) > 0 && !hasString(q["iids[]"], strconv.Itoa(m.IID)):
					continue
				}
				out = append(out, m.json(gl, p.Path))
			}
			fakePage(w, r, out)
		}},
		fakeRoute{"GET /api/v4/projects/{project}/milestones/{id}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			if p, m := gl.milestone(w, r); m != nil {
				fakeJSON(w, 200, m.json(gl, p.Path))
			}
		}},
		fakeRoute{"POST /api/v4/projects/{project}/milestones", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			p := gl.project(w, r)
			if p == nil {
				return
			}
			b := fakeBody(r)
			title, _ := b["title"].(string)
			ms := fakeMilestones(gl)[p.Path]
			for _, m := range ms {
				if m.Title == title {
					fakeJSON(w, 400, map[string]any{"message": map[string]any{"title": []string{"has already been taken"}}})
					return
				}
			}
			m := &fakeMilestone{ID: gl.id(), IID: len(ms) + 1, Title: title, State: "active", UpdatedAt: gl.now()}
			m.Description, _ = b["description"].(string)
			m.StartDate, _ = b["start_date"].(string)
			m.DueDate, _ = b["due_date"].(string)
			fakeMilestones(gl)[p.Path] = append(ms, m)
			fakeJSON(w, 201, m.json(gl, p.Path))
		}},
		fakeRoute{"PUT /api/v4/projects/{project}/milestones/{id}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			p, m := gl.milestone(w, r)
			if m == nil {
				return
			}
			b := fakeBody(r)
			for k, dst := range map[string]*string{"title": &m.Title, "description": &m.Description, "start_date": &m.StartDate, "due_date": &m.DueDate} {
				if v, ok := b[k].(string); ok {
					*dst = v
				}
			}
			switch b["state_event"] {
			case "close":
				m.State = "closed"
			case "activate":
				m.State = "active"
			}
			m.UpdatedAt = gl.now()
			fakeJSON(w, 200, m.json(gl, p.Path))
		}},
	)
	addConformanceSamples(map[string][]string{
		"milestone create": {"--project", "acme/web", "--title", "v3.0", "--due-date", "2026-12-31"},
		"milestone update": {"2", "--project", "acme/web", "--due-date", "2026-11-15"},
		"milestone close":  {"3", "--project", "acme/web"},
	})
}

// milestone finds the {id} (global ID) of a request in its project.
func (gl *fakeGitLab) milestone(w http.ResponseWriter, r *http.Request) (*fakeProject, *fakeMilestone) {
	p := gl.project(w, r)
	if p == nil {
		return nil, nil
	}
	for _, m := range fakeMilestones(gl)[p.Path] {
		if strconv.Itoa(m.ID) == r.PathValue("id") {
			return p, m
		}
	}
	fakeJSON(w, 404, map[string]any{"message": "404 Milestone Not Found"})
	return nil, nil
}

func (m *fakeMilestone) json(gl *fakeGitLab, project string) map[string]any {
	out := map[string]any{"id": m.ID, "iid": m.IID, "project_id": 1, "title": m.Title, "description": m.Description,
		"state": m.State, "start_date": nil, "due_date": nil, "expired": m.DueDate != "" && m.DueDate < "2026-10-12",
		"web_url":    gl.URL + "/" + project + "/-/milestones/" + strconv.Itoa(m.IID),
		"created_at": "2026-01-05T10:00:00.000+01:00", "updated_at": m.UpdatedAt}
	if m.StartDate != "" {
		out["start_date"] = m.StartDate
	}
	if m.DueDate != "" {
		out["due_date"] = m.DueDate
	}
	return out
}

// ---------------------------------------------------------------- behaviour matrix

func TestMilestoneMatrix(t *testing.T) {
	cases := []matrixCase{
		{Path: "milestone list", Args: []string{"--project", "acme/web", "--state", "all"},
			EmptyArgs: []string{"--project", "acme/empty"},
			Invalid: []invalidCase{
				{[]string{}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"--project", "acme/web", "--state", "open"}, "E_USAGE", "invalid_value", "--state"},
				{[]string{"--project", "acme/web", "--limit", "500"}, "E_USAGE", "invalid_value", "--limit"},
			}},
		{Path: "milestone get", Args: []string{"2", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--milestone"},
				{[]string{"2"}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"acme/web/-/milestones/x"}, "E_VALIDATION", "invalid_value", "--milestone"},
			}},
		{Path: "milestone create", Args: []string{"--project", "acme/web", "--title", "v3.0", "--description", "Later",
			"--start-date", "2026-11-01", "--due-date", "2026-12-31"},
			Role: "reporter", RefusedStatus: 400,
			RequestFields: map[string]string{"title": "title", "description": "description", "start_date": "start_date", "due_date": "due_date"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--title"},
				{[]string{"--project", "acme/web", "--title", "x", "--due-date", "31/12/2026"}, "E_VALIDATION", "invalid_value", "--due-date"},
				{[]string{"--project", "acme/web", "--title", "x", "--start-date", "2026-12-01", "--due-date", "2026-11-01"},
					"E_VALIDATION", "invalid_value", "--due-date"},
			}},
		{Path: "milestone update", Args: []string{"2", "--project", "acme/web", "--title", "v1.1.1", "--due-date", "2026-11-15"},
			Role:          "reporter",
			RequestFields: map[string]string{"title": "title", "description": "description", "start_date": "start_date", "due_date": "due_date"},
			Invalid: []invalidCase{
				{[]string{"2", "--project", "acme/web"}, "E_USAGE", "missing_parameter", "--title"},
				{[]string{"2", "--project", "acme/web", "--state-event", "reopen"}, "E_USAGE", "invalid_value", "--state-event"},
			}},
		{Path: "milestone close", Args: []string{"3", "--project", "acme/web"}, Role: "reporter",
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--milestone"},
			}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// ---------------------------------------------------------------- focused tests

// TestMilestoneFormsAreOneTarget: the iid, %iid, the ref, the URL and the title
// read the same milestone and bind as one request (CLI-048, CLI-054).
func TestMilestoneFormsAreOneTarget(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"milestone", "get", "2", "--project", "acme/web"},
		{"milestone", "get", "%2", "--project", "acme/web"},
		{"milestone", "get", "acme/web/-/milestones/2"},
		{"milestone", "get", e.gl.URL + "/acme/web/-/milestones/2"},
		{"milestone", "get", "acme/web%2"},
		{"milestone", "get", "v1.1", "--project", "acme/web"},
		{"milestone", "get", `%"v1.1"`, "--project", "acme/web"},
		{"milestones", "get", "--milestone", "2", "-R", "acme/web"},
	} {
		r := e.run(args...)
		if r.exit != 0 || r.data()["ref"] != "acme/web/-/milestones/2" || r.data()["iid"] != "2" {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
	r := e.run("milestone", "get", "acme/web/-/milestones/2")
	d := r.data()
	if d["start_date"] != "2026-09-01" || d["updated_at"] == nil || d["created_at"] != "2026-01-05T09:00:00Z" {
		t.Fatalf("dates and UTC times: %s", r.out)
	}
	tok := e.dryRun("milestone", "close", "v1.1", "--project", "acme/web")
	if r := e.confirm(tok, "milestone", "close", "acme/web/-/milestones/2"); r.exit != 0 {
		t.Fatalf("the title and the ref must bind alike: %s", r.out)
	}
	if r := e.run("milestone", "get", "v9", "--project", "acme/web"); r.code() != "E_NOT_FOUND" {
		t.Fatalf("an unknown title: %s", r.out)
	}
}

func TestMilestoneListStateAndScope(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("milestone", "list", "--project", "acme/web")
	if r.exit != 0 || len(r.items()) != 2 || r.items()[0]["ref"] != "acme/web/-/milestones/3" {
		t.Fatalf("active milestones, newest first: %s", r.out)
	}
	scope := r.data()["scope"].(map[string]any)
	if scope["state"] != "active" || !strings.Contains(scope["covers"].(string), "groups are not included") || r.data()["sort"] != "id_desc" {
		t.Fatalf("scope and sort: %s", r.out)
	}
	if _, has := r.items()[0]["description"]; has {
		t.Fatalf("description is not a default field: %s", r.out)
	}
	if r = e.run("milestone", "list", "--project", "acme/web", "--state", "closed", "--query", "v1"); len(r.items()) != 1 ||
		r.items()[0]["title"] != "v1.0" || r.items()[0]["expired"] != true {
		t.Fatalf("closed v1: %s", r.out)
	}
}

func TestMilestoneCloseIsBlockedWhenClosed(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("milestone", "close", "1", "--project", "acme/web", "--dry-run")
	if r.exit != 0 || r.preview()["can_apply"] != false || r.token() != "" {
		t.Fatalf("an already closed milestone: %s", r.out)
	}
	args := []string{"milestone", "update", "1", "--project", "acme/web", "--state-event", "activate"}
	r = e.confirm(e.dryRun(args...), args...)
	if r.exit != 0 || r.data()["result"].(map[string]any)["state"] != "active" || r.receipt()["observed"].(map[string]any)["state"] != "active" {
		t.Fatalf("reopen: %s", r.out)
	}
}

func TestMilestoneCreateIsBlockedWhenTheTitleExists(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("milestone", "create", "--project", "acme/web", "--title", "v2.0", "--dry-run")
	if r.exit != 0 || r.preview()["can_apply"] != false {
		t.Fatalf("an existing title blocks the create: %s", r.out)
	}
	args := []string{"milestone", "create", "--project", "acme/web", "--title", "v2.1"}
	r = e.confirm(e.dryRun(args...), args...)
	obs := r.receipt()["observed"].(map[string]any)
	if r.exit != 0 || obs["ref"] != "acme/web/-/milestones/4" || r.data()["result"].(map[string]any)["iid"] != "4" {
		t.Fatalf("create records the new ref: %s", r.out)
	}
}

// TestMilestoneUpdateRefusesDrift: the base version is updated_at (CLI-057).
func TestMilestoneUpdateRefusesDrift(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"milestone", "update", "2", "--project", "acme/web", "--due-date", "2026-11-20"}
	dry := e.run(append(args, "--dry-run")...)
	if base := dry.preview()["base"].(map[string]any); base["source"] != "updated_at" {
		t.Fatalf("base: %s", dry.out)
	}
	e.gl.locked(func() { fakeMilestones(e.gl)["acme/web"][1].UpdatedAt = e.gl.now() })
	r := e.confirm(dry.token(), args...)
	if r.code() != "E_CONFLICT" || r.details()["conflict"] != "state_drift" || e.gl.Writes() != 0 {
		t.Fatalf("drift: %s", r.out)
	}
}

// TestMilestoneDeprecatedMilestoneID: 1.x's --milestone-id (the global ID) keeps
// working, with a deprecated notice (CLI-081).
func TestMilestoneDeprecatedMilestoneID(t *testing.T) {
	e := newTestEnv(t)
	var id int
	e.gl.locked(func() { id = fakeMilestones(e.gl)["acme/web"][2].ID })
	r := e.run("milestone", "get", "--milestone-id", strconv.Itoa(id), "--project", "acme/web")
	if r.exit != 0 || r.data()["ref"] != "acme/web/-/milestones/3" {
		t.Fatalf("--milestone-id: %s", r.out)
	}
	notices, _ := r.doc["meta"].(map[string]any)["notices"].([]any)
	if len(notices) == 0 || notices[0].(map[string]any)["type"] != "deprecated" {
		t.Fatalf("a deprecated notice: %s", r.out)
	}
	if r.data()["id"] != json.Number(strconv.Itoa(id)) && r.data()["id"] != strconv.Itoa(id) {
		t.Fatalf("ids are strings: %v", r.data()["id"])
	}
}

func TestParseMilestoneArg(t *testing.T) {
	cases := []struct {
		in, project, title string
		iid                int
	}{
		{"3", "", "", 3}, {"%3", "", "", 3}, {"acme/web%3", "acme/web", "", 3},
		{"acme/web/-/milestones/3", "acme/web", "", 3}, {"https://h/g/p/-/milestones/7/edit", "g/p", "", 7},
		{"v1.0", "", "v1.0", 0}, {`%"Sprint 12"`, "", "Sprint 12", 0}, {`acme/web%"2026"`, "acme/web", "2026", 0},
		{"50% done", "", "50% done", 0}, {`"quoted"`, "", "quoted", 0},
	}
	for _, c := range cases {
		p, iid, title, err := parseMilestoneArg(c.in)
		if err != nil || p != c.project || iid != c.iid || title != c.title {
			t.Errorf("parseMilestoneArg(%q) = %q %d %q %v", c.in, p, iid, title, err)
		}
	}
	for _, bad := range []string{"", "acme/web/-/milestones/x", "https://h/acme/web/-/issues/3", `%""`} {
		if _, _, _, err := parseMilestoneArg(bad); err == nil {
			t.Errorf("parseMilestoneArg(%q) should fail", bad)
		}
	}
}
