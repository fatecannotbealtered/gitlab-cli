package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
)

// APIBasePath is the GitLab REST API v4 base path.
const APIBasePath = "/api/v4"

const (
	defaultMaxRetries = 3
	maxRateLimitWait  = 60 * time.Second
)

// defaultUserAgent returns GITLAB_CLI_USER_AGENT if set, otherwise a clear CLI UA.
func defaultUserAgent() string {
	if ua := strings.TrimSpace(os.Getenv("GITLAB_CLI_USER_AGENT")); ua != "" {
		return ua
	}
	return "gitlab-cli"
}

// EncodeProjectPath URL-encodes a "group/subgroup/project" path for use in
// /api/v4/projects/:id endpoints. A numeric project ID is left as-is.
func EncodeProjectPath(projectPath string) string {
	p := strings.TrimSpace(projectPath)
	// Numeric ID: leave untouched.
	if _, err := strconv.Atoi(p); err == nil {
		return p
	}
	return url.PathEscape(p)
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			// Re-apply Authorization on redirect so Bearer survives SSO hops,
			// but ONLY when staying on the same host. Cross-origin redirects
			// must NOT leak the bearer token.
			if len(via) > 0 {
				prev := via[len(via)-1]
				sameHost := strings.EqualFold(req.URL.Host, prev.URL.Host)
				if sameHost {
					if auth := prev.Header.Get("Authorization"); auth != "" {
						req.Header.Set("Authorization", auth)
					}
				}
				if ua := prev.Header.Get("User-Agent"); ua != "" {
					req.Header.Set("User-Agent", ua)
				}
			}
			return nil
		},
	}
}

func (c *Client) applyJSONHeaders(req *http.Request, jsonBody bool) {
	req.Header.Set("Authorization", c.authHeader)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent())
	if jsonBody {
		req.Header.Set("Content-Type", "application/json")
	}
}

// APIError represents an error returned by the GitLab API.
type APIError struct {
	StatusCode    int
	ErrorMessages []string
	Errors        map[string]string
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

// Pagination holds the page metadata returned in GitLab response headers.
type Pagination struct {
	Total      int    // X-Total (may be empty for very large lists)
	TotalPages int    // X-Total-Pages
	Page       int    // X-Page
	PerPage    int    // X-Per-Page
	NextPage   int    // X-Next-Page (0 if last)
	PrevPage   int    // X-Prev-Page (0 if first)
	Link       string // Link header
}

func extractPagination(h http.Header) Pagination {
	atoi := func(s string) int {
		n, _ := strconv.Atoi(strings.TrimSpace(s))
		return n
	}
	return Pagination{
		Total:      atoi(h.Get("X-Total")),
		TotalPages: atoi(h.Get("X-Total-Pages")),
		Page:       atoi(h.Get("X-Page")),
		PerPage:    atoi(h.Get("X-Per-Page")),
		NextPage:   atoi(h.Get("X-Next-Page")),
		PrevPage:   atoi(h.Get("X-Prev-Page")),
		Link:       h.Get("Link"),
	}
}

func maxRetries() int {
	if s := strings.TrimSpace(os.Getenv("GITLAB_CLI_MAX_RETRIES")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			return n
		}
	}
	return defaultMaxRetries
}

// rateLimitWait returns how long to wait after a 429 using Retry-After or RateLimit-Reset (capped).
func rateLimitWait(h http.Header) time.Duration {
	var wait time.Duration
	if ra := h.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
			wait = time.Duration(secs) * time.Second
		}
	}
	if wait == 0 {
		if reset := h.Get("RateLimit-Reset"); reset != "" {
			if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
				if d := time.Until(time.Unix(ts, 0)); d > 0 {
					wait = d
				}
			}
		}
	}
	if wait <= 0 {
		wait = retryBaseWait()
	}
	if wait > maxRateLimitWait {
		return maxRateLimitWait
	}
	return wait
}

func isRetryableNetworkErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "temporary failure") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "tls handshake timeout")
}

func serverErrorWait(attempt int) time.Duration {
	return time.Duration(1<<uint(attempt)) * retryBaseWait()
}

func retryBaseWait() time.Duration {
	if s := strings.TrimSpace(os.Getenv("GITLAB_CLI_RETRY_BASE_MS")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			return time.Duration(n) * time.Millisecond
		}
	}
	return time.Second
}

func shouldRetry(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= 500
}

func waitForRetry(ctx context.Context, attempt int, statusCode int, header http.Header) error {
	var wait time.Duration
	switch {
	case statusCode == http.StatusTooManyRequests:
		wait = rateLimitWait(header)
	case statusCode >= 500:
		wait = serverErrorWait(attempt)
	default:
		return nil
	}
	select {
	case <-time.After(wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Client wraps the GitLab REST API v4 HTTP client.
type Client struct {
	host       string
	authHeader string // "Bearer <PAT>"
	httpClient *http.Client

	// DownloadClient is used for large/binary downloads (artifacts, etc.).
	DownloadClient *http.Client

	// API groups — populated by NewClient. Subagents implement methods on each
	// API type in their own file (e.g. internal/api/mr.go). Always alphabetical.
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

// APIPath returns "/api/v4" + the relative resource path.
func (c *Client) APIPath(resource string) string {
	if !strings.HasPrefix(resource, "/") {
		resource = "/" + resource
	}
	return APIBasePath + resource
}

// NewClient creates a new API client from the given configuration.
func NewClient(cfg *config.Config) *Client {
	c := &Client{
		host:           strings.TrimRight(cfg.Host, "/"),
		authHeader:     "Bearer " + strings.TrimSpace(cfg.Token),
		httpClient:     newHTTPClient(30 * time.Second),
		DownloadClient: newHTTPClient(30 * time.Minute),
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

// Host returns the underlying GitLab host (without trailing slash).
func (c *Client) Host() string { return c.host }

// doWithRetry executes a request with retry logic.
//
//	429 → respect Retry-After / RateLimit-Reset
//	5xx → exponential backoff (1s, 2s, 4s)
//	4xx → no retry
func (c *Client) doWithRetry(ctx context.Context, method, path string, body any) ([]byte, int, http.Header, error) {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, 0, nil, err
		}

		var reqBody io.Reader
		if body != nil {
			encoded, err := json.Marshal(body)
			if err != nil {
				return nil, 0, nil, fmt.Errorf("encoding request body: %w", err)
			}
			reqBody = bytes.NewReader(encoded)
		}

		req, err := http.NewRequestWithContext(ctx, method, c.host+path, reqBody)
		if err != nil {
			return nil, 0, nil, fmt.Errorf("creating request: %w", err)
		}

		c.applyJSONHeaders(req, body != nil)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if isRetryableNetworkErr(err) && attempt < maxRetries() {
				continue
			}
			return nil, 0, nil, fmt.Errorf("executing request: %w", err)
		}

		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, resp.StatusCode, resp.Header, fmt.Errorf("reading response body: %w", readErr)
		}

		statusCode := resp.StatusCode
		header := resp.Header

		if statusCode < 400 {
			return data, statusCode, header, nil
		}

		if shouldRetry(statusCode) {
			if attempt >= maxRetries() {
				return nil, statusCode, header, c.parseError(statusCode, data)
			}
			if err := waitForRetry(ctx, attempt, statusCode, header); err != nil {
				return nil, 0, nil, err
			}
			continue
		}

		return nil, statusCode, header, c.parseError(statusCode, data)
	}
}

// parseError converts an HTTP status code and response body into an APIError.
//
// GitLab returns several error shapes:
//
//	{"message": "404 Project Not Found"}                          (string message)
//	{"message": {"name": ["has already been taken"]}}             (per-field validation)
//	{"error": "invalid_grant", "error_description": "..."}        (OAuth-style)
func (c *Client) parseError(statusCode int, data []byte) *APIError {
	apiErr := &APIError{StatusCode: statusCode}

	if len(data) > 0 {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err == nil {
			// Try "message"
			if msg, ok := raw["message"]; ok {
				// String form
				var s string
				if err := json.Unmarshal(msg, &s); err == nil && s != "" {
					apiErr.ErrorMessages = []string{s}
				} else {
					// Map form (validation errors)
					var m map[string][]string
					if err := json.Unmarshal(msg, &m); err == nil && len(m) > 0 {
						apiErr.Errors = make(map[string]string, len(m))
						for k, vs := range m {
							apiErr.Errors[k] = strings.Join(vs, "; ")
						}
					}
				}
			}
			// Try "error" + "error_description" (OAuth style)
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

	// Friendly fallbacks for known status codes.
	switch statusCode {
	case http.StatusUnauthorized:
		if len(apiErr.ErrorMessages) == 0 {
			apiErr.ErrorMessages = []string{"not logged in: run 'gitlab-cli auth login' or set GITLAB_TOKEN env var"}
		}
	case http.StatusForbidden:
		if len(apiErr.ErrorMessages) == 0 {
			apiErr.ErrorMessages = []string{"permission denied: check your PAT scope and project membership"}
		}
	case http.StatusNotFound:
		if len(apiErr.ErrorMessages) == 0 {
			apiErr.ErrorMessages = []string{"resource not found"}
		}
	}

	if len(apiErr.ErrorMessages) == 0 && len(apiErr.Errors) == 0 {
		apiErr.ErrorMessages = []string{fmt.Sprintf("unexpected status code %d", statusCode)}
	}
	return apiErr
}

// ─── Convenience verbs ──────────────────────────────────────────────────────

// Get sends a GET request. Returns body bytes only.
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	data, _, _, err := c.doWithRetry(ctx, http.MethodGet, path, nil)
	return data, err
}

// GetWithPagination sends a GET request and returns body + pagination metadata
// from the response headers. Useful for list endpoints.
func (c *Client) GetWithPagination(ctx context.Context, path string) ([]byte, Pagination, error) {
	data, _, header, err := c.doWithRetry(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, Pagination{}, err
	}
	return data, extractPagination(header), nil
}

// Post sends a POST request.
func (c *Client) Post(ctx context.Context, path string, body any) ([]byte, error) {
	data, _, _, err := c.doWithRetry(ctx, http.MethodPost, path, body)
	return data, err
}

// Put sends a PUT request.
func (c *Client) Put(ctx context.Context, path string, body any) ([]byte, error) {
	data, _, _, err := c.doWithRetry(ctx, http.MethodPut, path, body)
	return data, err
}

// Delete sends a DELETE request.
func (c *Client) Delete(ctx context.Context, path string) error {
	_, _, _, err := c.doWithRetry(ctx, http.MethodDelete, path, nil)
	return err
}

// DeleteWithBody sends a DELETE request with a JSON body. Required for GitLab
// endpoints that take parameters in the body on DELETE (e.g. repository file
// deletion takes branch + commit_message in the body).
func (c *Client) DeleteWithBody(ctx context.Context, path string, body any) error {
	_, _, _, err := c.doWithRetry(ctx, http.MethodDelete, path, body)
	return err
}

// DownloadTo streams a GET response body to w using DownloadClient (long timeout).
func (c *Client) DownloadTo(ctx context.Context, path string, w io.Writer) error {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.host+path, nil)
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Authorization", c.authHeader)
		req.Header.Set("User-Agent", defaultUserAgent())

		resp, err := c.DownloadClient.Do(req)
		if err != nil {
			return fmt.Errorf("executing request: %w", err)
		}

		if resp.StatusCode < 400 {
			_, copyErr := io.Copy(w, resp.Body)
			_ = resp.Body.Close()
			if copyErr != nil {
				return fmt.Errorf("reading response body: %w", copyErr)
			}
			return nil
		}

		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		statusCode := resp.StatusCode

		if shouldRetry(statusCode) {
			if attempt >= maxRetries() {
				return c.parseError(statusCode, data)
			}
			if err := waitForRetry(ctx, attempt, statusCode, resp.Header); err != nil {
				return err
			}
			continue
		}

		return c.parseError(statusCode, data)
	}
}

// Upload uploads a file using multipart/form-data.
// fieldName is the form field name; "file" works for most GitLab upload endpoints.

// uploadCreateFormFile builds a multipart file field; overridden in tests. // test hook
var uploadCreateFormFile = func(w *multipart.Writer, fieldName, fileName string) (io.Writer, error) {
	return w.CreateFormFile(fieldName, fileName)
}

// uploadCopyFile copies upload content into the multipart body; overridden in tests. // test hook
var uploadCopyFile = func(dst io.Writer, src io.Reader) (int64, error) {
	return io.Copy(dst, src)
}

func (c *Client) Upload(ctx context.Context, path, fieldName, filePath string) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		f, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("opening file %s: %w", filePath, err)
		}

		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		part, err := uploadCreateFormFile(w, fieldName, filepath.Base(filePath))
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("creating form file: %w", err)
		}
		if _, err := uploadCopyFile(part, f); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("copying file content: %w", err)
		}
		_ = f.Close()
		_ = w.Close()

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+path, &buf)
		if err != nil {
			return nil, fmt.Errorf("creating upload request: %w", err)
		}

		req.Header.Set("Authorization", c.authHeader)
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", defaultUserAgent())

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("executing upload request: %w", err)
		}

		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("reading upload response: %w", readErr)
		}

		statusCode := resp.StatusCode
		if statusCode < 400 {
			return data, nil
		}

		if shouldRetry(statusCode) {
			if attempt >= maxRetries() {
				return nil, c.parseError(statusCode, data)
			}
			if err := waitForRetry(ctx, attempt, statusCode, resp.Header); err != nil {
				return nil, err
			}
			continue
		}

		return nil, c.parseError(statusCode, data)
	}
}

// RawGet performs a GET with optional extra headers and returns (statusCode, body, error).
// It retries on 429/5xx but NOT on 416 (range not satisfiable — expected for streaming).
func (c *Client) RawGet(ctx context.Context, path string, headers map[string]string) (int, []byte, error) {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return 0, nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.host+path, nil)
		if err != nil {
			return 0, nil, fmt.Errorf("creating request: %w", err)
		}
		c.applyJSONHeaders(req, false)
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return 0, nil, fmt.Errorf("executing request: %w", err)
		}
		data, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return resp.StatusCode, nil, fmt.Errorf("reading response body: %w", readErr)
		}

		sc := resp.StatusCode
		if sc < 400 || sc == http.StatusRequestedRangeNotSatisfiable {
			return sc, data, nil
		}
		if shouldRetry(sc) {
			if attempt >= maxRetries() {
				return sc, data, nil
			}
			if err := waitForRetry(ctx, attempt, sc, resp.Header); err != nil {
				return 0, nil, err
			}
			continue
		}
		return sc, data, nil
	}
}
