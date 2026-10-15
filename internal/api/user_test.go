package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUser_SearchPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api/v4/users" || q.Get("search") != "carol" || q.Get("active") != "true" ||
			q.Get("page") != "3" || q.Get("per_page") != "10" || q.Has("order_by") {
			t.Errorf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("X-Next-Page", "4")
		_, _ = w.Write([]byte(`[{"id":7,"username":"carol","name":"Carol","state":"active"}]`))
	}))
	defer srv.Close()
	users, pag, err := newTestClient(srv.URL).Users.SearchPage(testCtx, " carol ", true, 3, 10)
	if err != nil || len(users) != 1 || users[0].Username != "carol" || pag.NextPage != 4 {
		t.Fatalf("SearchPage = %+v, %+v, %v", users, pag, err)
	}
}

func TestUser_SearchPageWithoutActiveAndBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("active") {
			t.Errorf("active sent without being asked: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()
	if _, _, err := newTestClient(srv.URL).Users.SearchPage(testCtx, "x", false, 1, 20); err == nil || !strings.Contains(err.Error(), "parsing users") {
		t.Fatalf("expected parse error, got %v", err)
	}
}
