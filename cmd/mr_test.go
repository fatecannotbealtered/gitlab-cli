package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- fake GitLab

// fakeMR is a merge request as the fake keeps it, with its approvals, diff and
// discussions (every note belongs to one, as in GitLab).
type fakeMR struct {
	ID, IID                    int
	Project                    string
	Title, Description, State  string
	Source, Target, Author     string
	Assignees, Reviewers       []string
	Labels                     []string
	SHA, BaseSHA               string
	Pipeline                   string // head pipeline status; "" none
	Status                     string // detailed_merge_status
	Conflicts                  bool
	Squash, RemoveSource       bool
	ApprovedBy                 []string
	Created, Updated, MergedAt string
	Discussions                []*fakeDiscussion
	Diffs                      []fakeDiff
}

type fakeDiscussion struct {
	ID         string
	Individual bool
	Notes      []*fakeNote
}

type fakeNote struct {
	ID                           int
	Author, Body                 string
	System, Resolvable, Resolved bool
	Created, Updated             string
	Path                         string
	NewLine                      int
}

type fakeDiff struct {
	Old, New, Diff            string
	NewFile, Deleted, Renamed bool
}

// fakeMRs is the merge request state of the fake: merge requests by project path.
func fakeMRs(gl *fakeGitLab) map[string][]*fakeMR { return gl.state["mrs"].(map[string][]*fakeMR) }

// mrSeed is the fake's merge requests in acme/web (acme/empty has none):
//
//	!5 opened, mergeable, pipeline success; assigned to carol, reviewed by erin;
//	   comments 9001 (carol), thread d5b (erin, unresolved, src/client.go:42),
//	   thread d5c (resolved); three changed files
//	!6 opened, pipeline failed, approved by carol; a system note and a comment
//	!7 closed; !8 a draft, pipeline running; !9 merged
func mrSeed(gl *fakeGitLab) {
	day := func(d int) string { return fmt.Sprintf("2026-10-%02dT09:00:00.000Z", d) }
	five := &fakeMR{ID: 5005, IID: 5, Project: "acme/web", Title: "Add retry to the payment client",
		Description: "Retries idempotent calls.\n\nCloses #12.", State: "opened", Source: "feature/retry", Target: "main",
		Author: "dave", Assignees: []string{"carol"}, Reviewers: []string{"erin"}, Labels: []string{"backend"},
		SHA: "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a", BaseSHA: "b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0",
		Pipeline: "success", Status: "mergeable", Created: day(1), Updated: day(10),
		Discussions: []*fakeDiscussion{
			{ID: "d5a", Individual: true, Notes: []*fakeNote{{ID: 9001, Author: "carol", Body: "Looks good overall.", Created: day(2), Updated: day(2)}}},
			{ID: "d5b", Notes: []*fakeNote{
				{ID: 9002, Author: "erin", Body: "This can be nil when the config is missing.", Resolvable: true, Created: day(3), Updated: day(3),
					Path: "src/client.go", NewLine: 42},
				{ID: 9003, Author: "dave", Body: "Good catch, fixing.", Resolvable: true, Created: day(4), Updated: day(4),
					Path: "src/client.go", NewLine: 42}}},
			{ID: "d5c", Notes: []*fakeNote{{ID: 9004, Author: "erin", Body: "Rename this?", Resolvable: true, Resolved: true,
				Created: day(5), Updated: day(6), Path: "src/retry.go", NewLine: 7}}},
		},
		Diffs: []fakeDiff{
			{Old: "src/client.go", New: "src/client.go", Diff: "@@ -40,3 +40,5 @@\n func call() {\n-\treturn do()\n+\tif cfg == nil {\n+\t\treturn errNoConfig\n+\t}\n+\treturn retry(do)\n"},
			{Old: "src/retry.go", New: "src/retry.go", NewFile: true, Diff: "@@ -0,0 +1,2 @@\n+package client\n+func retry() {}\n"},
			{Old: "docs/old.md", New: "docs/old.md", Deleted: true, Diff: "@@ -1,2 +0,0 @@\n-# Old\n-gone\n"},
		}}
	six := &fakeMR{ID: 5006, IID: 6, Project: "acme/web", Title: "Fix flaky login test", State: "opened", Source: "fix/login-flake",
		Target: "main", Author: "erin", Reviewers: []string{"carol"}, Labels: []string{"bug"},
		SHA: "6a6a6a6a6a6a6a6a6a6a6a6a6a6a6a6a6a6a6a6a", BaseSHA: "b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0",
		Pipeline: "failed", Status: "mergeable", ApprovedBy: []string{"carol"}, Created: day(2), Updated: day(11),
		Discussions: []*fakeDiscussion{
			{ID: "d6s", Individual: true, Notes: []*fakeNote{{ID: 9005, Author: "erin", Body: "added 1 commit", System: true, Created: day(3), Updated: day(3)}}},
			{ID: "d6a", Individual: true, Notes: []*fakeNote{{ID: 9006, Author: "dave", Body: "Is this still flaky?", Created: day(4), Updated: day(4)}}},
		}}
	seven := &fakeMR{ID: 5007, IID: 7, Project: "acme/web", Title: "Drop the legacy exporter", State: "closed", Source: "chore/drop-exporter",
		Target: "main", Author: "carol", SHA: "7a7a7a7a7a7a7a7a7a7a7a7a7a7a7a7a7a7a7a7a", Pipeline: "failed", Status: "not_open",
		Created: day(3), Updated: day(8)}
	eight := &fakeMR{ID: 5008, IID: 8, Project: "acme/web", Title: "Draft: Rework the settings page", State: "opened",
		Source: "feature/settings", Target: "main", Author: "carol", Labels: []string{"ui"},
		SHA: "8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a8a", BaseSHA: "b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0",
		Pipeline: "running", Status: "draft_status", Created: day(4), Updated: day(9)}
	nine := &fakeMR{ID: 5009, IID: 9, Project: "acme/web", Title: "Bump dependencies", State: "merged", Source: "deps/bump",
		Target: "main", Author: "dave", SHA: "9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a9a", Pipeline: "success", Status: "not_open",
		Created: day(1), Updated: day(7), MergedAt: day(7)}
	gl.state["mrs"] = map[string][]*fakeMR{"acme/web": {five, six, seven, eight, nine}, "acme/empty": {}}
}

func init() {
	addFakeSeed(mrSeed)
	base := "/api/v4/projects/{project}/merge_requests"
	addFakeRoutes(
		fakeRoute{"GET " + base, fakeMRList},
		fakeRoute{"POST " + base, fakeMRCreate},
		fakeRoute{"GET " + base + "/{iid}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			if m := gl.mr(w, r); m != nil {
				fakeJSON(w, 200, m.json(gl, true))
			}
		}},
		fakeRoute{"PUT " + base + "/{iid}", fakeMRUpdate},
		fakeRoute{"PUT " + base + "/{iid}/merge", fakeMRMerge},
		fakeRoute{"GET " + base + "/{iid}/approvals", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			if m := gl.mr(w, r); m != nil {
				fakeJSON(w, 200, m.approvals(gl))
			}
		}},
		fakeRoute{"POST " + base + "/{iid}/approve", fakeMRApprove},
		fakeRoute{"POST " + base + "/{iid}/unapprove", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			m := gl.mr(w, r)
			if m == nil {
				return
			}
			me := gl.caller(r)
			if !hasString(m.ApprovedBy, me) {
				fakeJSON(w, 404, map[string]any{"message": "404 Not Found"})
				return
			}
			m.ApprovedBy = without(m.ApprovedBy, me)
			fakeJSON(w, 201, map[string]any{})
		}},
		fakeRoute{"GET " + base + "/{iid}/diffs", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			m := gl.mr(w, r)
			if m == nil {
				return
			}
			out := []map[string]any{}
			for _, d := range m.Diffs {
				out = append(out, map[string]any{"old_path": d.Old, "new_path": d.New, "diff": d.Diff, "new_file": d.NewFile,
					"deleted_file": d.Deleted, "renamed_file": d.Renamed, "too_large": false, "collapsed": false})
			}
			fakePage(w, r, out)
		}},
		fakeRoute{"GET " + base + "/{iid}/raw_diffs", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			m := gl.mr(w, r)
			if m == nil {
				return
			}
			var b strings.Builder
			for _, d := range m.Diffs {
				fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n%s", d.Old, d.New, d.Old, d.New, d.Diff)
			}
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(b.String()))
		}},
		fakeRoute{"GET " + base + "/{iid}/notes", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			m := gl.mr(w, r)
			if m == nil {
				return
			}
			var notes []*fakeNote
			for _, d := range m.Discussions {
				notes = append(notes, d.Notes...)
			}
			sort.SliceStable(notes, func(i, j int) bool { return notes[i].Created < notes[j].Created })
			out := []map[string]any{}
			for _, n := range notes {
				out = append(out, n.json(gl, m))
			}
			fakePage(w, r, out)
		}},
		fakeRoute{"POST " + base + "/{iid}/notes", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			m := gl.mr(w, r)
			if m == nil {
				return
			}
			body, _ := fakeBody(r)["body"].(string)
			if body == "" {
				fakeJSON(w, 400, map[string]any{"message": "400 Bad request - body is missing"})
				return
			}
			now := gl.now()
			n := &fakeNote{ID: gl.id(), Author: gl.caller(r), Body: body, Created: now, Updated: now}
			m.Discussions = append(m.Discussions, &fakeDiscussion{ID: fmt.Sprintf("dn%d", n.ID), Individual: true, Notes: []*fakeNote{n}})
			fakeJSON(w, 201, n.json(gl, m))
		}},
		fakeRoute{"GET " + base + "/{iid}/notes/{note}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			if m, _, n := gl.note(w, r); n != nil {
				fakeJSON(w, 200, n.json(gl, m))
			}
		}},
		fakeRoute{"DELETE " + base + "/{iid}/notes/{note}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			m, d, n := gl.note(w, r)
			if n == nil {
				return
			}
			if n.System {
				fakeJSON(w, 403, map[string]any{"message": "403 Forbidden"})
				return
			}
			var kept []*fakeNote
			for _, x := range d.Notes {
				if x != n {
					kept = append(kept, x)
				}
			}
			d.Notes = kept
			if len(kept) == 0 {
				var ds []*fakeDiscussion
				for _, x := range m.Discussions {
					if x != d {
						ds = append(ds, x)
					}
				}
				m.Discussions = ds
			}
			w.WriteHeader(http.StatusNoContent)
		}},
		fakeRoute{"GET " + base + "/{iid}/discussions", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			m := gl.mr(w, r)
			if m == nil {
				return
			}
			out := []map[string]any{}
			for _, d := range m.Discussions {
				out = append(out, d.json(gl, m))
			}
			fakePage(w, r, out)
		}},
		fakeRoute{"POST " + base + "/{iid}/discussions", fakeMRDiscussionCreate},
		fakeRoute{"GET " + base + "/{iid}/discussions/{discussion}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			if m, d := gl.discussion(w, r); d != nil {
				fakeJSON(w, 200, d.json(gl, m))
			}
		}},
		fakeRoute{"PUT " + base + "/{iid}/discussions/{discussion}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			m, d := gl.discussion(w, r)
			if d == nil {
				return
			}
			resolved, ok := fakeBody(r)["resolved"].(bool)
			resolvable := false
			for _, n := range d.Notes {
				if n.Resolvable {
					resolvable = true
					n.Resolved, n.Updated = resolved, gl.now()
				}
			}
			if !ok || !resolvable {
				fakeJSON(w, 400, map[string]any{"message": "400 Bad request - discussion is not resolvable"})
				return
			}
			fakeJSON(w, 200, d.json(gl, m))
		}},
		fakeRoute{"POST " + base + "/{iid}/discussions/{discussion}/notes", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			m, d := gl.discussion(w, r)
			if d == nil {
				return
			}
			body, _ := fakeBody(r)["body"].(string)
			now := gl.now()
			n := &fakeNote{ID: gl.id(), Author: gl.caller(r), Body: body, Resolvable: !d.Individual, Created: now, Updated: now}
			d.Notes = append(d.Notes, n)
			fakeJSON(w, 201, n.json(gl, m))
		}},
	)
	addConformanceSamples(map[string][]string{
		"mr create":    {"--project", "acme/web", "--title", "Add caching", "--source-branch", "feature/cache", "--target-branch", "main"},
		"mr update":    {"5", "--project", "acme/web", "--title", "Add retries to the payment client"},
		"mr merge":     {"5", "--project", "acme/web"},
		"mr close":     {"5", "--project", "acme/web"},
		"mr reopen":    {"7", "--project", "acme/web"},
		"mr approve":   {"5", "--project", "acme/web"},
		"mr unapprove": {"6", "--project", "acme/web"},
	})
}

// mr finds the {iid} of a request; it answers 404 itself when there is none.
func (gl *fakeGitLab) mr(w http.ResponseWriter, r *http.Request) *fakeMR {
	p := gl.project(w, r)
	if p == nil {
		return nil
	}
	for _, m := range fakeMRs(gl)[p.Path] {
		if strconv.Itoa(m.IID) == r.PathValue("iid") {
			return m
		}
	}
	fakeJSON(w, 404, map[string]any{"message": "404 Not found"})
	return nil
}

func (gl *fakeGitLab) note(w http.ResponseWriter, r *http.Request) (*fakeMR, *fakeDiscussion, *fakeNote) {
	m := gl.mr(w, r)
	if m == nil {
		return nil, nil, nil
	}
	for _, d := range m.Discussions {
		for _, n := range d.Notes {
			if strconv.Itoa(n.ID) == r.PathValue("note") {
				return m, d, n
			}
		}
	}
	fakeJSON(w, 404, map[string]any{"message": "404 Note Not Found"})
	return nil, nil, nil
}

func (gl *fakeGitLab) discussion(w http.ResponseWriter, r *http.Request) (*fakeMR, *fakeDiscussion) {
	m := gl.mr(w, r)
	if m == nil {
		return nil, nil
	}
	for _, d := range m.Discussions {
		if d.ID == r.PathValue("discussion") {
			return m, d
		}
	}
	fakeJSON(w, 404, map[string]any{"message": "404 Discussion Not Found"})
	return nil, nil
}

// caller is the username of the request's token.
func (gl *fakeGitLab) caller(r *http.Request) string {
	return gl.users[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")].Username
}

func (gl *fakeGitLab) userJSON(name string) any {
	for _, u := range gl.people {
		if u.Username == name {
			return u.json(gl)
		}
	}
	return nil
}

func (gl *fakeGitLab) usernameByID(id int) string {
	for _, u := range gl.people {
		if u.ID == id {
			return u.Username
		}
	}
	return ""
}

func (m *fakeMR) json(gl *fakeGitLab, full bool) map[string]any {
	users := func(names []string) []any {
		out := []any{}
		for _, n := range names {
			out = append(out, gl.userJSON(n))
		}
		return out
	}
	labels := append([]string{}, m.Labels...)
	sort.Strings(labels)
	out := map[string]any{"id": m.ID, "iid": m.IID, "project_id": 1, "title": m.Title, "description": m.Description, "state": m.State,
		"draft": mrIsDraftTitle(m.Title), "source_branch": m.Source, "target_branch": m.Target,
		"web_url": gl.URL + "/" + m.Project + "/-/merge_requests/" + strconv.Itoa(m.IID), "author": gl.userJSON(m.Author),
		"assignees": users(m.Assignees), "reviewers": users(m.Reviewers), "labels": labels, "sha": m.SHA,
		"merge_status": "can_be_merged", "detailed_merge_status": m.Status, "has_conflicts": m.Conflicts, "squash": m.Squash,
		"force_remove_source_branch": m.RemoveSource, "created_at": m.Created, "updated_at": m.Updated, "merged_at": nil}
	if len(m.Assignees) > 0 {
		out["assignee"] = gl.userJSON(m.Assignees[0])
	}
	if m.MergedAt != "" {
		out["merged_at"] = m.MergedAt
	}
	if full {
		out["head_pipeline"] = nil
		if m.Pipeline != "" {
			out["head_pipeline"] = map[string]any{"id": 700 + m.IID, "status": m.Pipeline}
		}
		out["diff_refs"] = map[string]any{"base_sha": m.BaseSHA, "start_sha": m.BaseSHA, "head_sha": m.SHA}
		if m.BaseSHA == "" {
			out["diff_refs"] = map[string]any{"base_sha": nil, "start_sha": nil, "head_sha": nil}
		}
	}
	return out
}

func (m *fakeMR) approvals(gl *fakeGitLab) map[string]any {
	by := []any{}
	for _, n := range m.ApprovedBy {
		by = append(by, map[string]any{"user": gl.userJSON(n)})
	}
	left := 1
	if len(m.ApprovedBy) > 0 {
		left = 0
	}
	return map[string]any{"approved": len(m.ApprovedBy) > 0, "approvals_left": left, "approved_by": by}
}

func (n *fakeNote) json(gl *fakeGitLab, m *fakeMR) map[string]any {
	out := map[string]any{"id": n.ID, "body": n.Body, "author": gl.userJSON(n.Author), "system": n.System,
		"resolvable": n.Resolvable, "resolved": n.Resolved, "created_at": n.Created, "updated_at": n.Updated, "type": nil}
	if n.Path != "" {
		out["type"] = "DiffNote"
		out["position"] = map[string]any{"base_sha": m.BaseSHA, "start_sha": m.BaseSHA, "head_sha": m.SHA, "position_type": "text",
			"old_path": n.Path, "new_path": n.Path, "new_line": n.NewLine, "old_line": nil}
	}
	return out
}

func (d *fakeDiscussion) json(gl *fakeGitLab, m *fakeMR) map[string]any {
	notes := []map[string]any{}
	for _, n := range d.Notes {
		notes = append(notes, n.json(gl, m))
	}
	return map[string]any{"id": d.ID, "individual_note": d.Individual, "notes": notes}
}

func without(list []string, s string) []string {
	var out []string
	for _, x := range list {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}

func fakeMRList(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	p := gl.project(w, r)
	if p == nil {
		return
	}
	q := r.URL.Query()
	var mrs []*fakeMR
	for _, m := range fakeMRs(gl)[p.Path] {
		switch {
		case q.Get("state") != "" && q.Get("state") != m.State:
		case q.Get("author_username") != "" && q.Get("author_username") != m.Author:
		case q.Get("assignee_username") != "" && !hasString(m.Assignees, q.Get("assignee_username")):
		case q.Get("reviewer_username") != "" && !hasString(m.Reviewers, q.Get("reviewer_username")):
		case q.Get("source_branch") != "" && q.Get("source_branch") != m.Source:
		case q.Get("target_branch") != "" && q.Get("target_branch") != m.Target:
		case q.Get("search") != "" && !strings.Contains(strings.ToLower(m.Title+" "+m.Description), strings.ToLower(q.Get("search"))):
		case !fakeHasLabels(m, q.Get("labels")):
		default:
			mrs = append(mrs, m)
		}
	}
	key := func(m *fakeMR) string { return m.Created }
	if q.Get("order_by") == "updated_at" {
		key = func(m *fakeMR) string { return m.Updated }
	}
	sort.SliceStable(mrs, func(i, j int) bool { return key(mrs[i]) > key(mrs[j]) })
	out := []map[string]any{}
	for _, m := range mrs {
		out = append(out, m.json(gl, false))
	}
	fakePage(w, r, out)
}

func fakeHasLabels(m *fakeMR, labels string) bool {
	for _, l := range strings.Split(labels, ",") {
		if l != "" && !hasString(m.Labels, l) {
			return false
		}
	}
	return true
}

// fakeIDs reads a JSON list of user IDs as usernames.
func (gl *fakeGitLab) fakeIDs(v any) []string {
	out := []string{}
	list, _ := v.([]any)
	for _, x := range list {
		if f, ok := x.(float64); ok {
			if name := gl.usernameByID(int(f)); name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

func fakeMRCreate(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	p := gl.project(w, r)
	if p == nil {
		return
	}
	b := fakeBody(r)
	title, _ := b["title"].(string)
	source, _ := b["source_branch"].(string)
	target, _ := b["target_branch"].(string)
	if title == "" || source == "" || target == "" {
		fakeJSON(w, 400, map[string]any{"message": "400 Bad request - title, source_branch, target_branch are missing"})
		return
	}
	iid := 0
	for _, m := range fakeMRs(gl)[p.Path] {
		if m.State == "opened" && m.Source == source && m.Target == target {
			fakeJSON(w, 409, map[string]any{"message": []string{fmt.Sprintf("Another open merge request already exists for this source branch: !%d", m.IID)}})
			return
		}
		iid = max(iid, m.IID)
	}
	now := gl.now()
	m := &fakeMR{ID: gl.id(), IID: iid + 1, Project: p.Path, Title: title, State: "opened", Source: source, Target: target,
		Author: gl.caller(r), SHA: fmt.Sprintf("%040x", gl.id()), BaseSHA: "b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0",
		Status: "mergeable", Created: now, Updated: now}
	m.Description, _ = b["description"].(string)
	m.Assignees, m.Reviewers = gl.fakeIDs(b["assignee_ids"]), gl.fakeIDs(b["reviewer_ids"])
	if l, _ := b["labels"].(string); l != "" {
		m.Labels = strings.Split(l, ",")
	}
	m.Squash, _ = b["squash"].(bool)
	m.RemoveSource, _ = b["remove_source_branch"].(bool)
	fakeMRs(gl)[p.Path] = append(fakeMRs(gl)[p.Path], m)
	fakeJSON(w, 201, m.json(gl, true))
}

func fakeMRUpdate(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	m := gl.mr(w, r)
	if m == nil {
		return
	}
	b := fakeBody(r)
	if v, ok := b["title"].(string); ok {
		m.Title = v
	}
	if v, ok := b["description"].(string); ok {
		m.Description = v
	}
	if v, ok := b["target_branch"].(string); ok {
		m.Target = v
	}
	if _, ok := b["assignee_ids"]; ok {
		m.Assignees = gl.fakeIDs(b["assignee_ids"])
	}
	if _, ok := b["reviewer_ids"]; ok {
		m.Reviewers = gl.fakeIDs(b["reviewer_ids"])
	}
	if v, _ := b["add_labels"].(string); v != "" {
		for _, l := range strings.Split(v, ",") {
			if !hasString(m.Labels, l) {
				m.Labels = append(m.Labels, l)
			}
		}
	}
	if v, _ := b["remove_labels"].(string); v != "" {
		for _, l := range strings.Split(v, ",") {
			m.Labels = without(m.Labels, l)
		}
	}
	switch b["state_event"] {
	case "close":
		if m.State == "opened" {
			m.State = "closed"
		}
	case "reopen":
		if m.State == "closed" {
			m.State = "opened"
		}
	}
	m.Updated = gl.now()
	fakeJSON(w, 200, m.json(gl, true))
}

func fakeMRMerge(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	m := gl.mr(w, r)
	if m == nil {
		return
	}
	b := fakeBody(r)
	switch {
	case m.State != "opened" || mrIsDraftTitle(m.Title):
		fakeJSON(w, 405, map[string]any{"message": "405 Method Not Allowed"})
		return
	case m.Conflicts:
		fakeJSON(w, 406, map[string]any{"message": "Branch cannot be merged"})
		return
	case b["sha"] != nil && b["sha"] != m.SHA:
		fakeJSON(w, 409, map[string]any{"message": "SHA does not match HEAD of source branch"})
		return
	}
	now := gl.now()
	m.State, m.MergedAt, m.Updated, m.Status = "merged", now, now, "not_open"
	if v, ok := b["squash"].(bool); ok {
		m.Squash = v
	}
	fakeJSON(w, 200, m.json(gl, true))
}

func fakeMRApprove(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	m := gl.mr(w, r)
	if m == nil {
		return
	}
	me := gl.caller(r)
	if sha, ok := fakeBody(r)["sha"].(string); ok && sha != m.SHA {
		fakeJSON(w, 409, map[string]any{"message": "SHA does not match HEAD of source branch"})
		return
	}
	if hasString(m.ApprovedBy, me) {
		fakeJSON(w, 401, map[string]any{"message": "401 Unauthorized"})
		return
	}
	m.ApprovedBy = append(m.ApprovedBy, me)
	fakeJSON(w, 201, m.approvals(gl))
}

func fakeMRDiscussionCreate(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	m := gl.mr(w, r)
	if m == nil {
		return
	}
	b := fakeBody(r)
	body, _ := b["body"].(string)
	pos, _ := b["position"].(map[string]any)
	now := gl.now()
	n := &fakeNote{ID: gl.id(), Author: gl.caller(r), Body: body, Created: now, Updated: now}
	if pos != nil {
		if pos["head_sha"] != m.SHA || pos["base_sha"] != m.BaseSHA {
			fakeJSON(w, 400, map[string]any{"message": "400 Bad request - Note {:line_code=>[\"can't be blank\"]}"})
			return
		}
		n.Resolvable = true
		n.Path, _ = pos["new_path"].(string)
		if l, ok := pos["new_line"].(float64); ok {
			n.NewLine = int(l)
		}
	}
	d := &fakeDiscussion{ID: fmt.Sprintf("dc%d", n.ID), Notes: []*fakeNote{n}}
	m.Discussions = append(m.Discussions, d)
	fakeJSON(w, 201, d.json(gl, m))
}

// fakeMRByIID is merge request iid of acme/web (call under gl.locked).
func fakeMRByIID(gl *fakeGitLab, iid int) *fakeMR {
	for _, m := range fakeMRs(gl)["acme/web"] {
		if m.IID == iid {
			return m
		}
	}
	return nil
}

// ---------------------------------------------------------------- behaviour matrix

func TestMRMatrix(t *testing.T) {
	cases := []matrixCase{
		{Path: "mr list", Args: []string{"--project", "acme/web"}, EmptyArgs: []string{"--project", "acme/empty"},
			Invalid: []invalidCase{
				{[]string{}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"--project", "acme/web", "--state", "open"}, "E_USAGE", "invalid_value", "--state"},
				{[]string{"--project", "acme/web", "--pipeline-status", "red"}, "E_USAGE", "invalid_value", "--pipeline-status"},
				{[]string{"--project", "acme/web", "--limit", "101"}, "E_USAGE", "invalid_value", "--limit"},
				{[]string{"--project", "acme/web", "--search", "a", "--query", "b"}, "E_USAGE", "conflicting_flags", "--search"},
			}},
		{Path: "mr get", Args: []string{"5", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--mr"},
				{[]string{"acme/web!x"}, "E_VALIDATION", "invalid_value", "--mr"},
				{[]string{"5"}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"acme/web!5", "--project", "acme/other"}, "E_USAGE", "conflicting_flags", "--project"},
			}},
		{Path: "mr current", Args: []string{"--project", "acme/web", "--branch", "feature/retry"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/../web", "--branch", "x"}, "E_USAGE", "invalid_value", "--project"},
			}},
		{Path: "mr diff", Args: []string{"5", "--project", "acme/web"}, EmptyArgs: []string{"9", "--project", "acme/web"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--mr"},
				{[]string{"acme/web!5:a.go", "--path", "b.go"}, "E_USAGE", "conflicting_flags", "--path"},
				{[]string{"acme/web!5", "--offset", "3"}, "E_USAGE", "invalid_value", "--offset"},
			}},
		{Path: "mr create", Args: []string{"--project", "acme/web", "--title", "Add caching", "--source-branch", "feature/cache",
			"--target-branch", "main", "--description", "Caches the rates.", "--assignee", "carol", "--reviewer", "dave", "--label", "backend"},
			Role: "developer", PreviewReads: 3,
			RequestFields: map[string]string{"title": "title", "source_branch": "source_branch", "target_branch": "target_branch",
				"description": "description"},
			Invalid: []invalidCase{
				{[]string{"--project", "acme/web", "--source-branch", "x"}, "E_USAGE", "missing_parameter", "--title"},
				{[]string{"--project", "acme/web", "--title", "x"}, "E_USAGE", "missing_parameter", "--source-branch"},
				{[]string{"--title", "x", "--source-branch", "x"}, "E_USAGE", "missing_parameter", "--project"},
				{[]string{"--project", "acme/web", "--title", "x", "--source-branch", "x", "--target-branch", "x"}, "E_VALIDATION", "invalid_value", "--target-branch"},
				{[]string{"--project", "acme/web", "--title", " ", "--source-branch", "x"}, "E_USAGE", "missing_parameter", "--title"},
			}},
		{Path: "mr update", Args: []string{"5", "--project", "acme/web", "--title", "Add retries", "--description", "New text.",
			"--assignee", "dave", "--add-labels", "needs-review", "--target-branch", "develop"},
			Role: "developer", PreviewReads: 2,
			RequestFields: map[string]string{"title": "title", "description": "description", "target_branch": "target_branch"},
			Invalid: []invalidCase{
				{[]string{"5", "--project", "acme/web"}, "E_USAGE", "missing_parameter", "--title"},
				{[]string{"--project", "acme/web", "--title", "x"}, "E_USAGE", "missing_parameter", "--mr"},
				{[]string{"5", "--project", "acme/web", "--title", " "}, "E_VALIDATION", "invalid_value", "--title"},
				{[]string{"5", "--project", "acme/web", "--add-labels", "a", "--remove-labels", "a"}, "E_USAGE", "conflicting_flags", "--remove-labels"},
				{[]string{"5", "--project", "acme/web", "--description", "x", "--description-file", "f.md"}, "E_USAGE", "conflicting_flags", "--description-file"},
			}},
		{Path: "mr merge", Args: []string{"5", "--project", "acme/web", "--squash", "--should-remove-source-branch"}, Role: "developer",
			Invalid: []invalidCase{{[]string{"--project", "acme/web", "--dangerous"}, "E_USAGE", "missing_parameter", "--mr"}}},
		{Path: "mr close", Args: []string{"5", "--project", "acme/web"}, Role: "developer",
			Invalid: []invalidCase{{[]string{"x", "--project", "acme/web"}, "E_VALIDATION", "invalid_value", "--mr"}}},
		{Path: "mr reopen", Args: []string{"7", "--project", "acme/web"}, Role: "developer",
			Invalid: []invalidCase{{[]string{"7"}, "E_USAGE", "missing_parameter", "--project"}}},
		{Path: "mr approve", Args: []string{"5", "--project", "acme/web"}, Role: "developer", PreviewReads: 3,
			Invalid: []invalidCase{{[]string{"--project", "acme/web"}, "E_USAGE", "missing_parameter", "--mr"}}},
		{Path: "mr unapprove", Args: []string{"6", "--project", "acme/web"}, Role: "developer", PreviewReads: 3,
			Invalid: []invalidCase{{[]string{"acme/web!0"}, "E_VALIDATION", "invalid_value", "--mr"}}},
	}
	for _, c := range cases {
		t.Run(c.Path, func(t *testing.T) { runMatrix(t, c) })
	}
}

// ---------------------------------------------------------------- focused tests

// TestMRCommandMapLine pins the merge request line of the Skill's command map
// (SKL-002): the verbs in registration order, the nested groups after them.
func TestMRCommandMapLine(t *testing.T) {
	got := newTestApp(map[string]string{}).CommandMap()
	want := "- `mr`: list, get, current, diff, create, update, merge (dangerous), close, reopen, approve, unapprove; " +
		"`mr comment` list|add|delete (dangerous); `mr discussion` list|get|reply|create|resolve|unresolve; " +
		"`mr bulk` merge (dangerous)|approve|close|update\n"
	if !strings.Contains(got, want) {
		t.Fatalf("command map:\n%s\nwant the line:\n%s", got, want)
	}
	if strings.Index(got, "- `milestone`") > strings.Index(got, "- `mr`") {
		t.Fatalf("mr follows milestone in registerCommands order:\n%s", got)
	}
}

// TestMRFormsAreOneTarget: the iid with --project, !iid, the ref, the web URL (with
// a suffix or an anchor) and a numeric project ID read the same merge request and
// bind as one request (CLI-048, CLI-054): a dry-run with the ref confirms with the
// URL.
func TestMRFormsAreOneTarget(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"mr", "get", "5", "--project", "acme/web"},
		{"mr", "get", "!5", "-p", "acme/web"},
		{"mr", "get", "acme/web!5"},
		{"mrs", "view", "https://gitlab.example.com/acme/web/-/merge_requests/5/diffs"},
		{"mr", "show", "https://gitlab.example.com/acme/web/-/merge_requests/5#note_9001"},
		{"mr", "get", "--mr", "5", "--repo", "acme/web"},
		{"mr", "get", "5", "--project", "1"},
	} {
		r := e.run(args...)
		if r.exit != 0 || r.data()["ref"] != "acme/web!5" || r.data()["pipeline_status"] != "success" {
			t.Fatalf("%v: %s", args, r.out)
		}
	}
	tok := e.dryRun("mr", "close", "acme/web!5")
	if r := e.confirm(tok, "mr", "close", "https://gitlab.example.com/acme/web/-/merge_requests/5"); r.exit != 0 {
		t.Fatalf("the ref and the URL must bind alike: %s", r.out)
	}
}

// TestMRAgentGuesses: the fleet's guesses for this group are accepted or answered
// with an exact did_you_mean (CLI-022): plural nouns, view/show/ls/add/edit, note
// for comment, the root shortcuts of the nested groups.
func TestMRAgentGuesses(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"mrs", "ls", "-R", "acme/web"},
		{"mr", "view", "acme/web!5"},
		{"mr", "notes", "ls", "acme/web!5"},
		{"mr", "note", "list", "acme/web!5"},
		{"mr", "threads", "ls", "acme/web!5"},
		{"mr-discussion", "list", "acme/web!5"},
		{"mr-comment", "list", "acme/web!5"},
	} {
		if r := e.run(args...); r.exit != 0 {
			t.Fatalf("%v should be accepted: %s", args, r.out)
		}
	}
	for _, args := range [][]string{
		{"mr", "new", "--project", "acme/web", "--title", "x", "--source-branch", "y", "--target-branch", "main", "--dry-run"},
		{"mr", "edit", "acme/web!5", "--title", "y", "--dry-run"},
		{"mr", "note", "add", "acme/web!5", "--body", "z", "--dry-run"},
		{"mr-bulk", "close", "--project", "acme/web", "--ids", "5", "--dry-run"},
	} {
		if r := e.run(args...); r.exit != 0 || r.token() == "" {
			t.Fatalf("%v should be accepted: %s", args, r.out)
		}
	}
	r := e.run("mr", "mrege", "acme/web!5", "--dangerous")
	if r.details()["problem"] != "unknown_command" || stringsOf(r.details()["did_you_mean"])[0] != "gitlab-cli mr merge" {
		t.Fatalf("a typo: %s", r.out)
	}
}

func TestParseMRArg(t *testing.T) {
	type want struct {
		project string
		iid     int
		rest    string
	}
	for in, w := range map[string]want{
		"5": {"", 5, ""}, "!5": {"", 5, ""}, "acme/web!5": {"acme/web", 5, ""},
		"acme/web!5#note_12":                          {"acme/web", 5, "#note_12"},
		"acme/web!5/notes/12":                         {"acme/web", 5, "#note_12"},
		"acme/web!5/discussions/d5b":                  {"acme/web", 5, "#discussion_d5b"},
		"acme/web!5#discussion_d5b":                   {"acme/web", 5, "#discussion_d5b"},
		"acme/web!5:src/a:b.go":                       {"acme/web", 5, ":src/a:b.go"},
		"!5:x.go":                                     {"", 5, ":x.go"},
		"https://h/g/s/web/-/merge_requests/5#note_3": {"g/s/web", 5, "#note_3"},
		"https://h/acme/web/-/merge_requests/5/diffs": {"acme/web", 5, ""},
	} {
		p, iid, rest, err := parseMRArg(in)
		if err != nil || p != w.project || iid != w.iid || rest != w.rest {
			t.Errorf("parseMRArg(%q) = %q %d %q %v; want %+v", in, p, iid, rest, err, w)
		}
	}
	for _, bad := range []string{"", "x", "acme/web#5", "acme/web!x", "!0", "https://h/acme/web/-/issues/3"} {
		if _, _, _, err := parseMRArg(bad); err == nil {
			t.Errorf("parseMRArg(%q) should fail", bad)
		}
	}
}

// TestMRListDefaultsAndFilters: the eval's default fields (CLI-040), the filters
// echoed in scope, users given as @name or me, and pipeline_status read only when
// it is shown.
func TestMRListDefaultsAndFilters(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("mr", "list", "--project", "acme/web")
	items := r.items()
	if r.exit != 0 || len(items) != 3 || items[0]["ref"] != "acme/web!6" || items[1]["ref"] != "acme/web!5" {
		t.Fatalf("opened, most recently updated first: %s", r.out)
	}
	want := []string{"assignees", "labels", "pipeline_status", "ref", "reviewers", "state", "title", "updated_at", "web_url"}
	if got := sortedBy(items[1]); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("default fields %v, want %v", got, want)
	}
	if items[0]["pipeline_status"] != "failed" || items[1]["assignees"].([]any)[0] != "@carol" || r.data()["sort"] != "updated_at_desc" ||
		r.data()["total"] != json.Number("3") {
		t.Fatalf("items: %s", r.out)
	}
	if u := stringsOf(r.doc["meta"].(map[string]any)["untrusted"]); !hasString(u, "data.items[].title") || !hasString(u, "data.items[].labels") {
		t.Fatalf("titles and labels are untrusted: %v", u)
	}
	gets := 0
	for _, req := range e.gl.Requests() {
		if strings.Count(req.Path, "/merge_requests/") == 1 {
			gets++
		}
	}
	if gets != 3 {
		t.Fatalf("one read per merge request for its pipeline, got %d", gets)
	}

	// Without pipeline_status in --fields, no per-MR read.
	before := len(e.gl.Requests())
	r = e.run("mr", "list", "--project", "acme/web", "--fields", "ref,title")
	if r.exit != 0 || len(e.gl.Requests()) != before+1 || r.items()[0]["pipeline_status"] != nil {
		t.Fatalf("--fields ref,title reads no pipelines: %s (%d requests)", r.out, len(e.gl.Requests())-before)
	}

	r = e.run("mr", "list", "--project", "acme/web", "--state", "all", "--reviewer", "@carol", "--label", "bug", "--query", "FLAKY",
		"--target-branch", "main")
	scope, _ := r.data()["scope"].(map[string]any)
	if r.exit != 0 || len(r.items()) != 1 || r.items()[0]["ref"] != "acme/web!6" || scope["reviewer"] != "@carol" ||
		scope["query"] != "FLAKY" || scope["state"] != "all" || scope["target_branch"] != "main" {
		t.Fatalf("filters: %s", r.out)
	}
	r = e.run("mr", "list", "--project", "acme/web", "--author", "me", "--state", "all")
	if r.exit != 0 || len(r.items()) != 2 || r.data()["scope"].(map[string]any)["author"] != "@carol" {
		t.Fatalf("--author me: %s", r.out)
	}
	// 1.x's --search keeps working, with a deprecated notice (CLI-081).
	r = e.run("mr", "list", "--project", "acme/web", "--search", "retry")
	notices, _ := r.doc["meta"].(map[string]any)["notices"].([]any)
	if r.exit != 0 || len(r.items()) != 1 || len(notices) == 0 || notices[0].(map[string]any)["type"] != "deprecated" {
		t.Fatalf("--search: %s", r.out)
	}
}

// TestMRListByPipelineStatus: "which MRs have failed pipelines" in one call
// (CLI-044); scope says what was checked, and a check that stopped short is
// truncated (CLI-041).
func TestMRListByPipelineStatus(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("mr", "list", "--project", "acme/web", "--state", "all", "--pipeline-status", "failed")
	scope, _ := r.data()["scope"].(map[string]any)
	if r.exit != 0 || len(r.items()) != 2 || r.items()[0]["ref"] != "acme/web!6" || r.items()[1]["ref"] != "acme/web!7" ||
		r.data()["has_more"] != false || r.data()["total"] != json.Number("2") || scope["pipeline_status"] != "failed" ||
		scope["checked"] != json.Number("5") || !strings.Contains(scope["covers"].(string), "100 most recently updated") {
		t.Fatalf("failed pipelines: %s", r.out)
	}
	r = e.run("mr", "list", "--project", "acme/web", "--state", "all", "--pipeline-status", "failed", "--limit", "1")
	if r.data()["has_more"] != true || r.data()["next_offset"] != json.Number("1") || len(r.items()) != 1 {
		t.Fatalf("a page of the matches: %s", r.out)
	}
	r = e.run("mr", "list", "--project", "acme/web", "--state", "all", "--pipeline-status", "failed", "--limit", "1", "--offset", "1")
	if r.data()["has_more"] != false || len(r.items()) != 1 || r.items()[0]["ref"] != "acme/web!7" {
		t.Fatalf("the next page: %s", r.out)
	}
	// More matching merge requests than one check covers: the list says so.
	e.gl.locked(func() {
		for i := 0; i < mrPipelineScan; i++ {
			fakeMRs(e.gl)["acme/web"] = append(fakeMRs(e.gl)["acme/web"], &fakeMR{ID: 6000 + i, IID: 100 + i, Project: "acme/web",
				Title: "Bulk " + strconv.Itoa(i), State: "opened", Source: "b" + strconv.Itoa(i), Target: "main", Author: "dave",
				Pipeline: "success", Created: "2026-10-20T09:00:00.000Z", Updated: "2026-10-20T09:00:00.000Z"})
		}
	})
	r = e.run("mr", "list", "--project", "acme/web", "--pipeline-status", "failed")
	if r.exit != 0 || len(r.items()) != 0 || r.data()["truncated"] != true || r.data()["has_more"] != true ||
		r.data()["continuation_reason"] == nil || r.data()["total"] != nil {
		t.Fatalf("a capped check is truncated: %s", r.out)
	}
}

func TestMRGetShowsDescriptionOnRequest(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("mr", "get", "acme/web!5")
	if r.exit != 0 || r.data()["description"] != nil || r.data()["source_branch"] != "feature/retry" || r.data()["author"] != "@dave" ||
		r.data()["detailed_merge_status"] != "mergeable" || r.data()["updated_at"] != "2026-10-10T09:00:00Z" {
		t.Fatalf("get: %s", r.out)
	}
	r = e.run("mr", "get", "acme/web!5", "--fields", "+description,+diff_refs")
	if r.exit != 0 || r.data()["description"] == nil || r.data()["diff_refs"] == nil || r.data()["title"] == nil {
		t.Fatalf("--fields +description,+diff_refs: %s", r.out)
	}
	r = e.run("mr", "get", "acme/web!5", "--fields", "ref,description,diff_refs")
	refs, _ := r.data()["diff_refs"].(map[string]any)
	if r.exit != 0 || !strings.HasPrefix(r.data()["description"].(string), "Retries") || refs["head_sha"] == nil {
		t.Fatalf("description and diff refs: %s", r.out)
	}
	if !hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "data.description") {
		t.Fatalf("the description is untrusted: %s", r.out)
	}
	r = e.run("mr", "get", "55", "--project", "acme/web")
	if r.code() != "E_NOT_FOUND" || !strings.Contains(r.details()["next_action"].(string), "mr list --project acme/web") {
		t.Fatalf("a missing merge request: %s", r.out)
	}
}

// mrGitCheckout makes a git repository whose origin is acme/web on a GitLab host,
// on branch, with one commit, and makes it the working directory.
func mrGitCheckout(t *testing.T, branch, subject string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		c := exec.Command("git", args...)
		c.Dir = dir
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-q", "-b", branch)
	run("remote", "add", "origin", "https://gitlab.example.com/acme/web.git")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	run("commit", "-q", "-m", subject)
	t.Chdir(dir)
}

// TestMRCurrentUsesTheCheckout: the project and branch come from the git checkout
// in the working directory unless given.
func TestMRCurrentUsesTheCheckout(t *testing.T) {
	e := newTestEnv(t)
	mrGitCheckout(t, "feature/retry", "Add retry")
	r := e.run("mr", "current")
	if r.exit != 0 || r.data()["ref"] != "acme/web!5" || r.data()["pipeline_status"] != "success" {
		t.Fatalf("current: %s", r.out)
	}
	r = e.run("mr", "current", "--branch", "deps/bump")
	if r.code() != "E_NOT_FOUND" || !strings.Contains(r.details()["next_action"].(string), "--source-branch deps/bump") {
		t.Fatalf("no open merge request: %s", r.out)
	}
	e.gl.locked(func() {
		fakeMRs(e.gl)["acme/web"] = append(fakeMRs(e.gl)["acme/web"], &fakeMR{ID: 5010, IID: 10, Project: "acme/web", Title: "Same branch",
			State: "opened", Source: "feature/retry", Target: "release", Author: "dave", Updated: "2026-10-12T09:00:00.000Z"})
	})
	r = e.run("mr", "current")
	if r.code() != "E_AMBIGUOUS" || len(r.details()["candidates"].([]any)) != 2 {
		t.Fatalf("two open merge requests from the branch: %s", r.out)
	}
	t.Chdir(t.TempDir())
	if r = e.run("mr", "current"); r.code() != "E_USAGE" || r.details()["parameter"] != "--project" {
		t.Fatalf("outside a checkout: %s", r.out)
	}
	if r = e.run("mr", "current", "--project", "acme/web"); r.code() != "E_USAGE" || r.details()["parameter"] != "--branch" {
		t.Fatalf("no branch: %s", r.out)
	}
}

// TestMRDiffIsAnInventoryFirst: the changed files with line counts by default, the
// diff text on request, one file by --path or its ref, the unified diff raw
// (CLI-043).
func TestMRDiffIsAnInventoryFirst(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("mr", "diff", "acme/web!5")
	items := r.items()
	if r.exit != 0 || len(items) != 3 || items[0]["ref"] != "acme/web!5:src/client.go" || items[0]["additions"] != json.Number("4") ||
		items[0]["deletions"] != json.Number("1") || items[1]["status"] != "added" || items[2]["status"] != "deleted" ||
		items[0]["diff"] != nil {
		t.Fatalf("inventory: %s", r.out)
	}
	r = e.run("mr", "diff", "acme/web!5:src/retry.go", "--fields", "+diff")
	if r.exit != 0 || len(r.items()) != 1 || !strings.Contains(r.items()[0]["diff"].(string), "+package client") ||
		r.data()["scope"].(map[string]any)["path"] != "src/retry.go" {
		t.Fatalf("one file with its diff: %s", r.out)
	}
	if !hasString(stringsOf(r.doc["meta"].(map[string]any)["untrusted"]), "data.items[].diff") {
		t.Fatalf("diff text is untrusted: %s", r.out)
	}
	if r = e.run("mr", "diff", "5", "--project", "acme/web", "--path", "nope.go"); r.exit != 0 || len(r.items()) != 0 {
		t.Fatalf("an unchanged path is an empty list: %s", r.out)
	}
	r = e.run("mr", "diff", "acme/web!5", "--format", "raw")
	if r.exit != 0 || !strings.HasPrefix(r.out, "diff --git a/src/client.go") || !strings.Contains(r.out, "docs/old.md") {
		t.Fatalf("raw: %s", r.out)
	}
	r = e.run("mr", "diff", "acme/web!5:docs/old.md", "--format", "raw")
	if r.exit != 0 || !strings.HasPrefix(r.out, "diff --git a/docs/old.md") || strings.Contains(r.out, "src/client.go") {
		t.Fatalf("raw, one file: %s", r.out)
	}
}

// TestMRCreatePreviewsAndRecordsTheRef: users and labels in the preview, the new
// ref observed, the draft prefix, the default target branch, and a second open
// merge request between the same branches blocked (CLI-029, CLI-059).
func TestMRCreatePreviewsAndRecordsTheRef(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"mr", "create", "--project", "acme/web", "--title", "Rework caching", "--source-branch", "feature/cache",
		"--target-branch", "main", "--assignee", "@carol,dave", "--label", "backend,perf", "--draft", "--idempotency-key", "k1"}
	dry := e.run(append(args, "--dry-run")...)
	changes, _ := json.Marshal(dry.preview()["changes"])
	if dry.exit != 0 || !strings.Contains(string(changes), `"after":["@carol","@dave"]`) || !strings.Contains(string(changes), `"after":"Draft: Rework caching"`) ||
		!strings.Contains(string(changes), `"add":["backend","perf"]`) {
		t.Fatalf("preview: %s", dry.out)
	}
	r := e.confirm(dry.token(), args...)
	res, _ := r.data()["result"].(map[string]any)
	if r.exit != 0 || res["ref"] != "acme/web!10" || res["draft"] != true || r.outcome()["verification"] != "passed" ||
		r.receipt()["observed"].(map[string]any)["ref"] != "acme/web!10" {
		t.Fatalf("create: %s", r.out)
	}
	if req, _ := e.gl.lastWrite(); req.Body["title"] != "Draft: Rework caching" || fmt.Sprint(req.Body["assignee_ids"]) != "[7 8]" {
		t.Fatalf("request: %+v", req.Body)
	}
	if again := e.run(append(args, "--dry-run")...); again.receipt()["replayed"] != true || e.gl.Writes() != 1 {
		t.Fatalf("the same key replays (P4): %s", again.out)
	}
	// Without the key, the open merge request blocks the plan.
	dup := e.run("mr", "create", "--project", "acme/web", "--title", "x", "--source-branch", "feature/cache", "--target-branch", "main", "--dry-run")
	if b, _ := dup.preview()["blocked_by"].([]any); dup.exit != 0 || dup.token() != "" || len(b) != 1 ||
		!strings.Contains(b[0].(map[string]any)["message"].(string), "acme/web!10") {
		t.Fatalf("a duplicate: %s", dup.out)
	}
	// No --target-branch: the project's default branch, read in the preview.
	e.gl.fail(fakeFault{Method: http.MethodGet, Path: "/projects/acme%2Fweb", Status: 200, Times: 1,
		Body: `{"id":1,"path_with_namespace":"acme/web","default_branch":"develop"}`})
	r = e.run("mr", "create", "--project", "acme/web", "--title", "x", "--source-branch", "feature/y", "--dry-run")
	if r.exit != 0 || r.token() == "" || !strings.Contains(fmt.Sprint(r.preview()["changes"]), "after:develop") {
		t.Fatalf("the default branch: %s", r.out)
	}
	// A project without a default branch (an empty repository): blocked, not guessed.
	e.gl.fail(fakeFault{Method: http.MethodGet, Path: "/projects/acme%2Fweb", Status: 200, Times: 1,
		Body: `{"id":1,"path_with_namespace":"acme/web","default_branch":null}`})
	r = e.run("mr", "create", "--project", "acme/web", "--title", "x", "--source-branch", "feature/y", "--dry-run")
	if b, _ := r.preview()["blocked_by"].([]any); r.exit != 0 || len(b) != 1 || b[0].(map[string]any)["kind"] != "no_target_branch" {
		t.Fatalf("no default branch: %s", r.out)
	}
	if r = e.run("mr", "create", "--project", "acme/web", "--title", "x", "--source-branch", "y", "--target-branch", "main",
		"--assignee", "zed", "--dry-run"); r.code() != "E_NOT_FOUND" || r.details()["parameter"] != "--assignee" {
		t.Fatalf("an unknown user: %s", r.out)
	}
}

// TestMRCreateAutoUsesTheCheckout: --auto takes the project, branch and title from
// git and binds them as if given (CLI-054).
func TestMRCreateAutoUsesTheCheckout(t *testing.T) {
	e := newTestEnv(t)
	mrGitCheckout(t, "feature/auto", "Add the auto mode")
	dry := e.run("mr", "create", "--auto", "--target-branch", "main", "--dry-run")
	inputs, _ := dry.preview()["inputs"].(map[string]any)
	if dry.exit != 0 || inputs["project"] != "acme/web" || inputs["source_branch"] != "feature/auto" || inputs["title"] != "Add the auto mode" {
		t.Fatalf("--auto: %s", dry.out)
	}
	r := e.confirm(dry.token(), "mr", "create", "--project", "acme/web", "--source-branch", "feature/auto", "--title", "Add the auto mode",
		"--auto", "--target-branch", "main")
	if r.exit != 0 {
		t.Fatalf("the derived values bind as given: %s", r.out)
	}
	t.Chdir(t.TempDir())
	if r = e.run("mr", "create", "--auto", "--dry-run"); r.code() != "E_USAGE" || r.details()["parameter"] != "--project" {
		t.Fatalf("--auto outside a checkout: %s", r.out)
	}
}

// TestMRUpdateChangesOnlyWhatIsGiven: assignees replaced or cleared, labels added
// and removed, the rest untouched (CLI-029); a concurrent edit is state_drift.
func TestMRUpdateChangesOnlyWhatIsGiven(t *testing.T) {
	e := newTestEnv(t)
	args := []string{"mr", "update", "acme/web!5", "--assignee", "", "--reviewer", "me,dave", "--add-labels", "needs-review", "--remove-labels", "backend"}
	r := e.confirm(e.dryRun(args...), args...)
	res, _ := r.data()["result"].(map[string]any)
	if r.exit != 0 || r.outcome()["verification"] != "passed" || len(res["assignees"].([]any)) != 0 ||
		fmt.Sprint(res["reviewers"]) != "[@carol @dave]" || fmt.Sprint(res["labels"]) != "[needs-review]" || res["title"] != "Add retry to the payment client" {
		t.Fatalf("update: %s", r.out)
	}
	req, _ := e.gl.lastWrite()
	if _, sent := req.Body["title"]; sent || fmt.Sprint(req.Body["assignee_ids"]) != "[]" {
		t.Fatalf("only the given fields are sent: %+v", req.Body)
	}
	if _, has := res["description"]; has {
		t.Fatalf("the result leaves the description out: %v", res)
	}
	args = []string{"mr", "update", "5", "--project", "acme/web", "--description", "Mine."}
	tok := e.dryRun(args...)
	e.gl.locked(func() { fakeMRByIID(e.gl, 5).Updated = "2026-10-13T00:00:00.000Z" })
	if r = e.confirm(tok, args...); r.code() != "E_CONFLICT" || r.details()["conflict"] != "state_drift" || e.gl.Writes() != 1 {
		t.Fatalf("drift: %s", r.out)
	}
	if r = e.run("mr", "update", "9", "--project", "acme/web", "--target-branch", "x", "--dry-run"); r.token() != "" ||
		r.preview()["blocked_by"].([]any)[0].(map[string]any)["kind"] != "merged" {
		t.Fatalf("a merged merge request keeps its target: %s", r.out)
	}
}

// TestMRQuickActionsAreBlocked: a line GitLab would run as a quick action (/merge)
// blocks a description or comment; inside a code block it is text.
func TestMRQuickActionsAreBlocked(t *testing.T) {
	for text, want := range map[string]bool{"/merge": true, "LGTM\n/approve\nthanks": true, "/assign @carol": true,
		"see /usr/bin/env": false, "/usr/bin/env": false, "```\n/merge\n```": false, " /close": false, "a/b": false} {
		if got := hasQuickAction(text); got != want {
			t.Errorf("hasQuickAction(%q) = %v", text, got)
		}
	}
	e := newTestEnv(t)
	r := e.run("mr", "update", "acme/web!5", "--description", "Done.\n/merge", "--dry-run")
	if r.exit != 0 || r.token() != "" || r.preview()["blocked_by"].([]any)[0].(map[string]any)["kind"] != "quick_action" {
		t.Fatalf("a quick action in a description: %s", r.out)
	}
}

// TestMRMergeIsGuarded: dangerous; blocked for drafts, closed and already merged
// merge requests and a stale --sha; a push between the dry-run and the confirm is
// state_drift; the previewed head SHA is what GitLab is asked to merge.
func TestMRMergeIsGuarded(t *testing.T) {
	e := newTestEnv(t)
	if r := e.run("mr", "merge", "acme/web!5", "--dry-run"); r.details()["problem"] != "dangerous_flag_required" {
		t.Fatalf("merge needs --dangerous: %s", r.out)
	}
	for ref, kind := range map[string]string{"acme/web!8": "draft", "acme/web!7": "closed", "acme/web!9": "merged"} {
		r := e.run("mr", "merge", ref, "--dangerous", "--dry-run")
		if b, _ := r.preview()["blocked_by"].([]any); r.exit != 0 || r.token() != "" || len(b) == 0 || b[0].(map[string]any)["kind"] != kind {
			t.Fatalf("%s: %s", ref, r.out)
		}
	}
	if r := e.run("mr", "merge", "acme/web!5", "--sha", "0000", "--dangerous", "--dry-run"); r.token() != "" {
		t.Fatalf("a stale --sha blocks: %s", r.out)
	}
	e.gl.locked(func() { fakeMRByIID(e.gl, 6).Status = "discussions_not_resolved" })
	if r := e.run("mr", "merge", "acme/web!6", "--dangerous", "--dry-run"); r.token() != "" ||
		r.preview()["blocked_by"].([]any)[0].(map[string]any)["kind"] != "discussions_not_resolved" {
		t.Fatalf("GitLab's merge status blocks: %s", r.out)
	}
	args := []string{"mr", "merge", "acme/web!5", "--dangerous"}
	tok := e.dryRun(args...)
	e.gl.locked(func() { fakeMRByIID(e.gl, 5).SHA = "5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b" })
	if r := e.confirm(tok, args...); r.code() != "E_CONFLICT" || r.details()["conflict"] != "state_drift" || e.gl.Writes() != 0 {
		t.Fatalf("a push after the dry-run: %s", r.out)
	}
	r := e.confirm(e.dryRun(args...), args...)
	if r.exit != 0 || r.data()["result"].(map[string]any)["state"] != "merged" || r.receipt()["observed"].(map[string]any)["state"] != "merged" {
		t.Fatalf("merge: %s", r.out)
	}
	if req, _ := e.gl.lastWrite(); req.Body["sha"] != "5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b5b" {
		t.Fatalf("the previewed head is sent: %+v", req.Body)
	}
}

func TestMRStateChangesAreBlockedWhenMoot(t *testing.T) {
	e := newTestEnv(t)
	for _, c := range []struct{ verb, ref, kind string }{
		{"close", "acme/web!7", "already_closed"}, {"close", "acme/web!9", "merged"},
		{"reopen", "acme/web!5", "already_open"}, {"reopen", "acme/web!9", "merged"},
	} {
		r := e.run("mr", c.verb, c.ref, "--dry-run")
		if r.exit != 0 || r.token() != "" || r.preview()["blocked_by"].([]any)[0].(map[string]any)["kind"] != c.kind {
			t.Fatalf("%s %s: %s", c.verb, c.ref, r.out)
		}
	}
	args := []string{"mr", "reopen", "acme/web!7"}
	if r := e.confirm(e.dryRun(args...), args...); r.exit != 0 || r.data()["result"].(map[string]any)["state"] != "opened" {
		t.Fatalf("reopen: %s", r.out)
	}
}

// TestMRApprovalsActAsTheTokenUser: approve adds this account, unapprove removes
// it; each is blocked when moot, and approve is bound to the head commit.
func TestMRApprovalsActAsTheTokenUser(t *testing.T) {
	e := newTestEnv(t)
	if r := e.run("mr", "approve", "acme/web!6", "--dry-run"); r.token() != "" ||
		r.preview()["blocked_by"].([]any)[0].(map[string]any)["kind"] != "already_approved" {
		t.Fatalf("approved already: %s", r.out)
	}
	if r := e.run("mr", "unapprove", "acme/web!5", "--dry-run"); r.token() != "" ||
		r.preview()["blocked_by"].([]any)[0].(map[string]any)["kind"] != "not_approved" {
		t.Fatalf("not approved: %s", r.out)
	}
	args := []string{"mr", "approve", "acme/web!5"}
	r := e.confirm(e.dryRun(args...), args...)
	res, _ := r.data()["result"].(map[string]any)
	if r.exit != 0 || r.outcome()["verification"] != "passed" || res["approved"] != true || fmt.Sprint(res["approved_by"]) != "[@carol]" {
		t.Fatalf("approve: %s", r.out)
	}
	if req, _ := e.gl.lastWrite(); req.Body["sha"] != "5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a5a" {
		t.Fatalf("the approval names the head commit: %+v", req.Body)
	}
	args = []string{"mr", "unapprove", "acme/web!6"}
	if r = e.confirm(e.dryRun(args...), args...); r.exit != 0 || r.data()["result"].(map[string]any)["approved"] != false {
		t.Fatalf("unapprove: %s", r.out)
	}
	// Another token is another account: dave has approved nothing.
	e.vars["GITLAB_CLI_TOKEN"] = daveToken
	if r = e.run("mr", "unapprove", "acme/web!5", "--dry-run"); r.token() != "" {
		t.Fatalf("dave: %s", r.out)
	}
}
