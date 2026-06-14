package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
)

func newProjectTestClient(serverURL string) *Client {
	return NewClient(&config.Config{Host: serverURL, Token: "test-token"})
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

func TestProject_Members(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/members") {
			t.Errorf("path = %q, want contains /members", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":10,"username":"alice","name":"Alice","state":"active","access_level":40}]`))
	}))
	defer srv.Close()

	c := newProjectTestClient(srv.URL)
	members, err := c.Projects.Members(testCtx, "123", "", 20)
	if err != nil {
		t.Fatalf("Members: %v", err)
	}
	if len(members) != 1 || members[0].Username != "alice" {
		t.Errorf("unexpected: %+v", members)
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

func TestProject_Members_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newProjectTestClient(srv.URL)
	_, err := c.Projects.Members(testCtx, "1", "", 20)
	if err == nil || !strings.Contains(err.Error(), "parsing project members") {
		t.Fatalf("expected parse error, got %v", err)
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
