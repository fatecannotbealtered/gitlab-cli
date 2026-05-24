//go:build integration

package e2e

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const cliBuildTimeout = 3 * time.Minute

var (
	cliOnce sync.Once
	cliBin  string
	cliErr  error
)

// BuildCLI compiles gitlab-cli once for integration tests.
func BuildCLI(t *testing.T) string {
	t.Helper()
	cliOnce.Do(func() {
		dir := filepath.Join(os.TempDir(), "gitlab-cli-integration")
		_ = os.MkdirAll(dir, 0o755)
		out := filepath.Join(dir, "gitlab-cli-e2e")
		if runtime.GOOS == "windows" {
			out += ".exe"
		}
		ctx, cancel := context.WithTimeout(context.Background(), cliBuildTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "build", "-o", out, "./cmd/gitlab-cli")
		cmd.Dir = RepoRoot()
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			cliErr = err
			return
		}
		cliBin = out
	})
	if cliErr != nil {
		t.Fatalf("go build ./cmd/gitlab-cli: %v", cliErr)
	}
	return cliBin
}

// RunOptions overrides process environment and working directory for a CLI invocation.
type RunOptions struct {
	WorkDir string
	ExtraEnv map[string]string
}

// RunCLI runs the built CLI with GITLAB_CLI_* from the environment.
func RunCLI(t *testing.T, args []string, opts ...RunOptions) (stdout, stderr string, exitCode int) {
	t.Helper()
	var opt RunOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	bin := BuildCLI(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	if opt.WorkDir != "" {
		cmd.Dir = opt.WorkDir
	} else {
		cmd.Dir = RepoRoot()
	}
	env := os.Environ()
	for k, v := range opt.ExtraEnv {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("gitlab-cli %v: %v\nstderr:\n%s", args, err, errBuf.String())
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// RequireEnv skips the test when GITLAB_CLI_HOST/TOKEN are missing.
func RequireEnv(t *testing.T) {
	t.Helper()
	LoadLocalEnv()
	if !Configured() {
		t.Skip("skipping integration test: set GITLAB_CLI_HOST and GITLAB_CLI_TOKEN, or run .\\scripts\\e2e-wait.ps1")
	}
}

// RequireExit asserts CLI exit code.
func RequireExit(t *testing.T, want, got int, stdout, stderr string) {
	t.Helper()
	if got != want {
		t.Fatalf("exit=%d want=%d\nstdout:\n%s\nstderr:\n%s", got, want, stdout, stderr)
	}
}

// Host returns configured GitLab base URL.
func Host() string {
	return strings.TrimSpace(os.Getenv("GITLAB_CLI_HOST"))
}
