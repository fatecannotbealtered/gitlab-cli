package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	updateVerifySignature = func(_, _, _ string) error { return nil }
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
	updateVerifySignature = func(_, _, _ string) error {
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
