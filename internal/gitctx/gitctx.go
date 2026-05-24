// Package gitctx reads local git context (current branch, remotes, parsed
// GitLab project path) so that "auto" composite commands like
// `gitlab-cli mr create --auto` can derive sensible defaults from cwd.
package gitctx

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNotAGitRepo is returned when the current working directory is not a git repository.
var ErrNotAGitRepo = errors.New("not a git repository")

// ErrNoGitLabRemote is returned when no remote URL points at a GitLab-style host.
var ErrNoGitLabRemote = errors.New("no GitLab remote found")

// runGit executes a git subcommand in the given dir (empty = cwd) and returns trimmed stdout.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr := strings.TrimSpace(string(ee.Stderr))
			if strings.Contains(stderr, "not a git repository") {
				return "", ErrNotAGitRepo
			}
			if stderr != "" {
				return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), stderr)
			}
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// IsGitRepo returns true if dir (or cwd if empty) is inside a git working tree.
func IsGitRepo(dir string) bool {
	out, err := runGit(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && out == "true"
}

// CurrentBranch returns the current branch name, or empty string if HEAD is detached.
func CurrentBranch(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	if out == "HEAD" {
		// Detached HEAD
		return "", nil
	}
	return out, nil
}

// RepoRoot returns the absolute path to the git repository root.
func RepoRoot(dir string) (string, error) {
	out, err := runGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return filepath.Clean(out), nil
}

// RemoteURL returns the URL of the named remote.
func RemoteURL(dir, remote string) (string, error) {
	if remote == "" {
		remote = "origin"
	}
	return runGit(dir, "remote", "get-url", remote)
}

// Remotes returns a map from remote name → fetch URL.
func Remotes(dir string) (map[string]string, error) {
	out, err := runGit(dir, "remote", "-v")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return map[string]string{}, nil
	}
	result := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		// "<name>\t<url> (fetch|push)"
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if fields[2] != "(fetch)" {
			continue
		}
		result[fields[0]] = fields[1]
	}
	return result, nil
}

// GitLabRemote describes a parsed remote URL pointing at a GitLab host.
type GitLabRemote struct {
	Host        string // e.g. "gitlab.example.com"
	ProjectPath string // e.g. "group/subgroup/project" (no .git suffix, no leading slash)
	URL         string // original URL
}

// ParseGitLabRemote parses an SSH or HTTPS git remote URL into a GitLabRemote.
// Accepts:
//
//	git@gitlab.example.com:group/subgroup/project.git
//	ssh://git@gitlab.example.com:22/group/subgroup/project.git
//	https://gitlab.example.com/group/subgroup/project.git
//	https://oauth2:token@gitlab.example.com/group/subgroup/project.git
//	http://gitlab.example.com/group/project (rare; allowed for local dev)
//
// Returns ErrNoGitLabRemote if the URL is empty or unrecognised.
func ParseGitLabRemote(remoteURL string) (*GitLabRemote, error) {
	rawURL := strings.TrimSpace(remoteURL)
	if rawURL == "" {
		return nil, ErrNoGitLabRemote
	}

	// SSH "scp-style" (git@host:path) — convert to ssh:// form so url.Parse handles it.
	if !strings.Contains(rawURL, "://") && strings.Contains(rawURL, "@") && strings.Contains(rawURL, ":") {
		// e.g. "git@gitlab.example.com:group/proj.git"
		at := strings.Index(rawURL, "@")
		colon := strings.Index(rawURL[at:], ":") + at
		if colon > at {
			host := rawURL[at+1 : colon]
			path := strings.TrimPrefix(rawURL[colon+1:], "/")
			return &GitLabRemote{
				Host:        host,
				ProjectPath: cleanProjectPath(path),
				URL:         rawURL,
			}, nil
		}
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parsing remote URL: %w", err)
	}
	if u.Host == "" {
		return nil, ErrNoGitLabRemote
	}
	host := u.Host
	// Strip embedded port if non-standard SSH (rare for HTTPS).
	if strings.Contains(host, ":") {
		host = strings.SplitN(host, ":", 2)[0]
	}
	path := strings.TrimPrefix(u.Path, "/")
	return &GitLabRemote{
		Host:        host,
		ProjectPath: cleanProjectPath(path),
		URL:         rawURL,
	}, nil
}

func cleanProjectPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimSuffix(p, ".git")
	p = strings.TrimPrefix(p, "/")
	return p
}

// DetectGitLabRemote scans the working directory's remotes (in priority order:
// "origin", "upstream", then any other) and returns the first parsable GitLab remote.
func DetectGitLabRemote(dir string) (*GitLabRemote, error) {
	remotes, err := Remotes(dir)
	if err != nil {
		return nil, err
	}
	if len(remotes) == 0 {
		return nil, ErrNoGitLabRemote
	}

	prefer := []string{"origin", "upstream"}
	tried := map[string]bool{}
	for _, name := range prefer {
		if u, ok := remotes[name]; ok {
			tried[name] = true
			if r, err := ParseGitLabRemote(u); err == nil && r.ProjectPath != "" {
				return r, nil
			}
		}
	}
	for name, u := range remotes {
		if tried[name] {
			continue
		}
		if r, err := ParseGitLabRemote(u); err == nil && r.ProjectPath != "" {
			return r, nil
		}
	}
	return nil, ErrNoGitLabRemote
}

// Context bundles the most useful git context for composite commands.
type Context struct {
	Repo          string        // absolute path to git toplevel; empty if not a repo
	CurrentBranch string        // empty if detached HEAD or not a repo
	Remote        *GitLabRemote // nil if no GitLab remote found
}

// Detect returns the context for dir (or cwd if empty).
// Returns a populated Context with whatever fields could be determined; never returns ErrNotAGitRepo.
func Detect(dir string) (*Context, error) {
	ctx := &Context{}
	if !IsGitRepo(dir) {
		return ctx, nil
	}
	if root, err := RepoRoot(dir); err == nil {
		ctx.Repo = root
	}
	if branch, err := CurrentBranch(dir); err == nil {
		ctx.CurrentBranch = branch
	}
	if r, err := DetectGitLabRemote(dir); err == nil {
		ctx.Remote = r
	}
	return ctx, nil
}
