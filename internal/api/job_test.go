package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJob_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/jobs/99" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":99,"name":"build","status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	j, err := c.Jobs.Get(testCtx, "42", 99)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if j.ID != 99 || j.Name != "build" {
		t.Errorf("unexpected job: %+v", j)
	}
}

func TestJob_ArtifactsTo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/jobs/99/artifacts" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("artifact-bytes"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	var buf bytes.Buffer
	if err := c.Jobs.ArtifactsTo(testCtx, "42", 99, &buf); err != nil {
		t.Fatalf("ArtifactsTo: %v", err)
	}
	if buf.String() != "artifact-bytes" {
		t.Errorf("got %q, want artifact-bytes", buf.String())
	}
}
