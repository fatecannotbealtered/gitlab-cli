package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// LabelAPI wraps label-related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/labels/
type LabelAPI struct{ client *Client }

// Label represents a GitLab project label.
type Label struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
	Priority    *int   `json:"priority"`
}

// List returns labels for a project.
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

// LabelCreateOpts holds parameters for creating a label.
type LabelCreateOpts struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`
	Priority    *int   `json:"priority,omitempty"`
}

// Create creates a new label.
//
// POST /api/v4/projects/:id/labels
func (a *LabelAPI) Create(ctx context.Context, projectID string, opts LabelCreateOpts) (*Label, error) {
	// Normalize color: named colors → hex
	opts.Color = normalizeColor(opts.Color)
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

// LabelUpdateOpts holds parameters for updating a label.
type LabelUpdateOpts struct {
	NewName     string `json:"new_name,omitempty"`
	Color       string `json:"color,omitempty"`
	Description string `json:"description,omitempty"`
	Priority    *int   `json:"priority,omitempty"`
}

// Update updates an existing label.
//
// PUT /api/v4/projects/:id/labels/:label_id
func (a *LabelAPI) Update(ctx context.Context, projectID string, labelID int, opts LabelUpdateOpts) (*Label, error) {
	if opts.Color != "" {
		opts.Color = normalizeColor(opts.Color)
	}
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/labels/" + strconv.Itoa(labelID))
	data, err := a.client.Put(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	var out Label
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing updated label: %w", err)
	}
	return &out, nil
}

// Delete deletes a label.
//
// DELETE /api/v4/projects/:id/labels/:label_id
func (a *LabelAPI) Delete(ctx context.Context, projectID string, labelID int) error {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/labels/" + strconv.Itoa(labelID))
	return a.client.Delete(ctx, path)
}

// namedColors maps common color names to hex values accepted by GitLab.
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

// normalizeColor converts a named color to its hex equivalent.
// If the value already starts with '#' or is unknown, it is returned as-is.
func normalizeColor(c string) string {
	if len(c) > 0 && c[0] == '#' {
		return c
	}
	if hex, ok := namedColors[c]; ok {
		return hex
	}
	return c
}
