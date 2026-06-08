package output

import (
	"strings"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// assertKeys checks that every expected key is present in m and that no key
// contains "_" except the spec-defined _untrusted marker.
func assertKeys(t *testing.T, fn string, m map[string]any, keys []string) {
	t.Helper()
	keys = append(keys, UntrustedKey)
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Errorf("%s missing key %q", fn, k)
		}
	}
	for k := range m {
		if strings.Contains(k, "_") && k != UntrustedKey {
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

func TestFilterMap_FiltersUntrustedMarker(t *testing.T) {
	m := map[string]any{
		"id":         "1",
		"title":      "external title",
		"state":      "opened",
		UntrustedKey: []string{"title"},
	}
	got := FilterMap(m, []string{"id", UntrustedKey})
	if _, ok := got[UntrustedKey]; ok {
		t.Fatalf("_untrusted should be omitted when no untrusted field is selected: %v", got)
	}

	got = FilterMap(m, []string{"id", "title"})
	fields, ok := got[UntrustedKey].([]string)
	if !ok {
		t.Fatalf("_untrusted = %T, want []string", got[UntrustedKey])
	}
	if len(fields) != 1 || fields[0] != "title" {
		t.Fatalf("_untrusted = %v, want [title]", fields)
	}
}

func TestToFlatMR(t *testing.T) {
	mr := &api.MergeRequest{
		IID: 1, Title: "t", State: "opened",
		SourceBranch: "feat", TargetBranch: "main",
		WebURL: "http://x", Draft: true,
		Author: &api.User{Username: "alice"},
	}
	got := ToFlatMR(mr)
	if got.Author != "alice" || !got.Draft {
		t.Fatalf("ToFlatMR with author = %+v", got)
	}
	nilAuthor := ToFlatMR(&api.MergeRequest{IID: 2})
	if nilAuthor.Author != "" {
		t.Errorf("nil author = %q", nilAuthor.Author)
	}
}

func TestToFlatMRNote(t *testing.T) {
	n := &api.MergeRequestNote{
		ID: 1, Body: "hi", CreatedAt: "2024-01-01",
		Author: &api.User{Username: "bob"},
	}
	got := ToFlatMRNote(n)
	if got.Author != "bob" {
		t.Fatalf("ToFlatMRNote = %+v", got)
	}
	nilAuthor := ToFlatMRNote(&api.MergeRequestNote{ID: 2})
	if nilAuthor.Author != "" {
		t.Errorf("nil author = %q", nilAuthor.Author)
	}
	_ = MRNoteToMap(got)
}

func TestToFlatProject(t *testing.T) {
	p := &api.Project{
		ID: 1, Name: "n", PathWithNamespace: "g/n",
		Visibility: "private", WebURL: "http://x", DefaultBranch: "main",
	}
	got := ToFlatProject(p)
	if got.Name != "n" {
		t.Fatalf("ToFlatProject = %+v", got)
	}
}

func TestToFlatProjectMember(t *testing.T) {
	m := &api.ProjectMember{ID: 1, Username: "u", Name: "U", State: "active", AccessLevel: 30}
	got := ToFlatProjectMember(m)
	if got.AccessLevel != 30 {
		t.Fatalf("ToFlatProjectMember = %+v", got)
	}
}

func TestToFlatRelease(t *testing.T) {
	r := &api.Release{
		TagName: "v1", Name: "Release", Description: "d",
		CreatedAt: "2024-01-01", ReleasedAt: "2024-01-02",
		Author: &api.User{Username: "alice"},
		Commit: &api.Commit{ShortID: "abc123"},
		Assets: &api.ReleaseAssets{Count: 3},
	}
	got := ToFlatRelease(r)
	if got.Author != "alice" || got.CommitID != "abc123" || got.AssetCount != 3 {
		t.Fatalf("ToFlatRelease full = %+v", got)
	}
	bare := ToFlatRelease(&api.Release{TagName: "v0"})
	if bare.Author != "" || bare.CommitID != "" || bare.AssetCount != 0 {
		t.Fatalf("ToFlatRelease bare = %+v", bare)
	}
}

func TestToFlatBranch(t *testing.T) {
	b := &api.Branch{
		Name: "main", Default: true, Protected: true, Merged: false,
		WebURL: "http://x", Commit: &api.Commit{ShortID: "deadbeef"},
	}
	got := ToFlatBranch(b)
	if got.CommitID != "deadbeef" {
		t.Fatalf("ToFlatBranch = %+v", got)
	}
	bare := ToFlatBranch(&api.Branch{Name: "dev"})
	if bare.CommitID != "" {
		t.Fatalf("ToFlatBranch bare = %+v", bare)
	}
}

func TestToFlatCommit(t *testing.T) {
	c := &api.Commit{
		ID: "full", ShortID: "short", Title: "t", AuthorName: "Alice",
		AuthoredDate: "2024-01-01", CommittedDate: "2024-01-02", WebURL: "http://x",
	}
	got := ToFlatCommit(c)
	if got.ShortID != "short" {
		t.Fatalf("ToFlatCommit = %+v", got)
	}
}

func TestToFlatTreeEntry(t *testing.T) {
	e := &api.TreeEntry{ID: "1", Name: "main.go", Type: "blob", Path: "main.go", Mode: "100644"}
	got := ToFlatTreeEntry(e)
	if got.Name != "main.go" {
		t.Fatalf("ToFlatTreeEntry = %+v", got)
	}
}

func TestToFlatSearch(t *testing.T) {
	sp := ToFlatSearchProject(&api.SearchProject{ID: 1, Name: "p", PathWithNamespace: "g/p", WebURL: "http://x", Visibility: "public"})
	if sp.ID != 1 {
		t.Fatalf("ToFlatSearchProject = %+v", sp)
	}
	si := ToFlatSearchIssue(&api.SearchIssue{ID: 1, IID: 2, Title: "t", State: "opened", ProjectID: 5, WebURL: "http://x"})
	if si.ProjectID != 5 {
		t.Fatalf("ToFlatSearchIssue = %+v", si)
	}
	sm := ToFlatSearchMR(&api.SearchMR{ID: 1, IID: 2, Title: "t", State: "opened", ProjectID: 5, WebURL: "http://x"})
	if sm.IID != 2 {
		t.Fatalf("ToFlatSearchMR = %+v", sm)
	}
	sb := ToFlatSearchBlob(&api.SearchBlob{Filename: "f.go", Path: "f.go", Ref: "main", StartLine: 1, Data: "x", ProjectID: 5})
	if sb.Data != "x" {
		t.Fatalf("ToFlatSearchBlob = %+v", sb)
	}
	sc := ToFlatSearchCommit(&api.SearchCommit{ID: "a", ShortID: "b", Title: "t", AuthorName: "Alice", CreatedAt: "2024-01-01", WebURL: "http://x"})
	if sc.ShortID != "b" {
		t.Fatalf("ToFlatSearchCommit = %+v", sc)
	}
}

func TestToFlatVariable(t *testing.T) {
	v := &api.Variable{
		Key: "K", Value: "secret", VariableType: "env_var",
		Protected: true, Masked: true, Raw: true,
		EnvironmentScope: "*", Description: "desc",
	}
	flat := ToFlatVariable(v)
	if flat.Key != "K" {
		t.Fatalf("ToFlatVariable = %+v", flat)
	}
	withValue := ToFlatVariableWithValue(v)
	if withValue.Value != "secret" {
		t.Fatalf("ToFlatVariableWithValue = %+v", withValue)
	}
	_ = VariableWithValueToMap(withValue)
}

func TestIssueToMap_OptionalFields(t *testing.T) {
	f := FlatIssue{IID: 1, Title: "t", State: "opened", Author: "a", Assignee: "b", Labels: "l", Milestone: "m", WebURL: "http://x"}
	m := IssueToMap(f)
	for _, k := range []string{"author", "assignee", "labels", "milestone", "webUrl"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing %q", k)
		}
	}
}

func TestLabelToMap_WithPriority(t *testing.T) {
	p := 1
	m := LabelToMap(FlatLabel{ID: 1, Name: "bug", Color: "#f00", Description: "d", Priority: &p})
	if m["priority"] != 1 {
		t.Fatalf("LabelToMap = %v", m)
	}
}

func TestPipelineToMap_OptionalFields(t *testing.T) {
	m := PipelineToMap(FlatPipeline{
		ID: 1, IID: 2, ProjectID: 3, Ref: "main", SHA: "sha", Status: "success",
		Source: "push", WebURL: "http://x", CreatedAt: "c", UpdatedAt: "u", Username: "alice",
	})
	for _, k := range []string{"projectId", "sha", "source", "webUrl", "createdAt", "updatedAt", "username"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing %q", k)
		}
	}
}

func TestJobToMap_OptionalFields(t *testing.T) {
	m := JobToMap(FlatJob{
		ID: 1, Name: "build", Status: "success", Stage: "build", Ref: "main",
		WebURL: "http://x", CreatedAt: "c", StartedAt: "s", FinishedAt: "f",
		Duration: 1.5, Username: "alice", PipelineID: 9,
	})
	for _, k := range []string{"stage", "ref", "webUrl", "createdAt", "startedAt", "finishedAt", "duration", "username", "pipelineId"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing %q", k)
		}
	}
}

func TestHintForErrorCode_Cancelled(t *testing.T) {
	if HintForErrorCode(ErrCancelled) == "" {
		t.Error("ErrCancelled should have hint")
	}
}
