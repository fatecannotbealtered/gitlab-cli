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
	// Description carries the full MR body. It is only emitted by single-MR reads
	// (MRDetailToMap); list output omits it for token efficiency.
	Description string `json:"description,omitempty"`
}

// ToFlatMR converts a MergeRequest to FlatMR.
func ToFlatMR(mr *api.MergeRequest) FlatMR {
	author := ""
	if mr.Author != nil {
		author = mr.Author.Username
	}
	return FlatMR{
		IID:         mr.IID,
		Title:       mr.Title,
		State:       mr.State,
		Source:      mr.SourceBranch,
		Target:      mr.TargetBranch,
		Author:      author,
		WebURL:      mr.WebURL,
		Draft:       mr.Draft,
		Description: mr.Description,
	}
}

// MRToMap converts a FlatMR to a map for field filtering (list shape, no body).
func MRToMap(m FlatMR) map[string]any {
	return mrToMap(m, false)
}

// MRDetailToMap is the single-MR read shape: it adds the full description body
// (tagged _untrusted) that MRToMap omits for token efficiency.
func MRDetailToMap(m FlatMR) map[string]any {
	return mrToMap(m, true)
}

func mrToMap(m FlatMR, withDescription bool) map[string]any {
	out := map[string]any{
		"iid":    ID(m.IID),
		"title":  m.Title,
		"state":  m.State,
		"source": m.Source,
		"target": m.Target,
		"author": m.Author,
		"webUrl": m.WebURL,
		"draft":  m.Draft,
	}
	// author is an attacker-controllable username; tag it untrusted with title.
	untrusted := []string{"title", "author"}
	if withDescription && m.Description != "" {
		out["description"] = m.Description
		untrusted = append(untrusted, "description")
	}
	return MarkUntrusted(out, untrusted...)
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
	return MarkUntrusted(map[string]any{
		"id":      ID(n.ID),
		"author":  n.Author,
		"body":    n.Body,
		"created": n.Created,
	}, "body", "author")
}

// FlatMRDiscussion is a token-efficient representation of an MR discussion thread.
type FlatMRDiscussion struct {
	ID             string       `json:"id"`
	IndividualNote bool         `json:"individualNote"`
	Notes          []FlatMRNote `json:"notes"`
}

// ToFlatMRDiscussion converts an api.MergeRequestDiscussion to FlatMRDiscussion.
func ToFlatMRDiscussion(d *api.MergeRequestDiscussion) FlatMRDiscussion {
	notes := make([]FlatMRNote, len(d.Notes))
	for i := range d.Notes {
		notes[i] = ToFlatMRNote(&d.Notes[i])
	}
	return FlatMRDiscussion{
		ID:             d.ID,
		IndividualNote: d.IndividualNote,
		Notes:          notes,
	}
}

// MRDiscussionToMap converts a FlatMRDiscussion to a map. The nested notes carry
// the attacker-controllable body/author fields, each already tagged _untrusted by
// MRNoteToMap, so an agent never confuses thread text for instructions.
func MRDiscussionToMap(d FlatMRDiscussion) map[string]any {
	notes := make([]map[string]any, len(d.Notes))
	for i, n := range d.Notes {
		notes[i] = MRNoteToMap(n)
	}
	return map[string]any{
		"id":             d.ID,
		"individualNote": d.IndividualNote,
		"notes":          notes,
	}
}
