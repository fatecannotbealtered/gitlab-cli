package output

import (
	"strings"
	"testing"
)

// assertKeys checks that every expected key is present in m and that no key contains "_".
func assertKeys(t *testing.T, fn string, m map[string]any, keys []string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Errorf("%s missing key %q", fn, k)
		}
	}
	for k := range m {
		if strings.Contains(k, "_") {
			t.Errorf("%s has snake_case key %q", fn, k)
		}
	}
}

func TestIssueToMap_Keys(t *testing.T) {
	f := FlatIssue{
		IID: 2, Title: "t", State: "opened", Author: "alice",
		Assignee: "bob", Labels: "bug", Milestone: "v1", WebURL: "http://x",
	}
	m := IssueToMap(f)
	assertKeys(t, "IssueToMap", m, []string{"iid", "title", "state", "author", "assignee", "labels", "milestone", "webUrl"})
}

func TestMRToMap_Keys(t *testing.T) {
	f := FlatMR{
		IID: 1, Title: "t", State: "opened", Source: "feat", Target: "main",
		Author: "alice", WebURL: "http://x", Draft: false,
	}
	m := MRToMap(f)
	assertKeys(t, "MRToMap", m, []string{"iid", "title", "state", "source", "target", "author", "webUrl", "draft"})
}

func TestPipelineToMap_Keys(t *testing.T) {
	f := FlatPipeline{
		ID: 1, IID: 2, Ref: "main", Status: "success", Source: "push",
		WebURL: "http://x", CreatedAt: "2024-01-01", UpdatedAt: "2024-01-02", ProjectID: 5,
	}
	m := PipelineToMap(f)
	assertKeys(t, "PipelineToMap", m, []string{"id", "iid", "ref", "status", "source", "webUrl", "createdAt", "updatedAt", "projectId"})
}

func TestJobToMap_Keys(t *testing.T) {
	f := FlatJob{
		ID: 1, Name: "build", Status: "success", Stage: "build", Ref: "main",
		WebURL: "http://x", CreatedAt: "2024-01-01", StartedAt: "2024-01-01",
		FinishedAt: "2024-01-01", Duration: 10.5, PipelineID: 3,
	}
	m := JobToMap(f)
	assertKeys(t, "JobToMap", m, []string{"id", "name", "status", "stage", "ref", "webUrl", "createdAt", "startedAt", "finishedAt", "duration", "pipelineId"})
}

func TestLabelToMap_Keys(t *testing.T) {
	f := FlatLabel{ID: 1, Name: "bug", Color: "#e11", Description: "a bug"}
	m := LabelToMap(f)
	assertKeys(t, "LabelToMap", m, []string{"id", "name", "color", "description"})
}

func TestMilestoneToMap_Keys(t *testing.T) {
	f := FlatMilestone{
		ID: 1, IID: 2, Title: "v1", State: "active",
		StartDate: "2024-01-01", DueDate: "2024-06-01", WebURL: "http://x",
	}
	m := MilestoneToMap(f)
	assertKeys(t, "MilestoneToMap", m, []string{"id", "iid", "title", "state", "startDate", "dueDate", "webUrl"})
}

func TestProjectToMap_Keys(t *testing.T) {
	f := FlatProject{
		ID: 1, Name: "proj", PathWithNamespace: "g/proj",
		WebURL: "http://x", DefaultBranch: "main", Visibility: "public",
	}
	m := ProjectToMap(f)
	assertKeys(t, "ProjectToMap", m, []string{"id", "name", "pathWithNamespace", "webUrl", "defaultBranch", "visibility"})
}

func TestProjectMemberToMap_Keys(t *testing.T) {
	f := FlatProjectMember{ID: 1, Username: "alice", Name: "Alice", AccessLevel: 40, State: "active"}
	m := ProjectMemberToMap(f)
	assertKeys(t, "ProjectMemberToMap", m, []string{"id", "username", "name", "accessLevel", "state"})
}

func TestReleaseToMap_Keys(t *testing.T) {
	f := FlatRelease{
		TagName: "v1.0", Name: "Release", Description: "desc",
		CreatedAt: "2024-01-01", ReleasedAt: "2024-01-02", CommitID: "abc123", AssetCount: 2,
	}
	m := ReleaseToMap(f)
	assertKeys(t, "ReleaseToMap", m, []string{"tagName", "name", "description", "createdAt", "releasedAt", "commitId", "assetCount"})
}

func TestBranchToMap_Keys(t *testing.T) {
	f := FlatBranch{Name: "main", Default: true, Protected: true, WebURL: "http://x", CommitID: "abc"}
	m := BranchToMap(f)
	assertKeys(t, "BranchToMap", m, []string{"name", "default", "protected", "webUrl", "commitId"})
}

func TestCommitToMap_Keys(t *testing.T) {
	f := FlatCommit{
		ID: "abc", ShortID: "abc123", Title: "fix", AuthorName: "Alice",
		AuthoredDate: "2024-01-01", CommittedDate: "2024-01-01", WebURL: "http://x",
	}
	m := CommitToMap(f)
	assertKeys(t, "CommitToMap", m, []string{"id", "shortId", "title", "authorName", "authoredDate", "committedDate", "webUrl"})
}

func TestTreeEntryToMap_Keys(t *testing.T) {
	f := FlatTreeEntry{ID: "abc", Name: "main.go", Type: "blob", Path: "main.go", Mode: "100644"}
	m := TreeEntryToMap(f)
	assertKeys(t, "TreeEntryToMap", m, []string{"id", "name", "type", "path", "mode"})
}

func TestVariableToMap_Keys(t *testing.T) {
	f := FlatVariable{Key: "FOO", Type: "env_var", Protected: true, Masked: false, EnvScope: "*"}
	m := VariableToMap(f)
	assertKeys(t, "VariableToMap", m, []string{"key", "type", "protected", "masked", "envScope"})
	if _, ok := m["value"]; ok {
		t.Error("VariableToMap must NOT include 'value' key")
	}
}

func TestUserToMap_Keys(t *testing.T) {
	f := FlatUser{ID: 1, Username: "alice", Name: "Alice", Email: "a@x", State: "active", WebURL: "http://x"}
	m := UserToMap(f)
	assertKeys(t, "UserToMap", m, []string{"id", "username", "name", "email", "state", "webUrl"})
}

func TestSearchProjectToMap_Keys(t *testing.T) {
	f := FlatSearchProject{ID: 1, Name: "proj", PathWithNamespace: "g/proj", WebURL: "http://x"}
	m := SearchProjectToMap(f)
	assertKeys(t, "SearchProjectToMap", m, []string{"id", "name", "pathWithNamespace", "webUrl"})
}

func TestSearchIssueToMap_Keys(t *testing.T) {
	f := FlatSearchIssue{ID: 1, IID: 2, Title: "bug", State: "opened", ProjectID: 5, WebURL: "http://x"}
	m := SearchIssueToMap(f)
	assertKeys(t, "SearchIssueToMap", m, []string{"id", "iid", "title", "state", "projectId", "webUrl"})
}

func TestSearchMRToMap_Keys(t *testing.T) {
	f := FlatSearchMR{ID: 1, IID: 2, Title: "feat", State: "opened", ProjectID: 5, WebURL: "http://x"}
	m := SearchMRToMap(f)
	assertKeys(t, "SearchMRToMap", m, []string{"id", "iid", "title", "state", "projectId", "webUrl"})
}

func TestSearchBlobToMap_Keys(t *testing.T) {
	f := FlatSearchBlob{Filename: "main.go", Path: "main.go", Ref: "main", StartLine: 1, Data: "code", ProjectID: 5}
	m := SearchBlobToMap(f)
	assertKeys(t, "SearchBlobToMap", m, []string{"filename", "path", "ref", "startLine", "data", "projectId"})
}

func TestSearchCommitToMap_Keys(t *testing.T) {
	f := FlatSearchCommit{ID: "abc", ShortID: "abc123", Title: "fix", AuthorName: "Alice", CreatedAt: "2024-01-01", WebURL: "http://x"}
	m := SearchCommitToMap(f)
	assertKeys(t, "SearchCommitToMap", m, []string{"id", "shortId", "title", "authorName", "createdAt", "webUrl"})
}

func TestFilterMap_CaseInsensitive(t *testing.T) {
	m := map[string]any{"webUrl": "http://x", "id": 1}
	got := FilterMap(m, []string{"WEBURL", "ID"})
	if got["webUrl"] != "http://x" {
		t.Error("webUrl missing")
	}
	if got["id"] != 1 {
		t.Error("id missing")
	}
}
