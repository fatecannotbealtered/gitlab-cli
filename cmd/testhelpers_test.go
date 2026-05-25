package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func init() {
	// Tests run non-interactively; disable agent-safe restrictions unless explicitly testing them.
	_ = os.Setenv("GITLAB_CLI_AGENT_SAFE", "0")
}

// withAgentSafe enables default agent-safe restrictions for the duration of the test.
func withAgentSafe(t *testing.T) {
	t.Helper()
	t.Setenv("GITLAB_CLI_AGENT_SAFE", "1")
	t.Setenv("GITLAB_CLI_ALLOW_FORCE", "")
	t.Setenv("GITLAB_CLI_ALLOW_SHOW_VALUES", "")
}

// resetRootPersistentFlags clears persistent CLI flags that leak between Execute() calls.
func resetRootPersistentFlags(t *testing.T) {
	t.Helper()
	for _, f := range []struct{ name, value string }{
		{"json", "false"},
		{"force", "false"},
		{"dry-run", "false"},
		{"confirm", ""},
		{"quiet", "false"},
		{"compact", "false"},
	} {
		if err := rootCmd.PersistentFlags().Set(f.name, f.value); err != nil {
			t.Fatalf("reset flag %q: %v", f.name, err)
		}
	}
	forceMode = false
	confirmFlag = ""
	dryRun = false
	jsonMode = false
	quietMode = false
	compactJSON = false
}

func resetVariableFlags(t *testing.T) {
	t.Helper()
	resetRootPersistentFlags(t)
	for _, kv := range []struct {
		cmd         *cobra.Command
		name, value string
	}{
		{variableListCmd, "project", ""},
		{variableListCmd, "limit", "20"},
		{variableListCmd, "fields", ""},
		{variableGetCmd, "project", ""},
		{variableGetCmd, "key", ""},
		{variableGetCmd, "filter", ""},
		{variableGetCmd, "fields", ""},
	} {
		if err := kv.cmd.Flags().Set(kv.name, kv.value); err != nil {
			t.Fatalf("reset variable flag %q: %v", kv.name, err)
		}
	}
	if err := variableCmd.PersistentFlags().Set("show-values", "false"); err != nil {
		t.Fatalf("reset show-values: %v", err)
	}
}

func resetAuthLoginFlags(t *testing.T) {
	t.Helper()
	resetRootPersistentFlags(t)
	authLoginHostFlag = ""
	authLoginTokenFlag = ""
	authLoginProfileFlag = "default"
	for _, kv := range []struct{ name, value string }{
		{"host", ""},
		{"token", ""},
		{"profile", "default"},
	} {
		if err := authLoginCmd.Flags().Set(kv.name, kv.value); err != nil {
			t.Fatalf("reset auth login flag %q: %v", kv.name, err)
		}
	}
}

// withStdinInput replaces os.Stdin with a pipe preloaded with input (non-TTY for term.IsTerminal).
func withStdinInput(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if _, err := io.WriteString(w, input); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		_ = r.Close()
	})
}

// writeGitLabCLIFile writes a file under ~/.gitlab-cli/ inside home.
func writeGitLabCLIFile(t *testing.T, home, name, content string) {
	t.Helper()
	dir := filepath.Join(home, ".gitlab-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// blockProfilesRemove makes profiles.json a non-empty directory so os.Remove fails.
func blockProfilesRemove(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".gitlab-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	profDir := filepath.Join(dir, "profiles.json")
	if err := os.Mkdir(profDir, 0o700); err != nil {
		t.Fatalf("mkdir profiles.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profDir, "block"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write block file: %v", err)
	}
}

// blockConfigRemove makes config.json a non-empty directory so os.Remove fails.
func blockConfigRemove(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".gitlab-cli")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfgDir := filepath.Join(dir, "config.json")
	if err := os.Mkdir(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir config.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "block"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write block file: %v", err)
	}
}

// withNonInteractiveStdin replaces os.Stdin with a pipe so requireConfirm treats the session as non-TTY.
func withNonInteractiveStdin(t *testing.T) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		_ = r.Close()
	})
}

// isolateConfigHome points config file resolution away from the developer machine (Windows uses USERPROFILE).
func isolateConfigHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	return dir
}

// captureStdout captures stdout during fn().
// On Windows pipes can deadlock if the buffer fills, so we drain concurrently.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fn()
	_ = w.Close()
	os.Stdout = orig
	<-done
	_ = r.Close()
	return buf.String()
}

// captureCombinedOutput captures stdout and stderr during fn().
func captureCombinedOutput(t *testing.T, fn func()) string {
	t.Helper()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}

	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout = outW
	os.Stderr = errW

	var outBuf, errBuf bytes.Buffer
	outDone := make(chan struct{})
	errDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&outBuf, outR)
		close(outDone)
	}()
	go func() {
		_, _ = io.Copy(&errBuf, errR)
		close(errDone)
	}()

	fn()

	_ = outW.Close()
	_ = errW.Close()
	os.Stdout = origOut
	os.Stderr = origErr
	<-outDone
	<-errDone
	_ = outR.Close()
	_ = errR.Close()
	return outBuf.String() + errBuf.String()
}

// captureStderr captures stderr during fn().
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fn()
	_ = w.Close()
	os.Stderr = orig
	<-done
	_ = r.Close()
	return buf.String()
}
