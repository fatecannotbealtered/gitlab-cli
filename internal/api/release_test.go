package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRelease_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/releases") {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0","name":"Release 1.0"},{"tag_name":"v0.9","name":"Release 0.9"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	releases, err := c.Releases.List(testCtx, "1", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(releases) != 2 || releases[0].TagName != "v1.0" {
		t.Errorf("unexpected releases: %+v", releases)
	}
}

func TestRelease_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1.0") {
			t.Errorf("path = %q, want suffix /v1.0", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v1.0","name":"Release 1.0","description":"First release"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	r, err := c.Releases.Get(testCtx, "1", "v1.0")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if r.TagName != "v1.0" || r.Description != "First release" {
		t.Errorf("unexpected release: %+v", r)
	}
}

func TestRelease_Create(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"tag_name":"v2.0","name":"Release 2.0","description":"Second"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	r, err := c.Releases.Create(testCtx, "1", ReleaseCreateBody{
		TagName:     "v2.0",
		Name:        "Release 2.0",
		Description: "Second",
		Ref:         "main",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if r.TagName != "v2.0" {
		t.Errorf("tag_name = %q, want v2.0", r.TagName)
	}
}

func TestRelease_Update(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1.0") {
			t.Errorf("path = %q, want suffix /v1.0", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v1.0","name":"Updated Release","description":"updated"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	r, err := c.Releases.Update(testCtx, "1", "v1.0", ReleaseUpdateBody{Name: "Updated Release"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if r.Name != "Updated Release" {
		t.Errorf("name = %q, want 'Updated Release'", r.Name)
	}
}

func TestRelease_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/v1.0") {
			t.Errorf("path = %q, want suffix /v1.0", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v1.0"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if err := c.Releases.Delete(testCtx, "1", "v1.0"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestRelease_TagName_URLEncoded(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tag_name":"v1.0+beta","name":"Beta"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Releases.Get(testCtx, "1", "v1.0+beta")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(gotPath, "v1.0") {
		t.Errorf("path %q should contain encoded tag", gotPath)
	}
}

func TestRelease_List_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Releases.List(testCtx, "1", 10)
	if err == nil || !strings.Contains(err.Error(), "parsing releases") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestRelease_Get_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Releases.Get(testCtx, "1", "v1.0")
	if err == nil || !strings.Contains(err.Error(), "parsing release") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestRelease_Create_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Releases.Create(testCtx, "1", ReleaseCreateBody{TagName: "v1", Ref: "main"})
	if err == nil || !strings.Contains(err.Error(), "parsing release") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestRelease_Update_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Releases.Update(testCtx, "1", "v1.0", ReleaseUpdateBody{Name: "x"})
	if err == nil || !strings.Contains(err.Error(), "parsing release") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
