package output

// ─── Issue ───────────────────────────────────────────────────────────────────

// FlatIssue is a token-efficient representation of a GitLab issue.
type FlatIssue struct {
	IID       int    `json:"iid"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Author    string `json:"author,omitempty"`
	Assignee  string `json:"assignee,omitempty"`
	Labels    string `json:"labels,omitempty"`
	Milestone string `json:"milestone,omitempty"`
	WebURL    string `json:"webUrl,omitempty"`
}

// IssueToMap converts a FlatIssue to a map for field filtering.
func IssueToMap(f FlatIssue) map[string]any {
	m := map[string]any{
		"iid":   ID(f.IID),
		"title": f.Title,
		"state": f.State,
	}
	if f.Author != "" {
		m["author"] = f.Author
	}
	if f.Assignee != "" {
		m["assignee"] = f.Assignee
	}
	if f.Labels != "" {
		m["labels"] = f.Labels
	}
	if f.Milestone != "" {
		m["milestone"] = f.Milestone
	}
	if f.WebURL != "" {
		m["webUrl"] = f.WebURL
	}
	return MarkUntrusted(m, "title", "labels", "milestone")
}

// ─── Label ───────────────────────────────────────────────────────────────────

// FlatLabel is a token-efficient representation of a GitLab label.
type FlatLabel struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description,omitempty"`
	Priority    *int   `json:"priority,omitempty"`
}

// LabelToMap converts a FlatLabel to a map for field filtering.
func LabelToMap(f FlatLabel) map[string]any {
	m := map[string]any{
		"id":    ID(f.ID),
		"name":  f.Name,
		"color": f.Color,
	}
	if f.Description != "" {
		m["description"] = f.Description
	}
	if f.Priority != nil {
		m["priority"] = *f.Priority
	}
	return MarkUntrusted(m, "name", "description")
}

// ─── Milestone ───────────────────────────────────────────────────────────────

// FlatMilestone is a token-efficient representation of a GitLab milestone.
type FlatMilestone struct {
	ID        int    `json:"id"`
	IID       int    `json:"iid"`
	Title     string `json:"title"`
	State     string `json:"state"`
	StartDate string `json:"startDate,omitempty"`
	DueDate   string `json:"dueDate,omitempty"`
	WebURL    string `json:"webUrl,omitempty"`
}

// MilestoneToMap converts a FlatMilestone to a map for field filtering.
func MilestoneToMap(f FlatMilestone) map[string]any {
	m := map[string]any{
		"id":    ID(f.ID),
		"iid":   ID(f.IID),
		"title": f.Title,
		"state": f.State,
	}
	if f.StartDate != "" {
		m["startDate"] = f.StartDate
	}
	if f.DueDate != "" {
		m["dueDate"] = f.DueDate
	}
	if f.WebURL != "" {
		m["webUrl"] = f.WebURL
	}
	return MarkUntrusted(m, "title")
}
