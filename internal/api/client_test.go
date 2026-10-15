package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

type failWriter = failingWriter

var testCtx = context.Background()

// newTestClient returns a Client pointed at the given test server URL, with short
// retry back-offs.
func newTestClient(serverURL string) *Client {
	return NewClient(&Config{Host: serverURL, Token: "test-token", RetryBase: time.Millisecond})
}

// closedURL is the URL of a port nothing listens on: connections are refused.
func closedURL(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return "http://" + addr
}

func TestEncodeProjectPath(t *testing.T) {
	cases := map[string]string{"42": "42", "acme/web": "acme%2Fweb", " group/sub/proj ": "group%2Fsub%2Fproj"} //nolint:gocritic // the spaces are the input under test
	for in, want := range cases {
		if got := EncodeProjectPath(in); got != want {
			t.Errorf("EncodeProjectPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewClient_DefaultsAndHost(t *testing.T) {
	c := NewClient(&Config{Host: "https://gitlab.example.com/", Token: " tok "})
	if c.Host() != "https://gitlab.example.com" {
		t.Errorf("Host() = %q", c.Host())
	}
	if c.authHeader != "Bearer tok" {
		t.Errorf("authHeader = %q", c.authHeader)
	}
	if c.httpClient.Timeout != DefaultTimeout {
		t.Errorf("timeout = %v, want %v", c.httpClient.Timeout, DefaultTimeout)
	}
	if NewClient(&Config{Host: "h", Timeout: time.Hour}).httpClient.Timeout != DefaultTimeout {
		t.Error("a timeout above 30 s must be capped (CLI-110)")
	}
	if NewClient(&Config{Host: "h", Timeout: time.Second}).httpClient.Timeout != time.Second {
		t.Error("a shorter timeout must be kept")
	}
	if NewClient(&Config{Host: "h"}).authHeader != "" {
		t.Error("no token, no Authorization header")
	}
}

func TestAPIPath(t *testing.T) {
	c := newTestClient("http://example.com")
	if got := c.APIPath("projects"); got != "/api/v4/projects" {
		t.Errorf("APIPath = %q", got)
	}
	if got := c.APIPath("/user"); got != "/api/v4/user" {
		t.Errorf("APIPath = %q", got)
	}
}

func TestGet_SendsTokenAndUserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "gitlab-cli/2.0.0 aicli/2.0.0" {
			t.Errorf("User-Agent = %q", got)
		}
		if r.Header.Get("Idempotency-Key") != "" {
			t.Error("no Idempotency-Key header: GitLab ignores it and net/http would re-send a write carrying it")
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	c := NewClient(&Config{Host: srv.URL, Token: "test-token", UserAgent: "gitlab-cli/2.0.0 aicli/2.0.0"})
	data, err := c.Get(testCtx, "/api/v4/x")
	if err != nil || string(data) != `{"ok":true}` {
		t.Fatalf("Get = %s, %v", data, err)
	}
}

func TestDebugf_OneLinePerRequestWithoutSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	var lines []string
	c := NewClient(&Config{Host: srv.URL, Token: "s3cr3t", Debugf: func(f string, a ...any) {
		lines = append(lines, strings.TrimSpace(sprintf(f, a...)))
	}})
	if _, err := c.Get(testCtx, "/api/v4/user"); err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "GET /api/v4/user -> 200") || strings.Contains(lines[0], "s3cr3t") {
		t.Fatalf("debug lines = %q", lines)
	}
}

func TestParseError_Shapes(t *testing.T) {
	cases := []struct {
		body     string
		messages []string
		fields   map[string]string
	}{
		{`{"message":"404 Project Not Found"}`, []string{"404 Project Not Found"}, nil},
		{`{"message":{"name":["has already been taken"]}}`, nil, map[string]string{"name": "has already been taken"}},
		{`{"error":"invalid_grant","error_description":"expired"}`, []string{"invalid_grant: expired"}, nil},
		{`not json`, []string{"unexpected status code 400"}, nil},
	}
	for _, tc := range cases {
		e := parseError(400, nil, []byte(tc.body))
		if strings.Join(e.ErrorMessages, "|") != strings.Join(tc.messages, "|") {
			t.Errorf("%s: messages %v, want %v", tc.body, e.ErrorMessages, tc.messages)
		}
		for k, v := range tc.fields {
			if e.Errors[k] != v {
				t.Errorf("%s: field %s = %q", tc.body, k, e.Errors[k])
			}
		}
	}
	e := parseError(422, nil, []byte(`{"message":{"title":["is too long"],"color":["is invalid"]}}`))
	if e.Upstream() != "color: is invalid; title: is too long" {
		t.Errorf("Upstream() = %q", e.Upstream())
	}
	if !strings.Contains(e.Error(), "422") {
		t.Errorf("Error() = %q", e.Error())
	}
}

func TestRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)
	h := http.Header{}
	h.Set("Retry-After", "7")
	if got := retryAfter(h, now); got != 7*time.Second {
		t.Errorf("seconds: %v", got)
	}
	h = http.Header{}
	h.Set("Retry-After", now.Add(3*time.Second).Format(http.TimeFormat))
	if got := retryAfter(h, now); got != 3*time.Second {
		t.Errorf("date: %v", got)
	}
	h = http.Header{}
	h.Set("RateLimit-Reset", strconv.FormatInt(now.Add(5*time.Second).Unix(), 10))
	if got := retryAfter(h, now); got != 5*time.Second {
		t.Errorf("reset: %v", got)
	}
	if got := retryAfter(http.Header{}, now); got != 0 {
		t.Errorf("none: %v", got)
	}
}

// TestReadsRetryOnlyTheRetryableStatuses: 429, 502, 503 and 504 are retried at most
// twice; 500 and other errors are not (CLI-110).
func TestReadsRetryOnlyTheRetryableStatuses(t *testing.T) {
	for status, wantAttempts := range map[int]int32{429: 3, 502: 3, 503: 3, 504: 3, 500: 1, 404: 1, 401: 1} {
		var attempts atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)
			w.WriteHeader(status)
		}))
		c := newTestClient(srv.URL)
		_, err := c.Get(testCtx, "/api/v4/x")
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != status {
			t.Errorf("%d: err = %v", status, err)
		}
		if attempts.Load() != wantAttempts {
			t.Errorf("%d: %d attempts, want %d", status, attempts.Load(), wantAttempts)
		}
		srv.Close()
	}
}

func TestReadRetrySucceeds(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	if _, err := newTestClient(srv.URL).Get(testCtx, "/api/v4/x"); err != nil || attempts.Load() != 2 {
		t.Fatalf("err %v after %d attempts", err, attempts.Load())
	}
}

// TestRetryAfterAboveTenSecondsIsReturned: a requested wait over 10 s is not waited
// for; the error carries it (retry_after_ms, CLI-016).
func TestRetryAfterAboveTenSecondsIsReturned(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Retry-After", "11")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	start := time.Now()
	_, err := newTestClient(srv.URL).Get(testCtx, "/api/v4/x")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.RetryAfter != 11*time.Second || attempts.Load() != 1 || time.Since(start) > 5*time.Second {
		t.Fatalf("err %v, attempts %d", err, attempts.Load())
	}
}

// TestWritesAreNeverRetried: a write is sent once, whatever the answer (CLI-110).
func TestWritesAreNeverRetried(t *testing.T) {
	for _, status := range []int{429, 500, 502, 503, 504} {
		var attempts atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)
			w.WriteHeader(status)
		}))
		c := newTestClient(srv.URL)
		for name, call := range map[string]func() error{
			"POST":   func() error { _, err := c.Post(testCtx, "/api/v4/x", map[string]any{"a": 1}); return err },
			"PUT":    func() error { _, err := c.Put(testCtx, "/api/v4/x", map[string]any{"a": 1}); return err },
			"DELETE": func() error { return c.Delete(testCtx, "/api/v4/x") },
		} {
			attempts.Store(0)
			if err := call(); err == nil {
				t.Errorf("%s %d: expected an error", name, status)
			}
			if attempts.Load() != 1 {
				t.Errorf("%s %d: sent %d times, want once", name, status, attempts.Load())
			}
		}
		srv.Close()
	}
}

// TestRefusedConnection: a read is retried twice and reports that nothing was sent;
// a write is tried once and reports the same.
func TestRefusedConnection(t *testing.T) {
	c := newTestClient(closedURL(t))
	_, err := c.Get(testCtx, "/api/v4/x")
	var ne *NetworkError
	if !errors.As(err, &ne) || ne.Sent || ne.Timeout {
		t.Fatalf("GET: %v", err)
	}
	_, err = c.Post(testCtx, "/api/v4/x", map[string]any{})
	if !errors.As(err, &ne) || ne.Sent {
		t.Fatalf("POST: %v", err)
	}
}

// TestDroppedConnectionAfterSending: GitLab read the write and closed the
// connection without answering; the write may have been applied (Sent).
func TestDroppedConnectionAfterSending(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		_, _ = io.ReadAll(r.Body)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = conn.Close()
		}
	}))
	defer srv.Close()
	_, err := newTestClient(srv.URL).Post(testCtx, "/api/v4/x", map[string]any{"title": "x"})
	var ne *NetworkError
	if !errors.As(err, &ne) || !ne.Sent {
		t.Fatalf("err = %v", err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("the write was sent %d times", attempts.Load())
	}
}

func TestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()
	c := NewClient(&Config{Host: srv.URL, Timeout: 50 * time.Millisecond})
	_, err := c.Get(testCtx, "/api/v4/x")
	var ne *NetworkError
	if !errors.As(err, &ne) || !ne.Timeout || !ne.Sent {
		t.Fatalf("err = %v", err)
	}
}

func TestTLSFailureIsMarked(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c := newTestClient(srv.URL) // the default transport does not trust the test CA
	_, err := c.Post(testCtx, "/api/v4/x", map[string]any{})
	var ne *NetworkError
	if !errors.As(err, &ne) || !ne.TLS || ne.Sent {
		t.Fatalf("err = %v", err)
	}
}

func TestGetWithPagination_ParsesHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Total", "42")
		w.Header().Set("X-Total-Pages", "3")
		w.Header().Set("X-Page", "1")
		w.Header().Set("X-Per-Page", "20")
		w.Header().Set("X-Next-Page", "2")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	_, pag, err := newTestClient(srv.URL).GetWithPagination(testCtx, "/api/v4/x")
	if err != nil {
		t.Fatal(err)
	}
	if !pag.HasTotal || pag.Total != 42 || pag.TotalPages != 3 || pag.Page != 1 || pag.PerPage != 20 || pag.NextPage != 2 {
		t.Errorf("pagination = %+v", pag)
	}
	if extractPagination(http.Header{}).HasTotal {
		t.Error("no X-Total, no HasTotal")
	}
}

func TestGetPage_SetsPageAndPerPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("page") != "3" || q.Get("per_page") != "10" || q.Get("state") != "active" {
			t.Errorf("query = %v", q)
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	if _, _, err := GetPage(testCtx, c, "/api/v4/x?state=active", 3, 10); err != nil {
		t.Fatal(err)
	}
}

func TestPostPutDelete_Verbs(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method != http.MethodDelete && r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("%s without a JSON content type", r.Method)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	_, _ = c.Post(testCtx, "/api/v4/x", map[string]any{"a": 1})
	_, _ = c.Put(testCtx, "/api/v4/x", map[string]any{"a": 1})
	_ = c.Delete(testCtx, "/api/v4/x")
	_ = c.DeleteWithBody(testCtx, "/api/v4/x", map[string]any{"branch": "main"})
	if strings.Join(methods, ",") != "POST,PUT,DELETE,DELETE" {
		t.Errorf("methods = %v", methods)
	}
}

func TestRedirects_KeepAuthOnlyOnTheSameHost(t *testing.T) {
	var gotAuth atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{}`))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/away" {
			http.Redirect(w, r, other.URL+"/landing", http.StatusFound)
			return
		}
		if r.URL.Path == "/api/v4/here" {
			http.Redirect(w, r, "/api/v4/landing", http.StatusFound)
			return
		}
		gotAuth.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	if _, err := c.Get(testCtx, "/api/v4/here"); err != nil || gotAuth.Load() != "Bearer test-token" {
		t.Fatalf("same host: auth %v, err %v", gotAuth.Load(), err)
	}
	// 127.0.0.1 with another port is another host:port, so the token must not follow.
	if _, err := c.Get(testCtx, "/api/v4/away"); err != nil || gotAuth.Load() != "" {
		t.Fatalf("cross host: auth %v, err %v", gotAuth.Load(), err)
	}
}

func TestRawGet(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/range":
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		case "/flaky":
			if attempts.Add(1) == 1 {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("log"))
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	if code, _, err := c.RawGet(testCtx, "/range", map[string]string{"Range": "bytes=10-"}); err != nil || code != 416 {
		t.Errorf("416: %d %v", code, err)
	}
	if code, body, err := c.RawGet(testCtx, "/flaky", nil); err != nil || code != 206 || string(body) != "log" {
		t.Errorf("retry: %d %s %v", code, body, err)
	}
	if code, _, err := c.RawGet(testCtx, "/denied", nil); err != nil || code != 401 {
		t.Errorf("401: %d %v", code, err)
	}
}

func TestDownloadTo(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("artifact-bytes"))
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	var buf bytes.Buffer
	if err := c.DownloadTo(testCtx, "/file", &buf); err != nil || buf.String() != "artifact-bytes" {
		t.Fatalf("download: %q %v", buf.String(), err)
	}
	var apiErr *APIError
	if err := c.DownloadTo(testCtx, "/missing", &buf); !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Fatalf("404: %v", err)
	}
	if err := c.DownloadTo(testCtx, "/file", failingWriter{}); err == nil {
		t.Fatal("a failing writer must fail the download")
	}
}

func TestUpload_IsAWriteSentOnce(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("content type %q", r.Header.Get("Content-Type"))
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("multipart: %v", err)
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	file := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newTestClient(srv.URL)
	if _, err := c.Upload(testCtx, "/api/v4/upload", "file", file); err == nil {
		t.Fatal("expected the 502")
	}
	if attempts.Load() != 1 {
		t.Fatalf("uploaded %d times, want once", attempts.Load())
	}
	if _, err := c.Upload(testCtx, "/api/v4/upload", "file", filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a missing file must fail")
	}
	orig := uploadCreateFormFile
	uploadCreateFormFile = func(*multipart.Writer, string, string) (io.Writer, error) { return nil, errors.New("boom") }
	t.Cleanup(func() { uploadCreateFormFile = orig })
	if _, err := c.Upload(testCtx, "/api/v4/upload", "file", file); err == nil {
		t.Fatal("a form error must fail")
	}
}

func TestContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := newTestClient("http://example.com").Get(ctx, "/api/v4/x")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestUser_Me(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/user" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":7,"username":"carol","name":"Carol"}`))
	}))
	defer srv.Close()
	u, err := newTestClient(srv.URL).Users.Me(testCtx)
	if err != nil || u.Username != "carol" || u.ID != 7 {
		t.Fatalf("Me = %+v, %v", u, err)
	}
}

func TestUser_SearchAndGetByUsername(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Query().Get("username") == "nobody":
			_, _ = w.Write([]byte(`[]`))
		default:
			_, _ = w.Write([]byte(`[{"id":7,"username":"carol"}]`))
		}
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	if users, err := c.Users.Search(testCtx, "car", 20); err != nil || len(users) != 1 {
		t.Fatalf("Search = %v, %v", users, err)
	}
	if u, err := c.Users.GetByUsername(testCtx, "carol"); err != nil || u == nil || u.ID != 7 {
		t.Fatalf("GetByUsername = %v, %v", u, err)
	}
	if u, err := c.Users.GetByUsername(testCtx, "nobody"); err != nil || u != nil {
		t.Fatalf("missing user = %v, %v", u, err)
	}
}

func sprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }
