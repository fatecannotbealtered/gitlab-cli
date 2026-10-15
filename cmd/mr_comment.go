package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// The comments of a merge request (GitLab's notes): list, add, delete. A comment's
// ref is its merge request's, its collection and ID, acme/web!5/notes/12, as for
// every child of a merge request or an issue (docs/migration-2.0/notes.md); the commands take that
// ref, GitLab's anchor (acme/web!5#note_12) or web URL (…/-/merge_requests/5#note_12),
// or the merge request and --note-id.

const mrCommentEvidence = "simulated:cmd/mr_comment_test.go"

var (
	mrNoteFields   = []string{"ref", "id", "author", "body", "system", "resolvable", "resolved", "created_at", "updated_at"}
	mrNoteDefaults = []string{"ref", "author", "body", "created_at"}
)

func registerMRComment(app *aicli.App, parent *cobra.Command) {
	group := &cobra.Command{Use: "comment", Aliases: []string{"comments", "note", "notes"}} // the fleet's guesses (CLI-022)
	parent.AddCommand(group)
	app.Group(group, "List, add and delete the comments of a merge request")
	app.Schema("merge_request_note", aicli.SchemaDef{Shape: "object", Fields: mrNoteFields, Untrusted: []string{"author", "body"}})
	app.Schema("merge_request_note_page", aicli.SchemaDef{Shape: "page", Fields: mrNoteFields, DefaultFields: mrNoteDefaults,
		Untrusted: []string{"author", "body"}})
	app.Schema("merge_request_note_deleted", aicli.SchemaDef{Shape: "object", Fields: []string{"ref", "deleted"}})
	addMRCommentList(app, group)
	addMRCommentAdd(app, group)
	addMRCommentDelete(app, group)
}

func mrNoteRef(sel mrSel, id int) string { return sel.ref() + "/notes/" + strconv.Itoa(id) }

// mrNoteOut is a comment as every comment and discussion command returns it.
func mrNoteOut(sel mrSel, n api.MergeRequestNote) map[string]any {
	return map[string]any{"ref": mrNoteRef(sel, n.ID), "id": idString(n.ID), "author": mrUsername(n.Author), "body": n.Body,
		"system": n.System, "resolvable": n.Resolvable, "resolved": n.Resolved,
		"created_at": utcTime(n.CreatedAt), "updated_at": utcTime(n.UpdatedAt)}
}

// ---------------------------------------------------------------- shared input

// mrBody reads --body or --body-file (stdin with -): required, not empty.
func mrBody(app *aicli.App, cmd *cobra.Command) (string, error) {
	body, ok, err := mrTextInput(app, cmd, "body")
	if err != nil {
		return "", err
	}
	if !ok {
		e := aicli.Usage("missing_parameter", "--body", fmt.Sprintf("`%s` needs the text: --body, or --body-file (- reads stdin).", cmd.CommandPath()))
		e.Details["next_action"] = "Add --body \"<text>\", or --body-file - and the text on stdin."
		return "", e
	}
	if body == "" {
		return "", aicli.Validation("--body", "a non-empty text", "The text is empty.")
	}
	return body, nil
}

func addMRBodyFlags(c *cobra.Command) {
	c.Flags().String("body", "", "Text (Markdown)")
	c.Flags().String("body-file", "", "Read the text from this file, or - for stdin")
}

// mrNoteTarget reads the merge request and the comment: the argument may be the
// comment's ref (acme/web!5/notes/12) or its URL, else --note-id names it. The note
// ID is bound as --note-id either way (CLI-054).
func mrNoteTarget(cmd *cobra.Command, gl *lazyClient) (mrSel, int, error) {
	sel, rest, err := mrTarget(cmd, gl)
	if err != nil {
		return mrSel{}, 0, err
	}
	raw, _ := cmd.Flags().GetString("note-id")
	raw = strings.TrimSpace(raw)
	if fromRef, ok := strings.CutPrefix(rest, "#note_"); ok {
		if raw != "" && raw != fromRef {
			e := aicli.Usage("conflicting_flags", "--note-id", "--note-id names another comment than the ref given as the argument.")
			e.Details["next_action"] = "Give the comment once: as its ref (acme/web!5/notes/12), or as the merge request and --note-id."
			return mrSel{}, 0, e
		}
		raw = fromRef
		if err := aicli.SetResolved(cmd, "note-id", raw); err != nil {
			return mrSel{}, 0, err
		}
	}
	if raw == "" {
		e := aicli.Usage("missing_parameter", "--note-id", fmt.Sprintf("`%s` needs the comment: its ref (acme/web!5/notes/12), or --note-id.", cmd.CommandPath()))
		e.Details["next_action"] = fmt.Sprintf("Run `gitlab-cli mr comment list %s` to find the comment's ref.", sel.ref())
		return mrSel{}, 0, e
	}
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return mrSel{}, 0, aicli.Validation("--note-id", "a positive comment ID", "--note-id must be a positive number.")
	}
	return sel, id, nil
}

// mrNoteNotFound is the 404 of a comment read.
func mrNoteNotFound(sel mrSel) errOpt {
	return onNotFound("The merge request has no comment with that ID, or this account cannot see it.",
		fmt.Sprintf("Run `gitlab-cli mr comment list %s` to find the comment.", sel.ref()))
}

// ---------------------------------------------------------------- list

func addMRCommentList(app *aicli.App, parent *cobra.Command) {
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
		notes, pag, err := client.MergeRequests.NotesPage(cmd.Context(), sel.project, sel.iid, req.Page, req.Limit)
		if err != nil {
			return readError(err, mrNotFound(sel.project))
		}
		items := []map[string]any{}
		for _, n := range notes {
			if !n.System {
				items = append(items, mrNoteOut(sel, n))
			}
		}
		pag.HasTotal = false // GitLab's total counts the system notes left out
		scope := map[string]any{"mr": sel.ref(),
			"covers": "comments people wrote, oldest first; GitLab's system notes are left out of each page, so a page can hold fewer than --limit"}
		return app.List(cmd, gitlabPage(items, len(items), req, pag, scope, "created_at_asc"))
	}}
	addMRTargetFlags(c)
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "List the comments on a merge request, oldest first (system notes left out)",
		Aliases: []string{"ls"}, Effects: []aicli.Effect{aicli.Observe}, Output: "merge_request_note_page", Positional: "mr",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{mrCommentEvidence, liveEvidence("mr")},
		Examples: []string{"gitlab-cli mr comment list acme/web!5", "gitlab-cli mr comment list 5 --project acme/web --limit 50"}})
}

// ---------------------------------------------------------------- add

func addMRCommentAdd(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "add <mr>", RunE: func(cmd *cobra.Command, _ []string) error {
		body, err := mrBody(app, cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		sel, _, err := mrTarget(cmd, gl)
		if err != nil {
			return err
		}
		return app.Write(cmd, aicli.WriteSpec{
			Target:   mrTargetOf(sel),
			ReadBack: "mr comment list " + sel.ref(),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				if _, _, err := mrRead(ctx, gl, sel); err != nil {
					return aicli.Preview{}, err
				}
				pv := aicli.Preview{Action: "Comment on merge request " + sel.ref(), CanApply: true,
					Changes: []aicli.FieldChange{{Field: "body", After: body}}}
				if hasQuickAction(body) {
					pv.CanApply, pv.BlockedBy = false, []aicli.Block{quickActionBlock("comment")}
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				c, err := mrApplyClient(gl)
				if err != nil {
					return aicli.Result{}, err
				}
				n, err := c.MergeRequests.AddNote(ctx, sel.project, sel.iid, body)
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("guest"), mrNotFound(sel.project))
				}
				return aicli.Result{Observed: map[string]any{"ref": mrNoteRef(sel, n.ID), "body": mrText(n.Body)},
					Data: mrNoteOut(sel, *n), Untrusted: []string{"author", "body"}}, nil
			},
		})
	}}
	addMRTargetFlags(c)
	addMRBodyFlags(c)
	c.Flags().String("idempotency-key", "", "Repeat-safe key: repeating the same comment with the same key returns the first result")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Comment on a merge request (a line-anchored review comment is `mr discussion create`)",
		Aliases: []string{"new", "create"}, Effects: []aicli.Effect{aicli.Change}, Output: "merge_request_note", Positional: "mr",
		FileInputs: map[string]string{"body-file": "body"},
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{mrCommentEvidence, liveEvidence("mr")},
		Examples: []string{`gitlab-cli mr comment add acme/web!5 --body "Looks good; one nit below." --dry-run`,
			`gitlab-cli mr comment add acme/web!5 --body "Looks good; one nit below." --confirm <confirm_token>`}})
}

// ---------------------------------------------------------------- delete

func addMRCommentDelete(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "delete <comment>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, id, err := mrNoteTarget(cmd, gl)
		if err != nil {
			return err
		}
		ref := mrNoteRef(sel, id)
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: ref, Kind: "merge_request_note"},
			ReadBack: "mr comment list " + sel.ref(),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				c, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				n, err := c.MergeRequests.GetNote(ctx, sel.project, sel.iid, id)
				if err != nil {
					return aicli.Preview{}, readError(err, mrNoteNotFound(sel))
				}
				pv := aicli.Preview{Action: "Delete comment " + ref + " permanently", CanApply: true,
					Changes:   []aicli.FieldChange{{Field: "body", Before: mrExcerpt(n.Body, 200), Note: "the comment is deleted; cannot be undone"}},
					Untrusted: []string{"changes[].before"}}
				if n.System {
					pv.CanApply, pv.BlockedBy = false, []aicli.Block{{Kind: "system_note", Message: "This is a GitLab system note, which cannot be deleted."}}
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				c, err := mrApplyClient(gl)
				if err != nil {
					return aicli.Result{}, err
				}
				if err := c.MergeRequests.DeleteNote(ctx, sel.project, sel.iid, id); err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("maintainer"), mrNoteNotFound(sel))
				}
				// Read back: the comment must be gone.
				switch _, err := c.MergeRequests.GetNote(ctx, sel.project, sel.iid, id); {
				case mrIs(err, 404):
					return aicli.Result{Outcome: aicli.Outcome{Verification: "passed"}, Observed: map[string]any{"exists": false},
						Data: map[string]any{"ref": ref, "deleted": true}}, nil
				case err == nil:
					return aicli.Result{Outcome: aicli.Outcome{Verification: "failed"}, Observed: map[string]any{"exists": true},
						Mismatched: []string{"exists"}, Data: map[string]any{"ref": ref, "deleted": false}}, nil
				}
				return aicli.Result{Data: map[string]any{"ref": ref, "deleted": true}}, nil
			},
		})
	}}
	c.Flags().String("mr", "", "The comment's ref (acme/web!5/notes/12) or URL; or the merge request (iid, ref or URL) with --note-id")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when the argument is a ref or URL")
	c.Flags().String("note-id", "", "Comment ID, when the argument names only the merge request")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Delete a comment from a merge request (cannot be undone)",
		Aliases: []string{"rm", "remove"}, Effects: []aicli.Effect{aicli.Change}, Dangerous: true, Output: "merge_request_note_deleted",
		Positional: "mr", ErrorCodes: gitlabWriteCodes, Maturity: aicli.Experimental, Evidence: []string{mrCommentEvidence},
		Examples: []string{"gitlab-cli mr comment delete acme/web!5/notes/12 --dangerous --dry-run",
			"gitlab-cli mr comment delete acme/web!5/notes/12 --dangerous --confirm <confirm_token>"}})
}
