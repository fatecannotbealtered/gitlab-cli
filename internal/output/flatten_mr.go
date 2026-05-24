package output

import "github.com/fatecannotbealtered/gitlab-cli/internal/api"

// FlatMR is a token-efficient representation of a GitLab merge request.
type FlatMR struct {
	IID    int    `json:"iid"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Source string `json:"source"`
	Target string `json:"target"`
	Author string `json:"author"`
	WebURL string `json:"webUrl"`
	Draft  bool   `json:"draft"`
}

// ToFlatMR converts a MergeRequest to FlatMR.
func ToFlatMR(mr *api.MergeRequest) FlatMR {
	author := ""
	if mr.Author != nil {
		author = mr.Author.Username
	}
	return FlatMR{
		IID:    mr.IID,
		Title:  mr.Title,
		State:  mr.State,
		Source: mr.SourceBranch,
		Target: mr.TargetBranch,
		Author: author,
		WebURL: mr.WebURL,
		Draft:  mr.Draft,
	}
}

// MRToMap converts a FlatMR to a map for field filtering.
func MRToMap(m FlatMR) map[string]any {
	return map[string]any{
		"iid":    m.IID,
		"title":  m.Title,
		"state":  m.State,
		"source": m.Source,
		"target": m.Target,
		"author": m.Author,
		"webUrl": m.WebURL,
		"draft":  m.Draft,
	}
}

// FlatMRNote is a token-efficient representation of a GitLab MR note.
type FlatMRNote struct {
	ID      int    `json:"id"`
	Author  string `json:"author"`
	Body    string `json:"body"`
	Created string `json:"created"`
}

// ToFlatMRNote converts a MergeRequestNote to FlatMRNote.
func ToFlatMRNote(n *api.MergeRequestNote) FlatMRNote {
	author := ""
	if n.Author != nil {
		author = n.Author.Username
	}
	return FlatMRNote{
		ID:      n.ID,
		Author:  author,
		Body:    n.Body,
		Created: n.CreatedAt,
	}
}

// MRNoteToMap converts a FlatMRNote to a map for field filtering.
func MRNoteToMap(n FlatMRNote) map[string]any {
	return map[string]any{
		"id":      n.ID,
		"author":  n.Author,
		"body":    n.Body,
		"created": n.Created,
	}
}
