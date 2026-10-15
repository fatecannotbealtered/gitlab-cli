package cmd

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- fake GitLab

// fakeLabel is a label as the fake keeps it.
type fakeLabel struct {
	ID          int
	Name, Color string
	Description string
	Priority    *int
	Project     bool // false: inherited from a group
	OpenIssues  int
}

// fakeLabels is the label state of the fake: labels by project path.
func fakeLabels(gl *fakeGitLab) map[string][]*fakeLabel {
	return gl.state["labels"].(map[string][]*fakeLabel)
}

func init() {
	addFakeSeed(func(gl *fakeGitLab) {
		one := 1
		gl.state["labels"] = map[string][]*fakeLabel{
			"acme/web": {
				{ID: gl.id(), Name: "bug", Color: "#D73A4A", Description: "Something is broken", Priority: &one, Project: true, OpenIssues: 4},
				{ID: gl.id(), Name: "feature", Color: "#A2EEEF", Description: "New functionality", Project: true, OpenIssues: 2},
				{ID: gl.id(), Name: "needs-review", Color: "#FBCA04", Project: true},
				{ID: gl.id(), Name: "group-wide", Color: "#5319E7", Description: "A label of the acme group"},
			},
			"acme/empty": {},
		}
	})
	addFakeRoutes(
		fakeRoute{"GET /api/v4/projects/{project}/labels", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			p := gl.project(w, r)
			if p == nil {
				return
			}
			search := strings.ToLower(r.URL.Query().Get("search"))
			counts := r.URL.Query().Get("with_counts") == "true"
			var out []map[string]any
			for _, l := range sortedLabels(fakeLabels(gl)[p.Path]) {
				if search == "" || strings.Contains(strings.ToLower(l.Name), search) {
					out = append(out, l.json(counts))
				}
			}
			fakePage(w, r, out)
		}},
		fakeRoute{"GET /api/v4/projects/{project}/labels/{label}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			if l := gl.label(w, r); l != nil {
				fakeJSON(w, 200, l.json(true))
			}
		}},
		fakeRoute{"POST /api/v4/projects/{project}/labels", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			p := gl.project(w, r)
			if p == nil {
				return
			}
			b := fakeBody(r)
			name, _ := b["name"].(string)
			color, _ := b["color"].(string)
			if name == "" || color == "" {
				fakeJSON(w, 400, map[string]any{"message": "name, color are missing"})
				return
			}
			for _, l := range fakeLabels(gl)[p.Path] {
				if l.Name == name {
					fakeJSON(w, 409, map[string]any{"message": "Label already exists"})
					return
				}
			}
			l := &fakeLabel{ID: gl.id(), Name: name, Color: strings.ToUpper(color), Project: true}
			l.Description, _ = b["description"].(string)
			if pr, ok := b["priority"].(float64); ok {
				n := int(pr)
				l.Priority = &n
			}
			fakeLabels(gl)[p.Path] = append(fakeLabels(gl)[p.Path], l)
			fakeJSON(w, 201, l.json(false))
		}},
		fakeRoute{"PUT /api/v4/projects/{project}/labels/{label}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			l := gl.label(w, r)
			if l == nil {
				return
			}
			b := fakeBody(r)
			if v, ok := b["new_name"].(string); ok {
				l.Name = v
			}
			if v, ok := b["color"].(string); ok {
				l.Color = strings.ToUpper(v)
			}
			if v, ok := b["description"].(string); ok {
				l.Description = v
			}
			if v, ok := b["priority"].(float64); ok {
				n := int(v)
				l.Priority = &n
			}
			fakeJSON(w, 200, l.json(false))
		}},
		fakeRoute{"DELETE /api/v4/projects/{project}/labels/{label}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			l := gl.label(w, r)
			if l == nil {
				return
			}
			p := gl.project(w, r)
			kept := fakeLabels(gl)[p.Path][:0]
			for _, x := range fakeLabels(gl)[p.Path] {
				if x != l {
					kept = append(kept, x)
				}
			}
			fakeLabels(gl)[p.Path] = kept
			w.WriteHeader(http.StatusNoContent)
		}},
	)
	addConformanceSamples(map[string][]string{
		"label create": {"--project", "acme/web", "--name", "triage", "--color", "#0e8a16"},
		"label update": {"bug", "--project", "acme/web", "--color", "#ee0701"},
		"label delete": {"needs-review", "--project", "acme/web"},
	})
}

// label finds the {label} of a request by ID or name, like GitLab; it answers 404
// itself when there is none.
func (gl *fakeGitLab) label(w http.ResponseWriter, r *http.Request) *fakeLabel {
	p := gl.project(w, r)
	if p == nil {
		return nil
	}
	key := r.PathValue("label")
	for _, l := range fakeLabels(gl)[p.Path] {
		if strconv.Itoa(l.ID) == key || l.Name == key {
			return l
		}
	}
	fakeJSON(w, 404, map[string]any{"message": "404 Label Not Found"})
	return nil
}

func (l *fakeLabel) json(counts bool) map[string]any {
	out := map[string]any{"id": l.ID, "name": l.Name, "color": l.Color, "text_color": "#FFFFFF", "description": l.Description,
		"priority": l.Priority, "is_project_label": l.Project, "subscribed": false}
	if counts {
		out["open_issues_count"], out["closed_issues_count"], out["open_merge_requests_count"] = l.OpenIssues, 0, 0
	}
	return out
}

func sortedLabels(in []*fakeLabel) []*fakeLabel {
	out := append([]*fakeLabel(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ---------------------------------------------------------------- behaviour matrix

func TestLabelMatrix(t *testing.T) {
	cases := []matrixCase{
		{Path: "label list", Args: []string{"--project", "acme/web"}, EmptyArgs: []string{"--project", "acme/empty"},
			Invalid: []invalidCase{
				{[]string{}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"--project", "acme/web", "--limit", "0"}, "E_USAGE", "invalid_value", "--limit"},
				{[]string{"--project", "acme/web", "--limit", "101"}, "E_USAGE", "invalid_value", "--limit"},
				{[]string{"--project", "acme/web", "--limit", "2", "--offset", "3"}, "E_USAGE", "invalid_value", "--offset"},
				{[]string{"--project", "acme/../web"}, "E_USAGE", "invalid_value", "--project"},
			}},
		{Path: "label get", Args: []string{"bug", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--label"},
				{[]string{"bug"}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"acme/web/-/labels/bug", "--project", "acme/other"}, "E_USAGE", "conflicting_flags", "--project"},
			}},
		{Path: "label create", Args: []string{"--project", "acme/web", "--name", "triage", "--color", "#0e8a16",
			"--description", "Needs triage", "--priority", "3"},
			Role: "reporter", PreviewReads: 2,
			RequestFields: map[string]string{"name": "name", "color": "color", "description": "description", "priority": "priority"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web", "--color", "#000000"}, "E_USAGE", "missing_parameter", "--name"},
				{[]string{"--project", "acme/web", "--name", "x", "--color", "teal"}, "E_VALIDATION", "invalid_value", "--color"},
				{[]string{"--project", "acme/web", "--name", " ", "--color", "red"}, "E_VALIDATION", "invalid_value", "--name"},
				{[]string{"--project", "acme/web", "--name", "x", "--color", "red", "--priority", "-1"}, "E_VALIDATION", "invalid_value", "--priority"},
			}},
		{Path: "label update", Args: []string{"bug", "--project", "acme/web", "--color", "#ee0701", "--description", "Broken behaviour"},
			Role:          "reporter",
			RequestFields: map[string]string{"name": "new_name", "color": "color", "description": "description", "priority": "priority"},
			Invalid: []invalidCase{
				{[]string{"bug", "--project", "acme/web"}, "E_USAGE", "missing_parameter", "--name"},
				{[]string{"--project", "acme/web", "--color", "red"}, "E_USAGE", "missing_parameter", "--label"},
				{[]string{"bug", "--label-id", "5", "--project", "acme/web", "--color", "red"}, "E_USAGE", "conflicting_flags", "--label-id"},
			}},
		{Path: "label delete", Args: []string{"needs-review", "--project", "acme/web"}, Role: "reporter",
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web", "--dangerous"}, "E_USAGE", "missing_parameter", "--label"},
			}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// ---------------------------------------------------------------- focused tests

// TestLabelFormsAreOneTarget: a name with --project, the ref, GitLab's
// acme/web~bug and ~bug read the same label, and bind as one request (CLI-048,
// CLI-054): a dry-run with the ref confirms with the name.
func TestLabelFormsAreOneTarget(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"label", "get", "bug", "--project", "acme/web"},
		{"label", "get", "acme/web/-/labels/bug"},
		{"label", "get", "https://gitlab.example.com/acme/web/-/labels/bug"},
		{"label", "get", "acme/web~bug"},
		{"label", "get", "~bug", "-p", "acme/web"},
		{"labels", "get", "bug", "--repo", "https://gitlab.example.com/acme/web"},
		{"label", "get", "bug", "--project", "1"},
	} {
		r := e.run(args...)
		if r.exit != 0 || r.data()["ref"] != "acme/web/-/labels/bug" || r.data()["name"] != "bug" {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
	tok := e.dryRun("label", "update", "acme/web/-/labels/bug", "--color", "#00ff00")
	if r := e.confirm(tok, "label", "update", "bug", "--project", "acme/web", "--color", "#00ff00"); r.exit != 0 {
		t.Fatalf("the name and the ref must bind alike: %s", r.out)
	}
}

func TestLabelListScopeQueryAndCounts(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("label", "list", "--project", "acme/web", "--query", "BU")
	if r.exit != 0 || len(r.items()) != 1 || r.items()[0]["name"] != "bug" {
		t.Fatalf("query: %s", r.out)
	}
	scope := r.data()["scope"].(map[string]any)
	if scope["project"] != "acme/web" || scope["query"] != "BU" || scope["limit"] != json.Number("20") ||
		!strings.Contains(scope["covers"].(string), "groups") || r.data()["sort"] != "name_asc" || r.data()["total"] != json.Number("1") {
		t.Fatalf("scope, sort and total: %s", r.out)
	}
	if _, has := r.items()[0]["open_issues_count"]; has {
		t.Fatalf("counts only when asked: %s", r.out)
	}
	for _, req := range e.gl.Requests() {
		if strings.Contains(req.Query, "with_counts") {
			t.Fatalf("with_counts sent without being asked: %s", req.Query)
		}
	}
	r = e.run("label", "list", "--project", "acme/web", "--fields", "+open_issues_count")
	if r.exit != 0 || r.items()[0]["open_issues_count"] == nil || r.items()[0]["color"] == nil {
		t.Fatalf("--fields +open_issues_count: %s", r.out)
	}
	if !strings.Contains(e.gl.Requests()[len(e.gl.Requests())-1].Query, "with_counts=true") {
		t.Fatal("the counts must be asked of GitLab")
	}
	if !hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "data.items[].name") {
		t.Fatalf("label names are untrusted: %s", r.out)
	}
}

func TestLabelCreateIsBlockedWhenTheNameExists(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("label", "create", "--project", "acme/web", "--name", "group-wide", "--color", "red", "--dry-run")
	p := r.preview()
	if r.exit != 0 || p["can_apply"] != false || r.data()["confirm_token"] != nil {
		t.Fatalf("an existing name blocks the create: %s", r.out)
	}
	if b := p["blocked_by"].([]any)[0].(map[string]any); b["kind"] != "already_exists" {
		t.Fatalf("blocked_by: %v", b)
	}
	r = e.run("label", "create", "--project", "acme/nowhere", "--name", "x", "--color", "red", "--dry-run")
	if r.code() != "E_NOT_FOUND" || !strings.Contains(r.details()["next_action"].(string), "--project") {
		t.Fatalf("a missing project: %s", r.out)
	}
}

func TestLabelCreateRecordsTheNewRef(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"label", "create", "--project", "acme/web", "--name", "ops", "--color", "Blue", "--idempotency-key", "k-1"}
	r := e.confirm(e.dryRun(args...), args...)
	res := r.data()["result"].(map[string]any)
	if r.exit != 0 || res["ref"] != "acme/web/-/labels/ops" || res["color"] != "#0000FF" {
		t.Fatalf("create: %s", r.out)
	}
	if obs := r.receipt()["observed"].(map[string]any); obs["color"] != "#0000ff" || obs["name"] != "ops" {
		t.Fatalf("observed: %v", obs)
	}
	// The same create with the same key is a replay, answered at the dry-run already
	// (P4): the first receipt and result, no confirm token, nothing sent.
	again := e.run(append(args, "--dry-run")...)
	if again.exit != 0 || again.receipt()["replayed"] != true || again.token() != "" || e.gl.Writes() != 1 {
		t.Fatalf("replay: %s (writes %d)", again.out, e.gl.Writes())
	}
	// Without the key, the label that now exists blocks the plan.
	plain := e.run("label", "create", "--project", "acme/web", "--name", "ops", "--color", "Blue", "--dry-run")
	if plain.exit != 0 || plain.preview()["can_apply"] != false || plain.token() != "" {
		t.Fatalf("a second create without the key: %s", plain.out)
	}
}

func TestLabelUpdateRenamesAndRefusesDrift(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"label", "update", "feature", "--project", "acme/web", "--name", "enhancement"}
	r := e.confirm(e.dryRun(args...), args...)
	res := r.data()["result"].(map[string]any)
	if r.exit != 0 || res["ref"] != "acme/web/-/labels/enhancement" || r.outcome()["verification"] != "passed" {
		t.Fatalf("rename: %s", r.out)
	}
	// A label changed between the dry-run and the confirm is state_drift.
	args = []string{"label", "update", "bug", "--project", "acme/web", "--description", "mine"}
	tok := e.dryRun(args...)
	e.gl.locked(func() { fakeLabels(e.gl)["acme/web"][0].Description = "edited elsewhere" })
	r = e.confirm(tok, args...)
	if r.code() != "E_CONFLICT" || r.details()["conflict"] != "state_drift" || e.gl.Writes() != 1 {
		t.Fatalf("drift: %s", r.out)
	}
}

func TestLabelGroupLabelsAreBlocked(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"label", "update", "group-wide", "--project", "acme/web", "--color", "red", "--dry-run"},
		{"label", "delete", "group-wide", "--project", "acme/web", "--dangerous", "--dry-run"},
	} {
		r := e.run(args...)
		if r.exit != 0 || r.preview()["can_apply"] != false || r.token() != "" {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
}

func TestLabelDeleteChecksTheTargetAndReadsBack(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("label", "delete", "nope", "--project", "acme/web", "--dangerous", "--dry-run")
	if r.code() != "E_NOT_FOUND" || r.exit != 3 {
		t.Fatalf("a missing label is E_NOT_FOUND at the dry-run (CLI-029): %s", r.out)
	}
	r = e.run("label", "delete", "bug", "--project", "acme/web", "--dry-run")
	if r.code() != "E_CONFIRMATION_REQUIRED" || r.details()["problem"] != "dangerous_flag_required" {
		t.Fatalf("delete needs --dangerous: %s", r.out)
	}
	args := []string{"label", "delete", "bug", "--project", "acme/web", "--dangerous"}
	r = e.confirm(e.dryRun(args...), args...)
	if r.exit != 0 || r.outcome()["verification"] != "passed" || r.receipt()["observed"].(map[string]any)["exists"] != false {
		t.Fatalf("delete: %s", r.out)
	}
	if res := r.data()["result"].(map[string]any); res["ref"] != "acme/web/-/labels/bug" || res["deleted"] != true {
		t.Fatalf("result: %v", res)
	}
}

// TestLabelDeprecatedLabelID: 1.x's --label-id keeps working through 2.x, with a
// deprecated notice (CLI-081).
func TestLabelDeprecatedLabelID(t *testing.T) {
	e := newTestEnv(t)
	var id int
	e.gl.locked(func() { id = fakeLabels(e.gl)["acme/web"][1].ID })
	args := []string{"label", "update", "--label-id", strconv.Itoa(id), "--project", "acme/web", "--color", "#123456"}
	r := e.confirm(e.dryRun(args...), args...)
	if r.exit != 0 || r.data()["result"].(map[string]any)["ref"] != "acme/web/-/labels/feature" {
		t.Fatalf("--label-id: %s", r.out)
	}
	notices, _ := r.doc["meta"].(map[string]any)["notices"].([]any)
	if len(notices) == 0 || notices[0].(map[string]any)["type"] != "deprecated" {
		t.Fatalf("a deprecated notice: %s", r.out)
	}
}

func TestParseLabelArg(t *testing.T) {
	cases := map[string][2]string{
		"bug":                        {"", "bug"},
		"~bug":                       {"", "bug"},
		`~"feature request"`:         {"", "feature request"},
		"acme/web~bug":               {"acme/web", "bug"},
		"acme/web/-/labels/bug":      {"acme/web", "bug"},
		"acme/web/-/labels/a%2Fb":    {"acme/web", "a/b"},
		"frontend/react":             {"", "frontend/react"},
		"priority::high":             {"", "priority::high"},
		"https://h/g/s/p/-/labels/x": {"g/s/p", "x"},
	}
	for in, want := range cases {
		p, n, err := parseLabelArg(in)
		if err != nil || p != want[0] || n != want[1] {
			t.Errorf("parseLabelArg(%q) = %q, %q, %v; want %q, %q", in, p, n, err, want[0], want[1])
		}
	}
	for _, bad := range []string{"", "~", `~""`, "https://h/acme/web/-/issues/3"} {
		if _, _, err := parseLabelArg(bad); err == nil {
			t.Errorf("parseLabelArg(%q) should fail", bad)
		}
	}
}
