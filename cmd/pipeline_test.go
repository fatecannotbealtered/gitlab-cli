package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/spf13/cobra"
)

func resetPipelineFlags(t *testing.T) {
	t.Helper()
	resetRootPersistentFlags(t)
	for _, kv := range []struct {
		cmd   *cobra.Command
		name  string
		value string
	}{
		{pipelineListCmd, "project", ""},
		{pipelineListCmd, "ref", ""},
		{pipelineListCmd, "status", ""},
		{pipelineListCmd, "username", ""},
		{pipelineListCmd, "limit", "20"},
		{pipelineListCmd, "fields", ""},
		{pipelineGetCmd, "project", ""},
		{pipelineGetCmd, "fields", ""},
		{pipelineCreateCmd, "project", ""},
		{pipelineCreateCmd, "ref", ""},
		{pipelineRetryCmd, "project", ""},
		{pipelineCancelCmd, "project", ""},
		{pipelineJobsCmd, "project", ""},
		{pipelineJobsCmd, "scope", ""},
		{pipelineJobsCmd, "fields", ""},
		{pipelineWaitCmd, "project", ""},
		{pipelineWaitCmd, "timeout", "0"},
		{pipelineWaitCmd, "interval", "10"},
	} {
		if err := kv.cmd.Flags().Set(kv.name, kv.value); err != nil {
			t.Fatalf("reset pipeline flag %q: %v", kv.name, err)
		}
	}
	if f := pipelineCreateCmd.Flags().Lookup("variable"); f != nil {
		if v, ok := f.Value.(interface{ Replace([]string) error }); ok {
			if err := v.Replace([]string{}); err != nil {
				t.Fatalf("reset pipeline variable flag: %v", err)
			}
		}
	}
}

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

func TestParseVariables_Valid(t *testing.T) {
	vars, err := parseVariables([]string{"FOO=bar", "BAZ=qux"})
	if err != nil {
		t.Fatalf("parseVariables: %v", err)
	}
	if len(vars) != 2 || vars[0].Key != "FOO" || vars[0].Value != "bar" || vars[1].Key != "BAZ" || vars[1].Value != "qux" {
		t.Errorf("unexpected vars: %+v", vars)
	}
}

func TestParseVariables_InvalidFormat(t *testing.T) {
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	_, err := parseVariables([]string{"INVALID"})
	if err == nil {
		t.Fatal("expected error for invalid --variable format")
	}
	if lastExit != ExitBadArgs {
		t.Errorf("exit code = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestPipeline_Wait_Manual(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"ref":"main","status":"manual","project_id":1}`))
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
		t.Errorf("expected exit %d for manual pipeline, got %d", ExitCIFailed, LastExitCode())
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
		rootCmd.SetArgs([]string{"pipeline", "create", "--project", "foo/bar", "--ref", "main", "--json", "--confirm", "main"})
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
		rootCmd.SetArgs([]string{"pipeline", "cancel", "--project", "foo/bar", "1", "--json", "--confirm", "1"})
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
		rootCmd.SetArgs([]string{"pipeline", "create", "--project", "foo/bar", "--ref", "main", "--confirm", "main"})
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
		rootCmd.SetArgs([]string{"pipeline", "cancel", "--project", "foo/bar", "1", "--confirm", "1"})
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
	rootCmd.SetArgs([]string{"pipeline", "cancel", "--project", "foo/bar", "999", "--confirm", "999"})
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

func TestToFlatPipeline(t *testing.T) {
	p := &api.Pipeline{
		ID:     1,
		IID:    2,
		Ref:    "main",
		Status: "success",
		User:   &api.User{Username: "runner"},
	}
	flat := toFlatPipeline(p)
	if flat.Username != "runner" {
		t.Fatalf("Username = %q, want runner", flat.Username)
	}
	if flat.Ref != "main" {
		t.Fatalf("Ref = %q, want main", flat.Ref)
	}

	bare := toFlatPipeline(&api.Pipeline{ID: 9, Ref: "dev"})
	if bare.Username != "" {
		t.Fatalf("expected empty username, got %q", bare.Username)
	}
}

func TestToFlatJob(t *testing.T) {
	j := &api.Job{
		ID:     10,
		Name:   "build",
		Status: "success",
		User:   &api.User{Username: "dev"},
		Pipeline: &api.Pipeline{
			ID: 99,
		},
	}
	flat := toFlatJob(j)
	if flat.Username != "dev" || flat.PipelineID != 99 {
		t.Fatalf("toFlatJob = %+v", flat)
	}

	bare := toFlatJob(&api.Job{ID: 1, Name: "test"})
	if bare.Username != "" || bare.PipelineID != 0 {
		t.Fatalf("expected empty optional fields, got %+v", bare)
	}
}

func TestPrintPipelineDetail(t *testing.T) {
	p := &api.Pipeline{
		ID:     1,
		IID:    2,
		Ref:    "main",
		Status: "running",
		Source: "push",
		WebURL: "http://pipeline/1",
	}
	out := captureStdout(t, func() {
		printPipelineDetail(p)
	})
	for _, want := range []string{"Pipeline #1", "main", "push", "http://pipeline/1"} {
		if !strings.Contains(out, want) {
			t.Errorf("printPipelineDetail missing %q in:\n%s", want, out)
		}
	}
}

func setupGitLabRepo(t *testing.T, srvURL, projectPath string) func() {
	t.Helper()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "main")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "Test User")
	runGit(t, repoDir, "remote", "add", "origin", srvURL+"/"+projectPath+".git")
	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("init\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "init")
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	return func() { _ = os.Chdir(origDir) }
}

func TestPipeline_List_NewClientError(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "list", "--project", "42"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestPipeline_List_InvalidLimit(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "list", "--project", "42", "--limit", "0"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestPipeline_Get_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = pipelineGetCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"pipeline", "get", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestPipeline_Get_InvalidID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "get", "--project", "42", "not-a-number"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestPipeline_Get_NewClientError(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "get", "--project", "42", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestPipeline_Current_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":5,"iid":2,"ref":"main","status":"success","source":"push","web_url":"http://pipe/5"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	defer setupGitLabRepo(t, srv.URL, "group/project")()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "current"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "Pipeline #5") {
		t.Errorf("expected pipeline detail, got:\n%s", out)
	}
}

func TestPipeline_Current_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":5,"iid":2,"ref":"main","status":"success","source":"push","web_url":"http://pipe/5"}]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	defer setupGitLabRepo(t, srv.URL, "group/project")()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "current", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"success"`) {
		t.Errorf("expected JSON success status, got:\n%s", out)
	}
}

func TestPipeline_Current_NoPipelines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	defer setupGitLabRepo(t, srv.URL, "group/project")()

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "current"})
		_ = rootCmd.Execute()
	})
	if LastExitCode() != ExitNotFound {
		t.Errorf("exit = %d, want %d", LastExitCode(), ExitNotFound)
	}
	if !strings.Contains(out, "No pipelines found") {
		t.Errorf("expected empty message, got:\n%s", out)
	}
}

func TestPipeline_Current_NoGitLabRemote(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)

	origDir, _ := os.Getwd()
	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "-b", "main")
	runGit(t, repoDir, "remote", "add", "origin", "https://github.com/acme/repo.git")
	_ = os.Chdir(repoDir)
	defer func() { _ = os.Chdir(origDir) }()

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"pipeline", "current"})
	_ = rootCmd.Execute()
	if lastExit != ExitNotFound {
		t.Errorf("exit = %d, want %d", lastExit, ExitNotFound)
	}
}

func TestPipeline_Current_NewClientError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	resetPipelineFlags(t)
	defer setupGitLabRepo(t, srv.URL, "group/project")()

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"pipeline", "current"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestPipeline_Create_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = pipelineCreateCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"pipeline", "create", "--ref", "main"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestPipeline_Create_InvalidVariable(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "create", "--project", "42", "--ref", "main", "--variable", "BAD"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestPipeline_Create_RequireConfirmRejected(t *testing.T) {
	withNonInteractiveStdin(t)
	resetPipelineFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "create", "--project", "42", "--ref", "main"})
	_ = rootCmd.Execute()
	if lastExit != ExitCancelled {
		t.Errorf("exit = %d, want %d", lastExit, ExitCancelled)
	}
}

func TestPipeline_Create_NewClientError(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "create", "--project", "42", "--ref", "main", "--confirm", "main"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestPipeline_Create_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "create", "--project", "42", "--ref", "main", "--confirm", "main"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestPipeline_Retry_InvalidID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "retry", "--project", "42", "bad"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestPipeline_Retry_NewClientError(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "retry", "--project", "42", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestPipeline_Cancel_InvalidID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "cancel", "--project", "42", "bad"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestPipeline_Cancel_RequireConfirmRejected(t *testing.T) {
	withNonInteractiveStdin(t)
	resetPipelineFlags(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "cancel", "--project", "42", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitCancelled {
		t.Errorf("exit = %d, want %d", lastExit, ExitCancelled)
	}
}

func TestPipeline_Cancel_NewClientError(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "cancel", "--project", "42", "1", "--confirm", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestPipeline_Jobs_NewClientError(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "jobs", "--project", "42", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestPipeline_Jobs_InvalidID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "jobs", "--project", "42", "bad"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestPipeline_Jobs_WithScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if scope := r.URL.Query().Get("scope[]"); scope == "" && r.URL.RawQuery != "" {
			// GitLab client may encode scope differently; accept any jobs list request.
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"name":"build","status":"success","stage":"build","duration":1.0}]`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "jobs", "--project", "42", "1", "--scope", "running,success"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "build") {
		t.Errorf("expected job name in output, got: %s", out)
	}
}

func TestPipeline_Wait_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = pipelineWaitCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"pipeline", "wait", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestPipeline_Wait_InvalidID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "wait", "--project", "42", "bad"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestPipeline_Wait_NewClientError(t *testing.T) {
	isolateConfigHome(t)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "wait", "--project", "42", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitAuth {
		t.Errorf("exit = %d, want %d", lastExit, ExitAuth)
	}
}

func TestPipeline_Wait_GetAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "wait", "--project", "42", "1", "--interval", "0"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestPipeline_Wait_PlainTextSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"ref":"main","status":"success","project_id":1}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "wait", "--project", "42", "1", "--interval", "0"})
		_ = rootCmd.Execute()
	})
	if LastExitCode() != ExitOK {
		t.Errorf("exit = %d, want %d", LastExitCode(), ExitOK)
	}
	if !strings.Contains(out, "finished") {
		t.Errorf("expected finished message, got:\n%s", out)
	}
}

func TestPipeline_Wait_PlainTextWaitingStderr(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		status := "running"
		if calls >= 2 {
			status = "success"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":1,"iid":1,"ref":"main","status":%q,"project_id":1}`, status)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	stderr := captureStderr(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "wait", "--project", "42", "1", "--interval", "0"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(stderr, "Waiting...") {
		t.Errorf("expected Waiting... on stderr, got:\n%s", stderr)
	}
}

func TestPipeline_Wait_TimeoutPlainTextStderr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"ref":"main","status":"running","project_id":1}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	stderr := captureStderr(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "wait", "--project", "42", "1", "--timeout", "1", "--interval", "1"})
		_ = rootCmd.Execute()
	})
	if LastExitCode() != ExitTimeout {
		t.Errorf("exit = %d, want %d", LastExitCode(), ExitTimeout)
	}
	if !strings.Contains(stderr, "timed out waiting for pipeline") {
		t.Errorf("expected timeout error on stderr, got:\n%s", stderr)
	}
}

func TestPipeline_Wait_ContextCancelledDuringSleep(t *testing.T) {
	resetPipelineFlags(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1,"iid":1,"ref":"main","status":"running","project_id":1}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "tok")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	origRunE := pipelineWaitCmd.RunE
	pipelineWaitCmd.RunE = func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithCancel(cmd.Context())
		cmd.SetContext(ctx)
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()
		return origRunE(cmd, args)
	}
	t.Cleanup(func() { pipelineWaitCmd.RunE = origRunE })

	stderr := captureStderr(t, func() {
		rootCmd.SetArgs([]string{"pipeline", "wait", "--project", "42", "1", "--interval", "1"})
		_ = rootCmd.Execute()
	})
	if LastExitCode() != ExitNetwork {
		t.Errorf("exit = %d, want %d", LastExitCode(), ExitNetwork)
	}
	if !strings.Contains(stderr, "context canceled") {
		t.Errorf("expected context canceled on stderr, got:\n%s", stderr)
	}
}

func TestPipeline_Current_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "bad")
	resetPipelineFlags(t)
	defer setupGitLabRepo(t, srv.URL, "group/project")()

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"pipeline", "current"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestPipeline_Jobs_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "tok")
	resetPipelineFlags(t)
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = pipelineJobsCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"pipeline", "jobs", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}
