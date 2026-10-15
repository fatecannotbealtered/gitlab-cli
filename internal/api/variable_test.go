package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVariable_ListPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/variables") {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("page") != "2" || r.URL.Query().Get("per_page") != "10" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[{"key":"FOO","value":"bar","variable_type":"env_var","masked":true,"environment_scope":"*"},
			{"key":"HIDDEN","value":null,"hidden":true,"masked":true,"environment_scope":"production"}]`))
	}))
	defer srv.Close()

	vars, _, err := newTestClient(srv.URL).Variables.ListPage(testCtx, "group/proj", 2, 10)
	if err != nil {
		t.Fatalf("ListPage: %v", err)
	}
	if len(vars) != 2 || vars[0].Key != "FOO" || !vars[0].Masked || *vars[0].Value != "bar" || vars[1].Value != nil || !vars[1].Hidden {
		t.Errorf("unexpected: %+v", vars)
	}
}

// TestVariable_FilterParameter: GitLab's filter is filter[environment_scope] (1.x sent
// filter[env_scope], which GitLab ignores); no scope sends no filter.
func TestVariable_FilterParameter(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/variables/FOO") {
			t.Errorf("path = %q", r.URL.Path)
		}
		queries = append(queries, r.URL.Query().Get("filter[environment_scope]")+"|"+r.URL.RawQuery)
		switch r.Method {
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = w.Write([]byte(`{"key":"FOO","value":"v","environment_scope":"review/*"}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if v, err := c.Variables.Get(testCtx, "g/p", "FOO", "review/*"); err != nil || v.EnvironmentScope != "review/*" {
		t.Fatalf("Get: %+v %v", v, err)
	}
	if _, err := c.Variables.Get(testCtx, "g/p", "FOO", ""); err != nil {
		t.Fatal(err)
	}
	value := "x"
	if _, err := c.Variables.Update(testCtx, "g/p", "FOO", "production", &VariableUpdateOpts{Value: &value}); err != nil {
		t.Fatal(err)
	}
	if err := c.Variables.Delete(testCtx, "g/p", "FOO", "staging"); err != nil {
		t.Fatal(err)
	}
	want := []string{"review/*|filter%5Benvironment_scope%5D=review%2F%2A", "|", "production|filter%5Benvironment_scope%5D=production",
		"staging|filter%5Benvironment_scope%5D=staging"}
	for i := range want {
		if queries[i] != want[i] {
			t.Errorf("request %d: %q, want %q", i, queries[i], want[i])
		}
	}
}

func TestVariable_CreateSendsEveryFlag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q", r.Method)
		}
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		if b["protected"] != false || b["masked"] != false || b["raw"] != false || b["key"] != "NEW_VAR" {
			t.Errorf("body = %v", b)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"key":"NEW_VAR","value":"newval","variable_type":"env_var","environment_scope":"*"}`))
	}))
	defer srv.Close()

	v, err := newTestClient(srv.URL).Variables.Create(testCtx, "group/proj", &VariableCreateOpts{Key: "NEW_VAR", Value: "newval"})
	if err != nil || v.Key != "NEW_VAR" {
		t.Fatalf("Create: %+v %v", v, err)
	}
}

func TestVariable_UpdateSendsOnlyTheGivenFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		if len(b) != 1 || b["protected"] != true {
			t.Errorf("body = %v", b)
		}
		_, _ = w.Write([]byte(`{"key":"FOO","protected":true}`))
	}))
	defer srv.Close()

	on := true
	if v, err := newTestClient(srv.URL).Variables.Update(testCtx, "g/p", "FOO", "", &VariableUpdateOpts{Protected: &on}); err != nil || !v.Protected {
		t.Fatalf("Update: %+v %v", v, err)
	}
}

func TestVariable_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Variable Not Found"}`))
			return
		}
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, _, err := c.Variables.ListPage(testCtx, "g/p", 1, 20); err == nil || !strings.Contains(err.Error(), "parsing variables") {
		t.Errorf("ListPage: %v", err)
	}
	if _, err := c.Variables.Get(testCtx, "g/p", "FOO", ""); err == nil || !strings.Contains(err.Error(), "parsing variable") {
		t.Errorf("Get: %v", err)
	}
	if _, err := c.Variables.Create(testCtx, "g/p", &VariableCreateOpts{Key: "X", Value: "y"}); err == nil || !strings.Contains(err.Error(), "parsing created variable") {
		t.Errorf("Create: %v", err)
	}
	if _, err := c.Variables.Update(testCtx, "g/p", "FOO", "", &VariableUpdateOpts{}); err == nil || !strings.Contains(err.Error(), "parsing updated variable") {
		t.Errorf("Update: %v", err)
	}
	if err := c.Variables.Delete(testCtx, "g/p", "FOO", ""); err == nil {
		t.Error("Delete: want the 404")
	}
}
