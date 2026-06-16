package cmd

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMRDiscussion_List_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":"abc123","individual_note":false,"notes":[{"id":1,"body":"please fix","author":{"username":"alice"},"created_at":"2024-01-01"}]}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM }()
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "discussion", "list", "--project", "foo/bar", "7", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"abc123"`) || !strings.Contains(out, `"please fix"`) {
		t.Errorf("expected discussion + note body in output, got:\n%s", out)
	}
}

func TestMRDiscussion_List_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[{"id":"abc123","individual_note":false,"notes":[{"id":1,"body":"hello","author":{"username":"alice"},"created_at":"2024-01-01"}]}]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "discussion", "list", "--project", "foo/bar", "7"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "abc123") {
		t.Errorf("expected discussion id in plain text, got:\n%s", out)
	}
}

func TestMRDiscussion_List_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	defer func() { jsonMode = origJM; _ = rootCmd.PersistentFlags().Set("json", "false") }()
	setTextFormatForTest(t)
	_ = rootCmd.PersistentFlags().Set("json", "false")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "discussion", "list", "--project", "foo/bar", "7"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "No discussions") {
		t.Errorf("expected 'No discussions' in output, got:\n%s", out)
	}
}

func TestMRDiscussion_List_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = mrDiscussionListCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"mr", "discussion", "list", "7"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMRDiscussion_List_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "bad")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = mrDiscussionListCmd.Flags().Set("project", "foo/bar")
	rootCmd.SetArgs([]string{"mr", "discussion", "list", "--project", "foo/bar", "7"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

func TestMRDiscussion_Reply_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	origExit := lastExit
	defer func() { dryRun = origDR; jsonMode = origJM; lastExit = origExit }()
	dryRun = true
	jsonMode = true
	lastExit = 0
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "discussion", "reply", "--project", "foo/bar", "7", "--discussion-id", "abc123", "--body", "done", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Errorf("expected exit 0 for dry-run, got %d", lastExit)
	}
	for _, want := range []string{`"confirm_token"`, `"action": "reply mr discussion"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestMRDiscussion_Reply_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"id":42,"body":"done","author":{"username":"alice"},"created_at":"2024-01-01"}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "discussion", "reply", "--project", "foo/bar", "7", "--discussion-id", "abc123", "--body", "done", "--json"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"done"`) {
		t.Errorf("expected reply body in output, got:\n%s", out)
	}
}

func TestMRDiscussion_Reply_MissingDiscussionID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = mrDiscussionReplyCmd.Flags().Set("project", "")
	_ = mrDiscussionReplyCmd.Flags().Set("discussion-id", "")
	_ = mrDiscussionReplyCmd.Flags().Set("body", "")
	rootCmd.SetArgs([]string{"mr", "discussion", "reply", "--project", "foo/bar", "7", "--body", "hi"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMRDiscussion_Reply_MissingBody(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = mrDiscussionReplyCmd.Flags().Set("project", "")
	_ = mrDiscussionReplyCmd.Flags().Set("discussion-id", "")
	_ = mrDiscussionReplyCmd.Flags().Set("body", "")
	rootCmd.SetArgs([]string{"mr", "discussion", "reply", "--project", "foo/bar", "7", "--discussion-id", "abc123"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMRDiscussion_Reply_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = mrDiscussionReplyCmd.Flags().Set("project", "")
	_ = mrDiscussionReplyCmd.Flags().Set("discussion-id", "")
	_ = mrDiscussionReplyCmd.Flags().Set("body", "")
	rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "discussion", "reply", "--project", "foo/bar", "7", "--discussion-id", "nope", "--body", "hi", "--json"}))
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}

// ─── discussion create (inline) ───────────────────────────────────────────────

// mrWithDiffRefs is the GET response an MR diff_refs lookup expects.
const mrWithDiffRefs = `{"iid":7,"title":"feat","state":"opened","diff_refs":{"base_sha":"base1","start_sha":"start1","head_sha":"head1"}}`

// inlineDiscussionServer answers the MR GET (diff_refs auto-fill) and the
// discussion POST so the auto-fill path can be exercised end to end.
func inlineDiscussionServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			fmt.Fprint(w, `{"id":"disc1","individual_note":false,"notes":[{"id":1,"body":"nil deref here","author":{"username":"alice"},"created_at":"2024-01-01","resolvable":true,"resolved":false}]}`)
			return
		}
		fmt.Fprint(w, mrWithDiffRefs)
	}))
}

func TestMRDiscussion_Create_DryRun_JSON(t *testing.T) {
	// Explicit SHAs keep the dry-run free of a network lookup; auto-fill is covered
	// by the execute test below.
	origDR := dryRun
	origJM := jsonMode
	origExit := lastExit
	defer func() { dryRun = origDR; jsonMode = origJM; lastExit = origExit }()
	dryRun = true
	jsonMode = true
	lastExit = 0
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "discussion", "create", "--project", "foo/bar", "7",
			"--new-path", "src/app.go", "--new-line", "42", "--body", "nil deref",
			"--base-sha", "b", "--start-sha", "s", "--head-sha", "h", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Errorf("expected exit 0 for dry-run, got %d", lastExit)
	}
	for _, want := range []string{`"confirm_token"`, `"action": "create mr discussion"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestMRDiscussion_Create_AutoFillSHAs_JSON(t *testing.T) {
	srv := inlineDiscussionServer()
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	_ = mrDiscussionCreateCmd.Flags().Set("base-sha", "")
	_ = mrDiscussionCreateCmd.Flags().Set("start-sha", "")
	_ = mrDiscussionCreateCmd.Flags().Set("head-sha", "")
	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "discussion", "create", "--project", "foo/bar", "7",
			"--new-path", "src/app.go", "--new-line", "42", "--body", "nil deref here", "--json"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"disc1"`) || !strings.Contains(out, `"nil deref here"`) {
		t.Errorf("expected created discussion id + body in output, got:\n%s", out)
	}
	if !strings.Contains(out, `"resolved"`) {
		t.Errorf("expected resolved state on resolvable note, got:\n%s", out)
	}
}

func TestMRDiscussion_Create_IncompleteDiffRefs(t *testing.T) {
	// GitLab leaves base/head SHA null while an MR diff is still computing; the
	// command must fail with a usage error rather than POST an invalid position.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"iid":7,"title":"feat","diff_refs":{"base_sha":null,"start_sha":"s1","head_sha":null}}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = mrDiscussionCreateCmd.Flags().Set("base-sha", "")
	_ = mrDiscussionCreateCmd.Flags().Set("start-sha", "")
	_ = mrDiscussionCreateCmd.Flags().Set("head-sha", "")
	rootCmd.SetArgs([]string{"mr", "discussion", "create", "--project", "foo/bar", "7", "--new-path", "a.go", "--new-line", "1", "--body", "hi", "--json"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d (incomplete diff_refs should be a usage error)", lastExit, ExitBadArgs)
	}
}

func TestMRDiscussion_Create_MissingProject(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = mrDiscussionCreateCmd.Flags().Set("project", "")
	rootCmd.SetArgs([]string{"mr", "discussion", "create", "7", "--new-path", "a.go", "--new-line", "1", "--body", "hi"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMRDiscussion_Create_MissingPosition(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = mrDiscussionCreateCmd.Flags().Set("project", "")
	_ = mrDiscussionCreateCmd.Flags().Set("new-path", "")
	_ = mrDiscussionCreateCmd.Flags().Set("old-path", "")
	_ = mrDiscussionCreateCmd.Flags().Set("new-line", "0")
	_ = mrDiscussionCreateCmd.Flags().Set("old-line", "0")
	rootCmd.SetArgs([]string{"mr", "discussion", "create", "--project", "foo/bar", "7", "--body", "hi"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d (missing position should be a usage error)", lastExit, ExitBadArgs)
	}
}

func TestMRDiscussion_Create_MissingBody(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = mrDiscussionCreateCmd.Flags().Set("project", "")
	_ = mrDiscussionCreateCmd.Flags().Set("body", "")
	rootCmd.SetArgs([]string{"mr", "discussion", "create", "--project", "foo/bar", "7", "--new-path", "a.go", "--new-line", "1"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

// ─── discussion resolve ───────────────────────────────────────────────────────

func TestMRDiscussion_Resolve_DryRun_JSON(t *testing.T) {
	origDR := dryRun
	origJM := jsonMode
	origExit := lastExit
	defer func() { dryRun = origDR; jsonMode = origJM; lastExit = origExit }()
	dryRun = true
	jsonMode = true
	lastExit = 0
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test-token")
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "discussion", "resolve", "--project", "foo/bar", "7", "--discussion-id", "disc1", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Errorf("expected exit 0 for dry-run, got %d", lastExit)
	}
	for _, want := range []string{`"confirm_token"`, `"action": "resolve mr discussion"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run output, got:\n%s", want, out)
		}
	}
}

func TestMRDiscussion_Resolve_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"disc1","individual_note":false,"notes":[{"id":1,"body":"fix","author":{"username":"alice"},"created_at":"2024-01-01","resolvable":true,"resolved":true}]}`)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origDR := dryRun
	origJM := jsonMode
	defer func() { dryRun = origDR; jsonMode = origJM }()
	dryRun = false
	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "discussion", "resolve", "--project", "foo/bar", "7", "--discussion-id", "disc1", "--json"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"disc1"`) || !strings.Contains(out, `"resolved": true`) {
		t.Errorf("expected resolved discussion in output, got:\n%s", out)
	}
}

func TestMRDiscussion_Resolve_MissingDiscussionID(t *testing.T) {
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = mrDiscussionResolveCmd.Flags().Set("project", "")
	_ = mrDiscussionResolveCmd.Flags().Set("discussion-id", "")
	rootCmd.SetArgs([]string{"mr", "discussion", "resolve", "--project", "foo/bar", "7"})
	_ = rootCmd.Execute()
	if lastExit != ExitBadArgs {
		t.Errorf("exit = %d, want %d", lastExit, ExitBadArgs)
	}
}

func TestMRDiscussion_Resolve_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origExit := lastExit
	origDR := dryRun
	defer func() { lastExit = origExit; dryRun = origDR }()
	lastExit = 0
	dryRun = false
	_ = mrDiscussionResolveCmd.Flags().Set("project", "")
	_ = mrDiscussionResolveCmd.Flags().Set("discussion-id", "")
	rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "discussion", "resolve", "--project", "foo/bar", "7", "--discussion-id", "nope", "--json"}))
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for API error, got %d", lastExit)
	}
}
