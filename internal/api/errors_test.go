package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func badRequestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad request"}`))
	}))
}

func okHTTPResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestIssue_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, _, err := c.Issues.ListPage(testCtx, "42", IssueListOpts{}); err == nil {
		t.Fatal("ListPage expected error")
	}
	if _, err := c.Issues.Get(testCtx, "42", 1); err == nil {
		t.Fatal("Get expected error")
	}
	if _, err := c.Issues.Create(testCtx, "42", IssueCreateOpts{Title: "x"}); err == nil {
		t.Fatal("Create expected error")
	}
	title := "x"
	if _, err := c.Issues.Update(testCtx, "42", 1, IssueUpdateOpts{Title: &title}); err == nil {
		t.Fatal("Update expected error")
	}
	if _, _, err := c.Issues.ListNotesPage(testCtx, "42", 1, IssueNoteListOpts{}); err == nil {
		t.Fatal("ListNotesPage expected error")
	}
	if _, err := c.Issues.GetNote(testCtx, "42", 1, 2); err == nil {
		t.Fatal("GetNote expected error")
	}
	if _, err := c.Issues.AddNote(testCtx, "42", 1, "hi"); err == nil {
		t.Fatal("AddNote expected error")
	}
	if err := c.Issues.DeleteNote(testCtx, "42", 1, 2); err == nil {
		t.Fatal("DeleteNote expected error")
	}
}

func TestJob_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, err := c.Jobs.Get(testCtx, "42", 99); err == nil {
		t.Fatal("Get expected error")
	}
	if _, err := c.Jobs.Log(testCtx, "42", 99); err == nil {
		t.Fatal("Log expected error")
	}
	if _, err := c.Jobs.Retry(testCtx, "42", 99); err == nil {
		t.Fatal("Retry expected error")
	}
	if _, err := c.Jobs.Cancel(testCtx, "42", 99); err == nil {
		t.Fatal("Cancel expected error")
	}
	if _, err := c.Jobs.Play(testCtx, "42", 99, nil); err == nil {
		t.Fatal("Play expected error")
	}
	if _, err := c.Jobs.TraceFrom(testCtx, "42", 99, 0); err == nil {
		t.Fatal("TraceFrom expected error")
	}
	if err := c.Jobs.ArtifactsTo(testCtx, "42", 99, &bytes.Buffer{}); err == nil {
		t.Fatal("ArtifactsTo expected error")
	}
}

func TestLabel_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, err := c.Labels.List(testCtx, "42", 20); err == nil {
		t.Fatal("List expected error")
	}
	if _, _, err := c.Labels.ListPage(testCtx, "42", LabelListOpts{}); err == nil {
		t.Fatal("ListPage expected error")
	}
	if _, err := c.Labels.Get(testCtx, "42", "bug"); err == nil {
		t.Fatal("Get expected error")
	}
	if _, err := c.Labels.Create(testCtx, "42", LabelCreateOpts{Name: "x", Color: "#000"}); err == nil {
		t.Fatal("Create expected error")
	}
	name := "x"
	if _, err := c.Labels.Update(testCtx, "42", "1", LabelUpdateOpts{NewName: &name}); err == nil {
		t.Fatal("Update expected error")
	}
	if err := c.Labels.Delete(testCtx, "42", "bug"); err == nil {
		t.Fatal("Delete expected error")
	}
}

func TestMilestone_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, err := c.Milestones.List(testCtx, "42", nil); err == nil {
		t.Fatal("List expected error")
	}
	if _, _, err := c.Milestones.ListPage(testCtx, "42", MilestoneListOpts{}); err == nil {
		t.Fatal("ListPage expected error")
	}
	if _, err := c.Milestones.GetByID(testCtx, "42", 1); err == nil {
		t.Fatal("GetByID expected error")
	}
	if _, err := c.Milestones.Create(testCtx, "42", MilestoneCreateOpts{Title: "v1"}); err == nil {
		t.Fatal("Create expected error")
	}
	title := "x"
	if _, err := c.Milestones.Update(testCtx, "42", 1, MilestoneUpdateOpts{Title: &title}); err == nil {
		t.Fatal("Update expected error")
	}
}

func TestMR_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	title := "x"
	if _, err := c.MergeRequests.Update(testCtx, "group/proj", 1, MergeRequestUpdateOpts{Title: &title}); err == nil {
		t.Fatal("Update expected error")
	}
	if _, err := c.MergeRequests.Merge(testCtx, "group/proj", 1, MergeRequestMergeOpts{}); err == nil {
		t.Fatal("Merge expected error")
	}
	if _, _, err := c.MergeRequests.NotesPage(testCtx, "group/proj", 1, 1, 20); err == nil {
		t.Fatal("NotesPage expected error")
	}
	if _, err := c.MergeRequests.AddNote(testCtx, "group/proj", 1, "hi"); err == nil {
		t.Fatal("AddNote expected error")
	}
	if _, err := c.MergeRequests.Approve(testCtx, "group/proj", 1, ""); err == nil {
		t.Fatal("Approve expected error")
	}
	if err := c.MergeRequests.Unapprove(testCtx, "group/proj", 1); err == nil {
		t.Fatal("Unapprove expected error")
	}
	if _, err := c.MergeRequests.RawDiff(testCtx, "group/proj", 1); err == nil {
		t.Fatal("RawDiff expected error")
	}
	if _, err := c.MergeRequests.ReplyDiscussion(testCtx, "group/proj", 1, "d", "hi"); err == nil {
		t.Fatal("ReplyDiscussion expected error")
	}
	if _, err := c.MergeRequests.ResolveDiscussion(testCtx, "group/proj", 1, "d", true); err == nil {
		t.Fatal("ResolveDiscussion expected error")
	}
	if _, err := c.MergeRequests.CreateDiscussion(testCtx, "group/proj", 1, "hi", nil); err == nil {
		t.Fatal("CreateDiscussion expected error")
	}
	if err := c.MergeRequests.DeleteNote(testCtx, "group/proj", 1, 2); err == nil {
		t.Fatal("DeleteNote expected error")
	}
}

func TestPipeline_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, _, err := c.Pipelines.ListPage(testCtx, "42", PipelineListOpts{}); err == nil {
		t.Fatal("ListPage expected error")
	}
	if _, err := c.Pipelines.Get(testCtx, "42", 1); err == nil {
		t.Fatal("Get expected error")
	}
	if _, err := c.Pipelines.Create(testCtx, "42", PipelineCreateBody{Ref: "main"}); err == nil {
		t.Fatal("Create expected error")
	}
	if _, err := c.Pipelines.Retry(testCtx, "42", 1); err == nil {
		t.Fatal("Retry expected error")
	}
	if _, err := c.Pipelines.Cancel(testCtx, "42", 1); err == nil {
		t.Fatal("Cancel expected error")
	}
	if _, _, err := c.Pipelines.JobsPage(testCtx, "42", 1, PipelineJobsOpts{}); err == nil {
		t.Fatal("JobsPage expected error")
	}
}

func TestProject_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, err := c.Projects.List(testCtx, nil); err == nil {
		t.Fatal("List expected error")
	}
	if _, err := c.Projects.Get(testCtx, "1"); err == nil {
		t.Fatal("Get expected error")
	}
	if _, _, err := c.Projects.ListPage(testCtx, ProjectListOpts{Search: "x"}); err == nil {
		t.Fatal("ListPage expected error")
	}
	if _, err := c.Projects.Create(testCtx, &ProjectCreateRequest{Name: "x"}); err == nil {
		t.Fatal("Create expected error")
	}
	if _, _, err := c.Projects.EffectiveMembersPage(testCtx, "1", MemberListOpts{Query: "alice"}); err == nil {
		t.Fatal("EffectiveMembersPage expected error")
	}
	if _, _, err := c.Projects.DirectMembersPage(testCtx, "1", MemberListOpts{UserIDs: []int{7}}); err == nil {
		t.Fatal("DirectMembersPage expected error")
	}
	if _, err := c.Projects.GetNamespace(testCtx, "acme"); err == nil {
		t.Fatal("GetNamespace expected error")
	}
}

func TestRelease_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, _, err := c.Releases.ListPage(testCtx, "1", 1, 10); err == nil {
		t.Fatal("ListPage expected error")
	}
	if _, err := c.Releases.Get(testCtx, "1", "v1.0"); err == nil {
		t.Fatal("Get expected error")
	}
	if _, err := c.Releases.Create(testCtx, "1", ReleaseCreateBody{TagName: "v1", Ref: "main"}); err == nil {
		t.Fatal("Create expected error")
	}
	name := "x"
	if _, err := c.Releases.Update(testCtx, "1", "v1.0", ReleaseUpdateBody{Name: &name}); err == nil {
		t.Fatal("Update expected error")
	}
	if err := c.Releases.Delete(testCtx, "1", "v1.0"); err == nil {
		t.Fatal("Delete expected error")
	}
}

func TestRepo_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	for what, call := range map[string]func() error{
		"GetFile":    func() error { _, err := c.Repos.GetFile(testCtx, "1", "f.txt", "main"); return err },
		"CreateFile": func() error { return c.Repos.CreateFile(testCtx, "1", "f.txt", FileWriteBody{Branch: "main"}) },
		"UpdateFile": func() error { return c.Repos.UpdateFile(testCtx, "1", "f.txt", FileWriteBody{Branch: "main"}) },
		"DeleteFile": func() error { return c.Repos.DeleteFile(testCtx, "1", "f.txt", "main", "m", "") },
		"ListBranchesPage": func() error {
			_, _, err := c.Repos.ListBranchesPage(testCtx, "1", BranchListOpts{Search: "main"})
			return err
		},
		"GetBranch":       func() error { _, err := c.Repos.GetBranch(testCtx, "1", "main"); return err },
		"CreateBranch":    func() error { _, err := c.Repos.CreateBranch(testCtx, "1", "feat/x", "main"); return err },
		"DeleteBranch":    func() error { return c.Repos.DeleteBranch(testCtx, "1", "feat/x") },
		"GetTag":          func() error { _, err := c.Repos.GetTag(testCtx, "1", "v1"); return err },
		"ListCommitsPage": func() error { _, _, err := c.Repos.ListCommitsPage(testCtx, "1", CommitListOpts{}); return err },
		"ListCommits":     func() error { _, err := c.Repos.ListCommits(testCtx, "1", nil); return err },
		"GetCommit":       func() error { _, err := c.Repos.GetCommit(testCtx, "1", "abc"); return err },
		"ListCommitDiff":  func() error { _, _, err := c.Repos.ListCommitDiff(testCtx, "1", "abc", 10); return err },
		"CreateCommit":    func() error { _, err := c.Repos.CreateCommit(testCtx, "1", CommitCreateOpts{}); return err },
		"ListTreePage":    func() error { _, _, err := c.Repos.ListTreePage(testCtx, "1", TreeOpts{}); return err },
		"GroupProjectPaths": func() error {
			_, _, err := c.Repos.GroupProjectPaths(testCtx, "acme", 10)
			return err
		},
		"MemberProjectPaths": func() error { _, _, err := c.Repos.MemberProjectPaths(testCtx, 10); return err },
	} {
		var apiErr *APIError
		if err := call(); !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: expected a 400 APIError, got %v", what, err)
		}
	}
}

func TestSearch_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, _, err := c.Search.ProjectsPage(testCtx, "q", 1, 10); err == nil {
		t.Fatal("ProjectsPage expected error")
	}
	for _, project := range []string{"", "42"} {
		opts := SearchOpts{Project: project, Query: "q", PerPage: 10}
		if _, _, err := c.Search.IssuesPage(testCtx, opts); err == nil {
			t.Fatalf("IssuesPage(%q) expected error", project)
		}
		if _, _, err := c.Search.MergeRequestsPage(testCtx, opts); err == nil {
			t.Fatalf("MergeRequestsPage(%q) expected error", project)
		}
		if _, _, err := c.Search.BlobsPage(testCtx, opts); err == nil {
			t.Fatalf("BlobsPage(%q) expected error", project)
		}
		if _, _, err := c.Search.CommitsPage(testCtx, opts); err == nil {
			t.Fatalf("CommitsPage(%q) expected error", project)
		}
	}
}

func TestUser_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, err := c.Users.Me(testCtx); err == nil {
		t.Fatal("Me expected error")
	}
	if _, err := c.Users.Search(testCtx, "alice", 10); err == nil {
		t.Fatal("Search expected error")
	}
	if _, _, err := c.Users.SearchPage(testCtx, "alice", true, 1, 10); err == nil {
		t.Fatal("SearchPage expected error")
	}
	if _, err := c.Users.GetByUsername(testCtx, "alice"); err == nil {
		t.Fatal("GetByUsername expected error")
	}
}

func TestVariable_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, _, err := c.Variables.ListPage(testCtx, "group/proj", 1, 20); err == nil {
		t.Fatal("ListPage expected error")
	}
	if _, err := c.Variables.Get(testCtx, "group/proj", "FOO", "production"); err == nil {
		t.Fatal("Get expected error")
	}
	if _, err := c.Variables.Create(testCtx, "group/proj", &VariableCreateOpts{Key: "X", Value: "y"}); err == nil {
		t.Fatal("Create expected error")
	}
	value := "x"
	if _, err := c.Variables.Update(testCtx, "group/proj", "FOO", "production", &VariableUpdateOpts{Value: &value}); err == nil {
		t.Fatal("Update expected error")
	}
	if err := c.Variables.Delete(testCtx, "group/proj", "FOO", "production"); err == nil {
		t.Fatal("Delete expected error")
	}
}

func TestJob_Cancel_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Jobs.Cancel(testCtx, "42", 99)
	if err == nil || !strings.Contains(err.Error(), "parsing job") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestJob_LogStream_RawGetError(t *testing.T) {
	c := newTestClient("http://example.com")
	c.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/trace") {
			return nil, errors.New("connection reset by peer")
		}
		return okHTTPResponse(`{"id":99,"status":"running"}`), nil
	})
	err := c.Jobs.LogStream(testCtx, "42", 99, &bytes.Buffer{}, time.Millisecond)
	var ne *NetworkError
	if !errors.As(err, &ne) {
		t.Fatalf("expected RawGet's network error, got %v", err)
	}
}
