package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
)

func newMRTestClient(serverURL string) *Client {
	return NewClient(&config.Config{Host: serverURL, Token: "test-token"})
}

func TestMR_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/merge_requests") {
			t.Errorf("path = %q, want contains /merge_requests", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"iid":1,"title":"feat: add login","state":"opened","source_branch":"feat/login","target_branch":"main"}]`))
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	mrs, err := c.MergeRequests.List(testCtx, "group/proj", &MergeRequestListOpts{State: "opened"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(mrs) != 1 || mrs[0].IID != 1 || mrs[0].Title != "feat: add login" {
		t.Errorf("unexpected: %+v", mrs)
	}
}

func TestMR_List_WithOpts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != "closed" {
			t.Errorf("state = %q, want closed", q.Get("state"))
		}
		if q.Get("assignee_username") != "alice" {
			t.Errorf("assignee_username = %q, want alice", q.Get("assignee_username"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	_, err := c.MergeRequests.List(testCtx, "123", &MergeRequestListOpts{State: "closed", AssigneeUser: "alice"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
}

func TestMR_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/merge_requests/42") {
			t.Errorf("path = %q, want suffix /merge_requests/42", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"iid":42,"title":"fix: bug","state":"opened","source_branch":"fix/bug","target_branch":"main","author":{"id":1,"username":"bob"}}`))
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	mr, err := c.MergeRequests.Get(testCtx, "group/proj", 42)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if mr.IID != 42 || mr.Author == nil || mr.Author.Username != "bob" {
		t.Errorf("unexpected: %+v", mr)
	}
}

func TestMR_Create(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"iid":5,"title":"new feature","state":"opened","source_branch":"feat/x","target_branch":"main"}`))
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	mr, err := c.MergeRequests.Create(testCtx, "group/proj", &MergeRequestCreateRequest{
		Title:        "new feature",
		SourceBranch: "feat/x",
		TargetBranch: "main",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if mr.IID != 5 || mr.Title != "new feature" {
		t.Errorf("unexpected: %+v", mr)
	}
}

func TestMR_Update(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"iid":3,"title":"updated title","state":"opened","source_branch":"feat/x","target_branch":"main"}`))
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	mr, err := c.MergeRequests.Update(testCtx, "group/proj", 3, &MergeRequestUpdateRequest{Title: "updated title"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if mr.Title != "updated title" {
		t.Errorf("unexpected title: %q", mr.Title)
	}
}

func TestMR_Merge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/merge") {
			t.Errorf("path = %q, want suffix /merge", r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"iid":7,"title":"done","state":"merged","source_branch":"feat/x","target_branch":"main"}`))
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	mr, err := c.MergeRequests.Merge(testCtx, "group/proj", 7, &MergeRequestMergeRequest{Squash: true})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if mr.State != "merged" {
		t.Errorf("state = %q, want merged", mr.State)
	}
}

func TestMR_Approve(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/approve") {
			t.Errorf("path = %q, want suffix /approve", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	if err := c.MergeRequests.Approve(testCtx, "group/proj", 7); err != nil {
		t.Fatalf("Approve: %v", err)
	}
}

func TestMR_Unapprove(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/unapprove") {
			t.Errorf("path = %q, want suffix /unapprove", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	if err := c.MergeRequests.Unapprove(testCtx, "group/proj", 7); err != nil {
		t.Fatalf("Unapprove: %v", err)
	}
}

func TestMR_Close_StateEvent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"iid":2,"title":"x","state":"closed","source_branch":"b","target_branch":"main"}`))
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	mr, err := c.MergeRequests.Close(testCtx, "group/proj", 2)
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if mr.State != "closed" {
		t.Errorf("state = %q, want closed", mr.State)
	}
}

func TestMR_GetRawDiff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/raw_diffs") {
			t.Errorf("path = %q, want suffix /raw_diffs", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("--- a/file.go\n+++ b/file.go\n@@ -1 +1 @@\n-old\n+new\n"))
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	diff, err := c.MergeRequests.GetRawDiff(testCtx, "group/proj", 1)
	if err != nil {
		t.Fatalf("GetRawDiff: %v", err)
	}
	if !strings.Contains(diff, "--- a/file.go") {
		t.Errorf("unexpected diff: %q", diff)
	}
}

func TestMR_ListNotes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/notes") {
			t.Errorf("path = %q, want suffix /notes", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":10,"body":"LGTM","author":{"id":1,"username":"alice"},"created_at":"2024-01-01T00:00:00Z"}]`))
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	notes, err := c.MergeRequests.ListNotes(testCtx, "group/proj", 1, 20)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if len(notes) != 1 || notes[0].ID != 10 || notes[0].Body != "LGTM" {
		t.Errorf("unexpected: %+v", notes)
	}
}

func TestMR_AddNote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":11,"body":"looks good","author":{"id":2,"username":"bob"},"created_at":"2024-01-02T00:00:00Z"}`))
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	note, err := c.MergeRequests.AddNote(testCtx, "group/proj", 1, "looks good")
	if err != nil {
		t.Fatalf("AddNote: %v", err)
	}
	if note.ID != 11 || note.Body != "looks good" {
		t.Errorf("unexpected: %+v", note)
	}
}

func TestMR_DeleteNote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/notes/99") {
			t.Errorf("path = %q, want suffix /notes/99", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	if err := c.MergeRequests.DeleteNote(testCtx, "group/proj", 1, 99); err != nil {
		t.Fatalf("DeleteNote: %v", err)
	}
}

// 鈹€鈹€鈹€ Error path tests 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func TestMR_List_401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	_, err := c.MergeRequests.List(testCtx, "group/proj", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 401 {
		t.Errorf("expected 401 APIError, got %v", err)
	}
}

func TestMR_Get_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	_, err := c.MergeRequests.Get(testCtx, "group/proj", 9999)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Errorf("expected 404 APIError, got %v", err)
	}
}

func TestMR_Create_422(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":{"source_branch":["is invalid"]}}`))
	}))
	defer srv.Close()

	c := newMRTestClient(srv.URL)
	_, err := c.MergeRequests.Create(testCtx, "group/proj", &MergeRequestCreateRequest{
		Title:        "x",
		SourceBranch: "bad branch",
		TargetBranch: "main",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 422 {
		t.Errorf("expected 422 APIError, got %v", err)
	}
}
