package cmd

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// issueBulkServer returns a mock that fails the issues whose iid is in failIIDs
// (returns 404), and succeeds the rest. It tracks how many update calls landed.
func issueBulkServer(t *testing.T, failIIDs map[int]bool) (*httptest.Server, *int) {
	t.Helper()
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// /users for assign resolution
		if strings.Contains(r.URL.Path, "/users") {
			_, _ = w.Write([]byte(`[{"id":42,"username":"alice"}]`))
			return
		}
		mu.Lock()
		calls++
		mu.Unlock()
		// extract iid: .../issues/<iid> or .../issues/<iid>/notes
		iid := extractIIDFromPath(r.URL.Path)
		if failIIDs[iid] {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Not found"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":` + strconv.Itoa(iid) + `,"iid":` + strconv.Itoa(iid) + `,"state":"closed","author":{"username":"alice"},"created_at":"2024-01-01","updated_at":"2024-01-01"}`))
	}))
	return srv, &calls
}

func extractIIDFromPath(path string) int {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == "issues" && i+1 < len(parts) {
			n, _ := strconv.Atoi(parts[i+1])
			return n
		}
		if p == "merge_requests" && i+1 < len(parts) {
			n, _ := strconv.Atoi(parts[i+1])
			return n
		}
	}
	return 0
}

func TestIssueBulkClose_DryRun(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	resetSliceFlagsForTest(t, []string{"issue", "bulk", "close"})
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "x")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "bulk", "close", "--project", "g/r", "--ids", "1,2,3", "--dry-run", "--json", "--compact"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"total":3`, `"action":"issue bulk close"`, `"targets":["1","2","3"]`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dry-run, got:\n%s", want, out)
		}
	}
}

func TestIssueBulkClose_Confirm_Success(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	srv, _ := issueBulkServer(t, nil)
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "x")
	dryRun = false

	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"issue", "bulk", "close", "--project", "g/r", "--ids", "1,2,3", "--json", "--compact"}))
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"total":3`, `"succeeded":3`, `"failed":0`, `"target":"1"`, `"target":"3"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in confirm output, got:\n%s", want, out)
		}
	}
}

func TestIssueBulkClose_PartialFailure_Aggregated(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	srv, _ := issueBulkServer(t, map[int]bool{2: true})
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "x")
	dryRun = false

	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"issue", "bulk", "close", "--project", "g/r", "--ids", "1,2,3", "--json", "--compact"}))
		_ = rootCmd.Execute()
	})
	// Top-level ok=true (batch ran), per-item failure for iid 2, others succeed.
	for _, want := range []string{`"ok":true`, `"succeeded":2`, `"failed":1`, `"E_NOT_FOUND"`, `"target":"2"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in partial-failure output, got:\n%s", want, out)
		}
	}
}

func TestIssueBulkClose_ConfirmReplay_Rejected(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	srv, _ := issueBulkServer(t, nil)
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "x")
	dryRun = false

	confirmedArgs := withConfirmForTest(t, []string{"issue", "bulk", "close", "--project", "g/r", "--ids", "1,2", "--json", "--compact"})

	// First confirm consumes the token.
	_ = captureStdout(t, func() {
		rootCmd.SetArgs(confirmedArgs)
		_ = rootCmd.Execute()
	})
	// Replay the same token: single-use → E_CONFLICT.
	out := captureStdout(t, func() {
		resetSliceFlagsForTest(t, confirmedArgs)
		rootCmd.SetArgs(confirmedArgs)
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "E_CONFLICT") {
		t.Errorf("expected E_CONFLICT on token replay, got:\n%s", out)
	}
}

func TestIssueBulkClose_StopOnFirstError(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	srv, _ := issueBulkServer(t, map[int]bool{1: true})
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "x")
	dryRun = false

	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"issue", "bulk", "close", "--project", "g/r", "--ids", "1,2,3", "--continue-on-error=false", "--json", "--compact"}))
		_ = rootCmd.Execute()
	})
	// Stops at iid 1; 2 and 3 reported as skipped, summary covers only attempted.
	for _, want := range []string{`"failed":1`, `"total":1`, `"skipped":["2","3"]`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in stop-on-error output, got:\n%s", want, out)
		}
	}
}

func TestIssueBulkClose_EmptyIDs_Validation(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	resetSliceFlagsForTest(t, []string{"issue", "bulk", "close"})
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "x")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"issue", "bulk", "close", "--project", "g/r", "--dry-run", "--json", "--compact"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "E_VALIDATION") {
		t.Errorf("expected E_VALIDATION for empty --ids, got:\n%s", out)
	}
}

func TestIssueBulk_ReopenUpdateLabelAssignComment_Confirm(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	srv, _ := issueBulkServer(t, nil)
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "x")
	dryRun = false

	cases := [][]string{
		{"issue", "bulk", "reopen", "--project", "g/r", "--ids", "1,2", "--json", "--compact"},
		{"issue", "bulk", "update", "--project", "g/r", "--ids", "1,2", "--add-labels", "triage", "--json", "--compact"},
		{"issue", "bulk", "label", "--project", "g/r", "--ids", "1,2", "--add", "bug", "--json", "--compact"},
		{"issue", "bulk", "assign", "alice", "--project", "g/r", "--ids", "1,2", "--json", "--compact"},
		{"issue", "bulk", "comment", "--project", "g/r", "--ids", "1,2", "--body", "triaged", "--json", "--compact"},
	}
	for _, args := range cases {
		out := captureStdout(t, func() {
			rootCmd.SetArgs(withConfirmForTest(t, args))
			_ = rootCmd.Execute()
		})
		if !strings.Contains(out, `"succeeded":2`) {
			t.Errorf("%v: expected succeeded:2, got:\n%s", args, out)
		}
	}
}
