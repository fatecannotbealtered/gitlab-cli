package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
)

func newSearchTestClient(serverURL string) *Client {
	return NewClient(&config.Config{Host: serverURL, Token: "test-token"})
}

func TestSearch_Projects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/search" {
			t.Errorf("path = %q, want /api/v4/search", r.URL.Path)
		}
		if r.URL.Query().Get("scope") != "projects" {
			t.Errorf("scope = %q, want projects", r.URL.Query().Get("scope"))
		}
		if r.URL.Query().Get("search") != "myquery" {
			t.Errorf("search = %q, want myquery", r.URL.Query().Get("search"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"name":"found","path_with_namespace":"g/found","web_url":"https://gl.example.com/g/found"}]`))
	}))
	defer srv.Close()

	c := newSearchTestClient(srv.URL)
	results, err := c.Search.Projects(testCtx, "myquery", 10)
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	if len(results) != 1 || results[0].ID != 1 {
		t.Errorf("unexpected: %+v", results)
	}
}

func TestSearch_Issues_Global(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/search" {
			t.Errorf("path = %q, want /api/v4/search", r.URL.Path)
		}
		if r.URL.Query().Get("scope") != "issues" {
			t.Errorf("scope = %q, want issues", r.URL.Query().Get("scope"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":5,"iid":2,"title":"bug","state":"opened","web_url":"https://gl.example.com/g/p/-/issues/2"}]`))
	}))
	defer srv.Close()

	c := newSearchTestClient(srv.URL)
	results, err := c.Search.Issues(testCtx, "bug", "", 10)
	if err != nil {
		t.Fatalf("Issues: %v", err)
	}
	if len(results) != 1 || results[0].IID != 2 {
		t.Errorf("unexpected: %+v", results)
	}
}

func TestSearch_Issues_ProjectScoped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/search" {
			t.Errorf("path = %q, want /api/v4/projects/42/search", r.URL.Path)
		}
		if r.URL.Query().Get("scope") != "issues" {
			t.Errorf("scope = %q, want issues", r.URL.Query().Get("scope"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newSearchTestClient(srv.URL)
	_, err := c.Search.Issues(testCtx, "bug", "42", 10)
	if err != nil {
		t.Fatalf("Issues project-scoped: %v", err)
	}
}

func TestSearch_Code_RequiresProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/99/search" {
			t.Errorf("path = %q, want /api/v4/projects/99/search", r.URL.Path)
		}
		if r.URL.Query().Get("scope") != "blobs" {
			t.Errorf("scope = %q, want blobs", r.URL.Query().Get("scope"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"basename":"main","data":"func main()","path":"main.go","filename":"main.go","ref":"main","startline":1,"project_id":99}]`))
	}))
	defer srv.Close()

	c := newSearchTestClient(srv.URL)
	results, err := c.Search.Code(testCtx, "func main", "99", 5)
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if len(results) != 1 || results[0].Filename != "main.go" {
		t.Errorf("unexpected: %+v", results)
	}
}

func TestSearch_MergeRequests_Global(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/search" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("scope") != "merge_requests" {
			t.Errorf("scope = %q, want merge_requests", r.URL.Query().Get("scope"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":9,"iid":4,"title":"feat","state":"opened","web_url":"https://gl.example.com/mr/4","project_id":1}]`))
	}))
	defer srv.Close()

	c := newSearchTestClient(srv.URL)
	results, err := c.Search.MergeRequests(testCtx, "feat", "", 10)
	if err != nil {
		t.Fatalf("MergeRequests: %v", err)
	}
	if len(results) != 1 || results[0].IID != 4 {
		t.Errorf("unexpected: %+v", results)
	}
}

func TestSearch_MergeRequests_ProjectScoped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/search" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("scope") != "merge_requests" {
			t.Errorf("scope = %q", r.URL.Query().Get("scope"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newSearchTestClient(srv.URL)
	if _, err := c.Search.MergeRequests(testCtx, "fix", "42", 10); err != nil {
		t.Fatalf("MergeRequests project-scoped: %v", err)
	}
}

func TestSearch_MergeRequests_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	c := newSearchTestClient(srv.URL)
	_, err := c.Search.MergeRequests(testCtx, "x", "", 10)
	if err == nil || !strings.Contains(err.Error(), "parsing search mrs") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestSearch_Projects_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newSearchTestClient(srv.URL)
	_, err := c.Search.Projects(testCtx, "q", 10)
	if err == nil || !strings.Contains(err.Error(), "parsing search projects") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestSearch_Issues_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newSearchTestClient(srv.URL)
	_, err := c.Search.Issues(testCtx, "q", "", 10)
	if err == nil || !strings.Contains(err.Error(), "parsing search issues") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestSearch_Code_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newSearchTestClient(srv.URL)
	_, err := c.Search.Code(testCtx, "q", "99", 5)
	if err == nil || !strings.Contains(err.Error(), "parsing search blobs") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestSearch_Commits_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newSearchTestClient(srv.URL)
	_, err := c.Search.Commits(testCtx, "q", "", 10)
	if err == nil || !strings.Contains(err.Error(), "parsing search commits") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestSearch_Commits_Global(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("scope") != "commits" {
			t.Errorf("scope = %q, want commits", r.URL.Query().Get("scope"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"abc123","short_id":"abc123","title":"fix: bug","author_name":"Alice","created_at":"2024-01-01T00:00:00Z","web_url":"https://gl.example.com/g/p/-/commit/abc123"}]`))
	}))
	defer srv.Close()

	c := newSearchTestClient(srv.URL)
	results, err := c.Search.Commits(testCtx, "fix", "", 10)
	if err != nil {
		t.Fatalf("Commits: %v", err)
	}
	if len(results) != 1 || results[0].ShortID != "abc123" {
		t.Errorf("unexpected: %+v", results)
	}
}
