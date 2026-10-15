package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPipeline_ListPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api/v4/projects/group/proj/pipelines" && r.URL.EscapedPath() != "/api/v4/projects/group%2Fproj/pipelines" {
			t.Errorf("unexpected path: %q", r.URL.EscapedPath())
		}
		if q.Get("ref") != "main" || q.Get("status") != "failed" || q.Get("username") != "carol" || q.Get("page") != "2" || q.Get("per_page") != "5" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		w.Header().Set("X-Next-Page", "3")
		w.Header().Set("X-Total", "12")
		_, _ = w.Write([]byte(`[{"id":1,"iid":1,"ref":"main","status":"failed","name":"Nightly"},{"id":2,"iid":2,"ref":"main","status":"failed"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	ps, pag, err := c.Pipelines.ListPage(testCtx, "group/proj", PipelineListOpts{Ref: "main", Status: "failed", Username: "carol", Page: 2, PerPage: 5})
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	if len(ps) != 2 || ps[0].Name != "Nightly" || pag.NextPage != 3 || !pag.HasTotal || pag.Total != 12 {
		t.Errorf("unexpected: %+v %+v", ps, pag)
	}
}

func TestPipeline_ListPage_NoFilters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, k := range []string{"ref", "status", "username"} {
			if r.URL.Query().Has(k) {
				t.Errorf("%s sent without being asked: %q", k, r.URL.RawQuery)
			}
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	if _, _, err := newTestClient(srv.URL).Pipelines.ListPage(testCtx, "42", PipelineListOpts{}); err != nil {
		t.Fatal(err)
	}
}

func TestPipeline_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/pipelines/7" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":7,"iid":3,"ref":"feature","status":"running","web_url":"https://x","duration":61,"coverage":"80.5",
			"yaml_errors":null,"user":{"username":"carol"},"started_at":"2026-10-12T09:00:00Z"}`))
	}))
	defer srv.Close()

	p, err := newTestClient(srv.URL).Pipelines.Get(testCtx, "42", 7)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if p.ID != 7 || p.Status != "running" || p.Duration == nil || *p.Duration != 61 || *p.Coverage != "80.5" ||
		p.YamlErrors != nil || p.User.Username != "carol" || p.StartedAt == "" {
		t.Errorf("unexpected: %+v", p)
	}
}

func TestPipeline_Create(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// GitLab's create endpoint is the singular /pipeline.
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/projects/42/pipeline" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":10,"ref":"main","status":"created"}`))
	}))
	defer srv.Close()

	p, err := newTestClient(srv.URL).Pipelines.Create(testCtx, "42", PipelineCreateBody{Ref: "main",
		Variables: []PipelineVariable{{Key: "A", Value: "1"}}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID != 10 || p.Status != "created" {
		t.Errorf("unexpected: %+v", p)
	}
}

func TestPipeline_RetryAndCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/pipelines/10/retry"):
			_, _ = w.Write([]byte(`{"id":10,"status":"pending"}`))
		case strings.HasSuffix(r.URL.Path, "/pipelines/10/cancel"):
			_, _ = w.Write([]byte(`{"id":10,"status":"canceled"}`))
		default:
			t.Errorf("path = %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if p, err := c.Pipelines.Retry(testCtx, "42", 10); err != nil || p.Status != "pending" {
		t.Fatalf("Retry: %+v %v", p, err)
	}
	if p, err := c.Pipelines.Cancel(testCtx, "42", 10); err != nil || p.Status != "canceled" {
		t.Fatalf("Cancel: %+v %v", p, err)
	}
}

func TestPipeline_JobsPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/pipelines/10/jobs") {
			t.Errorf("path = %q", r.URL.Path)
		}
		if scopes := r.URL.Query()["scope[]"]; len(scopes) != 2 || scopes[0] != "failed" || scopes[1] != "canceled" {
			t.Errorf("scope[] = %v", scopes)
		}
		_, _ = w.Write([]byte(`[{"id":5,"name":"test","status":"failed","failure_reason":"script_failure","pipeline":{"id":10}}]`))
	}))
	defer srv.Close()

	jobs, _, err := newTestClient(srv.URL).Pipelines.JobsPage(testCtx, "42", 10, PipelineJobsOpts{Scope: []string{"failed", "canceled"}})
	if err != nil {
		t.Fatalf("JobsPage: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != 5 || jobs[0].FailureReason != "script_failure" || jobs[0].Pipeline.ID != 10 {
		t.Errorf("unexpected: %+v", jobs)
	}
}

func TestPipeline_ParseErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, _, err := c.Pipelines.ListPage(testCtx, "42", PipelineListOpts{}); err == nil || !strings.Contains(err.Error(), "parsing pipelines") {
		t.Errorf("ListPage: %v", err)
	}
	if _, err := c.Pipelines.Get(testCtx, "42", 1); err == nil || !strings.Contains(err.Error(), "parsing pipeline") {
		t.Errorf("Get: %v", err)
	}
	if _, err := c.Pipelines.Create(testCtx, "42", PipelineCreateBody{Ref: "main"}); err == nil || !strings.Contains(err.Error(), "parsing pipeline") {
		t.Errorf("Create: %v", err)
	}
	if _, err := c.Pipelines.Retry(testCtx, "42", 1); err == nil || !strings.Contains(err.Error(), "parsing pipeline") {
		t.Errorf("Retry: %v", err)
	}
	if _, err := c.Pipelines.Cancel(testCtx, "42", 1); err == nil || !strings.Contains(err.Error(), "parsing pipeline") {
		t.Errorf("Cancel: %v", err)
	}
	if _, _, err := c.Pipelines.JobsPage(testCtx, "42", 1, PipelineJobsOpts{}); err == nil || !strings.Contains(err.Error(), "parsing jobs") {
		t.Errorf("JobsPage: %v", err)
	}
}
