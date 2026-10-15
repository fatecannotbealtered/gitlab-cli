package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// APIBasePath is the GitLab REST API v4 base path.
const APIBasePath = "/api/v4"

// Network rules (CLI-110). A request has a timeout of at most 30 s. Only reads
// (GET and HEAD) are retried, at most twice, after a connection failure or an HTTP
// 429, 502, 503 or 504, honouring Retry-After up to 10 s; a longer requested wait
// returns the error at once. A write is sent exactly once: a write that may have
// reached GitLab is never re-sent (CLI-015).
const (
	DefaultTimeout = 30 * time.Second
	maxTimeout     = 30 * time.Second
	maxReadRetries = 2
	maxRetryAfter  = 10 * time.Second
	downloadIdle   = 30 * time.Second
)

// Config is what a Client needs. The caller builds the transport (proxy, TLS and CA
// file) from its own environment, so this package reads no environment variable.
type Config struct {
	Host      string            // https://gitlab.example.com; a trailing slash is dropped
	Token     string            // personal access token; "" sends no Authorization header
	UserAgent string            // "gitlab-cli/<version> aicli/<spec version>" (CLI-110)
	Transport http.RoundTripper // nil: a clone of http.DefaultTransport
	Timeout   time.Duration     // per request; 0 or more than 30 s means 30 s
	// RetryBase is the first back-off of a retried read without Retry-After; the
	// second waits twice as long. 0 means 250 ms (connection failures wait 100 ms).
	RetryBase time.Duration
	// Debugf receives one line per request (method, path, status, duration), never
	// a header or a body. nil logs nothing.
	Debugf func(format string, args ...any)
}

// Client wraps the GitLab REST API v4.
type Client struct {
	host       string
	authHeader string
	userAgent  string
	retryBase  time.Duration
	debugf     func(format string, args ...any)
	httpClient *http.Client

	// DownloadClient is used for large or binary downloads (artifacts). It has no
	// overall timeout; DownloadTo fails after 30 s without data instead.
	DownloadClient *http.Client

	// API groups, populated by NewClient. Each command group implements the methods
	// of its API in its own file (internal/api/mr.go, ...). Alphabetical.
	Issues        *IssueAPI
	Jobs          *JobAPI
	Labels        *LabelAPI
	MergeRequests *MergeRequestAPI
	Milestones    *MilestoneAPI
	Pipelines     *PipelineAPI
	Projects      *ProjectAPI
	Releases      *ReleaseAPI
	Repos         *RepoAPI
	Search        *SearchAPI
	Users         *UserAPI
	Variables     *VariableAPI
}

// NewClient creates a client from cfg.
func NewClient(cfg *Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 || timeout > maxTimeout {
		timeout = DefaultTimeout
	}
	tr := cfg.Transport
	if tr == nil {
		tr = http.DefaultTransport.(*http.Transport).Clone()
	}
	ua := strings.TrimSpace(cfg.UserAgent)
	if ua == "" {
		ua = "gitlab-cli"
	}
	c := &Client{
		host:           strings.TrimRight(strings.TrimSpace(cfg.Host), "/"),
		userAgent:      ua,
		retryBase:      cfg.RetryBase,
		debugf:         cfg.Debugf,
		httpClient:     newHTTPClient(tr, timeout),
		DownloadClient: newHTTPClient(tr, 0),
	}
	if c.retryBase <= 0 {
		c.retryBase = 250 * time.Millisecond
	}
	if tok := strings.TrimSpace(cfg.Token); tok != "" {
		c.authHeader = "Bearer " + tok
	}
	c.Issues = &IssueAPI{client: c}
	c.Jobs = &JobAPI{client: c}
	c.Labels = &LabelAPI{client: c}
	c.MergeRequests = &MergeRequestAPI{client: c}
	c.Milestones = &MilestoneAPI{client: c}
	c.Pipelines = &PipelineAPI{client: c}
	c.Projects = &ProjectAPI{client: c}
	c.Releases = &ReleaseAPI{client: c}
	c.Repos = &RepoAPI{client: c}
	c.Search = &SearchAPI{client: c}
	c.Users = &UserAPI{client: c}
	c.Variables = &VariableAPI{client: c}
	return c
}

// Host returns the GitLab host URL (without a trailing slash).
func (c *Client) Host() string { return c.host }

// APIPath returns "/api/v4" + the relative resource path.
func (c *Client) APIPath(resource string) string {
	if !strings.HasPrefix(resource, "/") {
		resource = "/" + resource
	}
	return APIBasePath + resource
}

// EncodeProjectPath URL-encodes a "group/subgroup/project" path for use in
// /api/v4/projects/:id endpoints. A numeric project ID is left as-is.
func EncodeProjectPath(projectPath string) string {
	p := strings.TrimSpace(projectPath)
	if _, err := strconv.Atoi(p); err == nil {
		return p
	}
	return url.PathEscape(p)
}

func newHTTPClient(tr http.RoundTripper, timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: tr,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			// Keep Authorization only on the same host: a cross-origin redirect must
			// never carry the token.
			if len(via) > 0 {
				prev := via[len(via)-1]
				if strings.EqualFold(req.URL.Host, prev.URL.Host) {
					if auth := prev.Header.Get("Authorization"); auth != "" {
						req.Header.Set("Authorization", auth)
					}
				} else {
					req.Header.Del("Authorization")
				}
				if ua := prev.Header.Get("User-Agent"); ua != "" {
					req.Header.Set("User-Agent", ua)
				}
			}
			return nil
		},
	}
}

// ─── Errors ─────────────────────────────────────────────────────────────────

// APIError is an HTTP error answer from GitLab.
type APIError struct {
	StatusCode    int
	ErrorMessages []string
	Errors        map[string]string
	// RetryAfter is the wait GitLab asked for (Retry-After or RateLimit-Reset), 0
	// when it named none.
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	if len(e.ErrorMessages) > 0 {
		return fmt.Sprintf("GitLab API error %d: %s", e.StatusCode, e.ErrorMessages[0])
	}
	if len(e.Errors) > 0 {
		return fmt.Sprintf("GitLab API error %d: %v", e.StatusCode, e.Errors)
	}
	return fmt.Sprintf("GitLab API error %d", e.StatusCode)
}

// Upstream returns GitLab's own error text, for details.upstream_message (never
// for a message written for the agent, CLI-017).
func (e *APIError) Upstream() string {
	parts := append([]string{}, e.ErrorMessages...)
	for _, k := range sortedKeys(e.Errors) {
		parts = append(parts, k+": "+e.Errors[k])
	}
	return strings.Join(parts, "; ")
}

// NetworkError is a request that got no HTTP answer. Sent reports whether the
// request's headers were written to a connection: when it is false nothing reached
// GitLab, so a write was not dispatched; when it is true a write may have been
// applied (CLI-015). Timeout marks a deadline that expired; TLS a certificate or
// handshake failure (nothing was sent).
type NetworkError struct {
	Method  string
	Sent    bool
	Timeout bool
	TLS     bool
	Err     error
}

func (e *NetworkError) Error() string {
	state := "nothing was sent"
	if e.Sent {
		state = "the request was sent"
	}
	return fmt.Sprintf("%s request failed (%s): %v", e.Method, state, e.Err)
}

func (e *NetworkError) Unwrap() error { return e.Err }

func classifyTransportError(method string, sent bool, err error) *NetworkError {
	ne := &NetworkError{Method: method, Sent: sent, Err: err}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		ne.Timeout = true
	}
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalid x509.CertificateInvalidError
	var verify *tls.CertificateVerificationError
	var record tls.RecordHeaderError
	if errors.As(err, &unknownAuthority) || errors.As(err, &hostname) || errors.As(err, &invalid) ||
		errors.As(err, &verify) || errors.As(err, &record) {
		ne.TLS = true
	}
	return ne
}

// parseError converts an HTTP status code and response body into an APIError.
//
// GitLab returns several error shapes:
//
//	{"message": "404 Project Not Found"}                          (string message)
//	{"message": {"name": ["has already been taken"]}}             (per-field validation)
//	{"error": "invalid_grant", "error_description": "..."}        (OAuth-style)
func parseError(statusCode int, header http.Header, data []byte) *APIError {
	apiErr := &APIError{StatusCode: statusCode, RetryAfter: retryAfter(header, time.Now())}
	if len(data) > 0 {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err == nil {
			if msg, ok := raw["message"]; ok {
				var s string
				if err := json.Unmarshal(msg, &s); err == nil && s != "" {
					apiErr.ErrorMessages = []string{s}
				} else {
					var m map[string][]string
					if err := json.Unmarshal(msg, &m); err == nil && len(m) > 0 {
						apiErr.Errors = make(map[string]string, len(m))
						for k, vs := range m {
							apiErr.Errors[k] = strings.Join(vs, "; ")
						}
					}
				}
			}
			if errStr, ok := raw["error"]; ok {
				var s string
				if err := json.Unmarshal(errStr, &s); err == nil && s != "" {
					if desc, hasDesc := raw["error_description"]; hasDesc {
						var d string
						if err := json.Unmarshal(desc, &d); err == nil && d != "" {
							s = s + ": " + d
						}
					}
					apiErr.ErrorMessages = append(apiErr.ErrorMessages, s)
				}
			}
		}
	}
	if len(apiErr.ErrorMessages) == 0 && len(apiErr.Errors) == 0 {
		apiErr.ErrorMessages = []string{fmt.Sprintf("unexpected status code %d", statusCode)}
	}
	return apiErr
}

// retryAfter reads the wait GitLab asks for: Retry-After (seconds or an HTTP date),
// else RateLimit-Reset (a Unix time). 0 when neither names a future time.
func retryAfter(h http.Header, now time.Time) time.Duration {
	if h == nil {
		return 0
	}
	if ra := strings.TrimSpace(h.Get("Retry-After")); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
		if t, err := http.ParseTime(ra); err == nil && t.After(now) {
			return t.Sub(now)
		}
	}
	if reset := strings.TrimSpace(h.Get("RateLimit-Reset")); reset != "" {
		if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
			if d := time.Unix(ts, 0).Sub(now); d > 0 {
				return d
			}
		}
	}
	return 0
}

// ─── Pagination ─────────────────────────────────────────────────────────────

// Pagination holds the page metadata GitLab returns in response headers.
type Pagination struct {
	Total      int    // X-Total; valid only when HasTotal (GitLab omits it for large lists)
	HasTotal   bool   // the response carried X-Total
	TotalPages int    // X-Total-Pages
	Page       int    // X-Page
	PerPage    int    // X-Per-Page
	NextPage   int    // X-Next-Page (0 on the last page)
	PrevPage   int    // X-Prev-Page (0 on the first page)
	Link       string // Link header
}

func extractPagination(h http.Header) Pagination {
	atoi := func(s string) int {
		n, _ := strconv.Atoi(strings.TrimSpace(s))
		return n
	}
	p := Pagination{
		Total:      atoi(h.Get("X-Total")),
		TotalPages: atoi(h.Get("X-Total-Pages")),
		Page:       atoi(h.Get("X-Page")),
		PerPage:    atoi(h.Get("X-Per-Page")),
		NextPage:   atoi(h.Get("X-Next-Page")),
		PrevPage:   atoi(h.Get("X-Prev-Page")),
		Link:       h.Get("Link"),
	}
	if _, err := strconv.Atoi(strings.TrimSpace(h.Get("X-Total"))); err == nil {
		p.HasTotal = true
	}
	return p
}

// ─── Requests ───────────────────────────────────────────────────────────────

// isRead reports the methods CLI-110 lets the client retry.
func isRead(method string) bool { return method == http.MethodGet || method == http.MethodHead }

// response is one HTTP answer.
type response struct {
	status int
	header http.Header
	body   []byte
}

// send performs one attempt. The trace records whether the request's headers were
// written, which tells a failed write that never left the machine from one that
// may have reached GitLab.
func (c *Client) send(ctx context.Context, client *http.Client, method, path string, body []byte, contentType string,
	headers map[string]string) (*response, error) {
	if err := ctx.Err(); err != nil {
		return nil, &NetworkError{Method: method, Err: err, Timeout: errors.Is(err, context.DeadlineExceeded)}
	}
	wrote := false
	trace := &httptrace.ClientTrace{WroteHeaders: func() { wrote = true }}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), method, c.host+path, reader)
	if err != nil {
		return nil, &NetworkError{Method: method, Err: fmt.Errorf("creating request: %w", err)}
	}
	if c.authHeader != "" {
		req.Header.Set("Authorization", c.authHeader)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		c.debug("%s %s -> no answer after %d ms", method, path, time.Since(start).Milliseconds())
		return nil, classifyTransportError(method, wrote, err)
	}
	data, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	c.debug("%s %s -> %d in %d ms", method, path, resp.StatusCode, time.Since(start).Milliseconds())
	if readErr != nil {
		// The status line arrived, so the request was sent; the answer is incomplete.
		return nil, classifyTransportError(method, true, fmt.Errorf("reading response body: %w", readErr))
	}
	return &response{status: resp.StatusCode, header: resp.Header, body: data}, nil
}

func (c *Client) debug(format string, args ...any) {
	if c.debugf != nil {
		c.debugf(format, args...)
	}
}

// retryWait decides whether a failed read attempt is retried and how long to wait
// first (CLI-110): connection failures (never a timeout, a TLS failure or a
// request that could not be built) and 429, 502, 503 and 504, at most twice, with
// Retry-After honoured up to 10 s. A longer requested wait is not retried.
func (c *Client) retryWait(attempt int, resp *response, err error) (time.Duration, bool) {
	if attempt >= maxReadRetries {
		return 0, false
	}
	if err != nil {
		var ne *NetworkError
		if !errors.As(err, &ne) || ne.Timeout || ne.TLS || errors.Is(err, context.Canceled) {
			return 0, false
		}
		var opErr *net.OpError
		if !errors.As(err, &opErr) && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, false
		}
		// A refused or dropped connection: retry soon (100 ms, then 200 ms by default).
		return c.retryBase * 2 / 5 * time.Duration(1<<uint(attempt)), true
	}
	switch resp.status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
	default:
		return 0, false
	}
	if d := retryAfter(resp.header, time.Now()); d > 0 {
		if d > maxRetryAfter {
			return 0, false
		}
		return d, true
	}
	return c.retryBase * time.Duration(1<<uint(attempt)), true
}

// do runs a request: reads with the retry rules of CLI-110, writes exactly once.
// A status of 400 or more is returned as *APIError.
func (c *Client) do(ctx context.Context, method, path string, body []byte, contentType string,
	headers map[string]string) (*response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := c.send(ctx, c.httpClient, method, path, body, contentType, headers)
		if err == nil && resp.status < 400 {
			return resp, nil
		}
		if isRead(method) {
			if wait, retry := c.retryWait(attempt, resp, err); retry {
				if serr := sleep(ctx, wait); serr != nil {
					return nil, &NetworkError{Method: method, Err: serr}
				}
				continue
			}
		}
		if err != nil {
			return nil, err
		}
		return resp, parseError(resp.status, resp.header, resp.body)
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// doJSON encodes body as JSON (when not nil) and runs the request.
func (c *Client) doJSON(ctx context.Context, method, path string, body any) ([]byte, http.Header, error) {
	var payload []byte
	contentType := ""
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("encoding request body: %w", err)
		}
		payload, contentType = b, "application/json"
	}
	resp, err := c.do(ctx, method, path, payload, contentType, nil)
	if err != nil {
		return nil, nil, err
	}
	return resp.body, resp.header, nil
}

// ─── Convenience verbs ──────────────────────────────────────────────────────

// Get sends a GET request and returns the body.
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	data, _, err := c.doJSON(ctx, http.MethodGet, path, nil)
	return data, err
}

// GetWithPagination sends a GET request and returns the body and the pagination
// headers. Useful for list endpoints.
func (c *Client) GetWithPagination(ctx context.Context, path string) ([]byte, Pagination, error) {
	data, header, err := c.doJSON(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, Pagination{}, err
	}
	return data, extractPagination(header), nil
}

// Post sends a POST request (sent once, never retried).
func (c *Client) Post(ctx context.Context, path string, body any) ([]byte, error) {
	data, _, err := c.doJSON(ctx, http.MethodPost, path, body)
	return data, err
}

// Put sends a PUT request (sent once, never retried).
func (c *Client) Put(ctx context.Context, path string, body any) ([]byte, error) {
	data, _, err := c.doJSON(ctx, http.MethodPut, path, body)
	return data, err
}

// Delete sends a DELETE request (sent once, never retried).
func (c *Client) Delete(ctx context.Context, path string) error {
	_, _, err := c.doJSON(ctx, http.MethodDelete, path, nil)
	return err
}

// DeleteWithBody sends a DELETE request with a JSON body. GitLab takes parameters in
// the body on some DELETE endpoints (repository file deletion).
func (c *Client) DeleteWithBody(ctx context.Context, path string, body any) error {
	_, _, err := c.doJSON(ctx, http.MethodDelete, path, body)
	return err
}

// RawGet performs a GET with optional extra headers and returns the status code and
// body. Statuses of 400 or more are returned, not turned into errors; reads retry
// as Get does, and 416 (range not satisfiable, expected while streaming) is final.
func (c *Client) RawGet(ctx context.Context, path string, headers map[string]string) (int, []byte, error) {
	for attempt := 0; ; attempt++ {
		resp, err := c.send(ctx, c.httpClient, http.MethodGet, path, nil, "", headers)
		if err == nil && (resp.status < 400 || resp.status == http.StatusRequestedRangeNotSatisfiable) {
			return resp.status, resp.body, nil
		}
		if wait, retry := c.retryWait(attempt, resp, err); retry {
			if serr := sleep(ctx, wait); serr != nil {
				return 0, nil, &NetworkError{Method: http.MethodGet, Err: serr}
			}
			continue
		}
		if err != nil {
			return 0, nil, err
		}
		return resp.status, resp.body, nil
	}
}

// DownloadTo streams a GET response body to w. It has no overall timeout but fails
// after 30 s without data. Failures before the body starts are retried like reads;
// once bytes reached w, nothing is retried.
func (c *Client) DownloadTo(ctx context.Context, path string, w io.Writer) error {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return &NetworkError{Method: http.MethodGet, Err: err}
		}
		dctx, cancel := context.WithCancel(ctx)
		req, err := http.NewRequestWithContext(dctx, http.MethodGet, c.host+path, nil)
		if err != nil {
			cancel()
			return &NetworkError{Method: http.MethodGet, Err: fmt.Errorf("creating request: %w", err)}
		}
		if c.authHeader != "" {
			req.Header.Set("Authorization", c.authHeader)
		}
		req.Header.Set("User-Agent", c.userAgent)
		timer := time.AfterFunc(downloadIdle, cancel)
		resp, err := c.DownloadClient.Do(req)
		if err != nil {
			timer.Stop()
			cancel()
			ne := classifyTransportError(http.MethodGet, false, err)
			if wait, retry := c.retryWait(attempt, nil, ne); retry {
				if serr := sleep(ctx, wait); serr != nil {
					return &NetworkError{Method: http.MethodGet, Err: serr}
				}
				continue
			}
			return ne
		}
		if resp.StatusCode < 400 {
			_, copyErr := io.Copy(w, &idleReader{r: resp.Body, timer: timer})
			_ = resp.Body.Close()
			timer.Stop()
			cancel()
			if copyErr != nil {
				return classifyTransportError(http.MethodGet, true, fmt.Errorf("reading response body: %w", copyErr))
			}
			return nil
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		timer.Stop()
		cancel()
		r := &response{status: resp.StatusCode, header: resp.Header, body: data}
		if wait, retry := c.retryWait(attempt, r, nil); retry {
			if serr := sleep(ctx, wait); serr != nil {
				return &NetworkError{Method: http.MethodGet, Err: serr}
			}
			continue
		}
		return parseError(resp.StatusCode, resp.Header, data)
	}
}

// idleReader resets the download's idle timer whenever data arrives.
type idleReader struct {
	r     io.Reader
	timer *time.Timer
}

func (i *idleReader) Read(p []byte) (int, error) {
	n, err := i.r.Read(p)
	if n > 0 {
		i.timer.Reset(downloadIdle)
	}
	return n, err
}

// uploadCreateFormFile builds a multipart file field; overridden in tests.
var uploadCreateFormFile = func(w *multipart.Writer, fieldName, fileName string) (io.Writer, error) {
	return w.CreateFormFile(fieldName, fileName)
}

// uploadCopyFile copies upload content into the multipart body; overridden in tests.
var uploadCopyFile = io.Copy

// Upload uploads a file as multipart/form-data. It is a write: sent once, never
// retried. fieldName is the form field; "file" works for most GitLab endpoints.
func (c *Client) Upload(ctx context.Context, path, fieldName, filePath string) ([]byte, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening file %s: %w", filePath, err)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := uploadCreateFormFile(mw, fieldName, filepath.Base(filePath))
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("creating form file: %w", err)
	}
	if _, err := uploadCopyFile(part, f); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("copying file content: %w", err)
	}
	_ = f.Close()
	_ = mw.Close()
	resp, err := c.do(ctx, http.MethodPost, path, buf.Bytes(), mw.FormDataContentType(), nil)
	if err != nil {
		return nil, err
	}
	return resp.body, nil
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
