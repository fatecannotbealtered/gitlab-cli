package cmd

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// The discussions (threads) of a merge request: list, get, reply, create (an inline
// review comment on a diff line), resolve and unresolve. A thread is a heavy
// sub-resource (CLI-043): list returns bounded summaries (who opened it, an excerpt,
// where it is anchored, whether it is resolved), get returns the whole thread. A
// thread's ref is its merge request's, its collection and ID,
// acme/web!5/discussions/<id>; GitLab's anchor (#discussion_<id>) is accepted too.

const mrDiscussionEvidence = "simulated:cmd/mr_discussion_test.go"

var (
	mrDiscussionFields = []string{"ref", "id", "individual_note", "resolvable", "resolved", "note_count", "author", "excerpt",
		"path", "line", "created_at", "updated_at", "notes"}
	mrDiscussionDefaults  = []string{"ref", "resolvable", "resolved", "note_count", "author", "excerpt", "path", "line", "updated_at"}
	mrDiscussionUntrusted = []string{"author", "excerpt", "path", "notes[].author", "notes[].body"}
	mrDiscussionID        = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

func registerMRDiscussion(app *aicli.App, parent *cobra.Command) {
	group := &cobra.Command{Use: "discussion", Aliases: []string{"discussions", "thread", "threads"}}
	parent.AddCommand(group)
	app.Group(group, "List and read the discussion threads of a merge request, reply to them, start inline review threads, resolve them")
	app.Schema("merge_request_discussion", aicli.SchemaDef{Shape: "object", Fields: mrDiscussionFields, Untrusted: mrDiscussionUntrusted})
	app.Schema("merge_request_discussion_page", aicli.SchemaDef{Shape: "page", Fields: mrDiscussionFields,
		DefaultFields: mrDiscussionDefaults, Untrusted: mrDiscussionUntrusted})
	addMRDiscussionList(app, group)
	addMRDiscussionGet(app, group)
	addMRDiscussionReply(app, group)
	addMRDiscussionCreate(app, group)
	addMRDiscussionResolve(app, group, true)
	addMRDiscussionResolve(app, group, false)
}

func mrDiscussionRef(sel mrSel, id string) string { return sel.ref() + "/discussions/" + id }

// mrDiscussionResolved is GitLab's resolved state of a thread: resolvable, and
// every resolvable note resolved.
func mrDiscussionResolved(d api.MergeRequestDiscussion) (resolvable, resolved bool) {
	resolved = true
	for _, n := range d.Notes {
		if n.Resolvable {
			resolvable = true
			resolved = resolved && n.Resolved
		}
	}
	return resolvable, resolvable && resolved
}

// mrDiscussionOut is a thread: a summary, with every note when notes is true.
func mrDiscussionOut(sel mrSel, d api.MergeRequestDiscussion, notes bool) map[string]any {
	resolvable, resolved := mrDiscussionResolved(d)
	out := map[string]any{"ref": mrDiscussionRef(sel, d.ID), "id": d.ID, "individual_note": d.IndividualNote,
		"resolvable": resolvable, "resolved": resolved, "note_count": len(d.Notes), "author": nil, "excerpt": "",
		"path": nil, "line": nil, "created_at": nil, "updated_at": nil}
	if len(d.Notes) > 0 {
		first := d.Notes[0]
		out["author"], out["excerpt"], out["created_at"] = mrUsername(first.Author), mrExcerpt(first.Body, 200), utcTime(first.CreatedAt)
		if p := first.Position; p != nil {
			out["path"], out["line"] = mrPositionAt(p)
		}
		latest := ""
		for _, n := range d.Notes {
			if n.UpdatedAt > latest {
				latest = n.UpdatedAt
			}
		}
		out["updated_at"] = utcTime(latest)
	}
	if notes {
		list := make([]map[string]any, 0, len(d.Notes))
		for _, n := range d.Notes {
			list = append(list, mrNoteOut(sel, n))
		}
		out["notes"] = list
	}
	return out
}

// mrPositionAt is where a diff position points: the new path and line, else the
// old ones (a removed line).
func mrPositionAt(p *api.DiffPosition) (path, line any) {
	path, line = nil, nil
	switch {
	case p.NewPath != "":
		path = p.NewPath
	case p.OldPath != "":
		path = p.OldPath
	}
	switch {
	case p.NewLine != nil:
		line = *p.NewLine
	case p.OldLine != nil:
		line = *p.OldLine
	}
	return path, line
}

// mrSystemThread reports a thread of GitLab's own notes ("added 1 commit").
func mrSystemThread(d api.MergeRequestDiscussion) bool {
	for _, n := range d.Notes {
		if !n.System {
			return false
		}
	}
	return len(d.Notes) > 0
}

// mrDiscussionTarget reads the merge request and the thread: the argument may be
// the thread's ref (acme/web!5/discussions/<id>), else --discussion-id names it. The
// ID is bound as --discussion-id either way (CLI-054).
func mrDiscussionTarget(cmd *cobra.Command, gl *lazyClient) (mrSel, string, error) {
	sel, rest, err := mrTarget(cmd, gl)
	if err != nil {
		return mrSel{}, "", err
	}
	id, _ := cmd.Flags().GetString("discussion-id")
	id = strings.TrimSpace(id)
	if fromRef, ok := strings.CutPrefix(rest, "#discussion_"); ok {
		if id != "" && id != fromRef {
			e := aicli.Usage("conflicting_flags", "--discussion-id", "--discussion-id names another thread than the ref given as the argument.")
			e.Details["next_action"] = "Give the thread once: as its ref (acme/web!5/discussions/<id>), or as the merge request and --discussion-id."
			return mrSel{}, "", e
		}
		id = fromRef
		if err := aicli.SetResolved(cmd, "discussion-id", id); err != nil {
			return mrSel{}, "", err
		}
	}
	if id == "" {
		e := aicli.Usage("missing_parameter", "--discussion-id", fmt.Sprintf("`%s` needs the thread: its ref (acme/web!5/discussions/<id>), or --discussion-id.", cmd.CommandPath()))
		e.Details["next_action"] = fmt.Sprintf("Run `gitlab-cli mr discussion list %s` to find the thread's ref.", sel.ref())
		return mrSel{}, "", e
	}
	if !mrDiscussionID.MatchString(id) {
		return mrSel{}, "", aicli.Validation("--discussion-id", "a discussion ID (letters and digits)", "That is not a discussion ID.")
	}
	return sel, id, nil
}

func addMRDiscussionTargetFlags(c *cobra.Command) {
	c.Flags().String("mr", "", "The thread's ref (acme/web!5/discussions/<id>); or the merge request (iid, ref or URL) with --discussion-id")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the argument is a ref or URL")
	c.Flags().String("discussion-id", "", "Thread ID, when the argument names only the merge request")
}

// mrDiscussionNotFound is the 404 of a thread read.
func mrDiscussionNotFound(sel mrSel) errOpt {
	return onNotFound("The merge request has no thread with that ID, or this account cannot see it.",
		fmt.Sprintf("Run `gitlab-cli mr discussion list %s` to find the thread.", sel.ref()))
}

// ---------------------------------------------------------------- list, get

func addMRDiscussionList(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "list <mr>", RunE: func(cmd *cobra.Command, _ []string) error {
		req, err := readPage(cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		sel, _, err := mrTarget(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		ds, pag, err := client.MergeRequests.DiscussionsPage(cmd.Context(), sel.project, sel.iid, req.Page, req.Limit)
		if err != nil {
			return readError(err, mrNotFound(sel.project))
		}
		items := []map[string]any{}
		for _, d := range ds {
			if !mrSystemThread(d) {
				items = append(items, mrDiscussionOut(sel, d, false))
			}
		}
		pag.HasTotal = false // GitLab's total counts the system threads left out
		scope := map[string]any{"mr": sel.ref(),
			"covers": "threads and comments people wrote, oldest first, as summaries; threads of GitLab system notes are left out of each page, so a page can hold fewer than --limit"}
		return app.List(cmd, gitlabPage(items, len(items), req, pag, scope, "created_at_asc"))
	}}
	addMRTargetFlags(c)
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "List a merge request's discussion threads as summaries: author, excerpt, file and line, resolved or not",
		Aliases: []string{"ls"}, Effects: []aicli.Effect{aicli.Observe}, Output: "merge_request_discussion_page", Positional: "mr",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{mrDiscussionEvidence, liveEvidence("mr")},
		Examples: []string{"gitlab-cli mr discussion list acme/web!5", "gitlab-cli mr discussion list acme/web!5 --fields +id"}})
}

func addMRDiscussionGet(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "get <discussion>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, id, err := mrDiscussionTarget(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		d, err := client.MergeRequests.GetDiscussion(cmd.Context(), sel.project, sel.iid, id)
		if err != nil {
			return readError(err, mrDiscussionNotFound(sel))
		}
		return app.OK(cmd, mrDiscussionOut(sel, *d, true))
	}}
	addMRDiscussionTargetFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Show one discussion thread with all its notes",
		Aliases: []string{"view", "show"}, Effects: []aicli.Effect{aicli.Observe}, Output: "merge_request_discussion", Positional: "mr",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{mrDiscussionEvidence, liveEvidence("mr")},
		Examples: []string{"gitlab-cli mr discussion get acme/web!5/discussions/6a9c1750b37d513a43987b574953fceb50b03ce7"}})
}

// ---------------------------------------------------------------- reply

func addMRDiscussionReply(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "reply <discussion>", RunE: func(cmd *cobra.Command, _ []string) error {
		body, err := mrBody(app, cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		sel, id, err := mrDiscussionTarget(cmd, gl)
		if err != nil {
			return err
		}
		ref := mrDiscussionRef(sel, id)
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: ref, Kind: "merge_request_discussion"},
			ReadBack: "mr discussion get " + ref,
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				c, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				if _, err := c.MergeRequests.GetDiscussion(ctx, sel.project, sel.iid, id); err != nil {
					return aicli.Preview{}, readError(err, mrDiscussionNotFound(sel))
				}
				pv := aicli.Preview{Action: "Reply to thread " + ref, CanApply: true, Changes: []aicli.FieldChange{{Field: "body", After: body}}}
				if hasQuickAction(body) {
					pv.CanApply, pv.BlockedBy = false, []aicli.Block{quickActionBlock("reply")}
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				c, err := mrApplyClient(gl)
				if err != nil {
					return aicli.Result{}, err
				}
				n, err := c.MergeRequests.ReplyDiscussion(ctx, sel.project, sel.iid, id, body)
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("guest"), mrDiscussionNotFound(sel))
				}
				return aicli.Result{Observed: map[string]any{"ref": mrNoteRef(sel, n.ID), "body": mrText(n.Body)},
					Data: mrNoteOut(sel, *n), Untrusted: []string{"author", "body"}}, nil
			},
		})
	}}
	addMRDiscussionTargetFlags(c)
	addMRBodyFlags(c)
	c.Flags().String("idempotency-key", "", "Repeat-safe key: repeating the same reply with the same key returns the first result")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Reply to a discussion thread of a merge request",
		Effects: []aicli.Effect{aicli.Change}, Output: "merge_request_note", Positional: "mr",
		FileInputs: map[string]string{"body-file": "body"},
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{mrDiscussionEvidence, liveEvidence("mr")},
		Examples: []string{`gitlab-cli mr discussion reply acme/web!5/discussions/6a9c1750b37d --body "Fixed in the last commit." --dry-run`,
			`gitlab-cli mr discussion reply acme/web!5/discussions/6a9c1750b37d --body "Fixed in the last commit." --confirm <confirm_token>`}})
}

// ---------------------------------------------------------------- create

// mrPositionWant is where an inline comment goes: a file and a line of the diff,
// and the diff version (SHAs) when the agent pins one.
type mrPositionWant struct {
	newPath, oldPath           string
	newLine, oldLine           int
	baseSHA, startSHA, headSHA string
}

func readMRPosition(cmd *cobra.Command) (mrPositionWant, error) {
	f := cmd.Flags()
	var p mrPositionWant
	p.newPath, _ = f.GetString("new-path")
	p.oldPath, _ = f.GetString("old-path")
	p.newPath, p.oldPath = strings.TrimSpace(p.newPath), strings.TrimSpace(p.oldPath)
	p.newLine, _ = f.GetInt("new-line")
	p.oldLine, _ = f.GetInt("old-line")
	p.baseSHA, _ = f.GetString("base-sha")
	p.startSHA, _ = f.GetString("start-sha")
	p.headSHA, _ = f.GetString("head-sha")
	if p.newPath == "" && p.oldPath == "" {
		e := aicli.Usage("missing_parameter", "--new-path", "An inline comment needs the file: --new-path (or --old-path for a removed file).")
		e.Details["next_action"] = "Add --new-path <file> and --new-line <n>; a comment on the whole merge request is `mr comment add`."
		return p, e
	}
	if p.newLine < 0 || p.oldLine < 0 {
		return p, aicli.Validation("--new-line", "a line number from 1", "Line numbers start at 1.")
	}
	if p.newLine == 0 && p.oldLine == 0 {
		e := aicli.Usage("missing_parameter", "--new-line", "An inline comment needs the line: --new-line (or --old-line for a removed line).")
		e.Details["next_action"] = "Add --new-line <n>, the line's number in the new version of the file (from `mr diff --fields +diff`)."
		return p, e
	}
	// One path covers a file that was not renamed: GitLab wants both.
	if p.oldPath == "" {
		p.oldPath = p.newPath
	}
	if p.newPath == "" {
		p.newPath = p.oldPath
	}
	return p, nil
}

func addMRDiscussionCreate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "create <mr>", RunE: func(cmd *cobra.Command, _ []string) error {
		body, err := mrBody(app, cmd)
		if err != nil {
			return err
		}
		want, err := readMRPosition(cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		sel, _, err := mrTarget(cmd, gl)
		if err != nil {
			return err
		}
		var pos api.DiffPosition
		line := want.newLine
		if line == 0 {
			line = want.oldLine
		}
		return app.Write(cmd, aicli.WriteSpec{
			Target:   mrTargetOf(sel),
			ReadBack: "mr discussion list " + sel.ref(),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				_, cur, err := mrRead(ctx, gl, sel)
				if err != nil {
					return aicli.Preview{}, err
				}
				// The diff version: the SHAs pinned by the agent, else the MR's current
				// diff refs. The head SHA is the base version, so a push between the
				// dry-run and the confirm is state_drift (CLI-057).
				pos = api.DiffPosition{BaseSHA: want.baseSHA, StartSHA: want.startSHA, HeadSHA: want.headSHA, PositionType: "text",
					NewPath: want.newPath, OldPath: want.oldPath}
				if cur.DiffRefs != nil {
					pos.BaseSHA = mrFirst(pos.BaseSHA, cur.DiffRefs.BaseSHA)
					pos.StartSHA = mrFirst(pos.StartSHA, cur.DiffRefs.StartSHA)
					pos.HeadSHA = mrFirst(pos.HeadSHA, cur.DiffRefs.HeadSHA)
				}
				if want.newLine > 0 {
					pos.NewLine = ptr(want.newLine)
				}
				if want.oldLine > 0 {
					pos.OldLine = ptr(want.oldLine)
				}
				pv := aicli.Preview{Action: "Start an inline review thread on merge request " + sel.ref(), CanApply: true,
					Changes: sortChanges([]aicli.FieldChange{{Field: "body", After: body}, {Field: "path", After: want.newPath},
						{Field: "line", After: line}}),
					BaseVersion: pos.HeadSHA, BaseSource: "head_sha"}
				var blocks []aicli.Block
				if pos.BaseSHA == "" || pos.HeadSHA == "" {
					blocks = append(blocks, aicli.Block{Kind: "diff_not_ready",
						Message: "GitLab has not computed the merge request's diff yet (no base or head SHA); try again shortly, or pass --base-sha and --head-sha."})
				}
				if hasQuickAction(body) {
					blocks = append(blocks, quickActionBlock("comment"))
				}
				if len(blocks) > 0 {
					pv.CanApply, pv.BlockedBy = false, blocks
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				c, err := mrApplyClient(gl)
				if err != nil {
					return aicli.Result{}, err
				}
				d, err := c.MergeRequests.CreateDiscussion(ctx, sel.project, sel.iid, body, &pos)
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("guest"), mrNotFound(sel.project))
				}
				out := mrDiscussionOut(sel, *d, true)
				obs := map[string]any{"ref": out["ref"], "path": out["path"], "line": out["line"]}
				if len(d.Notes) > 0 {
					obs["body"] = mrText(d.Notes[0].Body)
				}
				return aicli.Result{Observed: obs, Data: out, Untrusted: mrDiscussionUntrusted}, nil
			},
		})
	}}
	addMRTargetFlags(c)
	addMRBodyFlags(c)
	c.Flags().String("new-path", "", "File, as named in the new version of the diff")
	c.Flags().String("old-path", "", "File in the old version (default: --new-path; set it for a renamed or removed file)")
	c.Flags().Int("new-line", 0, "Line number in the new version (an added or unchanged line)")
	c.Flags().Int("old-line", 0, "Line number in the old version (a removed line)")
	c.Flags().String("base-sha", "", "Diff base SHA (default: the merge request's current diff)")
	c.Flags().String("start-sha", "", "Diff start SHA (default: the merge request's current diff)")
	c.Flags().String("head-sha", "", "Diff head SHA (default: the merge request's current diff)")
	c.Flags().String("idempotency-key", "", "Repeat-safe key: repeating the same comment with the same key returns the first result")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Start an inline review thread on one line of a merge request's diff",
		Aliases: []string{"add", "new"}, Effects: []aicli.Effect{aicli.Change}, Output: "merge_request_discussion", Positional: "mr",
		FileInputs: map[string]string{"body-file": "body"},
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{mrDiscussionEvidence, liveEvidence("mr")},
		Examples: []string{`gitlab-cli mr discussion create acme/web!5 --new-path src/app.go --new-line 12 --body "This can be nil." --dry-run`,
			`gitlab-cli mr discussion create acme/web!5 --new-path src/app.go --new-line 12 --body "This can be nil." --confirm <confirm_token>`}})
}

func mrFirst(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return b
}

// ---------------------------------------------------------------- resolve, unresolve

func addMRDiscussionResolve(app *aicli.App, parent *cobra.Command, resolve bool) {
	verb, summary := "resolve", "Resolve a discussion thread of a merge request"
	if !resolve {
		verb, summary = "unresolve", "Reopen a resolved discussion thread of a merge request"
	}
	c := &cobra.Command{Use: verb + " <discussion>", RunE: func(cmd *cobra.Command, _ []string) error {
		want := resolve
		if f := cmd.Flags().Lookup("unresolve"); f != nil && f.Changed {
			want, _ = cmd.Flags().GetBool("unresolve")
			want = !want
		}
		gl := gitlabFor(app)
		sel, id, err := mrDiscussionTarget(cmd, gl)
		if err != nil {
			return err
		}
		ref := mrDiscussionRef(sel, id)
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: ref, Kind: "merge_request_discussion"},
			ReadBack: "mr discussion get " + ref,
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				c, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				d, err := c.MergeRequests.GetDiscussion(ctx, sel.project, sel.iid, id)
				if err != nil {
					return aicli.Preview{}, readError(err, mrDiscussionNotFound(sel))
				}
				resolvable, resolved := mrDiscussionResolved(*d)
				action := map[bool]string{true: "Resolve thread ", false: "Reopen thread "}[want] + ref
				pv := aicli.Preview{Action: action, CanApply: true,
					Changes: []aicli.FieldChange{{Field: "resolved", Before: resolved, After: want}}}
				var block *aicli.Block
				switch {
				case !resolvable:
					block = &aicli.Block{Kind: "not_resolvable", Message: "This thread cannot be resolved (a plain comment, or a system note)."}
				case want && resolved:
					block = &aicli.Block{Kind: "already_resolved", Message: "The thread is already resolved."}
				case !want && !resolved:
					block = &aicli.Block{Kind: "not_resolved", Message: "The thread is not resolved."}
				}
				if block != nil {
					pv.CanApply, pv.BlockedBy = false, []aicli.Block{*block}
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				c, err := mrApplyClient(gl)
				if err != nil {
					return aicli.Result{}, err
				}
				d, err := c.MergeRequests.ResolveDiscussion(ctx, sel.project, sel.iid, id, want)
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("developer"), mrDiscussionNotFound(sel))
				}
				_, resolved := mrDiscussionResolved(*d)
				return aicli.Result{Observed: map[string]any{"resolved": resolved}, Data: mrDiscussionOut(sel, *d, false),
					Untrusted: mrDiscussionUntrusted}, nil
			},
		})
	}}
	addMRDiscussionTargetFlags(c)
	meta := aicli.Meta{Summary: summary, Effects: []aicli.Effect{aicli.Change}, Output: "merge_request_discussion", Positional: "mr",
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Experimental, Evidence: []string{mrDiscussionEvidence},
		Examples: []string{fmt.Sprintf("gitlab-cli mr discussion %s acme/web!5/discussions/6a9c1750b37d --dry-run", verb),
			fmt.Sprintf("gitlab-cli mr discussion %s acme/web!5/discussions/6a9c1750b37d --confirm <confirm_token>", verb)}}
	if resolve {
		c.Flags().Bool("unresolve", false, "Reopen the thread instead")
		meta.DeprecatedFlags = map[string]string{"unresolve": "use `mr discussion unresolve`"}
		// resolve ran live; unresolve did not
		meta.Maturity, meta.Evidence = aicli.Verified, append(meta.Evidence, liveEvidence("mr"))
	}
	parent.AddCommand(c)
	app.Register(c, meta)
}
