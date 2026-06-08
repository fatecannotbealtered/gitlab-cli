package output

import "github.com/fatecannotbealtered/gitlab-cli/internal/api"

// FlatRelease is a token-efficient representation of a GitLab release.
type FlatRelease struct {
	TagName     string `json:"tagName"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"createdAt,omitempty"`
	ReleasedAt  string `json:"releasedAt,omitempty"`
	Author      string `json:"author,omitempty"`
	CommitID    string `json:"commitId,omitempty"`
	AssetCount  int    `json:"assetCount"`
}

// ReleaseToMap converts a FlatRelease to a map for field filtering.
func ReleaseToMap(r FlatRelease) map[string]any {
	return MarkUntrusted(map[string]any{
		"tagName":     r.TagName,
		"name":        r.Name,
		"description": r.Description,
		"createdAt":   r.CreatedAt,
		"releasedAt":  r.ReleasedAt,
		"author":      r.Author,
		"commitId":    r.CommitID,
		"assetCount":  r.AssetCount,
	}, "tagName", "name", "description")
}

// ToFlatRelease converts an api.Release to a FlatRelease.
func ToFlatRelease(r *api.Release) FlatRelease {
	f := FlatRelease{
		TagName:     r.TagName,
		Name:        r.Name,
		Description: r.Description,
		CreatedAt:   r.CreatedAt,
		ReleasedAt:  r.ReleasedAt,
	}
	if r.Author != nil {
		f.Author = r.Author.Username
	}
	if r.Commit != nil {
		f.CommitID = r.Commit.ShortID
	}
	if r.Assets != nil {
		f.AssetCount = r.Assets.Count
	}
	return f
}
