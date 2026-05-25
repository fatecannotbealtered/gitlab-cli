package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"
)

// Job represents a GitLab CI job.
type Job struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Stage      string    `json:"stage"`
	Ref        string    `json:"ref"`
	WebURL     string    `json:"web_url"`
	CreatedAt  string    `json:"created_at"`
	StartedAt  string    `json:"started_at"`
	FinishedAt string    `json:"finished_at"`
	Duration   float64   `json:"duration"`
	User       *User     `json:"user"`
	Pipeline   *Pipeline `json:"pipeline"`
}

// JobAPI wraps CI job–related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/jobs/
//
// Methods are implemented in this file by Phase 1 Wave A.
type JobAPI struct{ client *Client }

// Get returns a single job.
//
// GET /api/v4/projects/:id/jobs/:job_id
func (a *JobAPI) Get(ctx context.Context, projectID string, jobID int) (*Job, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/jobs/" + strconv.Itoa(jobID))
	data, err := a.client.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	var j Job
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("parsing job: %w", err)
	}
	return &j, nil
}

// Log returns the plain-text trace for a job.
//
// GET /api/v4/projects/:id/jobs/:job_id/trace
func (a *JobAPI) Log(ctx context.Context, projectID string, jobID int) ([]byte, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/jobs/" + strconv.Itoa(jobID) + "/trace")
	return a.client.Get(ctx, path)
}

// Retry retries a job.
//
// POST /api/v4/projects/:id/jobs/:job_id/retry
func (a *JobAPI) Retry(ctx context.Context, projectID string, jobID int) (*Job, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/jobs/" + strconv.Itoa(jobID) + "/retry")
	data, err := a.client.Post(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var j Job
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("parsing job: %w", err)
	}
	return &j, nil
}

// Cancel cancels a job.
//
// POST /api/v4/projects/:id/jobs/:job_id/cancel
func (a *JobAPI) Cancel(ctx context.Context, projectID string, jobID int) (*Job, error) {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/jobs/" + strconv.Itoa(jobID) + "/cancel")
	data, err := a.client.Post(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var j Job
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("parsing job: %w", err)
	}
	return &j, nil
}

// Artifacts downloads the artifacts archive for a job and returns the raw bytes.
//
// GET /api/v4/projects/:id/jobs/:job_id/artifacts
func (a *JobAPI) Artifacts(ctx context.Context, projectID string, jobID int) ([]byte, error) {
	var buf bytes.Buffer
	if err := a.ArtifactsTo(ctx, projectID, jobID, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ArtifactsTo streams the artifacts archive for a job to w.
//
// GET /api/v4/projects/:id/jobs/:job_id/artifacts
func (a *JobAPI) ArtifactsTo(ctx context.Context, projectID string, jobID int, w io.Writer) error {
	path := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/jobs/" + strconv.Itoa(jobID) + "/artifacts")
	return a.client.DownloadTo(ctx, path, w)
}

// LogStream polls the job trace endpoint and writes new bytes to w until the
// job reaches a terminal state or ctx is cancelled.
// interval is the poll interval; defaults to 3s if zero.
func (a *JobAPI) LogStream(ctx context.Context, projectID string, jobID int, w io.Writer, interval time.Duration) error {
	if interval == 0 {
		interval = 3 * time.Second
	}
	offset := 0
	jobPath := a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/jobs/" + strconv.Itoa(jobID))
	tracePath := jobPath + "/trace"

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		sc, body, err := a.client.RawGet(ctx, tracePath, map[string]string{
			"Range": fmt.Sprintf("bytes=%d-", offset),
		})
		if err != nil {
			return err
		}
		if sc == 206 || sc == 200 {
			if len(body) > 0 {
				if _, err := w.Write(body); err != nil {
					return err
				}
				offset += len(body)
			}
		}
		// 416 = log not yet available; fall through to status check

		statusData, err := a.client.Get(ctx, jobPath)
		if err != nil {
			return err
		}
		var j Job
		if err := json.Unmarshal(statusData, &j); err != nil {
			return fmt.Errorf("parsing job status: %w", err)
		}
		switch j.Status {
		case "success", "failed", "canceled", "skipped", "manual":
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
