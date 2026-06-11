package cmd

import (
	"bytes"
	"context"
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
		{jobRetryCmd, "project", ""},
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
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "retry", "--project", "42", "99", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"confirm_token"`) {
		t.Errorf("expected dryRun:true, got:\n%s", out)
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
	for _, want := range []string{"get", "log", "retry", "cancel", "artifacts", "wait"} {
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
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "retry", "--project", "42", "10", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"confirm_token"`) {
		t.Errorf("expected dryRun:true, got:\n%s", out)
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
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/retry") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"id":10,"name":"build","status":"pending","stage":"build","ref":"main","web_url":"http://x","created_at":"2024-01-01","duration":0}`)
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
	for _, want := range []string{`"log"`, "chunk1", `"success"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
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
	rootCmd.SetArgs(withConfirmForTest(t, []string{"job", "retry", "--project", "42", "99"}))
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
	var gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/trace"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("done\n"))
		case strings.HasSuffix(r.URL.Path, "/jobs/99"):
			if gets.Add(1) == 1 {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"id":99,"name":"build","status":"success","stage":"build","ref":"main"}`))
				return
			}
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
