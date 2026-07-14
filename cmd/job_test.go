package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

func resetJobFlags(t *testing.T) {
	t.Helper()
	resetRootPersistentFlags(t)
	for _, kv := range []struct {
		cmd   *cobra.Command
		name  string
		value string
	}{
		{jobGetCmd, "project", ""},
		{jobGetCmd, "fields", ""},
		{jobLogCmd, "project", ""},
		{jobLogCmd, "follow", "false"},
		{jobLogCmd, "timeout", "0"},
		{jobLogCmd, "tail", "0"},
		{jobLogCmd, "grep", ""},
		{jobLogCmd, "max-bytes", "0"},
		{jobRetryCmd, "project", ""},
		{jobPlayCmd, "project", ""},
		{jobCancelCmd, "project", ""},
		{jobArtifactsCmd, "project", ""},
		{jobArtifactsCmd, "output", ""},
		{jobWaitCmd, "project", ""},
		{jobWaitCmd, "timeout", "0"},
		{jobWaitCmd, "interval", "5"},
	} {
		if err := kv.cmd.Flags().Set(kv.name, kv.value); err != nil {
			t.Fatalf("reset job flag %q: %v", kv.name, err)
		}
	}
	resetSliceFlagsForTest(t, []string{"job", "play"})
}

func TestJobHelp_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"job", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"get", "log", "retry", "cancel", "artifacts"} {
		if !strings.Contains(out, want) {
			t.Errorf("job --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestJobGet_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":99,"name":"build","status":"success","stage":"build","ref":"main"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "get", "--project", "42", "99", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"success"`) || !strings.Contains(out, `"build"`) {
		t.Errorf("expected job JSON, got:\n%s", out)
	}
}

func TestJobLog_Output(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Running...\nDone."))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "log", "--project", "42", "99"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Running...") {
		t.Errorf("expected log output, got:\n%s", out)
	}
}

func TestJobRetry_DryRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"name":"build","status":"failed","stage":"build","ref":"main"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "retry", "--project", "42", "99", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"confirm_token"`) {
		t.Errorf("expected dry-run confirm_token, got:\n%s", out)
	}
}

func TestJobCancel_DryRun(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "cancel", "--project", "42", "99", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"confirm_token"`) {
		t.Errorf("expected dryRun:true, got:\n%s", out)
	}
}

func TestJobWaitCmd(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		status := "running"
		if callCount >= 2 {
			status = "failed"
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"id":99,"name":"build","status":%q,"stage":"build","ref":"main"}`, status)
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	lastExit = 0
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "wait", "--project", "42", "99", "--interval", "0"})
		_ = rootCmd.Execute()
	})
	if LastExitCode() != ExitCIFailed {
		t.Errorf("expected exit %d for failed job, got %d", ExitCIFailed, LastExitCode())
	}
	lastExit = 0
}

// ── new tests ──────────────────────────────────────────────────────────────────

func TestJob_Help_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"job", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"get", "log", "retry", "play", "cancel", "artifacts", "wait"} {
		if !strings.Contains(out, want) {
			t.Errorf("job --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestJob_List_MissingProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"job", "get", "--project", "", "10"})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", lastExit, ExitBadArgs)
	}
}

func TestJob_Get_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":10,"name":"build","status":"success","stage":"build","ref":"main","web_url":"http://x","created_at":"2024-01-01","duration":5.0}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "get", "--project", "42", "10", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"name": "build"`) {
		t.Errorf("expected name:build in output, got:\n%s", out)
	}
}

func TestJob_Log_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello log"))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "log", "--project", "42", "10"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "hello log") {
		t.Errorf("expected 'hello log' in output, got:\n%s", out)
	}
}

func TestJob_Retry_DryRun_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":10,"name":"build","status":"failed","stage":"build","ref":"main"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "retry", "--project", "42", "10", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"confirm_token"`) {
		t.Errorf("expected dry-run confirm_token, got:\n%s", out)
	}
}

func TestJob_Cancel_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "cancel", "--project", "42", "10", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"confirm_token"`) {
		t.Errorf("expected dryRun:true, got:\n%s", out)
	}
}

func TestJob_Wait_Success(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		status := "running"
		if callCount >= 2 {
			status = "success"
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"id":10,"name":"build","status":%q,"stage":"build","ref":"main"}`, status)
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "wait", "--project", "42", "10", "--interval", "0"})
		_ = rootCmd.Execute()
	})
	if LastExitCode() != ExitOK {
		t.Errorf("expected exit 0, got %d", LastExitCode())
	}
}

func TestJob_Wait_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":10,"name":"build","status":"failed","stage":"build","ref":"main"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "wait", "--project", "42", "10", "--interval", "0"})
		_ = rootCmd.Execute()
	})
	if LastExitCode() != ExitCIFailed {
		t.Errorf("expected exit %d for failed job, got %d", ExitCIFailed, LastExitCode())
	}
}

func TestJob_Wait_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":10,"name":"build","status":"running","stage":"build","ref":"main"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "wait", "--project", "42", "10", "--timeout", "1", "--interval", "1"})
		_ = rootCmd.Execute()
	})
	if LastExitCode() != ExitTimeout {
		t.Errorf("expected exit %d for timeout, got %d", ExitTimeout, LastExitCode())
	}
}

func TestJob_Retry_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/retry") {
			_, _ = fmt.Fprint(w, `{"id":10,"name":"build","status":"pending","stage":"build","ref":"main","web_url":"http://x","created_at":"2024-01-01","duration":0}`)
			return
		}
		// Pre-retry status probe: return a retryable (non-manual) job.
		_, _ = fmt.Fprint(w, `{"id":10,"name":"build","status":"failed","stage":"build","ref":"main"}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"job", "retry", "--project", "foo/bar", "10", "--json"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"pending"`) {
		t.Errorf("expected status pending in output, got: %s", out)
	}
}

func TestJob_Cancel_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/cancel") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":10,"name":"build","status":"canceled","stage":"build","ref":"main","web_url":"http://x","created_at":"2024-01-01","duration":0}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"job", "cancel", "--project", "foo/bar", "10", "--json"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"canceled"`) {
		t.Errorf("expected status canceled in output, got: %s", out)
	}
}

func TestJob_Get_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":99,"name":"build","status":"success","stage":"build","ref":"main","web_url":"http://x"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "get", "--project", "42", "99"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "build") {
		t.Errorf("expected job name in plain text output:\n%s", out)
	}
}

func TestJob_Retry_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"name":"build","status":"pending","stage":"build","ref":"main"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	dryRun = false
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"job", "retry", "--project", "42", "99"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "retried") && !strings.Contains(out, "build") {
		t.Errorf("expected 'retried' or job name in output:\n%s", out)
	}
}

func TestJob_Cancel_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"name":"build","status":"canceled","stage":"build","ref":"main"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	dryRun = false
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"job", "cancel", "--project", "42", "99"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "canceled") && !strings.Contains(out, "build") {
		t.Errorf("expected 'canceled' or job name in output:\n%s", out)
	}
}

func TestJob_Log_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Running...\nDone."))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "log", "--project", "42", "99"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Running") {
		t.Errorf("expected log content in output:\n%s", out)
	}
}

func TestJob_Artifacts_MissingOutput(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "artifacts", "--project", "foo/bar", "99"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestJob_Wait_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"name":"build","status":"success","stage":"build","ref":"main"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origJM := jsonMode
	origExit := lastExit
	defer func() { jsonMode = origJM; lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "wait", "--project", "42", "99", "--interval", "0", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"success"`) {
		t.Errorf("expected success status in JSON output:\n%s", out)
	}
}

func TestJob_Artifacts_JSON(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "artifacts-*.zip")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PK\x03\x04")) // minimal zip header
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origJM := jsonMode
	origExit := lastExit
	defer func() { jsonMode = origJM; lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "artifacts", "--project", "42", "99", "--output", tmpFile.Name(), "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"path"`) && lastExit != ExitOK {
		t.Errorf("expected path in output or exit 0, got: %s (exit %d)", out, lastExit)
	}
}

func TestJob_Wait_Timeout2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":10,"name":"build","status":"running","stage":"build","ref":"main"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origJM := jsonMode
	origExit := lastExit
	defer func() { jsonMode = origJM; lastExit = origExit; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	lastExit = 0
	_ = rootCmd.PersistentFlags().Set("json", "false")
	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "wait", "--project", "42", "10", "--interval", "0", "--timeout", "1"})
		_ = rootCmd.Execute()
	})
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for timeout, got %d", lastExit)
	}
}

func TestJob_Get_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origJM := jsonMode
	defer func() { lastExit = origExit; jsonMode = origJM }()
	lastExit = 0
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	rootCmd.SetArgs([]string{"job", "get", "--project", "foo/bar", "999"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestJob_Log_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origJM := jsonMode
	defer func() { lastExit = origExit; jsonMode = origJM }()
	lastExit = 0
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	rootCmd.SetArgs([]string{"job", "log", "--project", "foo/bar", "999"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestJob_Retry_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	origJM := jsonMode
	defer func() { lastExit = origExit; dryRun = origDR; jsonMode = origJM }()
	lastExit = 0
	dryRun = false
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	rootCmd.SetArgs([]string{"job", "retry", "--project", "foo/bar", "999"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestJob_Cancel_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	origJM := jsonMode
	defer func() { lastExit = origExit; dryRun = origDR; jsonMode = origJM }()
	lastExit = 0
	dryRun = false
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	rootCmd.SetArgs([]string{"job", "cancel", "--project", "foo/bar", "999"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestJob_Wait_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = jobWaitCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"job", "wait", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func jobLogFollowServer(t *testing.T) *httptest.Server {
	t.Helper()
	var polls atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			w.Header().Set("Content-Type", "application/json")
			status := "running"
			if n >= 2 {
				status = "success"
			}
			_, _ = fmt.Fprintf(w, `{"id":99,"name":"build","status":%q,"stage":"build","ref":"main"}`, status)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestJob_Get_NewClientError(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "get", "--project", "42", "99"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestJob_Get_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = jobGetCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"job", "get", "99"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestJob_Get_InvalidID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "get", "--project", "42", "bad"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestJob_Log_NewClientError(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "log", "--project", "42", "99"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestJob_Log_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = jobLogCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"job", "log", "99"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestJob_Log_InvalidID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "log", "--project", "42", "bad"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestJob_Log_Follow_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	srv := jobLogFollowServer(t)
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "log", "--project", "42", "99", "--follow"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "chunk1") || !strings.Contains(out, "chunk2") {
		t.Errorf("expected streamed log chunks, got:\n%s", out)
	}
}

func TestJob_Log_Follow_JSON(t *testing.T) {
	srv := jobLogFollowServer(t)
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "log", "--project", "42", "99", "--follow", "--json"})
		_ = rootCmd.Execute()
	})
	// NDJSON stream: per-chunk {type:"chunk"} lines + final {type:"summary"} line.
	for _, want := range []string{`"type":"chunk"`, "chunk1", `"type":"summary"`, `"status":"success"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in NDJSON output:\n%s", want, out)
		}
	}
	// Every emitted line must be an independent valid JSON envelope.
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Errorf("NDJSON line is not valid JSON: %q (%v)", line, err)
			continue
		}
		if m["ok"] != true || m["schema_version"] == nil || m["type"] == nil {
			t.Errorf("NDJSON line missing ok/schema_version/type: %q", line)
		}
	}
}

func TestJob_Log_Follow_WithTimeout(t *testing.T) {
	srv := jobLogFollowServer(t)
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "log", "--project", "42", "99", "--follow", "--timeout", "60"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "chunk1") {
		t.Errorf("expected streamed log, got:\n%s", out)
	}
}

func TestJob_Retry_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = jobRetryCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"job", "retry", "99"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestJob_Retry_InvalidID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "retry", "--project", "42", "bad"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestJob_Retry_NewClientError(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "retry", "--project", "42", "99"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestJob_Cancel_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = jobCancelCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"job", "cancel", "99"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestJob_Cancel_InvalidID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "cancel", "--project", "42", "bad"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestJob_Cancel_NewClientError(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs(withConfirmForTest(t, []string{"job", "cancel", "--project", "42", "99"}))
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestJob_Artifacts_NewClientError(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "artifacts", "--project", "42", "99", "--output", "out.zip"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestJob_Artifacts_InvalidOutputPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "artifacts", "--project", "42", "99", "--output", "../escape.zip"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestJob_Artifacts_InvalidID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "artifacts", "--project", "42", "bad", "--output", "out.zip"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestJob_Artifacts_OpenFileError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	dir := t.TempDir()
	rootCmd.SetArgs([]string{"job", "artifacts", "--project", "42", "99", "--output", dir})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit when output is a directory, got %d", lastExit)
	}
}

func TestJob_Artifacts_APIError(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "artifacts.zip")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "artifacts", "--project", "42", "99", "--output", tmpFile})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
	if _, err := os.Stat(tmpFile); err == nil {
		t.Error("expected output file removed after API error")
	}
}

func TestJob_Artifacts_PlainText(t *testing.T) {
	setTextFormatForTest(t)
	tmpFile := filepath.Join(t.TempDir(), "artifacts.zip")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PK\x03\x04artifact"))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "artifacts", "--project", "42", "99", "--output", tmpFile})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Artifacts saved") {
		t.Errorf("expected success message, got:\n%s", out)
	}
}

func TestJob_Artifacts_CloseError(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "artifacts.zip")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("PK\x03\x04"))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)

	origClose := closeOutputFile
	origExit := lastExit
	defer func() {
		closeOutputFile = origClose
		lastExit = origExit
	}()
	realClose := origClose
	closeOutputFile = func(f *os.File) error {
		defer realClose(f)
		return errors.New("close failed")
	}
	lastExit = 0

	rootCmd.SetArgs([]string{"job", "artifacts", "--project", "42", "99", "--output", tmpFile})
	_ = rootCmd.Execute()
	if lastExit != ExitNetwork {
		t.Fatalf("exit=%d want=%d", lastExit, ExitNetwork)
	}
}

func TestJob_Wait_NewClientError(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "wait", "--project", "42", "99"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestJob_Wait_InvalidID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "wait", "--project", "42", "bad"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestJob_Wait_GetAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "wait", "--project", "42", "99", "--interval", "0"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestJob_Wait_PlainTextSuccess(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"name":"build","status":"success","stage":"build","ref":"main"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "wait", "--project", "42", "99", "--interval", "0"})
		_ = rootCmd.Execute()
	})
	if LastExitCode() != ExitOK {
		t.Errorf("exit = %d, want %d", LastExitCode(), ExitOK)
	}
	if !strings.Contains(out, "finished") {
		t.Errorf("expected finished message, got:\n%s", out)
	}
}

func TestJob_Wait_PlainTextWaitingStderr(t *testing.T) {
	setTextFormatForTest(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		status := "running"
		if calls >= 2 {
			status = "success"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":99,"name":"build","status":%q,"stage":"build","ref":"main"}`, status)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	stderr := captureStderr(t, func() {
		rootCmd.SetArgs([]string{"job", "wait", "--project", "42", "99", "--interval", "0"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(stderr, "Waiting...") {
		t.Errorf("expected Waiting... on stderr, got:\n%s", stderr)
	}
}

func TestJob_Wait_TimeoutPlainTextStderr(t *testing.T) {
	setTextFormatForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"name":"build","status":"running","stage":"build","ref":"main"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	stderr := captureStderr(t, func() {
		rootCmd.SetArgs([]string{"job", "wait", "--project", "42", "99", "--timeout", "1", "--interval", "1"})
		_ = rootCmd.Execute()
	})
	if LastExitCode() != ExitTimeout {
		t.Errorf("exit = %d, want %d", LastExitCode(), ExitTimeout)
	}
	if !strings.Contains(stderr, "timed out waiting for job") {
		t.Errorf("expected timeout error on stderr, got:\n%s", stderr)
	}
}

func TestJob_Wait_ContextCancelledDuringSleep(t *testing.T) {
	resetJobFlags(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":99,"name":"build","status":"running","stage":"build","ref":"main"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	origRunE := jobWaitCmd.RunE
	jobWaitCmd.RunE = func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		cmd.SetContext(ctx)
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()
		return origRunE(cmd, args)
	}
	t.Cleanup(func() { jobWaitCmd.RunE = origRunE })

	stderr := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"job", "wait", "--project", "42", "99", "--interval", "1"})
		_ = rootCmd.Execute()
	})
	if LastExitCode() != ExitNetwork {
		t.Errorf("exit = %d, want %d", LastExitCode(), ExitNetwork)
	}
	if !strings.Contains(stderr, "context canceled") {
		t.Errorf("expected context canceled on stderr, got:\n%s", stderr)
	}
}

func TestJob_Log_Follow_JSON_LogStreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "log", "--project", "42", "99", "--follow", "--json"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for LogStream API error, got %d", lastExit)
	}
}

func TestJob_Log_Follow_JSON_GetError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/trace"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("done\n"))
		case strings.HasSuffix(r.URL.Path, "/jobs/99"):
			// The in-loop status fetch fails, so the NDJSON stream aborts with a
			// non-zero exit even though a chunk was already streamed.
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "log", "--project", "42", "99", "--follow", "--json"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for Get API error after follow, got %d", lastExit)
	}
}

func TestJob_Artifacts_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = jobArtifactsCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"job", "artifacts", "99", "--output", "out.zip"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

// ── manual-aware retry (#16) ─────────────────────────────────────────────────

func TestJob_Retry_ManualRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":50,"name":"deploy","status":"manual","stage":"deploy","ref":"main"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "retry", "--project", "42", "50", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	// A manual job must be rejected before a confirm token is issued, with a hint
	// pointing at `job play` rather than an auth-troubleshooting message.
	if strings.Contains(out, `"confirm_token"`) {
		t.Errorf("manual job should not receive a confirm token, got:\n%s", out)
	}
	if !strings.Contains(out, "job play") {
		t.Errorf("expected hint pointing to job play, got:\n%s", out)
	}
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d (E_VALIDATION)", lastExit, ExitBadArgs)
	}
}

// ── job play (#18) ───────────────────────────────────────────────────────────

// jobPlayServer mocks GitLab for `job play`: the GET status probe returns a job
// with the given status; POST .../play returns a started (pending) job.
func jobPlayServer(t *testing.T, getStatus string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/play") {
			_, _ = fmt.Fprint(w, `{"id":50,"name":"deploy","status":"pending","stage":"deploy","ref":"main","web_url":"http://x","created_at":"2024-01-01","duration":0}`)
			return
		}
		_, _ = fmt.Fprintf(w, `{"id":50,"name":"deploy","status":%q,"stage":"deploy","ref":"main","pipeline":{"id":7,"ref":"main"}}`, getStatus)
	}))
}

func TestJob_Play_DryRun_JSON(t *testing.T) {
	srv := jobPlayServer(t, "manual")
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "play", "--project", "42", "50", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"confirm_token"`) {
		t.Errorf("expected dry-run confirm_token, got:\n%s", out)
	}
}

func TestJob_Play_JSON(t *testing.T) {
	srv := jobPlayServer(t, "manual")
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"job", "play", "--project", "foo/bar", "50", "--json"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"pending"`) {
		t.Errorf("expected played job (status pending), got:\n%s", out)
	}
}

func TestJob_Play_WithVariables_DryRunKeysOnly(t *testing.T) {
	srv := jobPlayServer(t, "manual")
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "play", "--project", "42", "50", "--variable", "TOKEN=s3cr3t", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"confirm_token"`) {
		t.Errorf("expected dry-run confirm_token, got:\n%s", out)
	}
	// The preview binds/exposes the variable KEY but must never leak the value.
	if !strings.Contains(out, "TOKEN") {
		t.Errorf("expected variable key TOKEN in preview, got:\n%s", out)
	}
	if strings.Contains(out, "s3cr3t") {
		t.Errorf("variable value leaked into dry-run preview:\n%s", out)
	}
}

func TestJob_Play_NotManualRejected(t *testing.T) {
	srv := jobPlayServer(t, "success")
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "play", "--project", "42", "50", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if strings.Contains(out, `"confirm_token"`) {
		t.Errorf("non-manual job should not receive a confirm token, got:\n%s", out)
	}
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d (E_VALIDATION)", lastExit, ExitBadArgs)
	}
}

func TestJob_Play_InvalidVariable(t *testing.T) {
	srv := jobPlayServer(t, "manual")
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "play", "--project", "42", "50", "--variable", "BROKEN", "--dry-run", "--json"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d for invalid --variable", lastExit, ExitBadArgs)
	}
}

func TestJob_Play_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = jobPlayCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"job", "play", "50"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestJob_Play_InvalidID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "play", "--project", "42", "bad"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestJob_Play_NewClientError(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	resetJobFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"job", "play", "--project", "42", "50"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

// ── token-efficient job log read modes (#17) ─────────────────────────────────

func jobTraceServer(t *testing.T, trace string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(trace))
	}))
}

func TestJob_Log_Tail(t *testing.T) {
	srv := jobTraceServer(t, "l1\nl2\nl3\nl4\nl5\n")
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "log", "--project", "42", "99", "--tail", "2"})
		_ = rootCmd.Execute()
	})
	if strings.Contains(out, "l3") || !strings.Contains(out, "l4") || !strings.Contains(out, "l5") {
		t.Errorf("--tail 2 should return only the last 2 lines, got:\n%s", out)
	}
}

func TestJob_Log_Grep(t *testing.T) {
	srv := jobTraceServer(t, "starting\nPlan: 1 to add\nApply complete!\ncleanup\n")
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "log", "--project", "42", "99", "--grep", "Plan:|Apply complete"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Plan: 1 to add") || !strings.Contains(out, "Apply complete!") {
		t.Errorf("--grep should keep matching lines, got:\n%s", out)
	}
	if strings.Contains(out, "starting") || strings.Contains(out, "cleanup") {
		t.Errorf("--grep should drop non-matching lines, got:\n%s", out)
	}
}

func TestJob_Log_MaxBytes(t *testing.T) {
	srv := jobTraceServer(t, "0123456789ABCDEF")
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "log", "--project", "42", "99", "--max-bytes", "4", "--json"})
		_ = rootCmd.Execute()
	})
	m := unwrapJSONDataMap(t, out)
	if log, _ := m["log"].(string); log != "CDEF" {
		t.Errorf("--max-bytes 4 should keep the last 4 bytes 'CDEF', got %q\nfull:\n%s", log, out)
	}
}

func TestJob_Log_Filter_JSONMeta(t *testing.T) {
	srv := jobTraceServer(t, "l1\nl2\nl3\nl4\nl5\n")
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "log", "--project", "42", "99", "--tail", "1", "--json"})
		_ = rootCmd.Execute()
	})
	m := unwrapJSONDataMap(t, out)
	if _, ok := m["totalBytes"]; !ok {
		t.Errorf("expected totalBytes in filtered log JSON, got:\n%s", out)
	}
	if trunc, _ := m["truncated"].(bool); !trunc {
		t.Errorf("expected truncated=true when tailing, got:\n%s", out)
	}
	if log, _ := m["log"].(string); !strings.Contains(log, "l5") || strings.Contains(log, "l1") {
		t.Errorf("expected only the last line, got:\n%s", out)
	}
}

func TestJob_Log_NoFilter_NoMeta(t *testing.T) {
	srv := jobTraceServer(t, "hello\nworld\n")
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "log", "--project", "42", "99", "--json"})
		_ = rootCmd.Execute()
	})
	m := unwrapJSONDataMap(t, out)
	if _, ok := m["totalBytes"]; ok {
		t.Errorf("unfiltered log should not carry truncation meta, got:\n%s", out)
	}
	if log, _ := m["log"].(string); !strings.Contains(log, "hello") || !strings.Contains(log, "world") {
		t.Errorf("expected the full log, got:\n%s", out)
	}
}

// ── review follow-ups: scheduled state, state-drift conflict, UTF-8 boundary ──

func TestJob_Play_Scheduled_DryRun(t *testing.T) {
	srv := jobPlayServer(t, "scheduled")
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "play", "--project", "42", "50", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	// GitLab's play endpoint accepts scheduled (delayed) jobs too, so play must
	// treat them as startable, not reject them.
	if !strings.Contains(out, `"confirm_token"`) {
		t.Errorf("scheduled job should be playable (expect confirm_token), got:\n%s", out)
	}
}

func TestJob_Retry_ScheduledRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":50,"name":"deploy","status":"scheduled","stage":"deploy","ref":"main"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "retry", "--project", "42", "50", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if strings.Contains(out, `"confirm_token"`) {
		t.Errorf("scheduled job should not be retryable, got:\n%s", out)
	}
	if !strings.Contains(out, "job play") {
		t.Errorf("expected hint pointing to job play, got:\n%s", out)
	}
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d (E_VALIDATION)", lastExit, ExitBadArgs)
	}
}

func TestJob_Play_StateDrift_Conflict(t *testing.T) {
	// GET returns manual on the dry-run probe, then running on the confirm probe
	// — simulating another actor playing the job in between. POST /play must not
	// be reached: the confirm-token comparison fails closed first.
	var getCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/play") {
			t.Errorf("POST /play must not be reached on state drift")
			_, _ = fmt.Fprint(w, `{"id":50,"name":"deploy","status":"pending","stage":"deploy","ref":"main"}`)
			return
		}
		status := "manual"
		if getCount.Add(1) >= 2 {
			status = "running"
		}
		_, _ = fmt.Fprintf(w, `{"id":50,"name":"deploy","status":%q,"stage":"deploy","ref":"main","pipeline":{"id":7,"ref":"main"}}`, status)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	origExit := lastExit
	origDR := dryRun
	origJM := jsonMode
	defer func() { lastExit = origExit; dryRun = origDR; jsonMode = origJM }()
	lastExit = 0
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"job", "play", "--project", "foo/bar", "50", "--json"}))
		_ = rootCmd.Execute()
	})
	// A token issued while manual, confirmed after drift to running, must fail
	// closed with E_CONFLICT (exit 6) — not E_VALIDATION.
	if !strings.Contains(out, "E_CONFLICT") {
		t.Errorf("expected E_CONFLICT on state drift, got:\n%s", out)
	}
	if LastExitCode() != ExitConflict {
		t.Errorf("exit = %d, want %d (E_CONFLICT)", LastExitCode(), ExitConflict)
	}
}

func TestJob_Log_MaxBytes_RuneBoundary(t *testing.T) {
	// "世界" runes are 3 bytes each in UTF-8; a byte cap of 7 lands mid-rune.
	srv := jobTraceServer(t, "hello 世界世界世界世界")
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetJobFlags(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "log", "--project", "42", "99", "--max-bytes", "7", "--json"})
		_ = rootCmd.Execute()
	})
	m := unwrapJSONDataMap(t, out)
	log, _ := m["log"].(string)
	if !utf8.ValidString(log) {
		t.Errorf("--max-bytes must not split a UTF-8 rune; got invalid UTF-8: %q", log)
	}
	if len(log) > 7 {
		t.Errorf("returned %d bytes, want <= 7 (the cap)", len(log))
	}
}
