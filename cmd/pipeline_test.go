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

func TestPipelineHelp_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"pipeline", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"list", "get", "current", "create", "retry", "cancel", "jobs"} {
		if !strings.Contains(out, want) {
			t.Errorf("pipeline --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestPipelineList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"iid":1,"ref":"main","status":"success"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "list", "--project", "42", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"id"`) || !strings.Contains(out, `"success"`) {
		t.Errorf("expected JSON with pipeline data, got:\n%s", out)
	}
}

func TestPipelineGet_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":7,"iid":3,"ref":"main","status":"running"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "get", "--project", "42", "7", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"running"`) {
		t.Errorf("expected running status in JSON, got:\n%s", out)
	}
}

func TestPipelineCreate_DryRun(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "create", "--project", "42", "--ref", "main", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"dryRun": true`) {
		t.Errorf("expected dryRun:true in output, got:\n%s", out)
	}
}

func TestPipelineRetry_DryRun(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "retry", "--project", "42", "10", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"dryRun": true`) {
		t.Errorf("expected dryRun:true, got:\n%s", out)
	}
}

func TestPipelineCancel_DryRun(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "cancel", "--project", "42", "10", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"dryRun": true`) {
		t.Errorf("expected dryRun:true, got:\n%s", out)
	}
}

func TestPipelineJobs_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":5,"name":"test","status":"failed","stage":"test"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "jobs", "--project", "42", "10", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"failed"`) {
		t.Errorf("expected failed status in JSON, got:\n%s", out)
	}
}

func TestPipelineWaitCmd(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		status := "running"
		if callCount >= 3 {
			status = "success"
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"id":10,"iid":1,"ref":"main","status":%q}`, status)
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "wait", "--project", "42", "10", "--interval", "0", "--json"})
		_ = rootCmd.Execute()
	})
	if LastExitCode() != ExitOK {
		t.Errorf("expected exit 0, got %d", LastExitCode())
	}
	if !strings.Contains(out, `"success"`) {
		t.Errorf("expected status success in JSON output, got:\n%s", out)
	}
	lastExit = 0
}

// ── new tests ──────────────────────────────────────────────────────────────────

func TestPipeline_Help_ListsSubcommands(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"pipeline", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(os.Stdout)
	out := buf.String()
	for _, want := range []string{"list", "get", "current", "create", "retry", "cancel", "jobs", "wait"} {
		if !strings.Contains(out, want) {
			t.Errorf("pipeline --help missing %q, got:\n%s", want, out)
		}
	}
}

func TestPipeline_List_MissingProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"pipeline", "list", "--project", ""})
	_ = rootCmd.Execute()

	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d (ExitBadArgs)", lastExit, ExitBadArgs)
	}
}

func TestPipeline_List_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"iid":1,"ref":"main","status":"success","source":"push","created_at":"2024-01-01","web_url":"http://x","project_id":1}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "list", "--project", "42", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"id"`, `"success"`, `"main"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestPipeline_Get_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"ref":"main","status":"success","source":"push","created_at":"2024-01-01","web_url":"http://x","project_id":1}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "get", "--project", "42", "1", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"projectId"`) {
		t.Errorf("expected camelCase projectId key, got:\n%s", out)
	}
}

func TestPipeline_Create_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "create", "--project", "42", "--ref", "main", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"dryRun": true`) {
		t.Errorf("expected dryRun:true, got:\n%s", out)
	}
}

func TestPipeline_Retry_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "retry", "--project", "42", "10", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"dryRun": true`) {
		t.Errorf("expected dryRun:true, got:\n%s", out)
	}
}

func TestPipeline_Cancel_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "cancel", "--project", "42", "10", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"dryRun": true`) {
		t.Errorf("expected dryRun:true, got:\n%s", out)
	}
}

func TestPipeline_Jobs_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":10,"name":"build","status":"success","stage":"build","ref":"main","web_url":"http://x","created_at":"2024-01-01","duration":5.0}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "jobs", "--project", "42", "10", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"build"`, `"success"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestPipeline_Wait_Success(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		status := "running"
		if callCount >= 2 {
			status = "success"
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, `{"id":1,"iid":1,"ref":"main","status":%q,"project_id":1}`, status)
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "wait", "--project", "42", "1", "--interval", "0", "--json"})
		_ = rootCmd.Execute()
	})
	if LastExitCode() != ExitOK {
		t.Errorf("expected exit 0, got %d", LastExitCode())
	}
	if !strings.Contains(out, `"success"`) {
		t.Errorf("expected status success in JSON output, got:\n%s", out)
	}
}

func TestPipeline_Wait_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"ref":"main","status":"failed","project_id":1}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "wait", "--project", "42", "1", "--interval", "0"})
		_ = rootCmd.Execute()
	})
	if LastExitCode() != ExitCIFailed {
		t.Errorf("expected exit %d for failed pipeline, got %d", ExitCIFailed, LastExitCode())
	}
}

func TestPipeline_Wait_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"ref":"main","status":"running","project_id":1}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "wait", "--project", "42", "1", "--timeout", "1", "--interval", "1"})
		_ = rootCmd.Execute()
	})
	if LastExitCode() != ExitTimeout {
		t.Errorf("expected exit %d for timeout, got %d", ExitTimeout, LastExitCode())
	}
}

func TestPipeline_Create_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/pipeline") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":1,"iid":1,"ref":"main","status":"created","source":"api","created_at":"2024-01-01","web_url":"http://x","project_id":1}`)
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
		rootCmd.SetArgs([]string{"pipeline", "create", "--project", "foo/bar", "--ref", "main", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"created"`) {
		t.Errorf("expected status created in output, got: %s", out)
	}
}

func TestPipeline_Retry_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/retry") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":1,"iid":1,"ref":"main","status":"pending","source":"api","created_at":"2024-01-01","web_url":"http://x","project_id":1}`)
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
		rootCmd.SetArgs([]string{"pipeline", "retry", "--project", "foo/bar", "1", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"pending"`) {
		t.Errorf("expected status pending in output, got: %s", out)
	}
}

func TestPipeline_Cancel_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/cancel") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"id":1,"iid":1,"ref":"main","status":"canceled","source":"api","created_at":"2024-01-01","web_url":"http://x","project_id":1}`)
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
		rootCmd.SetArgs([]string{"pipeline", "cancel", "--project", "foo/bar", "1", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"canceled"`) {
		t.Errorf("expected status canceled in output, got: %s", out)
	}
}

func TestPipeline_Get_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"ref":"main","status":"success","source":"push","web_url":"http://x"}`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "get", "--project", "foo/bar", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Pipeline") {
		t.Errorf("expected Pipeline in plain text output:\n%s", out)
	}
}

func TestPipeline_List_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"id":1,"iid":1,"ref":"main","status":"success","source":"push","created_at":"2024-01-01"}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "list", "--project", "42"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "main") && !strings.Contains(out, "success") {
		t.Errorf("expected ref or status in plain text output:\n%s", out)
	}
}

func TestPipeline_Create_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"ref":"main","status":"created","source":"api","created_at":"2024-01-01","web_url":"http://x","project_id":1}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	dryRun = false
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "create", "--project", "foo/bar", "--ref", "main"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Pipeline") {
		t.Errorf("expected 'Pipeline' in output, got: %s", out)
	}
}

func TestPipeline_Retry_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"ref":"main","status":"pending","source":"api","created_at":"2024-01-01","web_url":"http://x","project_id":1}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	dryRun = false
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "retry", "--project", "foo/bar", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "retried") {
		t.Errorf("expected 'retried' in output, got: %s", out)
	}
}

func TestPipeline_Cancel_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"ref":"main","status":"canceled","source":"api","created_at":"2024-01-01","web_url":"http://x","project_id":1}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	dryRun = false
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "cancel", "--project", "foo/bar", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "canceled") {
		t.Errorf("expected 'canceled' in output, got: %s", out)
	}
}

func TestPipeline_Jobs_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"name":"build","status":"success","stage":"build","ref":"main","duration":10.5}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "jobs", "--project", "foo/bar", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "build") {
		t.Errorf("expected job name in output, got: %s", out)
	}
}

func TestPipeline_Wait_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"ref":"main","status":"success","source":"push","created_at":"2024-01-01","web_url":"http://x","project_id":1}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	origExit := lastExit
	defer func() { jsonMode = origJM; lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "wait", "--project", "foo/bar", "1", "--interval", "0", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"success"`) {
		t.Errorf("expected success status in JSON output:\n%s", out)
	}
}

func TestPipeline_Jobs_JSON2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"name":"build","status":"success","stage":"build","ref":"main","duration":10.5}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "jobs", "--project", "foo/bar", "1", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"build"`) {
		t.Errorf("expected job name in JSON output:\n%s", out)
	}
}

func TestPipeline_List_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "list", "--project", "foo/bar"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No pipelines") {
		t.Errorf("expected 'No pipelines' in output, got: %s", out)
	}
}

func TestPipeline_Current_NotInGitRepo(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(origDir) }()
	rootCmd.SetArgs([]string{"pipeline", "current"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit when not in git repo, got %d", lastExit)
	}
}

func TestPipeline_List_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "bad-token")
	origExit := lastExit
	origJM := jsonMode
	defer func() { lastExit = origExit; jsonMode = origJM }()
	lastExit = 0
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	rootCmd.SetArgs([]string{"pipeline", "list", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestPipeline_Get_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"pipeline", "get", "--project", "foo/bar", "999"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestPipeline_Create_MissingRef(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = pipelineCreateCmd.Flags().Set("ref", "")
	rootCmd.SetArgs([]string{"pipeline", "create", "--project", "foo/bar"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestPipeline_Retry_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"pipeline", "retry", "--project", "foo/bar", "999"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestPipeline_Cancel_APIError(t *testing.T) {
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
	rootCmd.SetArgs([]string{"pipeline", "cancel", "--project", "foo/bar", "999"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestPipeline_Jobs_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "bad-token")
	origExit := lastExit
	origJM := jsonMode
	defer func() { lastExit = origExit; jsonMode = origJM }()
	lastExit = 0
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	rootCmd.SetArgs([]string{"pipeline", "jobs", "--project", "foo/bar", "1"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestPipeline_Jobs_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	jsonMode = false
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "jobs", "--project", "foo/bar", "1"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No jobs") {
		t.Errorf("expected 'No jobs' in output, got: %s", out)
	}
}

func TestPipeline_Retry_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = pipelineRetryCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"pipeline", "retry", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestPipeline_Cancel_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = pipelineCancelCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"pipeline", "cancel", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}
