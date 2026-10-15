package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// LabelAPI wraps label-related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/labels/
type LabelAPI struct{ client *Client }

// Label represents a GitLab project label (project labels and, in project lists,
// the labels inherited from its groups).
type Label struct {
	ID                     int    `json:"id"`
	Name                   string `json:"name"`
	Color                  string `json:"color"`
	TextColor              string `json:"text_color"`
	Description            string `json:"description"`
	Priority               *int   `json:"priority"`
	IsProjectLabel         bool   `json:"is_project_label"`
	Subscribed             bool   `json:"subscribed"`
	OpenIssuesCount        *int   `json:"open_issues_count"`
	ClosedIssuesCount      *int   `json:"closed_issues_count"`
	OpenMergeRequestsCount *int   `json:"open_merge_requests_count"`
}

// LabelListOpts filters a label list.
type LabelListOpts struct {
	Search     string // GitLab's search: a substring of the label name
	WithCounts bool   // add issue and merge request counts (slower on GitLab's side)
	Page       int
	PerPage    int
}

// ListPage returns one page of a project's labels, its own and the ones it
// inherits from its groups (include_ancestor_groups, GitLab's default).
//
// GET /api/v4/projects/:id/labels
func (a *LabelAPI) ListPage(ctx context.Context, projectID string, opts LabelListOpts) ([]Label, Pagination, error) {
	q := url.Values{}
	q.Set("include_ancestor_groups", "true")
	if opts.WithCounts {
		q.Set("with_counts", "true")
	}
	if s := strings.TrimSpace(opts.Search); s != "" {
		q.Set("search", s)
	}
	path := a.client.APIPath("/projects/"+EncodeProjectPath(projectID)+"/labels") + "?" + q.Encode()
	data, pag, err := GetPage(ctx, a.client, path, opts.Page, opts.PerPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []Label
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing labels: %w", err)
	}
	return out, pag, nil
}

// List returns up to limit labels of a project (all pages merged).
//
// GET /api/v4/projects/:id/labels
func (a *LabelAPI) List(ctx context.Context, projectID string, limit int) ([]Label, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/labels")
	data, _, err := PaginateGET(ctx, a.client, path+"?per_page="+strconv.Itoa(listPerPage(limit)), limit)
	if err != nil {
		return nil, err
	}
	var out []Label
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing labels: %w", err)
	}
	return out, nil
}

// labelPath is the path of one label; GitLab accepts the label's ID or its name.
func (a *LabelAPI) labelPath(projectID, idOrName string) string {
	return a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/labels/" + url.PathEscape(idOrName))
}

// Get returns one label by its ID or name.
//
// GET /api/v4/projects/:id/labels/:label_id
func (a *LabelAPI) Get(ctx context.Context, projectID, idOrName string) (*Label, error) {
	data, err := a.client.Get(ctx, a.labelPath(projectID, idOrName)+"?include_ancestor_groups=true")
	if err != nil {
		return nil, err
	}
	var out Label
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing label: %w", err)
	}
	return &out, nil
}

// LabelCreateOpts holds parameters for creating a label.
type LabelCreateOpts struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`
	Priority    *int   `json:"priority,omitempty"`
}

// Create creates a new label. Named colours ("red") are sent as hex.
//
// POST /api/v4/projects/:id/labels
func (a *LabelAPI) Create(ctx context.Context, projectID string, opts LabelCreateOpts) (*Label, error) {
	opts.Color = NormalizeColor(opts.Color)
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/labels")
	data, err := a.client.Post(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	var out Label
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing created label: %w", err)
	}
	return &out, nil
}

// LabelUpdateOpts holds the fields to change; nil fields are left as they are.
type LabelUpdateOpts struct {
	NewName     *string `json:"new_name,omitempty"`
	Color       *string `json:"color,omitempty"`
	Description *string `json:"description,omitempty"`
	Priority    *int    `json:"priority,omitempty"`
}

// Update changes a label, found by its ID or name.
//
// PUT /api/v4/projects/:id/labels/:label_id
func (a *LabelAPI) Update(ctx context.Context, projectID, idOrName string, opts LabelUpdateOpts) (*Label, error) {
	if opts.Color != nil {
		c := NormalizeColor(*opts.Color)
		opts.Color = &c
	}
	data, err := a.client.Put(ctx, a.labelPath(projectID, idOrName), opts)
	if err != nil {
		return nil, err
	}
	var out Label
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing updated label: %w", err)
	}
	return &out, nil
}

// Delete deletes a label, found by its ID or name.
//
// DELETE /api/v4/projects/:id/labels/:label_id
func (a *LabelAPI) Delete(ctx context.Context, projectID, idOrName string) error {
	return a.client.Delete(ctx, a.labelPath(projectID, idOrName))
}

// namedColors maps common colour names to the hex values GitLab stores.
var namedColors = map[string]string{
	"red":    "#FF0000",
	"green":  "#00FF00",
	"blue":   "#0000FF",
	"yellow": "#FFFF00",
	"orange": "#FF8C00",
	"purple": "#800080",
	"pink":   "#FF69B4",
	"black":  "#000000",
	"white":  "#FFFFFF",
	"gray":   "#808080",
	"grey":   "#808080",
}

// NormalizeColor converts a named colour to its hex equivalent. A value that
// already starts with '#', or an unknown name, is returned as it is.
func NormalizeColor(c string) string {
	c = strings.TrimSpace(c)
	if len(c) > 0 && c[0] == '#' {
		return c
	}
	if hex, ok := namedColors[strings.ToLower(c)]; ok {
		return hex
	}
	return c
}
