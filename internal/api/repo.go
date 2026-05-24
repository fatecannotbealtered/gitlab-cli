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

// DeleteFile deletes a file from the repository.
func (a *RepoAPI) DeleteFile(ctx context.Context, projectID, filePath, branch, commitMessage string) error {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/repository/files/" + url.PathEscape(filePath))
	body := map[string]string{
		"branch":         branch,
		"commit_message": commitMessage,
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
