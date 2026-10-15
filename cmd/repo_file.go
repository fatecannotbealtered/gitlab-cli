package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// repo file: get (metadata), content (the body, paged by lines: a heavy
// sub-resource of its own, CLI-043), and create, update and delete, each one commit
// on a branch. A file is named by its path with --project and --ref (or --branch),
// by its ref (acme/web/-/blob/main/docs/guide.md) or by its web URL.

const (
	repoFileDefaultLines = 200
	repoFileMaxLines     = 2000
	// repoFileBudget is the content one JSON response carries, in characters,
	// so the envelope stays within the success budget (CLI-042).
	repoFileBudget = 6000
)

var repoFileUntrusted = []string{"ref", "path", "name"}

// repoFileOut is a file as file get and the file writes return it.
func repoFileOut(host, project, ref, path string, f *api.RepoFile) map[string]any {
	key := repoBlobRef(project, ref, path)
	return map[string]any{"ref": key, "path": path, "name": f.FileName, "ref_name": ref, "size": f.Size,
		"content_sha256": f.ContentSHA256, "blob_id": f.BlobID, "commit_id": f.CommitID, "last_commit_id": f.LastCommitID,
		"web_url": repoWebURL(host, key)}
}

// addRepoFileFlags declares the flags that name one file; refFlag is "ref" for
// reads and "branch" for writes.
func addRepoFileFlags(c *cobra.Command, refFlag string) {
	c.Flags().String("path", "", "File: a path (docs/guide.md), its ref (acme/web/-/blob/main/docs/guide.md) or web URL")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the path is a ref")
	if refFlag == "ref" {
		c.Flags().String("ref", "", "Branch, tag or commit SHA (default: the default branch)")
		return
	}
	c.Flags().String("branch", "", "Branch the commit goes to; optional when the path is a ref that names it")
}

// repoMissingPath is the usage error of a file command without a path.
func repoMissingPath(cmd *cobra.Command) error {
	e := aicli.Usage("missing_parameter", "--path", fmt.Sprintf("`%s` needs the file: its path, ref or URL.", cmd.CommandPath()))
	e.Details["next_action"] = fmt.Sprintf("Pass the file as the argument, for example `%s README.md --project acme/web`.", cmd.CommandPath())
	return e
}

// repoFileRead reads the file a read command names, trying each ref/path split of
// a ref or URL until one exists.
func repoFileRead(cmd *cobra.Command, gl *lazyClient) (project string, at repoRefPath, f *api.RepoFile, host string, err error) {
	raw, _ := cmd.Flags().GetString("path")
	if strings.TrimSpace(raw) == "" {
		return "", at, nil, "", repoMissingPath(cmd)
	}
	arg, perr := parseRepoWebArg(raw, "blob", "raw")
	if perr != nil {
		return "", at, nil, "", aicli.Validation("--path", "a file path (docs/guide.md), its ref (acme/web/-/blob/main/docs/guide.md) or web URL",
			"That is not a file path, ref or URL.")
	}
	if project, err = projectFlag(cmd, arg.project, gl); err != nil {
		return "", at, nil, "", err
	}
	cands, err := repoReadSplits(cmd, arg, "ref", false)
	if err != nil {
		return "", at, nil, "", err
	}
	client, err := gl.get()
	if err != nil {
		return "", at, nil, "", err
	}
	f, at, err = repoTryRead(cmd.Context(), cands, func(ctx context.Context, c repoRefPath) (*api.RepoFile, error) {
		return client.Repos.GetFile(ctx, project, c.path, c.ref)
	})
	if err != nil {
		return "", at, nil, "", readError(err, repoFileNotFound(project))
	}
	return project, at, f, client.Host(), nil
}

// repoIsBinary reports content that is not text: invalid UTF-8 or a NUL byte.
func repoIsBinary(b []byte) bool { return !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 }

func addRepoFileGet(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "get <path>", RunE: func(cmd *cobra.Command, _ []string) error {
		if err := repoOutputFlagGone(cmd); err != nil {
			return err
		}
		project, at, f, host, err := repoFileRead(cmd, gitlabFor(app))
		if err != nil {
			return err
		}
		if repoRawFormat(cmd) {
			b, err := f.Decoded()
			if err != nil {
				return err
			}
			return app.OK(cmd, b)
		}
		app.Notice(aicli.Notice{Type: "content_not_included", Severity: "info",
			Message: "This is the file's metadata; run `gitlab-cli repo file content` with the same arguments for its content."})
		return app.OK(cmd, repoFileOut(host, project, at.ref, at.path, f))
	}}
	addRepoFileFlags(c, "ref")
	addRepoOutputFlag(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show a file's metadata: size, blob, last commit (--format raw: its bytes)",
		Aliases: []string{"view", "show"}, Effects: []aicli.Effect{aicli.Observe}, Output: "file", Positional: "path",
		RawOutput: true, ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{repoFileEvidence, liveEvidence("repo")},
		Examples: []string{"gitlab-cli repo file get README.md --project acme/web --ref main",
			"gitlab-cli repo file get acme/web/-/blob/main/README.md --format raw"}})
}

func addRepoFileContent(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "content <path>", RunE: func(cmd *cobra.Command, _ []string) error {
		if err := repoOutputFlagGone(cmd); err != nil {
			return err
		}
		offset, _ := cmd.Flags().GetInt("offset")
		limit, _ := cmd.Flags().GetInt("limit")
		if limit < 1 || limit > repoFileMaxLines {
			e := aicli.Usage("invalid_value", "--limit", fmt.Sprintf("--limit must be between 1 and %d lines.", repoFileMaxLines))
			e.Details["expected"] = fmt.Sprintf("1-%d", repoFileMaxLines)
			return e
		}
		if offset < 0 {
			e := aicli.Usage("invalid_value", "--offset", "--offset must be 0 or more: next_offset from the previous call.")
			e.Details["expected"] = "0 or more"
			return e
		}
		if repoRawFormat(cmd) && (cmd.Flags().Changed("offset") || cmd.Flags().Changed("limit")) {
			e := aicli.Usage("conflicting_flags", "--offset", "--format raw prints the whole file; --offset and --limit page the JSON form.")
			e.Details["next_action"] = "Drop --offset and --limit, or drop --format raw."
			return e
		}
		project, at, f, _, err := repoFileRead(cmd, gitlabFor(app))
		if err != nil {
			return err
		}
		data, err := f.Decoded()
		if err != nil {
			return err
		}
		if repoRawFormat(cmd) {
			return app.OK(cmd, data)
		}
		out := map[string]any{"ref": repoBlobRef(project, at.ref, at.path), "path": at.path, "ref_name": at.ref, "size": len(data)}
		if repoIsBinary(data) {
			out["binary"], out["content"] = true, nil
			app.Notice(aicli.Notice{Type: "binary_content", Severity: "info",
				Message: "The file is binary, so its content is not shown as text; add --format raw for its bytes."})
			return app.OK(cmd, out)
		}
		for k, v := range repoFileWindow(string(data), offset, limit) {
			out[k] = v
		}
		if out["truncated"] == true {
			app.Notice(aicli.Notice{Type: "line_truncated", Severity: "warning",
				Message: "A line is longer than one response carries and was cut; add --format raw for the whole file."})
		}
		return app.OK(cmd, out)
	}}
	addRepoFileFlags(c, "ref")
	addRepoOutputFlag(c)
	c.Flags().Int("offset", 0, "Lines to skip: next_offset from the previous call")
	c.Flags().Int("limit", repoFileDefaultLines, fmt.Sprintf("Lines per call, 1-%d (fewer when the lines are long)", repoFileMaxLines))
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Read a file's content a window of lines at a time (--format raw: all of it)",
		Aliases: []string{"raw", "cat", "read"}, Effects: []aicli.Effect{aicli.Observe}, Output: "file_content", Positional: "path",
		RawOutput: true, ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{repoFileEvidence, liveEvidence("repo")},
		Examples: []string{"gitlab-cli repo file content README.md --project acme/web",
			"gitlab-cli repo file content acme/web/-/blob/main/src/app.go --offset 200",
			"gitlab-cli repo file content README.md --project acme/web --format raw"}})
}

// repoFileWindow is the window of lines from offset: at most limit lines and at
// most repoFileBudget characters, with what is needed to continue. A single line
// longer than the budget is cut (truncated).
func repoFileWindow(text string, offset, limit int) map[string]any {
	lines := strings.SplitAfter(text, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	total := len(lines)
	start := min(offset, total)
	var b strings.Builder
	n, used, truncated := 0, 0, false
	for i := start; i < total && n < limit; i++ {
		size := utf8.RuneCountInString(lines[i])
		if used+size > repoFileBudget {
			if n == 0 {
				cut, _ := repoClip(lines[i], repoFileBudget)
				b.WriteString(cut)
				n, truncated = 1, true
			}
			break
		}
		b.WriteString(lines[i])
		used += size
		n++
	}
	out := map[string]any{"binary": false, "content": b.String(), "truncated": truncated, "start_line": start + 1,
		"line_count": n, "total_lines": total, "has_more": start+n < total}
	if start+n < total {
		out["next_offset"] = start + n
	}
	return out
}

// addRepoOutputFlag keeps 1.x's --output, hidden: writing a local file would make
// the read a gated artifact write (CLI-025), so the command refuses it and names the
// replacement.
func addRepoOutputFlag(c *cobra.Command) {
	c.Flags().String("output", "", "Removed in 2.0: use --format raw and redirect stdout")
	_ = c.Flags().MarkHidden("output")
}

func repoOutputFlagGone(cmd *cobra.Command) error {
	if !cmd.Flags().Changed("output") {
		return nil
	}
	e := aicli.Usage("invalid_value", "--output", "This command no longer writes files; it prints the content with --format raw.")
	e.Details["next_action"] = "Run `gitlab-cli repo file content <path> --project <group/project> --format raw` and redirect stdout to the file."
	return e
}

// ---------------------------------------------------------------- writes

// repoFileSel is the file a write commits to.
type repoFileSel struct{ project, branch, path string }

func (s repoFileSel) key() string { return repoBlobRef(s.project, s.branch, s.path) }

// repoFileWriteTarget reads the path argument (or --path), --branch and the project,
// and binds their canonical forms (CLI-054), so a ref, a URL and a path with
// --branch are one request. When a ref's branch could end at more than one "/"
// (feat/login), the branches are looked up before the gate to tell: the one kind of
// read a write may make before it (an input it cannot canonicalise locally).
func repoFileWriteTarget(cmd *cobra.Command, gl *lazyClient) (repoFileSel, error) {
	raw, _ := cmd.Flags().GetString("path")
	if strings.TrimSpace(raw) == "" {
		return repoFileSel{}, repoMissingPath(cmd)
	}
	arg, err := parseRepoWebArg(raw, "blob")
	if err != nil {
		return repoFileSel{}, aicli.Validation("--path", "a file path (docs/guide.md), its ref (acme/web/-/blob/main/docs/guide.md) or web URL",
			"That is not a file path, ref or URL.")
	}
	project, err := projectFlag(cmd, arg.project, gl)
	if err != nil {
		return repoFileSel{}, err
	}
	sel := repoFileSel{project: project}
	branchRaw, _ := cmd.Flags().GetString("branch")
	switch {
	case !arg.web:
		if sel.path, err = repoFilePath(arg.rest, "--path", false); err != nil {
			return sel, err
		}
		if strings.TrimSpace(branchRaw) == "" {
			e := aicli.Usage("missing_parameter", "--branch", fmt.Sprintf("`%s` needs --branch: the branch the commit goes to.", cmd.CommandPath()))
			e.Details["next_action"] = fmt.Sprintf("Add --branch <branch>, or pass the file's ref (acme/web/-/blob/<branch>/<path>) to `%s`.", cmd.CommandPath())
			return sel, e
		}
		if sel.branch, err = repoRefName(branchRaw, "--branch", "branch"); err != nil {
			return sel, err
		}
	case strings.TrimSpace(branchRaw) != "":
		if sel.branch, err = repoRefName(branchRaw, "--branch", "branch"); err != nil {
			return sel, err
		}
		if !strings.HasPrefix(arg.rest, sel.branch+"/") {
			e := aicli.Usage("conflicting_flags", "--branch", "--branch names another branch than the file's ref or URL.")
			e.Details["next_action"] = "Drop --branch, or pass the bare path with it."
			return sel, e
		}
		if sel.path, err = repoFilePath(arg.rest[len(sel.branch)+1:], "--path", false); err != nil {
			return sel, err
		}
	default:
		var cands []repoRefPath
		for _, s := range refPathSplits(arg.rest, false) {
			b, berr := repoRefName(s.ref, "--branch", "branch")
			p, perr := repoFilePath(s.path, "--path", false)
			if berr == nil && perr == nil {
				cands = append(cands, repoRefPath{ref: b, path: p})
			}
		}
		switch len(cands) {
		case 0:
			return sel, aicli.Validation("--path", "a ref like acme/web/-/blob/main/docs/guide.md", "The ref or URL names no branch and file path.")
		case 1:
			sel.branch, sel.path = cands[0].ref, cands[0].path
		default:
			client, err := gl.get()
			if err != nil {
				return sel, err
			}
			found, at, err := repoTryRead(cmd.Context(), cands, func(ctx context.Context, c repoRefPath) (*api.Branch, error) {
				return client.Repos.GetBranch(ctx, project, c.ref)
			})
			if err != nil || found == nil {
				return sel, readError(err, repoBranchNotFound(project))
			}
			sel.branch, sel.path = at.ref, at.path
		}
	}
	if err := aicli.SetResolved(cmd, "path", sel.path); err != nil {
		return sel, err
	}
	if err := aicli.SetResolved(cmd, "branch", sel.branch); err != nil {
		return sel, err
	}
	return sel, nil
}

// repoMessage reads a required commit message flag.
func repoMessage(cmd *cobra.Command, flag string) (string, error) {
	msg, _ := cmd.Flags().GetString(flag)
	if strings.TrimSpace(msg) == "" {
		return "", aicli.Validation("--"+flag, "a commit message", "The commit message is empty.")
	}
	return msg, nil
}

// repoContentInput is the content a file will hold: --content, or --content-file
// read once through aicli so the token binds these bytes (CLI-054). With
// --encoding base64 the value is base64 and is decoded first.
func repoContentInput(app *aicli.App, cmd *cobra.Command) ([]byte, error) {
	var content []byte
	switch {
	case cmd.Flags().Changed("content-file"):
		b, err := app.ReadFileInput(cmd, "content-file")
		if err != nil {
			return nil, err
		}
		content = b
	case cmd.Flags().Changed("content"):
		s, _ := cmd.Flags().GetString("content")
		content = []byte(s)
	default:
		e := aicli.Usage("missing_parameter", "--content", fmt.Sprintf("`%s` needs the file's content: --content or --content-file.", cmd.CommandPath()))
		e.Details["next_action"] = "Add --content <text>, or --content-file <path> (- reads stdin)."
		return nil, e
	}
	if enc, _ := cmd.Flags().GetString("encoding"); enc == "base64" {
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(content)))
		if err != nil {
			return nil, aicli.Validation("--encoding", "base64 content", "--encoding base64 needs base64 content.")
		}
		content = b
	}
	return content, nil
}

// repoEncode is content as GitLab takes it: text when it is UTF-8, base64 otherwise.
func repoEncode(content []byte) (text, encoding string) {
	if repoIsBinary(content) {
		return base64.StdEncoding.EncodeToString(content), "base64"
	}
	return string(content), "text"
}

// repoBranchWritable reads the branch a file write commits to: E_NOT_FOUND when it
// does not exist, and a block when this account cannot push to it.
func repoBranchWritable(ctx context.Context, c *api.Client, project, branch string) ([]aicli.Block, error) {
	b, err := c.Repos.GetBranch(ctx, project, branch)
	if err != nil {
		return nil, readError(err, repoBranchNotFound(project))
	}
	if !b.CanPush {
		return []aicli.Block{{Kind: "cannot_push", Message: "This account cannot push to the branch (it is protected); commit to another branch."}}, nil
	}
	return nil, nil
}

// repoFileReadBack reads a written file back: its digest and size are what the
// write meant to change.
func repoFileReadBack(ctx context.Context, c *api.Client, sel repoFileSel) aicli.Result {
	f, err := c.Repos.GetFile(ctx, sel.project, sel.path, sel.branch)
	if err != nil {
		return aicli.Result{Data: map[string]any{"ref": sel.key(), "path": sel.path, "ref_name": sel.branch}, Untrusted: repoFileUntrusted}
	}
	return aicli.Result{Observed: map[string]any{"content_sha256": f.ContentSHA256, "size": f.Size},
		Data: repoFileOut(c.Host(), sel.project, sel.branch, sel.path, f), Untrusted: repoFileUntrusted}
}

// addRepoFileWriteFlags declares the flags of file create and update.
func addRepoFileWriteFlags(c *cobra.Command) {
	addRepoFileFlags(c, "branch")
	c.Flags().String("content", "", "The file's content")
	c.Flags().String("content-file", "", "Read the content from a file (- reads stdin); binary files are sent as base64")
	c.Flags().String("encoding", "text", "How --content or --content-file is written: text, or base64 (decoded first)")
	c.Flags().String("commit-message", "", "Commit message")
	_ = c.MarkFlagRequired("commit-message")
}

func addRepoFileCreate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "create <path>", RunE: func(cmd *cobra.Command, _ []string) error {
		msg, err := repoMessage(cmd, "commit-message")
		if err != nil {
			return err
		}
		content, err := repoContentInput(app, cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		sel, err := repoFileWriteTarget(cmd, gl)
		if err != nil {
			return err
		}
		sum, size := repoSHA256(content), len(content)
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: sel.key(), Kind: "file"},
			ReadBack: "repo file get " + shellArg(sel.key()),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				blocks, err := repoBranchWritable(ctx, client, sel.project, sel.branch)
				// A missing branch is no error in an empty repository (a new project):
				// GitLab's first commit creates it, and there is no file to find.
				first := false
				if err != nil {
					if !repoIsNotFound(err) || !repoEmpty(ctx, client, sel.project) {
						return aicli.Preview{}, err
					}
					first = true
				}
				action := fmt.Sprintf("Create %s on branch %s of %s in a new commit", sel.path, sel.branch, sel.project)
				changes := []aicli.FieldChange{{Field: "content_sha256", After: sum}, {Field: "size", After: size}}
				if first {
					action = fmt.Sprintf("Create %s in the empty repository of %s: its first commit, which creates branch %s", sel.path, sel.project, sel.branch)
					changes = append(changes, aicli.FieldChange{Field: "ref_name", Note: "the repository is empty: this first commit creates the branch"})
				} else {
					// A repeat of an applied create with the same --idempotency-key never gets
					// here: the gate replays it first (CLI-062).
					switch _, err := client.Repos.GetFile(ctx, sel.project, sel.path, sel.branch); {
					case err == nil:
						blocks = append(blocks, aicli.Block{Kind: "already_exists", Message: "The branch already has a file at this path; use repo file update."})
					case !repoIsNotFound(err):
						return aicli.Preview{}, readError(err)
					}
				}
				return aicli.Preview{Action: action, Changes: changes, CanApply: len(blocks) == 0, BlockedBy: blocks}, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				text, encoding := repoEncode(content)
				err = client.Repos.CreateFile(ctx, sel.project, sel.path, api.FileWriteBody{Branch: sel.branch, Content: text,
					CommitMessage: msg, Encoding: encoding})
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), repoBranchNotFound(sel.project))
				}
				return repoFileReadBack(ctx, client, sel), nil
			},
		})
	}}
	addRepoFileWriteFlags(c)
	c.Flags().String("idempotency-key", "", "Repeat-safe key: repeating the same create with the same key returns the first result")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Create a file on a branch in one new commit",
		FlagAliases: map[string]string{"message": "commit-message"}, // CLI-022, Q11
		Aliases:     []string{"add", "new"}, Effects: []aicli.Effect{aicli.Change}, Output: "file", Positional: "path",
		FileInputs: map[string]string{"content-file": "content"},
		Params:     map[string]aicli.ParamDoc{"encoding": {Enum: []string{"text", "base64"}}},
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{repoFileEvidence, liveEvidence("repo")},
		Examples: []string{"gitlab-cli repo file create docs/setup.md --project acme/web --branch main --content-file setup.md --commit-message \"Add setup guide\" --dry-run",
			"gitlab-cli repo file create docs/setup.md --project acme/web --branch main --content-file setup.md --commit-message \"Add setup guide\" --confirm <confirm_token>"}})
}

func addRepoFileUpdate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "update <path>", RunE: func(cmd *cobra.Command, _ []string) error {
		msg, err := repoMessage(cmd, "commit-message")
		if err != nil {
			return err
		}
		content, err := repoContentInput(app, cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		sel, err := repoFileWriteTarget(cmd, gl)
		if err != nil {
			return err
		}
		sum, size := repoSHA256(content), len(content)
		var lastCommit string // the file's last commit as the latest Preview read it
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: sel.key(), Kind: "file"},
			ReadBack: "repo file get " + shellArg(sel.key()),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				blocks, err := repoBranchWritable(ctx, client, sel.project, sel.branch)
				if err != nil {
					return aicli.Preview{}, err
				}
				cur, err := client.Repos.GetFile(ctx, sel.project, sel.path, sel.branch)
				if err != nil {
					return aicli.Preview{}, readError(err, repoFileNotFound(sel.project))
				}
				lastCommit = cur.LastCommitID
				if cur.ContentSHA256 == sum {
					blocks = append(blocks, aicli.Block{Kind: "no_change", Message: "The file already has this content, so there is nothing to commit."})
				}
				// The file's last commit is the base: a commit to it after the dry-run is
				// state_drift at confirm, and GitLab refuses the update too (last_commit_id).
				return aicli.Preview{Action: fmt.Sprintf("Update %s on branch %s of %s in a new commit", sel.path, sel.branch, sel.project),
					Changes: []aicli.FieldChange{{Field: "content_sha256", Before: cur.ContentSHA256, After: sum},
						{Field: "size", Before: cur.Size, After: size}},
					BaseVersion: cur.LastCommitID, BaseSource: "last_commit_id", CanApply: len(blocks) == 0, BlockedBy: blocks}, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				text, encoding := repoEncode(content)
				err = client.Repos.UpdateFile(ctx, sel.project, sel.path, api.FileWriteBody{Branch: sel.branch, Content: text,
					CommitMessage: msg, Encoding: encoding, LastCommitID: lastCommit})
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), repoFileNotFound(sel.project))
				}
				return repoFileReadBack(ctx, client, sel), nil
			},
		})
	}}
	addRepoFileWriteFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Replace a file's content on a branch in one new commit",
		FlagAliases: map[string]string{"message": "commit-message"}, // CLI-022, Q11
		Aliases:     []string{"edit"}, Effects: []aicli.Effect{aicli.Change}, Output: "file", Positional: "path",
		FileInputs: map[string]string{"content-file": "content"},
		Params:     map[string]aicli.ParamDoc{"encoding": {Enum: []string{"text", "base64"}}},
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{repoFileEvidence, liveEvidence("repo")},
		Examples: []string{"gitlab-cli repo file update README.md --project acme/web --branch main --content-file README.md --commit-message \"Update readme\" --dry-run",
			"gitlab-cli repo file update README.md --project acme/web --branch main --content-file README.md --commit-message \"Update readme\" --confirm <confirm_token>"}})
}

func addRepoFileDelete(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "delete <path>", RunE: func(cmd *cobra.Command, _ []string) error {
		msg, err := repoMessage(cmd, "commit-message")
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		sel, err := repoFileWriteTarget(cmd, gl)
		if err != nil {
			return err
		}
		var lastCommit string
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: sel.key(), Kind: "file"},
			ReadBack: "repo file get " + shellArg(sel.key()),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				blocks, err := repoBranchWritable(ctx, client, sel.project, sel.branch)
				if err != nil {
					return aicli.Preview{}, err
				}
				// A missing file is E_NOT_FOUND at the dry-run, never a success (CLI-029).
				cur, err := client.Repos.GetFile(ctx, sel.project, sel.path, sel.branch)
				if err != nil {
					return aicli.Preview{}, readError(err, repoFileNotFound(sel.project))
				}
				lastCommit = cur.LastCommitID
				return aicli.Preview{Action: fmt.Sprintf("Delete %s from branch %s of %s in a new commit", sel.path, sel.branch, sel.project),
					Changes: []aicli.FieldChange{{Field: "exists", Before: true, After: false,
						Note: "removed from the branch by a new commit; earlier commits keep it"}},
					BaseVersion: cur.LastCommitID, BaseSource: "last_commit_id", CanApply: len(blocks) == 0, BlockedBy: blocks}, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				if err := client.Repos.DeleteFile(ctx, sel.project, sel.path, sel.branch, msg, lastCommit); err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), repoFileNotFound(sel.project))
				}
				// Read back: the file must be gone from the branch.
				switch _, err := client.Repos.GetFile(ctx, sel.project, sel.path, sel.branch); {
				case repoIsNotFound(err):
					return aicli.Result{Observed: map[string]any{"exists": false}, Data: map[string]any{"ref": sel.key(), "deleted": true}}, nil
				case err == nil:
					return aicli.Result{Observed: map[string]any{"exists": true}, Data: map[string]any{"ref": sel.key(), "deleted": false}}, nil
				}
				return aicli.Result{Data: map[string]any{"ref": sel.key(), "deleted": true}}, nil
			},
		})
	}}
	addRepoFileFlags(c, "branch")
	c.Flags().String("commit-message", "", "Commit message")
	_ = c.MarkFlagRequired("commit-message")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Delete a file from a branch in one new commit",
		FlagAliases: map[string]string{"message": "commit-message"}, // CLI-022, Q11
		Aliases:     []string{"rm", "remove"}, Effects: []aicli.Effect{aicli.Change}, Dangerous: true, Output: "file_deleted", Positional: "path",
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Experimental, Evidence: []string{repoFileEvidence},
		Examples: []string{"gitlab-cli repo file delete docs/old.md --project acme/web --branch main --commit-message \"Remove old doc\" --dangerous --dry-run",
			"gitlab-cli repo file delete docs/old.md --project acme/web --branch main --commit-message \"Remove old doc\" --dangerous --confirm <confirm_token>"}})
}
