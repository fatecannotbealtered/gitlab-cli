package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRelease_ListPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.EscapedPath(), "/projects/acme%2Fweb/releases") {
			t.Errorf("path = %q", r.URL.EscapedPath())
		}
		if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("per_page") != "5" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		w.Header().Set("X-Next-Page", "3")
		w.Header().Set("X-Total", "12")
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0","name":"Release 1.0","author":{"username":"carol"},
			"milestones":[{"id":7,"iid":1,"title":"v1.0"}],"_links":{"self":"https://h/acme/web/-/releases/v1.0"}}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	releases, pag, err := c.Releases.ListPage(testCtx, "acme/web", 2, 5)
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	if len(releases) != 1 || releases[0].TagName != "v1.0" || releases[0].Author.Username != "carol" ||
		releases[0].Milestones[0].Title != "v1.0" || releases[0].Links.Self == "" {
		t.Errorf("unexpected releases: %+v", releases)
	}
	if pag.NextPage != 3 || !pag.HasTotal || pag.Total != 12 {
		t.Errorf("pagination: %+v", pag)
	}
}

func TestRelease_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1.0") {
			t.Errorf("path = %q, want suffix /v1.0", r.URL.Path)
		}
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
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"tag_name":"v2.0","name":"Release 2.0","description":"Second"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	r, err := c.Releases.Create(testCtx, "1", ReleaseCreateBody{TagName: "v2.0", Name: "Release 2.0", Description: "Second",
		Ref: "main", Milestones: []string{"v2.0"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if r.TagName != "v2.0" || body["ref"] != "main" || body["milestones"].([]any)[0] != "v2.0" {
		t.Errorf("tag_name = %q, body %v", r.TagName, body)
	}
}

// TestRelease_UpdateSendsOnlyTheFieldsGiven: nil fields are left out, an empty
// description and an empty milestone list are sent, so they clear (CLI-029).
func TestRelease_UpdateSendsOnlyTheFieldsGiven(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.HasSuffix(r.URL.Path, "/v1.0") {
			t.Errorf("request = %s %q", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		body = map[string]any{}
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"tag_name":"v1.0","name":"Updated Release","description":""}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	name := "Updated Release"
	r, err := c.Releases.Update(testCtx, "1", "v1.0", ReleaseUpdateBody{Name: &name})
	if err != nil || r.Name != "Updated Release" {
		t.Fatalf("Update: %v %+v", err, r)
	}
	if _, has := body["description"]; has || body["name"] != "Updated Release" || body["milestones"] != nil {
		t.Errorf("only the name is sent: %v", body)
	}
	empty, none := "", []string{}
	if _, err := c.Releases.Update(testCtx, "1", "v1.0", ReleaseUpdateBody{Description: &empty, Milestones: &none}); err != nil {
		t.Fatal(err)
	}
	if d, has := body["description"]; !has || d != "" {
		t.Errorf("an empty description clears it: %v", body)
	}
	if m, ok := body["milestones"].([]any); !ok || len(m) != 0 {
		t.Errorf("an empty milestone list clears them: %v", body)
	}
}

func TestRelease_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || !strings.HasSuffix(r.URL.Path, "/v1.0") {
			t.Errorf("request = %s %q", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.0"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if err := c.Releases.Delete(testCtx, "1", "v1.0"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestRelease_TagName_URLEncoded(t *testing.T) {
	var gotURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.RequestURI
		_, _ = w.Write([]byte(`{"tag_name":"release/1.0","name":"One"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, err := c.Releases.Get(testCtx, "1", "release/1.0"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(gotURI, "/releases/release%2F1.0") {
		t.Errorf("RequestURI %q should escape the slash in the tag", gotURI)
	}
}

func TestRelease_ParseErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	name := "x"
	for what, call := range map[string]func() error{
		"ListPage": func() error { _, _, err := c.Releases.ListPage(testCtx, "1", 1, 10); return err },
		"Get":      func() error { _, err := c.Releases.Get(testCtx, "1", "v1.0"); return err },
		"Create": func() error {
			_, err := c.Releases.Create(testCtx, "1", ReleaseCreateBody{TagName: "v1", Ref: "main"})
			return err
		},
		"Update": func() error {
			_, err := c.Releases.Update(testCtx, "1", "v1.0", ReleaseUpdateBody{Name: &name})
			return err
		},
	} {
		if err := call(); err == nil || !strings.Contains(err.Error(), "parsing release") {
			t.Errorf("%s: expected a parse error, got %v", what, err)
		}
	}
}
