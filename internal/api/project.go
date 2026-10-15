package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ProjectAPI wraps project-related API calls: projects, their members, and the
// namespaces projects are created in.
//
// Endpoint reference: https://docs.gitlab.com/api/projects/
type ProjectAPI struct{ client *Client }

// ─── DTOs ────────────────────────────────────────────────────────────────────

// Project mirrors the GitLab project object (the fields the CLI surfaces). The
// list with simple=true and the search API leave out visibility and archived, so
// there Visibility is "" and Archived nil.
type Project struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	NameWithNamespace string `json:"name_with_namespace"`
	Path              string `json:"path"`
	PathWithNamespace string `json:"path_with_namespace"`
	Visibility        string `json:"visibility"`
	WebURL            string `json:"web_url"`
	DefaultBranch     string `json:"default_branch"`
	Description       string `json:"description"`
	Archived          *bool  `json:"archived"`
	EmptyRepo         bool   `json:"empty_repo"` // no commit yet: a new project before its first push
	SSHURLToRepo      string `json:"ssh_url_to_repo"`
	HTTPURLToRepo     string `json:"http_url_to_repo"`
	StarCount         int    `json:"star_count"`
	ForksCount        int    `json:"forks_count"`
	CreatedAt         string `json:"created_at"`
	LastActivityAt    string `json:"last_activity_at"`
}

// ProjectMember is a project member as the members API returns it: the user and the
// access level the membership grants (10 guest, 20 reporter, 30 developer, 40
// maintainer, 50 owner).
type ProjectMember struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	State       string `json:"state"`
	AccessLevel int    `json:"access_level"`
	ExpiresAt   string `json:"expires_at"`
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

// Namespace is a group or a user's personal namespace.
type Namespace struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Kind     string `json:"kind"` // group or user
	FullPath string `json:"full_path"`
}

// ProjectListOpts holds query parameters for listing projects. List reads Limit
// (every page up to it); ListPage reads Page, PerPage, Simple, OrderBy and Sort.
type ProjectListOpts struct {
	Owned      bool
	Membership bool
	Search     string // GitLab's search: part of the path, name or description
	Visibility string // public|internal|private
	Limit      int    // List only
	Simple     bool   // ListPage only: GitLab's limited fields (no visibility, no archived)
	OrderBy    string // ListPage only: id, name, path, created_at, updated_at, last_activity_at
	Sort       string // ListPage only: asc or desc
	Page       int    // ListPage only
	PerPage    int    // ListPage only
}

func (o *ProjectListOpts) filters() url.Values {
	params := url.Values{}
	if o == nil {
		return params
	}
	if o.Owned {
		params.Set("owned", "true")
	}
	if o.Membership {
		params.Set("membership", "true")
	}
	if s := strings.TrimSpace(o.Search); s != "" {
		params.Set("search", s)
	}
	if o.Visibility != "" {
		params.Set("visibility", o.Visibility)
	}
	return params
}

func (o *ProjectListOpts) encode() string {
	if o == nil {
		return ""
	}
	params := o.filters()
	params.Set("per_page", strconv.Itoa(listPerPage(o.Limit)))
	return params.Encode()
}

// MemberListOpts filters a project's members.
type MemberListOpts struct {
	Query   string // GitLab's query: part of the name, username or email
	UserIDs []int  // only these users
	Page    int
	PerPage int
}

func (o MemberListOpts) query() url.Values {
	q := url.Values{}
	if s := strings.TrimSpace(o.Query); s != "" {
		q.Set("query", s)
	}
	for _, id := range o.UserIDs {
		q.Add("user_ids[]", strconv.Itoa(id))
	}
	return q
}

// ─── Methods ─────────────────────────────────────────────────────────────────

// List returns up to opts.Limit projects matching the options (all pages merged).
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

// ListPage returns one page of the projects this account can see, filtered and
// ordered by opts.
//
// GET /api/v4/projects
func (a *ProjectAPI) ListPage(ctx context.Context, opts ProjectListOpts) ([]Project, Pagination, error) {
	q := opts.filters()
	if opts.Simple {
		q.Set("simple", "true")
	}
	if opts.OrderBy != "" {
		q.Set("order_by", opts.OrderBy)
	}
	if opts.Sort != "" {
		q.Set("sort", opts.Sort)
	}
	data, pag, err := GetPage(ctx, a.client, a.client.APIPath("/projects")+"?"+q.Encode(), opts.Page, opts.PerPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []Project
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing projects: %w", err)
	}
	return out, pag, nil
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

// EffectiveMembersPage returns one page of a project's effective members: its direct
// members, the members of its parent groups, and the members of groups the project
// is shared with (those only when this account can see the group, or is a member of
// the project). A user who is a member several ways appears once, with the highest
// access level. GitLab documents no order for this list.
//
// GET /api/v4/projects/:id/members/all
func (a *ProjectAPI) EffectiveMembersPage(ctx context.Context, projectID string, opts MemberListOpts) ([]ProjectMember, Pagination, error) {
	return a.membersPage(ctx, projectID, "/members/all", opts)
}

// DirectMembersPage returns one page of the project's direct members only: no member
// of a parent group or of a group the project is shared with.
//
// GET /api/v4/projects/:id/members
func (a *ProjectAPI) DirectMembersPage(ctx context.Context, projectID string, opts MemberListOpts) ([]ProjectMember, Pagination, error) {
	return a.membersPage(ctx, projectID, "/members", opts)
}

func (a *ProjectAPI) membersPage(ctx context.Context, projectID, suffix string, opts MemberListOpts) ([]ProjectMember, Pagination, error) {
	path := a.client.APIPath("/projects/"+EncodeProjectPath(projectID)+suffix) + "?" + opts.query().Encode()
	data, pag, err := GetPage(ctx, a.client, path, opts.Page, opts.PerPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []ProjectMember
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing project members: %w", err)
	}
	return out, pag, nil
}

// GetNamespace returns a group or user namespace by ID or full path.
//
// GET /api/v4/namespaces/:id
func (a *ProjectAPI) GetNamespace(ctx context.Context, idOrPath string) (*Namespace, error) {
	data, err := a.client.Get(ctx, a.client.APIPath("/namespaces/"+EncodeProjectPath(idOrPath)))
	if err != nil {
		return nil, err
	}
	var ns Namespace
	if err := json.Unmarshal(data, &ns); err != nil {
		return nil, fmt.Errorf("parsing namespace: %w", err)
	}
	return &ns, nil
}
