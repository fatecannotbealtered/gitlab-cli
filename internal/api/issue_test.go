package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIssue_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v4/projects/") || !strings.HasSuffix(r.URL.Path, "/issues") {
			t.Errorf("unexpected path: %q", r.URL.Path)
		}
		if r.URL.Query().Get("state") != "opened" {
			t.Errorf("state = %q, want opened", r.URL.Query().Get("state"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"iid":1,"title":"Bug","state":"opened"},{"iid":2,"title":"Feature","state":"opened"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	issues, err := c.Issues.List(testCtx, "group/proj", &IssueListOpts{State: "opened", Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(issues) != 2 || issues[0].IID != 1 || issues[1].Title != "Feature" {
		t.Errorf("unexpected issues: %+v", issues)
	}
}

func TestIssue_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/issues/7" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"iid":7,"title":"Test","state":"opened"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	issue, err := c.Issues.Get(testCtx, "42", 7)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if issue.IID != 7 || issue.Title != "Test" {
		t.Errorf("unexpected issue: %+v", issue)
	}
}

func TestIssue_Create(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"iid":3,"title":"New Issue","state":"opened"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	issue, err := c.Issues.Create(testCtx, "42", IssueCreateOpts{Title: "New Issue"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if issue.IID != 3 || issue.Title != "New Issue" {
		t.Errorf("unexpected issue: %+v", issue)
	}
}

func TestIssue_Update(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"iid":3,"title":"Updated","state":"closed"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	issue, err := c.Issues.Update(testCtx, "42", 3, IssueUpdateOpts{StateEvent: "close"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if issue.State != "closed" {
		t.Errorf("state = %q, want closed", issue.State)
	}
}

func TestIssue_ListNotes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/notes") {
			t.Errorf("path = %q, want suffix /notes", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":10,"body":"hello","system":false}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	notes, err := c.Issues.ListNotes(testCtx, "42", 3, 20)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 1 || notes[0].ID != 10 || notes[0].Body != "hello" {
		t.Errorf("unexpected notes: %+v", notes)
	}
}

func TestIssue_AddNote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":11,"body":"new comment"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	note, err := c.Issues.AddNote(testCtx, "42", 3, "new comment")
	if err != nil {
		t.Fatalf("AddNote: %v", err)
	}
	if note.ID != 11 {
		t.Errorf("note.ID = %d, want 11", note.ID)
	}
}

func TestIssue_DeleteNote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if err := c.Issues.DeleteNote(testCtx, "42", 3, 11); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
}

func TestIssueListOpts_Encode(t *testing.T) {
	opts := &IssueListOpts{State: "opened", AssigneeUsername: "alice", Limit: 5}
	q := opts.encode()
	if !strings.Contains(q, "state=opened") {
		t.Errorf("missing state: %s", q)
	}
	if !strings.Contains(q, "assignee_username=alice") {
		t.Errorf("missing assignee_username: %s", q)
	}
	if !strings.Contains(q, "per_page=5") {
		t.Errorf("missing per_page: %s", q)
	}
}

func TestIssueListOpts_AllState_OmitsStateParam(t *testing.T) {
	opts := &IssueListOpts{State: "all", Limit: 10}
	q := opts.encode()
	if strings.Contains(q, "state=") {
		t.Errorf("state=all should not be sent, got: %s", q)
	}
}
