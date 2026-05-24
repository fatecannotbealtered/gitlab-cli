package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
)

var testCtx = context.Background()

// newTestClient returns a Client pointed at the given test server URL.
func newTestClient(serverURL string) *Client {
	return NewClient(&config.Config{Host: serverURL, Token: "test-token"})
}

func TestEncodeProjectPath(t *testing.T) {
	cases := map[string]string{
		"123":                   "123",
		"group/project":         "group%2Fproject",
		"group/sub/project":     "group%2Fsub%2Fproject",
		"group / spaced / proj": "group%20%2F%20spaced%20%2F%20proj",
		"":                      "",
	}
	for in, want := range cases {
		got := EncodeProjectPath(in)
		if got != want {
			t.Errorf("EncodeProjectPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewClient_StripsTrailingSlash(t *testing.T) {
	c := NewClient(&config.Config{Host: "https://gitlab.example.com/", Token: "tok"})
	if c.host != "https://gitlab.example.com" {
		t.Errorf("host = %q, want without trailing slash", c.host)
	}
	if c.authHeader != "Bearer tok" {
		t.Errorf("authHeader = %q, want 'Bearer tok'", c.authHeader)
	}
}

func TestAPIPath(t *testing.T) {
	c := NewClient(&config.Config{Host: "https://gitlab.example.com", Token: "tok"})
	cases := map[string]string{
		"/user":          "/api/v4/user",
		"user":           "/api/v4/user",
		"/projects/1/mr": "/api/v4/projects/1/mr",
	}
	for in, want := range cases {
		if got := c.APIPath(in); got != want {
			t.Errorf("APIPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGet_SendsBearerToken(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, err := c.Get(testCtx, c.APIPath("/user")); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "Bearer test-token" {
		t.Errorf("Authorization header = %q, want 'Bearer test-token'", got)
	}
}

func TestGet_NotFound_ReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Get(testCtx, c.APIPath("/projects/missing"))
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", apiErr.StatusCode)
	}
	if len(apiErr.ErrorMessages) == 0 || !strings.Contains(apiErr.ErrorMessages[0], "404") {
		t.Errorf("expected message containing '404', got %v", apiErr.ErrorMessages)
	}
}

func TestParseError_ValidationMap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":{"name":["has already been taken"],"path":["is invalid"]}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Post(testCtx, c.APIPath("/projects"), map[string]string{"name": "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
	if len(apiErr.Errors) != 2 {
		t.Errorf("Errors = %v, want 2 entries", apiErr.Errors)
	}
	if apiErr.Errors["name"] != "has already been taken" {
		t.Errorf("Errors[name] = %q", apiErr.Errors["name"])
	}
}

func TestParseError_OAuthStyle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"token expired"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Get(testCtx, c.APIPath("/user"))
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	// The OAuth-style description should appear in ErrorMessages.
	joined := strings.Join(apiErr.ErrorMessages, "|")
	if !strings.Contains(joined, "invalid_grant") || !strings.Contains(joined, "token expired") {
		t.Errorf("expected OAuth-style message, got %v", apiErr.ErrorMessages)
	}
}

func TestRetry_500_ExponentialBackoff(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"500"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	data, err := c.Get(testCtx, c.APIPath("/user"))
	if err != nil {
		t.Fatalf("Get after retries: %v", err)
	}
	if !strings.Contains(string(data), "ok") {
		t.Errorf("body = %s, want contains 'ok'", string(data))
	}
	if attempts.Load() != 3 {
		t.Errorf("attempts = %d, want 3", attempts.Load())
	}
}

func TestRetry_429_RespectsRetryAfter(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"429"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, err := c.Get(testCtx, c.APIPath("/user")); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if attempts.Load() != 2 {
		t.Errorf("attempts = %d, want 2", attempts.Load())
	}
}

func TestGetWithPagination_ParsesHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Total", "42")
		w.Header().Set("X-Total-Pages", "3")
		w.Header().Set("X-Page", "2")
		w.Header().Set("X-Per-Page", "20")
		w.Header().Set("X-Next-Page", "3")
		w.Header().Set("X-Prev-Page", "1")
		w.Header().Set("Link", `<https://example.com/?page=3>; rel="next"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, p, err := c.GetWithPagination(testCtx, c.APIPath("/projects?page=2"))
	if err != nil {
		t.Fatalf("GetWithPagination: %v", err)
	}
	if p.Total != 42 || p.TotalPages != 3 || p.Page != 2 || p.PerPage != 20 || p.NextPage != 3 || p.PrevPage != 1 {
		t.Errorf("unexpected pagination: %+v", p)
	}
	if !strings.Contains(p.Link, `rel="next"`) {
		t.Errorf("Link header missing: %q", p.Link)
	}
}

func TestPostPutDelete_Verbs(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method)
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "hello") {
				t.Errorf("body missing 'hello': %s", string(body))
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, err := c.Post(testCtx, c.APIPath("/x"), map[string]string{"key": "hello"}); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if _, err := c.Put(testCtx, c.APIPath("/x"), map[string]string{"key": "hello"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := c.Delete(testCtx, c.APIPath("/x")); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	want := []string{"POST", "PUT", "DELETE"}
	if fmt.Sprintf("%v", seen) != fmt.Sprintf("%v", want) {
		t.Errorf("seen = %v, want %v", seen, want)
	}
}

func TestUser_Me(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/user" {
			t.Errorf("path = %q, want /api/v4/user", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"username":"alice","name":"Alice","email":"a@x","state":"active","web_url":"https://x"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	u, err := c.Users.Me(testCtx)
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if u.Username != "alice" || u.ID != 1 || u.State != "active" {
		t.Errorf("unexpected user: %+v", u)
	}
}

func TestUser_Search(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("search")
		if q != "alice" {
			t.Errorf("search = %q, want alice", q)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"username":"alice"},{"id":2,"username":"alicia"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	users, err := c.Users.Search(testCtx, "alice", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(users) != 2 || users[0].Username != "alice" {
		t.Errorf("unexpected users: %+v", users)
	}
}

func TestUser_GetByUsername_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	u, err := c.Users.GetByUsername(testCtx, "ghost")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if u != nil {
		t.Errorf("expected nil, got %+v", u)
	}
}
