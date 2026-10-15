package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestIssue_ListPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/api/v4/projects/acme%2Fweb/issues" {
			t.Errorf("path = %q", r.URL.EscapedPath())
		}
		q := r.URL.Query()
		want := map[string]string{"state": "opened", "labels": "bug,needs triage", "milestone": "v1.1",
			"author_username": "carol", "assignee_username": "dave", "search": "timeout", "in": "title",
			"order_by": "created_at", "sort": "desc", "page": "2", "per_page": "5"}
		for k, v := range want {
			if q.Get(k) != v {
				t.Errorf("%s = %q, want %q (query %v)", k, q.Get(k), v, q)
			}
		}
		if !reflect.DeepEqual(q["iids[]"], []string{"2", "6"}) {
			t.Errorf("iids[] = %v", q["iids[]"])
		}
		w.Header().Set("X-Next-Page", "3")
		w.Header().Set("X-Total", "11")
		_, _ = w.Write([]byte(`[{"id":502,"iid":2,"title":"Timeout in checkout","state":"opened","labels":["bug"],
			"assignees":[{"id":8,"username":"dave"}],"author":{"id":7,"username":"carol"},
			"milestone":{"id":40,"iid":2,"title":"v1.1"},"user_notes_count":3,"closed_at":null,"description":null}]`))
	}))
	defer srv.Close()
	issues, pag, err := newTestClient(srv.URL).Issues.ListPage(testCtx, "acme/web", IssueListOpts{State: "opened",
		Labels: []string{"bug", "needs triage"}, Milestone: "v1.1", AuthorUsername: "carol", AssigneeUsername: "dave",
		Search: " timeout ", In: []string{"title"}, IIDs: []int{2, 6}, OrderBy: "created_at", Sort: "desc", Page: 2, PerPage: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %+v", issues)
	}
	iss := issues[0]
	if iss.IID != 2 || iss.Assignees[0].Username != "dave" || iss.Author.Username != "carol" || iss.Milestone.IID != 2 ||
		iss.UserNotesCount != 3 || iss.Description != "" || iss.ClosedAt != "" {
		t.Errorf("issue = %+v", iss)
	}
	if pag.NextPage != 3 || !pag.HasTotal || pag.Total != 11 {
		t.Errorf("pagination = %+v", pag)
	}
}

func TestIssueListOpts_AllStateAndEmptyFiltersAreNotSent(t *testing.T) {
	q := IssueListOpts{State: "all", AssigneeID: "None"}.query()
	if q.Has("state") || q.Has("labels") || q.Has("search") || q.Has("in") || q.Get("assignee_id") != "None" {
		t.Errorf("query = %v", q)
	}
}

func TestIssue_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/issues/7" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"iid":7,"title":"Test","state":"opened","updated_at":"2026-10-12T09:00:01.000Z"}`))
	}))
	defer srv.Close()
	issue, err := newTestClient(srv.URL).Issues.Get(testCtx, "42", 7)
	if err != nil || issue.IID != 7 || issue.Title != "Test" || issue.UpdatedAt == "" {
		t.Fatalf("Get = %+v, %v", issue, err)
	}
}

func TestIssue_Create(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/projects/42/issues" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"iid":3,"title":"New Issue","state":"opened"}`))
	}))
	defer srv.Close()
	issue, err := newTestClient(srv.URL).Issues.Create(testCtx, "42", IssueCreateOpts{Title: "New Issue", Labels: "bug,ops"})
	if err != nil || issue.IID != 3 {
		t.Fatalf("Create = %+v, %v", issue, err)
	}
	if body["title"] != "New Issue" || body["labels"] != "bug,ops" || body["description"] != nil || body["assignee_ids"] != nil {
		t.Errorf("body = %v", body)
	}
}

// TestIssue_UpdateSendsOnlyTheGivenFields: nil fields stay out of the body; an
// empty assignee list and milestone 0 are sent, because they clear (CLI-029).
func TestIssue_UpdateSendsOnlyTheGivenFields(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v4/projects/42/issues/3" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		body = nil
		_ = json.Unmarshal(b, &body)
		_, _ = w.Write([]byte(`{"iid":3,"title":"Updated","state":"closed"}`))
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	issue, err := c.Issues.Update(testCtx, "42", 3, IssueUpdateOpts{StateEvent: "close"})
	if err != nil || issue.State != "closed" {
		t.Fatalf("Update = %+v, %v", issue, err)
	}
	if len(body) != 1 || body["state_event"] != "close" {
		t.Errorf("body = %v", body)
	}
	empty, none, zero := "", []int{0}, 0
	if _, err := c.Issues.Update(testCtx, "42", 3, IssueUpdateOpts{Description: &empty, AssigneeIDs: &none, MilestoneID: &zero,
		AddLabels: "a,b"}); err != nil {
		t.Fatal(err)
	}
	if body["description"] != "" || !reflect.DeepEqual(body["assignee_ids"], []any{float64(0)}) || body["milestone_id"] != float64(0) ||
		body["add_labels"] != "a,b" || body["title"] != nil || body["remove_labels"] != nil {
		t.Errorf("body = %v", body)
	}
}

func TestIssue_Notes(t *testing.T) {
	var posted map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/issues/3/notes":
			q := r.URL.Query()
			if q.Get("order_by") != "created_at" || q.Get("sort") != "desc" || q.Get("page") != "1" || q.Get("per_page") != "20" {
				t.Errorf("query = %v", q)
			}
			w.Header().Set("X-Total", "2")
			_, _ = w.Write([]byte(`[{"id":10,"body":"hello","system":false,"noteable_iid":3},{"id":11,"body":"added ~bug","system":true}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42/issues/3/notes/10":
			_, _ = w.Write([]byte(`{"id":10,"body":"hello","author":{"username":"carol"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/projects/42/issues/3/notes":
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &posted)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":12,"body":"new comment"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/projects/42/issues/3/notes/12":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	notes, pag, err := c.Issues.ListNotesPage(testCtx, "42", 3, IssueNoteListOpts{OrderBy: "created_at", Sort: "desc", Page: 1, PerPage: 20})
	if err != nil || len(notes) != 2 || notes[0].NoteableIID != 3 || !notes[1].System || pag.Total != 2 {
		t.Fatalf("ListNotesPage = %+v %+v %v", notes, pag, err)
	}
	n, err := c.Issues.GetNote(testCtx, "42", 3, 10)
	if err != nil || n.Author.Username != "carol" {
		t.Fatalf("GetNote = %+v, %v", n, err)
	}
	n, err = c.Issues.AddNote(testCtx, "42", 3, "new comment")
	if err != nil || n.ID != 12 || posted["body"] != "new comment" {
		t.Fatalf("AddNote = %+v, %v (posted %v)", n, err, posted)
	}
	if err := c.Issues.DeleteNote(testCtx, "42", 3, 12); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
}

func TestIssue_ParseErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	title := "x"
	for name, call := range map[string]func() error{
		"parsing issues":         func() error { _, _, err := c.Issues.ListPage(testCtx, "42", IssueListOpts{}); return err },
		"parsing issue":          func() error { _, err := c.Issues.Get(testCtx, "42", 1); return err },
		"parsing created issue":  func() error { _, err := c.Issues.Create(testCtx, "42", IssueCreateOpts{Title: "x"}); return err },
		"parsing updated issue":  func() error { _, err := c.Issues.Update(testCtx, "42", 1, IssueUpdateOpts{Title: &title}); return err },
		"parsing notes":          func() error { _, _, err := c.Issues.ListNotesPage(testCtx, "42", 1, IssueNoteListOpts{}); return err },
		"parsing note (get)":     func() error { _, err := c.Issues.GetNote(testCtx, "42", 1, 2); return err },
		"parsing note (created)": func() error { _, err := c.Issues.AddNote(testCtx, "42", 1, "hi"); return err },
	} {
		want := strings.SplitN(name, " (", 2)[0]
		if err := call(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}
