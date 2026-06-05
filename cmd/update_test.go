package cmd

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
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
	origNow := updateNow
	t.Cleanup(func() {
		version = origVersion
		updateGitHubAPI = origAPI
		updateHTTPClient = origHTTP
		updatePlatform = origPlatform
		updateExecutable = origExecutable
		updateApply = origApply
		updateNow = origNow
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
	for _, want := range []string{`"status": "available"`, `"currentVersion": "1.0.0"`, `"targetVersion": "1.2.3"`, `"updateAvailable": true`} {
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
	if !strings.Contains(out, `"status": "up_to_date"`) || !strings.Contains(out, `"updateAvailable": false`) {
		t.Fatalf("expected up_to_date JSON, got:\n%s", out)
	}
}

func TestUpdate_DryRunJSON_IncludesConfirmToken(t *testing.T) {
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
	for _, want := range []string{`"status": "dry_run"`, `"dryRun": true`, `"confirm": "1.2.3"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestUpdate_InstallWithConfirm(t *testing.T) {
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
		rootCmd.SetArgs([]string{"update", "--confirm", "1.2.3", "--json"})
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
	errOut := captureStderr(t, func() {
		rootCmd.SetArgs([]string{"update", "--confirm", "1.2.3", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitNetwork {
		t.Fatalf("exit=%d want=%d\nstderr:\n%s", lastExit, ExitNetwork, errOut)
	}
	if !strings.Contains(errOut, "checksum mismatch") {
		t.Fatalf("expected checksum error, got:\n%s", errOut)
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
	errOut := captureStderr(t, func() {
		rootCmd.SetArgs([]string{"update", "--check", "--json"})
		_ = rootCmd.Execute()
	})
	if lastExit != ExitBadArgs {
		t.Fatalf("exit=%d want=%d\nstderr:\n%s", lastExit, ExitBadArgs, errOut)
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
	if !strings.Contains(out, `"targetVersion": "1.2.3"`) {
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
	for _, want := range []string{`"status": "downgrade"`, `"downgrade": true`, `"updateAvailable": false`} {
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
		{"1.3.0", "1.2.0", 1},
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
		writeUpdateReleaseJSON(t, w, srv.URL, relVersion, assetName, includeAsset)
	})
	mux.HandleFunc("/repos/fatecannotbealtered/gitlab-cli/releases/tags/v"+relVersion, func(w http.ResponseWriter, r *http.Request) {
		writeUpdateReleaseJSON(t, w, srv.URL, relVersion, assetName, includeAsset)
	})
	mux.HandleFunc("/downloads/"+assetName, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archiveBytes)
	})
	mux.HandleFunc("/downloads/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(checksum))
	})
	srv = httptest.NewServer(mux)
	updateGitHubAPI = srv.URL
	updateHTTPClient = srv.Client()
	return srv
}

func writeUpdateReleaseJSON(t *testing.T, w http.ResponseWriter, baseURL, relVersion, assetName string, includeAsset bool) {
	t.Helper()
	assets := []updateReleaseAsset{
		{Name: "checksums.txt", BrowserDownloadURL: baseURL + "/downloads/checksums.txt"},
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
