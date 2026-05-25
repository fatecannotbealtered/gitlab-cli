package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (errReader) Close() error             { return nil }

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

type failWriter = failingWriter

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

func TestUser_Me_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Users.Me(testCtx)
	if err == nil || !strings.Contains(err.Error(), "parsing user") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestUser_Search_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Users.Search(testCtx, "alice", 10)
	if err == nil || !strings.Contains(err.Error(), "parsing users") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestUser_GetByUsername_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Users.GetByUsername(testCtx, "alice")
	if err == nil || !strings.Contains(err.Error(), "parsing users") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestUser_GetByUsername_Found(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"username":"alice"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	u, err := c.Users.GetByUsername(testCtx, "alice")
	if err != nil {
		t.Fatalf("GetByUsername: %v", err)
	}
	if u == nil || u.Username != "alice" {
		t.Errorf("unexpected user: %+v", u)
	}
}

func TestDefaultUserAgent_EnvOverride(t *testing.T) {
	t.Setenv("GITLAB_CLI_USER_AGENT", "custom-agent/1.0")
	if got := defaultUserAgent(); got != "custom-agent/1.0" {
		t.Errorf("defaultUserAgent() = %q, want custom-agent/1.0", got)
	}
}

func TestHost(t *testing.T) {
	c := NewClient(&config.Config{Host: "https://gitlab.example.com", Token: "tok"})
	if c.Host() != "https://gitlab.example.com" {
		t.Errorf("Host() = %q", c.Host())
	}
}

func TestAPIError_Error(t *testing.T) {
	msgErr := &APIError{StatusCode: 400, ErrorMessages: []string{"bad request"}}
	if !strings.Contains(msgErr.Error(), "bad request") {
		t.Errorf("Error() = %q", msgErr.Error())
	}

	fieldErr := &APIError{StatusCode: 422, Errors: map[string]string{"name": "taken"}}
	if !strings.Contains(fieldErr.Error(), "name") {
		t.Errorf("Error() = %q", fieldErr.Error())
	}

	plain := &APIError{StatusCode: 500}
	if plain.Error() != "GitLab API error 500" {
		t.Errorf("Error() = %q", plain.Error())
	}
}

func TestMaxRetries_Env(t *testing.T) {
	t.Setenv("GITLAB_CLI_MAX_RETRIES", "1")
	if got := maxRetries(); got != 1 {
		t.Errorf("maxRetries() = %d, want 1", got)
	}

	t.Setenv("GITLAB_CLI_MAX_RETRIES", "not-a-number")
	if got := maxRetries(); got != defaultMaxRetries {
		t.Errorf("maxRetries() invalid env = %d, want default %d", got, defaultMaxRetries)
	}

	t.Setenv("GITLAB_CLI_MAX_RETRIES", "-1")
	if got := maxRetries(); got != defaultMaxRetries {
		t.Errorf("maxRetries() negative env = %d, want default %d", got, defaultMaxRetries)
	}
}

func TestIsRetryableNetworkErr(t *testing.T) {
	if isRetryableNetworkErr(nil) {
		t.Error("nil error should not be retryable")
	}
	cases := []string{
		"connection reset by peer",
		"connection refused",
		"i/o timeout",
		"temporary failure in name resolution",
		"no such host",
		"tls handshake timeout",
	}
	for _, msg := range cases {
		if !isRetryableNetworkErr(errors.New(msg)) {
			t.Errorf("expected retryable: %q", msg)
		}
	}
	if isRetryableNetworkErr(errors.New("something else entirely")) {
		t.Error("unexpected retryable error")
	}
}

func TestRateLimitWait_DefaultAndCap(t *testing.T) {
	wait := rateLimitWait(http.Header{})
	if wait != time.Second {
		t.Errorf("default wait = %v, want 1s", wait)
	}

	h := http.Header{}
	h.Set("Retry-After", "9999")
	if got := rateLimitWait(h); got != maxRateLimitWait {
		t.Errorf("capped wait = %v, want %v", got, maxRateLimitWait)
	}

	h2 := http.Header{}
	h2.Set("Retry-After", "not-a-number")
	if got := rateLimitWait(h2); got != time.Second {
		t.Errorf("invalid Retry-After wait = %v, want 1s", got)
	}
}

func TestWaitForRetry_NonRetryStatus(t *testing.T) {
	if err := waitForRetry(testCtx, 0, http.StatusBadRequest, nil); err != nil {
		t.Errorf("expected nil for 4xx, got %v", err)
	}
}

func TestWaitForRetry_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitForRetry(ctx, 0, http.StatusInternalServerError, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestParseError_AllBranches(t *testing.T) {
	c := NewClient(&config.Config{Host: "http://example.com", Token: "t"})

	e := c.parseError(400, []byte(`{"message":"bad request"}`))
	if len(e.ErrorMessages) != 1 || e.ErrorMessages[0] != "bad request" {
		t.Errorf("string message: %+v", e)
	}

	e = c.parseError(401, []byte(`{"error":"invalid_grant","error_description":"token expired"}`))
	if len(e.ErrorMessages) == 0 || !strings.Contains(e.ErrorMessages[0], "invalid_grant") {
		t.Errorf("oauth with desc: %+v", e)
	}

	e = c.parseError(401, []byte(`{"error":"invalid_grant"}`))
	if len(e.ErrorMessages) == 0 || e.ErrorMessages[0] != "invalid_grant" {
		t.Errorf("oauth without desc: %+v", e)
	}

	e = c.parseError(401, []byte(`not json`))
	if len(e.ErrorMessages) != 1 || !strings.Contains(e.ErrorMessages[0], "not logged in") {
		t.Errorf("401 fallback: %+v", e)
	}

	e = c.parseError(403, []byte(`{}`))
	if len(e.ErrorMessages) != 1 || !strings.Contains(e.ErrorMessages[0], "permission denied") {
		t.Errorf("403 fallback: %+v", e)
	}

	e = c.parseError(404, []byte(`{}`))
	if len(e.ErrorMessages) != 1 || !strings.Contains(e.ErrorMessages[0], "resource not found") {
		t.Errorf("404 fallback: %+v", e)
	}

	e = c.parseError(418, []byte(`{}`))
	if len(e.ErrorMessages) != 1 || !strings.Contains(e.ErrorMessages[0], "unexpected status code 418") {
		t.Errorf("unexpected status fallback: %+v", e)
	}

	e = c.parseError(400, []byte(`{"message":{"field":["a","b"]}}`))
	if e.Errors["field"] != "a; b" {
		t.Errorf("joined validation errors = %q", e.Errors["field"])
	}
}

func TestDoWithRetry_401NoRetry(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Get(testCtx, c.APIPath("/user"))
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1 (no retry on 401)", attempts.Load())
	}
}

func TestDoWithRetry_RetriesExhausted(t *testing.T) {
	t.Setenv("GITLAB_CLI_MAX_RETRIES", "1")
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Get(testCtx, c.APIPath("/user"))
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if attempts.Load() != 2 {
		t.Errorf("attempts = %d, want 2", attempts.Load())
	}
}

func TestDoWithRetry_NetworkErrorRetry(t *testing.T) {
	t.Setenv("GITLAB_CLI_MAX_RETRIES", "2")
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	base := http.DefaultTransport
	var transportCalls atomic.Int32
	c.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if transportCalls.Add(1) == 1 {
			return nil, errors.New("connection reset by peer")
		}
		return base.RoundTrip(req)
	})

	data, err := c.Get(testCtx, c.APIPath("/user"))
	if err != nil {
		t.Fatalf("Get after network retry: %v", err)
	}
	if !strings.Contains(string(data), "ok") {
		t.Errorf("body = %s", string(data))
	}
}

func TestDoWithRetry_NonRetryableNetworkError(t *testing.T) {
	c := newTestClient("http://example.com")
	c.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("certificate verify failed")
	})
	_, err := c.Get(testCtx, c.APIPath("/user"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "executing request") {
		t.Errorf("err = %v", err)
	}
}

func TestDoWithRetry_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newTestClient("http://example.com")
	_, err := c.Get(ctx, c.APIPath("/user"))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestDoWithRetry_JSONMarshalError(t *testing.T) {
	c := newTestClient("http://example.com")
	_, err := c.Post(testCtx, c.APIPath("/x"), make(chan int))
	if err == nil {
		t.Fatal("expected marshal error")
	}
	if !strings.Contains(err.Error(), "encoding request body") {
		t.Errorf("err = %v", err)
	}
}

func TestDoWithRetry_ReadBodyError(t *testing.T) {
	c := newTestClient("http://example.com")
	c.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       errReader{},
			Header:     make(http.Header),
		}, nil
	})
	_, err := c.Get(testCtx, c.APIPath("/user"))
	if err == nil {
		t.Fatal("expected read error")
	}
	if !strings.Contains(err.Error(), "reading response body") {
		t.Errorf("err = %v", err)
	}
}

func TestDoWithRetry_WaitCanceledOn429(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"slow down"}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	c := newTestClient(srv.URL)
	_, err := c.Get(ctx, c.APIPath("/user"))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1 before cancel", attempts.Load())
	}
}

func TestHTTPClient_CheckRedirect_SameHostPreservesAuth(t *testing.T) {
	var gotAuth, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/start":
			http.Redirect(w, r, "/api/v4/end", http.StatusFound)
		case "/api/v4/end":
			gotAuth = r.Header.Get("Authorization")
			gotUA = r.Header.Get("User-Agent")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, err := c.Get(testCtx, c.APIPath("/start")); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization after same-host redirect = %q", gotAuth)
	}
	if gotUA != defaultUserAgent() {
		t.Errorf("User-Agent after redirect = %q, want %q", gotUA, defaultUserAgent())
	}
}

func TestHTTPClient_CheckRedirect_CrossHostStripsAuth(t *testing.T) {
	hc := newHTTPClient(time.Second)
	prev, err := http.NewRequest(http.MethodGet, "http://origin.example/api/v4/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	prev.Header.Set("Authorization", "Bearer test-token")
	prev.Header.Set("User-Agent", "test-ua")

	req, err := http.NewRequest(http.MethodGet, "http://other.example/api/v4/target", nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := hc.CheckRedirect(req, []*http.Request{prev}); err != nil {
		t.Fatalf("CheckRedirect: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "" {
		t.Errorf("cross-host redirect should not re-apply Authorization, got %q", got)
	}
	if req.Header.Get("User-Agent") != "test-ua" {
		t.Errorf("User-Agent = %q, want test-ua", req.Header.Get("User-Agent"))
	}
}

func TestHTTPClient_CheckRedirect_TooManyRedirects(t *testing.T) {
	hc := newHTTPClient(time.Second)
	via := make([]*http.Request, 10)
	err := hc.CheckRedirect(httptest.NewRequest(http.MethodGet, "http://example.com/x", nil), via)
	if err == nil || !strings.Contains(err.Error(), "stopped after 10 redirects") {
		t.Fatalf("expected redirect limit error, got %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"x", http.StatusFound)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err = c.Get(testCtx, c.APIPath("/a"))
	if err == nil {
		t.Fatal("expected redirect limit error from client")
	}
	if !strings.Contains(err.Error(), "stopped after 10 redirects") {
		t.Errorf("err = %v", err)
	}
}

func TestDownloadTo_WritesFile(t *testing.T) {
	body := []byte("artifact-bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "artifact.bin")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer f.Close()

	c := newTestClient(srv.URL)
	if err := c.DownloadTo(testCtx, c.APIPath("/artifacts"), f); err != nil {
		t.Fatalf("DownloadTo: %v", err)
	}
	_ = f.Close()

	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("file content = %q, want %q", got, body)
	}
}

func TestDownloadTo_RetryThenSuccess(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"message":"unavailable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("done"))
	}))
	defer srv.Close()

	var buf bytes.Buffer
	c := newTestClient(srv.URL)
	if err := c.DownloadTo(testCtx, c.APIPath("/file"), &buf); err != nil {
		t.Fatalf("DownloadTo: %v", err)
	}
	if buf.String() != "done" {
		t.Errorf("body = %q", buf.String())
	}
}

func TestDownloadTo_NoRetryOn401(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"nope"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	err := c.DownloadTo(testCtx, c.APIPath("/file"), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1", attempts.Load())
	}
}

func TestDownloadTo_CopyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	err := c.DownloadTo(testCtx, c.APIPath("/file"), failingWriter{})
	if err == nil {
		t.Fatal("expected copy error")
	}
	if !strings.Contains(err.Error(), "reading response body") {
		t.Errorf("err = %v", err)
	}
}

func TestDownloadTo_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newTestClient("http://example.com")
	err := c.DownloadTo(ctx, c.APIPath("/file"), &bytes.Buffer{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestUpload_Multipart(t *testing.T) {
	var gotField, gotFilename string
	var gotContent []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		file, hdr, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		defer file.Close()
		gotField = "file"
		gotFilename = hdr.Filename
		gotContent, _ = io.ReadAll(file)
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "upload.txt")
	if err := os.WriteFile(filePath, []byte("hello upload"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c := newTestClient(srv.URL)
	data, err := c.Upload(testCtx, c.APIPath("/upload"), "file", filePath)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if !strings.Contains(string(data), `"id"`) {
		t.Errorf("response = %s", string(data))
	}
	if gotField != "file" || gotFilename != "upload.txt" {
		t.Errorf("field=%q filename=%q", gotField, gotFilename)
	}
	if string(gotContent) != "hello upload" {
		t.Errorf("upload content = %q", gotContent)
	}
}

func TestUpload_RetryThenSuccess(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Header().Set("Retry-After", "1")
			_, _ = w.Write([]byte(`{"message":"429"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c := newTestClient(srv.URL)
	data, err := c.Upload(testCtx, c.APIPath("/upload"), "file", filePath)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if !strings.Contains(string(data), "ok") {
		t.Errorf("response = %s", string(data))
	}
}

func TestUpload_FileNotFound(t *testing.T) {
	c := newTestClient("http://example.com")
	_, err := c.Upload(testCtx, c.APIPath("/upload"), "file", filepath.Join(t.TempDir(), "missing.txt"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "opening file") {
		t.Errorf("err = %v", err)
	}
}

func TestUpload_ReadResponseError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c := newTestClient("http://example.com")
	c.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       errReader{},
			Header:     make(http.Header),
		}, nil
	})
	_, err := c.Upload(testCtx, c.APIPath("/upload"), "file", filePath)
	if err == nil {
		t.Fatal("expected read error")
	}
	if !strings.Contains(err.Error(), "reading upload response") {
		t.Errorf("err = %v", err)
	}
}

func TestUpload_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newTestClient("http://example.com")
	_, err := c.Upload(ctx, c.APIPath("/upload"), "file", "missing")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRawGet_OK206And416(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Range") {
		case "bytes=0-1":
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("ab"))
		case "bytes=999-":
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			_, _ = w.Write([]byte("range bad"))
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("full"))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)

	sc, data, err := c.RawGet(testCtx, c.APIPath("/blob"), nil)
	if err != nil || sc != http.StatusOK || string(data) != "full" {
		t.Fatalf("200: sc=%d data=%q err=%v", sc, data, err)
	}

	sc, data, err = c.RawGet(testCtx, c.APIPath("/blob"), map[string]string{"Range": "bytes=0-1"})
	if err != nil || sc != http.StatusPartialContent || string(data) != "ab" {
		t.Fatalf("206: sc=%d data=%q err=%v", sc, data, err)
	}

	sc, data, err = c.RawGet(testCtx, c.APIPath("/blob"), map[string]string{"Range": "bytes=999-"})
	if err != nil || sc != http.StatusRequestedRangeNotSatisfiable || string(data) != "range bad" {
		t.Fatalf("416: sc=%d data=%q err=%v", sc, data, err)
	}
}

func TestRawGet_RetryThenSuccess(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"message":"502"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	sc, data, err := c.RawGet(testCtx, c.APIPath("/stream"), nil)
	if err != nil || sc != http.StatusOK || string(data) != "ok" {
		t.Fatalf("RawGet: sc=%d data=%q err=%v", sc, data, err)
	}
}

func TestRawGet_401NoRetry(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	sc, _, err := c.RawGet(testCtx, c.APIPath("/stream"), nil)
	if err != nil {
		t.Fatalf("RawGet should return status without error: %v", err)
	}
	if sc != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", sc)
	}
	if attempts.Load() != 1 {
		t.Errorf("attempts = %d, want 1", attempts.Load())
	}
}

func TestRawGet_RetriesExhaustedReturnsStatus(t *testing.T) {
	t.Setenv("GITLAB_CLI_MAX_RETRIES", "0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"500"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	sc, data, err := c.RawGet(testCtx, c.APIPath("/stream"), nil)
	if err != nil {
		t.Fatalf("RawGet exhausted: %v", err)
	}
	if sc != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", sc)
	}
	if !strings.Contains(string(data), "500") {
		t.Errorf("data = %s", data)
	}
}

func TestRawGet_ReadBodyError(t *testing.T) {
	c := newTestClient("http://example.com")
	c.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       errReader{},
			Header:     make(http.Header),
		}, nil
	})
	sc, _, err := c.RawGet(testCtx, c.APIPath("/x"), nil)
	if err == nil {
		t.Fatal("expected read error")
	}
	if sc != http.StatusOK {
		t.Errorf("status = %d", sc)
	}
	if !strings.Contains(err.Error(), "reading response body") {
		t.Errorf("err = %v", err)
	}
}

func TestRawGet_ContextCanceledDuringRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"429"}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	c := newTestClient(srv.URL)
	_, _, err := c.RawGet(ctx, c.APIPath("/stream"), nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRawGet_ContextCanceledImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newTestClient("http://example.com")
	_, _, err := c.RawGet(ctx, c.APIPath("/x"), nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestDownloadTo_WaitRetryCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"503"}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	c := newTestClient(srv.URL)
	err := c.DownloadTo(ctx, c.APIPath("/file"), &bytes.Buffer{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestUpload_CreateFormFileError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := uploadCreateFormFile
	uploadCreateFormFile = func(*multipart.Writer, string, string) (io.Writer, error) {
		return nil, errors.New("create form file failed")
	}
	t.Cleanup(func() { uploadCreateFormFile = orig })

	c := newTestClient("http://example.com")
	_, err := c.Upload(testCtx, c.APIPath("/upload"), "file", filePath)
	if err == nil || !strings.Contains(err.Error(), "creating form file") {
		t.Fatalf("expected create form file error, got %v", err)
	}
}

func TestUpload_CopyFileError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := uploadCopyFile
	uploadCopyFile = func(io.Writer, io.Reader) (int64, error) {
		return 0, errors.New("copy failed")
	}
	t.Cleanup(func() { uploadCopyFile = orig })

	c := newTestClient("http://example.com")
	_, err := c.Upload(testCtx, c.APIPath("/upload"), "file", filePath)
	if err == nil || !strings.Contains(err.Error(), "copying file content") {
		t.Fatalf("expected copy error, got %v", err)
	}
}

func TestUpload_HTTPDoError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := newTestClient("http://example.com")
	c.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})
	_, err := c.Upload(testCtx, c.APIPath("/upload"), "file", filePath)
	if err == nil || !strings.Contains(err.Error(), "executing upload request") {
		t.Fatalf("expected upload transport error, got %v", err)
	}
}

func TestUpload_ClientErrorAfterRetry(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad upload"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Upload(testCtx, c.APIPath("/upload"), "file", filePath)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDoWithRetry_WaitCanceledOn500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"500"}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	c := newTestClient(srv.URL)
	_, err := c.Get(ctx, c.APIPath("/user"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDownloadTo_RetryExhausted(t *testing.T) {
	t.Setenv("GITLAB_CLI_MAX_RETRIES", "0")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"503"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	err := c.DownloadTo(testCtx, c.APIPath("/file"), &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
}

func TestUpload_WaitRetryCancelled(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"429"}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	c := newTestClient(srv.URL)
	_, err := c.Upload(ctx, c.APIPath("/upload"), "file", filePath)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestPaginateGET_MultiPageStopsWhenNoNextPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Next-Page", "0")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1},{"id":2}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	data, _, err := PaginateGET(testCtx, c, c.APIPath("/projects"), 150)
	if err != nil {
		t.Fatalf("PaginateGET: %v", err)
	}
	if string(data) != `[{"id":1},{"id":2}]` {
		t.Errorf("body = %s", string(data))
	}
}

func TestDoWithRetry_NetworkRetriesExhausted(t *testing.T) {
	t.Setenv("GITLAB_CLI_MAX_RETRIES", "0")
	c := newTestClient("http://example.com")
	c.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection reset by peer")
	})
	_, err := c.Get(testCtx, c.APIPath("/user"))
	if err == nil || !strings.Contains(err.Error(), "executing request") {
		t.Fatalf("expected executing request error, got %v", err)
	}
}

func TestUpload_RetryExhausted(t *testing.T) {
	t.Setenv("GITLAB_CLI_MAX_RETRIES", "0")
	dir := t.TempDir()
	filePath := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"500"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Upload(testCtx, c.APIPath("/upload"), "file", filePath)
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
}

func TestPaginateGET_SinglePageHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad"}`))
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	_, _, err := PaginateGET(testCtx, c, c.APIPath("/projects"), 10)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_InvalidHostNewRequestErrors(t *testing.T) {
	c := newTestClient("http://example.com")
	c.host = "http://[\n"

	_, err := c.Get(testCtx, "/api/v4/user")
	if err == nil || !strings.Contains(err.Error(), "creating request") {
		t.Fatalf("Get: expected creating request error, got %v", err)
	}

	err = c.DownloadTo(testCtx, "/api/v4/file", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "creating request") {
		t.Fatalf("DownloadTo: expected creating request error, got %v", err)
	}

	_, _, err = c.RawGet(testCtx, "/api/v4/trace", nil)
	if err == nil || !strings.Contains(err.Error(), "creating request") {
		t.Fatalf("RawGet: expected creating request error, got %v", err)
	}
}

func TestDownloadTo_NonRetryableTransportError(t *testing.T) {
	c := newTestClient("http://example.com")
	c.DownloadClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("certificate verify failed")
	})
	err := c.DownloadTo(testCtx, "/api/v4/file", &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "executing request") {
		t.Fatalf("expected executing request error, got %v", err)
	}
}

func TestUpload_InvalidHostNewRequest(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := newTestClient("http://example.com")
	c.host = "http://[\n"
	_, err := c.Upload(testCtx, "/api/v4/upload", "file", filePath)
	if err == nil || !strings.Contains(err.Error(), "creating upload request") {
		t.Fatalf("expected creating upload request error, got %v", err)
	}
}

func TestDeleteWithBody_SendsJSON(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if err := c.DeleteWithBody(testCtx, c.APIPath("/files/x"), map[string]string{"branch": "main"}); err != nil {
		t.Fatalf("DeleteWithBody: %v", err)
	}
	if !strings.Contains(body, "main") {
		t.Errorf("body = %s", body)
	}
}

func clientWithInvalidHost() *Client {
	c := newTestClient("http://example.com")
	c.host = "http://[::1"
	return c
}

func TestDoWithRetry_InvalidRequestURL(t *testing.T) {
	c := clientWithInvalidHost()
	_, err := c.Get(testCtx, c.APIPath("/user"))
	if err == nil || !strings.Contains(err.Error(), "creating request") {
		t.Fatalf("expected creating request error, got %v", err)
	}
}

func TestDoWithRetry_NetworkRetryContextCancelled(t *testing.T) {
	t.Setenv("GITLAB_CLI_MAX_RETRIES", "10")
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	c := newTestClient("http://127.0.0.1:1")
	c.httpClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) >= 2 {
			cancel()
		}
		return nil, errors.New("connection reset by peer")
	})

	_, err := c.Get(ctx, c.APIPath("/user"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDownloadTo_InvalidRequestURL(t *testing.T) {
	c := clientWithInvalidHost()
	err := c.DownloadTo(testCtx, c.APIPath("/file"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "creating request") {
		t.Fatalf("expected creating request error, got %v", err)
	}
}

func TestDownloadTo_TransportError(t *testing.T) {
	c := newTestClient("http://127.0.0.1:1")
	c.DownloadClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})
	err := c.DownloadTo(testCtx, c.APIPath("/file"), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "executing request") {
		t.Fatalf("expected executing request error, got %v", err)
	}
}

func TestUpload_InvalidRequestURL(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := clientWithInvalidHost()
	_, err := c.Upload(testCtx, c.APIPath("/upload"), "file", filePath)
	if err == nil || !strings.Contains(err.Error(), "creating upload request") {
		t.Fatalf("expected creating upload request error, got %v", err)
	}
}

func TestRawGet_InvalidRequestURL(t *testing.T) {
	c := clientWithInvalidHost()
	_, _, err := c.RawGet(testCtx, c.APIPath("/stream"), nil)
	if err == nil || !strings.Contains(err.Error(), "creating request") {
		t.Fatalf("expected creating request error, got %v", err)
	}
}

func TestDefaultUserAgent_Default(t *testing.T) {
	t.Setenv("GITLAB_CLI_USER_AGENT", "")
	if got := defaultUserAgent(); got != "gitlab-cli" {
		t.Errorf("defaultUserAgent() = %q, want gitlab-cli", got)
	}
}

func TestParseError_EmptyMessageString(t *testing.T) {
	c := newTestClient("http://example.com")
	err := c.parseError(400, []byte(`{"message":""}`))
	if len(err.ErrorMessages) == 0 || !strings.Contains(err.ErrorMessages[0], "unexpected status code") {
		t.Errorf("unexpected: %+v", err)
	}
}

func TestRateLimitWait_RateLimitResetFuture(t *testing.T) {
	reset := strconv.FormatInt(time.Now().Add(5*time.Second).Unix(), 10)
	h := http.Header{}
	h.Set("RateLimit-Reset", reset)
	wait := rateLimitWait(h)
	if wait <= 0 || wait > maxRateLimitWait {
		t.Errorf("wait = %v", wait)
	}
}
