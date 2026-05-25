package gitctx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseGitLabRemote_SSH(t *testing.T) {
	r, err := ParseGitLabRemote("git@gitlab.example.com:group/subgroup/project.git")
	if err != nil {
		t.Fatalf("ParseGitLabRemote: %v", err)
	}
	if r.Host != "gitlab.example.com" {
		t.Errorf("Host = %q", r.Host)
	}
	if r.ProjectPath != "group/subgroup/project" {
		t.Errorf("ProjectPath = %q", r.ProjectPath)
	}
}

func TestParseGitLabRemote_SSHRoot(t *testing.T) {
	r, err := ParseGitLabRemote("git@gitlab.example.com:user/proj.git")
	if err != nil {
		t.Fatalf("ParseGitLabRemote: %v", err)
	}
	if r.ProjectPath != "user/proj" {
		t.Errorf("ProjectPath = %q", r.ProjectPath)
	}
}

func TestParseGitLabRemote_HTTPS(t *testing.T) {
	r, err := ParseGitLabRemote("https://gitlab.example.com/group/subgroup/project.git")
	if err != nil {
		t.Fatalf("ParseGitLabRemote: %v", err)
	}
	if r.Host != "gitlab.example.com" {
		t.Errorf("Host = %q", r.Host)
	}
	if r.ProjectPath != "group/subgroup/project" {
		t.Errorf("ProjectPath = %q", r.ProjectPath)
	}
}

func TestParseGitLabRemote_HTTPSWithAuth(t *testing.T) {
	r, err := ParseGitLabRemote("https://oauth2:token@gitlab.example.com/group/proj.git")
	if err != nil {
		t.Fatalf("ParseGitLabRemote: %v", err)
	}
	if r.Host != "gitlab.example.com" {
		t.Errorf("Host = %q (auth should be stripped)", r.Host)
	}
	if r.ProjectPath != "group/proj" {
		t.Errorf("ProjectPath = %q", r.ProjectPath)
	}
}

func TestParseGitLabRemote_SSHURIScheme(t *testing.T) {
	r, err := ParseGitLabRemote("ssh://git@gitlab.example.com:22/group/proj.git")
	if err != nil {
		t.Fatalf("ParseGitLabRemote: %v", err)
	}
	if r.Host != "gitlab.example.com" {
		t.Errorf("Host = %q (port should be stripped)", r.Host)
	}
	if r.ProjectPath != "group/proj" {
		t.Errorf("ProjectPath = %q", r.ProjectPath)
	}
}

func TestParseGitLabRemote_NoSuffix(t *testing.T) {
	r, err := ParseGitLabRemote("https://gitlab.example.com/group/proj")
	if err != nil {
		t.Fatalf("ParseGitLabRemote: %v", err)
	}
	if r.ProjectPath != "group/proj" {
		t.Errorf("ProjectPath = %q", r.ProjectPath)
	}
}

func TestParseGitLabRemote_Empty(t *testing.T) {
	if _, err := ParseGitLabRemote(""); !errors.Is(err, ErrNoGitLabRemote) {
		t.Errorf("expected ErrNoGitLabRemote, got %v", err)
	}
}

func TestParseGitLabRemote_EmptyHost(t *testing.T) {
	if _, err := ParseGitLabRemote("http:///group/proj"); !errors.Is(err, ErrNoGitLabRemote) {
		t.Errorf("expected ErrNoGitLabRemote for empty host, got %v", err)
	}
}

func TestParseGitLabRemote_InvalidURL(t *testing.T) {
	_, err := ParseGitLabRemote("https://%zz.example.com/group/proj")
	if err == nil {
		t.Fatal("expected parse error for malformed URL")
	}
	if !strings.Contains(err.Error(), "parsing remote URL") {
		t.Errorf("error = %v, want parsing remote URL wrapper", err)
	}
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// initGitRepo creates a git repo in a temp dir with the given remotes and branch.
// Returns the repo path. Skips test if `git` binary is not available.
func initGitRepo(t *testing.T, remotes map[string]string, branch string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "--initial-branch=main", "."},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	// Need at least one commit so HEAD is valid.
	c := exec.Command("git", "commit", "--allow-empty", "-m", "init")
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}

	if branch != "" && branch != "main" {
		c := exec.Command("git", "checkout", "-b", branch)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git checkout -b: %v\n%s", err, out)
		}
	}

	for name, url := range remotes {
		c := exec.Command("git", "remote", "add", name, url)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git remote add: %v\n%s", err, out)
		}
	}
	return dir
}

func TestIsGitRepo(t *testing.T) {
	dir := initGitRepo(t, nil, "")
	if !IsGitRepo(dir) {
		t.Error("IsGitRepo should be true for initialised repo")
	}
	if IsGitRepo(t.TempDir()) {
		t.Error("IsGitRepo should be false for empty temp dir")
	}
}

func TestCurrentBranch(t *testing.T) {
	dir := initGitRepo(t, nil, "feature/x")
	branch, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "feature/x" {
		t.Errorf("CurrentBranch = %q, want feature/x", branch)
	}
}

func TestRemotes(t *testing.T) {
	dir := initGitRepo(t, map[string]string{
		"origin":   "git@gitlab.example.com:group/proj.git",
		"upstream": "https://gitlab.example.com/parent/proj.git",
	}, "")
	remotes, err := Remotes(dir)
	if err != nil {
		t.Fatalf("Remotes: %v", err)
	}
	if remotes["origin"] != "git@gitlab.example.com:group/proj.git" {
		t.Errorf("origin = %q", remotes["origin"])
	}
	if remotes["upstream"] != "https://gitlab.example.com/parent/proj.git" {
		t.Errorf("upstream = %q", remotes["upstream"])
	}
}

func TestDetectGitLabRemote_PrefersOrigin(t *testing.T) {
	dir := initGitRepo(t, map[string]string{
		"origin":   "git@gitlab.example.com:owner/proj.git",
		"upstream": "https://gitlab.example.com/parent/proj.git",
	}, "")
	r, err := DetectGitLabRemote(dir)
	if err != nil {
		t.Fatalf("DetectGitLabRemote: %v", err)
	}
	if r.ProjectPath != "owner/proj" {
		t.Errorf("ProjectPath = %q, want owner/proj (origin wins)", r.ProjectPath)
	}
}

func TestDetect_NotAGitRepo_ReturnsEmptyContext(t *testing.T) {
	dir := t.TempDir()
	ctx, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if ctx.Repo != "" || ctx.CurrentBranch != "" || ctx.Remote != nil {
		t.Errorf("expected empty context, got %+v", ctx)
	}
}

func TestDetect_FullContext(t *testing.T) {
	dir := initGitRepo(t, map[string]string{
		"origin": "git@gitlab.example.com:team/svc.git",
	}, "feat/abc")
	ctx, err := Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if ctx.CurrentBranch != "feat/abc" {
		t.Errorf("CurrentBranch = %q", ctx.CurrentBranch)
	}
	if ctx.Remote == nil || ctx.Remote.ProjectPath != "team/svc" {
		t.Errorf("Remote = %+v", ctx.Remote)
	}
}

func TestRemoteURL(t *testing.T) {
	dir := initGitRepo(t, map[string]string{
		"origin": "git@gitlab.example.com:group/proj.git",
	}, "")

	url, err := RemoteURL(dir, "origin")
	if err != nil {
		t.Fatalf("RemoteURL(origin): %v", err)
	}
	if url != "git@gitlab.example.com:group/proj.git" {
		t.Errorf("RemoteURL = %q", url)
	}

	url, err = RemoteURL(dir, "")
	if err != nil {
		t.Fatalf("RemoteURL(default): %v", err)
	}
	if url != "git@gitlab.example.com:group/proj.git" {
		t.Errorf("default remote URL = %q", url)
	}
}

func TestRemoteURL_NotAGitRepo(t *testing.T) {
	_, err := RemoteURL(t.TempDir(), "origin")
	if !errors.Is(err, ErrNotAGitRepo) {
		t.Errorf("expected ErrNotAGitRepo, got %v", err)
	}
}

func TestRemoteURL_MissingRemote(t *testing.T) {
	dir := initGitRepo(t, nil, "")
	_, err := RemoteURL(dir, "missing")
	if err == nil {
		t.Fatal("expected error for missing remote")
	}
	if !strings.Contains(err.Error(), "git remote get-url") {
		t.Errorf("error = %v, want git remote get-url context", err)
	}
}

func TestCurrentBranch_NotAGitRepo(t *testing.T) {
	_, err := CurrentBranch(t.TempDir())
	if !errors.Is(err, ErrNotAGitRepo) {
		t.Errorf("expected ErrNotAGitRepo, got %v", err)
	}
}

func TestCurrentBranch_DetachedHEAD(t *testing.T) {
	dir := initGitRepo(t, nil, "")
	runGitCmd(t, dir, "checkout", "--detach", "HEAD")

	branch, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "" {
		t.Errorf("CurrentBranch = %q, want empty for detached HEAD", branch)
	}
}

func TestRepoRoot_NotAGitRepo(t *testing.T) {
	_, err := RepoRoot(t.TempDir())
	if !errors.Is(err, ErrNotAGitRepo) {
		t.Errorf("expected ErrNotAGitRepo, got %v", err)
	}
}

func TestRepoRoot(t *testing.T) {
	dir := initGitRepo(t, nil, "")
	root, err := RepoRoot(dir)
	if err != nil {
		t.Fatalf("RepoRoot: %v", err)
	}
	want, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	if root != want {
		t.Errorf("RepoRoot = %q, want %q", root, want)
	}
}

func TestRemotes_Empty(t *testing.T) {
	dir := initGitRepo(t, nil, "")
	remotes, err := Remotes(dir)
	if err != nil {
		t.Fatalf("Remotes: %v", err)
	}
	if len(remotes) != 0 {
		t.Errorf("Remotes = %v, want empty map", remotes)
	}
}

func TestRemotes_NotAGitRepo(t *testing.T) {
	_, err := Remotes(t.TempDir())
	if !errors.Is(err, ErrNotAGitRepo) {
		t.Errorf("expected ErrNotAGitRepo, got %v", err)
	}
}

func TestRunGit_NoGitBinary(t *testing.T) {
	emptyBin := t.TempDir()
	t.Setenv("PATH", emptyBin)

	_, err := runGit("", "version")
	if err == nil {
		t.Fatal("expected error when git binary is unavailable")
	}
	if strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("unexpected ErrNotAGitRepo: %v", err)
	}
}

func TestRunGit_ExitErrorWithStderr(t *testing.T) {
	dir := initGitRepo(t, nil, "")
	_, err := runGit(dir, "checkout", "does-not-exist")
	if err == nil {
		t.Fatal("expected checkout error")
	}
	if strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("unexpected ErrNotAGitRepo: %v", err)
	}
	if !strings.Contains(err.Error(), "git checkout") {
		t.Errorf("error = %v, want git subcommand context", err)
	}
}

func TestDetectGitLabRemote_NoRemotes(t *testing.T) {
	dir := initGitRepo(t, nil, "")
	_, err := DetectGitLabRemote(dir)
	if !errors.Is(err, ErrNoGitLabRemote) {
		t.Errorf("expected ErrNoGitLabRemote, got %v", err)
	}
}

func TestDetectGitLabRemote_NotAGitRepo(t *testing.T) {
	_, err := DetectGitLabRemote(t.TempDir())
	if !errors.Is(err, ErrNotAGitRepo) {
		t.Errorf("expected ErrNotAGitRepo, got %v", err)
	}
}

func TestDetectGitLabRemote_PrefersUpstreamWhenOriginEmpty(t *testing.T) {
	dir := initGitRepo(t, map[string]string{
		"origin":   "https://example.com/",
		"upstream": "git@gitlab.example.com:parent/proj.git",
	}, "")
	r, err := DetectGitLabRemote(dir)
	if err != nil {
		t.Fatalf("DetectGitLabRemote: %v", err)
	}
	if r.ProjectPath != "parent/proj" {
		t.Errorf("ProjectPath = %q, want parent/proj from upstream", r.ProjectPath)
	}
}

func TestDetectGitLabRemote_FallsBackToOtherRemote(t *testing.T) {
	dir := initGitRepo(t, map[string]string{
		"origin":   "https://example.com/",
		"upstream": "https://example.com/",
		"mirror":   "git@gitlab.example.com:mirror/proj.git",
	}, "")
	r, err := DetectGitLabRemote(dir)
	if err != nil {
		t.Fatalf("DetectGitLabRemote: %v", err)
	}
	if r.ProjectPath != "mirror/proj" {
		t.Errorf("ProjectPath = %q, want mirror/proj from fallback remote", r.ProjectPath)
	}
}

func TestDetectGitLabRemote_AllInvalid(t *testing.T) {
	dir := initGitRepo(t, map[string]string{
		"origin": "https://example.com/",
	}, "")
	_, err := DetectGitLabRemote(dir)
	if !errors.Is(err, ErrNoGitLabRemote) {
		t.Errorf("expected ErrNoGitLabRemote, got %v", err)
	}
}

func installFakeGit(t *testing.T, body string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git binary not available")
	}
	t.Setenv("GIT_REAL_PATH", realGit)

	work := t.TempDir()
	src := filepath.Join(work, "fakegit.go")
	source := `package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
` + body + `
	real := os.Getenv("GIT_REAL_PATH")
	if real == "" {
		fmt.Fprintln(os.Stderr, "GIT_REAL_PATH not set")
		os.Exit(1)
	}
	cmd := exec.Command(real, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		os.Exit(1)
	}
}
`
	if err := os.WriteFile(src, []byte(source), 0600); err != nil {
		t.Fatalf("WriteFile fake git source: %v", err)
	}

	out := filepath.Join(work, "git")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	build := exec.Command("go", "build", "-o", out, src)
	if outBytes, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake git: %v\n%s", err, outBytes)
	}

	pathKey := "PATH"
	if runtime.GOOS == "windows" {
		pathKey = "Path"
	}
	t.Setenv(pathKey, work+string(os.PathListSeparator)+os.Getenv(pathKey))
}

func TestRemotes_SkipsMalformedLines(t *testing.T) {
	installFakeGit(t, `
	if len(os.Args) >= 3 && os.Args[1] == "remote" && os.Args[2] == "-v" {
		fmt.Println("broken-line")
		fmt.Println("origin https://gitlab.example.com/group/proj.git (fetch)")
		os.Exit(0)
	}
`)

	remotes, err := Remotes(t.TempDir())
	if err != nil {
		t.Fatalf("Remotes: %v", err)
	}
	if remotes["origin"] != "https://gitlab.example.com/group/proj.git" {
		t.Errorf("Remotes = %v, want origin fetch URL only", remotes)
	}
}
