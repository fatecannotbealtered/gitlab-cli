package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// MilestoneAPI wraps milestone-related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/milestones/
type MilestoneAPI struct{ client *Client }

// Milestone represents a GitLab project milestone.
type Milestone struct {
	ID          int    `json:"id"`
	IID         int    `json:"iid"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	StartDate   string `json:"start_date"`
	DueDate     string `json:"due_date"`
	WebURL      string `json:"web_url"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// MilestoneListOpts holds query parameters for listing milestones.
type MilestoneListOpts struct {
	State string // active|closed|all
	Limit int
}

func (o *MilestoneListOpts) encode() string {
	if o == nil {
		return ""
	}
	q := url.Values{}
	if o.State != "" && o.State != "all" {
		q.Set("state", o.State)
	}
	q.Set("per_page", strconv.Itoa(listPerPage(o.Limit)))
	return q.Encode()
}

// List returns milestones for a project.
//
// GET /api/v4/projects/:id/milestones
func (a *MilestoneAPI) List(ctx context.Context, projectID string, opts *MilestoneListOpts) ([]Milestone, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/milestones")
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
	var out []Milestone
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing milestones: %w", err)
	}
	return out, nil
}

// GetByID returns a single milestone by its ID.
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

// MilestoneUpdateOpts holds parameters for updating a milestone.
type MilestoneUpdateOpts struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	StartDate   string `json:"start_date,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
	StateEvent  string `json:"state_event,omitempty"` // close|activate
}

// Update updates an existing milestone.
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

// Delete deletes a milestone.
//
// DELETE /api/v4/projects/:id/milestones/:milestone_id
func (a *MilestoneAPI) Delete(ctx context.Context, projectID string, milestoneID int) error {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/milestones/" + strconv.Itoa(milestoneID))
	return a.client.Delete(ctx, path)
}
