package output

import "github.com/fatecannotbealtered/gitlab-cli/internal/api"

// FlatBranch is a token-efficient representation of a GitLab branch.
type FlatBranch struct {
	Name      string `json:"name"`
	Default   bool   `json:"default"`
	Protected bool   `json:"protected"`
	Merged    bool   `json:"merged"`
	WebURL    string `json:"webUrl,omitempty"`
	CommitID  string `json:"commitId,omitempty"`
}

// BranchToMap converts a FlatBranch to a map for field filtering.
func BranchToMap(b FlatBranch) map[string]any {
	return MarkUntrusted(map[string]any{
		"name":      b.Name,
		"default":   b.Default,
		"protected": b.Protected,
		"merged":    b.Merged,
		"webUrl":    b.WebURL,
		"commitId":  b.CommitID,
	}, "name")
}

// ToFlatBranch converts an api.Branch to a FlatBranch.
func ToFlatBranch(b *api.Branch) FlatBranch {
	f := FlatBranch{
		Name:      b.Name,
		Default:   b.Default,
		Protected: b.Protected,
		Merged:    b.Merged,
		WebURL:    b.WebURL,
	}
	if b.Commit != nil {
		f.CommitID = b.Commit.ShortID
	}
	return f
}

// FlatCommit is a token-efficient representation of a GitLab commit.
type FlatCommit struct {
	ID            string `json:"id"`
	ShortID       string `json:"shortId"`
	Title         string `json:"title"`
	AuthorName    string `json:"authorName"`
	AuthoredDate  string `json:"authoredDate,omitempty"`
	CommittedDate string `json:"committedDate,omitempty"`
	WebURL        string `json:"webUrl,omitempty"`
}

// CommitToMap converts a FlatCommit to a map for field filtering.
func CommitToMap(c FlatCommit) map[string]any {
	return MarkUntrusted(map[string]any{
		"id":            c.ID,
		"shortId":       c.ShortID,
		"title":         c.Title,
		"authorName":    c.AuthorName,
		"authoredDate":  c.AuthoredDate,
		"committedDate": c.CommittedDate,
		"webUrl":        c.WebURL,
	}, "title", "authorName")
}

// ToFlatCommit converts an api.Commit to a FlatCommit.
func ToFlatCommit(c *api.Commit) FlatCommit {
	return FlatCommit{
		ID:            c.ID,
		ShortID:       c.ShortID,
		Title:         c.Title,
		AuthorName:    c.AuthorName,
		AuthoredDate:  c.AuthoredDate,
		CommittedDate: c.CommittedDate,
		WebURL:        c.WebURL,
	}
}

// FlatTreeEntry is a token-efficient representation of a GitLab tree entry.
type FlatTreeEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Path string `json:"path"`
	Mode string `json:"mode,omitempty"`
}

// TreeEntryToMap converts a FlatTreeEntry to a map for field filtering.
func TreeEntryToMap(e FlatTreeEntry) map[string]any {
	return MarkUntrusted(map[string]any{
		"id":   e.ID,
		"name": e.Name,
		"type": e.Type,
		"path": e.Path,
		"mode": e.Mode,
	}, "name", "path")
}

// ToFlatTreeEntry converts an api.TreeEntry to a FlatTreeEntry.
func ToFlatTreeEntry(e *api.TreeEntry) FlatTreeEntry {
	return FlatTreeEntry{
		ID:   e.ID,
		Name: e.Name,
		Type: e.Type,
		Path: e.Path,
		Mode: e.Mode,
	}
}
