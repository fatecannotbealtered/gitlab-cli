package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

func TestListPerPage(t *testing.T) {
	if got := listPerPage(0); got != 20 {
		t.Errorf("listPerPage(0) = %d, want 20", got)
	}
	if got := listPerPage(200); got != maxPerPage {
		t.Errorf("listPerPage(200) = %d, want %d", got, maxPerPage)
	}
}

func TestPaginateGET_InvalidQuery(t *testing.T) {
	c := newTestClient("http://example.com")
	_, _, err := PaginateGET(testCtx, c, c.APIPath("/projects?%zz"), 10)
	if err == nil {
		t.Fatal("expected query parse error")
	}
}

func TestPaginateGET_SinglePageLimitAtMax(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("per_page") != "100" {
			t.Errorf("per_page = %q, want 100", r.URL.Query().Get("per_page"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	data, _, err := PaginateGET(testCtx, c, c.APIPath("/projects"), 100)
	if err != nil {
		t.Fatalf("PaginateGET: %v", err)
	}
	if string(data) != `[{"id":1}]` {
		t.Errorf("body = %s", string(data))
	}
}

func TestPaginateGET_MultiPageUnmarshalError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Next-Page", "2")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, _, err := PaginateGET(testCtx, c, c.APIPath("/projects"), 150)
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
}

func TestPaginateGET_MultiPageRequestError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		if page == "" || page == "1" {
			w.Header().Set("X-Next-Page", "2")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":1}]`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, _, err := PaginateGET(testCtx, c, c.APIPath("/projects"), 150)
	if err == nil {
		t.Fatal("expected error on second page")
	}
}

func TestPaginateGET_TruncatesToLimit(t *testing.T) {
	var page int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ = strconv.Atoi(r.URL.Query().Get("page"))
		if page == 0 {
			page = 1
		}
		switch page {
		case 1:
			w.Header().Set("X-Next-Page", "2")
			w.WriteHeader(http.StatusOK)
			items := make([]string, 100)
			for i := range items {
				items[i] = fmt.Sprintf(`{"id":%d}`, i+1)
			}
			_, _ = w.Write([]byte("[" + strings.Join(items, ",") + "]"))
		case 2:
			w.Header().Set("X-Next-Page", "0")
			w.WriteHeader(http.StatusOK)
			items := make([]string, 100)
			for i := range items {
				items[i] = fmt.Sprintf(`{"id":%d}`, i+101)
			}
			_, _ = w.Write([]byte("[" + strings.Join(items, ",") + "]"))
		default:
			t.Fatalf("unexpected page %d", page)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	data, _, err := PaginateGET(testCtx, c, c.APIPath("/projects"), 150)
	if err != nil {
		t.Fatalf("PaginateGET: %v", err)
	}
	var items []map[string]int
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 150 {
		t.Errorf("len = %d, want 150", len(items))
	}
}

func TestSplitPathAndQuery(t *testing.T) {
	base, q, err := splitPathAndQuery("/projects?owned=true")
	if err != nil {
		t.Fatalf("splitPathAndQuery: %v", err)
	}
	if base != "/projects" || q.Get("owned") != "true" {
		t.Errorf("base=%q q=%v", base, q)
	}
	base, q, err = splitPathAndQuery("/projects")
	if err != nil || base != "/projects" || len(q) != 0 {
		t.Errorf("no query: base=%q q=%v err=%v", base, q, err)
	}
}
