package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLabel_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/labels") {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"name":"bug","color":"#FF0000"},{"id":2,"name":"feature","color":"#00FF00"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	labels, err := c.Labels.List(testCtx, "42", 20)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(labels) != 2 || labels[0].Name != "bug" {
		t.Errorf("unexpected labels: %+v", labels)
	}
}

func TestLabel_Create(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":3,"name":"urgent","color":"#FF0000"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	label, err := c.Labels.Create(testCtx, "42", LabelCreateOpts{Name: "urgent", Color: "#FF0000"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if label.ID != 3 || label.Name != "urgent" {
		t.Errorf("unexpected label: %+v", label)
	}
}

func TestLabel_Create_NamedColor(t *testing.T) {
	var gotColor string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":4,"name":"x","color":"#FF0000"}`))
	}))
	defer srv.Close()
	_ = gotColor

	c := newTestClient(srv.URL)
	// "red" should be normalized to "#FF0000"
	normalized := normalizeColor("red")
	if normalized != "#FF0000" {
		t.Errorf("normalizeColor(red) = %q, want #FF0000", normalized)
	}
	_, _ = c.Labels.Create(testCtx, "42", LabelCreateOpts{Name: "x", Color: "red"})
}

func TestLabel_Update(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"name":"bug-renamed","color":"#FF0000"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	label, err := c.Labels.Update(testCtx, "42", 1, LabelUpdateOpts{NewName: "bug-renamed"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if label.Name != "bug-renamed" {
		t.Errorf("name = %q, want bug-renamed", label.Name)
	}
}

func TestLabel_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if err := c.Labels.Delete(testCtx, "42", 1); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestNormalizeColor_Branches(t *testing.T) {
	if got := normalizeColor("#AABBCC"); got != "#AABBCC" {
		t.Errorf("hex passthrough = %q", got)
	}
	if got := normalizeColor("unknown"); got != "unknown" {
		t.Errorf("unknown = %q", got)
	}
	if got := normalizeColor("grey"); got != "#808080" {
		t.Errorf("named grey = %q", got)
	}
	if got := normalizeColor("green"); got != "#00FF00" {
		t.Errorf("named green = %q", got)
	}
}

func TestLabel_List_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Labels.List(testCtx, "42", 20)
	if err == nil || !strings.Contains(err.Error(), "parsing labels") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestLabel_Create_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Labels.Create(testCtx, "42", LabelCreateOpts{Name: "x", Color: "#000"})
	if err == nil || !strings.Contains(err.Error(), "parsing created label") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestLabel_Update_WithColor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"name":"bug","color":"#00FF00"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	label, err := c.Labels.Update(testCtx, "42", 1, LabelUpdateOpts{Color: "green"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if label.Color != "#00FF00" {
		t.Errorf("color = %q", label.Color)
	}
}

func TestLabel_Update_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Labels.Update(testCtx, "42", 1, LabelUpdateOpts{NewName: "x"})
	if err == nil || !strings.Contains(err.Error(), "parsing updated label") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
