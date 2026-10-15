package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Job is a GitLab CI job.
type Job struct {
	ID                int            `json:"id"`
	Name              string         `json:"name"`
	Stage             string         `json:"stage"`
	Status            string         `json:"status"`
	FailureReason     string         `json:"failure_reason"` // set when the job failed: script_failure, ...
	AllowFailure      bool           `json:"allow_failure"`
	Ref               string         `json:"ref"` // the branch or tag
	Tag               bool           `json:"tag"`
	WebURL            string         `json:"web_url"`
	CreatedAt         string         `json:"created_at"`
	StartedAt         string         `json:"started_at"`
	FinishedAt        string         `json:"finished_at"`
	Duration          *float64       `json:"duration"`        // seconds; null before the job starts
	QueuedDuration    *float64       `json:"queued_duration"` // seconds
	Coverage          *float64       `json:"coverage"`
	User              *User          `json:"user"`
	Pipeline          *Pipeline      `json:"pipeline"`
	ArtifactsFile     *ArtifactsFile `json:"artifacts_file"` // the archive; nil when there is none
	Artifacts         []JobArtifact  `json:"artifacts"`
	ArtifactsExpireAt string         `json:"artifacts_expire_at"`
}

// ArtifactsFile is a job's artifacts archive.
type ArtifactsFile struct {
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// JobArtifact is one artifact file of a job (archive, metadata, trace, reports).
type JobArtifact struct {
	FileType   string `json:"file_type"`
	Size       int64  `json:"size"`
	Filename   string `json:"filename"`
	FileFormat string `json:"file_format"`
}

// JobVariable is a key/value pair supplied when playing a manual job.
type JobVariable struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// JobAPI wraps CI job–related API calls.
//
// Endpoint reference: https://docs.gitlab.com/api/jobs/
type JobAPI struct{ client *Client }

func (a *JobAPI) path(projectID string, jobID int, rest string) string {
	return a.client.APIPath("/projects/" + EncodeProjectPath(projectID) + "/jobs/" + strconv.Itoa(jobID) + rest)
}

// Get returns a single job.
//
// GET /api/v4/projects/:id/jobs/:job_id
func (a *JobAPI) Get(ctx context.Context, projectID string, jobID int) (*Job, error) {
	data, err := a.client.Get(ctx, a.path(projectID, jobID, ""))
	if err != nil {
		return nil, err
	}
	return parseJob(data)
}

// Log returns the whole plain-text trace of a job.
//
// GET /api/v4/projects/:id/jobs/:job_id/trace
func (a *JobAPI) Log(ctx context.Context, projectID string, jobID int) ([]byte, error) {
	return a.client.Get(ctx, a.path(projectID, jobID, "/trace"))
}

// TraceFrom returns the trace bytes past offset. It asks for them with a Range
// header; GitLab answers 206 with the new bytes, or ignores the range and sends the
// whole trace (200), of which the bytes past offset are returned. 416 means there is
// nothing new yet.
func (a *JobAPI) TraceFrom(ctx context.Context, projectID string, jobID, offset int) ([]byte, error) {
	status, body, err := a.client.RawGet(ctx, a.path(projectID, jobID, "/trace"),
		map[string]string{"Range": fmt.Sprintf("bytes=%d-", offset)})
	switch {
	case err != nil:
		return nil, err
	case status == http.StatusPartialContent:
		return body, nil
	case status == http.StatusRequestedRangeNotSatisfiable:
		return nil, nil
	case status >= 400:
		return nil, parseError(status, nil, body)
	case offset >= len(body):
		return nil, nil
	}
	return body[offset:], nil
}

// LogStream follows a job's trace into w until the job reaches a final status
// (success, failed, canceled, skipped or manual) or ctx ends, polling every interval
// (0 means 3 s).
func (a *JobAPI) LogStream(ctx context.Context, projectID string, jobID int, w io.Writer, interval time.Duration) error {
	if interval == 0 {
		interval = 3 * time.Second
	}
	offset := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		chunk, err := a.TraceFrom(ctx, projectID, jobID, offset)
		if err != nil {
			return err
		}
		if len(chunk) > 0 {
			if _, err := w.Write(chunk); err != nil {
				return err
			}
			offset += len(chunk)
		}
		data, err := a.client.Get(ctx, a.path(projectID, jobID, ""))
		if err != nil {
			return err
		}
		var j Job
		if err := json.Unmarshal(data, &j); err != nil {
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

// Retry retries a job; GitLab answers with the new job it created.
//
// POST /api/v4/projects/:id/jobs/:job_id/retry
func (a *JobAPI) Retry(ctx context.Context, projectID string, jobID int) (*Job, error) {
	data, err := a.client.Post(ctx, a.path(projectID, jobID, "/retry"), nil)
	if err != nil {
		return nil, err
	}
	return parseJob(data)
}

// Cancel cancels a job.
//
// POST /api/v4/projects/:id/jobs/:job_id/cancel
func (a *JobAPI) Cancel(ctx context.Context, projectID string, jobID int) (*Job, error) {
	data, err := a.client.Post(ctx, a.path(projectID, jobID, "/cancel"), nil)
	if err != nil {
		return nil, err
	}
	return parseJob(data)
}

// Play starts a manual or scheduled (delayed) job, optionally with job variables.
//
// POST /api/v4/projects/:id/jobs/:job_id/play
func (a *JobAPI) Play(ctx context.Context, projectID string, jobID int, vars []JobVariable) (*Job, error) {
	var body any
	if len(vars) > 0 {
		body = map[string]any{"job_variables_attributes": vars}
	}
	data, err := a.client.Post(ctx, a.path(projectID, jobID, "/play"), body)
	if err != nil {
		return nil, err
	}
	return parseJob(data)
}

// ArtifactsTo streams a job's artifacts archive to w.
//
// GET /api/v4/projects/:id/jobs/:job_id/artifacts
func (a *JobAPI) ArtifactsTo(ctx context.Context, projectID string, jobID int, w io.Writer) error {
	return a.client.DownloadTo(ctx, a.path(projectID, jobID, "/artifacts"), w)
}

func parseJob(data []byte) (*Job, error) {
	var j Job
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("parsing job: %w", err)
	}
	return &j, nil
}
