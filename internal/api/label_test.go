package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLabel_ListPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.EscapedPath() != "/api/v4/projects/acme%2Fweb/labels" {
			t.Errorf("path = %q", r.URL.EscapedPath())
		}
		if q.Get("search") != "bu" || q.Get("with_counts") != "true" || q.Get("include_ancestor_groups") != "true" ||
			q.Get("page") != "2" || q.Get("per_page") != "5" {
			t.Errorf("query = %v", q)
		}
		w.Header().Set("X-Next-Page", "3")
		w.Header().Set("X-Total", "12")
		_, _ = w.Write([]byte(`[{"id":1,"name":"bug","color":"#FF0000","is_project_label":true,"open_issues_count":4}]`))
	}))
	defer srv.Close()
	labels, pag, err := newTestClient(srv.URL).Labels.ListPage(testCtx, "acme/web",
		LabelListOpts{Search: "bu", WithCounts: true, Page: 2, PerPage: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 1 || labels[0].Name != "bug" || !labels[0].IsProjectLabel || *labels[0].OpenIssuesCount != 4 {
		t.Errorf("labels = %+v", labels)
	}
	if pag.NextPage != 3 || !pag.HasTotal || pag.Total != 12 {
		t.Errorf("pagination = %+v", pag)
	}
}

func TestLabel_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"id":1,"name":"bug"},{"id":2,"name":"feature"}]`))
	}))
	defer srv.Close()
	labels, err := newTestClient(srv.URL).Labels.List(testCtx, "42", 20)
	if err != nil || len(labels) != 2 {
		t.Fatalf("List = %+v, %v", labels, err)
	}
}

func TestLabel_GetByNameEscapesIt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/api/v4/projects/acme%2Fweb/labels/priority::high%2Furgent" &&
			r.URL.EscapedPath() != "/api/v4/projects/acme%2Fweb/labels/priority:%3Ahigh%2Furgent" {
			t.Errorf("path = %q", r.URL.EscapedPath())
		}
		_, _ = w.Write([]byte(`{"id":9,"name":"priority::high/urgent"}`))
	}))
	defer srv.Close()
	l, err := newTestClient(srv.URL).Labels.Get(testCtx, "acme/web", "priority::high/urgent")
	if err != nil || l.ID != 9 {
		t.Fatalf("Get = %+v, %v", l, err)
	}
}

func TestLabel_CreateNormalizesNamedColors(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":3,"name":"urgent","color":"#FF0000"}`))
	}))
	defer srv.Close()
	p := 2
	l, err := newTestClient(srv.URL).Labels.Create(testCtx, "42", LabelCreateOpts{Name: "urgent", Color: "Red", Priority: &p})
	if err != nil || l.ID != 3 {
		t.Fatalf("Create = %+v, %v", l, err)
	}
	if body["color"] != "#FF0000" || body["priority"] != float64(2) {
		t.Errorf("body = %v", body)
	}
}

func TestLabel_UpdateSendsOnlyTheGivenFields(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.EscapedPath() != "/api/v4/projects/42/labels/bug" {
			t.Errorf("%s %s", r.Method, r.URL.EscapedPath())
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = w.Write([]byte(`{"id":1,"name":"defect","color":"#00FF00"}`))
	}))
	defer srv.Close()
	name, color, empty := "defect", "green", ""
	l, err := newTestClient(srv.URL).Labels.Update(testCtx, "42", "bug", LabelUpdateOpts{NewName: &name, Color: &color, Description: &empty})
	if err != nil || l.Name != "defect" {
		t.Fatalf("Update = %+v, %v", l, err)
	}
	if len(body) != 3 || body["new_name"] != "defect" || body["color"] != "#00FF00" || body["description"] != "" {
		t.Errorf("body = %v", body)
	}
}

func TestLabel_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.EscapedPath() != "/api/v4/projects/42/labels/feature%20request" {
			t.Errorf("%s %s", r.Method, r.URL.EscapedPath())
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	if err := newTestClient(srv.URL).Labels.Delete(testCtx, "42", "feature request"); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeColor(t *testing.T) {
	cases := map[string]string{"#abc": "#abc", "red": "#FF0000", " Grey ": "#808080", "teal": "teal", "": ""} //nolint:gocritic // the spaces are the input under test
	for in, want := range cases {
		if got := NormalizeColor(in); got != want {
			t.Errorf("NormalizeColor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLabel_ParseErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	if _, _, err := c.Labels.ListPage(testCtx, "42", LabelListOpts{}); err == nil {
		t.Error("ListPage")
	}
	if _, err := c.Labels.List(testCtx, "42", 20); err == nil {
		t.Error("List")
	}
	if _, err := c.Labels.Get(testCtx, "42", "x"); err == nil {
		t.Error("Get")
	}
	if _, err := c.Labels.Create(testCtx, "42", LabelCreateOpts{Name: "x", Color: "#000"}); err == nil {
		t.Error("Create")
	}
	if _, err := c.Labels.Update(testCtx, "42", "x", LabelUpdateOpts{}); err == nil {
		t.Error("Update")
	}
}
