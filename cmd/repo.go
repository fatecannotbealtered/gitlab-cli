package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// The repository group (spec 2.0): `repo tree`, `repo file`, `repo branch` and
// `repo commit`, plus the `release` group (release.go). Every resource carries a
// ref that mirrors its GitLab web path, and every command takes the ref, the web
// URL or the plain name (CLI-048):
//
//	branch     acme/web/-/branches/<name>        (web: /-/tree/<name> or /-/branches/<name>)
//	file       acme/web/-/blob/<ref>/<path>      (web: the same path)
//	directory  acme/web/-/tree/<ref>/<path>
//	commit     acme/web/-/commit/<sha>           (also GitLab's acme/web@<sha>)
//	release    acme/web/-/releases/<tag>         (also /-/tags/<tag>)
//
// A file write's target key is the file's ref on the branch it commits to, so a
// scope grant for acme/web/-/blob/<branch> covers the files of that branch and
// acme/web/-/blob/<branch>/docs only those under docs/. A branch name, a tag or a
// path inside a ref is text people wrote, so refs that hold one are untrusted
// (CLI-046). Diffs, file contents and commit bodies are their own commands, with
// bounded summaries in lists (CLI-043).

const (
	repoEvidence       = "simulated:cmd/repo_test.go"
	repoFileEvidence   = "simulated:cmd/repo_file_test.go"
	repoBranchEvidence = "simulated:cmd/repo_branch_test.go"
	repoCommitEvidence = "simulated:cmd/repo_commit_test.go"
)

var (
	repoTreeEntryFields = []string{"ref", "path", "name", "type", "mode", "id"}
	repoFileFields      = []string{"ref", "path", "name", "ref_name", "size", "content_sha256", "blob_id", "commit_id",
		"last_commit_id", "web_url"}
	repoFileContentFields = []string{"ref", "path", "ref_name", "size", "binary", "content", "truncated", "start_line",
		"line_count", "total_lines", "has_more", "next_offset"}
	repoBranchFields = []string{"ref", "name", "default", "protected", "merged", "developers_can_push", "developers_can_merge",
		"can_push", "commit_sha", "commit_title", "committed_date", "web_url"}
	repoCommitListFields = []string{"ref", "sha", "short_sha", "title", "author_name", "author_email", "authored_date",
		"committer_name", "committer_email", "committed_date", "parent_shas", "web_url", "additions", "deletions", "total"}
	repoCommitFields        = append(append([]string{}, repoCommitListFields...), "message", "message_truncated")
	repoCommitListUntrusted = []string{"title", "author_name", "author_email", "committer_name", "committer_email"}
	repoCommitUntrusted     = append(append([]string{}, repoCommitListUntrusted...), "message")
	repoCommitDiffFields    = []string{"ref", "path", "old_path", "new_path", "change", "additions", "deletions", "diff", "diff_truncated"}
)

// registerRepo registers the repo group and its nested groups, the release group,
// and the root shortcuts for the nested nouns (CLI-022, R02).
func registerRepo(app *aicli.App) {
	repo := addGroup(app, "repo", "Browse and change a project's repository: tree, files, branches and commits", "repos")
	registerRepoSchemas(app)
	addRepoTree(app, repo)

	file := repoSubgroup(app, repo, "file", "Read a file's metadata and content; create, update or delete a file in one commit", "files")
	addRepoFileGet(app, file)
	addRepoFileContent(app, file)
	addRepoFileCreate(app, file)
	addRepoFileUpdate(app, file)
	addRepoFileDelete(app, file)

	branch := repoSubgroup(app, repo, "branch", "List, read, create and delete branches", "branches")
	addRepoBranchList(app, branch)
	addRepoBranchGet(app, branch)
	addRepoBranchCreate(app, branch)
	addRepoBranchDelete(app, branch)

	commit := repoSubgroup(app, repo, "commit", "List and read commits and their diffs; create one commit from several file changes", "commits")
	addRepoCommitList(app, commit)
	addRepoCommitGet(app, commit)
	addRepoCommitDiff(app, commit)
	addRepoCommitCreate(app, commit)

	// The nouns agents type at the root (the fleet's guess list, CLI-022).
	for word, path := range map[string][]string{
		"branch": {"repo", "branch"}, "branches": {"repo", "branch"},
		"file": {"repo", "file"}, "files": {"repo", "file"},
		"commit": {"repo", "commit"}, "commits": {"repo", "commit"},
		"tree": {"repo", "tree"},
	} {
		app.AddShortcut(word, path...)
	}
	registerRelease(app)
}

func registerRepoSchemas(app *aicli.App) {
	app.Schema("tree_entry_page", aicli.SchemaDef{Shape: "page", Fields: repoTreeEntryFields,
		DefaultFields: []string{"ref", "path", "type"}, Untrusted: []string{"ref", "path", "name"}})
	app.Schema("file", aicli.SchemaDef{Shape: "object", Fields: repoFileFields, Untrusted: []string{"ref", "path", "name"}})
	app.Schema("file_content", aicli.SchemaDef{Shape: "object", Fields: repoFileContentFields, Untrusted: []string{"ref", "path", "content"}})
	app.Schema("file_deleted", aicli.SchemaDef{Shape: "object", Fields: []string{"ref", "deleted"}, Untrusted: []string{"ref"}})
	app.Schema("branch", aicli.SchemaDef{Shape: "object", Fields: repoBranchFields, Untrusted: []string{"ref", "name", "commit_title"}})
	app.Schema("branch_page", aicli.SchemaDef{Shape: "page", Fields: repoBranchFields,
		DefaultFields: []string{"ref", "name", "default", "protected", "merged", "committed_date"},
		Untrusted:     []string{"ref", "name", "commit_title"}})
	app.Schema("branch_deleted", aicli.SchemaDef{Shape: "object", Fields: []string{"ref", "deleted"}, Untrusted: []string{"ref"}})
	app.Schema("commit", aicli.SchemaDef{Shape: "object", Fields: repoCommitFields, Untrusted: repoCommitUntrusted})
	// Lists carry the title, never the message body (CLI-043); the line counts are
	// default fields, present when GitLab computed them (--with-stats).
	app.Schema("commit_page", aicli.SchemaDef{Shape: "page", Fields: repoCommitListFields,
		DefaultFields: []string{"ref", "short_sha", "title", "author_name", "committed_date", "additions", "deletions"},
		Untrusted:     repoCommitListUntrusted})
	// A diff entry's ref is the file as the commit left it (null for a deleted file),
	// ready for repo file content.
	app.Schema("commit_diff_page", aicli.SchemaDef{Shape: "page", Fields: repoCommitDiffFields,
		DefaultFields: repoCommitDiffFields, Untrusted: []string{"ref", "path", "old_path", "new_path", "diff"}})
	app.Schema("commit_action", aicli.SchemaDef{Shape: "object", Fields: []string{"ref", "path", "action", "commit_ref", "commit_sha"},
		Untrusted: []string{"ref", "path"}})
}

// repoSubgroup adds a nested noun (repo file, repo branch, repo commit).
func repoSubgroup(app *aicli.App, parent *cobra.Command, use, summary string, aliases ...string) *cobra.Command {
	g := &cobra.Command{Use: use, Aliases: aliases}
	parent.AddCommand(g)
	app.Group(g, summary)
	return g
}

// ---------------------------------------------------------------- refs

func repoBranchRef(project, name string) string { return project + "/-/branches/" + name }
func repoCommitRef(project, sha string) string  { return project + "/-/commit/" + sha }
func repoBlobRef(project, ref, path string) string {
	return project + "/-/blob/" + ref + "/" + path
}

// repoTreeRef is a directory's ref; the root directory has no path.
func repoTreeRef(project, ref, path string) string {
	if path == "" {
		return project + "/-/tree/" + ref
	}
	return project + "/-/tree/" + ref + "/" + path
}

// repoWebURL is a GitLab web URL for a ref-style path, each segment escaped.
func repoWebURL(host, refPath string) string {
	segs := strings.Split(refPath, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.TrimSuffix(host, "/") + "/" + strings.Join(segs, "/")
}

// repoWebArg is an argument split at a GitLab web-path part ("/-/blob/"): the
// project before it and the rest after it. web is false for a plain value.
type repoWebArg struct {
	project, part, rest string
	web                 bool
}

// parseRepoWebArg reads a ref or web URL whose path holds one of parts ("blob",
// "tree", "branches"). A URL without one of them is an error; a value without one is
// plain (web false).
func parseRepoWebArg(raw string, parts ...string) (repoWebArg, error) {
	s := strings.TrimSpace(raw)
	isURL := strings.Contains(s, "://")
	if isURL {
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			return repoWebArg{}, errors.New("not a GitLab URL")
		}
		s = u.Path
	}
	best, bestPart := -1, ""
	for _, p := range parts {
		if i := strings.Index(s, "/-/"+p+"/"); i >= 0 && (best < 0 || i < best) {
			best, bestPart = i, p
		}
	}
	if best < 0 {
		if isURL {
			return repoWebArg{}, errors.New("the URL names no " + strings.Join(parts, " or "))
		}
		return repoWebArg{rest: s}, nil
	}
	project, _, err := parseProject(s[:best])
	if err != nil {
		return repoWebArg{}, err
	}
	// A ref is taken as written (refs are never escaped); a URL's path is decoded.
	rest := s[best+len("/-/"+bestPart+"/"):]
	if rest == "" {
		return repoWebArg{}, errors.New("nothing after the " + bestPart + " part")
	}
	return repoWebArg{project: project, part: bestPart, rest: rest, web: true}, nil
}

// repoRefPath is one way to split "<ref>/<path>": a ref name may hold slashes
// (feat/login), so the web path alone does not say where the ref ends.
type repoRefPath struct{ ref, path string }

// refPathSplits lists the splits of rest, shortest ref first; a split whose path
// is empty is kept only when allowEmpty (a tree's root).
func refPathSplits(rest string, allowEmpty bool) []repoRefPath {
	var out []repoRefPath
	for i := 0; i < len(rest); i++ {
		if rest[i] == '/' && i > 0 && i < len(rest)-1 {
			out = append(out, repoRefPath{ref: rest[:i], path: rest[i+1:]})
		}
	}
	if allowEmpty {
		out = append(out, repoRefPath{ref: strings.TrimSuffix(rest, "/")})
	}
	return out
}

// ---------------------------------------------------------------- validation

var (
	// repoRefForbidden are the characters git refuses in a ref name (git check-ref-format).
	repoRefForbidden = regexp.MustCompile(`[\x00-\x20\x7f~^:?*\[\\]`)
	repoHexSHA       = regexp.MustCompile(`^[0-9a-fA-F]{7,64}$`)
)

// repoRefName checks a branch or tag name by git's rules (git check-ref-format),
// so an invalid name is refused before anything is sent and a target key never
// holds a "." or ".." segment.
func repoRefName(raw, flag, what string) (string, error) {
	name := strings.TrimSpace(raw)
	bad := name == "" || name == "@" || repoRefForbidden.MatchString(name) || strings.Contains(name, "..") ||
		strings.Contains(name, "@{") || strings.Contains(name, "//") || strings.HasPrefix(name, "-") ||
		strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/") || strings.HasSuffix(name, ".")
	for _, seg := range strings.Split(name, "/") {
		if strings.HasPrefix(seg, ".") || strings.HasSuffix(seg, ".lock") {
			bad = true
		}
	}
	if bad {
		e := aicli.Validation(flag, "a "+what+" name git accepts: no spaces, ~ ^ : ? * [ \\, '..' or '@{', no leading '-' or '.'",
			"That is not a valid "+what+" name.")
		return "", e
	}
	return name, nil
}

// repoRevision checks a ref read by name: a branch, a tag or a commit SHA.
func repoRevision(raw, flag string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.ContainsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return "", aicli.Validation(flag, "a branch, tag or commit SHA", "That is not a branch, tag or commit SHA.")
	}
	return s, nil
}

// repoFilePath canonicalises a path in the repository: relative, "/" separators, no
// empty, "." or ".." segment, so one file has one spelling (SEC-012). A leading "/"
// or "./" is dropped. dir allows the root ("") and a trailing "/".
func repoFilePath(raw, flag string, dir bool) (string, error) {
	p := strings.TrimSpace(raw)
	p = strings.TrimLeft(strings.TrimPrefix(p, "./"), "/")
	if dir {
		p = strings.TrimSuffix(p, "/")
		if p == "" {
			return "", nil
		}
	}
	bad := p == "" || strings.ContainsFunc(p, unicode.IsControl) || strings.Contains(p, "\\")
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			bad = true
		}
	}
	if bad {
		what := "a file path in the repository, like docs/guide.md"
		if dir {
			what = "a directory path in the repository, like src/api, or nothing for the root"
		}
		return "", aicli.Validation(flag, what, "That is not a path in the repository: use / between names, without '.' or '..' segments.")
	}
	return p, nil
}

// ---------------------------------------------------------------- errors

func repoBranchNotFound(project string) errOpt {
	return onNotFound("The project has no branch with that name, or this account cannot see it.",
		fmt.Sprintf("Run `gitlab-cli repo branch list --project %s` to find the branch.", project))
}

func repoFileNotFound(project string) errOpt {
	return onNotFound("No file at that path on that ref, or the ref does not exist.",
		fmt.Sprintf("Run `gitlab-cli repo tree --project %s --recursive` to find the file; a ref with a / in its name needs --ref.", project))
}

func repoTreeNotFound(project string) errOpt {
	return onNotFound("No directory at that path on that ref, or the ref does not exist.",
		fmt.Sprintf("Run `gitlab-cli repo tree --project %s` from the root, or `gitlab-cli repo branch list --project %s` for the ref.", project, project))
}

func repoCommitNotFound(project string) errOpt {
	return onNotFound("The project has no commit, branch or tag by that SHA or name, or this account cannot see it.",
		fmt.Sprintf("Run `gitlab-cli repo commit list --project %s` to find the commit.", project))
}

// repoEmpty reports whether the project's repository has no commit yet (GitLab's
// empty_repo), as a project `project create` just made. GitLab's first commit
// there creates the branch it names, so a file create, or a commit of new files,
// to a branch that does not exist yet is no error in an empty repository.
func repoEmpty(ctx context.Context, c *api.Client, project string) bool {
	p, err := c.Projects.Get(ctx, project)
	return err == nil && p.EmptyRepo
}

// repoIsNotFound reports a GitLab 404 (or an aicli E_NOT_FOUND).
func repoIsNotFound(err error) bool {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 404
	}
	var ae *aicli.Error
	return errors.As(err, &ae) && ae.Code == "E_NOT_FOUND"
}

// ---------------------------------------------------------------- values

// repoClip cuts s to at most max runes, at the last line end in the second half
// when there is one, and reports whether it cut.
func repoClip(s string, max int) (string, bool) {
	if utf8.RuneCountInString(s) <= max {
		return s, false
	}
	cut := string([]rune(s)[:max])
	if i := strings.LastIndexByte(cut, '\n'); i > len(cut)/2 {
		cut = cut[:i+1]
	}
	return cut, true
}

// repoTextValue is a long text as a preview shows it: itself up to 256 characters,
// its digest above (as preview.inputs shows long values), so previews stay small
// and the read-back compares the same form.
func repoTextValue(s string) any {
	if utf8.RuneCountInString(s) <= 256 {
		return s
	}
	return map[string]any{"sha256": repoSHA256([]byte(s)), "bytes": len(s)}
}

func repoSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// repoTimeFlag reads a --since or --until time: ISO 8601 (2026-06-01 or
// 2026-06-01T08:00:00Z) or relative to now (7d, 24h, 30m), as absolute UTC
// (CLI-028, aicli.ParseTime); "" when the flag is unset.
func repoTimeFlag(cmd *cobra.Command, name string, now time.Time) (string, error) {
	raw, _ := cmd.Flags().GetString(name)
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	t, err := aicli.ParseTime(raw, now)
	if err != nil {
		return "", aicli.Validation("--"+name, "ISO 8601 (2026-06-01 or 2026-06-01T08:00:00Z) or a time ago (7d, 24h, 30m)",
			"--"+name+" must be a date, a date and time, or a time ago like 7d.")
	}
	return t.Format(time.RFC3339), nil
}

// repoNow is the App's clock (App.Now, else the system clock).
func repoNow(app *aicli.App) time.Time {
	if app.Now != nil {
		return app.Now()
	}
	return time.Now()
}

// repoRawFormat reports a run with --format raw (aicli.OutputFormat, Q14).
func repoRawFormat(cmd *cobra.Command) bool { return aicli.OutputFormat(cmd) == "raw" }

// ---------------------------------------------------------------- repo tree

func repoTreeEntryOut(project, ref string, e api.TreeEntry) map[string]any {
	r := repoTreeRef(project, ref, e.Path)
	if e.Type == "blob" {
		r = repoBlobRef(project, ref, e.Path)
	}
	return map[string]any{"ref": r, "path": e.Path, "name": e.Name, "type": e.Type, "mode": e.Mode, "id": e.ID}
}

func addRepoTree(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "tree [<path>]", RunE: func(cmd *cobra.Command, _ []string) error {
		req, err := readPage(cmd)
		if err != nil {
			return err
		}
		raw, _ := cmd.Flags().GetString("path")
		arg, err := parseRepoWebArg(raw, "tree", "blob")
		if err != nil {
			e := aicli.Validation("--path", "a directory path (src/api), its ref (acme/web/-/tree/main/src/api) or web URL",
				"That is not a directory path, ref or URL.")
			return e
		}
		gl := gitlabFor(app)
		project, err := projectFlag(cmd, arg.project, gl)
		if err != nil {
			return err
		}
		cands, err := repoReadSplits(cmd, arg, "ref", true)
		if err != nil {
			return err
		}
		recursive, _ := cmd.Flags().GetBool("recursive")
		client, err := gl.get()
		if err != nil {
			return err
		}
		type treePage struct {
			entries []api.TreeEntry
			pag     api.Pagination
		}
		page, used, err := repoTryRead(cmd.Context(), cands, func(ctx context.Context, at repoRefPath) (treePage, error) {
			entries, pag, err := client.Repos.ListTreePage(ctx, project, api.TreeOpts{Path: at.path, Ref: at.ref,
				Recursive: recursive, Page: req.Page, PerPage: req.Limit})
			return treePage{entries, pag}, err
		})
		if err != nil {
			return readError(err, repoTreeNotFound(project))
		}
		items := make([]map[string]any, 0, len(page.entries))
		for _, e := range page.entries {
			items = append(items, repoTreeEntryOut(project, used.ref, e))
		}
		scope := map[string]any{"project": project, "ref_name": used.ref, "path": used.path, "recursive": recursive}
		return app.List(cmd, gitlabPage(items, len(items), req, page.pag, scope, ""))
	}}
	c.Flags().String("path", "", "Directory: a path (src/api), its ref (acme/web/-/tree/main/src/api) or web URL; the root when empty")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the path is a ref")
	c.Flags().String("ref", "", "Branch, tag or commit SHA (default: the default branch)")
	c.Flags().Bool("recursive", false, "List the whole subtree, not only the directory's own entries")
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "List the files and directories of a project's repository at a ref",
		Aliases: []string{"ls"}, Effects: []aicli.Effect{aicli.Observe}, Output: "tree_entry_page", Positional: "path",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{repoEvidence, liveEvidence("repo")},
		Examples: []string{"gitlab-cli repo tree --project acme/web", "gitlab-cli repo tree src --project acme/web --ref main --recursive",
			"gitlab-cli repo tree acme/web/-/tree/main/src"}})
}

// repoReadSplits is the ref/path pairs a read tries for its path argument: one for a
// plain path (at --ref, or HEAD), every split of a ref or URL (a ref name may hold
// slashes), or the one split --ref gives. dir allows directories (the root too).
func repoReadSplits(cmd *cobra.Command, arg repoWebArg, refFlag string, dir bool) ([]repoRefPath, error) {
	refRaw, _ := cmd.Flags().GetString(refFlag)
	ref := ""
	if strings.TrimSpace(refRaw) != "" {
		r, err := repoRevision(refRaw, "--"+refFlag)
		if err != nil {
			return nil, err
		}
		ref = r
	}
	if !arg.web {
		path, err := repoFilePath(arg.rest, "--path", dir)
		if err != nil {
			return nil, err
		}
		if ref == "" {
			ref = "HEAD"
		}
		return []repoRefPath{{ref: ref, path: path}}, nil
	}
	if ref != "" {
		rest := arg.rest
		if rest != ref && !strings.HasPrefix(rest, ref+"/") {
			e := aicli.Usage("conflicting_flags", "--"+refFlag, "--"+refFlag+" names another ref than the one in the path's ref or URL.")
			e.Details["next_action"] = "Drop --" + refFlag + ", or pass the bare path with it."
			return nil, e
		}
		path, err := repoFilePath(strings.TrimPrefix(strings.TrimPrefix(rest, ref), "/"), "--path", dir)
		if err != nil {
			return nil, err
		}
		return []repoRefPath{{ref: ref, path: path}}, nil
	}
	var out []repoRefPath
	for _, s := range refPathSplits(arg.rest, dir) {
		if p, err := repoFilePath(s.path, "--path", dir); err == nil {
			out = append(out, repoRefPath{ref: s.ref, path: p})
		}
	}
	if len(out) == 0 {
		return nil, aicli.Validation("--path", "a ref like acme/web/-/blob/main/README.md", "The ref or URL names no file path after the branch.")
	}
	return out, nil
}

// repoTryRead runs read for each split until one is not a 404; it returns the split
// that answered, or the last error.
func repoTryRead[T any](ctx context.Context, cands []repoRefPath, read func(context.Context, repoRefPath) (T, error)) (T, repoRefPath, error) {
	var zero T
	var last error
	for _, cand := range cands {
		v, err := read(ctx, cand)
		if err == nil {
			return v, cand, nil
		}
		last = err
		if !repoIsNotFound(err) {
			break
		}
	}
	return zero, repoRefPath{}, last
}
