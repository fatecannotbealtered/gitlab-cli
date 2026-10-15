package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// Pipeline is a GitLab CI pipeline. The list endpoint returns the first block of
// fields; the single-pipeline endpoints (get, create, retry, cancel) add the rest.
type Pipeline struct {
	ID        int    `json:"id"`
	IID       int    `json:"iid"`
	ProjectID int    `json:"project_id"`
	Ref       string `json:"ref"` // the branch or tag
	SHA       string `json:"sha"`
	Status    string `json:"status"`
	Source    string `json:"source"`
	Name      string `json:"name"` // workflow:name of .gitlab-ci.yml; "" when unset
	WebURL    string `json:"web_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	Tag            bool     `json:"tag"`
	YamlErrors     *string  `json:"yaml_errors"`
	User           *User    `json:"user"`
	StartedAt      string   `json:"started_at"`
	FinishedAt     string   `json:"finished_at"`
	Duration       *float64 `json:"duration"`        // seconds
	QueuedDuration *float64 `json:"queued_duration"` // seconds
	Coverage       *string  `json:"coverage"`
}

// PipelineListOpts filters a pipeline list. Empty values are not sent.
type PipelineListOpts struct {
	Ref      string // branch or tag
	Status   string // one of GitLab's pipeline statuses
	Username string // the user who triggered the pipeline
	Page     int
	PerPage  int
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

// PipelineJobsOpts filters the jobs of a pipeline.
type PipelineJobsOpts struct {
	Scope   []string // job statuses (GitLab's scope[]); empty means all
	Page    int
	PerPage int
}

// PipelineAPI wraps pipeline-related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/pipelines/
type PipelineAPI struct{ client *Client }

func (a *PipelineAPI) path(projectID string, rest string) string {
	return a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + rest)
}

// ListPage returns one page of a project's pipelines, newest first (GitLab's
// default order: id descending).
//
// GET /api/v4/projects/:id/pipelines
func (a *PipelineAPI) ListPage(ctx context.Context, projectID string, opts PipelineListOpts) ([]Pipeline, Pagination, error) {
	q := url.Values{}
	if opts.Ref != "" {
		q.Set("ref", opts.Ref)
	}
	if opts.Status != "" {
		q.Set("status", opts.Status)
	}
	if opts.Username != "" {
		q.Set("username", opts.Username)
	}
	path := a.path(projectID, "/pipelines")
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	data, pag, err := GetPage(ctx, a.client, path, opts.Page, opts.PerPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var out []Pipeline
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing pipelines: %w", err)
	}
	return out, pag, nil
}

// Get returns a single pipeline.
//
// GET /api/v4/projects/:id/pipelines/:pipeline_id
func (a *PipelineAPI) Get(ctx context.Context, projectID string, pipelineID int) (*Pipeline, error) {
	data, err := a.client.Get(ctx, a.path(projectID, "/pipelines/"+strconv.Itoa(pipelineID)))
	if err != nil {
		return nil, err
	}
	return parsePipeline(data)
}

// Create triggers a new pipeline (sent once, never retried).
//
// POST /api/v4/projects/:id/pipeline
func (a *PipelineAPI) Create(ctx context.Context, projectID string, body PipelineCreateBody) (*Pipeline, error) {
	data, err := a.client.Post(ctx, a.path(projectID, "/pipeline"), body)
	if err != nil {
		return nil, err
	}
	return parsePipeline(data)
}

// Retry retries the failed and canceled jobs of a pipeline.
//
// POST /api/v4/projects/:id/pipelines/:pipeline_id/retry
func (a *PipelineAPI) Retry(ctx context.Context, projectID string, pipelineID int) (*Pipeline, error) {
	data, err := a.client.Post(ctx, a.path(projectID, "/pipelines/"+strconv.Itoa(pipelineID)+"/retry"), nil)
	if err != nil {
		return nil, err
	}
	return parsePipeline(data)
}

// Cancel cancels the running and pending jobs of a pipeline.
//
// POST /api/v4/projects/:id/pipelines/:pipeline_id/cancel
func (a *PipelineAPI) Cancel(ctx context.Context, projectID string, pipelineID int) (*Pipeline, error) {
	data, err := a.client.Post(ctx, a.path(projectID, "/pipelines/"+strconv.Itoa(pipelineID)+"/cancel"), nil)
	if err != nil {
		return nil, err
	}
	return parsePipeline(data)
}

// JobsPage returns one page of a pipeline's jobs. GitLab lists the latest attempt of
// each job and leaves out trigger (bridge) jobs.
//
// GET /api/v4/projects/:id/pipelines/:pipeline_id/jobs
func (a *PipelineAPI) JobsPage(ctx context.Context, projectID string, pipelineID int, opts PipelineJobsOpts) ([]Job, Pagination, error) {
	path := a.path(projectID, "/pipelines/"+strconv.Itoa(pipelineID)+"/jobs")
	if len(opts.Scope) > 0 {
		q := url.Values{}
		for _, s := range opts.Scope {
			q.Add("scope[]", s)
		}
		path += "?" + q.Encode()
	}
	data, pag, err := GetPage(ctx, a.client, path, opts.Page, opts.PerPage)
	if err != nil {
		return nil, Pagination{}, err
	}
	var jobs []Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, Pagination{}, fmt.Errorf("parsing jobs: %w", err)
	}
	return jobs, pag, nil
}

func parsePipeline(data []byte) (*Pipeline, error) {
	var p Pipeline
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing pipeline: %w", err)
	}
	return &p, nil
}
