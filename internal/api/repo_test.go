package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestRepo_GetFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.Contains(r.URL.Path, "/repository/files/") {
			t.Errorf("request = %s %q", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("ref") != "main" {
			t.Errorf("ref = %q, want main", r.URL.Query().Get("ref"))
		}
		_, _ = w.Write([]byte(`{"file_name":"README.md","file_path":"README.md","size":5,"encoding":"base64","content":"aGVsbG8=","ref":"main","blob_id":"abc","commit_id":"def","last_commit_id":"ghi"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	f, err := c.Repos.GetFile(testCtx, "group/proj", "README.md", "main")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	b, err := f.Decoded()
	if f.FileName != "README.md" || f.LastCommitID != "ghi" || err != nil || string(b) != "hello" {
		t.Errorf("unexpected file: %+v %q %v", f, b, err)
	}
	f.Encoding, f.Content = "text", "plain"
	if b, _ := f.Decoded(); string(b) != "plain" {
		t.Errorf("text content: %q", b)
	}
	f.Encoding, f.Content = "base64", "!!"
	if _, err := f.Decoded(); err == nil {
		t.Error("bad base64 must fail")
	}
}

func TestRepo_GetFile_DefaultRefAndEscaping(t *testing.T) {
	var gotURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI = r.RequestURI
		if r.URL.Query().Get("ref") != "HEAD" {
			t.Errorf("ref = %q, want HEAD", r.URL.Query().Get("ref"))
		}
		_, _ = w.Write([]byte(`{"file_name":"bar.txt","file_path":"foo/bar.txt","encoding":"base64","content":""}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, err := c.Repos.GetFile(testCtx, "1", "foo/bar.txt", ""); err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	// The slash in the file path must be encoded as %2F in the raw URI.
	if !strings.Contains(gotURI, "foo%2Fbar.txt") {
		t.Errorf("RequestURI %q should contain foo%%2Fbar.txt", gotURI)
	}
}

func TestRepo_FileWrites(t *testing.T) {
	var method, rawBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		b, _ := io.ReadAll(r.Body)
		rawBody = string(b)
		if r.URL.Query().Get("branch") != "" {
			t.Errorf("branch belongs in the body, not the query: %q", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"file_path":"f.txt","branch":"main"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if err := c.Repos.CreateFile(testCtx, "1", "f.txt", FileWriteBody{Branch: "main", Content: "hi", CommitMessage: "add"}); err != nil || method != http.MethodPost {
		t.Fatalf("CreateFile: %v %s", err, method)
	}
	if err := c.Repos.UpdateFile(testCtx, "1", "f.txt", FileWriteBody{Branch: "main", Content: "x", CommitMessage: "upd", LastCommitID: "abc"}); err != nil ||
		method != http.MethodPut || !strings.Contains(rawBody, `"last_commit_id":"abc"`) {
		t.Fatalf("UpdateFile: %v %s %s", err, method, rawBody)
	}
	if err := c.Repos.DeleteFile(testCtx, "1", "f.txt", "main", "delete file", "abc"); err != nil || method != http.MethodDelete {
		t.Fatalf("DeleteFile: %v %s", err, method)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(rawBody), &payload); err != nil || payload["branch"] != "main" ||
		payload["commit_message"] != "delete file" || payload["last_commit_id"] != "abc" {
		t.Errorf("delete body: %s", rawBody)
	}
}

func TestRepo_Branches(t *testing.T) {
	var gotURI, method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURI, method = r.RequestURI, r.Method
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/repository/branches"):
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[{"name":"main","default":true},{"name":"dev","default":false}]`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"name":"feature","commit":{"id":"abc"}}`))
		default:
			_, _ = w.Write([]byte(`{"name":"release/1.0","protected":true,"commit":{"id":"abc","title":"t"}}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	branches, pag, err := c.Repos.ListBranchesPage(testCtx, "1", BranchListOpts{Search: "ma", Page: 1, PerPage: 2})
	if err != nil || len(branches) != 2 || branches[0].Name != "main" || pag.NextPage != 2 {
		t.Fatalf("ListBranchesPage: %v %+v %+v", err, branches, pag)
	}
	if !strings.Contains(gotURI, "search=ma") || !strings.Contains(gotURI, "per_page=2") {
		t.Errorf("list query: %s", gotURI)
	}
	b, err := c.Repos.GetBranch(testCtx, "1", "release/1.0")
	if err != nil || !b.Protected || b.Commit.ID != "abc" || !strings.Contains(gotURI, "/branches/release%2F1.0") {
		t.Fatalf("GetBranch: %v %+v %s", err, b, gotURI)
	}
	b, err = c.Repos.CreateBranch(testCtx, "1", "feature", "abc")
	if err != nil || b.Name != "feature" || method != http.MethodPost || !strings.Contains(gotURI, "branch=feature&ref=abc") {
		t.Fatalf("CreateBranch: %v %+v %s", err, b, gotURI)
	}
	if err := c.Repos.DeleteBranch(testCtx, "1", "feat/x"); err != nil || !strings.HasSuffix(gotURI, "/branches/feat%2Fx") {
		t.Fatalf("DeleteBranch: %v %s", err, gotURI)
	}
}

func TestRepo_GetTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/repository/tags/v1.0") {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"name":"v1.0","target":"abc","commit":{"id":"abc"}}`))
	}))
	defer srv.Close()

	tag, err := newTestClient(srv.URL).Repos.GetTag(testCtx, "1", "v1.0")
	if err != nil || tag.Name != "v1.0" || tag.Commit.ID != "abc" {
		t.Fatalf("GetTag: %v %+v", err, tag)
	}
}

func TestRepo_Commits(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		switch {
		case strings.HasSuffix(r.URL.Path, "/repository/commits"):
			w.Header().Set("X-Total", "1")
			_, _ = w.Write([]byte(`[{"id":"abc123","short_id":"abc","title":"init","author_name":"Alice","stats":{"additions":3,"deletions":1,"total":4}}]`))
		default:
			_, _ = w.Write([]byte(`{"id":"abc123","short_id":"abc","title":"init","message":"init\n\nbody","parent_ids":["p1"]}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	opts := CommitListOpts{RefName: "main", Since: "2024-01-01T00:00:00Z", Until: "2024-02-01T00:00:00Z", Path: "src/",
		Author: "alice", WithStats: true, AllBranches: true, Page: 2, PerPage: 5}
	commits, pag, err := c.Repos.ListCommitsPage(testCtx, "1", opts)
	if err != nil || len(commits) != 1 || commits[0].Stats.Total != 4 || pag.Total != 1 {
		t.Fatalf("ListCommitsPage: %v %+v %+v", err, commits, pag)
	}
	for _, want := range []string{"ref_name=main", "since=2024-01-01", "until=2024-02-01", "path=src%2F", "author=alice",
		"with_stats=true", "all=true", "page=2", "per_page=5"} {
		if !strings.Contains(query, want) {
			t.Errorf("query %q lacks %s", query, want)
		}
	}
	if all, err := c.Repos.ListCommits(testCtx, "1", &CommitListOpts{RefName: "main", Limit: 1}); err != nil || len(all) != 1 {
		t.Fatalf("ListCommits: %v %+v", err, all)
	}
	commit, err := c.Repos.GetCommit(testCtx, "1", "abc123")
	if err != nil || commit.ID != "abc123" || commit.ParentIDs[0] != "p1" {
		t.Fatalf("GetCommit: %v %+v", err, commit)
	}
}

func TestRepo_CreateCommit(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/repository/commits") {
			t.Errorf("request = %s %q", r.Method, r.URL.Path)
		}
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"abc","short_id":"abc1","title":"sync"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	commit, err := c.Repos.CreateCommit(testCtx, "1", CommitCreateOpts{Branch: "main", CommitMessage: "sync",
		Actions: []CommitAction{{Action: "create", FilePath: "a.txt", Content: "hi"}, {Action: "delete", FilePath: "old.txt", LastCommitID: "c1"}}})
	if err != nil || commit.ShortID != "abc1" {
		t.Fatalf("CreateCommit: %v %+v", err, commit)
	}
	for _, want := range []string{`"actions"`, `"create"`, `"a.txt"`, `"delete"`, `"old.txt"`, `"last_commit_id":"c1"`} {
		if !strings.Contains(gotBody, want) {
			t.Errorf("request body missing %q, got: %s", want, gotBody)
		}
	}
}

// TestRepo_ListCommitDiffReadsEveryPage: GitLab pages a commit's diff (20 files by
// default), so the whole diff takes several requests; maxFiles caps it.
func TestRepo_ListCommitDiffReadsEveryPage(t *testing.T) {
	const files = 250
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		var out []map[string]any
		for i := (page - 1) * per; i < min(page*per, files); i++ {
			out = append(out, map[string]any{"new_path": "f" + strconv.Itoa(i), "old_path": "f" + strconv.Itoa(i), "diff": "+x\n"})
		}
		if page*per < files {
			w.Header().Set("X-Next-Page", strconv.Itoa(page+1))
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	all, more, err := c.Repos.ListCommitDiff(testCtx, "1", "abc", 1000)
	if err != nil || len(all) != files || more || all[249].NewPath != "f249" {
		t.Fatalf("whole diff: %v %d %v", err, len(all), more)
	}
	capped, more, err := c.Repos.ListCommitDiff(testCtx, "1", "abc", 150)
	if err != nil || len(capped) != 150 || !more {
		t.Fatalf("capped diff: %v %d %v", err, len(capped), more)
	}
}

func TestRepo_ListTreePage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("recursive") != "true" || q.Get("path") != "src" || q.Get("ref") != "main" || q.Get("page") != "1" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[{"id":"x","name":"app.go","type":"blob","path":"src/app.go","mode":"100644"}]`))
	}))
	defer srv.Close()

	entries, _, err := newTestClient(srv.URL).Repos.ListTreePage(testCtx, "1", TreeOpts{Recursive: true, Path: "src", Ref: "main", Page: 1, PerPage: 20})
	if err != nil || len(entries) != 1 || entries[0].Path != "src/app.go" {
		t.Fatalf("ListTreePage: %v %+v", err, entries)
	}
}

// TestRepo_ProjectPathsForTheFanOut: group projects include subgroups and leave
// archived ones out; the member list is the token's memberships; both stop at max.
func TestRepo_ProjectPathsForTheFanOut(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.EscapedPath()+"?"+r.URL.RawQuery)
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 1 {
			w.Header().Set("X-Next-Page", "2")
			_, _ = w.Write([]byte(`[{"path_with_namespace":"acme/a"},{"path_with_namespace":"acme/b"}]`))
			return
		}
		_, _ = w.Write([]byte(`[{"path_with_namespace":"acme/sub/c"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	paths, more, err := c.Repos.GroupProjectPaths(testCtx, "acme", 10)
	if err != nil || len(paths) != 3 || more || paths[2] != "acme/sub/c" {
		t.Fatalf("GroupProjectPaths: %v %v %v", err, paths, more)
	}
	if q := queries[0]; !strings.Contains(q, "/groups/acme/projects") || !strings.Contains(q, "include_subgroups=true") ||
		!strings.Contains(q, "archived=false") {
		t.Errorf("group query: %s", q)
	}
	queries = nil
	paths, more, err = c.Repos.MemberProjectPaths(testCtx, 2)
	if err != nil || len(paths) != 2 || !more || len(queries) != 1 || !strings.Contains(queries[0], "membership=true") {
		t.Fatalf("MemberProjectPaths: %v %v %v %v", err, paths, more, queries)
	}
}

func TestRepo_ParseErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	for want, call := range map[string]func() error{
		"parsing repo file": func() error { _, err := c.Repos.GetFile(testCtx, "1", "f.txt", "main"); return err },
		"parsing branches":  func() error { _, _, err := c.Repos.ListBranchesPage(testCtx, "1", BranchListOpts{}); return err },
		"parsing branch":    func() error { _, err := c.Repos.GetBranch(testCtx, "1", "main"); return err },
		"parsing tag":       func() error { _, err := c.Repos.GetTag(testCtx, "1", "v1"); return err },
		"parsing commits":   func() error { _, _, err := c.Repos.ListCommitsPage(testCtx, "1", CommitListOpts{}); return err },
		"parsing commit":    func() error { _, err := c.Repos.GetCommit(testCtx, "1", "abc"); return err },
		"parsing commit diff": func() error {
			_, _, err := c.Repos.ListCommitDiff(testCtx, "1", "abc", 10)
			return err
		},
		"parsing created commit": func() error { _, err := c.Repos.CreateCommit(testCtx, "1", CommitCreateOpts{}); return err },
		"parsing tree":           func() error { _, _, err := c.Repos.ListTreePage(testCtx, "1", TreeOpts{}); return err },
		"parsing projects":       func() error { _, _, err := c.Repos.GroupProjectPaths(testCtx, "acme", 10); return err },
	} {
		if err := call(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v", want, err)
		}
	}
	if _, err := c.Repos.CreateBranch(testCtx, "1", "feat/x", "main"); err == nil || !strings.Contains(err.Error(), "parsing branch") {
		t.Errorf("CreateBranch: %v", err)
	}
	if _, err := c.Repos.ListCommits(testCtx, "1", nil); err == nil || !strings.Contains(err.Error(), "parsing commits") {
		t.Errorf("ListCommits: %v", err)
	}
}
