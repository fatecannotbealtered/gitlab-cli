package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func init() {
	// Tests run non-interactively; disable agent-safe restrictions unless explicitly testing them.
	_ = os.Setenv("GITLAB_CLI_AGENT_SAFE", "0")
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
