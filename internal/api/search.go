package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// SearchAPI wraps GitLab's search API, global and within a project.
//
// Endpoint reference: https://docs.gitlab.com/api/search/
//
// Order: GitLab ranks results by relevance only with advanced search; basic search
// lists issues and merge requests newest first and ranks no scope by relevance.
// Global commit and code search need advanced search (or exact code search for
// code); within a project, basic search serves both.
type SearchAPI struct{ client *Client }

// ─── DTOs ────────────────────────────────────────────────────────────────────

// SearchOpts selects one page of a search. Project is a path or numeric ID; ""
// searches every project this account can see.
type SearchOpts struct {
	Project string
	Query   string
	Page    int
	PerPage int
}

// References is the references object of an issue or merge request; Full is the
// cross-project form (acme/web#12, acme/web!5).
type References struct {
	Full string `json:"full"`
}

// SearchIssue is an issue as the search API returns it.
type SearchIssue struct {
	ID         int        `json:"id"`
	IID        int        `json:"iid"`
	ProjectID  int        `json:"project_id"`
	Title      string     `json:"title"`
	State      string     `json:"state"`
	Labels     []string   `json:"labels"`
	Author     *User      `json:"author"`
	CreatedAt  string     `json:"created_at"`
	UpdatedAt  string     `json:"updated_at"`
	WebURL     string     `json:"web_url"`
	References References `json:"references"`
}

// SearchMR is a merge request as the search API returns it.
type SearchMR struct {
	ID           int        `json:"id"`
	IID          int        `json:"iid"`
	ProjectID    int        `json:"project_id"`
	Title        string     `json:"title"`
	State        string     `json:"state"`
	Draft        bool       `json:"draft"`
	SourceBranch string     `json:"source_branch"`
	TargetBranch string     `json:"target_branch"`
	Labels       []string   `json:"labels"`
	Author       *User      `json:"author"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
	WebURL       string     `json:"web_url"`
	References   References `json:"references"`
}

// SearchBlob is a code search result: the matching lines of a file. Ref is the
// branch, tag or commit searched.
type SearchBlob struct {
	Basename  string `json:"basename"`
	Data      string `json:"data"`
	Path      string `json:"path"`
	Filename  string `json:"filename"`
	Ref       string `json:"ref"`
	StartLine int    `json:"startline"`
	ProjectID int    `json:"project_id"`
}

// SearchCommit is a commit as the search API returns it.
type SearchCommit struct {
	ID         string `json:"id"`
	ShortID    string `json:"short_id"`
	Title      string `json:"title"`
	AuthorName string `json:"author_name"`
	CreatedAt  string `json:"created_at"`
	WebURL     string `json:"web_url"`
	ProjectID  int    `json:"project_id"`
}

// ─── Methods ─────────────────────────────────────────────────────────────────

// page fetches one page of scope into out.
func (a *SearchAPI) page(ctx context.Context, scope string, opts SearchOpts, out any) (Pagination, error) {
	q := url.Values{}
	q.Set("scope", scope)
	q.Set("search", strings.TrimSpace(opts.Query))
	path := a.client.APIPath("/search")
	if opts.Project != "" {
		path = a.client.APIPath("/projects/" + EncodeProjectPath(opts.Project) + "/search")
	}
	data, pag, err := GetPage(ctx, a.client, path+"?"+q.Encode(), opts.Page, opts.PerPage)
	if err != nil {
		return Pagination{}, err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return Pagination{}, fmt.Errorf("parsing search %s: %w", scope, err)
	}
	return pag, nil
}

// ProjectsPage returns one page of the projects matching query (global search;
// GitLab's project search has no projects scope).
//
// GET /api/v4/search?scope=projects
func (a *SearchAPI) ProjectsPage(ctx context.Context, query string, page, perPage int) ([]Project, Pagination, error) {
	var out []Project
	pag, err := a.page(ctx, "projects", SearchOpts{Query: query, Page: page, PerPage: perPage}, &out)
	return out, pag, err
}

// IssuesPage returns one page of the issues matching opts.Query.
//
// GET /api/v4/search?scope=issues
// GET /api/v4/projects/:id/search?scope=issues
func (a *SearchAPI) IssuesPage(ctx context.Context, opts SearchOpts) ([]SearchIssue, Pagination, error) {
	var out []SearchIssue
	pag, err := a.page(ctx, "issues", opts, &out)
	return out, pag, err
}

// MergeRequestsPage returns one page of the merge requests matching opts.Query.
//
// GET /api/v4/search?scope=merge_requests
// GET /api/v4/projects/:id/search?scope=merge_requests
func (a *SearchAPI) MergeRequestsPage(ctx context.Context, opts SearchOpts) ([]SearchMR, Pagination, error) {
	var out []SearchMR
	pag, err := a.page(ctx, "merge_requests", opts, &out)
	return out, pag, err
}

// BlobsPage returns one page of code matches (global code search needs advanced or
// exact code search).
//
// GET /api/v4/projects/:id/search?scope=blobs
func (a *SearchAPI) BlobsPage(ctx context.Context, opts SearchOpts) ([]SearchBlob, Pagination, error) {
	var out []SearchBlob
	pag, err := a.page(ctx, "blobs", opts, &out)
	return out, pag, err
}

// CommitsPage returns one page of the commits matching opts.Query (global commit
// search needs advanced search).
//
// GET /api/v4/search?scope=commits
// GET /api/v4/projects/:id/search?scope=commits
func (a *SearchAPI) CommitsPage(ctx context.Context, opts SearchOpts) ([]SearchCommit, Pagination, error) {
	var out []SearchCommit
	pag, err := a.page(ctx, "commits", opts, &out)
	return out, pag, err
}
