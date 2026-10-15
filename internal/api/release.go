package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// ReleaseAPI wraps release-related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/releases/
type ReleaseAPI struct{ client *Client }

// ─── DTOs ────────────────────────────────────────────────────────────────────

// Release is returned by GET /projects/:id/releases
type Release struct {
	TagName         string             `json:"tag_name"`
	Name            string             `json:"name"`
	Description     string             `json:"description"`
	CreatedAt       string             `json:"created_at"`
	ReleasedAt      string             `json:"released_at"`
	UpcomingRelease bool               `json:"upcoming_release"`
	Author          *ReleaseAuthor     `json:"author"`
	Commit          *Commit            `json:"commit"`
	Milestones      []ReleaseMilestone `json:"milestones"`
	CommitPath      string             `json:"commit_path"`
	TagPath         string             `json:"tag_path"`
	Assets          *ReleaseAssets     `json:"assets"`
	Links           struct {
		Self string `json:"self"`
	} `json:"_links"`
}

// ReleaseAuthor is the user who created a release.
type ReleaseAuthor struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}

// ReleaseMilestone is a milestone a release is associated with.
type ReleaseMilestone struct {
	ID    int    `json:"id"`
	IID   int    `json:"iid"`
	Title string `json:"title"`
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

// ReleaseCreateBody is the request body for creating a release. Ref is required
// when the tag does not exist yet: GitLab creates the tag from it.
type ReleaseCreateBody struct {
	TagName     string   `json:"tag_name"`
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Ref         string   `json:"ref,omitempty"`
	Milestones  []string `json:"milestones,omitempty"`
}

// ReleaseUpdateBody is the request body for updating a release: a nil field is left
// as it is, an empty one cleared (an empty Milestones list removes every milestone).
type ReleaseUpdateBody struct {
	Name        *string   `json:"name,omitempty"`
	Description *string   `json:"description,omitempty"`
	Milestones  *[]string `json:"milestones,omitempty"`
}

// ─── Methods ─────────────────────────────────────────────────────────────────

func (a *ReleaseAPI) releasePath(projectID, tagName string) string {
	return a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/releases/" + url.PathEscape(tagName))
}

// ListPage returns one page of a project's releases, newest release date first
// (GitLab's default order).
//
// GET /api/v4/projects/:id/releases
func (a *ReleaseAPI) ListPage(ctx context.Context, projectID string, page, perPage int) ([]Release, Pagination, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/releases")
	data, pag, err := GetPage(ctx, a.client, path, page, perPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []Release
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing releases: %w", err)
	}
	return out, pag, nil
}

// Get returns a single release by tag name.
//
// GET /api/v4/projects/:id/releases/:tag_name
func (a *ReleaseAPI) Get(ctx context.Context, projectID, tagName string) (*Release, error) {
	data, err := a.client.Get(ctx, a.releasePath(projectID, tagName))
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
//
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
//
// PUT /api/v4/projects/:id/releases/:tag_name
func (a *ReleaseAPI) Update(ctx context.Context, projectID, tagName string, body ReleaseUpdateBody) (*Release, error) {
	data, err := a.client.Put(ctx, a.releasePath(projectID, tagName), body)
	if err != nil {
		return nil, err
	}
	var r Release
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parsing release: %w", err)
	}
	return &r, nil
}

// Delete deletes a release; its tag stays.
//
// DELETE /api/v4/projects/:id/releases/:tag_name
func (a *ReleaseAPI) Delete(ctx context.Context, projectID, tagName string) error {
	return a.client.Delete(ctx, a.releasePath(projectID, tagName))
}
