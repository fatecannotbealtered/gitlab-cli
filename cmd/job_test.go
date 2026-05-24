package cmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

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
	if !strings.Contains(out, `"dryRun": true`) {
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
	if !strings.Contains(out, `"dryRun": true`) {
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
		fmt.Fprintf(w, `{"id":99,"name":"build","status":%q,"stage":"build","ref":"main"}`, status)
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
	if !strings.Contains(out, `"dryRun": true`) {
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
	if !strings.Contains(out, `"dryRun": true`) {
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
		fmt.Fprintf(w, `{"id":10,"name":"build","status":%q,"stage":"build","ref":"main"}`, status)
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
			fmt.Fprint(w, `{"id":10,"name":"build","status":"pending","stage":"build","ref":"main","web_url":"http://x","created_at":"2024-01-01","duration":0}`)
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
		rootCmd.SetArgs([]string{"job", "retry", "--project", "foo/bar", "10", "--json"})
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
		rootCmd.SetArgs([]string{"job", "cancel", "--project", "foo/bar", "10", "--json"})
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
	jsonMode = false
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
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "retry", "--project", "42", "99"})
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
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"job", "cancel", "--project", "42", "99"})
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
	jsonMode = false
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
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

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
	jsonMode = false
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
	jsonMode = false
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
	jsonMode = false
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
	jsonMode = false
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
	jsonMode = false
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
