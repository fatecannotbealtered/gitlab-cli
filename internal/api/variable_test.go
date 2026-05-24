package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
)

func TestVariable_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/variables") {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"key":"FOO","value":"bar","variable_type":"env_var","protected":false,"masked":true,"environment_scope":"*"}]`))
	}))
	defer srv.Close()

	c := NewClient(&config.Config{Host: srv.URL, Token: "tok"})
	vars, err := c.Variables.List(testCtx, "group/proj", 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(vars) != 1 || vars[0].Key != "FOO" {
		t.Errorf("unexpected: %+v", vars)
	}
	if !vars[0].Masked {
		t.Error("expected masked=true")
	}
}

func TestVariable_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/variables/FOO") {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"key":"FOO","value":"secret","variable_type":"env_var","environment_scope":"*"}`))
	}))
	defer srv.Close()

	c := NewClient(&config.Config{Host: srv.URL, Token: "tok"})
	v, err := c.Variables.Get(testCtx, "group/proj", "FOO", "")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v.Key != "FOO" || v.Value != "secret" {
		t.Errorf("unexpected: %+v", v)
	}
}

func TestVariable_Get_WithEnvScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "filter") {
			t.Errorf("expected filter query param, got %q", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"key":"FOO","value":"prod-secret","environment_scope":"production"}`))
	}))
	defer srv.Close()

	c := NewClient(&config.Config{Host: srv.URL, Token: "tok"})
	v, err := c.Variables.Get(testCtx, "group/proj", "FOO", "production")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v.EnvironmentScope != "production" {
		t.Errorf("unexpected scope: %q", v.EnvironmentScope)
	}
}

func TestVariable_Create(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"key":"NEW_VAR","value":"newval","variable_type":"env_var","protected":true,"masked":false,"environment_scope":"*"}`))
	}))
	defer srv.Close()

	c := NewClient(&config.Config{Host: srv.URL, Token: "tok"})
	v, err := c.Variables.Create(testCtx, "group/proj", &VariableCreateOpts{
		Key:   "NEW_VAR",
		Value: "newval",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if v.Key != "NEW_VAR" {
		t.Errorf("unexpected key: %q", v.Key)
	}
}

func TestVariable_Update(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"key":"FOO","value":"updated","variable_type":"env_var","environment_scope":"*"}`))
	}))
	defer srv.Close()

	c := NewClient(&config.Config{Host: srv.URL, Token: "tok"})
	v, err := c.Variables.Update(testCtx, "group/proj", "FOO", "", &VariableUpdateOpts{Value: "updated"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if v.Value != "updated" {
		t.Errorf("unexpected value: %q", v.Value)
	}
}

func TestVariable_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/variables/FOO") {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(&config.Config{Host: srv.URL, Token: "tok"})
	if err := c.Variables.Delete(testCtx, "group/proj", "FOO", ""); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
