package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// MilestoneAPI wraps milestone-related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/milestones/
type MilestoneAPI struct{ client *Client }

// Milestone represents a GitLab project milestone. ID is global; IID is the number
// within the project that the web URL (/-/milestones/<iid>) and %<iid> use.
type Milestone struct {
	ID          int    `json:"id"`
	IID         int    `json:"iid"`
	ProjectID   int    `json:"project_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	StartDate   string `json:"start_date"`
	DueDate     string `json:"due_date"`
	Expired     *bool  `json:"expired"`
	WebURL      string `json:"web_url"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// MilestoneListOpts holds query parameters for listing milestones.
type MilestoneListOpts struct {
	State   string // active|closed; "" or "all" means every state
	Search  string // a substring of the title or description
	Title   string // the exact title
	IIDs    []int  // only these iids
	Limit   int    // List only: the number of milestones to return
	Page    int    // ListPage only
	PerPage int    // ListPage only
}

func (o *MilestoneListOpts) query() url.Values {
	q := url.Values{}
	if o == nil {
		return q
	}
	if o.State != "" && o.State != "all" {
		q.Set("state", o.State)
	}
	if s := strings.TrimSpace(o.Search); s != "" {
		q.Set("search", s)
	}
	if o.Title != "" {
		q.Set("title", o.Title)
	}
	for _, iid := range o.IIDs {
		q.Add("iids[]", strconv.Itoa(iid))
	}
	return q
}

// ListPage returns one page of a project's own milestones (not those of its
// groups), newest first.
//
// GET /api/v4/projects/:id/milestones
func (a *MilestoneAPI) ListPage(ctx context.Context, projectID string, opts MilestoneListOpts) ([]Milestone, Pagination, error) {
	path := a.client.APIPath("/projects/"+EncodeProjectPath(projectID)+"/milestones") + "?" + opts.query().Encode()
	data, pag, err := GetPage(ctx, a.client, path, opts.Page, opts.PerPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []Milestone
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing milestones: %w", err)
	}
	return out, pag, nil
}

// List returns up to opts.Limit milestones of a project (all pages merged).
//
// GET /api/v4/projects/:id/milestones
func (a *MilestoneAPI) List(ctx context.Context, projectID string, opts *MilestoneListOpts) ([]Milestone, error) {
	limit := 0
	q := url.Values{}
	if opts != nil {
		limit = opts.Limit
		q = opts.query()
	}
	q.Set("per_page", strconv.Itoa(listPerPage(limit)))
	path := a.client.APIPath("/projects/"+EncodeProjectPath(projectID)+"/milestones") + "?" + q.Encode()
	data, _, err := PaginateGET(ctx, a.client, path, limit)
	if err != nil {
		return nil, err
	}
	var out []Milestone
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing milestones: %w", err)
	}
	return out, nil
}

// GetByID returns a single milestone by its global ID.
//
// GET /api/v4/projects/:id/milestones/:milestone_id
func (a *MilestoneAPI) GetByID(ctx context.Context, projectID string, milestoneID int) (*Milestone, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/milestones/" + strconv.Itoa(milestoneID))
	data, err := a.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	var out Milestone
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing milestone: %w", err)
	}
	return &out, nil
}

// MilestoneCreateOpts holds parameters for creating a milestone.
type MilestoneCreateOpts struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	StartDate   string `json:"start_date,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
}

// Create creates a new milestone.
//
// POST /api/v4/projects/:id/milestones
func (a *MilestoneAPI) Create(ctx context.Context, projectID string, opts MilestoneCreateOpts) (*Milestone, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/milestones")
	data, err := a.client.Post(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	var out Milestone
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing created milestone: %w", err)
	}
	return &out, nil
}

// MilestoneUpdateOpts holds the fields to change; nil fields are left as they are.
type MilestoneUpdateOpts struct {
	Title       *string `json:"title,omitempty"`
	Description *string `json:"description,omitempty"`
	StartDate   *string `json:"start_date,omitempty"`
	DueDate     *string `json:"due_date,omitempty"`
	StateEvent  *string `json:"state_event,omitempty"` // close|activate
}

// Update changes a milestone, found by its global ID.
//
// PUT /api/v4/projects/:id/milestones/:milestone_id
func (a *MilestoneAPI) Update(ctx context.Context, projectID string, milestoneID int, opts MilestoneUpdateOpts) (*Milestone, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/milestones/" + strconv.Itoa(milestoneID))
	data, err := a.client.Put(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	var out Milestone
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing updated milestone: %w", err)
	}
	return &out, nil
}

// Delete deletes a milestone, found by its global ID.
//
// DELETE /api/v4/projects/:id/milestones/:milestone_id
func (a *MilestoneAPI) Delete(ctx context.Context, projectID string, milestoneID int) error {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/milestones/" + strconv.Itoa(milestoneID))
	return a.client.Delete(ctx, path)
}
