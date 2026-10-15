package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// VariableAPI wraps the project-level CI/CD variable API.
//
// Endpoint reference: https://docs.gitlab.com/api/project_level_variables/
type VariableAPI struct{ client *Client }

// Variable is a project CI/CD variable. Value is nil when GitLab withholds it (a
// hidden variable). One key may exist once per environment scope.
type Variable struct {
	Key              string  `json:"key"`
	Value            *string `json:"value"`
	VariableType     string  `json:"variable_type"` // env_var or file
	Protected        bool    `json:"protected"`
	Masked           bool    `json:"masked"`
	Hidden           bool    `json:"hidden"`
	Raw              bool    `json:"raw"`
	EnvironmentScope string  `json:"environment_scope"`
	Description      string  `json:"description"`
}

// VariableCreateOpts is the POST body for creating a variable. The flags are always
// sent, so the request says exactly what the variable will be.
type VariableCreateOpts struct {
	Key              string `json:"key"`
	Value            string `json:"value"`
	VariableType     string `json:"variable_type,omitempty"`
	Protected        bool   `json:"protected"`
	Masked           bool   `json:"masked"`
	Raw              bool   `json:"raw"`
	EnvironmentScope string `json:"environment_scope,omitempty"`
	Description      string `json:"description,omitempty"`
}

// VariableUpdateOpts holds the fields to change; nil fields are left as they are.
type VariableUpdateOpts struct {
	Value        *string `json:"value,omitempty"`
	VariableType *string `json:"variable_type,omitempty"`
	Protected    *bool   `json:"protected,omitempty"`
	Masked       *bool   `json:"masked,omitempty"`
	Raw          *bool   `json:"raw,omitempty"`
	Description  *string `json:"description,omitempty"`
}

func (a *VariableAPI) basePath(projectID string) string {
	return a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/variables")
}

// keyPath is the path of one variable; envScope "" sends no filter, so GitLab picks
// the only variable with the key (and answers 409 when there are several).
func (a *VariableAPI) keyPath(projectID, key, envScope string) string {
	path := a.basePath(projectID) + "/" + url.PathEscape(key)
	if envScope != "" {
		path += "?" + url.Values{"filter[environment_scope]": {envScope}}.Encode()
	}
	return path
}

// ListPage returns one page of a project's CI/CD variables.
//
// GET /api/v4/projects/:id/variables
func (a *VariableAPI) ListPage(ctx context.Context, projectID string, page, perPage int) ([]Variable, Pagination, error) {
	data, pag, err := GetPage(ctx, a.client, a.basePath(projectID), page, perPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []Variable
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing variables: %w", err)
	}
	return out, pag, nil
}

// Get returns one variable by key and, when envScope is not "", environment scope.
//
// GET /api/v4/projects/:id/variables/:key?filter[environment_scope]=<scope>
func (a *VariableAPI) Get(ctx context.Context, projectID, key, envScope string) (*Variable, error) {
	data, err := a.client.Get(ctx, a.keyPath(projectID, key, envScope))
	if err != nil {
		return nil, err
	}
	var v Variable
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parsing variable: %w", err)
	}
	return &v, nil
}

// Create creates a variable (sent once, never retried).
//
// POST /api/v4/projects/:id/variables
func (a *VariableAPI) Create(ctx context.Context, projectID string, opts *VariableCreateOpts) (*Variable, error) {
	data, err := a.client.Post(ctx, a.basePath(projectID), opts)
	if err != nil {
		return nil, err
	}
	var v Variable
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parsing created variable: %w", err)
	}
	return &v, nil
}

// Update changes the variable with key in envScope ("" sends no filter).
//
// PUT /api/v4/projects/:id/variables/:key?filter[environment_scope]=<scope>
func (a *VariableAPI) Update(ctx context.Context, projectID, key, envScope string, opts *VariableUpdateOpts) (*Variable, error) {
	data, err := a.client.Put(ctx, a.keyPath(projectID, key, envScope), opts)
	if err != nil {
		return nil, err
	}
	var v Variable
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parsing updated variable: %w", err)
	}
	return &v, nil
}

// Delete deletes the variable with key in envScope ("" sends no filter).
//
// DELETE /api/v4/projects/:id/variables/:key?filter[environment_scope]=<scope>
func (a *VariableAPI) Delete(ctx context.Context, projectID, key, envScope string) error {
	return a.client.Delete(ctx, a.keyPath(projectID, key, envScope))
}
