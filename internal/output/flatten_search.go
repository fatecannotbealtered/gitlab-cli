package output

import "github.com/fatecannotbealtered/gitlab-cli/internal/api"

// FlatSearchProject is a slim search result for a project.
type FlatSearchProject struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"pathWithNamespace"`
	WebURL            string `json:"webUrl"`
	Visibility        string `json:"visibility"`
}

// SearchProjectToMap converts FlatSearchProject to a map for field filtering.
func SearchProjectToMap(p FlatSearchProject) map[string]any {
	return MarkUntrusted(map[string]any{
		"id":                ID(p.ID),
		"name":              p.Name,
		"pathWithNamespace": p.PathWithNamespace,
		"webUrl":            p.WebURL,
		"visibility":        p.Visibility,
	}, "name", "pathWithNamespace")
}

// ToFlatSearchProject converts api.SearchProject to FlatSearchProject.
func ToFlatSearchProject(p *api.SearchProject) FlatSearchProject {
	return FlatSearchProject{
		ID:                p.ID,
		Name:              p.Name,
		PathWithNamespace: p.PathWithNamespace,
		WebURL:            p.WebURL,
		Visibility:        p.Visibility,
	}
}

// FlatSearchIssue is a slim search result for an issue.
type FlatSearchIssue struct {
	ID        int    `json:"id"`
	IID       int    `json:"iid"`
	Title     string `json:"title"`
	State     string `json:"state"`
	WebURL    string `json:"webUrl"`
	ProjectID int    `json:"projectId"`
}

// SearchIssueToMap converts FlatSearchIssue to a map for field filtering.
func SearchIssueToMap(i FlatSearchIssue) map[string]any {
	return MarkUntrusted(map[string]any{
		"id":        ID(i.ID),
		"iid":       ID(i.IID),
		"title":     i.Title,
		"state":     i.State,
		"webUrl":    i.WebURL,
		"projectId": ID(i.ProjectID),
	}, "title")
}

// ToFlatSearchIssue converts api.SearchIssue to FlatSearchIssue.
func ToFlatSearchIssue(i *api.SearchIssue) FlatSearchIssue {
	return FlatSearchIssue{
		ID:        i.ID,
		IID:       i.IID,
		Title:     i.Title,
		State:     i.State,
		WebURL:    i.WebURL,
		ProjectID: i.ProjectID,
	}
}

// FlatSearchMR is a slim search result for a merge request.
type FlatSearchMR struct {
	ID        int    `json:"id"`
	IID       int    `json:"iid"`
	Title     string `json:"title"`
	State     string `json:"state"`
	WebURL    string `json:"webUrl"`
	ProjectID int    `json:"projectId"`
}

// SearchMRToMap converts FlatSearchMR to a map for field filtering.
func SearchMRToMap(m FlatSearchMR) map[string]any {
	return MarkUntrusted(map[string]any{
		"id":        ID(m.ID),
		"iid":       ID(m.IID),
		"title":     m.Title,
		"state":     m.State,
		"webUrl":    m.WebURL,
		"projectId": ID(m.ProjectID),
	}, "title")
}

// ToFlatSearchMR converts api.SearchMR to FlatSearchMR.
func ToFlatSearchMR(m *api.SearchMR) FlatSearchMR {
	return FlatSearchMR{
		ID:        m.ID,
		IID:       m.IID,
		Title:     m.Title,
		State:     m.State,
		WebURL:    m.WebURL,
		ProjectID: m.ProjectID,
	}
}

// FlatSearchBlob is a slim code search result.
type FlatSearchBlob struct {
	Filename  string `json:"filename"`
	Path      string `json:"path"`
	Ref       string `json:"ref"`
	StartLine int    `json:"startLine"`
	Data      string `json:"data"`
	ProjectID int    `json:"projectId"`
}

// SearchBlobToMap converts FlatSearchBlob to a map for field filtering.
func SearchBlobToMap(b FlatSearchBlob) map[string]any {
	return MarkUntrusted(map[string]any{
		"filename":  b.Filename,
		"path":      b.Path,
		"ref":       b.Ref,
		"startLine": ID(b.StartLine),
		"data":      b.Data,
		"projectId": ID(b.ProjectID),
	}, "filename", "path", "ref", "data")
}

// ToFlatSearchBlob converts api.SearchBlob to FlatSearchBlob.
func ToFlatSearchBlob(b *api.SearchBlob) FlatSearchBlob {
	return FlatSearchBlob{
		Filename:  b.Filename,
		Path:      b.Path,
		Ref:       b.Ref,
		StartLine: b.StartLine,
		Data:      b.Data,
		ProjectID: b.ProjectID,
	}
}

// FlatSearchCommit is a slim commit search result.
type FlatSearchCommit struct {
	ID         string `json:"id"`
	ShortID    string `json:"shortId"`
	Title      string `json:"title"`
	AuthorName string `json:"authorName"`
	CreatedAt  string `json:"createdAt"`
	WebURL     string `json:"webUrl"`
}

// SearchCommitToMap converts FlatSearchCommit to a map for field filtering.
func SearchCommitToMap(c FlatSearchCommit) map[string]any {
	return MarkUntrusted(map[string]any{
		"id":         c.ID,
		"shortId":    c.ShortID,
		"title":      c.Title,
		"authorName": c.AuthorName,
		"createdAt":  c.CreatedAt,
		"webUrl":     c.WebURL,
	}, "title", "authorName")
}

// ToFlatSearchCommit converts api.SearchCommit to FlatSearchCommit.
func ToFlatSearchCommit(c *api.SearchCommit) FlatSearchCommit {
	return FlatSearchCommit{
		ID:         c.ID,
		ShortID:    c.ShortID,
		Title:      c.Title,
		AuthorName: c.AuthorName,
		CreatedAt:  c.CreatedAt,
		WebURL:     c.WebURL,
	}
}
