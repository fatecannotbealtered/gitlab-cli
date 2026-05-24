package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// MergeRequestAPI wraps Merge-Request-related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/merge_requests/
//
// Methods are implemented in this file by Phase 1 Wave A.
type MergeRequestAPI struct{ client *Client }

// ─── DTOs ────────────────────────────────────────────────────────────────────

// MergeRequest mirrors the GitLab MR object (subset of fields the CLI surfaces).
type MergeRequest struct {
	IID          int      `json:"iid"`
	Title        string   `json:"title"`
	State        string   `json:"state"`
	SourceBranch string   `json:"source_branch"`
	TargetBranch string   `json:"target_branch"`
	WebURL       string   `json:"web_url"`
	Description  string   `json:"description"`
	Draft        bool     `json:"draft"`
	Squash       bool     `json:"squash"`
	Author       *User    `json:"author"`
	Assignee     *User    `json:"assignee"`
	Labels       []string `json:"labels"`
	SHA          string   `json:"sha"`
	MergeStatus  string   `json:"merge_status"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}

// MergeRequestNote mirrors a GitLab MR note (comment).
type MergeRequestNote struct {
	ID        int    `json:"id"`
	Body      string `json:"body"`
	Author    *User  `json:"author"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	System    bool   `json:"system"`
}

// MergeRequestListOpts holds query parameters for listing MRs.
type MergeRequestListOpts struct {
	State        string // opened|closed|merged|all
	AssigneeUser string
	AuthorUser   string
	Labels       string // comma-separated
	Search       string
	SourceBranch string
	TargetBranch string
	Limit        int
}

func (o *MergeRequestListOpts) encode() string {
	if o == nil {
		return ""
	}
	params := url.Values{}
	if o.State != "" {
		params.Set("state", o.State)
	}
	if o.AssigneeUser != "" {
		params.Set("assignee_username", o.AssigneeUser)
	}
	if o.AuthorUser != "" {
		params.Set("author_username", o.AuthorUser)
	}
	if o.Labels != "" {
		params.Set("labels", o.Labels)
	}
	if o.Search != "" {
		params.Set("search", o.Search)
	}
	if o.SourceBranch != "" {
		params.Set("source_branch", o.SourceBranch)
	}
	if o.TargetBranch != "" {
		params.Set("target_branch", o.TargetBranch)
	}
	perPage := 20
	if o.Limit > 0 {
		perPage = o.Limit
		if perPage > maxPerPage {
			perPage = maxPerPage
		}
	}
	params.Set("per_page", strconv.Itoa(perPage))
	return params.Encode()
}

// MergeRequestCreateRequest is the POST body for creating an MR.
type MergeRequestCreateRequest struct {
	Title              string `json:"title"`
	SourceBranch       string `json:"source_branch"`
	TargetBranch       string `json:"target_branch"`
	Description        string `json:"description,omitempty"`
	AssigneeID         int    `json:"assignee_id,omitempty"`
	Labels             string `json:"labels,omitempty"`
	Draft              bool   `json:"draft,omitempty"`
	Squash             bool   `json:"squash,omitempty"`
	RemoveSourceBranch bool   `json:"remove_source_branch,omitempty"`
}

// MergeRequestUpdateRequest is the PUT body for updating an MR.
type MergeRequestUpdateRequest struct {
	Title        string `json:"title,omitempty"`
	Description  string `json:"description,omitempty"`
	AssigneeIDs  []int  `json:"assignee_ids,omitempty"`
	AddLabels    string `json:"add_labels,omitempty"`
	RemoveLabels string `json:"remove_labels,omitempty"`
	TargetBranch string `json:"target_branch,omitempty"`
	StateEvent   string `json:"state_event,omitempty"` // "close" or "reopen"
}

// MergeRequestMergeRequest is the PUT body for merging an MR.
type MergeRequestMergeRequest struct {
	Squash                   bool   `json:"squash,omitempty"`
	ShouldRemoveSourceBranch bool   `json:"should_remove_source_branch,omitempty"`
	MergeCommitMessage       string `json:"merge_commit_message,omitempty"`
	SHA                      string `json:"sha,omitempty"`
}

// ─── Methods ─────────────────────────────────────────────────────────────────

func (a *MergeRequestAPI) projectPath(projectID string) string {
	return "/projects/" + EncodeProjectPath(projectID) + "/merge_requests"
}

// List returns merge requests for a project.
func (a *MergeRequestAPI) List(ctx context.Context, projectID string, opts *MergeRequestListOpts) ([]MergeRequest, error) {
	path := a.client.APIPath(a.projectPath(projectID))
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
	var out []MergeRequest
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing merge requests: %w", err)
	}
	return out, nil
}

// Get returns a single MR by IID.
func (a *MergeRequestAPI) Get(ctx context.Context, projectID string, iid int) (*MergeRequest, error) {
	path := a.client.APIPath(a.projectPath(projectID) + "/" + strconv.Itoa(iid))
	data, err := a.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	var mr MergeRequest
	if err := json.Unmarshal(data, &mr); err != nil {
		return nil, fmt.Errorf("parsing merge request: %w", err)
	}
	return &mr, nil
}

// Create creates a new MR.
func (a *MergeRequestAPI) Create(ctx context.Context, projectID string, req *MergeRequestCreateRequest) (*MergeRequest, error) {
	path := a.client.APIPath(a.projectPath(projectID))
	data, err := a.client.Post(ctx, path, req)
	if err != nil {
		return nil, err
	}
	var mr MergeRequest
	if err := json.Unmarshal(data, &mr); err != nil {
		return nil, fmt.Errorf("parsing created merge request: %w", err)
	}
	return &mr, nil
}

// Update updates an existing MR.
func (a *MergeRequestAPI) Update(ctx context.Context, projectID string, iid int, req *MergeRequestUpdateRequest) (*MergeRequest, error) {
	path := a.client.APIPath(a.projectPath(projectID) + "/" + strconv.Itoa(iid))
	data, err := a.client.Put(ctx, path, req)
	if err != nil {
		return nil, err
	}
	var mr MergeRequest
	if err := json.Unmarshal(data, &mr); err != nil {
		return nil, fmt.Errorf("parsing updated merge request: %w", err)
	}
	return &mr, nil
}

// Merge merges an MR.
func (a *MergeRequestAPI) Merge(ctx context.Context, projectID string, iid int, req *MergeRequestMergeRequest) (*MergeRequest, error) {
	path := a.client.APIPath(a.projectPath(projectID) + "/" + strconv.Itoa(iid) + "/merge")
	data, err := a.client.Put(ctx, path, req)
	if err != nil {
		return nil, err
	}
	var mr MergeRequest
	if err := json.Unmarshal(data, &mr); err != nil {
		return nil, fmt.Errorf("parsing merged merge request: %w", err)
	}
	return &mr, nil
}

// Close closes an MR via state_event.
func (a *MergeRequestAPI) Close(ctx context.Context, projectID string, iid int) (*MergeRequest, error) {
	return a.Update(ctx, projectID, iid, &MergeRequestUpdateRequest{StateEvent: "close"})
}

// Reopen reopens an MR via state_event.
func (a *MergeRequestAPI) Reopen(ctx context.Context, projectID string, iid int) (*MergeRequest, error) {
	return a.Update(ctx, projectID, iid, &MergeRequestUpdateRequest{StateEvent: "reopen"})
}

// Approve approves an MR.
func (a *MergeRequestAPI) Approve(ctx context.Context, projectID string, iid int) error {
	path := a.client.APIPath(a.projectPath(projectID) + "/" + strconv.Itoa(iid) + "/approve")
	_, err := a.client.Post(ctx, path, map[string]any{})
	return err
}

// Unapprove removes approval from an MR.
func (a *MergeRequestAPI) Unapprove(ctx context.Context, projectID string, iid int) error {
	path := a.client.APIPath(a.projectPath(projectID) + "/" + strconv.Itoa(iid) + "/unapprove")
	_, err := a.client.Post(ctx, path, map[string]any{})
	return err
}

// GetRawDiff returns the unified diff text for an MR.
func (a *MergeRequestAPI) GetRawDiff(ctx context.Context, projectID string, iid int) (string, error) {
	path := a.client.APIPath(a.projectPath(projectID) + "/" + strconv.Itoa(iid) + "/raw_diffs")
	data, err := a.client.Get(ctx, path)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), "\n"), nil
}

// ListNotes returns comments for an MR.
func (a *MergeRequestAPI) ListNotes(ctx context.Context, projectID string, iid, limit int) ([]MergeRequestNote, error) {
	path := a.client.APIPath(a.projectPath(projectID) + "/" + strconv.Itoa(iid) + "/notes")
	data, _, err := PaginateGET(ctx, a.client, path+"?per_page="+strconv.Itoa(listPerPage(limit)), limit)
	if err != nil {
		return nil, err
	}
	var notes []MergeRequestNote
	if err := json.Unmarshal(data, &notes); err != nil {
		return nil, fmt.Errorf("parsing notes: %w", err)
	}
	return notes, nil
}

// AddNote adds a comment to an MR.
func (a *MergeRequestAPI) AddNote(ctx context.Context, projectID string, iid int, body string) (*MergeRequestNote, error) {
	path := a.client.APIPath(a.projectPath(projectID) + "/" + strconv.Itoa(iid) + "/notes")
	data, err := a.client.Post(ctx, path, map[string]string{"body": body})
	if err != nil {
		return nil, err
	}
	var note MergeRequestNote
	if err := json.Unmarshal(data, &note); err != nil {
		return nil, fmt.Errorf("parsing note: %w", err)
	}
	return &note, nil
}

// DeleteNote deletes a comment from an MR.
func (a *MergeRequestAPI) DeleteNote(ctx context.Context, projectID string, iid, noteID int) error {
	path := a.client.APIPath(a.projectPath(projectID) + "/" + strconv.Itoa(iid) + "/notes/" + strconv.Itoa(noteID))
	return a.client.Delete(ctx, path)
}

// listPerPage returns per_page for list helpers (default 20, max 100).
func listPerPage(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > maxPerPage {
		return maxPerPage
	}
	return limit
}
