package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRepoCommitCreate_DryRun_JSON(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		resetSliceFlagsForTest(t, []string{"repo", "commit", "create"})
		rootCmd.SetArgs([]string{
			"repo", "commit", "create",
			"--project", "1", "--branch", "main", "--message", "sync",
			"--action", "create:path=a.txt;content=hi",
			"--action", "delete:path=old.txt",
			"--dry-run", "--json", "--compact",
		})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"action":"repo commit create"`, `"total":2`, `"a.txt"`, `"old.txt"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestRepoCommitCreate_Confirm_AtomicSuccess(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/repository/commits") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"abc123def","short_id":"abc123d","title":"sync","web_url":"http://x/abc"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	dryRun = false

	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{
			"repo", "commit", "create",
			"--project", "1", "--branch", "main", "--message", "sync",
			"--action", "create:path=a.txt;content=hi",
			"--action", "delete:path=old.txt",
			"--json", "--compact",
		}))
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"commitId":"abc123def"`, `"shortId":"abc123d"`, `"succeeded":2`, `"failed":0`, `"target":"a.txt"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in confirm output, got:\n%s", want, out)
		}
	}
}

func TestRepoCommitCreate_AtomicFailure_AggregatesAllItems(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Whole commit rejected (e.g. file exists). Atomic: no action applied.
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"A file with this name already exists"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	dryRun = false

	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{
			"repo", "commit", "create",
			"--project", "1", "--branch", "main", "--message", "sync",
			"--action", "create:path=a.txt;content=hi",
			"--action", "create:path=b.txt;content=yo",
			"--json", "--compact",
		}))
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"failed":2`, `"succeeded":0`, `"ok":false`, `"target":"a.txt"`, `"target":"b.txt"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in failure output, got:\n%s", want, out)
		}
	}
}

func TestRepoCommitCreate_InvalidAction(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		resetSliceFlagsForTest(t, []string{"repo", "commit", "create"})
		rootCmd.SetArgs([]string{
			"repo", "commit", "create",
			"--project", "1", "--branch", "main", "--message", "x",
			"--action", "bogus:path=a.txt",
			"--json", "--compact",
		})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "E_VALIDATION") || !strings.Contains(out, "create|update|delete|move") {
		t.Errorf("expected validation error for bad action type, got:\n%s", out)
	}
}

func TestRepoCommitCreate_MoveRequiresPreviousPath(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	out := captureStdout(t, func() {
		resetSliceFlagsForTest(t, []string{"repo", "commit", "create"})
		rootCmd.SetArgs([]string{
			"repo", "commit", "create",
			"--project", "1", "--branch", "main", "--message", "x",
			"--action", "move:path=new.go",
			"--json", "--compact",
		})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "previous_path is required") {
		t.Errorf("expected previous_path error, got:\n%s", out)
	}
}

func TestRepoCommitCreate_ActionsCapEnforced(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")

	args := []string{
		"repo", "commit", "create",
		"--project", "1", "--branch", "main", "--message", "x",
	}
	for i := 0; i < commitActionsMax+1; i++ {
		args = append(args, "--action", "create:path=f"+itoaForTest(i)+".txt;content=x")
	}
	args = append(args, "--json", "--compact")

	out := captureStdout(t, func() {
		resetSliceFlagsForTest(t, []string{"repo", "commit", "create"})
		rootCmd.SetArgs(args)
		_ = rootCmd.Execute()
	})
	t.Cleanup(func() { resetSliceFlagsForTest(t, []string{"repo", "commit", "create"}) })
	if !strings.Contains(out, "at most 1000 actions") {
		t.Errorf("expected actions cap error, got:\n%s", out)
	}
}

func itoaForTest(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}
