package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// RepoAPI wraps repository-related API calls (files, branches, tags, commits, tree).
//
// Endpoint references:
//
//	https://docs.gitlab.com/api/repository_files/
//	https://docs.gitlab.com/api/branches/
//	https://docs.gitlab.com/api/tags/
//	https://docs.gitlab.com/api/commits/
//	https://docs.gitlab.com/api/repositories/
type RepoAPI struct{ client *Client }

// ─── DTOs ────────────────────────────────────────────────────────────────────

// RepoFile is returned by GET /projects/:id/repository/files/:path. Content is
// base64 (Encoding "base64"); Decoded returns the bytes.
type RepoFile struct {
	FileName        string `json:"file_name"`
	FilePath        string `json:"file_path"`
	Size            int    `json:"size"`
	Encoding        string `json:"encoding"`
	Content         string `json:"content"`
	ContentSHA256   string `json:"content_sha256"`
	Ref             string `json:"ref"`
	BlobID          string `json:"blob_id"`
	CommitID        string `json:"commit_id"`
	LastCommitID    string `json:"last_commit_id"`
	ExecuteFilemode bool   `json:"execute_filemode"`
}

// Decoded returns the file's content as bytes.
func (f *RepoFile) Decoded() ([]byte, error) {
	if f.Encoding == "base64" {
		b, err := base64.StdEncoding.DecodeString(f.Content)
		if err != nil {
			return nil, fmt.Errorf("decoding file content: %w", err)
		}
		return b, nil
	}
	return []byte(f.Content), nil
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

// Tag is returned by GET /projects/:id/repository/tags/:name
type Tag struct {
	Name      string  `json:"name"`
	Message   string  `json:"message"`
	Target    string  `json:"target"`
	Protected bool    `json:"protected"`
	Commit    *Commit `json:"commit"`
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
// return per-file line counts, so callers count them from the patch hunks. TooLarge
// and Collapsed mark a patch GitLab left out.
type CommitDiffFile struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	NewFile     bool   `json:"new_file"`
	DeletedFile bool   `json:"deleted_file"`
	RenamedFile bool   `json:"renamed_file"`
	Diff        string `json:"diff"`
	TooLarge    bool   `json:"too_large"`
	Collapsed   bool   `json:"collapsed"`
}

// TreeEntry is returned by GET /projects/:id/repository/tree
type TreeEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // "blob", "tree" or "commit" (a submodule)
	Path string `json:"path"`
	Mode string `json:"mode"`
}

// ─── List opts ───────────────────────────────────────────────────────────────

// BranchListOpts are options for listing branches.
type BranchListOpts struct {
	Search  string // GitLab's search: a substring; ^ anchors the start, $ the end
	Page    int
	PerPage int
}

// CommitListOpts are options for listing commits.
type CommitListOpts struct {
	RefName string
	Since   string
	Until   string
	Path    string
	Limit   int // ListCommits only
	Page    int // ListCommitsPage only
	PerPage int // ListCommitsPage only
	// Author filters commits server-side by author (GitLab 15.10+). On older
	// instances GitLab ignores the param and returns the unfiltered list.
	Author string
	// WithStats requests per-commit line counts (sets with_stats=true).
	WithStats bool
	// AllBranches lists commits across every ref (sets all=true), not just the
	// default/selected branch.
	AllBranches bool
}

func (o *CommitListOpts) query() url.Values {
	q := url.Values{}
	if o == nil {
		return q
	}
	if o.RefName != "" {
		q.Set("ref_name", o.RefName)
	}
	if o.Since != "" {
		q.Set("since", o.Since)
	}
	if o.Until != "" {
		q.Set("until", o.Until)
	}
	if o.Path != "" {
		q.Set("path", o.Path)
	}
	if o.Author != "" {
		q.Set("author", o.Author)
	}
	if o.WithStats {
		q.Set("with_stats", "true")
	}
	if o.AllBranches {
		q.Set("all", "true")
	}
	return q
}

// TreeOpts are options for listing tree entries.
type TreeOpts struct {
	Path      string
	Ref       string
	Recursive bool
	Page      int
	PerPage   int
}

func (o *TreeOpts) query() url.Values {
	q := url.Values{}
	if o == nil {
		return q
	}
	if o.Path != "" {
		q.Set("path", o.Path)
	}
	if o.Ref != "" {
		q.Set("ref", o.Ref)
	}
	if o.Recursive {
		q.Set("recursive", "true")
	}
	return q
}

func (a *RepoAPI) projectPath(projectID, rest string) string {
	return a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + rest)
}

// ─── File methods ─────────────────────────────────────────────────────────────

// GetFile returns a file's metadata and base64 content at ref ("HEAD" when empty).
//
// GET /api/v4/projects/:id/repository/files/:file_path?ref=
func (a *RepoAPI) GetFile(ctx context.Context, projectID, filePath, ref string) (*RepoFile, error) {
	if ref == "" {
		ref = "HEAD"
	}
	path := a.projectPath(projectID, "/repository/files/"+url.PathEscape(filePath)) + "?ref=" + url.QueryEscape(ref)
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

// CreateFile creates a new file in the repository (one commit on Branch).
//
// POST /api/v4/projects/:id/repository/files/:file_path
func (a *RepoAPI) CreateFile(ctx context.Context, projectID, filePath string, body FileWriteBody) error {
	_, err := a.client.Post(ctx, a.projectPath(projectID, "/repository/files/"+url.PathEscape(filePath)), body)
	return err
}

// UpdateFile updates an existing file in the repository (one commit on Branch).
//
// PUT /api/v4/projects/:id/repository/files/:file_path
func (a *RepoAPI) UpdateFile(ctx context.Context, projectID, filePath string, body FileWriteBody) error {
	_, err := a.client.Put(ctx, a.projectPath(projectID, "/repository/files/"+url.PathEscape(filePath)), body)
	return err
}

// DeleteFile deletes a file from the repository. A non-empty lastCommitID enables
// GitLab's optimistic concurrency: the delete is rejected if the file has moved
// past that commit.
//
// DELETE /api/v4/projects/:id/repository/files/:file_path
func (a *RepoAPI) DeleteFile(ctx context.Context, projectID, filePath, branch, commitMessage, lastCommitID string) error {
	body := map[string]string{
		"branch":         branch,
		"commit_message": commitMessage,
	}
	if lastCommitID != "" {
		body["last_commit_id"] = lastCommitID
	}
	return a.client.DeleteWithBody(ctx, a.projectPath(projectID, "/repository/files/"+url.PathEscape(filePath)), body)
}

// ─── Branch methods ───────────────────────────────────────────────────────────

// ListBranchesPage returns one page of a project's branches.
//
// GET /api/v4/projects/:id/repository/branches
func (a *RepoAPI) ListBranchesPage(ctx context.Context, projectID string, opts BranchListOpts) ([]Branch, Pagination, error) {
	q := url.Values{}
	if s := strings.TrimSpace(opts.Search); s != "" {
		q.Set("search", s)
	}
	path := a.projectPath(projectID, "/repository/branches") + "?" + q.Encode()
	data, pag, err := GetPage(ctx, a.client, path, opts.Page, opts.PerPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []Branch
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing branches: %w", err)
	}
	return out, pag, nil
}

// GetBranch returns one branch by name.
//
// GET /api/v4/projects/:id/repository/branches/:branch
func (a *RepoAPI) GetBranch(ctx context.Context, projectID, name string) (*Branch, error) {
	data, err := a.client.Get(ctx, a.projectPath(projectID, "/repository/branches/"+url.PathEscape(name)))
	if err != nil {
		return nil, err
	}
	var b Branch
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parsing branch: %w", err)
	}
	return &b, nil
}

// CreateBranch creates a branch named name at ref (a branch, tag or commit SHA).
//
// POST /api/v4/projects/:id/repository/branches?branch=&ref=
func (a *RepoAPI) CreateBranch(ctx context.Context, projectID, name, ref string) (*Branch, error) {
	path := a.projectPath(projectID, "/repository/branches") + "?branch=" + url.QueryEscape(name) + "&ref=" + url.QueryEscape(ref)
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
//
// DELETE /api/v4/projects/:id/repository/branches/:branch
func (a *RepoAPI) DeleteBranch(ctx context.Context, projectID, name string) error {
	return a.client.Delete(ctx, a.projectPath(projectID, "/repository/branches/"+url.PathEscape(name)))
}

// ─── Tag methods ──────────────────────────────────────────────────────────────

// GetTag returns one tag by name.
//
// GET /api/v4/projects/:id/repository/tags/:tag_name
func (a *RepoAPI) GetTag(ctx context.Context, projectID, name string) (*Tag, error) {
	data, err := a.client.Get(ctx, a.projectPath(projectID, "/repository/tags/"+url.PathEscape(name)))
	if err != nil {
		return nil, err
	}
	var t Tag
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parsing tag: %w", err)
	}
	return &t, nil
}

// ─── Commit methods ───────────────────────────────────────────────────────────

// ListCommitsPage returns one page of a project's commits, newest first.
//
// GET /api/v4/projects/:id/repository/commits
func (a *RepoAPI) ListCommitsPage(ctx context.Context, projectID string, opts CommitListOpts) ([]Commit, Pagination, error) {
	path := a.projectPath(projectID, "/repository/commits") + "?" + opts.query().Encode()
	data, pag, err := GetPage(ctx, a.client, path, opts.Page, opts.PerPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []Commit
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing commits: %w", err)
	}
	return out, pag, nil
}

// ListCommits lists up to opts.Limit commits (all pages merged).
func (a *RepoAPI) ListCommits(ctx context.Context, projectID string, opts *CommitListOpts) ([]Commit, error) {
	limit := 0
	if opts != nil {
		limit = opts.Limit
	}
	path := a.projectPath(projectID, "/repository/commits")
	if enc := opts.query().Encode(); enc != "" {
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
// bulk endpoint: create/update/delete/move many files land as one commit,
// server-side atomic. GitLab caps a single commit at 1000 actions.
func (a *RepoAPI) CreateCommit(ctx context.Context, projectID string, opts CommitCreateOpts) (*Commit, error) {
	data, err := a.client.Post(ctx, a.projectPath(projectID, "/repository/commits"), opts)
	if err != nil {
		return nil, err
	}
	var c Commit
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing created commit: %w", err)
	}
	return &c, nil
}

// GetCommit returns a single commit by SHA, or the head commit of a branch or tag
// name, with its line counts (GitLab's stats default).
//
// GET /api/v4/projects/:id/repository/commits/:sha
func (a *RepoAPI) GetCommit(ctx context.Context, projectID, sha string) (*Commit, error) {
	data, err := a.client.Get(ctx, a.projectPath(projectID, "/repository/commits/"+url.PathEscape(sha)))
	if err != nil {
		return nil, err
	}
	var c Commit
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing commit: %w", err)
	}
	return &c, nil
}

// ListCommitDiff returns a commit's per-file diff, reading GitLab's pages (100 files
// each) until the diff ends or maxFiles files were read; more reports that files
// beyond maxFiles were left unread. (GitLab pages this endpoint too: one plain GET
// returns only its first 20 files.)
//
// GET /api/v4/projects/:id/repository/commits/:sha/diff
func (a *RepoAPI) ListCommitDiff(ctx context.Context, projectID, sha string, maxFiles int) (files []CommitDiffFile, more bool, err error) {
	path := a.projectPath(projectID, "/repository/commits/"+url.PathEscape(sha)+"/diff")
	for page := 1; ; page++ {
		data, pag, err := GetPage(ctx, a.client, path, page, MaxPerPage)
		if err != nil {
			return nil, false, err
		}
		var chunk []CommitDiffFile
		if err := json.Unmarshal(data, &chunk); err != nil {
			return nil, false, fmt.Errorf("parsing commit diff: %w", err)
		}
		files = append(files, chunk...)
		if len(files) >= maxFiles {
			return files[:maxFiles], len(files) > maxFiles || pag.NextPage > 0, nil
		}
		if pag.NextPage == 0 || len(chunk) == 0 {
			return files, false, nil
		}
	}
}

// ─── Tree method ──────────────────────────────────────────────────────────────

// ListTreePage returns one page of a project's tree entries.
//
// GET /api/v4/projects/:id/repository/tree
func (a *RepoAPI) ListTreePage(ctx context.Context, projectID string, opts TreeOpts) ([]TreeEntry, Pagination, error) {
	path := a.projectPath(projectID, "/repository/tree") + "?" + opts.query().Encode()
	data, pag, err := GetPage(ctx, a.client, path, opts.Page, opts.PerPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []TreeEntry
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing tree: %w", err)
	}
	return out, pag, nil
}

// ─── Projects to fan out over (repo commit list --group / --all-projects) ─────

// fanOutProject is the part of a project the commit fan-out needs.
type fanOutProject struct {
	PathWithNamespace string `json:"path_with_namespace"`
}

// GroupProjectPaths returns the paths of the projects in a group and its subgroups,
// archived ones left out, up to max; more reports that the group has more. It is the
// repository group's own enumeration for the commit fan-out.
//
// GET /api/v4/groups/:id/projects?include_subgroups=true&archived=false&simple=true
func (a *RepoAPI) GroupProjectPaths(ctx context.Context, group string, max int) (paths []string, more bool, err error) {
	q := url.Values{}
	q.Set("include_subgroups", "true")
	q.Set("archived", "false")
	q.Set("simple", "true")
	q.Set("order_by", "path")
	q.Set("sort", "asc")
	return a.projectPaths(ctx, a.client.APIPath("/groups/"+EncodeProjectPath(group)+"/projects")+"?"+q.Encode(), max)
}

// MemberProjectPaths returns the paths of the projects the token's user is a
// member of, archived ones left out, up to max; more reports that there are more.
//
// GET /api/v4/projects?membership=true&archived=false&simple=true
func (a *RepoAPI) MemberProjectPaths(ctx context.Context, max int) (paths []string, more bool, err error) {
	q := url.Values{}
	q.Set("membership", "true")
	q.Set("archived", "false")
	q.Set("simple", "true")
	q.Set("order_by", "path")
	q.Set("sort", "asc")
	return a.projectPaths(ctx, a.client.APIPath("/projects")+"?"+q.Encode(), max)
}

func (a *RepoAPI) projectPaths(ctx context.Context, path string, max int) ([]string, bool, error) {
	var out []string
	for page := 1; ; page++ {
		data, pag, err := GetPage(ctx, a.client, path, page, MaxPerPage)
		if err != nil {
			return nil, false, err
		}
		var chunk []fanOutProject
		if err := json.Unmarshal(data, &chunk); err != nil {
			return nil, false, fmt.Errorf("parsing projects: %w", err)
		}
		for _, p := range chunk {
			if p.PathWithNamespace != "" {
				out = append(out, p.PathWithNamespace)
			}
		}
		if len(out) >= max {
			return out[:max], len(out) > max || pag.NextPage > 0, nil
		}
		if pag.NextPage == 0 || len(chunk) == 0 {
			return out, false, nil
		}
	}
}
