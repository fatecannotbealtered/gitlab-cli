package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRepo_GetFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/repository/files/") {
			t.Errorf("path = %q, want /repository/files/", r.URL.Path)
		}
		if r.URL.Query().Get("ref") != "main" {
			t.Errorf("ref = %q, want main", r.URL.Query().Get("ref"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"file_name":"README.md","file_path":"README.md","size":42,"encoding":"base64","content":"aGVsbG8=","ref":"main","blob_id":"abc","commit_id":"def","last_commit_id":"ghi"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	f, err := c.Repos.GetFile(testCtx, "group/proj", "README.md", "main")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	if f.FileName != "README.md" || f.Encoding != "base64" {
		t.Errorf("unexpected file: %+v", f)
	}
}

func TestRepo_GetFile_DefaultRef(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ref") != "HEAD" {
			t.Errorf("ref = %q, want HEAD", r.URL.Query().Get("ref"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"file_name":"f.txt","file_path":"f.txt","encoding":"base64","content":""}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Repos.GetFile(testCtx, "1", "f.txt", "")
	if err != nil {
		t.Fatalf("GetFile default ref: %v", err)
	}
}

func TestRepo_GetFileRaw(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/raw") {
			t.Errorf("path should end with /raw, got %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("raw content"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	data, err := c.Repos.GetFileRaw(testCtx, "1", "README.md", "main")
	if err != nil {
		t.Fatalf("GetFileRaw: %v", err)
	}
	if string(data) != "raw content" {
		t.Errorf("data = %q, want 'raw content'", string(data))
	}
}

func TestRepo_CreateFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"file_path":"new.txt","branch":"main"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	err := c.Repos.CreateFile(testCtx, "1", "new.txt", FileWriteBody{
		Branch:        "main",
		Content:       "hello",
		CommitMessage: "add file",
	})
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
}

func TestRepo_UpdateFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %q, want PUT", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"file_path":"f.txt","branch":"main"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	err := c.Repos.UpdateFile(testCtx, "1", "f.txt", FileWriteBody{
		Branch:        "main",
		Content:       "updated",
		CommitMessage: "update file",
	})
	if err != nil {
		t.Fatalf("UpdateFile: %v", err)
	}
}

func TestRepo_DeleteFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("body not JSON: %v (raw=%s)", err, string(body))
		}
		if payload["branch"] != "main" {
			t.Errorf("body.branch = %q, want main", payload["branch"])
		}
		if payload["commit_message"] != "delete file" {
			t.Errorf("body.commit_message = %q, want 'delete file'", payload["commit_message"])
		}
		// Query params should NOT carry the values now.
		if r.URL.Query().Get("branch") != "" {
			t.Errorf("branch should be in body, not query (got query=%q)", r.URL.Query().Get("branch"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	err := c.Repos.DeleteFile(testCtx, "1", "f.txt", "main", "delete file")
	if err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
}

func TestRepo_ListBranches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/repository/branches") {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"name":"main","default":true},{"name":"dev","default":false}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	branches, err := c.Repos.ListBranches(testCtx, "1", &BranchListOpts{Limit: 10})
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	if len(branches) != 2 || branches[0].Name != "main" {
		t.Errorf("unexpected branches: %+v", branches)
	}
}

func TestRepo_CreateBranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Query().Get("branch") != "feature" {
			t.Errorf("branch = %q, want feature", r.URL.Query().Get("branch"))
		}
		if r.URL.Query().Get("ref") != "main" {
			t.Errorf("ref = %q, want main", r.URL.Query().Get("ref"))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"feature","default":false}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	b, err := c.Repos.CreateBranch(testCtx, "1", "feature", "main")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if b.Name != "feature" {
		t.Errorf("name = %q, want feature", b.Name)
	}
}

func TestRepo_DeleteBranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/feature") {
			t.Errorf("path = %q, want suffix /feature", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if err := c.Repos.DeleteBranch(testCtx, "1", "feature"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
}

func TestRepo_ListCommits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/repository/commits") {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"abc123","short_id":"abc","title":"init","author_name":"Alice"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	commits, err := c.Repos.ListCommits(testCtx, "1", &CommitListOpts{RefName: "main", Limit: 5})
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	if len(commits) != 1 || commits[0].ShortID != "abc" {
		t.Errorf("unexpected commits: %+v", commits)
	}
}

func TestRepo_GetCommit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/abc123") {
			t.Errorf("path = %q, want suffix /abc123", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"abc123","short_id":"abc","title":"init","author_name":"Alice"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	commit, err := c.Repos.GetCommit(testCtx, "1", "abc123")
	if err != nil {
		t.Fatalf("GetCommit: %v", err)
	}
	if commit.ID != "abc123" {
		t.Errorf("id = %q, want abc123", commit.ID)
	}
}

func TestRepo_ListTree(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/repository/tree") {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":"x","name":"README.md","type":"blob","path":"README.md","mode":"100644"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	entries, err := c.Repos.ListTree(testCtx, "1", &TreeOpts{Ref: "main"})
	if err != nil {
		t.Fatalf("ListTree: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "README.md" {
		t.Errorf("unexpected entries: %+v", entries)
	}
}

func TestRepo_FilePath_URLEncoded(t *testing.T) {
	var gotURI string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use RequestURI which preserves the raw (un-decoded) path
		gotURI = r.RequestURI
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"file_name":"bar.txt","file_path":"foo/bar.txt","encoding":"base64","content":""}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Repos.GetFile(testCtx, "1", "foo/bar.txt", "main")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	// The slash in the file path must be encoded as %2F in the raw URI
	if !strings.Contains(gotURI, "foo%2Fbar.txt") {
		t.Errorf("RequestURI %q should contain foo%%2Fbar.txt (URL-encoded slash)", gotURI)
	}
}
