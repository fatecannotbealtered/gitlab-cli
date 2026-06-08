package output

import "github.com/fatecannotbealtered/gitlab-cli/internal/api"

// FlatProject is a token-efficient representation of a GitLab project.
type FlatProject struct {
	ID                int    `json:"id"`
	PathWithNamespace string `json:"pathWithNamespace"`
	Name              string `json:"name"`
	Visibility        string `json:"visibility"`
	WebURL            string `json:"webUrl"`
	DefaultBranch     string `json:"defaultBranch"`
}

// ToFlatProject converts an api.Project to FlatProject.
func ToFlatProject(p *api.Project) FlatProject {
	return FlatProject{
		ID:                p.ID,
		PathWithNamespace: p.PathWithNamespace,
		Name:              p.Name,
		Visibility:        p.Visibility,
		WebURL:            p.WebURL,
		DefaultBranch:     p.DefaultBranch,
	}
}

// ProjectToMap converts a FlatProject to a map for field filtering.
func ProjectToMap(p FlatProject) map[string]any {
	return MarkUntrusted(map[string]any{
		"id":                ID(p.ID),
		"pathWithNamespace": p.PathWithNamespace,
		"name":              p.Name,
		"visibility":        p.Visibility,
		"webUrl":            p.WebURL,
		"defaultBranch":     p.DefaultBranch,
	}, "pathWithNamespace", "name", "defaultBranch")
}

// FlatProjectMember is a token-efficient representation of a project member.
type FlatProjectMember struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	State       string `json:"state"`
	AccessLevel int    `json:"accessLevel"`
}

// ToFlatProjectMember converts an api.ProjectMember to FlatProjectMember.
func ToFlatProjectMember(m *api.ProjectMember) FlatProjectMember {
	return FlatProjectMember{
		ID:          m.ID,
		Username:    m.Username,
		Name:        m.Name,
		State:       m.State,
		AccessLevel: m.AccessLevel,
	}
}

// ProjectMemberToMap converts a FlatProjectMember to a map for field filtering.
func ProjectMemberToMap(m FlatProjectMember) map[string]any {
	return MarkUntrusted(map[string]any{
		"id":          ID(m.ID),
		"username":    m.Username,
		"name":        m.Name,
		"state":       m.State,
		"accessLevel": ID(m.AccessLevel),
	}, "username", "name")
}
