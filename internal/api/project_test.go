package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newProjectTestClient(serverURL string) *Client {
	return NewClient(&Config{Host: serverURL, Token: "test-token"})
}

func TestProject_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if r.URL.Path != "/api/v4/projects" {
			t.Errorf("path = %q, want /api/v4/projects", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"name":"myproject","path_with_namespace":"group/myproject","visibility":"private","web_url":"https://gitlab.example.com/group/myproject","default_branch":"main"}]`))
	}))
	defer srv.Close()

	c := newProjectTestClient(srv.URL)
	projects, err := c.Projects.List(testCtx, &ProjectListOpts{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(projects) != 1 || projects[0].ID != 1 || projects[0].Name != "myproject" {
		t.Errorf("unexpected: %+v", projects)
	}
}

func TestProject_List_WithOpts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("owned") != "true" {
			t.Errorf("owned = %q, want true", q.Get("owned"))
		}
		if q.Get("visibility") != "public" {
			t.Errorf("visibility = %q, want public", q.Get("visibility"))
		}
		if q.Get("search") != "foo" {
			t.Errorf("search = %q, want foo", q.Get("search"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newProjectTestClient(srv.URL)
	_, err := c.Projects.List(testCtx, &ProjectListOpts{Owned: true, Visibility: "public", Search: "foo"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
}

func TestProject_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Go HTTP server decodes %2F to / in r.URL.Path; check RawPath for encoded form.
		rawPath := r.URL.RawPath
		if rawPath == "" {
			rawPath = r.URL.Path
		}
		if !strings.HasSuffix(rawPath, "/projects/group%2Fmyproject") {
			t.Errorf("rawPath = %q, want suffix /projects/group%%2Fmyproject", rawPath)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":42,"name":"myproject","path_with_namespace":"group/myproject","visibility":"public","web_url":"https://gitlab.example.com/group/myproject","default_branch":"main"}`))
	}))
	defer srv.Close()

	c := newProjectTestClient(srv.URL)
	p, err := c.Projects.Get(testCtx, "group/myproject")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.ID != 42 || p.Name != "myproject" {
		t.Errorf("unexpected: %+v", p)
	}
}

func TestProject_ListPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api/v4/projects" || q.Get("search") != "payments" || q.Get("simple") != "true" ||
			q.Get("membership") != "true" || q.Get("order_by") != "created_at" || q.Get("sort") != "desc" ||
			q.Get("page") != "2" || q.Get("per_page") != "5" {
			t.Errorf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("X-Next-Page", "3")
		w.Header().Set("X-Total", "11")
		_, _ = w.Write([]byte(`[{"id":3,"name":"Payments Service","path":"payments-service","path_with_namespace":"acme/payments-service","archived":false}]`))
	}))
	defer srv.Close()
	ps, pag, err := newProjectTestClient(srv.URL).Projects.ListPage(testCtx, ProjectListOpts{Search: "payments", Simple: true,
		Membership: true, OrderBy: "created_at", Sort: "desc", Page: 2, PerPage: 5})
	if err != nil || len(ps) != 1 || ps[0].Path != "payments-service" || ps[0].Archived == nil || *ps[0].Archived {
		t.Fatalf("ListPage = %+v, %v", ps, err)
	}
	if pag.NextPage != 3 || !pag.HasTotal || pag.Total != 11 {
		t.Errorf("pagination = %+v", pag)
	}
}

func TestProject_MembersPages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch r.URL.EscapedPath() {
		case "/api/v4/projects/acme%2Fweb/members/all":
			if q.Get("query") != "car" || q.Get("page") != "1" || q.Get("per_page") != "20" {
				t.Errorf("effective query = %v", q)
			}
			_, _ = w.Write([]byte(`[{"id":10,"username":"alice","name":"Alice","state":"active","access_level":40,"expires_at":"2026-12-31"}]`))
		case "/api/v4/projects/acme%2Fweb/members":
			if ids := q["user_ids[]"]; len(ids) != 2 || ids[0] != "10" || ids[1] != "11" {
				t.Errorf("direct user_ids = %v", ids)
			}
			_, _ = w.Write([]byte(`[{"id":10,"username":"alice","access_level":30}]`))
		default:
			t.Errorf("path = %q", r.URL.EscapedPath())
		}
	}))
	defer srv.Close()
	c := newProjectTestClient(srv.URL)
	all, _, err := c.Projects.EffectiveMembersPage(testCtx, "acme/web", MemberListOpts{Query: "car", Page: 1, PerPage: 20})
	if err != nil || len(all) != 1 || all[0].Username != "alice" || all[0].AccessLevel != 40 || all[0].ExpiresAt != "2026-12-31" {
		t.Fatalf("EffectiveMembersPage = %+v, %v", all, err)
	}
	direct, _, err := c.Projects.DirectMembersPage(testCtx, "acme/web", MemberListOpts{UserIDs: []int{10, 11}, PerPage: 100})
	if err != nil || len(direct) != 1 || direct[0].AccessLevel != 30 {
		t.Fatalf("DirectMembersPage = %+v, %v", direct, err)
	}
}

func TestProject_GetNamespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/api/v4/namespaces/acme%2Fplatform" {
			t.Errorf("path = %q", r.URL.EscapedPath())
		}
		_, _ = w.Write([]byte(`{"id":87,"name":"Platform","path":"platform","kind":"group","full_path":"acme/platform"}`))
	}))
	defer srv.Close()
	ns, err := newProjectTestClient(srv.URL).Projects.GetNamespace(testCtx, "acme/platform")
	if err != nil || ns.ID != 87 || ns.Kind != "group" || ns.FullPath != "acme/platform" {
		t.Fatalf("GetNamespace = %+v, %v", ns, err)
	}
}

func TestProject_List_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newProjectTestClient(srv.URL)
	_, err := c.Projects.List(testCtx, nil)
	if err == nil || !strings.Contains(err.Error(), "parsing projects") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestProject_Get_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newProjectTestClient(srv.URL)
	_, err := c.Projects.Get(testCtx, "1")
	if err == nil || !strings.Contains(err.Error(), "parsing project") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestProject_PageParseErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newProjectTestClient(srv.URL)
	if _, _, err := c.Projects.ListPage(testCtx, ProjectListOpts{}); err == nil || !strings.Contains(err.Error(), "parsing projects") {
		t.Fatalf("ListPage: expected parse error, got %v", err)
	}
	if _, _, err := c.Projects.EffectiveMembersPage(testCtx, "1", MemberListOpts{}); err == nil || !strings.Contains(err.Error(), "parsing project members") {
		t.Fatalf("EffectiveMembersPage: expected parse error, got %v", err)
	}
	if _, err := c.Projects.GetNamespace(testCtx, "acme"); err == nil || !strings.Contains(err.Error(), "parsing namespace") {
		t.Fatalf("GetNamespace: expected parse error, got %v", err)
	}
}

func TestProjectListOpts_EncodeMembership(t *testing.T) {
	opts := &ProjectListOpts{Membership: true, Limit: 0}
	q := opts.encode()
	if !strings.Contains(q, "membership=true") {
		t.Errorf("encode() = %q", q)
	}
	if (*ProjectListOpts)(nil).encode() != "" {
		t.Error("nil encode should be empty")
	}
}

func TestProject_Create(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/api/v4/projects" {
			t.Errorf("path = %q, want /api/v4/projects", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":7,"name":"My App","path_with_namespace":"alice/my-app","visibility":"private","web_url":"https://gitlab.example.com/alice/my-app","default_branch":"main"}`))
	}))
	defer srv.Close()

	c := newProjectTestClient(srv.URL)
	p, err := c.Projects.Create(testCtx, &ProjectCreateRequest{Name: "My App", Visibility: "private"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID != 7 || p.PathWithNamespace != "alice/my-app" {
		t.Errorf("unexpected: %+v", p)
	}
}

func TestProject_Create_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	c := newProjectTestClient(srv.URL)
	if _, err := c.Projects.Create(testCtx, &ProjectCreateRequest{Name: "X"}); err == nil || !strings.Contains(err.Error(), "parsing created project") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
