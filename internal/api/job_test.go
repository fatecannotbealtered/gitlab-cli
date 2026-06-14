package api

import (
	"bytes"
	"context"
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
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":99,"name":"build","status":"success"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	j, err := c.Jobs.Get(testCtx, "42", 99)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if j.ID != 99 || j.Name != "build" {
		t.Errorf("unexpected job: %+v", j)
	}
}

func TestJob_ArtifactsTo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/jobs/99/artifacts" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("artifact-bytes"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	var buf bytes.Buffer
	if err := c.Jobs.ArtifactsTo(testCtx, "42", 99, &buf); err != nil {
		t.Fatalf("ArtifactsTo: %v", err)
	}
	if buf.String() != "artifact-bytes" {
		t.Errorf("got %q, want artifact-bytes", buf.String())
	}
}

func TestJob_LogStream_ManualTerminal(t *testing.T) {
	var statusChecks atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/trace"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("manual job log\n"))
		case strings.HasSuffix(r.URL.Path, "/jobs/99"):
			statusChecks.Add(1)
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"id":99,"name":"deploy","status":"manual"}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	var buf bytes.Buffer
	err := c.Jobs.LogStream(testCtx, "42", 99, &buf, time.Millisecond)
	if err != nil {
		t.Fatalf("LogStream: %v", err)
	}
	if got := buf.String(); got != "manual job log\n" {
		t.Errorf("log = %q, want %q", got, "manual job log\n")
	}
	if statusChecks.Load() != 1 {
		t.Errorf("status checks = %d, want 1 (should not poll after manual terminal state)", statusChecks.Load())
	}
}

func TestJob_Log(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/jobs/99/trace" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("trace line\n"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	data, err := c.Jobs.Log(testCtx, "42", 99)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if string(data) != "trace line\n" {
		t.Errorf("log = %q", string(data))
	}
}

func TestJob_Log_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Jobs.Log(testCtx, "42", 99)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestJob_Retry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/retry") {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":99,"name":"build","status":"pending"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	j, err := c.Jobs.Retry(testCtx, "42", 99)
	if err != nil {
		t.Fatalf("Retry: %v", err)
	}
	if j.Status != "pending" {
		t.Errorf("status = %q", j.Status)
	}
}

func TestJob_Retry_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Jobs.Retry(testCtx, "42", 99)
	if err == nil || !strings.Contains(err.Error(), "parsing job") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestJob_Cancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/cancel") {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":99,"name":"build","status":"canceled"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	j, err := c.Jobs.Cancel(testCtx, "42", 99)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if j.Status != "canceled" {
		t.Errorf("status = %q", j.Status)
	}
}

func TestJob_Cancel_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"forbidden"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Jobs.Cancel(testCtx, "42", 99)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestJob_Artifacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("zip-bytes"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	data, err := c.Jobs.Artifacts(testCtx, "42", 99)
	if err != nil {
		t.Fatalf("Artifacts: %v", err)
	}
	if string(data) != "zip-bytes" {
		t.Errorf("artifacts = %q", string(data))
	}
}

func TestJob_Get_ParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{bad`))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	_, err := c.Jobs.Get(testCtx, "42", 99)
	if err == nil || !strings.Contains(err.Error(), "parsing job") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestJob_LogStream_PartialContentAndSuccess(t *testing.T) {
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/trace"):
			if polls.Load() == 0 {
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte("chunk1\n"))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("chunk2\n"))
		case strings.HasSuffix(r.URL.Path, "/jobs/99"):
			n := polls.Add(1)
			w.WriteHeader(http.StatusOK)
			status := "running"
			if n >= 2 {
				status = "success"
			}
			_, _ = fmt.Fprintf(w, `{"id":99,"status":%q}`, status)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	var buf bytes.Buffer
	if err := c.Jobs.LogStream(testCtx, "42", 99, &buf, time.Millisecond); err != nil {
		t.Fatalf("LogStream: %v", err)
	}
	if got := buf.String(); got != "chunk1\nchunk2\n" {
		t.Errorf("log = %q", got)
	}
}

func TestJob_LogStream_RangeNotSatisfiable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/trace"):
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		case strings.HasSuffix(r.URL.Path, "/jobs/99"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":99,"status":"skipped"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	var buf bytes.Buffer
	if err := c.Jobs.LogStream(testCtx, "42", 99, &buf, time.Millisecond); err != nil {
		t.Fatalf("LogStream: %v", err)
	}
}

func TestJob_LogStream_WriteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/trace"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("log\n"))
		case strings.HasSuffix(r.URL.Path, "/jobs/99"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":99,"status":"running"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	err := c.Jobs.LogStream(testCtx, "42", 99, failWriter{}, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestJob_LogStream_StatusGetError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/trace"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/jobs/99"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"missing"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	err := c.Jobs.LogStream(testCtx, "42", 99, &bytes.Buffer{}, time.Millisecond)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestJob_LogStream_StatusParseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/trace"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/jobs/99"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`not-json`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	err := c.Jobs.LogStream(testCtx, "42", 99, &bytes.Buffer{}, time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "parsing job status") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestJob_LogStream_ContextCancelledAtStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := newTestClient("http://example.com")
	err := c.Jobs.LogStream(ctx, "42", 99, &bytes.Buffer{}, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestJob_LogStream_ContextCancelledWhileWaiting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/trace"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/jobs/99"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":99,"status":"running"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	c := newTestClient(srv.URL)
	err := c.Jobs.LogStream(ctx, "42", 99, &bytes.Buffer{}, 100*time.Millisecond)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestJob_LogStream_DefaultInterval(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/trace"):
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/jobs/99"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":99,"status":"failed"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	if err := c.Jobs.LogStream(testCtx, "42", 99, &bytes.Buffer{}, 0); err != nil {
		t.Fatalf("LogStream: %v", err)
	}
}

func TestJob_LogStreamStatus(t *testing.T) {
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/trace"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("line\n"))
		case strings.HasSuffix(r.URL.Path, "/jobs/99"):
			polls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":99,"status":"success"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	var chunks int
	status, err := c.Jobs.LogStreamStatus(context.Background(), "g/p", 99, time.Millisecond, func(b []byte, offset int) error {
		chunks++
		if offset != 0 || string(b) != "line\n" {
			t.Errorf("unexpected chunk offset=%d body=%q", offset, b)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("LogStreamStatus: %v", err)
	}
	if status != "success" || chunks != 1 {
		t.Errorf("status=%q chunks=%d", status, chunks)
	}
}
