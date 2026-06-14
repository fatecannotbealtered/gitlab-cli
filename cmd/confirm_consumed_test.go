package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// tokenFingerprint must be stable per token and collision-free for distinct ones,
// since single-use enforcement keys off it.
func TestTokenFingerprintStable(t *testing.T) {
	a := tokenFingerprint("ct_123_abc")
	b := tokenFingerprint("ct_123_abc")
	c := tokenFingerprint("ct_123_abd")
	if a != b {
		t.Fatal("fingerprint not stable for the same token")
	}
	if a == c {
		t.Fatal("fingerprint collided for different tokens")
	}
	if len(a) != 16 {
		t.Fatalf("fingerprint length = %d, want 16", len(a))
	}
}

// markConfirmTokenConsumed prunes entries past their expiry so the store cannot
// grow without bound.
func TestConsumedTokensPruneExpired(t *testing.T) {
	isolateConfigHome(t)
	now := time.Now().UTC()
	markConfirmTokenConsumed("ct_1_expired", now.Add(-time.Hour).Unix(), now)
	markConfirmTokenConsumed("ct_2_live", now.Add(time.Hour).Unix(), now)

	if isConfirmTokenConsumed("ct_1_expired", now) {
		t.Error("expired token should be pruned, not reported consumed")
	}
	if !isConfirmTokenConsumed("ct_2_live", now) {
		t.Error("live token should still be reported consumed")
	}
}

// A confirm token may drive exactly one write; replaying it (e.g. an agent
// retrying a confirmed write that timed out) must be rejected with E_CONFLICT so
// the operation cannot be duplicated.
func TestConfirmTokenSingleUse_Replay_Conflict(t *testing.T) {
	isolateConfigHome(t)

	deletes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/repository/branches/") {
			deletes++
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origExit := lastExit
	defer func() { lastExit = origExit; resetRootPersistentFlags(t) }()

	args := []string{
		"repo", "branch", "delete",
		"--project", "foo/bar", "--name", "feature",
		"--dangerous", "--json",
	}
	confirmArgs := withConfirmForTest(t, args)

	// First confirm: succeeds and consumes the token.
	resetRootPersistentFlags(t)
	lastExit = 0
	captureStdout(t, func() {
		rootCmd.SetArgs(confirmArgs)
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Fatalf("first confirm exit = %d, want %d", lastExit, ExitOK)
	}
	if deletes != 1 {
		t.Fatalf("first confirm should issue exactly one delete, got %d", deletes)
	}

	// Replay the same token: rejected as already-used, no second delete.
	resetRootPersistentFlags(t)
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs(confirmArgs)
		_ = rootCmd.Execute()
	})
	if lastExit != ExitConflict {
		t.Fatalf("replay exit = %d, want %d (E_CONFLICT)", lastExit, ExitConflict)
	}
	if !strings.Contains(out, "E_CONFLICT") || !strings.Contains(out, "already used") {
		t.Fatalf("replay should report already-used E_CONFLICT, got:\n%s", out)
	}
	if deletes != 1 {
		t.Fatalf("replay must not issue a second delete, total deletes = %d", deletes)
	}
}

// Write-dangerous commands require --dangerous in BOTH the dry-run and the
// confirm step; missing it must exit 5/E_CONFIRMATION_REQUIRED before any API
// call.
func TestDangerousGate_MissingFlag_BothSteps(t *testing.T) {
	isolateConfigHome(t)

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origExit := lastExit
	defer func() { lastExit = origExit; resetRootPersistentFlags(t) }()

	base := []string{"repo", "branch", "delete", "--project", "foo/bar", "--name", "feature", "--json"}

	// Dry-run step without --dangerous → exit 5.
	resetRootPersistentFlags(t)
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs(append(append([]string{}, base...), "--dry-run"))
		_ = rootCmd.Execute()
	})
	if lastExit != ExitConfirm {
		t.Fatalf("dry-run without --dangerous exit = %d, want %d", lastExit, ExitConfirm)
	}
	if !strings.Contains(out, "E_CONFIRMATION_REQUIRED") || !strings.Contains(out, "write-dangerous") {
		t.Fatalf("dry-run gate message missing, got:\n%s", out)
	}

	// Confirm step without --dangerous → exit 5, and no API call.
	token, _ := newConfirmToken("repo branch delete", map[string]any{"project": "foo/bar", "name": "feature"})
	resetRootPersistentFlags(t)
	lastExit = 0
	out = captureStdout(t, func() {
		rootCmd.SetArgs(append(append([]string{}, base...), "--confirm", token))
		_ = rootCmd.Execute()
	})
	if lastExit != ExitConfirm {
		t.Fatalf("confirm without --dangerous exit = %d, want %d", lastExit, ExitConfirm)
	}
	if !strings.Contains(out, "E_CONFIRMATION_REQUIRED") || !strings.Contains(out, "write-dangerous") {
		t.Fatalf("confirm gate message missing, got:\n%s", out)
	}
	if called {
		t.Fatal("dangerous gate must reject before any API call")
	}
}

// --idempotency-key must be sent as the Idempotency-Key HTTP header on the create
// request, and must be bound into the confirm scope so a token issued for one key
// cannot drive a create under a different key.
func TestIdempotencyKey_HeaderSentAndBound(t *testing.T) {
	isolateConfigHome(t)

	var mu sync.Mutex
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/issues") {
			mu.Lock()
			gotKey = r.Header.Get("Idempotency-Key")
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"iid":7,"title":"Bug"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origExit := lastExit
	defer func() { lastExit = origExit; resetRootPersistentFlags(t) }()

	const key = "idem-abc-123"
	args := []string{"issue", "create", "--project", "foo/bar", "--title", "Bug", "--idempotency-key", key, "--json"}

	// Header sent: full dry-run → confirm round-trip, then assert header.
	resetRootPersistentFlags(t)
	lastExit = 0
	captureStdout(t, func() {
		rootCmd.SetArgs(withConfirmForTest(t, args))
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Fatalf("create exit = %d, want %d", lastExit, ExitOK)
	}
	mu.Lock()
	sent := gotKey
	mu.Unlock()
	if sent != key {
		t.Fatalf("Idempotency-Key header = %q, want %q", sent, key)
	}

	// Bound: a token minted for key A must not validate a create with key B.
	tokenForA, _ := newConfirmToken("create issue", map[string]any{
		"project": "foo/bar", "title": "Bug", "idempotencyKey": "key-A",
	})
	resetRootPersistentFlags(t)
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{
			"issue", "create", "--project", "foo/bar", "--title", "Bug",
			"--idempotency-key", "key-B", "--confirm", tokenForA, "--json",
		})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitConflict {
		t.Fatalf("token bound to a different key should be rejected; exit = %d, want %d\n%s", lastExit, ExitConflict, out)
	}
}

// repo file update/delete bind last_commit_id; if the file moves between dry-run
// and confirm, the confirm token no longer matches and the write is rejected with
// E_CONFLICT rather than clobbering the concurrent edit.
func TestVersionBinding_RepoFile_Mismatch_Conflict(t *testing.T) {
	isolateConfigHome(t)

	var mu sync.Mutex
	commitID := "commit-AAA"
	puts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/repository/files/") && !strings.HasSuffix(r.URL.Path, "/raw") {
			mu.Lock()
			id := commitID
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_name":"README.md","last_commit_id":"` + id + `"}`))
			return
		}
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/repository/files/") {
			mu.Lock()
			puts++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"file_path":"README.md","branch":"main"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origExit := lastExit
	defer func() { lastExit = origExit; resetRootPersistentFlags(t) }()

	args := []string{
		"repo", "file", "update",
		"--project", "foo/bar", "--path", "README.md",
		"--branch", "main", "--content", "updated",
		"--commit-message", "update readme", "--json",
	}
	// Token is minted against last_commit_id=commit-AAA.
	confirmArgs := withConfirmForTest(t, args)

	// Simulate a concurrent commit: the file's last_commit_id moves on.
	mu.Lock()
	commitID = "commit-BBB"
	mu.Unlock()

	resetRootPersistentFlags(t)
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs(confirmArgs)
		_ = rootCmd.Execute()
	})
	if lastExit != ExitConflict {
		t.Fatalf("stale version should be rejected; exit = %d, want %d (E_CONFLICT)\n%s", lastExit, ExitConflict, out)
	}
	mu.Lock()
	n := puts
	mu.Unlock()
	if n != 0 {
		t.Fatalf("no PUT should reach the API on version mismatch, got %d", n)
	}
}
