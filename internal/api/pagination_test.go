package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestPaginateGET_SinglePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("per_page") != "20" {
			t.Errorf("per_page = %q, want 20", r.URL.Query().Get("per_page"))
		}
		w.Header().Set("X-Next-Page", "0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1},{"id":2}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	data, _, err := PaginateGET(testCtx, c, c.APIPath("/projects"), 0)
	if err != nil {
		t.Fatalf("PaginateGET: %v", err)
	}
	if string(data) != `[{"id":1},{"id":2}]` {
		t.Errorf("body = %s", string(data))
	}
}

func TestPaginateGET_MultiPage(t *testing.T) {
	var page int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ = strconv.Atoi(r.URL.Query().Get("page"))
		if page == 0 {
			page = 1
		}
		if r.URL.Query().Get("per_page") != "100" {
			t.Errorf("per_page = %q, want 100", r.URL.Query().Get("per_page"))
		}
		switch page {
		case 1:
			w.Header().Set("X-Next-Page", "2")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":1}]`))
		case 2:
			w.Header().Set("X-Next-Page", "0")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":2}]`))
		default:
			t.Fatalf("unexpected page %d", page)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	data, _, err := PaginateGET(context.Background(), c, c.APIPath("/projects"), 150)
	if err != nil {
		t.Fatalf("PaginateGET: %v", err)
	}
	want := `[{"id":1},{"id":2}]`
	if string(data) != want {
		t.Errorf("body = %s, want %s", string(data), want)
	}
}

func TestRateLimitWait_RateLimitReset(t *testing.T) {
	reset := strconv.FormatInt(1700000000, 10)
	h := http.Header{}
	h.Set("RateLimit-Reset", reset)
	wait := rateLimitWait(h)
	if wait <= 0 {
		t.Errorf("expected positive wait from RateLimit-Reset, got %v", wait)
	}
}
