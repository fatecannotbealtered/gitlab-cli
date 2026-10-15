package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestJob_Get(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/jobs/99" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":99,"name":"build","status":"failed","failure_reason":"script_failure","allow_failure":true,
			"duration":12.5,"pipeline":{"id":7,"status":"failed"},"artifacts_file":{"filename":"artifacts.zip","size":120},
			"artifacts":[{"file_type":"archive","size":120,"filename":"artifacts.zip","file_format":"zip"}]}`))
	}))
	defer srv.Close()

	j, err := newTestClient(srv.URL).Jobs.Get(testCtx, "42", 99)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if j.ID != 99 || j.FailureReason != "script_failure" || !j.AllowFailure || *j.Duration != 12.5 || j.Pipeline.ID != 7 ||
		j.ArtifactsFile.Size != 120 || len(j.Artifacts) != 1 || j.Artifacts[0].FileType != "archive" {
		t.Errorf("unexpected job: %+v", j)
	}
}

func TestJob_Log(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/jobs/99/trace" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte("trace line\n"))
	}))
	defer srv.Close()

	data, err := newTestClient(srv.URL).Jobs.Log(testCtx, "42", 99)
	if err != nil || string(data) != "trace line\n" {
		t.Fatalf("Log: %q %v", data, err)
	}
}

func TestJob_TraceFrom(t *testing.T) {
	cases := []struct {
		status int
		body   string
		offset int
		want   string
	}{
		{http.StatusPartialContent, "new\n", 4, "new\n"},     // GitLab honoured the range
		{http.StatusOK, "old\nnew\n", 4, "new\n"},            // the whole trace: the bytes past offset
		{http.StatusOK, "old\n", 4, ""},                      // nothing new
		{http.StatusRequestedRangeNotSatisfiable, "", 4, ""}, // nothing new yet
		{http.StatusOK, "", 0, ""},                           // an empty trace
		{http.StatusPartialContent, "first\n", 0, "first\n"}, // from the start
		{http.StatusOK, "line\nmore\n", 0, "line\nmore\n"},   // from the start, whole
		{http.StatusOK, "short", 10, ""},                     // the trace shrank (erased)
		{http.StatusPartialContent, "", 3, ""},               // an empty range
		{http.StatusOK, "abc", 3, ""},                        // exactly at the end
		{http.StatusOK, "abcdef", 3, "def"},                  // a partial line
		{http.StatusPartialContent, "x", 100, "x"},           // trust the 206
		{http.StatusOK, strings.Repeat("a", 5), 2, "aaa"},    // ASCII run
		{http.StatusOK, "日本語", 3, "本語"},                      // byte offsets, not runes
		{http.StatusRequestedRangeNotSatisfiable, "ignored", 0, ""},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if want := fmt.Sprintf("bytes=%d-", c.offset); r.Header.Get("Range") != want {
				t.Errorf("Range = %q, want %q", r.Header.Get("Range"), want)
			}
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(c.body))
		}))
		got, err := newTestClient(srv.URL).Jobs.TraceFrom(testCtx, "42", 99, c.offset)
		srv.Close()
		if err != nil || string(got) != c.want {
			t.Errorf("%d %q from %d: got %q %v, want %q", c.status, c.body, c.offset, got, err, c.want)
		}
	}
}

func TestJob_TraceFrom_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not found"}`))
	}))
	defer srv.Close()
	_, err := newTestClient(srv.URL).Jobs.TraceFrom(testCtx, "42", 99, 0)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 404 {
		t.Fatalf("want a 404 APIError, got %v", err)
	}
}

// TestJob_LogStream_FollowsUntilFinal: the whole trace (200) is cut at the offset,
// so nothing is written twice; the stream ends at a final status.
func TestJob_LogStream_FollowsUntilFinal(t *testing.T) {
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/trace"):
			if polls.Load() == 0 {
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte("chunk1\n"))
				return
			}
			_, _ = w.Write([]byte("chunk1\nchunk2\n"))
		case strings.HasSuffix(r.URL.Path, "/jobs/99"):
			status := "running"
			if polls.Add(1) >= 2 {
				status = "manual"
			}
			_, _ = fmt.Fprintf(w, `{"id":99,"status":%q}`, status)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	var buf bytes.Buffer
	if err := newTestClient(srv.URL).Jobs.LogStream(testCtx, "42", 99, &buf, time.Millisecond); err != nil {
		t.Fatalf("LogStream: %v", err)
	}
	if got := buf.String(); got != "chunk1\nchunk2\n" || polls.Load() != 2 {
		t.Errorf("log = %q after %d polls", got, polls.Load())
	}
}

func TestJob_LogStream_Failures(t *testing.T) {
	handler := func(status string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/trace"):
				_, _ = w.Write([]byte("log\n"))
			case status == "404":
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"missing"}`))
			default:
				_, _ = w.Write([]byte(status))
			}
		}
	}
	srv := httptest.NewServer(handler(`{"id":99,"status":"running"}`))
	err := newTestClient(srv.URL).Jobs.LogStream(testCtx, "42", 99, failWriter{}, time.Millisecond)
	srv.Close()
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Errorf("a failing writer: %v", err)
	}
	srv = httptest.NewServer(handler("404"))
	err = newTestClient(srv.URL).Jobs.LogStream(testCtx, "42", 99, &bytes.Buffer{}, time.Millisecond)
	srv.Close()
	if err == nil {
		t.Error("a missing job must fail the stream")
	}
	srv = httptest.NewServer(handler("not-json"))
	err = newTestClient(srv.URL).Jobs.LogStream(testCtx, "42", 99, &bytes.Buffer{}, time.Millisecond)
	srv.Close()
	if err == nil || !strings.Contains(err.Error(), "parsing job status") {
		t.Errorf("a bad status answer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := newTestClient("http://example.com").Jobs.LogStream(ctx, "42", 99, &bytes.Buffer{}, time.Second); !errors.Is(err, context.Canceled) {
		t.Errorf("a cancelled context: %v", err)
	}
}

func TestJob_Actions(t *testing.T) {
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		bodies = append(bodies, b)
		switch {
		case strings.HasSuffix(r.URL.Path, "/jobs/99/retry"):
			_, _ = w.Write([]byte(`{"id":100,"status":"pending"}`))
		case strings.HasSuffix(r.URL.Path, "/jobs/99/cancel"):
			_, _ = w.Write([]byte(`{"id":99,"status":"canceled"}`))
		case strings.HasSuffix(r.URL.Path, "/jobs/99/play"):
			_, _ = w.Write([]byte(`{"id":99,"status":"pending"}`))
		default:
			t.Errorf("path = %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if j, err := c.Jobs.Retry(testCtx, "42", 99); err != nil || j.ID != 100 {
		t.Fatalf("Retry: %+v %v", j, err)
	}
	if j, err := c.Jobs.Cancel(testCtx, "42", 99); err != nil || j.Status != "canceled" {
		t.Fatalf("Cancel: %+v %v", j, err)
	}
	if j, err := c.Jobs.Play(testCtx, "42", 99, nil); err != nil || j.Status != "pending" {
		t.Fatalf("Play: %+v %v", j, err)
	}
	if _, err := c.Jobs.Play(testCtx, "42", 99, []JobVariable{{Key: "A", Value: "1"}}); err != nil {
		t.Fatal(err)
	}
	if bodies[2] != nil {
		t.Errorf("a play without variables sends no body: %v", bodies[2])
	}
	if vars, _ := bodies[3]["job_variables_attributes"].([]any); len(vars) != 1 {
		t.Errorf("play variables: %v", bodies[3])
	}
}

func TestJob_ArtifactsTo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/jobs/99/artifacts" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte("artifact-bytes"))
	}))
	defer srv.Close()

	var buf bytes.Buffer
	if err := newTestClient(srv.URL).Jobs.ArtifactsTo(testCtx, "42", 99, &buf); err != nil || buf.String() != "artifact-bytes" {
		t.Fatalf("ArtifactsTo: %q %v", buf.String(), err)
	}
}

func TestJob_ParseErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if _, err := c.Jobs.Get(testCtx, "42", 99); err == nil || !strings.Contains(err.Error(), "parsing job") {
		t.Errorf("Get: %v", err)
	}
	if _, err := c.Jobs.Retry(testCtx, "42", 99); err == nil || !strings.Contains(err.Error(), "parsing job") {
		t.Errorf("Retry: %v", err)
	}
	if _, err := c.Jobs.Play(testCtx, "42", 99, nil); err == nil || !strings.Contains(err.Error(), "parsing job") {
		t.Errorf("Play: %v", err)
	}
}
