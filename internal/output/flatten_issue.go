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
	// Description carries the full issue body. It is only emitted by single-issue
	// reads (IssueDetailToMap); list output omits it for token efficiency.
	Description string `json:"description,omitempty"`
	WebURL      string `json:"webUrl,omitempty"`
}

// IssueToMap converts a FlatIssue to a map for field filtering (list shape, no body).
func IssueToMap(f FlatIssue) map[string]any {
	return issueToMap(f, false)
}

// IssueDetailToMap is the single-issue read shape: it adds the full description
// body (tagged _untrusted) that IssueToMap omits for token efficiency.
func IssueDetailToMap(f FlatIssue) map[string]any {
	return issueToMap(f, true)
}

func issueToMap(f FlatIssue, withDescription bool) map[string]any {
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
	// author/assignee display-adjacent usernames are attacker-controllable, so
	// tag them untrusted alongside title/labels/milestone.
	untrusted := []string{"title", "labels", "milestone", "author", "assignee"}
	if withDescription && f.Description != "" {
		m["description"] = f.Description
		untrusted = append(untrusted, "description")
	}
	return MarkUntrusted(m, untrusted...)
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
