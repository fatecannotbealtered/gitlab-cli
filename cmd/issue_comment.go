package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// issue comment list, create and delete: the comments (GitLab's notes) on one
// issue. A comment's ref is its issue's ref and its ID, acme/web#12/notes/99, so a
// scope grant for the issue covers its comments; its web URL is the issue's with
// #note_99.

const issueCommentEvidence = "simulated:cmd/issue_comment_test.go"

var (
	issueCommentFields        = []string{"ref", "id", "issue", "author", "body", "system", "internal", "created_at", "updated_at"}
	issueCommentDefaultFields = []string{"ref", "author", "created_at", "body"}
	issueCommentUntrusted     = []string{"body"}
)

func registerIssueComment(app *aicli.App, parent *cobra.Command) {
	group := &cobra.Command{Use: "comment", Aliases: []string{"comments", "note", "notes"}}
	parent.AddCommand(group)
	app.Group(group, "List, add and delete the comments on an issue")
	app.Schema("issue_comment", aicli.SchemaDef{Shape: "object", Fields: issueCommentFields, Untrusted: issueCommentUntrusted})
	app.Schema("issue_comment_page", aicli.SchemaDef{Shape: "page", Fields: issueCommentFields,
		DefaultFields: issueCommentDefaultFields, Untrusted: issueCommentUntrusted})
	app.Schema("issue_comment_deleted", aicli.SchemaDef{Shape: "object", Fields: []string{"ref", "deleted"}})
	addIssueCommentList(app, group)
	addIssueCommentCreate(app, group)
	addIssueCommentDelete(app, group)
}

// issueNoteRef is a comment's ref (CLI-048).
func issueNoteRef(project string, iid, noteID int) string {
	return issueRef(project, iid) + "/notes/" + strconv.Itoa(noteID)
}

// issueCommentOut is a comment as the comment commands return it.
func issueCommentOut(project string, iid int, n api.IssueNote) map[string]any {
	return map[string]any{"ref": issueNoteRef(project, iid, n.ID), "id": idString(n.ID), "issue": issueRef(project, iid),
		"author": issueUser(n.Author), "body": n.Body, "system": n.System, "internal": n.Internal,
		"created_at": utcTime(n.CreatedAt), "updated_at": utcTime(n.UpdatedAt)}
}

// parseIssueNoteArg reads a comment as an agent has it: its ref
// (acme/web#12/notes/99) or its web URL (…/acme/web/-/issues/12#note_99). ok is
// false when raw names no comment; it may still name an issue.
func parseIssueNoteArg(raw string) (project string, iid, noteID int, ok bool, err error) {
	s := strings.TrimSpace(raw)
	if strings.Contains(s, "://") {
		u, perr := url.Parse(s)
		if perr != nil || !strings.HasPrefix(u.Fragment, "note_") {
			return "", 0, 0, false, nil
		}
		n, nerr := strconv.Atoi(strings.TrimPrefix(u.Fragment, "note_"))
		u.Fragment = ""
		p, i, ierr := parseIssueArg(u.String())
		if nerr != nil || ierr != nil || n <= 0 || p == "" {
			return "", 0, 0, true, errors.New("not a comment URL")
		}
		return p, i, n, true, nil
	}
	head, tail, found := strings.Cut(s, "/notes/")
	if !found {
		return "", 0, 0, false, nil
	}
	n, nerr := strconv.Atoi(tail)
	p, i, ierr := parseIssueArg(head)
	if nerr != nil || ierr != nil || n <= 0 {
		return "", 0, 0, true, errors.New("not a comment ref")
	}
	return p, i, n, true, nil
}

// issueNoteSel is an issue and, for commands on one comment, the comment's ID.
type issueNoteSel struct {
	issueSel
	noteID int
}

func (s issueNoteSel) noteRef() string { return issueNoteRef(s.project, s.iid, s.noteID) }

// issueNoteTarget reads the issue positional (or --issue), which may also be a
// comment's ref or URL, the project and --note-id where the command has it, and
// binds the iid, the project and the comment's ID (CLI-054): `delete 12 --project
// acme/web --note-id 99` and `delete acme/web#12/notes/99` are one request.
func issueNoteTarget(cmd *cobra.Command, gl *lazyClient) (issueNoteSel, error) {
	raw, _ := cmd.Flags().GetString("issue")
	p, iid, noteID, isNote, err := parseIssueNoteArg(raw)
	if err != nil {
		e := aicli.Validation("--issue", "an issue (12, acme/web#12, its URL) or a comment ref (acme/web#12/notes/99) or URL",
			"That is not an issue or comment ref.")
		e.Details["next_action"] = "Run `gitlab-cli issue comment list <issue>` to find the comment's ref."
		return issueNoteSel{}, e
	}
	if !isNote {
		sel, err := issueTarget(cmd, gl)
		if err != nil {
			return issueNoteSel{}, err
		}
		out := issueNoteSel{issueSel: sel}
		if f := cmd.Flags().Lookup("note-id"); f != nil && f.Changed {
			if out.noteID, _ = cmd.Flags().GetInt("note-id"); out.noteID <= 0 {
				return issueNoteSel{}, aicli.Validation("--note-id", "a positive comment ID", "--note-id must be a positive number.")
			}
		}
		return out, nil
	}
	if f := cmd.Flags().Lookup("note-id"); f != nil && f.Changed {
		if given, _ := cmd.Flags().GetInt("note-id"); given != noteID {
			e := aicli.Usage("conflicting_flags", "--note-id", "--note-id names another comment than the ref given as the argument.")
			e.Details["next_action"] = "Give the comment once: drop --note-id, or pass the issue as the argument."
			return issueNoteSel{}, e
		}
	}
	project, err := projectFlag(cmd, p, gl)
	if err != nil {
		return issueNoteSel{}, err
	}
	sel := issueNoteSel{issueSel: issueSel{project: project, iid: iid}, noteID: noteID}
	if err := aicli.SetResolved(cmd, "issue", strconv.Itoa(iid)); err != nil {
		return issueNoteSel{}, err
	}
	if cmd.Flags().Lookup("note-id") != nil {
		if err := aicli.SetResolved(cmd, "note-id", strconv.Itoa(noteID)); err != nil {
			return issueNoteSel{}, err
		}
	}
	return sel, nil
}

// issueNoteNotFound is the 404 of a comment read.
func issueNoteNotFound(sel issueNoteSel) errOpt {
	return onNotFound("The issue has no comment with that ID, or this account cannot see it.",
		fmt.Sprintf("Run `gitlab-cli issue comment list %s` to find the comment.", sel.ref()))
}

func addIssueCommentList(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "list <issue>", RunE: func(cmd *cobra.Command, _ []string) error {
		req, err := readPage(cmd)
		if err != nil {
			return err
		}
		gl := gitlabFor(app)
		sel, err := issueNoteTarget(cmd, gl)
		if err != nil {
			return err
		}
		client, err := gl.get()
		if err != nil {
			return err
		}
		notes, pag, err := client.Issues.ListNotesPage(cmd.Context(), sel.project, sel.iid,
			api.IssueNoteListOpts{OrderBy: "created_at", Sort: "desc", Page: req.Page, PerPage: req.Limit})
		if err != nil {
			return readError(err, issueNotFound(sel.project))
		}
		items := make([]map[string]any, 0, len(notes))
		for _, n := range notes {
			if !n.System {
				items = append(items, issueCommentOut(sel.project, sel.iid, n))
			}
		}
		scope := map[string]any{"project": sel.project, "issue": sel.ref(),
			"covers": "comments people wrote; GitLab's system notes (label, state and assignee changes) are left out, so a page can hold fewer than --limit"}
		page := gitlabPage(items, len(items), req, pag, scope, "created_at_desc")
		// GitLab's X-Total counts the system notes too, so it is not this list's total.
		page.Total = nil
		return app.List(cmd, page)
	}}
	addIssueTargetFlags(c)
	addPageFlags(c)
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "List the comments on an issue, newest first (GitLab's system notes left out)",
		Aliases: []string{"ls"}, Effects: []aicli.Effect{aicli.Observe}, Output: "issue_comment_page", Positional: "issue",
		ErrorCodes: gitlabReadCodes, Maturity: aicli.Verified, Evidence: []string{issueCommentEvidence, liveEvidence("issue")},
		Examples: []string{"gitlab-cli issue comment list acme/web#12", "gitlab-cli issue comment list 12 --project acme/web --limit 50"}})
}

func addIssueCommentCreate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "create <issue>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, err := issueNoteTarget(cmd, gl)
		if err != nil {
			return err
		}
		body, _, err := readIssueText(app, cmd, "body")
		if err != nil {
			return err
		}
		if strings.TrimSpace(body) == "" {
			e := aicli.Usage("missing_parameter", "--body", "The comment is empty: give --body or --body-file.")
			e.Details["next_action"] = fmt.Sprintf("Add --body <text>, for example `gitlab-cli issue comment create %s --body \"Reproduced on staging.\"`.", sel.ref())
			return e
		}
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: sel.ref(), Kind: "issue"},
			ReadBack: "issue comment list " + sel.ref(),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				if _, err := client.Issues.Get(ctx, sel.project, sel.iid); err != nil {
					return aicli.Preview{}, readError(err, issueNotFound(sel.project))
				}
				// A comment adds to the issue and changes nothing in it, so no base
				// version: others commenting meanwhile is no reason to refuse. A line
				// GitLab would run as a quick action (/close, /move) would change it.
				pv := aicli.Preview{Action: "Comment on issue " + sel.ref(), CanApply: true,
					Changes: []aicli.FieldChange{{Field: "body", After: body}}}
				if hasQuickAction(body) {
					pv.CanApply, pv.BlockedBy = false, []aicli.Block{quickActionBlock("comment")}
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				n, err := client.Issues.AddNote(ctx, sel.project, sel.iid, body)
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("guest"), issueNotFound(sel.project))
				}
				// The new comment's ref is observed too, so `operation show` finds it
				// after a lost reply (CLI-059).
				return aicli.Result{Observed: map[string]any{"body": n.Body, "ref": issueNoteRef(sel.project, sel.iid, n.ID)},
					Data: issueCommentOut(sel.project, sel.iid, *n), Untrusted: issueCommentUntrusted}, nil
			},
		})
	}}
	addIssueTargetFlags(c)
	c.Flags().String("body", "", "Comment text (Markdown)")
	c.Flags().String("body-file", "", "Read the comment from a file, or - for stdin")
	c.Flags().String("idempotency-key", "", "Repeat-safe key: repeating the same comment with the same key returns the first result")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Add a comment to an issue", Aliases: []string{"add", "new"},
		Effects: []aicli.Effect{aicli.Change}, Output: "issue_comment", Positional: "issue",
		FileInputs: map[string]string{"body-file": "body"},
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{issueCommentEvidence, liveEvidence("issue")},
		Examples: []string{"gitlab-cli issue comment create acme/web#12 --body \"Reproduced on staging.\" --dry-run",
			"gitlab-cli issue comment create acme/web#12 --body \"Reproduced on staging.\" --confirm <confirm_token>"}})
}

func addIssueCommentDelete(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "delete <issue>", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		sel, err := issueNoteTarget(cmd, gl)
		if err != nil {
			return err
		}
		if sel.noteID == 0 {
			e := aicli.Usage("missing_parameter", "--note-id", "`issue comment delete` needs the comment: its ref, or the issue and --note-id.")
			e.Details["next_action"] = fmt.Sprintf("Run `gitlab-cli issue comment list %s` and pass the comment's ref as the argument.", sel.ref())
			return e
		}
		ref := sel.noteRef()
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: ref, Kind: "issue_comment"},
			ReadBack: "issue comment list " + sel.ref(),
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Preview{}, err
				}
				n, err := client.Issues.GetNote(ctx, sel.project, sel.iid, sel.noteID)
				if err != nil {
					return aicli.Preview{}, readError(err, issueNoteNotFound(sel))
				}
				author := "someone"
				if n.Author != nil && n.Author.Username != "" {
					author = userRef(n.Author.Username)
				}
				pv := aicli.Preview{Action: "Delete comment " + ref + " permanently", CanApply: true,
					Changes: []aicli.FieldChange{{Field: "comment", Before: issueExcerpt(n.Body, 200),
						Note: "written by " + author + "; deleted permanently, cannot be undone"}},
					BaseVersion: n.UpdatedAt, BaseSource: "updated_at", Untrusted: []string{"changes[].before"}}
				if n.System {
					pv.CanApply = false
					pv.BlockedBy = []aicli.Block{{Kind: "system_note", Message: "This is a note GitLab wrote itself; it cannot be deleted."}}
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				if err := client.Issues.DeleteNote(ctx, sel.project, sel.iid, sel.noteID); err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole("maintainer"), issueNoteNotFound(sel))
				}
				// Read back: the comment must be gone.
				var apiErr *api.APIError
				switch _, err := client.Issues.GetNote(ctx, sel.project, sel.iid, sel.noteID); {
				case errors.As(err, &apiErr) && apiErr.StatusCode == 404:
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
	addIssueTargetFlags(c)
	c.Flags().Int("note-id", 0, "Comment ID (the id of an issue comment list item); not needed with a comment ref")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Delete a comment from an issue permanently", Aliases: []string{"rm", "remove"},
		Effects: []aicli.Effect{aicli.Change}, Dangerous: true, Output: "issue_comment_deleted", Positional: "issue",
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Verified, Evidence: []string{issueCommentEvidence, liveEvidence("issue")},
		Examples: []string{"gitlab-cli issue comment delete acme/web#12/notes/99 --dangerous --dry-run",
			"gitlab-cli issue comment delete acme/web#12/notes/99 --dangerous --confirm <confirm_token>"}})
}

// issueExcerpt is the start of a text, at most n characters, for a preview.
func issueExcerpt(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
