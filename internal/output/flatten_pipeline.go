package output

// FlatPipeline is a token-efficient representation of a GitLab pipeline.
type FlatPipeline struct {
	ID        int    `json:"id"`
	IID       int    `json:"iid"`
	ProjectID int    `json:"projectId"`
	Ref       string `json:"ref"`
	SHA       string `json:"sha,omitempty"`
	Status    string `json:"status"`
	Source    string `json:"source,omitempty"`
	WebURL    string `json:"webUrl,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	Username  string `json:"username,omitempty"`
}

// PipelineToMap converts a FlatPipeline to a map for field filtering.
func PipelineToMap(p FlatPipeline) map[string]any {
	m := map[string]any{
		"id":     ID(p.ID),
		"iid":    ID(p.IID),
		"ref":    p.Ref,
		"status": p.Status,
	}
	if p.ProjectID != 0 {
		m["projectId"] = ID(p.ProjectID)
	}
	if p.SHA != "" {
		m["sha"] = p.SHA
	}
	if p.Source != "" {
		m["source"] = p.Source
	}
	if p.WebURL != "" {
		m["webUrl"] = p.WebURL
	}
	if p.CreatedAt != "" {
		m["createdAt"] = p.CreatedAt
	}
	if p.UpdatedAt != "" {
		m["updatedAt"] = p.UpdatedAt
	}
	if p.Username != "" {
		m["username"] = p.Username
	}
	return MarkUntrusted(m, "ref")
}

// FlatJob is a token-efficient representation of a GitLab CI job.
type FlatJob struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	Stage      string  `json:"stage,omitempty"`
	Ref        string  `json:"ref,omitempty"`
	WebURL     string  `json:"webUrl,omitempty"`
	CreatedAt  string  `json:"createdAt,omitempty"`
	StartedAt  string  `json:"startedAt,omitempty"`
	FinishedAt string  `json:"finishedAt,omitempty"`
	Duration   float64 `json:"duration,omitempty"`
	Username   string  `json:"username,omitempty"`
	PipelineID int     `json:"pipelineId,omitempty"`
}

// JobToMap converts a FlatJob to a map for field filtering.
func JobToMap(j FlatJob) map[string]any {
	m := map[string]any{
		"id":     ID(j.ID),
		"name":   j.Name,
		"status": j.Status,
	}
	if j.Stage != "" {
		m["stage"] = j.Stage
	}
	if j.Ref != "" {
		m["ref"] = j.Ref
	}
	if j.WebURL != "" {
		m["webUrl"] = j.WebURL
	}
	if j.CreatedAt != "" {
		m["createdAt"] = j.CreatedAt
	}
	if j.StartedAt != "" {
		m["startedAt"] = j.StartedAt
	}
	if j.FinishedAt != "" {
		m["finishedAt"] = j.FinishedAt
	}
	if j.Duration != 0 {
		m["duration"] = j.Duration
	}
	if j.Username != "" {
		m["username"] = j.Username
	}
	if j.PipelineID != 0 {
		m["pipelineId"] = ID(j.PipelineID)
	}
	return MarkUntrusted(m, "name", "stage", "ref")
}
