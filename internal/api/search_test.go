package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newSearchTestClient(serverURL string) *Client {
	return NewClient(&Config{Host: serverURL, Token: "test-token"})
}

// searchServer answers one search request after checking its path, scope, query
// and page, with body.
func searchServer(t *testing.T, path, scope, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.EscapedPath() != path || q.Get("scope") != scope || q.Get("search") != "login bug" ||
			q.Get("page") != "2" || q.Get("per_page") != "5" {
			t.Errorf("request = %s?%s, want %s scope=%s", r.URL.EscapedPath(), r.URL.RawQuery, path, scope)
		}
		w.Header().Set("X-Next-Page", "3")
		_, _ = w.Write([]byte(body))
	}))
}

func TestSearch_ProjectsPage(t *testing.T) {
	srv := searchServer(t, "/api/v4/search", "projects",
		`[{"id":1,"name":"found","path_with_namespace":"g/found","web_url":"https://gl.example.com/g/found"}]`)
	defer srv.Close()
	ps, pag, err := newSearchTestClient(srv.URL).Search.ProjectsPage(testCtx, " login bug ", 2, 5)
	if err != nil || len(ps) != 1 || ps[0].PathWithNamespace != "g/found" || ps[0].Archived != nil || pag.NextPage != 3 {
		t.Fatalf("ProjectsPage = %+v, %+v, %v", ps, pag, err)
	}
}

func TestSearch_IssuesPage(t *testing.T) {
	body := `[{"id":5,"iid":2,"project_id":1,"title":"bug","state":"opened","labels":["bug"],"author":{"username":"carol"},` +
		`"web_url":"https://gl.example.com/g/p/-/issues/2","references":{"full":"g/p#2"}}]`
	for _, c := range []struct{ project, path string }{{"", "/api/v4/search"}, {"g/p", "/api/v4/projects/g%2Fp/search"}} {
		srv := searchServer(t, c.path, "issues", body)
		is, _, err := newSearchTestClient(srv.URL).Search.IssuesPage(testCtx, SearchOpts{Project: c.project, Query: "login bug", Page: 2, PerPage: 5})
		srv.Close()
		if err != nil || len(is) != 1 || is[0].IID != 2 || is[0].References.Full != "g/p#2" || is[0].Author.Username != "carol" {
			t.Fatalf("IssuesPage(%q) = %+v, %v", c.project, is, err)
		}
	}
}

func TestSearch_MergeRequestsPage(t *testing.T) {
	srv := searchServer(t, "/api/v4/projects/42/search", "merge_requests",
		`[{"id":9,"iid":4,"title":"feat","state":"opened","draft":true,"source_branch":"feat","target_branch":"main","references":{"full":"g/p!4"}}]`)
	defer srv.Close()
	ms, _, err := newSearchTestClient(srv.URL).Search.MergeRequestsPage(testCtx, SearchOpts{Project: "42", Query: "login bug", Page: 2, PerPage: 5})
	if err != nil || len(ms) != 1 || ms[0].IID != 4 || !ms[0].Draft || ms[0].TargetBranch != "main" {
		t.Fatalf("MergeRequestsPage = %+v, %v", ms, err)
	}
}

func TestSearch_BlobsPage(t *testing.T) {
	srv := searchServer(t, "/api/v4/projects/99/search", "blobs",
		`[{"basename":"main","data":"func main()","path":"cmd/main.go","filename":"main.go","ref":"main","startline":7,"project_id":99}]`)
	defer srv.Close()
	bs, _, err := newSearchTestClient(srv.URL).Search.BlobsPage(testCtx, SearchOpts{Project: "99", Query: "login bug", Page: 2, PerPage: 5})
	if err != nil || len(bs) != 1 || bs[0].Path != "cmd/main.go" || bs[0].StartLine != 7 || bs[0].Ref != "main" {
		t.Fatalf("BlobsPage = %+v, %v", bs, err)
	}
}

func TestSearch_CommitsPage(t *testing.T) {
	srv := searchServer(t, "/api/v4/search", "commits",
		`[{"id":"abc123def","short_id":"abc123d","title":"fix: bug","author_name":"Alice","created_at":"2024-01-01T00:00:00Z","web_url":"https://gl.example.com/g/p/-/commit/abc123def"}]`)
	defer srv.Close()
	cs, _, err := newSearchTestClient(srv.URL).Search.CommitsPage(testCtx, SearchOpts{Query: "login bug", Page: 2, PerPage: 5})
	if err != nil || len(cs) != 1 || cs[0].ShortID != "abc123d" || !strings.HasSuffix(cs[0].WebURL, "/commit/abc123def") {
		t.Fatalf("CommitsPage = %+v, %v", cs, err)
	}
}

func TestSearch_ParseErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()
	c := newSearchTestClient(srv.URL)
	opts := SearchOpts{Project: "1", Query: "q"}
	checks := map[string]func() error{
		"projects":       func() error { _, _, err := c.Search.ProjectsPage(testCtx, "q", 1, 5); return err },
		"issues":         func() error { _, _, err := c.Search.IssuesPage(testCtx, opts); return err },
		"merge_requests": func() error { _, _, err := c.Search.MergeRequestsPage(testCtx, opts); return err },
		"blobs":          func() error { _, _, err := c.Search.BlobsPage(testCtx, opts); return err },
		"commits":        func() error { _, _, err := c.Search.CommitsPage(testCtx, opts); return err },
	}
	for scope, fn := range checks {
		if err := fn(); err == nil || !strings.Contains(err.Error(), "parsing search "+scope) {
			t.Errorf("%s: expected parse error, got %v", scope, err)
		}
	}
}
