package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mrCall is one request the MR client made, as the test server saw it.
type mrCall struct {
	method, path, query string
	body                map[string]any
}

// mrServer answers every request with status and body and records it.
func mrServer(t *testing.T, status int, body string, header map[string]string) (*Client, *[]mrCall) {
	t.Helper()
	var calls []mrCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := mrCall{method: r.Method, path: r.URL.EscapedPath(), query: r.URL.RawQuery}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &c.body)
		}
		calls = append(calls, c)
		for k, v := range header {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return newTestClient(srv.URL), &calls
}

func TestMR_ListPageSendsFiltersAndPages(t *testing.T) {
	c, calls := mrServer(t, 200, `[{"id":51,"iid":5,"title":"Add retry","state":"opened","assignees":[{"username":"carol"}],
		"reviewers":[{"username":"erin"}],"labels":["backend"],"detailed_merge_status":"mergeable"}]`,
		map[string]string{"X-Next-Page": "3", "X-Total": "41"})
	mrs, pag, err := c.MergeRequests.ListPage(testCtx, "acme/web", MergeRequestListOpts{State: "opened", AuthorUsername: "dave",
		AssigneeUsername: "carol", ReviewerUsername: "erin", Labels: []string{"backend", "ui"}, Search: "retry",
		SourceBranch: "feature/retry", TargetBranch: "main", OrderBy: "updated_at", Sort: "desc", Page: 2, PerPage: 20})
	if err != nil || len(mrs) != 1 || mrs[0].IID != 5 || mrs[0].Assignees[0].Username != "carol" || mrs[0].DetailedMergeStatus != "mergeable" {
		t.Fatalf("ListPage = %+v, %v", mrs, err)
	}
	if pag.NextPage != 3 || !pag.HasTotal || pag.Total != 41 {
		t.Errorf("pagination = %+v", pag)
	}
	got := (*calls)[0]
	want := "assignee_username=carol&author_username=dave&labels=backend%2Cui&order_by=updated_at&page=2&per_page=20" +
		"&reviewer_username=erin&search=retry&sort=desc&source_branch=feature%2Fretry&state=opened&target_branch=main"
	if got.path != "/api/v4/projects/acme%2Fweb/merge_requests" || got.query != want {
		t.Errorf("request = %s?%s", got.path, got.query)
	}
	// "all" drops the state filter.
	if _, _, err := c.MergeRequests.ListPage(testCtx, "acme/web", MergeRequestListOpts{State: "all"}); err != nil {
		t.Fatal(err)
	}
	if q := (*calls)[1].query; q != "page=1&per_page=20" {
		t.Errorf("state all: %s", q)
	}
}

func TestMR_GetParsesPipelineAndDiffRefs(t *testing.T) {
	c, calls := mrServer(t, 200, `{"iid":5,"head_pipeline":{"id":9,"status":"failed"},
		"diff_refs":{"base_sha":"b","start_sha":"s","head_sha":"h"},"author":{"username":"dave"}}`, nil)
	mr, err := c.MergeRequests.Get(testCtx, "acme/web", 5)
	if err != nil || mr.HeadPipeline.Status != "failed" || mr.DiffRefs.HeadSHA != "h" || mr.Author.Username != "dave" {
		t.Fatalf("Get = %+v, %v", mr, err)
	}
	if (*calls)[0].path != "/api/v4/projects/acme%2Fweb/merge_requests/5" {
		t.Errorf("path = %s", (*calls)[0].path)
	}
}

func TestMR_WritesSendTheirBodies(t *testing.T) {
	title, empty := "New", []int{}
	squash := true
	cases := []struct {
		name, method, path string
		call               func(c *Client) error
		body               map[string]any
	}{
		{"create", "POST", "/api/v4/projects/acme%2Fweb/merge_requests", func(c *Client) error {
			_, err := c.MergeRequests.Create(testCtx, "acme/web", &MergeRequestCreateRequest{Title: "T", SourceBranch: "f", TargetBranch: "main",
				AssigneeIDs: []int{7}, Labels: "a,b", Squash: true})
			return err
		}, map[string]any{"title": "T", "source_branch": "f", "target_branch": "main", "assignee_ids": []any{float64(7)},
			"labels": "a,b", "squash": true}},
		{"update", "PUT", "/api/v4/projects/acme%2Fweb/merge_requests/5", func(c *Client) error {
			_, err := c.MergeRequests.Update(testCtx, "acme/web", 5, MergeRequestUpdateOpts{Title: &title, AssigneeIDs: &empty, AddLabels: "x"})
			return err
		}, map[string]any{"title": "New", "assignee_ids": []any{}, "add_labels": "x"}},
		{"merge", "PUT", "/api/v4/projects/acme%2Fweb/merge_requests/5/merge", func(c *Client) error {
			_, err := c.MergeRequests.Merge(testCtx, "acme/web", 5, MergeRequestMergeOpts{SHA: "abc", Squash: &squash})
			return err
		}, map[string]any{"sha": "abc", "squash": true}},
		{"approve", "POST", "/api/v4/projects/acme%2Fweb/merge_requests/5/approve", func(c *Client) error {
			_, err := c.MergeRequests.Approve(testCtx, "acme/web", 5, "abc")
			return err
		}, map[string]any{"sha": "abc"}},
		{"unapprove", "POST", "/api/v4/projects/acme%2Fweb/merge_requests/5/unapprove", func(c *Client) error {
			return c.MergeRequests.Unapprove(testCtx, "acme/web", 5)
		}, nil},
		{"add note", "POST", "/api/v4/projects/acme%2Fweb/merge_requests/5/notes", func(c *Client) error {
			_, err := c.MergeRequests.AddNote(testCtx, "acme/web", 5, "hi")
			return err
		}, map[string]any{"body": "hi"}},
		{"delete note", "DELETE", "/api/v4/projects/acme%2Fweb/merge_requests/5/notes/9", func(c *Client) error {
			return c.MergeRequests.DeleteNote(testCtx, "acme/web", 5, 9)
		}, nil},
		{"create discussion", "POST", "/api/v4/projects/acme%2Fweb/merge_requests/5/discussions", func(c *Client) error {
			line := 12
			_, err := c.MergeRequests.CreateDiscussion(testCtx, "acme/web", 5, "nit", &DiffPosition{BaseSHA: "b", StartSHA: "s",
				HeadSHA: "h", PositionType: "text", NewPath: "a.go", OldPath: "a.go", NewLine: &line})
			return err
		}, map[string]any{"body": "nit", "position": map[string]any{"base_sha": "b", "start_sha": "s", "head_sha": "h",
			"position_type": "text", "new_path": "a.go", "old_path": "a.go", "new_line": float64(12)}}},
		{"reply", "POST", "/api/v4/projects/acme%2Fweb/merge_requests/5/discussions/d1/notes", func(c *Client) error {
			_, err := c.MergeRequests.ReplyDiscussion(testCtx, "acme/web", 5, "d1", "done")
			return err
		}, map[string]any{"body": "done"}},
		{"resolve", "PUT", "/api/v4/projects/acme%2Fweb/merge_requests/5/discussions/d1", func(c *Client) error {
			_, err := c.MergeRequests.ResolveDiscussion(testCtx, "acme/web", 5, "d1", true)
			return err
		}, map[string]any{"resolved": true}},
	}
	for _, tc := range cases {
		c, calls := mrServer(t, 200, `{"iid":5}`, nil)
		if err := tc.call(c); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got := (*calls)[0]
		if got.method != tc.method || got.path != tc.path {
			t.Errorf("%s: %s %s", tc.name, got.method, got.path)
		}
		if tc.body != nil && !jsonEqual(got.body, tc.body) {
			t.Errorf("%s: body %v, want %v", tc.name, got.body, tc.body)
		}
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func TestMR_ReadsOfApprovalsDiffsNotesAndDiscussions(t *testing.T) {
	c, calls := mrServer(t, 200, `{"approved":true,"approvals_left":0,"approved_by":[{"user":{"username":"carol"}}]}`, nil)
	ap, err := c.MergeRequests.Approvals(testCtx, "acme/web", 5)
	if err != nil || !ap.Approved || len(ap.Usernames()) != 1 || ap.Usernames()[0] != "carol" {
		t.Fatalf("Approvals = %+v, %v", ap, err)
	}
	if (*calls)[0].path != "/api/v4/projects/acme%2Fweb/merge_requests/5/approvals" {
		t.Errorf("path = %s", (*calls)[0].path)
	}

	c, calls = mrServer(t, 200, `[{"old_path":"a.go","new_path":"a.go","diff":"@@ -1 +1 @@\n-a\n+b\n","renamed_file":false}]`,
		map[string]string{"X-Next-Page": ""})
	diffs, _, err := c.MergeRequests.DiffsPage(testCtx, "acme/web", 5, 1, 10)
	if err != nil || len(diffs) != 1 || diffs[0].NewPath != "a.go" {
		t.Fatalf("DiffsPage = %+v, %v", diffs, err)
	}
	if got := (*calls)[0]; got.path != "/api/v4/projects/acme%2Fweb/merge_requests/5/diffs" || got.query != "page=1&per_page=10" {
		t.Errorf("request = %s?%s", got.path, got.query)
	}

	c, calls = mrServer(t, 200, "diff --git a/a.go b/a.go\n", nil)
	if raw, err := c.MergeRequests.RawDiff(testCtx, "acme/web", 5); err != nil || raw != "diff --git a/a.go b/a.go\n" {
		t.Fatalf("RawDiff = %q, %v", raw, err)
	}
	if (*calls)[0].path != "/api/v4/projects/acme%2Fweb/merge_requests/5/raw_diffs" {
		t.Errorf("path = %s", (*calls)[0].path)
	}

	c, calls = mrServer(t, 200, `[{"id":9,"body":"hi","system":false,"author":{"username":"carol"}}]`, nil)
	notes, _, err := c.MergeRequests.NotesPage(testCtx, "acme/web", 5, 2, 20)
	if err != nil || len(notes) != 1 || notes[0].Author.Username != "carol" {
		t.Fatalf("NotesPage = %+v, %v", notes, err)
	}
	if got := (*calls)[0]; got.query != "order_by=created_at&page=2&per_page=20&sort=asc" {
		t.Errorf("query = %s", got.query)
	}

	c, _ = mrServer(t, 200, `{"id":9,"body":"hi","position":{"new_path":"a.go","new_line":3}}`, nil)
	if n, err := c.MergeRequests.GetNote(testCtx, "acme/web", 5, 9); err != nil || n.Position.NewPath != "a.go" || *n.Position.NewLine != 3 {
		t.Fatalf("GetNote = %+v, %v", n, err)
	}

	c, calls = mrServer(t, 200, `[{"id":"d1","individual_note":false,"notes":[{"id":1,"resolvable":true}]}]`, nil)
	ds, _, err := c.MergeRequests.DiscussionsPage(testCtx, "acme/web", 5, 1, 20)
	if err != nil || len(ds) != 1 || ds[0].ID != "d1" || !ds[0].Notes[0].Resolvable {
		t.Fatalf("DiscussionsPage = %+v, %v", ds, err)
	}
	if got := (*calls)[0]; got.path != "/api/v4/projects/acme%2Fweb/merge_requests/5/discussions" || got.query != "page=1&per_page=20" {
		t.Errorf("request = %s?%s", got.path, got.query)
	}
	c, calls = mrServer(t, 200, `{"id":"d1","notes":[]}`, nil)
	if d, err := c.MergeRequests.GetDiscussion(testCtx, "acme/web", 5, "d1"); err != nil || d.ID != "d1" {
		t.Fatalf("GetDiscussion = %+v, %v", d, err)
	}
	if (*calls)[0].path != "/api/v4/projects/acme%2Fweb/merge_requests/5/discussions/d1" {
		t.Errorf("path = %s", (*calls)[0].path)
	}
}

func TestMR_ParseErrors(t *testing.T) {
	c, _ := mrServer(t, 200, `{not json`, nil)
	checks := map[string]error{}
	_, _, checks["ListPage"] = c.MergeRequests.ListPage(testCtx, "p/q", MergeRequestListOpts{})
	_, checks["Get"] = c.MergeRequests.Get(testCtx, "p/q", 1)
	_, checks["Approvals"] = c.MergeRequests.Approvals(testCtx, "p/q", 1)
	_, _, checks["DiffsPage"] = c.MergeRequests.DiffsPage(testCtx, "p/q", 1, 1, 20)
	_, _, checks["NotesPage"] = c.MergeRequests.NotesPage(testCtx, "p/q", 1, 1, 20)
	_, checks["GetNote"] = c.MergeRequests.GetNote(testCtx, "p/q", 1, 2)
	_, _, checks["DiscussionsPage"] = c.MergeRequests.DiscussionsPage(testCtx, "p/q", 1, 1, 20)
	_, checks["GetDiscussion"] = c.MergeRequests.GetDiscussion(testCtx, "p/q", 1, "d")
	for name, err := range checks {
		if err == nil {
			t.Errorf("%s: a malformed answer must be an error", name)
		}
	}
}
