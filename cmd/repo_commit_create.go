package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// repo commit create: one atomic commit of many file actions, GitLab's native bulk
// endpoint (POST /repository/commits actions[]), through App.WriteBatch with one
// target per file the commit touches and BatchSpec.ApplyAll sending them in one
// request (CLI-063, Q12). A plan that deletes a file is dangerous (Preview.Dangerous,
// Q13), as repo file delete is.

const (
	repoCommitActionsMax = 1000     // GitLab's cap on one commit's actions
	repoActionFileMax    = 64 << 20 // bytes of one action's content_file
)

// repoAction is one parsed --action of commit create.
type repoAction struct {
	kind, path, previous string // create, update, delete or move
	content              []byte
	hasContent           bool
}

// canonical is the action as the token binds it: the content's digest instead of
// the inline text or the file it came from (CLI-054), so content= and content_file=
// with the same bytes are one request, and a file changed after the dry-run is a
// token_mismatch.
func (a repoAction) canonical() string {
	s := a.kind + ":path=" + a.path
	if a.previous != "" {
		s += ";previous_path=" + a.previous
	}
	if a.hasContent {
		s += fmt.Sprintf(";content_sha256=%s;bytes=%d", repoSHA256(a.content), len(a.content))
	}
	return s
}

// repoActionError is the usage error of a malformed --action; it never echoes the value.
func repoActionError(i int, message string) error {
	e := aicli.Usage("invalid_value", "--action", fmt.Sprintf("--action number %d: %s", i+1, message))
	e.Details["expected"] = "type:path=<path>[;previous_path=<path>][;content=<text>|;content_file=<file>][;encoding=base64], type create, update, delete or move"
	return e
}

// parseRepoActions parses the --action values ('type:field=value;field=value'). A
// content_file is read here, once: its bytes are bound (canonical) and sent.
func parseRepoActions(specs []string) ([]repoAction, error) {
	if len(specs) > repoCommitActionsMax {
		e := aicli.Usage("input_too_large", "--action", fmt.Sprintf("One commit takes at most %d actions (GitLab's limit).", repoCommitActionsMax))
		e.Details["next_action"] = "Split the change into several commits."
		return nil, e
	}
	out := make([]repoAction, 0, len(specs))
	touched := map[string]bool{}
	for i, spec := range specs {
		kind, rest, ok := strings.Cut(spec, ":")
		kind = strings.TrimSpace(kind)
		if !ok || (kind != "create" && kind != "update" && kind != "delete" && kind != "move") {
			return nil, repoActionError(i, "the type is not create, update, delete or move.")
		}
		fields := map[string]string{}
		for _, part := range strings.Split(rest, ";") {
			if strings.TrimSpace(part) == "" {
				continue
			}
			k, v, ok := strings.Cut(part, "=")
			k = strings.TrimSpace(k)
			if !ok || (k != "path" && k != "previous_path" && k != "content" && k != "content_file" && k != "encoding") {
				return nil, repoActionError(i, "a field is not path, previous_path, content, content_file or encoding.")
			}
			fields[k] = v
		}
		a := repoAction{kind: kind}
		var err error
		if a.path, err = repoFilePath(fields["path"], "--action", false); err != nil {
			return nil, repoActionError(i, "path is missing or not a path in the repository.")
		}
		if kind == "move" {
			if a.previous, err = repoFilePath(fields["previous_path"], "--action", false); err != nil {
				return nil, repoActionError(i, "a move needs previous_path, a path in the repository.")
			}
		} else if _, has := fields["previous_path"]; has {
			return nil, repoActionError(i, "previous_path belongs to a move.")
		}
		_, inline := fields["content"]
		file, fromFile := fields["content_file"]
		switch {
		case inline && fromFile:
			return nil, repoActionError(i, "give content or content_file, not both.")
		case kind == "delete" && (inline || fromFile):
			return nil, repoActionError(i, "a delete takes no content.")
		case (kind == "create" || kind == "update") && !inline && !fromFile:
			return nil, repoActionError(i, "a create or update needs content or content_file.")
		case fromFile:
			b, err := repoReadActionFile(file)
			if err != nil {
				return nil, repoActionError(i, err.Error())
			}
			a.content, a.hasContent = b, true
		case inline:
			a.content, a.hasContent = []byte(fields["content"]), true
		}
		switch strings.TrimSpace(fields["encoding"]) {
		case "", "text":
		case "base64":
			b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(a.content)))
			if !a.hasContent || err != nil {
				return nil, repoActionError(i, "encoding=base64 needs base64 content.")
			}
			a.content = b
		default:
			return nil, repoActionError(i, "encoding is text or base64.")
		}
		for _, p := range []string{a.path, a.previous} {
			if p == "" {
				continue
			}
			if touched[p] {
				return nil, repoActionError(i, "each path may appear in one action only.")
			}
			touched[p] = true
		}
		out = append(out, a)
	}
	return out, nil
}

// repoReadActionFile reads an action's content_file once. Stdin is not offered: one
// command can read it once, and several actions could name it.
func repoReadActionFile(path string) ([]byte, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "-" {
		return nil, fmt.Errorf("content_file needs a file path (stdin is not read for actions)")
	}
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return nil, fmt.Errorf("content_file cannot be read")
	case info.IsDir():
		return nil, fmt.Errorf("content_file names a directory")
	case info.Size() > repoActionFileMax:
		return nil, fmt.Errorf("content_file is larger than 64 MiB")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("content_file cannot be read")
	}
	return b, nil
}

// repoCommitItem is one target of commit create: the file an action touches. A move
// touches two, its old path ("from") and its new one ("to"), and the scope must
// cover both.
type repoCommitItem struct {
	act  *repoAction
	role string // "" or, for a move, "from" or "to"
	path string
}

// repoCommitRun is the state one run of commit create shares across its items: the
// branch check, the files' last commits as the latest previews read them, and the
// one request every item's Apply reports on (the commit is atomic).
type repoCommitRun struct {
	project, branch, start, message string
	actions                         []repoAction

	checked   bool
	branchErr error
	readRef   string // where the files are read: the branch, or --start-branch for a new branch
	empty     bool   // an empty repository: this first commit creates the branch, no file exists
	blocks    []aicli.Block
	last      map[string]string // path -> last_commit_id

	sent     bool
	applyErr error
	commit   *api.Commit
	observed map[string]map[string]any // path -> what the commit's diff shows
}

// onlyCreates reports whether every action creates a file: the only commit an empty
// repository can take, since it has no file to update, delete or move.
func (r *repoCommitRun) onlyCreates() bool {
	for _, a := range r.actions {
		if a.kind != "create" {
			return false
		}
	}
	return len(r.actions) > 0
}

// branchState checks the branch once per run.
func (r *repoCommitRun) branchState(ctx context.Context, c *api.Client) error {
	if r.checked {
		return r.branchErr
	}
	r.checked = true
	b, err := c.Repos.GetBranch(ctx, r.project, r.branch)
	switch {
	case err == nil:
		r.readRef = r.branch
		if !b.CanPush {
			r.blocks = []aicli.Block{{Kind: "cannot_push", Message: "This account cannot push to the branch (it is protected); commit to another branch."}}
		}
	case repoIsNotFound(err) && r.start != "":
		if _, err := c.Repos.GetBranch(ctx, r.project, r.start); err != nil {
			r.branchErr = readError(err, onNotFound("--start-branch names no branch in the project.",
				fmt.Sprintf("Run `gitlab-cli repo branch list --project %s` to find the branch to start from.", r.project)))
			return r.branchErr
		}
		r.readRef = r.start
	case repoIsNotFound(err) && r.onlyCreates() && repoEmpty(ctx, c, r.project):
		// An empty repository (a new project): GitLab's first commit creates the
		// branch, and there is no file to read.
		r.readRef, r.empty = r.branch, true
	case repoIsNotFound(err):
		r.branchErr = readError(err, onNotFound("The branch does not exist; --start-branch creates it from another branch.",
			fmt.Sprintf("Add --start-branch <branch>, or run `gitlab-cli repo branch list --project %s`.", r.project)))
	default:
		r.branchErr = readError(err)
	}
	return r.branchErr
}

// preview plans one item.
func (r *repoCommitRun) preview(ctx context.Context, c *api.Client, it repoCommitItem) (aicli.Preview, error) {
	if err := r.branchState(ctx, c); err != nil {
		return aicli.Preview{}, err
	}
	pv := aicli.Preview{Action: fmt.Sprintf("Commit file changes to branch %s of %s", r.branch, r.project),
		BlockedBy: append([]aicli.Block(nil), r.blocks...)}
	var cur *api.RepoFile
	var err error
	if r.empty {
		pv.Action = fmt.Sprintf("Commit files to the empty repository of %s: its first commit, which creates branch %s", r.project, r.branch)
	} else {
		cur, err = c.Repos.GetFile(ctx, r.project, it.path, r.readRef)
		switch {
		case err == nil:
		case repoIsNotFound(err):
			cur = nil
		default:
			return aicli.Preview{}, readError(err)
		}
	}
	a := it.act
	mustExist := a.kind == "update" || a.kind == "delete" || it.role == "from"
	switch {
	case mustExist && cur == nil:
		return aicli.Preview{}, readError(err, repoFileNotFound(r.project))
	case !mustExist && cur != nil:
		pv.BlockedBy = append(pv.BlockedBy, aicli.Block{Kind: "already_exists", Message: "The branch already has a file at this path."})
	}
	if mustExist {
		r.last[it.path] = cur.LastCommitID
		pv.BaseVersion, pv.BaseSource = cur.LastCommitID, "last_commit_id"
	}
	pv.Changes = []aicli.FieldChange{{Field: "action", After: a.kind}}
	pv.Dangerous = a.kind == "delete" // removes a file, as repo file delete does (Q13)
	switch it.role {
	case "from":
		pv.Changes = append(pv.Changes, aicli.FieldChange{Field: "moved_to", After: a.path})
	case "to":
		pv.Changes = append(pv.Changes, aicli.FieldChange{Field: "moved_from", After: a.previous})
	}
	if a.hasContent && it.role != "from" {
		ch := aicli.FieldChange{Field: "content_sha256", After: repoSHA256(a.content)}
		if a.kind == "update" {
			ch.Before = cur.ContentSHA256
			if cur.ContentSHA256 == ch.After {
				pv.BlockedBy = append(pv.BlockedBy, aicli.Block{Kind: "no_change", Message: "The file already has this content."})
			}
		}
		pv.Changes = append(pv.Changes, ch)
	}
	pv.CanApply = len(pv.BlockedBy) == 0
	return pv, nil
}

// apply sends the commit, the batch's one native call (BatchSpec.ApplyAll), and
// reads its diff back; every item reports the one outcome (CLI-063: the upstream's
// atomicity, not more).
func (r *repoCommitRun) apply(ctx context.Context, gl *lazyClient) {
	if r.sent {
		return
	}
	r.sent = true
	c, err := gl.get()
	if err != nil {
		r.applyErr = aicli.NotDispatched(err)
		return
	}
	body := api.CommitCreateOpts{Branch: r.branch, CommitMessage: r.message}
	if r.readRef != r.branch {
		body.StartBranch = r.readRef
	}
	for _, a := range r.actions {
		act := api.CommitAction{Action: a.kind, FilePath: a.path, PreviousPath: a.previous}
		if a.hasContent {
			act.Content, act.Encoding = repoEncode(a.content)
		}
		switch a.kind {
		case "update", "delete":
			act.LastCommitID = r.last[a.path]
		case "move":
			act.LastCommitID = r.last[a.previous]
		}
		body.Actions = append(body.Actions, act)
	}
	commit, err := c.Repos.CreateCommit(ctx, r.project, body)
	if err != nil {
		r.applyErr = classifyWriteError(err, requiresRole("developer"), repoBranchNotFound(r.project))
		return
	}
	r.commit = commit
	files, _, err := c.Repos.ListCommitDiff(ctx, r.project, commit.ID, repoCommitActionsMax+1)
	if err != nil {
		return // the commit stands; its items report no read-back (verification not_run)
	}
	r.observed = map[string]map[string]any{}
	for _, f := range files {
		switch {
		case f.NewFile:
			r.observed[f.NewPath] = map[string]any{"action": "create"}
		case f.DeletedFile:
			r.observed[f.OldPath] = map[string]any{"action": "delete"}
		case f.RenamedFile:
			r.observed[f.NewPath] = map[string]any{"action": "move", "moved_from": f.OldPath}
			r.observed[f.OldPath] = map[string]any{"action": "move", "moved_to": f.NewPath}
		default:
			r.observed[f.NewPath] = map[string]any{"action": "update"}
		}
	}
}

func addRepoCommitCreate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "create", RunE: func(cmd *cobra.Command, _ []string) error {
		msg, err := repoMessage(cmd, "message")
		if err != nil {
			return err
		}
		specs, _ := cmd.Flags().GetStringArray("action")
		actions, err := parseRepoActions(specs)
		if err != nil {
			return err
		}
		branchRaw, _ := cmd.Flags().GetString("branch")
		branch, err := repoRefName(branchRaw, "--branch", "branch")
		if err != nil {
			return err
		}
		start := ""
		if s, _ := cmd.Flags().GetString("start-branch"); strings.TrimSpace(s) != "" {
			if start, err = repoRefName(s, "--start-branch", "branch"); err != nil {
				return err
			}
		}
		gl := gitlabFor(app)
		project, err := projectFlag(cmd, "", gl)
		if err != nil {
			return err
		}
		canon := make([]string, len(actions))
		for i, a := range actions {
			canon[i] = a.canonical()
		}
		if len(canon) > 0 {
			if err := aicli.SetResolved(cmd, "action", canon...); err != nil {
				return err
			}
		}
		run := &repoCommitRun{project: project, branch: branch, start: start, message: msg, actions: actions, last: map[string]string{}}
		var items []aicli.BatchItem
		var targets []repoCommitItem // the item behind each batch item, in order
		for i := range actions {
			a := &actions[i]
			its := []repoCommitItem{{act: a, path: a.path}}
			if a.kind == "move" {
				its = []repoCommitItem{{act: a, role: "from", path: a.previous}, {act: a, role: "to", path: a.path}}
			}
			for _, it := range its {
				it := it
				targets = append(targets, it)
				items = append(items, aicli.BatchItem{
					Target: aicli.Target{Key: repoBlobRef(project, branch, it.path), Kind: "file"},
					Preview: func(ctx context.Context) (aicli.Preview, error) {
						client, err := gl.get()
						if err != nil {
							return aicli.Preview{}, err
						}
						return run.preview(ctx, client, it)
					},
				})
			}
		}
		// One request carries every action (Q12): GitLab applies all or none, and each
		// item reports that outcome with its own read-back.
		applyAll := func(ctx context.Context, ops []aicli.Op) ([]aicli.ItemOutcome, error) {
			run.apply(ctx, gl)
			if run.applyErr != nil {
				return nil, run.applyErr
			}
			outs := make([]aicli.ItemOutcome, len(targets))
			for i, it := range targets {
				key := repoBlobRef(project, branch, it.path)
				data := map[string]any{"ref": key, "path": it.path, "action": it.act.kind,
					"commit_ref": repoCommitRef(project, run.commit.ID), "commit_sha": run.commit.ID}
				res := aicli.Result{Data: data, Untrusted: []string{"ref", "path"}}
				if run.observed != nil {
					res.Observed = run.observed[it.path]
					if res.Observed == nil {
						res.Observed = map[string]any{"action": "none"}
					}
				}
				outs[i] = aicli.ItemOutcome{Result: res}
			}
			return outs, nil
		}
		return app.WriteBatch(cmd, aicli.BatchSpec{Items: items, ApplyAll: applyAll})
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	c.Flags().String("branch", "", "Branch the commit goes to")
	_ = c.MarkFlagRequired("branch")
	c.Flags().String("message", "", "Commit message")
	_ = c.MarkFlagRequired("message")
	c.Flags().String("start-branch", "", "Create --branch from this branch when it does not exist yet")
	c.Flags().StringArray("action", nil, "A file change, repeatable: 'create:path=docs/a.md;content=hello', 'update:path=README.md;content_file=README.md', 'delete:path=old.txt', 'move:path=new.go;previous_path=old.go'")
	_ = c.MarkFlagRequired("action")
	c.Flags().String("idempotency-key", "", "Repeat-safe key: repeating the same commit with the same key returns the first result")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Commit file creates, updates, deletes and moves to a branch atomically",
		FlagAliases: map[string]string{"commit-message": "message"}, // CLI-022, Q11
		Aliases:     []string{"add", "new"}, Effects: []aicli.Effect{aicli.Change}, Output: "commit_action", BatchFlag: "action",
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{repoCommitEvidence, liveEvidence("repo")},
		Examples: []string{"gitlab-cli repo commit create --project acme/web --branch main --message \"Sync docs\" --action 'create:path=docs/a.md;content=hello' --action 'delete:path=docs/old.md' --dry-run",
			"gitlab-cli repo commit create --project acme/web --branch main --message \"Sync docs\" --action 'create:path=docs/a.md;content=hello' --action 'delete:path=docs/old.md' --confirm <confirm_token>"}})
}
