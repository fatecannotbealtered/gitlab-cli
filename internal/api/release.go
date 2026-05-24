package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// ReleaseAPI wraps release-related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/releases/
type ReleaseAPI struct{ client *Client }

// ─── DTOs ────────────────────────────────────────────────────────────────────

// Release is returned by GET /projects/:id/releases
type Release struct {
	TagName         string         `json:"tag_name"`
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	CreatedAt       string         `json:"created_at"`
	ReleasedAt      string         `json:"released_at"`
	Author          *User          `json:"author"`
	Commit          *Commit        `json:"commit"`
	Milestones      []interface{}  `json:"milestones"`
	CommitPath      string         `json:"commit_path"`
	TagPath         string         `json:"tag_path"`
	DescriptionHTML string         `json:"description_html"`
	Assets          *ReleaseAssets `json:"assets"`
}

// ReleaseAssets holds asset counts for a release.
type ReleaseAssets struct {
	Count   int `json:"count"`
	Sources []struct {
		Format string `json:"format"`
		URL    string `json:"url"`
	} `json:"sources"`
}

// ─── Request bodies ───────────────────────────────────────────────────────────

// ReleaseCreateBody is the request body for creating a release.
type ReleaseCreateBody struct {
	TagName     string   `json:"tag_name"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Ref         string   `json:"ref,omitempty"`
	Milestones  []string `json:"milestones,omitempty"`
}

// ReleaseUpdateBody is the request body for updating a release.
type ReleaseUpdateBody struct {
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Milestones  []string `json:"milestones,omitempty"`
}

// ─── Methods ─────────────────────────────────────────────────────────────────

// List returns releases for a project.
// GET /api/v4/projects/:id/releases
func (a *ReleaseAPI) List(ctx context.Context, projectID string, limit int) ([]Release, error) {
	path := a.client.APIPath("/projects/"+EncodeProjectPath(projectID)+"/releases") +
		"?per_page=" + strconv.Itoa(listPerPage(limit))
	data, _, err := PaginateGET(ctx, a.client, path, limit)
	if err != nil {
		return nil, err
	}
	var out []Release
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing releases: %w", err)
	}
	return out, nil
}

// Get returns a single release by tag name.
// GET /api/v4/projects/:id/releases/:tag_name
func (a *ReleaseAPI) Get(ctx context.Context, projectID, tagName string) (*Release, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/releases/" + url.PathEscape(tagName))
	data, err := a.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	var r Release
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing release: %w", err)
	}
	return &r, nil
}

// Create creates a new release.
// POST /api/v4/projects/:id/releases
func (a *ReleaseAPI) Create(ctx context.Context, projectID string, body ReleaseCreateBody) (*Release, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/releases")
	data, err := a.client.Post(ctx, path, body)
	if err != nil {
		return nil, err
	}
	var r Release
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing release: %w", err)
	}
	return &r, nil
}

// Update updates an existing release.
// PUT /api/v4/projects/:id/releases/:tag_name
func (a *ReleaseAPI) Update(ctx context.Context, projectID, tagName string, body ReleaseUpdateBody) (*Release, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/releases/" + url.PathEscape(tagName))
	data, err := a.client.Put(ctx, path, body)
	if err != nil {
		return nil, err
	}
	var r Release
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing release: %w", err)
	}
	return &r, nil
}

// Delete deletes a release.
// DELETE /api/v4/projects/:id/releases/:tag_name
func (a *ReleaseAPI) Delete(ctx context.Context, projectID, tagName string) error {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/releases/" + url.PathEscape(tagName))
	return a.client.Delete(ctx, path)
}
