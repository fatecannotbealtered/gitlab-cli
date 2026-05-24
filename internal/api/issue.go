package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// IssueAPI wraps issue-related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/issues/
type IssueAPI struct{ client *Client }

// Issue represents a GitLab project issue.
type Issue struct {
	IID          int      `json:"iid"`
	ID           int      `json:"id"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	State        string   `json:"state"`
	Labels       []string `json:"labels"`
	Confidential bool     `json:"confidential"`
	WebURL       string   `json:"web_url"`
	Author       *User    `json:"author"`
	Assignee     *User    `json:"assignee"`
	Milestone    *struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
	} `json:"milestone"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// IssueNote represents a comment on an issue.
type IssueNote struct {
	ID        int    `json:"id"`
	Body      string `json:"body"`
	Author    *User  `json:"author"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	System    bool   `json:"system"`
}

// IssueListOpts holds query parameters for listing issues.
type IssueListOpts struct {
	State            string // opened|closed|all
	AssigneeUsername string
	AuthorUsername   string
	Labels           string // comma-separated
	Search           string
	Milestone        string
	Limit            int
}

func (o *IssueListOpts) encode() string {
	if o == nil {
		return ""
	}
	q := url.Values{}
	if o.State != "" && o.State != "all" {
		q.Set("state", o.State)
	}
	if o.AssigneeUsername != "" {
		q.Set("assignee_username", o.AssigneeUsername)
	}
	if o.AuthorUsername != "" {
		q.Set("author_username", o.AuthorUsername)
	}
	if o.Labels != "" {
		q.Set("labels", o.Labels)
	}
	if o.Search != "" {
		q.Set("search", o.Search)
	}
	if o.Milestone != "" {
		q.Set("milestone", o.Milestone)
	}
	q.Set("per_page", strconv.Itoa(listPerPage(o.Limit)))
	return q.Encode()
}

// List returns issues for a project.
//
// GET /api/v4/projects/:id/issues
func (a *IssueAPI) List(ctx context.Context, projectID string, opts *IssueListOpts) ([]Issue, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/issues")
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
	var out []Issue
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing issues: %w", err)
	}
	return out, nil
}

// Get returns a single issue by IID.
//
// GET /api/v4/projects/:id/issues/:iid
func (a *IssueAPI) Get(ctx context.Context, projectID string, iid int) (*Issue, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/issues/" + strconv.Itoa(iid))
	data, err := a.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	var out Issue
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing issue: %w", err)
	}
	return &out, nil
}

// IssueCreateOpts holds parameters for creating an issue.
type IssueCreateOpts struct {
	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	AssigneeIDs  []int  `json:"assignee_ids,omitempty"`
	Labels       string `json:"labels,omitempty"`
	MilestoneID  int    `json:"milestone_id,omitempty"`
	Confidential bool   `json:"confidential,omitempty"`
}

// Create creates a new issue.
//
// POST /api/v4/projects/:id/issues
func (a *IssueAPI) Create(ctx context.Context, projectID string, opts IssueCreateOpts) (*Issue, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/issues")
	data, err := a.client.Post(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	var out Issue
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing created issue: %w", err)
	}
	return &out, nil
}

// IssueUpdateOpts holds parameters for updating an issue.
type IssueUpdateOpts struct {
	Title        string `json:"title,omitempty"`
	Description  string `json:"description,omitempty"`
	AssigneeIDs  []int  `json:"assignee_ids,omitempty"`
	AddLabels    string `json:"add_labels,omitempty"`
	RemoveLabels string `json:"remove_labels,omitempty"`
	MilestoneID  int    `json:"milestone_id,omitempty"`
	StateEvent   string `json:"state_event,omitempty"` // close|reopen
}

// Update updates an existing issue.
//
// PUT /api/v4/projects/:id/issues/:iid
func (a *IssueAPI) Update(ctx context.Context, projectID string, iid int, opts IssueUpdateOpts) (*Issue, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/issues/" + strconv.Itoa(iid))
	data, err := a.client.Put(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	var out Issue
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing updated issue: %w", err)
	}
	return &out, nil
}

// ListNotes returns comments on an issue.
//
// GET /api/v4/projects/:id/issues/:iid/notes
func (a *IssueAPI) ListNotes(ctx context.Context, projectID string, iid, limit int) ([]IssueNote, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/issues/" + strconv.Itoa(iid) + "/notes")
	data, _, err := PaginateGET(ctx, a.client, path+"?per_page="+strconv.Itoa(listPerPage(limit)), limit)
	if err != nil {
		return nil, err
	}
	var out []IssueNote
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing notes: %w", err)
	}
	return out, nil
}

// AddNote adds a comment to an issue.
//
// POST /api/v4/projects/:id/issues/:iid/notes
func (a *IssueAPI) AddNote(ctx context.Context, projectID string, iid int, body string) (*IssueNote, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/issues/" + strconv.Itoa(iid) + "/notes")
	data, err := a.client.Post(ctx, path, map[string]string{"body": body})
	if err != nil {
		return nil, err
	}
	var out IssueNote
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing note: %w", err)
	}
	return &out, nil
}

// DeleteNote deletes a comment from an issue.
//
// DELETE /api/v4/projects/:id/issues/:iid/notes/:note_id
func (a *IssueAPI) DeleteNote(ctx context.Context, projectID string, iid, noteID int) error {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/issues/" + strconv.Itoa(iid) + "/notes/" + strconv.Itoa(noteID))
	return a.client.Delete(ctx, path)
}
