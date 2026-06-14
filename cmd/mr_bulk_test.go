package cmd

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// mrBulkServer mocks MR mutation endpoints; iids in failIIDs return 404.
func mrBulkServer(t *testing.T, failIIDs map[int]bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		iid := extractIIDFromPath(r.URL.Path)
		if failIIDs[iid] {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"404 Not found"}`))
			return
		}
		// approve/unapprove return 201 with empty-ish body; merge/close/update return MR
		_, _ = w.Write([]byte(`{"id":` + strconv.Itoa(iid) + `,"iid":` + strconv.Itoa(iid) + `,"state":"merged","title":"x","author":{"username":"alice"}}`))
	}))
	return srv
}

func TestMRBulkMerge_MissingDangerous_Rejected(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	resetSliceFlagsForTest(t, []string{"mr", "bulk", "merge"})
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "x")

	out := captureStdout(t, func() {
		// Even with --dry-run, a dangerous batch without --dangerous is rejected.
		rootCmd.SetArgs([]string{"mr", "bulk", "merge", "--project", "g/r", "--ids", "7,8", "--dry-run", "--json", "--compact"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, "E_CONFIRMATION_REQUIRED") || !strings.Contains(out, "--dangerous") {
		t.Errorf("expected dangerous-gate rejection, got:\n%s", out)
	}
}

func TestMRBulkMerge_DryRun_WithDangerous(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	resetSliceFlagsForTest(t, []string{"mr", "bulk", "merge"})
	t.Setenv("GITLAB_CLI_HOST", "https://gitlab.example.com")
	t.Setenv("GITLAB_CLI_TOKEN", "x")

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"mr", "bulk", "merge", "--project", "g/r", "--ids", "7,8", "--dangerous", "--dry-run", "--json", "--compact"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"confirm_token"`, `"action":"mr bulk merge"`, `"total":2`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in dangerous dry-run, got:\n%s", want, out)
		}
	}
}

func TestMRBulkApprove_Confirm_Success(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	srv := mrBulkServer(t, nil)
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "x")
	dryRun = false

	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "bulk", "approve", "--project", "g/r", "--ids", "7,8", "--json", "--compact"}))
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"succeeded":2`, `"failed":0`, `"target":"7"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in approve output, got:\n%s", want, out)
		}
	}
}

func TestMRBulkClose_PartialFailure(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	srv := mrBulkServer(t, map[int]bool{8: true})
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "x")
	dryRun = false

	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "bulk", "close", "--project", "g/r", "--ids", "7,8", "--json", "--compact"}))
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"ok":true`, `"succeeded":1`, `"failed":1`, `"E_NOT_FOUND"`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in close partial-failure, got:\n%s", want, out)
		}
	}
}

func TestMRBulkUpdate_Confirm_Success(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	srv := mrBulkServer(t, nil)
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "x")
	dryRun = false

	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, []string{"mr", "bulk", "update", "--project", "g/r", "--ids", "7,8", "--add-labels", "ready", "--json", "--compact"}))
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"succeeded":2`) {
		t.Errorf("expected succeeded:2 in update output, got:\n%s", out)
	}
}

func TestMRBulkMerge_Confirm_Success(t *testing.T) {
	resetRootPersistentFlags(t)
	resetCompactForTest(t)
	srv := mrBulkServer(t, nil)
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "x")
	dryRun = false

	// Dangerous batch: --dangerous must be present in both dry-run and confirm.
	args := []string{"mr", "bulk", "merge", "--project", "g/r", "--ids", "7,8", "--dangerous", "--json", "--compact"}
	out := captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, args))
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"succeeded":2`, `"failed":0`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in merge output, got:\n%s", want, out)
		}
	}
}
