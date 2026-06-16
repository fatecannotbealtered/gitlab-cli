//go:build integration

package e2e

import (
	"os"
	"path/filepath"
)

// CommandCase describes one leaf CLI command integration test.
type CommandCase struct {
	Path     string
	Args     func(f *Fixture) []string
	WantExit int
	SkipIf   func(f *Fixture) string
	WorkDir  func(f *Fixture) string
	ExtraEnv func(f *Fixture) map[string]string
}

func j(args ...string) []string   { return append(args, "--json") }
func dry(args ...string) []string { return append(args, "--dry-run", "--json") }

func pa(f *Fixture, parts ...string) []string {
	out := append([]string{}, parts...)
	out = append(out, f.ProjectFlag()...)
	out = append(out, "--json")
	return out
}

func pad(f *Fixture, parts ...string) []string {
	out := pa(f, parts...)
	out = append(out, "--dry-run")
	return out
}

// BuildCommandCases returns integration cases for every leaf command (85).
func BuildCommandCases(f *Fixture) []CommandCase {
	host := os.Getenv("GITLAB_CLI_HOST")
	token := os.Getenv("GITLAB_CLI_TOKEN")

	skipNoCI := func(f *Fixture) string {
		if !f.HasPipeline {
			return "pipeline not ready"
		}
		return ""
	}
	skipNoJob := func(f *Fixture) string {
		if !f.HasJob {
			return "job not ready"
		}
		return ""
	}
	skipNoGit := func(f *Fixture) string {
		if f.GitCloneDir == "" {
			return "git clone unavailable"
		}
		return ""
	}
	skipNoRelease := func(f *Fixture) string {
		if f.CommitSHA == "" {
			return "no commit for release tag"
		}
		return ""
	}

	authHomeEnv := func(f *Fixture) map[string]string {
		home := TempConfigDirForAuth(f)
		return map[string]string{"HOME": home, "USERPROFILE": home}
	}

	return []CommandCase{
		// auth — write commands exercise the dry-run contract path here; the
		// full login→keyring→recover→logout cycle is covered by the recorded
		// manual live smoke (docs/E2E-ACCEPTANCE-REPORT.md).
		{Path: "auth login", Args: func(f *Fixture) []string {
			return dry("auth", "login", "--host", host, "--token", token)
		}, ExtraEnv: authHomeEnv},
		{Path: "auth logout", Args: func(f *Fixture) []string { return dry("auth", "logout") }},
		{Path: "auth status", Args: func(f *Fixture) []string { return j("auth", "status") }},
		{Path: "auth profile list", Args: func(f *Fixture) []string {
			return j("auth", "profile", "list")
		}, ExtraEnv: authHomeEnv},
		{Path: "auth profile use", Args: func(f *Fixture) []string {
			return dry("auth", "profile", "use", "default")
		}, ExtraEnv: authHomeEnv},
		{Path: "auth profile remove", Args: func(f *Fixture) []string {
			return dry("auth", "profile", "remove", "default")
		}, ExtraEnv: authHomeEnv},

		// lifecycle
		{Path: "changelog", Args: func(f *Fixture) []string { return j("changelog") }},

		// top-level read
		{Path: "context", Args: func(f *Fixture) []string { return j("context") }, WorkDir: func(f *Fixture) string { return f.GitCloneDir }, SkipIf: skipNoGit},
		{Path: "doctor", Args: func(f *Fixture) []string { return j("doctor") }},
		{Path: "reference", Args: func(f *Fixture) []string { return j("reference") }},
		{Path: "update", Args: func(f *Fixture) []string { return j("update", "--check") }, SkipIf: func(f *Fixture) string {
			return "update checks GitHub Releases; covered by unit tests"
		}},

		// project
		{Path: "project list", Args: func(f *Fixture) []string { return j("project", "list", "--limit", "5") }},
		{Path: "project get", Args: func(f *Fixture) []string { return j("project", "get", f.Project) }},
		{Path: "project members", Args: func(f *Fixture) []string { return j("project", "members", f.Project, "--limit", "5") }},

		// user
		{Path: "user me", Args: func(f *Fixture) []string { return j("user", "me") }},
		{Path: "user search", Args: func(f *Fixture) []string { return j("user", "search", "--query", "root", "--limit", "5") }},
		{Path: "user get", Args: func(f *Fixture) []string { return j("user", "get", "root") }},

		// search
		{Path: "search projects", Args: func(f *Fixture) []string { return j("search", "projects", "--query", "e2e", "--limit", "3") }},
		{Path: "search issues", Args: func(f *Fixture) []string { return pa(f, "search", "issues", "--query", "e2e", "--limit", "3") }},
		{Path: "search mrs", Args: func(f *Fixture) []string { return pa(f, "search", "mrs", "--query", "e2e", "--limit", "3") }},
		{Path: "search code", Args: func(f *Fixture) []string { return pa(f, "search", "code", "--query", "e2e", "--limit", "3") }},
		{Path: "search commits", Args: func(f *Fixture) []string { return pa(f, "search", "commits", "--query", "e2e", "--limit", "3") }},

		// issue
		{Path: "issue list", Args: func(f *Fixture) []string { return pa(f, "issue", "list", "--limit", "5") }},
		{Path: "issue get", Args: func(f *Fixture) []string { return pa(f, "issue", "get", f.IssueIIDStr()) }},
		{Path: "issue create", Args: func(f *Fixture) []string { return pad(f, "issue", "create", "--title", "e2e create") }},
		{Path: "issue update", Args: func(f *Fixture) []string { return pad(f, "issue", "update", f.IssueIIDStr(), "--title", "e2e updated") }},
		{Path: "issue close", Args: func(f *Fixture) []string { return pad(f, "issue", "close", f.IssueIIDStr()) }},
		{Path: "issue reopen", Args: func(f *Fixture) []string { return pad(f, "issue", "reopen", f.IssueIIDStr()) }},
		{Path: "issue assign", Args: func(f *Fixture) []string { return pad(f, "issue", "assign", f.IssueIIDStr(), "root") }},
		{Path: "issue label", Args: func(f *Fixture) []string { return pad(f, "issue", "label", f.IssueIIDStr(), "--add", f.LabelName) }},
		{Path: "issue comment list", Args: func(f *Fixture) []string { return pa(f, "issue", "comment", "list", f.IssueIIDStr(), "--limit", "5") }},
		{Path: "issue comment add", Args: func(f *Fixture) []string { return pad(f, "issue", "comment", "add", f.IssueIIDStr(), "--body", "e2e") }},
		{Path: "issue comment delete", Args: func(f *Fixture) []string {
			return pad(f, "issue", "comment", "delete", f.IssueIIDStr(), "--note-id", f.NoteIDStr())
		}},

		// mr
		{Path: "mr list", Args: func(f *Fixture) []string { return pa(f, "mr", "list", "--limit", "5") }},
		{Path: "mr get", Args: func(f *Fixture) []string { return pa(f, "mr", "get", f.MRIIDStr()) }},
		{Path: "mr current", Args: func(f *Fixture) []string { return j() }, WorkDir: func(f *Fixture) string { return f.GitCloneDir }, SkipIf: skipNoGit},
		{Path: "mr diff", Args: func(f *Fixture) []string { return pa(f, "mr", "diff", f.MRIIDStr()) }},
		{Path: "mr create", Args: func(f *Fixture) []string {
			return pad(f, "mr", "create", "--title", "e2e", "--source-branch", f.BranchFeature)
		}},
		{Path: "mr update", Args: func(f *Fixture) []string { return pad(f, "mr", "update", f.MRIIDStr(), "--title", "e2e mr") }},
		{Path: "mr merge", Args: func(f *Fixture) []string { return pad(f, "mr", "merge", f.MRIIDStr()) }},
		{Path: "mr close", Args: func(f *Fixture) []string { return pad(f, "mr", "close", f.MRIIDStr()) }},
		{Path: "mr reopen", Args: func(f *Fixture) []string { return pad(f, "mr", "reopen", f.MRIIDStr()) }},
		{Path: "mr approve", Args: func(f *Fixture) []string { return pad(f, "mr", "approve", f.MRIIDStr()) }},
		{Path: "mr unapprove", Args: func(f *Fixture) []string { return pad(f, "mr", "unapprove", f.MRIIDStr()) }},
		{Path: "mr comment list", Args: func(f *Fixture) []string { return pa(f, "mr", "comment", "list", f.MRIIDStr(), "--limit", "5") }},
		{Path: "mr comment add", Args: func(f *Fixture) []string { return pad(f, "mr", "comment", "add", f.MRIIDStr(), "--body", "e2e") }},
		{Path: "mr comment delete", Args: func(f *Fixture) []string {
			return pad(f, "mr", "comment", "delete", f.MRIIDStr(), "--note-id", f.MRNoteIDStr())
		}},
		// Inline review: dry-run with explicit SHAs so the case does not depend on
		// the fixture MR's diff being fully prepared (base/head SHA populated).
		{Path: "mr discussion create", Args: func(f *Fixture) []string {
			return pad(f, "mr", "discussion", "create", f.MRIIDStr(), "--new-path", f.FilePath, "--new-line", "1",
				"--base-sha", "x", "--start-sha", "x", "--head-sha", "x", "--body", "e2e inline")
		}},
		{Path: "mr discussion resolve", Args: func(f *Fixture) []string {
			return pad(f, "mr", "discussion", "resolve", f.MRIIDStr(), "--discussion-id", "x")
		}},

		// label
		{Path: "label list", Args: func(f *Fixture) []string { return pa(f, "label", "list", "--limit", "10") }},
		{Path: "label create", Args: func(f *Fixture) []string {
			return pad(f, "label", "create", "--name", "e2e-other", "--color", "#abcdef")
		}},
		{Path: "label update", Args: func(f *Fixture) []string {
			return pad(f, "label", "update", "--label-id", f.LabelIDStr(), "--description", "e2e")
		}},
		{Path: "label delete", Args: func(f *Fixture) []string { return pad(f, "label", "delete", "--label-id", "999999") }},

		// milestone
		{Path: "milestone list", Args: func(f *Fixture) []string { return pa(f, "milestone", "list", "--limit", "5") }},
		{Path: "milestone get", Args: func(f *Fixture) []string { return pa(f, "milestone", "get", "--milestone-id", f.MilestoneIDStr()) }},
		{Path: "milestone create", Args: func(f *Fixture) []string { return pad(f, "milestone", "create", "--title", "e2e-ms") }},
		{Path: "milestone update", Args: func(f *Fixture) []string {
			return pad(f, "milestone", "update", "--milestone-id", f.MilestoneIDStr(), "--title", "e2e-milestone")
		}},
		{Path: "milestone close", Args: func(f *Fixture) []string { return pad(f, "milestone", "close", "--milestone-id", f.MilestoneIDStr()) }},

		// variable
		{Path: "variable list", Args: func(f *Fixture) []string { return pa(f, "variable", "list", "--limit", "10") }},
		{Path: "variable get", Args: func(f *Fixture) []string { return pa(f, "variable", "get", "--key", f.VariableKey) }},
		{Path: "variable create", Args: func(f *Fixture) []string { return pad(f, "variable", "create", "--key", "E2E_OTHER", "--value", "v") }},
		{Path: "variable update", Args: func(f *Fixture) []string {
			return pad(f, "variable", "update", "--key", f.VariableKey, "--value", "v2")
		}},
		{Path: "variable delete", Args: func(f *Fixture) []string { return pad(f, "variable", "delete", "--key", "E2E_OTHER") }},

		// release
		{Path: "release list", Args: func(f *Fixture) []string { return pa(f, "release", "list", "--limit", "5") }},
		{Path: "release get", Args: func(f *Fixture) []string { return pa(f, "release", "get", "--tag", f.ReleaseTag) }, SkipIf: skipNoRelease},
		{Path: "release create", Args: func(f *Fixture) []string { return pad(f, "release", "create", "--tag", "v9.9.9-e2e", "--name", "e2e") }},
		{Path: "release update", Args: func(f *Fixture) []string { return pad(f, "release", "update", "--tag", f.ReleaseTag, "--name", "e2e") }, SkipIf: skipNoRelease},
		{Path: "release delete", Args: func(f *Fixture) []string { return pad(f, "release", "delete", "--tag", "v9.9.9-e2e") }},

		// repo
		{Path: "repo file get", Args: func(f *Fixture) []string { return pa(f, "repo", "file", "get", "--path", f.FilePath, "--ref", "main") }},
		{Path: "repo file create", Args: func(f *Fixture) []string {
			return pad(f, "repo", "file", "create", "--path", "e2e-new.txt", "--branch", "main", "--content", "x", "--commit-message", "e2e")
		}},
		{Path: "repo file update", Args: func(f *Fixture) []string {
			return pad(f, "repo", "file", "update", "--path", f.FilePath, "--branch", "main", "--content", "readme", "--commit-message", "e2e")
		}},
		{Path: "repo file delete", Args: func(f *Fixture) []string {
			return pad(f, "repo", "file", "delete", "--path", "e2e-del.txt", "--branch", "main", "--commit-message", "e2e")
		}},
		{Path: "repo branch list", Args: func(f *Fixture) []string { return pa(f, "repo", "branch", "list", "--limit", "10") }},
		{Path: "repo branch create", Args: func(f *Fixture) []string {
			return pad(f, "repo", "branch", "create", "--name", "feat/e2e-2", "--ref", "main")
		}},
		{Path: "repo branch delete", Args: func(f *Fixture) []string { return pad(f, "repo", "branch", "delete", "--name", "feat/e2e-2") }},
		{Path: "repo commit list", Args: func(f *Fixture) []string { return pa(f, "repo", "commit", "list", "--limit", "3") }},
		{Path: "repo commit get", Args: func(f *Fixture) []string { return pa(f, "repo", "commit", "get", f.CommitSHA) }, SkipIf: func(f *Fixture) string {
			if f.CommitSHA == "" {
				return "no commit sha"
			}
			return ""
		}},
		{Path: "repo tree", Args: func(f *Fixture) []string { return pa(f, "repo", "tree", "--limit", "10") }},

		// pipeline
		{Path: "pipeline list", Args: func(f *Fixture) []string { return pa(f, "pipeline", "list", "--limit", "5") }, SkipIf: skipNoCI},
		{Path: "pipeline get", Args: func(f *Fixture) []string { return pa(f, "pipeline", "get", f.PipelineIDStr()) }, SkipIf: skipNoCI},
		{Path: "pipeline current", Args: func(f *Fixture) []string { return j() }, WorkDir: func(f *Fixture) string { return f.GitCloneDir }, SkipIf: skipNoGit},
		{Path: "pipeline jobs", Args: func(f *Fixture) []string { return pa(f, "pipeline", "jobs", f.PipelineIDStr()) }, SkipIf: skipNoCI},
		{Path: "pipeline create", Args: func(f *Fixture) []string { return pad(f, "pipeline", "create", "--ref", "main") }},
		{Path: "pipeline retry", Args: func(f *Fixture) []string { return pad(f, "pipeline", "retry", f.PipelineIDStr()) }, SkipIf: skipNoCI},
		{Path: "pipeline cancel", Args: func(f *Fixture) []string { return pad(f, "pipeline", "cancel", f.PipelineIDStr()) }, SkipIf: skipNoCI},
		{Path: "pipeline wait", Args: func(f *Fixture) []string { return pa(f, "pipeline", "wait", f.PipelineIDStr(), "--timeout", "1") }, SkipIf: skipNoCI},

		// job
		{Path: "job get", Args: func(f *Fixture) []string { return pa(f, "job", "get", f.JobIDStr()) }, SkipIf: skipNoJob},
		{Path: "job log", Args: func(f *Fixture) []string {
			out := append(f.ProjectFlag(), "job", "log", f.JobIDStr())
			return out
		}, SkipIf: skipNoJob},
		{Path: "job retry", Args: func(f *Fixture) []string { return pad(f, "job", "retry", f.JobIDStr()) }, SkipIf: skipNoJob},
		{Path: "job cancel", Args: func(f *Fixture) []string { return pad(f, "job", "cancel", f.JobIDStr()) }, SkipIf: skipNoJob},
		{Path: "job artifacts", Args: func(f *Fixture) []string {
			return pa(f, "job", "artifacts", f.JobIDStr(), "--output", filepath.Join(os.TempDir(), "gitlab-cli-e2e-artifacts.zip"))
		}, SkipIf: skipNoJob},
		{Path: "job wait", Args: func(f *Fixture) []string { return pa(f, "job", "wait", f.JobIDStr(), "--timeout", "1") }, SkipIf: skipNoJob},
	}
}

// TempConfigDirForAuth uses a stable temp dir keyed by project for auth login (not per-test TempDir).
func TempConfigDirForAuth(f *Fixture) string {
	return RepoRoot() + "/.e2e-auth-home"
}
