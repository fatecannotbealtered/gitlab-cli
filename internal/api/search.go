package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// SearchAPI wraps the global search API.
//
// Endpoint reference: https://docs.gitlab.com/api/search/
//
// Methods are implemented in this file by Phase 1 Wave B.
type SearchAPI struct{ client *Client }

// ─── DTOs ────────────────────────────────────────────────────────────────────

// SearchProject is a slim project result from global search.
type SearchProject struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	WebURL            string `json:"web_url"`
	Visibility        string `json:"visibility"`
	DefaultBranch     string `json:"default_branch"`
}

// SearchIssue is a slim issue result from search.
type SearchIssue struct {
	ID        int    `json:"id"`
	IID       int    `json:"iid"`
	Title     string `json:"title"`
	State     string `json:"state"`
	WebURL    string `json:"web_url"`
	ProjectID int    `json:"project_id"`
}

// SearchMR is a slim merge request result from search.
type SearchMR struct {
	ID        int    `json:"id"`
	IID       int    `json:"iid"`
	Title     string `json:"title"`
	State     string `json:"state"`
	WebURL    string `json:"web_url"`
	ProjectID int    `json:"project_id"`
}

// SearchBlob is a code search result (blob/file match).
type SearchBlob struct {
	Basename  string `json:"basename"`
	Data      string `json:"data"`
	Path      string `json:"path"`
	Filename  string `json:"filename"`
	Ref       string `json:"ref"`
	StartLine int    `json:"startline"`
	ProjectID int    `json:"project_id"`
}

// SearchCommit is a slim commit result from search.
type SearchCommit struct {
	ID         string `json:"id"`
	ShortID    string `json:"short_id"`
	Title      string `json:"title"`
	AuthorName string `json:"author_name"`
	CreatedAt  string `json:"created_at"`
	WebURL     string `json:"web_url"`
	ProjectID  int    `json:"project_id"`
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func perPageParam(limit int) string {
	return strconv.Itoa(listPerPage(limit))
}

func (a *SearchAPI) globalSearch(ctx context.Context, scope, query string, limit int) ([]byte, error) {
	params := url.Values{}
	params.Set("scope", scope)
	params.Set("search", query)
	params.Set("per_page", perPageParam(limit))
	path := a.client.APIPath("/search") + "?" + params.Encode()
	data, _, err := PaginateGET(ctx, a.client, path, limit)
	return data, err
}

func (a *SearchAPI) projectSearch(ctx context.Context, projectID, scope, query string, limit int) ([]byte, error) {
	params := url.Values{}
	params.Set("scope", scope)
	params.Set("search", query)
	params.Set("per_page", perPageParam(limit))
	path := a.client.APIPath("/projects/"+EncodeProjectPath(projectID)+"/search") + "?" + params.Encode()
	data, _, err := PaginateGET(ctx, a.client, path, limit)
	return data, err
}

// ─── Methods ─────────────────────────────────────────────────────────────────

// Projects searches for projects globally.
//
// GET /api/v4/search?scope=projects&search=<q>
func (a *SearchAPI) Projects(ctx context.Context, query string, limit int) ([]SearchProject, error) {
	data, err := a.globalSearch(ctx, "projects", query, limit)
	if err != nil {
		return nil, err
	}
	var out []SearchProject
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing search projects: %w", err)
	}
	return out, nil
}

// Issues searches for issues. If project is non-empty, uses project-scoped endpoint.
//
// GET /api/v4/search?scope=issues&search=<q>
// GET /api/v4/projects/:id/search?scope=issues&search=<q>
func (a *SearchAPI) Issues(ctx context.Context, query, project string, limit int) ([]SearchIssue, error) {
	var (
		data []byte
		err  error
	)
	if project != "" {
		data, err = a.projectSearch(ctx, project, "issues", query, limit)
	} else {
		data, err = a.globalSearch(ctx, "issues", query, limit)
	}
	if err != nil {
		return nil, err
	}
	var out []SearchIssue
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing search issues: %w", err)
	}
	return out, nil
}

// MergeRequests searches for MRs. If project is non-empty, uses project-scoped endpoint.
//
// GET /api/v4/search?scope=merge_requests&search=<q>
// GET /api/v4/projects/:id/search?scope=merge_requests&search=<q>
func (a *SearchAPI) MergeRequests(ctx context.Context, query, project string, limit int) ([]SearchMR, error) {
	var (
		data []byte
		err  error
	)
	if project != "" {
		data, err = a.projectSearch(ctx, project, "merge_requests", query, limit)
	} else {
		data, err = a.globalSearch(ctx, "merge_requests", query, limit)
	}
	if err != nil {
		return nil, err
	}
	var out []SearchMR
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing search mrs: %w", err)
	}
	return out, nil
}

// Code searches for code (blobs) within a project. project is required.
//
// GET /api/v4/projects/:id/search?scope=blobs&search=<q>
func (a *SearchAPI) Code(ctx context.Context, query, project string, limit int) ([]SearchBlob, error) {
	data, err := a.projectSearch(ctx, project, "blobs", query, limit)
	if err != nil {
		return nil, err
	}
	var out []SearchBlob
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing search blobs: %w", err)
	}
	return out, nil
}

// Commits searches for commits. If project is non-empty, uses project-scoped endpoint.
//
// GET /api/v4/search?scope=commits&search=<q>
// GET /api/v4/projects/:id/search?scope=commits&search=<q>
func (a *SearchAPI) Commits(ctx context.Context, query, project string, limit int) ([]SearchCommit, error) {
	var (
		data []byte
		err  error
	)
	if project != "" {
		data, err = a.projectSearch(ctx, project, "commits", query, limit)
	} else {
		data, err = a.globalSearch(ctx, "commits", query, limit)
	}
	if err != nil {
		return nil, err
	}
	var out []SearchCommit
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing search commits: %w", err)
	}
	return out, nil
}
