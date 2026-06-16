package output

import (
	"strings"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

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

// FlatCommit is a token-efficient representation of a GitLab commit. Stats is
// non-nil only when the caller requested with_stats, so the line counts surface
// only when asked for.
type FlatCommit struct {
	ID            string           `json:"id"`
	ShortID       string           `json:"shortId"`
	Title         string           `json:"title"`
	AuthorName    string           `json:"authorName"`
	AuthoredDate  string           `json:"authoredDate,omitempty"`
	CommittedDate string           `json:"committedDate,omitempty"`
	WebURL        string           `json:"webUrl,omitempty"`
	Stats         *api.CommitStats `json:"stats,omitempty"`
}

// CommitToMap converts a FlatCommit to a map for field filtering. The line-count
// stats are flattened into additions/deletions/total so an agent can select them
// with --fields without a nested object.
func CommitToMap(c FlatCommit) map[string]any {
	m := map[string]any{
		"id":            c.ID,
		"shortId":       c.ShortID,
		"title":         c.Title,
		"authorName":    c.AuthorName,
		"authoredDate":  c.AuthoredDate,
		"committedDate": c.CommittedDate,
		"webUrl":        c.WebURL,
	}
	if c.Stats != nil {
		m["additions"] = c.Stats.Additions
		m["deletions"] = c.Stats.Deletions
		m["total"] = c.Stats.Total
	}
	return MarkUntrusted(m, "title", "authorName")
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
		Stats:         c.Stats,
	}
}

// FlatCommitDiffFile is the token-efficient, projectable shape of one changed
// file in a commit diff. additions/deletions are computed from the patch hunks
// (GitLab's diff endpoint does not return them) so `--fields path,additions,
// deletions` yields a cheap inventory without the full patch text.
type FlatCommitDiffFile struct {
	OldPath     string `json:"oldPath"`
	NewPath     string `json:"newPath"`
	NewFile     bool   `json:"newFile"`
	DeletedFile bool   `json:"deletedFile"`
	RenamedFile bool   `json:"renamedFile"`
	Additions   int    `json:"additions"`
	Deletions   int    `json:"deletions"`
	Diff        string `json:"diff"`
}

// ToFlatCommitDiffFile converts an api.CommitDiffFile, computing per-file line
// counts from the patch.
func ToFlatCommitDiffFile(f *api.CommitDiffFile) FlatCommitDiffFile {
	add, del := countDiffLines(f.Diff)
	return FlatCommitDiffFile{
		OldPath:     f.OldPath,
		NewPath:     f.NewPath,
		NewFile:     f.NewFile,
		DeletedFile: f.DeletedFile,
		RenamedFile: f.RenamedFile,
		Additions:   add,
		Deletions:   del,
		Diff:        f.Diff,
	}
}

// CommitDiffFileToMap converts a FlatCommitDiffFile to a map for field filtering.
// path (the new path, or old path for a deletion) and diff are GitLab-controlled
// untrusted content.
func CommitDiffFileToMap(f FlatCommitDiffFile) map[string]any {
	return MarkUntrusted(map[string]any{
		"oldPath":     f.OldPath,
		"newPath":     f.NewPath,
		"newFile":     f.NewFile,
		"deletedFile": f.DeletedFile,
		"renamedFile": f.RenamedFile,
		"additions":   f.Additions,
		"deletions":   f.Deletions,
		"diff":        f.Diff,
	}, "oldPath", "newPath", "diff")
}

// countDiffLines counts added/removed lines in a unified diff hunk, ignoring the
// +++/--- file headers so the counts reflect content changes only.
func countDiffLines(diff string) (additions, deletions int) {
	if diff == "" {
		return 0, 0
	}
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			continue
		case strings.HasPrefix(line, "+"):
			additions++
		case strings.HasPrefix(line, "-"):
			deletions++
		}
	}
	return additions, deletions
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
