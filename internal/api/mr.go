package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// MergeRequestAPI wraps the merge request endpoints: the merge requests
// themselves, their approvals, diffs, notes (comments) and discussions (threads).
//
// Endpoint reference: https://docs.gitlab.com/api/merge_requests/,
// https://docs.gitlab.com/api/notes/, https://docs.gitlab.com/api/discussions/
type MergeRequestAPI struct{ client *Client }

// ─── DTOs ────────────────────────────────────────────────────────────────────

// MergeRequest mirrors the GitLab MR object (the fields the CLI surfaces). The list
// endpoint leaves out HeadPipeline and DiffRefs; a single GET has them.
type MergeRequest struct {
	ID                  int      `json:"id"`
	IID                 int      `json:"iid"`
	Title               string   `json:"title"`
	Description         string   `json:"description"`
	State               string   `json:"state"`
	Draft               bool     `json:"draft"`
	SourceBranch        string   `json:"source_branch"`
	TargetBranch        string   `json:"target_branch"`
	WebURL              string   `json:"web_url"`
	Squash              bool     `json:"squash"`
	Author              *User    `json:"author"`
	Assignee            *User    `json:"assignee"`
	Assignees           []User   `json:"assignees"`
	Reviewers           []User   `json:"reviewers"`
	Labels              []string `json:"labels"`
	SHA                 string   `json:"sha"`
	MergeStatus         string   `json:"merge_status"`
	DetailedMergeStatus string   `json:"detailed_merge_status"`
	HasConflicts        bool     `json:"has_conflicts"`
	CreatedAt           string   `json:"created_at"`
	UpdatedAt           string   `json:"updated_at"`
	MergedAt            string   `json:"merged_at"`
	// HeadPipeline is the MR's latest pipeline; GitLab returns it on a single GET only.
	HeadPipeline *MergeRequestPipeline `json:"head_pipeline"`
	// DiffRefs carries the three SHAs that anchor a diff position (single GET only);
	// an inline discussion's position is invalid without them.
	DiffRefs *DiffRefs `json:"diff_refs"`
}

// MergeRequestPipeline is the head pipeline summary of a single-MR response.
type MergeRequestPipeline struct {
	ID     int    `json:"id"`
	Status string `json:"status"`
	WebURL string `json:"web_url"`
}

// DiffRefs are the base/start/head commit SHAs a diff position is anchored to.
type DiffRefs struct {
	BaseSHA  string `json:"base_sha"`
	StartSHA string `json:"start_sha"`
	HeadSHA  string `json:"head_sha"`
}

// MergeRequestNote mirrors a GitLab MR note (comment). Position is set on diff
// notes (inline review comments).
type MergeRequestNote struct {
	ID         int           `json:"id"`
	Type       string        `json:"type"`
	Body       string        `json:"body"`
	Author     *User         `json:"author"`
	CreatedAt  string        `json:"created_at"`
	UpdatedAt  string        `json:"updated_at"`
	System     bool          `json:"system"`
	Resolvable bool          `json:"resolvable"`
	Resolved   bool          `json:"resolved"`
	Position   *DiffPosition `json:"position"`
}

// MergeRequestDiscussion mirrors a GitLab MR discussion thread: an ordered set of
// notes. IndividualNote is true for a standalone comment, false for a thread.
type MergeRequestDiscussion struct {
	ID             string             `json:"id"`
	IndividualNote bool               `json:"individual_note"`
	Notes          []MergeRequestNote `json:"notes"`
}

// MergeRequestApprovals is an MR's approval state (GET .../approvals, and the
// answer of POST .../approve).
type MergeRequestApprovals struct {
	Approved      bool `json:"approved"`
	ApprovalsLeft int  `json:"approvals_left"`
	ApprovedBy    []struct {
		User User `json:"user"`
	} `json:"approved_by"`
}

// Usernames lists who approved, in GitLab's order.
func (a *MergeRequestApprovals) Usernames() []string {
	out := []string{}
	for _, b := range a.ApprovedBy {
		if b.User.Username != "" {
			out = append(out, b.User.Username)
		}
	}
	return out
}

// MergeRequestDiff is one file of an MR's diff (GET .../diffs). Diff holds the
// file's hunks without the ---/+++ header; it is empty when TooLarge.
type MergeRequestDiff struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	Diff        string `json:"diff"`
	NewFile     bool   `json:"new_file"`
	RenamedFile bool   `json:"renamed_file"`
	DeletedFile bool   `json:"deleted_file"`
	TooLarge    bool   `json:"too_large"`
	Collapsed   bool   `json:"collapsed"`
}

// DiffPosition anchors a discussion to one line of an MR diff. GitLab requires
// the three diff_refs SHAs and a text position (new_path/old_path and at least one
// of new_line/old_line). The lines are pointers, so an unset line is left out:
// GitLab rejects new_line=0 on a position that should only carry old_line.
type DiffPosition struct {
	BaseSHA      string `json:"base_sha"`
	StartSHA     string `json:"start_sha"`
	HeadSHA      string `json:"head_sha"`
	PositionType string `json:"position_type"`
	NewPath      string `json:"new_path,omitempty"`
	OldPath      string `json:"old_path,omitempty"`
	NewLine      *int   `json:"new_line,omitempty"`
	OldLine      *int   `json:"old_line,omitempty"`
}

// ─── Options ─────────────────────────────────────────────────────────────────

// MergeRequestListOpts holds the query parameters of a merge request list.
type MergeRequestListOpts struct {
	State            string   // opened|closed|merged|locked; "" or "all" means every state
	AuthorUsername   string   // author_username
	AssigneeUsername string   // assignee_username
	ReviewerUsername string   // reviewer_username
	Labels           []string // only MRs with every one of these labels
	Search           string   // a substring of the title or description
	SourceBranch     string
	TargetBranch     string
	OrderBy          string // created_at|updated_at|title; "" is GitLab's default (created_at)
	Sort             string // asc|desc
	Page             int
	PerPage          int
}

func (o MergeRequestListOpts) query() url.Values {
	q := url.Values{}
	if o.State != "" && o.State != "all" {
		q.Set("state", o.State)
	}
	set := func(key, value string) {
		if v := strings.TrimSpace(value); v != "" {
			q.Set(key, v)
		}
	}
	set("author_username", o.AuthorUsername)
	set("assignee_username", o.AssigneeUsername)
	set("reviewer_username", o.ReviewerUsername)
	set("labels", strings.Join(o.Labels, ","))
	set("search", o.Search)
	set("source_branch", o.SourceBranch)
	set("target_branch", o.TargetBranch)
	set("order_by", o.OrderBy)
	set("sort", o.Sort)
	return q
}

// MergeRequestCreateRequest is the POST body for creating an MR. GitLab has no
// draft parameter: a draft is a title starting with "Draft:".
type MergeRequestCreateRequest struct {
	Title              string `json:"title"`
	SourceBranch       string `json:"source_branch"`
	TargetBranch       string `json:"target_branch"`
	Description        string `json:"description,omitempty"`
	AssigneeIDs        []int  `json:"assignee_ids,omitempty"`
	ReviewerIDs        []int  `json:"reviewer_ids,omitempty"`
	Labels             string `json:"labels,omitempty"`
	Squash             bool   `json:"squash,omitempty"`
	RemoveSourceBranch bool   `json:"remove_source_branch,omitempty"`
}

// MergeRequestUpdateOpts holds the fields to change; nil fields are left as they
// are. An empty AssigneeIDs or ReviewerIDs (not nil) clears the list.
type MergeRequestUpdateOpts struct {
	Title        *string `json:"title,omitempty"`
	Description  *string `json:"description,omitempty"`
	AssigneeIDs  *[]int  `json:"assignee_ids,omitempty"`
	ReviewerIDs  *[]int  `json:"reviewer_ids,omitempty"`
	AddLabels    string  `json:"add_labels,omitempty"`
	RemoveLabels string  `json:"remove_labels,omitempty"`
	TargetBranch *string `json:"target_branch,omitempty"`
	StateEvent   *string `json:"state_event,omitempty"` // close|reopen
}

// MergeRequestMergeOpts is the PUT body of a merge. SHA, when set, makes GitLab
// refuse the merge if the source branch's head moved (HTTP 409).
type MergeRequestMergeOpts struct {
	SHA                      string `json:"sha,omitempty"`
	Squash                   *bool  `json:"squash,omitempty"`
	ShouldRemoveSourceBranch *bool  `json:"should_remove_source_branch,omitempty"`
	MergeCommitMessage       string `json:"merge_commit_message,omitempty"`
	SquashCommitMessage      string `json:"squash_commit_message,omitempty"`
}

// ─── Merge requests ──────────────────────────────────────────────────────────

func (a *MergeRequestAPI) projectPath(projectID string) string {
	return "/projects/" + EncodeProjectPath(projectID) + "/merge_requests"
}

func (a *MergeRequestAPI) mrPath(projectID string, iid int, rest string) string {
	return a.client.APIPath(a.projectPath(projectID) + "/" + strconv.Itoa(iid) + rest)
}

// ListPage returns one page of a project's merge requests.
//
// GET /api/v4/projects/:id/merge_requests
func (a *MergeRequestAPI) ListPage(ctx context.Context, projectID string, opts MergeRequestListOpts) ([]MergeRequest, Pagination, error) {
	path := a.client.APIPath(a.projectPath(projectID)) + "?" + opts.query().Encode()
	data, pag, err := GetPage(ctx, a.client, path, opts.Page, opts.PerPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []MergeRequest
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing merge requests: %w", err)
	}
	return out, pag, nil
}

// Get returns a single MR by IID, with its head pipeline and diff refs.
//
// GET /api/v4/projects/:id/merge_requests/:iid
func (a *MergeRequestAPI) Get(ctx context.Context, projectID string, iid int) (*MergeRequest, error) {
	return a.decodeMR(a.client.Get(ctx, a.mrPath(projectID, iid, "")))
}

// Create creates a new MR.
//
// POST /api/v4/projects/:id/merge_requests
func (a *MergeRequestAPI) Create(ctx context.Context, projectID string, req *MergeRequestCreateRequest) (*MergeRequest, error) {
	return a.decodeMR(a.client.Post(ctx, a.client.APIPath(a.projectPath(projectID)), req))
}

// Update changes an MR; only the fields set in opts are sent.
//
// PUT /api/v4/projects/:id/merge_requests/:iid
func (a *MergeRequestAPI) Update(ctx context.Context, projectID string, iid int, opts MergeRequestUpdateOpts) (*MergeRequest, error) {
	return a.decodeMR(a.client.Put(ctx, a.mrPath(projectID, iid, ""), opts))
}

// Merge merges an MR.
//
// PUT /api/v4/projects/:id/merge_requests/:iid/merge
func (a *MergeRequestAPI) Merge(ctx context.Context, projectID string, iid int, opts MergeRequestMergeOpts) (*MergeRequest, error) {
	return a.decodeMR(a.client.Put(ctx, a.mrPath(projectID, iid, "/merge"), opts))
}

func (a *MergeRequestAPI) decodeMR(data []byte, err error) (*MergeRequest, error) {
	if err != nil {
		return nil, err
	}
	var mr MergeRequest
	if err := json.Unmarshal(data, &mr); err != nil {
		return nil, fmt.Errorf("parsing merge request: %w", err)
	}
	return &mr, nil
}

// ─── Approvals ───────────────────────────────────────────────────────────────

// Approvals returns an MR's approval state.
//
// GET /api/v4/projects/:id/merge_requests/:iid/approvals
func (a *MergeRequestAPI) Approvals(ctx context.Context, projectID string, iid int) (*MergeRequestApprovals, error) {
	return decodeApprovals(a.client.Get(ctx, a.mrPath(projectID, iid, "/approvals")))
}

// Approve approves an MR as the token's user. sha, when set, must be the MR's
// head, or GitLab refuses (HTTP 409).
//
// POST /api/v4/projects/:id/merge_requests/:iid/approve
func (a *MergeRequestAPI) Approve(ctx context.Context, projectID string, iid int, sha string) (*MergeRequestApprovals, error) {
	body := map[string]any{}
	if sha != "" {
		body["sha"] = sha
	}
	return decodeApprovals(a.client.Post(ctx, a.mrPath(projectID, iid, "/approve"), body))
}

// Unapprove withdraws the token's user's approval.
//
// POST /api/v4/projects/:id/merge_requests/:iid/unapprove
func (a *MergeRequestAPI) Unapprove(ctx context.Context, projectID string, iid int) error {
	_, err := a.client.Post(ctx, a.mrPath(projectID, iid, "/unapprove"), map[string]any{})
	return err
}

func decodeApprovals(data []byte, err error) (*MergeRequestApprovals, error) {
	if err != nil {
		return nil, err
	}
	var out MergeRequestApprovals
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing approvals: %w", err)
	}
	return &out, nil
}

// ─── Diffs ───────────────────────────────────────────────────────────────────

// DiffsPage returns one page of an MR's changed files with their diffs.
//
// GET /api/v4/projects/:id/merge_requests/:iid/diffs
func (a *MergeRequestAPI) DiffsPage(ctx context.Context, projectID string, iid, page, perPage int) ([]MergeRequestDiff, Pagination, error) {
	data, pag, err := GetPage(ctx, a.client, a.mrPath(projectID, iid, "/diffs"), page, perPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []MergeRequestDiff
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing diffs: %w", err)
	}
	return out, pag, nil
}

// RawDiff returns an MR's unified diff as text.
//
// GET /api/v4/projects/:id/merge_requests/:iid/raw_diffs
func (a *MergeRequestAPI) RawDiff(ctx context.Context, projectID string, iid int) (string, error) {
	data, err := a.client.Get(ctx, a.mrPath(projectID, iid, "/raw_diffs"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ─── Notes ───────────────────────────────────────────────────────────────────

// NotesPage returns one page of an MR's notes, oldest first, system notes
// included (GitLab cannot leave them out).
//
// GET /api/v4/projects/:id/merge_requests/:iid/notes
func (a *MergeRequestAPI) NotesPage(ctx context.Context, projectID string, iid, page, perPage int) ([]MergeRequestNote, Pagination, error) {
	data, pag, err := GetPage(ctx, a.client, a.mrPath(projectID, iid, "/notes")+"?order_by=created_at&sort=asc", page, perPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []MergeRequestNote
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing notes: %w", err)
	}
	return out, pag, nil
}

// GetNote returns one note of an MR.
//
// GET /api/v4/projects/:id/merge_requests/:iid/notes/:note_id
func (a *MergeRequestAPI) GetNote(ctx context.Context, projectID string, iid, noteID int) (*MergeRequestNote, error) {
	return decodeNote(a.client.Get(ctx, a.mrPath(projectID, iid, "/notes/"+strconv.Itoa(noteID))))
}

// AddNote adds a comment to an MR.
//
// POST /api/v4/projects/:id/merge_requests/:iid/notes
func (a *MergeRequestAPI) AddNote(ctx context.Context, projectID string, iid int, body string) (*MergeRequestNote, error) {
	return decodeNote(a.client.Post(ctx, a.mrPath(projectID, iid, "/notes"), map[string]string{"body": body}))
}

// DeleteNote deletes a comment from an MR.
//
// DELETE /api/v4/projects/:id/merge_requests/:iid/notes/:note_id
func (a *MergeRequestAPI) DeleteNote(ctx context.Context, projectID string, iid, noteID int) error {
	return a.client.Delete(ctx, a.mrPath(projectID, iid, "/notes/"+strconv.Itoa(noteID)))
}

func decodeNote(data []byte, err error) (*MergeRequestNote, error) {
	if err != nil {
		return nil, err
	}
	var note MergeRequestNote
	if err := json.Unmarshal(data, &note); err != nil {
		return nil, fmt.Errorf("parsing note: %w", err)
	}
	return &note, nil
}

// ─── Discussions ─────────────────────────────────────────────────────────────

// DiscussionsPage returns one page of an MR's discussions, each with its notes.
//
// GET /api/v4/projects/:id/merge_requests/:iid/discussions
func (a *MergeRequestAPI) DiscussionsPage(ctx context.Context, projectID string, iid, page, perPage int) ([]MergeRequestDiscussion, Pagination, error) {
	data, pag, err := GetPage(ctx, a.client, a.mrPath(projectID, iid, "/discussions"), page, perPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []MergeRequestDiscussion
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing discussions: %w", err)
	}
	return out, pag, nil
}

// GetDiscussion returns one discussion with all its notes.
//
// GET /api/v4/projects/:id/merge_requests/:iid/discussions/:discussion_id
func (a *MergeRequestAPI) GetDiscussion(ctx context.Context, projectID string, iid int, discussionID string) (*MergeRequestDiscussion, error) {
	return decodeDiscussion(a.client.Get(ctx, a.mrPath(projectID, iid, "/discussions/"+url.PathEscape(discussionID))))
}

// CreateDiscussion starts a discussion thread on an MR; with position it is an
// inline (diff-anchored) review comment.
//
// POST /api/v4/projects/:id/merge_requests/:iid/discussions
func (a *MergeRequestAPI) CreateDiscussion(ctx context.Context, projectID string, iid int, body string, position *DiffPosition) (*MergeRequestDiscussion, error) {
	payload := map[string]any{"body": body}
	if position != nil {
		payload["position"] = position
	}
	return decodeDiscussion(a.client.Post(ctx, a.mrPath(projectID, iid, "/discussions"), payload))
}

// ReplyDiscussion adds a note to an existing discussion thread.
//
// POST /api/v4/projects/:id/merge_requests/:iid/discussions/:discussion_id/notes
func (a *MergeRequestAPI) ReplyDiscussion(ctx context.Context, projectID string, iid int, discussionID, body string) (*MergeRequestNote, error) {
	return decodeNote(a.client.Post(ctx, a.mrPath(projectID, iid, "/discussions/"+url.PathEscape(discussionID)+"/notes"),
		map[string]string{"body": body}))
}

// ResolveDiscussion marks a resolvable discussion resolved or unresolved; GitLab
// refuses it on a thread that cannot be resolved.
//
// PUT /api/v4/projects/:id/merge_requests/:iid/discussions/:discussion_id
func (a *MergeRequestAPI) ResolveDiscussion(ctx context.Context, projectID string, iid int, discussionID string, resolved bool) (*MergeRequestDiscussion, error) {
	return decodeDiscussion(a.client.Put(ctx, a.mrPath(projectID, iid, "/discussions/"+url.PathEscape(discussionID)),
		map[string]bool{"resolved": resolved}))
}

func decodeDiscussion(data []byte, err error) (*MergeRequestDiscussion, error) {
	if err != nil {
		return nil, err
	}
	var d MergeRequestDiscussion
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parsing discussion: %w", err)
	}
	return &d, nil
}
