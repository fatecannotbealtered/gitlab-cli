package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// ProjectAPI wraps project-related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/projects/
//
// Methods are implemented in this file by Phase 1 Wave B.
type ProjectAPI struct{ client *Client }

// ─── DTOs ────────────────────────────────────────────────────────────────────

// Project mirrors the GitLab project object (subset of fields the CLI surfaces).
type Project struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	Visibility        string `json:"visibility"`
	WebURL            string `json:"web_url"`
	DefaultBranch     string `json:"default_branch"`
	Description       string `json:"description"`
	SSHURLToRepo      string `json:"ssh_url_to_repo"`
	HTTPURLToRepo     string `json:"http_url_to_repo"`
	StarCount         int    `json:"star_count"`
	ForksCount        int    `json:"forks_count"`
	CreatedAt         string `json:"created_at"`
	LastActivityAt    string `json:"last_activity_at"`
}

// ProjectMember mirrors a GitLab project member.
type ProjectMember struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	State       string `json:"state"`
	AccessLevel int    `json:"access_level"`
	WebURL      string `json:"web_url"`
}

// ProjectCreateRequest is the POST body for creating a project. omitempty keeps
// the body minimal so GitLab applies its own defaults (e.g. namespace, visibility)
// for any field the caller leaves unset.
type ProjectCreateRequest struct {
	Name        string `json:"name"`
	Path        string `json:"path,omitempty"`
	Description string `json:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty"` // private|internal|public
	NamespaceID int    `json:"namespace_id,omitempty"`
}

// ProjectListOpts holds query parameters for listing projects.
type ProjectListOpts struct {
	Owned      bool
	Membership bool
	Search     string
	Visibility string // public|internal|private
	Limit      int
}

func (o *ProjectListOpts) encode() string {
	if o == nil {
		return ""
	}
	params := url.Values{}
	if o.Owned {
		params.Set("owned", "true")
	}
	if o.Membership {
		params.Set("membership", "true")
	}
	if o.Search != "" {
		params.Set("search", o.Search)
	}
	if o.Visibility != "" {
		params.Set("visibility", o.Visibility)
	}
	params.Set("per_page", strconv.Itoa(listPerPage(o.Limit)))
	return params.Encode()
}

// ─── Methods ─────────────────────────────────────────────────────────────────

// List returns projects matching the given options.
//
// GET /api/v4/projects
func (a *ProjectAPI) List(ctx context.Context, opts *ProjectListOpts) ([]Project, error) {
	path := a.client.APIPath("/projects")
	limit := 0
	var q string
	if opts != nil {
		limit = opts.Limit
		q = opts.encode()
	}
	if q != "" {
		path += "?" + q
	}
	data, _, err := PaginateGET(ctx, a.client, path, limit)
	if err != nil {
		return nil, err
	}
	var out []Project
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing projects: %w", err)
	}
	return out, nil
}

// ListGroupProjects returns projects under a group, including subgroups, so a
// "commits by author across a team" query can fan out over a whole group tree.
// GitLab has no group-level commits endpoint, so the caller loops these projects.
//
// GET /api/v4/groups/:id/projects?include_subgroups=true
func (a *ProjectAPI) ListGroupProjects(ctx context.Context, groupID string, limit int) ([]Project, error) {
	params := url.Values{}
	params.Set("include_subgroups", "true")
	// archived projects can't receive new commits in a window; drop them so the
	// fan-out doesn't waste a request per dead repo.
	params.Set("archived", "false")
	params.Set("per_page", strconv.Itoa(listPerPage(limit)))
	path := a.client.APIPath("/groups/"+EncodeProjectPath(groupID)+"/projects") + "?" + params.Encode()
	data, _, err := PaginateGET(ctx, a.client, path, limit)
	if err != nil {
		return nil, err
	}
	var out []Project
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing group projects: %w", err)
	}
	return out, nil
}

// Get returns a single project by ID or path.
//
// GET /api/v4/projects/:id
func (a *ProjectAPI) Get(ctx context.Context, idOrPath string) (*Project, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(idOrPath))
	data, err := a.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	var p Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing project: %w", err)
	}
	return &p, nil
}

// Create creates a new project.
//
// POST /api/v4/projects
func (a *ProjectAPI) Create(ctx context.Context, req *ProjectCreateRequest) (*Project, error) {
	path := a.client.APIPath("/projects")
	data, err := a.client.Post(ctx, path, req)
	if err != nil {
		return nil, err
	}
	var p Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing created project: %w", err)
	}
	return &p, nil
}

// Members returns members of a project.
//
// GET /api/v4/projects/:id/members
func (a *ProjectAPI) Members(ctx context.Context, idOrPath, query string, limit int) ([]ProjectMember, error) {
	params := url.Values{}
	if query != "" {
		params.Set("query", query)
	}
	params.Set("per_page", strconv.Itoa(listPerPage(limit)))
	path := a.client.APIPath("/projects/"+EncodeProjectPath(idOrPath)+"/members") + "?" + params.Encode()
	data, _, err := PaginateGET(ctx, a.client, path, limit)
	if err != nil {
		return nil, err
	}
	var out []ProjectMember
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing project members: %w", err)
	}
	return out, nil
}
