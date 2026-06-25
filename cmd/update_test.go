package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
)

func resetUpdateTestState(t *testing.T) {
	t.Helper()
	resetRootPersistentFlags(t)
	isolateConfigHome(t)
	t.Setenv("GITLAB_NO_AUDIT", "1")
	origVersion := version
	origAPI := updateGitHubAPI
	origHTTP := updateHTTPClient
	origPlatform := updatePlatform
	origExecutable := updateExecutable
	origApply := updateApply
	origSkillSync := updateSkillSync
	origNow := updateNow
	origVerifySig := updateVerifySignature
	origRunPM := updateRunPackageManager
	t.Cleanup(func() {
		version = origVersion
		updateGitHubAPI = origAPI
		updateHTTPClient = origHTTP
		updatePlatform = origPlatform
		updateExecutable = origExecutable
		updateApply = origApply
		updateSkillSync = origSkillSync
		updateNow = origNow
		updateVerifySignature = origVerifySig
		updateRunPackageManager = origRunPM
	})
	for _, kv := range []struct{ name, value string }{
		{"check", "false"},
		{"target-version", ""},
		{"reinstall", "false"},
	} {
		if err := updateCmd.Flags().Set(kv.name, kv.value); err != nil {
			t.Fatalf("reset update flag %q: %v", kv.name, err)
		}
	}
	updatePlatform = func() (string, string) { return "linux", "amd64" }
	updateExecutable = func() (string, error) {
		return filepath.Join(t.TempDir(), "gitlab-cli"), nil
	}
	updateNow = func() time.Time { return time.Unix(1700000000, 0) }
	updateSkillSync = func(context.Context, string) error { return nil }
	// Default: in-process Sigstore verification succeeds. Tests that exercise
	// the fail-closed path override this with an error-returning stub. A live
	// OIDC-signed bundle cannot be produced in a unit test, so the seam stands
	// in for the real verifier while the surrounding control flow is tested.
	updateVerifySignature = func(context.Context, string, string, string) error { return nil }
	// Package-manager-driven update is stubbed to a no-op success by default so
	// tests never shell out to a real npm/go. Tests asserting failure or
	// call-capture override this with their own closure.
	updateRunPackageManager = func(context.Context, string, string) error { return nil }
}

// updateInstallArgsForTest returns the args for a bare single-command update.
// The new contract takes NO confirm token: a bare `update` executes directly.
func updateInstallArgsForTest(_ *testing.T, _, _, _ string, reinstall bool) []string {
	args := []string{"update", "--json"}
	if reinstall {
		args = append(args, "--reinstall")
	}
	return args
}

func TestUpdate_Help(t *testing.T) {
	resetUpdateTestState(t)
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"update", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(nil)
	if f := updateCmd.Flags().Lookup("help"); f != nil {
		_ = f.Value.Set("false")
	}
	if !strings.Contains(buf.String(), "update") || !strings.Contains(buf.String(), "--check") {
		t.Fatalf("update help missing expected text:\n%s", buf.String())
	}
}

func TestUpdate_CheckJSON_Available(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "", true)
	defer srv.Close()

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"update", "--check", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"status": "available"`, `"current_version": "1.0.0"`, `"target_version": "1.2.3"`, `"update_available": true`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if lastExit != ExitOK {
		t.Fatalf("exit=%d want=0", lastExit)
	}
}

func TestUpdate_CheckJSON_UpToDate(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.2.3"
	srv := newUpdateTestServer(t, "1.2.3", []byte("same-binary"), "", true)
	defer srv.Close()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"update", "--check", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"status": "up_to_date"`) || !strings.Contains(out, `"update_available": false`) {
		t.Fatalf("expected up_to_date JSON, got:\n%s", out)
	}
}

// TestUpdate_DryRunJSON_NoConfirmToken asserts --dry-run is a read-only preview
// that issues NO confirm_token and NO expires_at — update is not a write gate.
func TestUpdate_DryRunJSON_NoConfirmToken(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "", true)
	defer srv.Close()
	updateApply = func(_, _ string) (updateApplyResult, error) {
		t.Fatal("dry-run must not apply update")
		return updateApplyResult{}, nil
	}

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"update", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"status": "dry_run"`) {
		t.Fatalf("missing dry_run status in:\n%s", out)
	}
	for _, forbidden := range []string{"confirm_token", "expires_at"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("dry-run must not emit %q in:\n%s", forbidden, out)
		}
	}
}

// TestUpdate_BareExecutesWithoutToken asserts a bare `update` performs the whole
// update in one call with no confirm token (the single-command contract).
func TestUpdate_BareExecutesWithoutToken(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "", true)
	defer srv.Close()
	applied := false
	updateApply = func(src, dst string) (updateApplyResult, error) {
		applied = true
		return updateApplyResult{Status: "installed", Path: dst}, nil
	}

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"update", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Fatalf("exit=%d want=0\n%s", lastExit, out)
	}
	if !applied {
		t.Fatal("bare update did not apply the install")
	}
	for _, want := range []string{`"status": "installed"`, `"binary_replaced": true`, `"skill_sync_status": "synced"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

// TestUpdate_Idempotent_NoOp asserts already-latest returns ok with a no-op.
func TestUpdate_Idempotent_NoOp(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.2.3"
	srv := newUpdateTestServer(t, "1.2.3", []byte("same"), "", true)
	defer srv.Close()
	updateApply = func(_, _ string) (updateApplyResult, error) {
		t.Fatal("up-to-date update must not apply")
		return updateApplyResult{}, nil
	}

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"update", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Fatalf("exit=%d want=0\n%s", lastExit, out)
	}
	if !strings.Contains(out, `"status": "up_to_date"`) || !strings.Contains(out, `"update_available": false`) {
		t.Fatalf("expected idempotent no-op, got:\n%s", out)
	}
}

// TestUpdate_SkillSyncFailure_PartialSuccess asserts that a skill_sync failure
// after a successful binary replace is partial success (ok:false,
// binary_replaced:true) carrying skill_sync_command and retryable:true.
func TestUpdate_SkillSyncFailure_PartialSuccess(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "", true)
	defer srv.Close()
	updateApply = func(_, dst string) (updateApplyResult, error) {
		return updateApplyResult{Status: "installed", Path: dst}, nil
	}
	updateSkillSync = func(context.Context, string) error {
		return fmt.Errorf("npx not found")
	}

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"update", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitNetwork {
		t.Fatalf("exit=%d want=%d\n%s", lastExit, ExitNetwork, out)
	}
	for _, want := range []string{`"ok": false`, `"binary_replaced": true`, `"skill_sync_status": "failed"`, `"skill_sync_command"`, `"retryable": true`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in partial success:\n%s", want, out)
		}
	}
}

func TestUpdate_Install(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "", true)
	defer srv.Close()
	updateApply = func(src, dst string) (updateApplyResult, error) {
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("read extracted binary: %v", err)
		}
		if string(data) != "new-binary" {
			t.Fatalf("extracted binary = %q", data)
		}
		return updateApplyResult{Status: "installed", Path: dst}, nil
	}

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs(updateInstallArgsForTest(t, srv.URL, "1.0.0", "1.2.3", false))
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Fatalf("exit=%d want=0", lastExit)
	}
	if !strings.Contains(out, `"status": "installed"`) {
		t.Fatalf("expected installed JSON, got:\n%s", out)
	}
}

func TestUpdate_ChecksumMismatch(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "0000", true)
	defer srv.Close()
	updateApply = func(_, _ string) (updateApplyResult, error) {
		t.Fatal("checksum mismatch must not apply update")
		return updateApplyResult{}, nil
	}

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	errOut := captureCombinedOutput(t, func() {
		rootCmd.SetArgs(updateInstallArgsForTest(t, srv.URL, "1.0.0", "1.2.3", false))
		_ = rootCmd.Execute()
	})
	if lastExit != ExitError {
		t.Fatalf("exit=%d want=%d\nstdout:\n%s", lastExit, ExitError, errOut)
	}
	for _, want := range []string{"checksum mismatch", "E_INTEGRITY", `"retryable": false`, `"stage": "verify_checksum"`, `"binary_replaced": false`} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("missing %q in integrity error:\n%s", want, errOut)
		}
	}
}

func TestUpdate_MissingSignatureBundle_Refused(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	// Release without checksums.txt.sigstore.json: must be refused, not skipped.
	srv := newUpdateTestServerWithBundle(t, "1.2.3", []byte("new-binary"), "", true, false)
	defer srv.Close()
	updateApply = func(_, _ string) (updateApplyResult, error) {
		t.Fatal("unsigned release must not be installed")
		return updateApplyResult{}, nil
	}

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	errOut := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"update", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitError {
		t.Fatalf("exit=%d want=%d\nstdout:\n%s", lastExit, ExitError, errOut)
	}
	for _, want := range []string{"unsigned release", "E_INTEGRITY", `"stage": "verify_signature"`} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("missing %q in unsigned-release refusal:\n%s", want, errOut)
		}
	}
}

func TestUpdate_SignatureVerificationFails_Refused(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "", true)
	defer srv.Close()
	// In-process verification rejects the bundle (wrong identity / tampered).
	updateVerifySignature = func(context.Context, string, string, string) error {
		return fmt.Errorf("signature verification failed: certificate identity mismatch")
	}
	updateApply = func(_, _ string) (updateApplyResult, error) {
		t.Fatal("release with failing signature must not be installed")
		return updateApplyResult{}, nil
	}

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	errOut := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"update", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitError {
		t.Fatalf("exit=%d want=%d\nstdout:\n%s", lastExit, ExitError, errOut)
	}
	for _, want := range []string{"signature verification failed", "E_INTEGRITY", `"retryable": false`, `"stage": "verify_signature"`} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("missing %q in signature failure refusal:\n%s", want, errOut)
		}
	}
}

func TestUpdate_MissingAsset(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "", false)
	defer srv.Close()

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	errOut := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"update", "--check", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitBadArgs {
		t.Fatalf("exit=%d want=%d\nstdout:\n%s", lastExit, ExitBadArgs, errOut)
	}
	if !strings.Contains(errOut, "does not include asset") {
		t.Fatalf("expected missing asset error, got:\n%s", errOut)
	}
}

// TestUpdate_Discover404_NotFound asserts a 404 on the release lookup is
// classified as the non-retryable E_NOT_FOUND (exit 3) — not collapsed into a
// retryable E_NETWORK — and carries the discover-stage invariant.
func TestUpdate_Discover404_NotFound(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}))
	defer srv.Close()
	updateGitHubAPI = srv.URL
	updateHTTPClient = srv.Client()

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	errOut := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"update", "--target-version", "9.9.9", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitNotFound {
		t.Fatalf("exit=%d want=%d\n%s", lastExit, ExitNotFound, errOut)
	}
	for _, want := range []string{"E_NOT_FOUND", `"retryable": false`, `"stage": "discover"`, `"binary_replaced": false`} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("missing %q in 404 discover error:\n%s", want, errOut)
		}
	}
}

// TestUpdate_Discover429_RateLimited asserts a 429 maps to the retryable
// E_RATE_LIMITED taxonomy (exit 7), distinct from a plain network failure.
func TestUpdate_Discover429_RateLimited(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"rate limited"}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()
	updateGitHubAPI = srv.URL
	updateHTTPClient = srv.Client()

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	errOut := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"update", "--check", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitRateLimit {
		t.Fatalf("exit=%d want=%d\n%s", lastExit, ExitRateLimit, errOut)
	}
	for _, want := range []string{"E_RATE_LIMITED", `"retryable": true`, `"stage": "discover"`} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("missing %q in 429 discover error:\n%s", want, errOut)
		}
	}
}

// TestUpdate_Discover5xx_Server asserts a 500 maps to the retryable E_SERVER
// taxonomy (exit 7) rather than being collapsed into E_NETWORK by message text.
func TestUpdate_Discover5xx_Server(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"boom"}`, http.StatusBadGateway)
	}))
	defer srv.Close()
	updateGitHubAPI = srv.URL
	updateHTTPClient = srv.Client()

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	errOut := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"update", "--check", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitNetwork {
		t.Fatalf("exit=%d want=%d\n%s", lastExit, ExitNetwork, errOut)
	}
	for _, want := range []string{"E_SERVER", `"retryable": true`, `"stage": "discover"`} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("missing %q in 5xx discover error:\n%s", want, errOut)
		}
	}
}

// TestUpdate_SignatureBundleDownloadFails_NetworkNotIntegrity asserts that a
// transport failure fetching the Sigstore bundle is a retryable network failure,
// NOT a non-retryable E_INTEGRITY. Integrity is reserved for an actually
// missing/invalid signature, not a download blip.
func TestUpdate_SignatureBundleDownloadFails_NetworkNotIntegrity(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServerBundle404(t, "1.2.3", []byte("new-binary"))
	defer srv.Close()
	updateApply = func(_, _ string) (updateApplyResult, error) {
		t.Fatal("bundle download failure must not install")
		return updateApplyResult{}, nil
	}

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	errOut := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"update", "--json"})
		_ = rootCmd.Execute()
	})
	// 404 on the bundle asset maps through the status taxonomy to E_NOT_FOUND.
	if lastExit != ExitNotFound {
		t.Fatalf("exit=%d want=%d\n%s", lastExit, ExitNotFound, errOut)
	}
	for _, want := range []string{`"stage": "verify_signature"`, `"binary_replaced": false`} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("missing %q in bundle-download failure:\n%s", want, errOut)
		}
	}
	if strings.Contains(errOut, "E_INTEGRITY") {
		t.Fatalf("bundle download failure must NOT be E_INTEGRITY:\n%s", errOut)
	}
}

// TestUpdateTrustedRoot_RefreshFailureIsNetwork drives the REAL TUF trust-root
// refresh path (the production updateTrustedRoot, not the stubbed verify seam)
// with an injected fetcher that fails, and asserts every refresh failure is
// wrapped in errTrustRootUnavailable — so the caller classifies it as retryable
// network, never the non-retryable E_INTEGRITY. A slow/down/DNS-failing Sigstore
// TUF registry is a network blip, not a forged-release verdict.
func TestUpdateTrustedRoot_RefreshFailureIsNetwork(t *testing.T) {
	resetUpdateTestState(t)
	origFetch := updateFetchTrustedRoot
	t.Cleanup(func() { updateFetchTrustedRoot = origFetch })

	cases := []struct {
		name      string
		injectErr error
		wantIs    error
	}{
		{name: "deadline_exceeded", injectErr: context.DeadlineExceeded, wantIs: context.DeadlineExceeded},
		{name: "transport", injectErr: fmt.Errorf("dial tcp: lookup tuf-repo-cdn.sigstore.dev: no such host"), wantIs: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			updateFetchTrustedRoot = func(*tuf.Options) (*root.TrustedRoot, error) {
				return nil, tc.injectErr
			}
			_, err := updateTrustedRoot(context.Background())
			if err == nil {
				t.Fatalf("expected trust-root failure, got nil")
			}
			if !errors.Is(err, errTrustRootUnavailable) {
				t.Fatalf("err must be errTrustRootUnavailable (retryable network), got %v", err)
			}
			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Fatalf("err must wrap %v, got %v", tc.wantIs, err)
			}
		})
	}
}

// TestUpdate_TrustRootRefreshFails_NetworkNotIntegrity drives the full update
// command through the REAL verify-signature classification glue with a trust-root
// refresh failure produced by the production updateTrustedRoot (injected failing
// fetcher), and asserts the failure is routed to the retryable network/timeout
// taxonomy — NOT the non-retryable E_INTEGRITY. This is the path the previous
// round left half-fixed: when only the child 30s timeout fires (parent ctx still
// alive), the wrapped DeadlineExceeded must not collapse into E_INTEGRITY.
//
// Unlike the masked stub of the last round (which returned nil and never drove
// the real classification), this test's verify seam returns the EXACT error the
// production updateTrustedRoot emits on a refresh failure, so the routing logic
// in verifyUpdateChecksumSignature -> runUpdateInstall is genuinely exercised.
func TestUpdate_TrustRootRefreshFails_NetworkNotIntegrity(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "", true)
	defer srv.Close()

	origFetch := updateFetchTrustedRoot
	t.Cleanup(func() { updateFetchTrustedRoot = origFetch })
	// Sigstore TUF registry is slow/down: the bounded refresh times out while the
	// parent ctx stays alive (only the child 30s timeout fires).
	updateFetchTrustedRoot = func(*tuf.Options) (*root.TrustedRoot, error) {
		return nil, context.DeadlineExceeded
	}
	// Drive the real classification: the verify seam invokes the production
	// updateTrustedRoot, whose failure must propagate as errTrustRootUnavailable.
	updateVerifySignature = func(ctx context.Context, _, _, _ string) error {
		_, err := updateTrustedRoot(ctx)
		return err
	}
	updateApply = func(_, _ string) (updateApplyResult, error) {
		t.Fatal("trust-root refresh failure must not install")
		return updateApplyResult{}, nil
	}

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	errOut := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"update", "--json"})
		_ = rootCmd.Execute()
	})
	// A trust-root refresh timeout is a retryable network condition. The wrapped
	// DeadlineExceeded classifies to E_TIMEOUT (exit 8); a transport error would
	// be E_NETWORK (exit 7). Both are retryable — the point is it is NOT exit 1.
	if lastExit != ExitTimeout {
		t.Fatalf("exit=%d want=%d (retryable timeout)\n%s", lastExit, ExitTimeout, errOut)
	}
	for _, want := range []string{`"stage": "verify_signature"`, `"binary_replaced": false`, `"retryable": true`, "E_TIMEOUT"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("missing %q in trust-root refresh failure:\n%s", want, errOut)
		}
	}
	if strings.Contains(errOut, "E_INTEGRITY") {
		t.Fatalf("trust-root refresh failure must NOT be E_INTEGRITY:\n%s", errOut)
	}
}

// TestUpdate_TrustRootTransportFails_NetworkNotIntegrity is the transport-error
// sibling of the timeout case: a DNS/connection-refused refresh failure must be
// retryable E_NETWORK (exit 7), still not E_INTEGRITY.
func TestUpdate_TrustRootTransportFails_NetworkNotIntegrity(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "", true)
	defer srv.Close()

	origFetch := updateFetchTrustedRoot
	t.Cleanup(func() { updateFetchTrustedRoot = origFetch })
	updateFetchTrustedRoot = func(*tuf.Options) (*root.TrustedRoot, error) {
		return nil, fmt.Errorf("dial tcp: connection refused")
	}
	updateVerifySignature = func(ctx context.Context, _, _, _ string) error {
		_, err := updateTrustedRoot(ctx)
		return err
	}
	updateApply = func(_, _ string) (updateApplyResult, error) {
		t.Fatal("trust-root refresh failure must not install")
		return updateApplyResult{}, nil
	}

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	errOut := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"update", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitNetwork {
		t.Fatalf("exit=%d want=%d (retryable network)\n%s", lastExit, ExitNetwork, errOut)
	}
	for _, want := range []string{`"stage": "verify_signature"`, `"retryable": true`, "E_NETWORK"} {
		if !strings.Contains(errOut, want) {
			t.Fatalf("missing %q in trust-root transport failure:\n%s", want, errOut)
		}
	}
	if strings.Contains(errOut, "E_INTEGRITY") {
		t.Fatalf("trust-root transport failure must NOT be E_INTEGRITY:\n%s", errOut)
	}
}

// TestDetectInstallMethod probes the install-method classifier against the
// well-known executable layouts and the explicit override env.
func TestDetectInstallMethod(t *testing.T) {
	resetUpdateTestState(t)
	tests := []struct {
		name string
		env  string
		exe  string
		want string
	}{
		{name: "override", env: "Homebrew", exe: "/anything/gitlab-cli", want: "homebrew"},
		{name: "npm", exe: "/usr/lib/node_modules/@scope/gitlab-cli/bin/gitlab-cli", want: "npm"},
		{name: "homebrew", exe: "/opt/homebrew/Cellar/gitlab-cli/1.2.3/bin/gitlab-cli", want: "homebrew"},
		{name: "go", exe: "/home/u/go/bin/gitlab-cli", want: "go"},
		{name: "binary", exe: "/usr/local/bin/gitlab-cli", want: "binary"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			updateGetenv = func(string) string { return tc.env }
			updateExecutable = func() (string, error) { return tc.exe, nil }
			if got := detectInstallMethod(); got != tc.want {
				t.Fatalf("detectInstallMethod(%q, env=%q)=%q want %q", tc.exe, tc.env, got, tc.want)
			}
		})
	}
}

// TestDetectInstallMethod_ExecutableError falls back to "binary" rather than an
// empty string when the executable path cannot be resolved.
func TestDetectInstallMethod_ExecutableError(t *testing.T) {
	resetUpdateTestState(t)
	updateGetenv = func(string) string { return "" }
	updateExecutable = func() (string, error) { return "", fmt.Errorf("nope") }
	if got := detectInstallMethod(); got != "binary" {
		t.Fatalf("detectInstallMethod with exe error = %q want binary", got)
	}
}

// TestUpdate_Install_ClearsNoticeCache asserts a successful install removes the
// stale "update_available" notice cache so business commands stop nagging to
// update to the version that was just installed.
func TestUpdate_Install_ClearsNoticeCache(t *testing.T) {
	resetUpdateTestState(t)
	updateNoticeTestForceEnabled = true
	t.Cleanup(func() { updateNoticeTestForceEnabled = false })
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "", true)
	defer srv.Close()
	updateApply = func(_, dst string) (updateApplyResult, error) {
		return updateApplyResult{Status: "installed", Path: dst}, nil
	}

	// Seed a stale notice cache as if a prior check had found 1.2.3 available.
	writeUpdateNoticeCache([]updateNotice{{
		Type:            "update_available",
		UpdateAvailable: true,
		CurrentVersion:  "1.0.0",
		LatestVersion:   "1.2.3",
		CheckedAt:       time.Now().UTC().Format(time.RFC3339),
	}})
	path, err := updateNoticeCachePath()
	if err != nil {
		t.Fatalf("cache path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("seed cache not written: %v", err)
	}

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	_ = captureStdout(t, func() {
		rootCmd.SetArgs([]string{"update", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Fatalf("exit=%d want=0", lastExit)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("notice cache should be cleared after install, stat err = %v", err)
	}
}

// newUpdateTestServerBundle404 serves a valid release whose signature bundle URL
// returns 404, exercising the verify-stage network-vs-integrity split.
func newUpdateTestServerBundle404(t *testing.T, relVersion string, binary []byte) *httptest.Server {
	t.Helper()
	assetName, err := updateArchiveName(relVersion)
	if err != nil {
		t.Fatalf("archive name: %v", err)
	}
	archiveBytes := makeUpdateTarGz(t, updateArchiveBinaryName(), binary)
	sum := sha256.Sum256(archiveBytes)
	checksum := fmt.Sprintf("%x  %s\n", sum[:], assetName)

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/fatecannotbealtered/gitlab-cli/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		writeUpdateReleaseJSON(t, w, srv.URL, relVersion, assetName, true, true)
	})
	mux.HandleFunc("/downloads/"+assetName, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archiveBytes)
	})
	mux.HandleFunc("/downloads/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(checksum))
	})
	mux.HandleFunc("/downloads/checksums.txt.sigstore.json", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	srv = httptest.NewServer(mux)
	updateGitHubAPI = srv.URL
	updateHTTPClient = srv.Client()
	return srv
}

func TestUpdate_TargetVersionUsesReleaseTagURL(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "", true)
	defer srv.Close()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"update", "--target-version", "1.2.3", "--check", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"target_version": "1.2.3"`) {
		t.Fatalf("expected target version JSON, got:\n%s", out)
	}
}

func TestUpdate_CheckJSON_DowngradeTarget(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.2.3"
	srv := newUpdateTestServer(t, "1.0.0", []byte("old-binary"), "", true)
	defer srv.Close()

	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"update", "--target-version", "1.0.0", "--check", "--json"})
		_ = rootCmd.Execute()
	})
	for _, want := range []string{`"status": "downgrade"`, `"downgrade": true`, `"update_available": false`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestUpdateArchiveName_WindowsARM64FallsBackToAMD64(t *testing.T) {
	resetUpdateTestState(t)
	updatePlatform = func() (string, string) { return "windows", "arm64" }
	got, err := updateArchiveName("1.2.3")
	if err != nil {
		t.Fatalf("updateArchiveName: %v", err)
	}
	want := "gitlab-cli-1.2.3-windows-amd64.zip"
	if got != want {
		t.Fatalf("archive = %q want %q", got, want)
	}
}

func TestExtractUpdateZip(t *testing.T) {
	resetUpdateTestState(t)
	updatePlatform = func() (string, string) { return "windows", "amd64" }
	tmpDir := t.TempDir()
	assetName, err := updateArchiveName("1.2.3")
	if err != nil {
		t.Fatalf("archive name: %v", err)
	}
	archivePath := filepath.Join(tmpDir, assetName)
	if err := os.WriteFile(archivePath, makeUpdateZip(t, updateArchiveBinaryName(), []byte("zip-binary")), 0o600); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	binPath, err := extractUpdateArchive(archivePath, assetName, tmpDir)
	if err != nil {
		t.Fatalf("extract zip: %v", err)
	}
	data, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("read extracted zip binary: %v", err)
	}
	if string(data) != "zip-binary" {
		t.Fatalf("zip binary = %q", data)
	}
}

// TestApplyUpdateBinary_InPlaceRename asserts the cross-platform rename trick
// replaces the target in place and reports installed (no scheduled/.cmd defer),
// leaving no .new/.old residue behind on success.
func TestApplyUpdateBinary_InPlaceRename(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "gitlab-cli")
	if err := os.WriteFile(dst, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("write dst: %v", err)
	}
	src := filepath.Join(dir, "downloaded")
	if err := os.WriteFile(src, []byte("new-binary"), 0o755); err != nil {
		t.Fatalf("write src: %v", err)
	}

	got, err := applyUpdateBinary(src, dst)
	if err != nil {
		t.Fatalf("applyUpdateBinary: %v", err)
	}
	if got.Status != "installed" {
		t.Fatalf("status = %q want installed", got.Status)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(data) != "new-binary" {
		t.Fatalf("dst content = %q want new-binary", data)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitlab-cli.new")); !os.IsNotExist(err) {
		t.Fatalf(".new residue should be gone, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitlab-cli.old")); !os.IsNotExist(err) {
		t.Fatalf(".old residue should be gone, stat err = %v", err)
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		current string
		target  string
		want    int
	}{
		{"1.0.0", "1.2.0", -1},
		{"1.2.0", "1.2.0", 0},
		{"2.0.0", "1.2.0", 1},
		{"1.2.0-rc.1", "1.2.0", -1},
		{"1.2.0-rc.10", "1.2.0-rc.2", 1},
		{"1.2.0-alpha.1", "1.2.0-alpha.beta", -1},
		{"dev", "1.2.0", -1},
	}
	for _, tt := range tests {
		got := compareVersions(tt.current, tt.target)
		switch {
		case tt.want < 0 && got >= 0:
			t.Fatalf("compareVersions(%q,%q)=%d want negative", tt.current, tt.target, got)
		case tt.want == 0 && got != 0:
			t.Fatalf("compareVersions(%q,%q)=%d want zero", tt.current, tt.target, got)
		case tt.want > 0 && got <= 0:
			t.Fatalf("compareVersions(%q,%q)=%d want positive", tt.current, tt.target, got)
		}
	}
}

func newUpdateTestServer(t *testing.T, relVersion string, binary []byte, checksumOverride string, includeAsset bool) *httptest.Server {
	t.Helper()
	return newUpdateTestServerWithBundle(t, relVersion, binary, checksumOverride, includeAsset, true)
}

func newUpdateTestServerWithBundle(t *testing.T, relVersion string, binary []byte, checksumOverride string, includeAsset, includeBundle bool) *httptest.Server {
	t.Helper()
	assetName, err := updateArchiveName(relVersion)
	if err != nil {
		t.Fatalf("archive name: %v", err)
	}
	archiveBytes := makeUpdateTarGz(t, updateArchiveBinaryName(), binary)
	sum := sha256.Sum256(archiveBytes)
	checksum := fmt.Sprintf("%x  %s\n", sum[:], assetName)
	if checksumOverride != "" {
		checksum = checksumOverride + "  " + assetName + "\n"
	}

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/fatecannotbealtered/gitlab-cli/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		writeUpdateReleaseJSON(t, w, srv.URL, relVersion, assetName, includeAsset, includeBundle)
	})
	mux.HandleFunc("/repos/fatecannotbealtered/gitlab-cli/releases/tags/v"+relVersion, func(w http.ResponseWriter, r *http.Request) {
		writeUpdateReleaseJSON(t, w, srv.URL, relVersion, assetName, includeAsset, includeBundle)
	})
	mux.HandleFunc("/downloads/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archiveBytes)
	})
	mux.HandleFunc("/downloads/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(checksum))
	})
	mux.HandleFunc("/downloads/checksums.txt.sigstore.json", func(w http.ResponseWriter, r *http.Request) {
		// Opaque bundle bytes; in-process verification is stubbed in tests.
		_, _ = w.Write([]byte(`{"bundle":"stub"}`))
	})
	srv = httptest.NewServer(mux)
	updateGitHubAPI = srv.URL
	updateHTTPClient = srv.Client()
	return srv
}

func writeUpdateReleaseJSON(t *testing.T, w http.ResponseWriter, baseURL, relVersion, assetName string, includeAsset, includeBundle bool) {
	t.Helper()
	assets := []updateReleaseAsset{
		{Name: "checksums.txt", BrowserDownloadURL: baseURL + "/downloads/checksums.txt"},
	}
	if includeBundle {
		assets = append(assets, updateReleaseAsset{Name: "checksums.txt.sigstore.json", BrowserDownloadURL: baseURL + "/downloads/checksums.txt.sigstore.json"})
	}
	if includeAsset {
		assets = append(assets, updateReleaseAsset{Name: assetName, BrowserDownloadURL: baseURL + "/downloads/" + assetName})
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"tag_name":"v%s","html_url":"%s/releases/v%s","assets":`, relVersion, baseURL, relVersion)
	data := mustJSONAssets(t, assets)
	_, _ = w.Write(data)
	_, _ = w.Write([]byte(`}`))
}

func mustJSONAssets(t *testing.T, assets []updateReleaseAsset) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, asset := range assets {
		if i > 0 {
			buf.WriteByte(',')
		}
		fmt.Fprintf(&buf, `{"name":%q,"browser_download_url":%q}`, asset.Name, asset.BrowserDownloadURL)
	}
	buf.WriteByte(']')
	return buf.Bytes()
}

func makeUpdateTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o700, Size: int64(len(content))}); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tar write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func makeUpdateZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(name)
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// A bare `update` on an npm install now DRIVES npm: it runs
// `npm install -g @pkg@<ver>` on the user's behalf (not just print it), then
// syncs the Skill, and reports status "installed". signature_status stays
// "not_checked" (npm provenance owns integrity on this path).
func TestUpdate_NPMInstallDrivesPackageManager(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "", true)
	defer srv.Close()

	// Simulate an npm-managed executable path.
	updateExecutable = func() (string, error) {
		return "/usr/lib/node_modules/@fateforge/gitlab-cli/bin/gitlab-cli", nil
	}

	var gotMethod, gotVersion string
	var skillSynced bool
	updateRunPackageManager = func(_ context.Context, method, target string) error {
		gotMethod, gotVersion = method, target
		return nil
	}
	updateSkillSync = func(context.Context, string) error { skillSynced = true; return nil }

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"update", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Fatalf("exit=%d want=0\n%s", lastExit, out)
	}
	if gotMethod != "npm" {
		t.Fatalf("expected npm to be driven, got method %q", gotMethod)
	}
	if normalizeVersion(gotVersion) != "1.2.3" {
		t.Fatalf("expected target 1.2.3 passed to npm, got %q", gotVersion)
	}
	if !skillSynced {
		t.Fatalf("expected Skill sync to run after npm install")
	}
	for _, want := range []string{`"status": "installed"`, `"skill_sync_status": "synced"`, `"signature_status": "not_checked"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestUpdate_GoInstallDrivesPackageManager(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "", true)
	defer srv.Close()

	// Simulate a go-managed executable path via GOBIN.
	t.Setenv("GOBIN", "/home/user/go/bin")
	updateExecutable = func() (string, error) {
		return "/home/user/go/bin/gitlab-cli", nil
	}
	updateGetenv = func(k string) string {
		if k == "GOBIN" {
			return "/home/user/go/bin"
		}
		return ""
	}

	var gotMethod string
	updateRunPackageManager = func(_ context.Context, method, _ string) error {
		gotMethod = method
		return nil
	}

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"update", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Fatalf("exit=%d want=0\n%s", lastExit, out)
	}
	if gotMethod != "go" {
		t.Fatalf("expected go to be driven, got method %q", gotMethod)
	}
	if !strings.Contains(out, `"install_method": "go"`) {
		t.Fatalf("expected go install method: %s", out)
	}
}

// --dry-run on a package-manager install is a read-only preview: it must NOT
// invoke the package manager, and must report the command it WOULD run.
func TestUpdate_PackageManagerDryRunDoesNotExecute(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "", true)
	defer srv.Close()

	updateExecutable = func() (string, error) {
		return "/usr/lib/node_modules/@fateforge/gitlab-cli/bin/gitlab-cli", nil
	}

	called := false
	updateRunPackageManager = func(context.Context, string, string) error { called = true; return nil }

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"update", "--dry-run", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitOK {
		t.Fatalf("exit=%d want=0\n%s", lastExit, out)
	}
	if called {
		t.Fatalf("dry-run must not invoke the package manager")
	}
	if !strings.Contains(out, `"dry_run": true`) {
		t.Fatalf("expected dry_run preview: %s", out)
	}
	if !strings.Contains(out, "npm install -g @fateforge/gitlab-cli@1.2.3") {
		t.Fatalf("expected planned npm command: %s", out)
	}
}

// When the package manager fails, the installed binary is unchanged: report
// E_IO (exit 1), binary_replaced:false, and surface the command to run manually.
func TestUpdate_PackageManagerFailureReportsUnchanged(t *testing.T) {
	resetUpdateTestState(t)
	version = "1.0.0"
	srv := newUpdateTestServer(t, "1.2.3", []byte("new-binary"), "", true)
	defer srv.Close()

	updateExecutable = func() (string, error) {
		return "/usr/lib/node_modules/@fateforge/gitlab-cli/bin/gitlab-cli", nil
	}
	updateRunPackageManager = func(context.Context, string, string) error {
		return errors.New("ETARGET no matching version")
	}

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	out := captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"update", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitIO {
		t.Fatalf("expected exit %d, got %d: %s", ExitIO, lastExit, out)
	}
	for _, want := range []string{`"code": "E_IO"`, `"binary_replaced": false`, "npm install -g @fateforge/gitlab-cli@1.2.3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
