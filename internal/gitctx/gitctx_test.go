package gitctx

import (
	"errors"
	"os/exec"
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
