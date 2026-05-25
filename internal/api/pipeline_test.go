package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPipeline_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v4/projects/") || !strings.HasSuffix(r.URL.Path, "/pipelines") {
			t.Errorf("unexpected path: %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"iid":1,"ref":"main","status":"success"},{"id":2,"iid":2,"ref":"main","status":"failed"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	pipelines, err := c.Pipelines.List(testCtx, "group/proj", &PipelineListOpts{Ref: "main", Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pipelines) != 2 || pipelines[0].ID != 1 || pipelines[1].Status != "failed" {
		t.Errorf("unexpected pipelines: %+v", pipelines)
	}
}

func TestPipeline_List_StatusFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("status") != "running" {
			t.Errorf("expected status=running, got %q", r.URL.Query().Get("status"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":3,"status":"running"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	pipelines, err := c.Pipelines.List(testCtx, "42", &PipelineListOpts{Status: "running"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pipelines) != 1 || pipelines[0].Status != "running" {
		t.Errorf("unexpected: %+v", pipelines)
	}
}

func TestPipeline_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/pipelines/7" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":7,"iid":3,"ref":"feature","status":"running","web_url":"https://x"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	p, err := c.Pipelines.Get(testCtx, "42", 7)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.ID != 7 || p.Status != "running" {
		t.Errorf("unexpected: %+v", p)
	}
}

func TestPipeline_Create(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		// Note singular /pipeline (GitLab quirk)
		if r.URL.Path != "/api/v4/projects/42/pipeline" {
			t.Errorf("path = %q, want /api/v4/projects/42/pipeline", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":10,"ref":"main","status":"created"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	p, err := c.Pipelines.Create(testCtx, "42", PipelineCreateBody{Ref: "main"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID != 10 || p.Status != "created" {
		t.Errorf("unexpected: %+v", p)
	}
}

func TestPipeline_Retry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/retry") {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":10,"status":"pending"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	p, err := c.Pipelines.Retry(testCtx, "42", 10)
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if p.Status != "pending" {
		t.Errorf("unexpected status: %q", p.Status)
	}
}

func TestPipeline_Cancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/cancel") {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":10,"status":"canceled"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	p, err := c.Pipelines.Cancel(testCtx, "42", 10)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if p.Status != "canceled" {
		t.Errorf("unexpected status: %q", p.Status)
	}
}

func TestPipeline_Jobs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/jobs") {
			t.Errorf("path = %q", r.URL.Path)
		}
		scopes := r.URL.Query()["scope[]"]
		if len(scopes) != 1 || scopes[0] != "failed" {
			t.Errorf("scope[] = %v, want [failed]", scopes)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":5,"name":"test","status":"failed"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	jobs, err := c.Pipelines.Jobs(testCtx, "42", 10, []string{"failed"})
	if err != nil {
		t.Fatalf("Jobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != 5 {
		t.Errorf("unexpected: %+v", jobs)
	}
}

func TestPipeline_List_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Pipelines.List(testCtx, "42", nil)
	if err == nil || !strings.Contains(err.Error(), "parsing pipelines") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestPipeline_Get_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Pipelines.Get(testCtx, "42", 1)
	if err == nil || !strings.Contains(err.Error(), "parsing pipeline") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestPipeline_Create_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Pipelines.Create(testCtx, "42", PipelineCreateBody{Ref: "main"})
	if err == nil || !strings.Contains(err.Error(), "parsing pipeline") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestPipeline_Retry_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Pipelines.Retry(testCtx, "42", 1)
	if err == nil || !strings.Contains(err.Error(), "parsing pipeline") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestPipeline_Cancel_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Pipelines.Cancel(testCtx, "42", 1)
	if err == nil || !strings.Contains(err.Error(), "parsing pipeline") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestPipeline_Jobs_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Pipelines.Jobs(testCtx, "42", 1, nil)
	if err == nil || !strings.Contains(err.Error(), "parsing jobs") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestPipelineListOpts_EncodeUsername(t *testing.T) {
	opts := &PipelineListOpts{Username: "alice", Ref: "main"}
	q := opts.encode()
	if !strings.Contains(q, "username=alice") || !strings.Contains(q, "ref=main") {
		t.Errorf("encode() = %q", q)
	}
	if (*PipelineListOpts)(nil).encode() != "" {
		t.Error("nil encode should be empty")
	}
}
