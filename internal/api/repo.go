package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// RepoAPI wraps repository-related API calls (files, branches, commits, tree).
//
// Endpoint references:
//
//	https://docs.gitlab.com/api/repository_files/
//	https://docs.gitlab.com/api/branches/
//	https://docs.gitlab.com/api/commits/
//	https://docs.gitlab.com/api/repositories/
type RepoAPI struct{ client *Client }

// ─── DTOs ────────────────────────────────────────────────────────────────────

// RepoFile is returned by GET /projects/:id/repository/files/:path
type RepoFile struct {
	FileName      string `json:"file_name"`
	FilePath      string `json:"file_path"`
	Size          int    `json:"size"`
	Encoding      string `json:"encoding"`
	Content       string `json:"content"`
	ContentSHA256 string `json:"content_sha256"`
	Ref           string `json:"ref"`
	BlobID        string `json:"blob_id"`
	CommitID      string `json:"commit_id"`
	LastCommitID  string `json:"last_commit_id"`
}

// Branch is returned by GET /projects/:id/repository/branches
type Branch struct {
	Name               string  `json:"name"`
	Merged             bool    `json:"merged"`
	Protected          bool    `json:"protected"`
	Default            bool    `json:"default"`
	DevelopersCanPush  bool    `json:"developers_can_push"`
	DevelopersCanMerge bool    `json:"developers_can_merge"`
	CanPush            bool    `json:"can_push"`
	WebURL             string  `json:"web_url"`
	Commit             *Commit `json:"commit"`
}

// Commit is returned by GET /projects/:id/repository/commits
type Commit struct {
	ID             string   `json:"id"`
	ShortID        string   `json:"short_id"`
	Title          string   `json:"title"`
	Message        string   `json:"message"`
	AuthorName     string   `json:"author_name"`
	AuthorEmail    string   `json:"author_email"`
	AuthoredDate   string   `json:"authored_date"`
	CommitterName  string   `json:"committer_name"`
	CommitterEmail string   `json:"committer_email"`
	CommittedDate  string   `json:"committed_date"`
	WebURL         string   `json:"web_url"`
	ParentIDs      []string `json:"parent_ids"`
	// Stats is populated only when with_stats=true is requested (CommitListOpts.WithStats);
	// GitLab omits it otherwise, so a nil Stats means "not requested", not "zero changes".
	Stats *CommitStats `json:"stats,omitempty"`
}

// CommitStats carries per-commit line change counts, returned by GitLab when
// with_stats=true. Lets an agent size a commit (and a whole author/time-range
// query) without fetching any diff.
type CommitStats struct {
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
	Total     int `json:"total"`
}

// CommitDiffFile is one file entry of GET /repository/commits/:sha/diff. GitLab
// returns the unified patch in Diff plus rename/create/delete flags; it does NOT
// return per-file line counts, so Additions/Deletions are computed client-side
// from the patch hunks (see countDiffLines) to support a cheap name+stat
// projection via --fields without shipping the patch text.
type CommitDiffFile struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	NewFile     bool   `json:"new_file"`
	DeletedFile bool   `json:"deleted_file"`
	RenamedFile bool   `json:"renamed_file"`
	Diff        string `json:"diff"`
}

// TreeEntry is returned by GET /projects/:id/repository/tree
type TreeEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "blob" or "tree"
	Path string `json:"path"`
	Mode string `json:"mode"`
}

// ─── List opts ───────────────────────────────────────────────────────────────

// BranchListOpts are options for listing branches.
type BranchListOpts struct {
	Search string
	Limit  int
}

// CommitListOpts are options for listing commits.
type CommitListOpts struct {
	RefName string
	Since   string
	Until   string
	Path    string
	Limit   int
	// Author filters commits server-side by author (GitLab 15.10+). On older
	// instances GitLab ignores the param and returns the unfiltered list.
	Author string
	// WithStats requests per-commit line counts (sets with_stats=true).
	WithStats bool
	// AllBranches lists commits across every ref (sets all=true), not just the
	// default/selected branch.
	AllBranches bool
}

// TreeOpts are options for listing tree entries.
type TreeOpts struct {
	Path      string
	Ref       string
	Recursive bool
	Limit     int
}

// ─── File methods ─────────────────────────────────────────────────────────────

// GetFile returns file metadata + base64 content.
func (a *RepoAPI) GetFile(ctx context.Context, projectID, filePath, ref string) (*RepoFile, error) {
	if ref == "" {
		ref = "HEAD"
	}
	path := a.client.APIPath("/projects/"+EncodeProjectPath(projectID)+"/repository/files/"+url.PathEscape(filePath)) +
		"?ref=" + url.QueryEscape(ref)
	data, err := a.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	var f RepoFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing repo file: %w", err)
	}
	return &f, nil
}

// GetFileRaw returns the raw bytes of a file.
func (a *RepoAPI) GetFileRaw(ctx context.Context, projectID, filePath, ref string) ([]byte, error) {
	if ref == "" {
		ref = "HEAD"
	}
	path := a.client.APIPath("/projects/"+EncodeProjectPath(projectID)+"/repository/files/"+url.PathEscape(filePath)+"/raw") +
		"?ref=" + url.QueryEscape(ref)
	return a.client.Get(ctx, path)
}

// FileWriteBody is the request body for create/update file.
type FileWriteBody struct {
	Branch        string `json:"branch"`
	Content       string `json:"content"`
	CommitMessage string `json:"commit_message"`
	Encoding      string `json:"encoding,omitempty"`
	// LastCommitID enables GitLab's optimistic concurrency: the update is rejected
	// if the file has moved past this commit since it was read.
	LastCommitID string `json:"last_commit_id,omitempty"`
}

// CreateFile creates a new file in the repository.
func (a *RepoAPI) CreateFile(ctx context.Context, projectID, filePath string, body FileWriteBody) error {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/repository/files/" + url.PathEscape(filePath))
	_, err := a.client.Post(ctx, path, body)
	return err
}

// UpdateFile updates an existing file in the repository.
func (a *RepoAPI) UpdateFile(ctx context.Context, projectID, filePath string, body FileWriteBody) error {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/repository/files/" + url.PathEscape(filePath))
	_, err := a.client.Put(ctx, path, body)
	return err
}

// DeleteFile deletes a file from the repository. A non-empty lastCommitID enables
// GitLab's optimistic concurrency: the delete is rejected if the file has moved
// past that commit.
func (a *RepoAPI) DeleteFile(ctx context.Context, projectID, filePath, branch, commitMessage, lastCommitID string) error {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/repository/files/" + url.PathEscape(filePath))
	body := map[string]string{
		"branch":         branch,
		"commit_message": commitMessage,
	}
	if lastCommitID != "" {
		body["last_commit_id"] = lastCommitID
	}
	return a.client.DeleteWithBody(ctx, path, body)
}

// ─── Branch methods ───────────────────────────────────────────────────────────

// ListBranches lists branches for a project.
func (a *RepoAPI) ListBranches(ctx context.Context, projectID string, opts *BranchListOpts) ([]Branch, error) {
	limit := 0
	if opts != nil {
		limit = opts.Limit
	}
	q := url.Values{}
	if opts != nil && opts.Search != "" {
		q.Set("search", opts.Search)
	}
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/repository/branches")
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	data, _, err := PaginateGET(ctx, a.client, path, limit)
	if err != nil {
		return nil, err
	}
	var out []Branch
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing branches: %w", err)
	}
	return out, nil
}

// CreateBranch creates a new branch.
func (a *RepoAPI) CreateBranch(ctx context.Context, projectID, name, ref string) (*Branch, error) {
	path := a.client.APIPath("/projects/"+EncodeProjectPath(projectID)+"/repository/branches") +
		"?branch=" + url.QueryEscape(name) + "&ref=" + url.QueryEscape(ref)
	data, err := a.client.Post(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var b Branch
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parsing branch: %w", err)
	}
	return &b, nil
}

// DeleteBranch deletes a branch.
func (a *RepoAPI) DeleteBranch(ctx context.Context, projectID, name string) error {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/repository/branches/" + url.PathEscape(name))
	return a.client.Delete(ctx, path)
}

// ─── Commit methods ───────────────────────────────────────────────────────────

// ListCommits lists commits for a project.
func (a *RepoAPI) ListCommits(ctx context.Context, projectID string, opts *CommitListOpts) ([]Commit, error) {
	limit := 0
	q := url.Values{}
	if opts != nil {
		limit = opts.Limit
		if opts.RefName != "" {
			q.Set("ref_name", opts.RefName)
		}
		if opts.Since != "" {
			q.Set("since", opts.Since)
		}
		if opts.Until != "" {
			q.Set("until", opts.Until)
		}
		if opts.Path != "" {
			q.Set("path", opts.Path)
		}
		if opts.Author != "" {
			q.Set("author", opts.Author)
		}
		if opts.WithStats {
			q.Set("with_stats", "true")
		}
		if opts.AllBranches {
			q.Set("all", "true")
		}
	}
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/repository/commits")
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	data, _, err := PaginateGET(ctx, a.client, path, limit)
	if err != nil {
		return nil, err
	}
	var out []Commit
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing commits: %w", err)
	}
	return out, nil
}

// CommitAction is one entry of the actions[] array on POST /repository/commits.
// action is one of create/update/delete/move; the other fields are populated per
// action (see GitLab commits API). Content may be base64 (set Encoding).
type CommitAction struct {
	Action       string `json:"action"`
	FilePath     string `json:"file_path"`
	PreviousPath string `json:"previous_path,omitempty"`
	Content      string `json:"content,omitempty"`
	Encoding     string `json:"encoding,omitempty"`
	LastCommitID string `json:"last_commit_id,omitempty"`
}

// CommitCreateOpts is the POST body for an atomic multi-file commit.
type CommitCreateOpts struct {
	Branch        string         `json:"branch"`
	CommitMessage string         `json:"commit_message"`
	StartBranch   string         `json:"start_branch,omitempty"`
	Actions       []CommitAction `json:"actions"`
}

// CreateCommit creates a single atomic commit applying all actions[] in one
// upstream request (POST /projects/:id/repository/commits). This is the native
// bulk endpoint (class A): create/update/delete/move many files land as one
// commit, server-side atomic. GitLab caps a single commit at 1000 actions.
func (a *RepoAPI) CreateCommit(ctx context.Context, projectID string, opts CommitCreateOpts) (*Commit, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/repository/commits")
	data, err := a.client.Post(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	var c Commit
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing created commit: %w", err)
	}
	return &c, nil
}

// GetCommit returns a single commit by SHA.
func (a *RepoAPI) GetCommit(ctx context.Context, projectID, sha string) (*Commit, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/repository/commits/" + url.PathEscape(sha))
	data, err := a.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	var c Commit
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing commit: %w", err)
	}
	return &c, nil
}

// GetCommitDiff returns the per-file unified diff for a single commit
// (GET /projects/:id/repository/commits/:sha/diff). This is a heavy sub-resource
// kept separate from GetCommit on purpose: an agent reads it on demand for the
// specific SHAs it cares about, never inlined into a list.
func (a *RepoAPI) GetCommitDiff(ctx context.Context, projectID, sha string) ([]CommitDiffFile, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/repository/commits/" + url.PathEscape(sha) + "/diff")
	data, err := a.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	var out []CommitDiffFile
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing commit diff: %w", err)
	}
	return out, nil
}

// ─── Tree method ──────────────────────────────────────────────────────────────

// ListTree lists tree entries for a project.
func (a *RepoAPI) ListTree(ctx context.Context, projectID string, opts *TreeOpts) ([]TreeEntry, error) {
	limit := 0
	q := url.Values{}
	if opts != nil {
		limit = opts.Limit
		if opts.Path != "" {
			q.Set("path", opts.Path)
		}
		if opts.Ref != "" {
			q.Set("ref", opts.Ref)
		}
		if opts.Recursive {
			q.Set("recursive", "true")
		}
	}
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/repository/tree")
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	data, _, err := PaginateGET(ctx, a.client, path, limit)
	if err != nil {
		return nil, err
	}
	var out []TreeEntry
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing tree: %w", err)
	}
	return out, nil
}
