//go:build integration

package e2e

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// RepoRoot is the gitlab-cli module root (parent of e2e/).
func RepoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

// LoadLocalEnv reads scripts/e2e.local.env into the process environment when vars are unset.
func LoadLocalEnv() {
	if os.Getenv("GITLAB_CLI_HOST") != "" && os.Getenv("GITLAB_CLI_TOKEN") != "" {
		return
	}
	path := filepath.Join(RepoRoot(), "scripts", "e2e.local.env")
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" {
			continue
		}
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
}

// Configured reports whether local GitLab E2E credentials are available.
func Configured() bool {
	return strings.TrimSpace(os.Getenv("GITLAB_CLI_HOST")) != "" &&
		strings.TrimSpace(os.Getenv("GITLAB_CLI_TOKEN")) != ""
}
