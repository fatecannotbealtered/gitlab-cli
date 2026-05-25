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

	if _, err := c.Issues.List(testCtx, "42", nil); err == nil {
		t.Fatal("List expected error")
	}
	if _, err := c.Issues.Get(testCtx, "42", 1); err == nil {
		t.Fatal("Get expected error")
	}
	if _, err := c.Issues.Create(testCtx, "42", IssueCreateOpts{Title: "x"}); err == nil {
		t.Fatal("Create expected error")
	}
	if _, err := c.Issues.Update(testCtx, "42", 1, IssueUpdateOpts{Title: "x"}); err == nil {
		t.Fatal("Update expected error")
	}
	if _, err := c.Issues.ListNotes(testCtx, "42", 1, 20); err == nil {
		t.Fatal("ListNotes expected error")
	}
	if _, err := c.Issues.AddNote(testCtx, "42", 1, "hi"); err == nil {
		t.Fatal("AddNote expected error")
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
	if _, err := c.Jobs.Artifacts(testCtx, "42", 99); err == nil {
		t.Fatal("Artifacts expected error")
	}
}

func TestLabel_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, err := c.Labels.List(testCtx, "42", 20); err == nil {
		t.Fatal("List expected error")
	}
	if _, err := c.Labels.Create(testCtx, "42", LabelCreateOpts{Name: "x", Color: "#000"}); err == nil {
		t.Fatal("Create expected error")
	}
	if _, err := c.Labels.Update(testCtx, "42", 1, LabelUpdateOpts{NewName: "x"}); err == nil {
		t.Fatal("Update expected error")
	}
}

func TestMilestone_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, err := c.Milestones.List(testCtx, "42", nil); err == nil {
		t.Fatal("List expected error")
	}
	if _, err := c.Milestones.GetByID(testCtx, "42", 1); err == nil {
		t.Fatal("GetByID expected error")
	}
	if _, err := c.Milestones.Create(testCtx, "42", MilestoneCreateOpts{Title: "v1"}); err == nil {
		t.Fatal("Create expected error")
	}
	if _, err := c.Milestones.Update(testCtx, "42", 1, MilestoneUpdateOpts{Title: "x"}); err == nil {
		t.Fatal("Update expected error")
	}
}

func TestMR_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, err := c.MergeRequests.Update(testCtx, "group/proj", 1, &MergeRequestUpdateRequest{Title: "x"}); err == nil {
		t.Fatal("Update expected error")
	}
	if _, err := c.MergeRequests.Merge(testCtx, "group/proj", 1, nil); err == nil {
		t.Fatal("Merge expected error")
	}
	if _, err := c.MergeRequests.ListNotes(testCtx, "group/proj", 1, 20); err == nil {
		t.Fatal("ListNotes expected error")
	}
	if _, err := c.MergeRequests.AddNote(testCtx, "group/proj", 1, "hi"); err == nil {
		t.Fatal("AddNote expected error")
	}
}

func TestPipeline_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, err := c.Pipelines.List(testCtx, "42", nil); err == nil {
		t.Fatal("List expected error")
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
	if _, err := c.Pipelines.Jobs(testCtx, "42", 1, nil); err == nil {
		t.Fatal("Jobs expected error")
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
	if _, err := c.Projects.Members(testCtx, "1", "alice", 20); err == nil {
		t.Fatal("Members expected error")
	}
}

func TestRelease_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, err := c.Releases.List(testCtx, "1", 10); err == nil {
		t.Fatal("List expected error")
	}
	if _, err := c.Releases.Get(testCtx, "1", "v1.0"); err == nil {
		t.Fatal("Get expected error")
	}
	if _, err := c.Releases.Create(testCtx, "1", ReleaseCreateBody{TagName: "v1", Ref: "main"}); err == nil {
		t.Fatal("Create expected error")
	}
	if _, err := c.Releases.Update(testCtx, "1", "v1.0", ReleaseUpdateBody{Name: "x"}); err == nil {
		t.Fatal("Update expected error")
	}
}

func TestRepo_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, err := c.Repos.GetFile(testCtx, "1", "f.txt", "main"); err == nil {
		t.Fatal("GetFile expected error")
	}
	if _, err := c.Repos.GetFileRaw(testCtx, "1", "f.txt", "main"); err == nil {
		t.Fatal("GetFileRaw expected error")
	}
	if _, err := c.Repos.ListBranches(testCtx, "1", &BranchListOpts{Search: "main"}); err == nil {
		t.Fatal("ListBranches expected error")
	}
	if _, err := c.Repos.CreateBranch(testCtx, "1", "feat/x", "main"); err == nil {
		t.Fatal("CreateBranch expected error")
	}
	if _, err := c.Repos.ListCommits(testCtx, "1", nil); err == nil {
		t.Fatal("ListCommits expected error")
	}
	if _, err := c.Repos.GetCommit(testCtx, "1", "abc"); err == nil {
		t.Fatal("GetCommit expected error")
	}
	if _, err := c.Repos.ListTree(testCtx, "1", nil); err == nil {
		t.Fatal("ListTree expected error")
	}
}

func TestSearch_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, err := c.Search.Projects(testCtx, "q", 10); err == nil {
		t.Fatal("Projects expected error")
	}
	if _, err := c.Search.Issues(testCtx, "q", "", 10); err == nil {
		t.Fatal("Issues expected error")
	}
	if _, err := c.Search.Issues(testCtx, "q", "42", 10); err == nil {
		t.Fatal("Issues project expected error")
	}
	if _, err := c.Search.MergeRequests(testCtx, "q", "", 10); err == nil {
		t.Fatal("MergeRequests expected error")
	}
	if _, err := c.Search.MergeRequests(testCtx, "q", "42", 10); err == nil {
		t.Fatal("MergeRequests project expected error")
	}
	if _, err := c.Search.Code(testCtx, "q", "99", 5); err == nil {
		t.Fatal("Code expected error")
	}
	if _, err := c.Search.Commits(testCtx, "q", "", 10); err == nil {
		t.Fatal("Commits expected error")
	}
	if _, err := c.Search.Commits(testCtx, "q", "42", 10); err == nil {
		t.Fatal("Commits project expected error")
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
	if _, err := c.Users.GetByUsername(testCtx, "alice"); err == nil {
		t.Fatal("GetByUsername expected error")
	}
}

func TestVariable_HTTPErrorPaths(t *testing.T) {
	srv := badRequestServer()
	defer srv.Close()
	c := newTestClient(srv.URL)

	if _, err := c.Variables.List(testCtx, "group/proj", 0); err == nil {
		t.Fatal("List expected error")
	}
	if _, err := c.Variables.Get(testCtx, "group/proj", "FOO", "production"); err == nil {
		t.Fatal("Get expected error")
	}
	if _, err := c.Variables.Create(testCtx, "group/proj", &VariableCreateOpts{Key: "X", Value: "y"}); err == nil {
		t.Fatal("Create expected error")
	}
	if _, err := c.Variables.Update(testCtx, "group/proj", "FOO", "production", &VariableUpdateOpts{Value: "x"}); err == nil {
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
	if err == nil || !strings.Contains(err.Error(), "executing request") {
		t.Fatalf("expected RawGet error, got %v", err)
	}
}
