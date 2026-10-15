package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMilestone_ListPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.EscapedPath() != "/api/v4/projects/acme%2Fweb/milestones" {
			t.Errorf("path = %q", r.URL.EscapedPath())
		}
		if q.Get("state") != "active" || q.Get("search") != "rel" || q.Get("page") != "1" || q.Get("per_page") != "20" ||
			q["iids[]"][0] != "3" || q.Get("title") != "v1.0" {
			t.Errorf("query = %v", q)
		}
		_, _ = w.Write([]byte(`[{"id":11,"iid":3,"title":"v1.0","state":"active","expired":false}]`))
	}))
	defer srv.Close()
	ms, pag, err := newTestClient(srv.URL).Milestones.ListPage(testCtx, "acme/web",
		MilestoneListOpts{State: "active", Search: "rel", Title: "v1.0", IIDs: []int{3}, Page: 1, PerPage: 20})
	if err != nil || len(ms) != 1 || ms[0].IID != 3 || ms[0].Expired == nil || pag.NextPage != 0 {
		t.Fatalf("ListPage = %+v, %+v, %v", ms, pag, err)
	}
}

func TestMilestone_ListAllStateOmitsTheFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("state") {
			t.Errorf("state sent for all: %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`[{"id":1,"iid":1,"title":"v1"}]`))
	}))
	defer srv.Close()
	ms, err := newTestClient(srv.URL).Milestones.List(testCtx, "42", &MilestoneListOpts{State: "all", Limit: 5})
	if err != nil || len(ms) != 1 {
		t.Fatalf("List = %+v, %v", ms, err)
	}
	if _, err := newTestClient(srv.URL).Milestones.List(testCtx, "42", nil); err != nil {
		t.Fatalf("List(nil) = %v", err)
	}
}

func TestMilestone_GetCreateUpdateDelete(t *testing.T) {
	var methods []string
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.EscapedPath())
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		bodies = append(bodies, m)
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(`{"id":11,"iid":3,"title":"v1.0","state":"closed"}`))
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	if m, err := c.Milestones.GetByID(testCtx, "42", 11); err != nil || m.IID != 3 {
		t.Fatalf("GetByID = %+v, %v", m, err)
	}
	if _, err := c.Milestones.Create(testCtx, "42", MilestoneCreateOpts{Title: "v1.0", DueDate: "2026-11-01"}); err != nil {
		t.Fatal(err)
	}
	close := "close"
	if _, err := c.Milestones.Update(testCtx, "42", 11, MilestoneUpdateOpts{StateEvent: &close}); err != nil {
		t.Fatal(err)
	}
	if err := c.Milestones.Delete(testCtx, "42", 11); err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /api/v4/projects/42/milestones/11", "POST /api/v4/projects/42/milestones",
		"PUT /api/v4/projects/42/milestones/11", "DELETE /api/v4/projects/42/milestones/11"}
	for i, w := range want {
		if methods[i] != w {
			t.Errorf("request %d = %q, want %q", i, methods[i], w)
		}
	}
	if bodies[1]["due_date"] != "2026-11-01" || len(bodies[2]) != 1 || bodies[2]["state_event"] != "close" {
		t.Errorf("bodies = %v", bodies)
	}
}

func TestMilestone_ParseErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	if _, _, err := c.Milestones.ListPage(testCtx, "42", MilestoneListOpts{}); err == nil {
		t.Error("ListPage")
	}
	if _, err := c.Milestones.List(testCtx, "42", nil); err == nil {
		t.Error("List")
	}
	if _, err := c.Milestones.GetByID(testCtx, "42", 1); err == nil {
		t.Error("GetByID")
	}
	if _, err := c.Milestones.Create(testCtx, "42", MilestoneCreateOpts{Title: "v1"}); err == nil {
		t.Error("Create")
	}
	if _, err := c.Milestones.Update(testCtx, "42", 1, MilestoneUpdateOpts{}); err == nil {
		t.Error("Update")
	}
}
