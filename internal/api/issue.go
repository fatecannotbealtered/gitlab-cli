package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// IssueAPI wraps issue-related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/issues/ and
// https://docs.gitlab.com/api/notes/#issues
type IssueAPI struct{ client *Client }

// Issue is a GitLab project issue as the REST API returns it. IID is the number
// in its web URL (/-/issues/<iid>) and in GitLab's reference (#<iid>); ID is global.
type Issue struct {
	ID             int             `json:"id"`
	IID            int             `json:"iid"`
	ProjectID      int             `json:"project_id"`
	Title          string          `json:"title"`
	Description    string          `json:"description"`
	State          string          `json:"state"` // opened|closed
	Labels         []string        `json:"labels"`
	Assignees      []User          `json:"assignees"`
	Author         *User           `json:"author"`
	Milestone      *IssueMilestone `json:"milestone"`
	Confidential   bool            `json:"confidential"`
	DueDate        string          `json:"due_date"`
	UserNotesCount int             `json:"user_notes_count"`
	WebURL         string          `json:"web_url"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
	ClosedAt       string          `json:"closed_at"`
}

// IssueMilestone is the milestone an issue belongs to, as the issue carries it.
type IssueMilestone struct {
	ID    int    `json:"id"`
	IID   int    `json:"iid"`
	Title string `json:"title"`
}

// IssueNote is a comment on an issue. System notes are the ones GitLab writes
// itself (label and state changes, mentions).
type IssueNote struct {
	ID          int    `json:"id"`
	Body        string `json:"body"`
	Author      *User  `json:"author"`
	System      bool   `json:"system"`
	Internal    bool   `json:"internal"`
	NoteableIID int    `json:"noteable_iid"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// IssueListOpts filters an issue list. Empty fields are not sent.
type IssueListOpts struct {
	State            string   // opened|closed; "" or "all" means every state
	Labels           []string // issues with all of these (GitLab's labels=a,b; None and Any as GitLab takes them)
	Milestone        string   // a milestone title, None or Any
	AuthorUsername   string
	AssigneeUsername string
	AssigneeID       string   // None or Any (no assignee, any assignee)
	Search           string   // GitLab's search, in title and description unless In narrows it
	In               []string // title, description
	IIDs             []int    // only these iids
	OrderBy, Sort    string   // GitLab's order_by (created_at, updated_at, ...) and sort (asc, desc)
	Page, PerPage    int
}

func (o IssueListOpts) query() url.Values {
	q := url.Values{}
	if o.State != "" && o.State != "all" {
		q.Set("state", o.State)
	}
	if len(o.Labels) > 0 {
		q.Set("labels", strings.Join(o.Labels, ","))
	}
	if o.Milestone != "" {
		q.Set("milestone", o.Milestone)
	}
	if o.AuthorUsername != "" {
		q.Set("author_username", o.AuthorUsername)
	}
	if o.AssigneeUsername != "" {
		q.Set("assignee_username", o.AssigneeUsername)
	}
	if o.AssigneeID != "" {
		q.Set("assignee_id", o.AssigneeID)
	}
	if s := strings.TrimSpace(o.Search); s != "" {
		q.Set("search", s)
	}
	if len(o.In) > 0 {
		q.Set("in", strings.Join(o.In, ","))
	}
	for _, iid := range o.IIDs {
		q.Add("iids[]", strconv.Itoa(iid))
	}
	if o.OrderBy != "" {
		q.Set("order_by", o.OrderBy)
	}
	if o.Sort != "" {
		q.Set("sort", o.Sort)
	}
	return q
}

func (a *IssueAPI) issuesPath(projectID string) string {
	return a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/issues")
}

func (a *IssueAPI) issuePath(projectID string, iid int) string {
	return a.issuesPath(projectID) + "/" + strconv.Itoa(iid)
}

// ListPage returns one page of a project's issues.
//
// GET /api/v4/projects/:id/issues
func (a *IssueAPI) ListPage(ctx context.Context, projectID string, opts IssueListOpts) ([]Issue, Pagination, error) {
	path := a.issuesPath(projectID)
	if q := opts.query().Encode(); q != "" {
		path += "?" + q
	}
	data, pag, err := GetPage(ctx, a.client, path, opts.Page, opts.PerPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []Issue
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing issues: %w", err)
	}
	return out, pag, nil
}

// Get returns a single issue by IID.
//
// GET /api/v4/projects/:id/issues/:iid
func (a *IssueAPI) Get(ctx context.Context, projectID string, iid int) (*Issue, error) {
	data, err := a.client.Get(ctx, a.issuePath(projectID, iid))
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
	Labels       string `json:"labels,omitempty"` // comma-separated
	MilestoneID  int    `json:"milestone_id,omitempty"`
	Confidential bool   `json:"confidential,omitempty"`
}

// Create creates a new issue. It is sent once; the client never retries a write.
//
// POST /api/v4/projects/:id/issues
func (a *IssueAPI) Create(ctx context.Context, projectID string, opts IssueCreateOpts) (*Issue, error) {
	data, err := a.client.Post(ctx, a.issuesPath(projectID), opts)
	if err != nil {
		return nil, err
	}
	var out Issue
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing created issue: %w", err)
	}
	return &out, nil
}

// IssueUpdateOpts holds the fields to change; nil and empty fields are not sent.
// AssigneeIDs []int{0} unassigns everyone and MilestoneID 0 removes the milestone,
// as GitLab documents.
type IssueUpdateOpts struct {
	Title        *string `json:"title,omitempty"`
	Description  *string `json:"description,omitempty"`
	AssigneeIDs  *[]int  `json:"assignee_ids,omitempty"`
	AddLabels    string  `json:"add_labels,omitempty"`    // comma-separated
	RemoveLabels string  `json:"remove_labels,omitempty"` // comma-separated
	MilestoneID  *int    `json:"milestone_id,omitempty"`
	StateEvent   string  `json:"state_event,omitempty"` // close|reopen
}

// Update changes an existing issue.
//
// PUT /api/v4/projects/:id/issues/:iid
func (a *IssueAPI) Update(ctx context.Context, projectID string, iid int, opts IssueUpdateOpts) (*Issue, error) {
	data, err := a.client.Put(ctx, a.issuePath(projectID, iid), opts)
	if err != nil {
		return nil, err
	}
	var out Issue
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing updated issue: %w", err)
	}
	return &out, nil
}

// IssueNoteListOpts orders and pages an issue's notes.
type IssueNoteListOpts struct {
	OrderBy, Sort string // created_at|updated_at; asc|desc
	Page, PerPage int
}

// ListNotesPage returns one page of an issue's notes, system notes included.
//
// GET /api/v4/projects/:id/issues/:iid/notes
func (a *IssueAPI) ListNotesPage(ctx context.Context, projectID string, iid int, opts IssueNoteListOpts) ([]IssueNote, Pagination, error) {
	q := url.Values{}
	if opts.OrderBy != "" {
		q.Set("order_by", opts.OrderBy)
	}
	if opts.Sort != "" {
		q.Set("sort", opts.Sort)
	}
	path := a.issuePath(projectID, iid) + "/notes"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	data, pag, err := GetPage(ctx, a.client, path, opts.Page, opts.PerPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []IssueNote
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing notes: %w", err)
	}
	return out, pag, nil
}

// GetNote returns one note of an issue.
//
// GET /api/v4/projects/:id/issues/:iid/notes/:note_id
func (a *IssueAPI) GetNote(ctx context.Context, projectID string, iid, noteID int) (*IssueNote, error) {
	data, err := a.client.Get(ctx, a.issuePath(projectID, iid)+"/notes/"+strconv.Itoa(noteID))
	if err != nil {
		return nil, err
	}
	var out IssueNote
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing note: %w", err)
	}
	return &out, nil
}

// AddNote adds a comment to an issue. It is sent once.
//
// POST /api/v4/projects/:id/issues/:iid/notes
func (a *IssueAPI) AddNote(ctx context.Context, projectID string, iid int, body string) (*IssueNote, error) {
	data, err := a.client.Post(ctx, a.issuePath(projectID, iid)+"/notes", map[string]string{"body": body})
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
	return a.client.Delete(ctx, a.issuePath(projectID, iid)+"/notes/"+strconv.Itoa(noteID))
}
