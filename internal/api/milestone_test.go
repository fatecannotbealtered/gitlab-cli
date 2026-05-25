package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMilestone_List(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/milestones") {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("state") != "active" {
			t.Errorf("state = %q, want active", r.URL.Query().Get("state"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"iid":1,"title":"v1.0","state":"active"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	ms, err := c.Milestones.List(testCtx, "42", &MilestoneListOpts{State: "active", Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ms) != 1 || ms[0].Title != "v1.0" {
		t.Errorf("unexpected milestones: %+v", ms)
	}
}

func TestMilestone_GetByID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/milestones/5" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":5,"iid":2,"title":"v2.0","state":"active"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	m, err := c.Milestones.GetByID(testCtx, "42", 5)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if m.ID != 5 || m.Title != "v2.0" {
		t.Errorf("unexpected milestone: %+v", m)
	}
}

func TestMilestone_Create(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":6,"iid":3,"title":"v3.0","state":"active"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	m, err := c.Milestones.Create(testCtx, "42", MilestoneCreateOpts{Title: "v3.0"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.Title != "v3.0" {
		t.Errorf("title = %q, want v3.0", m.Title)
	}
}

func TestMilestone_Update(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":6,"iid":3,"title":"v3.0","state":"closed"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	m, err := c.Milestones.Update(testCtx, "42", 6, MilestoneUpdateOpts{StateEvent: "close"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if m.State != "closed" {
		t.Errorf("state = %q, want closed", m.State)
	}
}

func TestMilestone_Delete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if err := c.Milestones.Delete(testCtx, "42", 6); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestMilestoneListOpts_AllState_OmitsStateParam(t *testing.T) {
	opts := &MilestoneListOpts{State: "all", Limit: 10}
	q := opts.encode()
	if strings.Contains(q, "state=") {
		t.Errorf("state=all should not be sent, got: %s", q)
	}
}

func TestMilestone_List_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Milestones.List(testCtx, "42", nil)
	if err == nil || !strings.Contains(err.Error(), "parsing milestones") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestMilestone_GetByID_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Milestones.GetByID(testCtx, "42", 1)
	if err == nil || !strings.Contains(err.Error(), "parsing milestone") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestMilestone_Create_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Milestones.Create(testCtx, "42", MilestoneCreateOpts{Title: "v1"})
	if err == nil || !strings.Contains(err.Error(), "parsing created milestone") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestMilestone_Update_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Milestones.Update(testCtx, "42", 1, MilestoneUpdateOpts{Title: "x"})
	if err == nil || !strings.Contains(err.Error(), "parsing updated milestone") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestMilestoneListOpts_NilEncode(t *testing.T) {
	if (*MilestoneListOpts)(nil).encode() != "" {
		t.Error("nil encode should be empty")
	}
}
