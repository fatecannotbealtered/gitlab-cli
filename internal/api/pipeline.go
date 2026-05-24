package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Pipeline represents a GitLab CI pipeline.
type Pipeline struct {
	ID        int    `json:"id"`
	IID       int    `json:"iid"`
	ProjectID int    `json:"project_id"`
	Ref       string `json:"ref"`
	SHA       string `json:"sha"`
	Status    string `json:"status"`
	Source    string `json:"source"`
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	User      *User  `json:"user"`
}

// PipelineListOpts holds query parameters for listing pipelines.
type PipelineListOpts struct {
	Ref      string
	Status   string
	Username string
	Limit    int
}

func (o *PipelineListOpts) encode() string {
	if o == nil {
		return ""
	}
	q := url.Values{}
	if o.Ref != "" {
		q.Set("ref", o.Ref)
	}
	if o.Status != "" {
		q.Set("status", o.Status)
	}
	if o.Username != "" {
		q.Set("username", o.Username)
	}
	q.Set("per_page", strconv.Itoa(listPerPage(o.Limit)))
	return q.Encode()
}

// PipelineCreateBody is the request body for creating a pipeline.
type PipelineCreateBody struct {
	Ref       string             `json:"ref"`
	Variables []PipelineVariable `json:"variables,omitempty"`
}

// PipelineVariable is a key/value pair for pipeline variables.
type PipelineVariable struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PipelineAPI wraps pipeline-related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/pipelines/
//
// Methods are implemented in this file by Phase 1 Wave A.
type PipelineAPI struct{ client *Client }

// List returns pipelines for a project.
//
// GET /api/v4/projects/:id/pipelines
func (a *PipelineAPI) List(ctx context.Context, projectID string, opts *PipelineListOpts) ([]Pipeline, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/pipelines")
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
	var out []Pipeline
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing pipelines: %w", err)
	}
	return out, nil
}

// Get returns a single pipeline.
//
// GET /api/v4/projects/:id/pipelines/:pipeline_id
func (a *PipelineAPI) Get(ctx context.Context, projectID string, pipelineID int) (*Pipeline, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/pipelines/" + strconv.Itoa(pipelineID))
	data, err := a.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	var p Pipeline
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing pipeline: %w", err)
	}
	return &p, nil
}

// Create triggers a new pipeline.
//
// POST /api/v4/projects/:id/pipeline
func (a *PipelineAPI) Create(ctx context.Context, projectID string, body PipelineCreateBody) (*Pipeline, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/pipeline")
	data, err := a.client.Post(ctx, path, body)
	if err != nil {
		return nil, err
	}
	var p Pipeline
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing pipeline: %w", err)
	}
	return &p, nil
}

// Retry retries a pipeline.
//
// POST /api/v4/projects/:id/pipelines/:pipeline_id/retry
func (a *PipelineAPI) Retry(ctx context.Context, projectID string, pipelineID int) (*Pipeline, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/pipelines/" + strconv.Itoa(pipelineID) + "/retry")
	data, err := a.client.Post(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var p Pipeline
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing pipeline: %w", err)
	}
	return &p, nil
}

// Cancel cancels a pipeline.
//
// POST /api/v4/projects/:id/pipelines/:pipeline_id/cancel
func (a *PipelineAPI) Cancel(ctx context.Context, projectID string, pipelineID int) (*Pipeline, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/pipelines/" + strconv.Itoa(pipelineID) + "/cancel")
	data, err := a.client.Post(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var p Pipeline
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing pipeline: %w", err)
	}
	return &p, nil
}

// Jobs returns jobs for a pipeline.
//
// GET /api/v4/projects/:id/pipelines/:pipeline_id/jobs
func (a *PipelineAPI) Jobs(ctx context.Context, projectID string, pipelineID int, scope []string) ([]Job, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/pipelines/" + strconv.Itoa(pipelineID) + "/jobs")
	if len(scope) > 0 {
		q := url.Values{}
		for _, s := range scope {
			q.Add("scope[]", s)
		}
		path += "?" + q.Encode()
	}
	data, err := a.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, fmt.Errorf("parsing jobs: %w", err)
	}
	return jobs, nil
}
