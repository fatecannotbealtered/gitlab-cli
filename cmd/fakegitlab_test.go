package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGitLab is the GitLab every command test and the conformance suite run
// against: an httptest server that answers like GitLab's REST API v4, counts the
// write requests it receives (Harness.Writes) and fails on request (faults).
//
// Each command group adds its endpoints and seed data from its own _test.go file,
// in an init function:
//
//	func init() {
//		addFakeRoutes(fakeRoute{"GET /api/v4/projects/{project}/labels", fakeLabelList}, ...)
//		addFakeSeed(func(gl *fakeGitLab) { gl.state["labels"] = seedLabels(gl) })
//	}
//
// so no group edits this file. Routes are net/http ServeMux patterns; {project}
// matches GitLab's URL-encoded project path (acme%2Fweb) or a numeric ID, and
// gl.project(w, r) resolves it. Group state lives in gl.state under the group's key.
type fakeGitLab struct {
	t   testing.TB
	srv *httptest.Server
	URL string

	mu       sync.Mutex
	writes   int // requests other than GET and HEAD
	requests []fakeRequest
	faults   []*fakeFault
	clock    int

	// Seed data, rebuilt by Reset.
	state    map[string]any
	users    map[string]fakeUser // by token
	people   []fakeUser          // every user, for /users lookups
	projects []*fakeProject
	nextID   int
}

// fakeRequest is one request as the fake received it.
type fakeRequest struct {
	Method, Path, Query, UserAgent string
	Body                           map[string]any
}

type fakeUser struct {
	ID             int
	Username, Name string
}

type fakeProject struct {
	ID   int
	Path string
}

// fakeRoute is one endpoint of the fake: a ServeMux pattern and its handler.
type fakeRoute struct {
	Pattern string
	Handle  func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request)
}

var (
	fakeRoutes []fakeRoute
	fakeSeeds  []func(gl *fakeGitLab)
	// fakeProjectDetails, when a group sets it (project_test.go), adds a project's
	// other fields (name, visibility, default branch, ...) to the shared
	// GET /projects/{project}, which answers the ID, path and URL by itself.
	fakeProjectDetails func(gl *fakeGitLab, p *fakeProject) map[string]any
)

// addFakeRoutes registers endpoints (from a group's init function).
func addFakeRoutes(routes ...fakeRoute) { fakeRoutes = append(fakeRoutes, routes...) }

// addFakeSeed registers a function that (re)builds a group's seed data.
func addFakeSeed(seed func(gl *fakeGitLab)) { fakeSeeds = append(fakeSeeds, seed) }

// The fake's accounts. testToken signs in as carol, the user every test acts as.
const (
	testToken = "glpat-test-carol-0001"
	daveToken = "glpat-test-dave-0002"
)

func init() {
	addFakeRoutes(
		fakeRoute{"GET /api/v4/user", fakeCurrentUser},
		fakeRoute{"GET /api/v4/version", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			fakeJSON(w, 200, map[string]any{"version": "18.4.0", "revision": "fake"})
		}},
		fakeRoute{"GET /api/v4/personal_access_tokens/self", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			fakeJSON(w, 200, map[string]any{"id": 1, "name": "agent token", "scopes": []string{"api"}, "active": true,
				"revoked": false, "expires_at": "2027-01-31"})
		}},
		fakeRoute{"GET /api/v4/projects/{project}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			if p := gl.project(w, r); p != nil {
				out := map[string]any{"id": p.ID, "path_with_namespace": p.Path, "web_url": gl.URL + "/" + p.Path}
				if fakeProjectDetails != nil {
					for k, v := range fakeProjectDetails(gl, p) {
						out[k] = v
					}
				}
				fakeJSON(w, 200, out)
			}
		}},
		// Users are shared by every group (assignees, reviewers, members): the fake
		// knows carol, dave and erin. GET /users filters by username or search.
		fakeRoute{"GET /api/v4/users", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			var out []map[string]any
			for _, u := range gl.people {
				switch {
				case q.Get("username") != "" && !strings.EqualFold(q.Get("username"), u.Username):
				case q.Get("search") != "" && !strings.Contains(strings.ToLower(u.Username+" "+u.Name), strings.ToLower(q.Get("search"))):
				default:
					out = append(out, u.json(gl))
				}
			}
			fakePage(w, r, out)
		}},
		fakeRoute{"GET /api/v4/users/{id}", func(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
			for _, u := range gl.people {
				if strconv.Itoa(u.ID) == r.PathValue("id") {
					fakeJSON(w, 200, u.json(gl))
					return
				}
			}
			fakeJSON(w, 404, map[string]any{"message": "404 User Not Found"})
		}},
	)
}

func (u fakeUser) json(gl *fakeGitLab) map[string]any {
	return map[string]any{"id": u.ID, "username": u.Username, "name": u.Name, "state": "active",
		"web_url": gl.URL + "/" + u.Username}
}

func fakeCurrentUser(gl *fakeGitLab, w http.ResponseWriter, r *http.Request) {
	u := gl.users[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	fakeJSON(w, 200, map[string]any{"id": u.ID, "username": u.Username, "name": u.Name, "state": "active"})
}

// newFakeGitLab starts the fake with its seed data; it stops with the test.
func newFakeGitLab(t testing.TB) *fakeGitLab {
	gl := &fakeGitLab{t: t}
	mux := http.NewServeMux()
	for _, rt := range fakeRoutes {
		rt := rt
		mux.HandleFunc(rt.Pattern, func(w http.ResponseWriter, r *http.Request) { rt.Handle(gl, w, r) })
	}
	gl.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { gl.serve(mux, w, r) }))
	// One connection per request, so a fake that stops listening refuses the next
	// request at once (the "failure before sending" rows).
	gl.srv.Config.SetKeepAlivesEnabled(false)
	gl.srv.Start()
	gl.URL = gl.srv.URL
	gl.Reset()
	t.Cleanup(gl.srv.Close)
	return gl
}

// Reset restores the seed data and clears the counters and faults (Harness.Reset).
func (gl *fakeGitLab) Reset() {
	gl.mu.Lock()
	gl.writes, gl.requests, gl.faults, gl.clock = 0, nil, nil, 0
	gl.state = map[string]any{}
	gl.people = []fakeUser{{ID: 7, Username: "carol", Name: "Carol Danvers"}, {ID: 8, Username: "dave", Name: "Dave Lister"},
		{ID: 9, Username: "erin", Name: "Erin Hannon"}}
	gl.users = map[string]fakeUser{testToken: gl.people[0], daveToken: gl.people[1]}
	gl.nextID = 100
	gl.projects = []*fakeProject{{ID: 1, Path: "acme/web"}, {ID: 2, Path: "acme/empty"}}
	for _, seed := range fakeSeeds {
		seed(gl) // under the lock, like the handlers
	}
	gl.mu.Unlock()
}

// Writes is the number of write requests GitLab received (Harness.Writes).
func (gl *fakeGitLab) Writes() int {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	return gl.writes
}

// Requests returns every request received so far.
func (gl *fakeGitLab) Requests() []fakeRequest {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	return append([]fakeRequest(nil), gl.requests...)
}

// lastWrite is the last request other than GET.
func (gl *fakeGitLab) lastWrite() (fakeRequest, bool) {
	reqs := gl.Requests()
	for i := len(reqs) - 1; i >= 0; i-- {
		if reqs[i].Method != http.MethodGet {
			return reqs[i], true
		}
	}
	return fakeRequest{}, false
}

// id allocates a global ID.
func (gl *fakeGitLab) id() int {
	gl.nextID++
	return gl.nextID
}

// now is a deterministic clock: each call is one second later.
func (gl *fakeGitLab) now() string {
	gl.clock++
	return time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC).Add(time.Duration(gl.clock) * time.Second).Format("2006-01-02T15:04:05.000Z")
}

// fakeFault makes the fake fail matching requests.
type fakeFault struct {
	Method string // "" every request, "WRITE" every non-GET, or one method
	Path   string // a substring of the escaped path; "" every path
	Status int    // answer with this status and Body (a GitLab error JSON by default)
	Header map[string]string
	Body   string
	Delay  time.Duration // wait before answering (timeouts)
	Reset  bool          // read the request, then drop the connection without an answer
	// DownAfter answers the request normally, then stops listening: every later
	// request is refused before anything is sent.
	DownAfter bool
	Times     int // apply to this many requests; 0 means every one
}

// fail adds a fault; faults match in the order they were added.
func (gl *fakeGitLab) fail(f fakeFault) {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	gl.faults = append(gl.faults, &f)
}

// failAfterReads lets the next n GET requests through, then stops listening, so
// the next request (a write's Apply) is refused before anything is sent.
func (gl *fakeGitLab) failAfterReads(n int) {
	for i := 1; i < n; i++ {
		gl.fail(fakeFault{Method: http.MethodGet, Times: 1})
	}
	gl.fail(fakeFault{Method: http.MethodGet, DownAfter: true, Times: 1})
}

// down stops the fake listening: every request is refused (E_NETWORK).
func (gl *fakeGitLab) down() { _ = gl.srv.Listener.Close() }

func (gl *fakeGitLab) matchFault(r *http.Request) *fakeFault {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	for i, f := range gl.faults {
		method := f.Method == "" || f.Method == r.Method || (f.Method == "WRITE" && r.Method != http.MethodGet)
		if !method || (f.Path != "" && !strings.Contains(r.URL.EscapedPath(), f.Path)) {
			continue
		}
		if f.Times > 0 {
			f.Times--
			if f.Times == 0 {
				gl.faults = append(gl.faults[:i:i], gl.faults[i+1:]...)
			}
		}
		return f
	}
	return nil
}

func (gl *fakeGitLab) serve(mux *http.ServeMux, w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(raw))
	req := fakeRequest{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery, UserAgent: r.UserAgent()}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req.Body)
	}
	gl.mu.Lock()
	gl.requests = append(gl.requests, req)
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		gl.writes++
	}
	gl.mu.Unlock()
	if f := gl.matchFault(r); f != nil {
		if f.Delay > 0 {
			select {
			case <-time.After(f.Delay):
			case <-r.Context().Done():
				return
			}
		}
		switch {
		case f.Reset:
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				_ = conn.Close()
			}
			return
		case f.Status != 0:
			for k, v := range f.Header {
				w.Header().Set(k, v)
			}
			body := f.Body
			if body == "" {
				body = fmt.Sprintf(`{"message":"%d %s"}`, f.Status, http.StatusText(f.Status))
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.Status)
			_, _ = io.WriteString(w, body)
			return
		case f.DownAfter:
			defer gl.down()
		}
	}
	// Handlers run one at a time under the lock, so tests can read the seed data
	// with gl.locked without racing a request.
	gl.mu.Lock()
	defer gl.mu.Unlock()
	if _, ok := gl.users[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]; !ok {
		fakeJSON(w, 401, map[string]any{"message": "401 Unauthorized"})
		return
	}
	if _, pattern := mux.Handler(r); pattern == "" {
		fakeJSON(w, 404, map[string]any{"message": "404 Not Found"})
		return
	}
	mux.ServeHTTP(w, r)
}

// locked runs fn with the fake's lock held: read or change seed data from a test.
func (gl *fakeGitLab) locked(fn func()) {
	gl.mu.Lock()
	defer gl.mu.Unlock()
	fn()
}

// project resolves the {project} path value (a path or a numeric ID); it answers
// 404 itself and returns nil when there is no such project.
func (gl *fakeGitLab) project(w http.ResponseWriter, r *http.Request) *fakeProject {
	key := r.PathValue("project")
	for _, p := range gl.projects {
		if p.Path == key || strconv.Itoa(p.ID) == key {
			return p
		}
	}
	fakeJSON(w, 404, map[string]any{"message": "404 Project Not Found"})
	return nil
}

// body is the request's JSON body.
func fakeBody(r *http.Request) map[string]any {
	var m map[string]any
	_ = json.NewDecoder(r.Body).Decode(&m)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

func fakeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// fakePage answers one page of items the way GitLab pages: page and per_page in the
// query, X-Page, X-Per-Page, X-Next-Page, X-Total and X-Total-Pages in the headers.
func fakePage[T any](w http.ResponseWriter, r *http.Request, items []T) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	if per < 1 {
		per = 20
	}
	start := min((page-1)*per, len(items))
	end := min(start+per, len(items))
	next := ""
	if end < len(items) {
		next = strconv.Itoa(page + 1)
	}
	w.Header().Set("X-Page", strconv.Itoa(page))
	w.Header().Set("X-Per-Page", strconv.Itoa(per))
	w.Header().Set("X-Next-Page", next)
	w.Header().Set("X-Total", strconv.Itoa(len(items)))
	w.Header().Set("X-Total-Pages", strconv.Itoa((len(items)+per-1)/per))
	out := items[start:end]
	if out == nil {
		out = []T{}
	}
	fakeJSON(w, 200, out)
}

// sortedBy returns the keys of m in order.
func sortedBy[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
