package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// VariableAPI wraps CI/CD variable–related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/project_level_variables/
//
// Methods are implemented in this file by Phase 1 Wave B.
type VariableAPI struct{ client *Client }

// ─── DTOs ────────────────────────────────────────────────────────────────────

// Variable mirrors a GitLab project-level CI/CD variable.
type Variable struct {
	Key              string `json:"key"`
	Value            string `json:"value"`
	VariableType     string `json:"variable_type"`
	Protected        bool   `json:"protected"`
	Masked           bool   `json:"masked"`
	Raw              bool   `json:"raw"`
	EnvironmentScope string `json:"environment_scope"`
	Description      string `json:"description"`
}

// VariableCreateOpts is the POST body for creating a variable.
type VariableCreateOpts struct {
	Key              string `json:"key"`
	Value            string `json:"value"`
	VariableType     string `json:"variable_type,omitempty"`
	Protected        bool   `json:"protected,omitempty"`
	Masked           bool   `json:"masked,omitempty"`
	Raw              bool   `json:"raw,omitempty"`
	EnvironmentScope string `json:"environment_scope,omitempty"`
	Description      string `json:"description,omitempty"`
}

// VariableUpdateOpts is the PUT body for updating a variable.
type VariableUpdateOpts struct {
	Value            string `json:"value,omitempty"`
	VariableType     string `json:"variable_type,omitempty"`
	Protected        *bool  `json:"protected,omitempty"`
	Masked           *bool  `json:"masked,omitempty"`
	Raw              *bool  `json:"raw,omitempty"`
	EnvironmentScope string `json:"environment_scope,omitempty"`
	Description      string `json:"description,omitempty"`
}

// ─── Methods ─────────────────────────────────────────────────────────────────

func (a *VariableAPI) basePath(projectID string) string {
	return "/projects/" + EncodeProjectPath(projectID) + "/variables"
}

// List returns all CI/CD variables for a project.
//
// GET /api/v4/projects/:id/variables?per_page=N
func (a *VariableAPI) List(ctx context.Context, projectID string, limit int) ([]Variable, error) {
	path := a.client.APIPath(a.basePath(projectID)) + "?per_page=" + strconv.Itoa(listPerPage(limit))
	data, _, err := PaginateGET(ctx, a.client, path, limit)
	if err != nil {
		return nil, err
	}
	var out []Variable
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing variables: %w", err)
	}
	return out, nil
}

// Get returns a single variable by key.
//
// GET /api/v4/projects/:id/variables/:key?filter[env_scope]=<scope>
func (a *VariableAPI) Get(ctx context.Context, projectID, key, envScope string) (*Variable, error) {
	path := a.client.APIPath(a.basePath(projectID) + "/" + url.PathEscape(key))
	if envScope != "" {
		path += "?filter%5Benv_scope%5D=" + url.QueryEscape(envScope)
	}
	data, err := a.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	var v Variable
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parsing variable: %w", err)
	}
	return &v, nil
}

// Create creates a new CI/CD variable.
//
// POST /api/v4/projects/:id/variables
func (a *VariableAPI) Create(ctx context.Context, projectID string, opts *VariableCreateOpts) (*Variable, error) {
	path := a.client.APIPath(a.basePath(projectID))
	data, err := a.client.Post(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	var v Variable
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parsing created variable: %w", err)
	}
	return &v, nil
}

// Update updates an existing CI/CD variable.
//
// PUT /api/v4/projects/:id/variables/:key?filter[env_scope]=<scope>
func (a *VariableAPI) Update(ctx context.Context, projectID, key, envScope string, opts *VariableUpdateOpts) (*Variable, error) {
	path := a.client.APIPath(a.basePath(projectID) + "/" + url.PathEscape(key))
	if envScope != "" {
		path += "?filter%5Benv_scope%5D=" + url.QueryEscape(envScope)
	}
	data, err := a.client.Put(ctx, path, opts)
	if err != nil {
		return nil, err
	}
	var v Variable
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parsing updated variable: %w", err)
	}
	return &v, nil
}

// Delete deletes a CI/CD variable.
//
// DELETE /api/v4/projects/:id/variables/:key?filter[env_scope]=<scope>
func (a *VariableAPI) Delete(ctx context.Context, projectID, key, envScope string) error {
	path := a.client.APIPath(a.basePath(projectID) + "/" + url.PathEscape(key))
	if envScope != "" {
		path += "?filter%5Benv_scope%5D=" + url.QueryEscape(envScope)
	}
	return a.client.Delete(ctx, path)
}
