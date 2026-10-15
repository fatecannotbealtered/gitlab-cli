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

// issue bulk: one change for many issues of one project in one call (CLI-063).
// GitLab has no bulk REST endpoint for issues, so each issue is its own request,
// behind one preview, one confirm token and one result: App.WriteBatch with
// Meta.BatchFlag "ids". The items' previews read their issues with one list request
// (iids[]) per 100 issues; the confirm is a new run and reads again.

const issueBulkEvidence = "simulated:cmd/issue_bulk_test.go"

// issueBriefUntrusted are the people-written fields of issueBrief.
var issueBriefUntrusted = []string{"title", "labels"}

func registerIssueBulk(app *aicli.App, parent *cobra.Command) {
	group := &cobra.Command{Use: "bulk"}
	parent.AddCommand(group)
	app.Group(group, "Change many issues of one project in one call: close, reopen, update, label, assign, comment")
	addIssueBulkState(app, group, "close", "Close many issues; each must be open")
	addIssueBulkState(app, group, "reopen", "Reopen many closed issues; each must be closed")
	addIssueBulkUpdate(app, group)
	addIssueBulkLabel(app, group)
	addIssueBulkAssign(app, group)
	addIssueBulkComment(app, group)
}

// addIssueBatchFlags declares --ids and --project; aicli adds --continue-on-error.
func addIssueBatchFlags(c *cobra.Command) {
	c.Flags().StringSlice("ids", nil, "Issues: numbers (2), refs (acme/web#2) or web URLs, comma-separated or repeated")
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID; optional when --ids are refs")
}

// issueBatchTargets reads --ids: iids, GitLab references (#2, acme/web#2) or web
// URLs, all of one project, de-duplicated in input order, and binds the iids and the
// project (CLI-054), so 2,6 with --project and acme/web#2,acme/web#6 are one
// request. The targets' keys are the refs.
func issueBatchTargets(cmd *cobra.Command, gl *lazyClient) (string, []int, error) {
	raw, _ := cmd.Flags().GetStringSlice("ids")
	refProject := ""
	var iids []int
	for _, v := range raw {
		if strings.TrimSpace(v) == "" {
			continue
		}
		p, iid, err := parseIssueArg(v)
		if err != nil {
			e := aicli.Validation("--ids", "issue numbers (2), refs (acme/web#2) or web URLs", "An --ids value is not an issue number, ref or web URL.")
			e.Details["next_action"] = "Run `gitlab-cli issue list --project <group/project>` and pass the items' refs."
			return "", nil, e
		}
		if p != "" {
			if refProject != "" && !strings.EqualFold(refProject, p) {
				e := aicli.Usage("conflicting_flags", "--ids", "--ids names issues of more than one project; a batch acts on one project.")
				e.Details["next_action"] = "Run one batch per project."
				return "", nil, e
			}
			refProject = p
		}
		if !issueHasInt(iids, iid) {
			iids = append(iids, iid)
		}
	}
	if len(iids) == 0 {
		e := aicli.Usage("missing_parameter", "--ids", "The batch names no issues; nothing was changed.")
		e.Details["next_action"] = "Give the issues with --ids, for example --ids 2,6."
		return "", nil, e
	}
	project, err := projectFlag(cmd, refProject, gl)
	if err != nil {
		return "", nil, err
	}
	bound := make([]string, len(iids))
	for i, iid := range iids {
		bound[i] = strconv.Itoa(iid)
	}
	if err := aicli.SetResolved(cmd, "ids", bound...); err != nil {
		return "", nil, err
	}
	return project, iids, nil
}

func issueHasInt(list []int, n int) bool {
	for _, x := range list {
		if x == n {
			return true
		}
	}
	return false
}

// issueBatchReader reads a batch's issues the first time an item's Preview asks:
// one list request per 100 iids, every state. An iid it did not return is
// E_NOT_FOUND, which blocks that item (CLI-063).
type issueBatchReader struct {
	gl      *lazyClient
	project string
	iids    []int
	done    bool
	byIID   map[int]api.Issue
	err     error
}

func (b *issueBatchReader) get(ctx context.Context, iid int) (*api.Issue, error) {
	if !b.done {
		b.done = true
		b.byIID = map[int]api.Issue{}
		client, err := b.gl.get()
		if err != nil {
			b.err = err
		}
		for start := 0; b.err == nil && start < len(b.iids); start += api.MaxPerPage {
			chunk := b.iids[start:min(start+api.MaxPerPage, len(b.iids))]
			issues, _, err := client.Issues.ListPage(ctx, b.project, api.IssueListOpts{IIDs: chunk, Page: 1, PerPage: len(chunk)})
			if err != nil {
				b.err = readError(err, projectNotFound())
				break
			}
			for _, iss := range issues {
				b.byIID[iss.IID] = iss
			}
		}
	}
	if b.err != nil {
		return nil, b.err
	}
	iss, ok := b.byIID[iid]
	if !ok {
		e := aicli.NotFound("The project has no issue with that number, or this account cannot see it.",
			fmt.Sprintf("Run `gitlab-cli issue list --project %s --state all` and drop the missing issue from --ids.", b.project))
		return nil, e
	}
	return &iss, nil
}

// issueBatchItems is one BatchItem per issue for a change applied with an issue
// update. verb names the change in the preview's action; role is the GitLab role a
// refusal names (CLI-014).
func issueBatchItems(gl *lazyClient, project string, iids []int, want issueChange, verb, role string) []aicli.BatchItem {
	reader := &issueBatchReader{gl: gl, project: project, iids: iids}
	r := newIssueResolver(gl, project)
	items := make([]aicli.BatchItem, 0, len(iids))
	for _, iid := range iids {
		var plan issuePlan
		items = append(items, aicli.BatchItem{
			Target: aicli.Target{Key: issueRef(project, iid), Kind: "issue"},
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				cur, err := reader.get(ctx, iid)
				if err != nil {
					return aicli.Preview{}, err
				}
				if plan, err = r.resolve(ctx, want); err != nil {
					return aicli.Preview{}, err
				}
				pv := aicli.Preview{Action: fmt.Sprintf("%s issues in %s", verb, project), CanApply: true,
					Changes: issueChanges(cur, want, plan), BaseVersion: cur.UpdatedAt, BaseSource: "updated_at",
					Untrusted: issueChangeUntrusted(true, want)}
				if b := issueBlocks(cur, want); len(b) > 0 {
					pv.CanApply, pv.BlockedBy = false, b
				}
				return pv, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				client, err := gl.get()
				if err != nil {
					return aicli.Result{}, aicli.NotDispatched(err)
				}
				iss, err := client.Issues.Update(ctx, project, iid, issueUpdateOpts(want, plan))
				if err != nil {
					return aicli.Result{}, classifyWriteError(err, requiresRole(role), issueNotFound(project))
				}
				return issueApplied(*iss, want, issueBrief(project, *iss), issueBriefUntrusted), nil
			},
		})
	}
	return items
}

// issueBatchWrite runs a bulk change that is an issue update for each issue.
func issueBatchWrite(app *aicli.App, cmd *cobra.Command, read func(project string) (issueChange, error), verb string) error {
	gl := gitlabFor(app)
	project, iids, err := issueBatchTargets(cmd, gl)
	if err != nil {
		return err
	}
	want, err := read(project)
	if err != nil {
		return err
	}
	return app.WriteBatch(cmd, aicli.BatchSpec{Items: issueBatchItems(gl, project, iids, want, verb, "reporter")})
}

// issueBatchMeta is the Meta every bulk command shares.
func issueBatchMeta(summary, output string, examples ...string) aicli.Meta {
	return aicli.Meta{Summary: summary, Effects: []aicli.Effect{aicli.Change}, Output: output, BatchFlag: "ids",
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Experimental, Evidence: []string{issueBulkEvidence}, Examples: examples}
}

// addIssueBulkState adds issue bulk close or issue bulk reopen.
func addIssueBulkState(app *aicli.App, parent *cobra.Command, verb, summary string) {
	c := &cobra.Command{Use: verb, RunE: func(cmd *cobra.Command, _ []string) error {
		return issueBatchWrite(app, cmd, func(string) (issueChange, error) { return issueChange{stateEvent: verb}, nil },
			strings.ToUpper(verb[:1])+verb[1:])
	}}
	addIssueBatchFlags(c)
	parent.AddCommand(c)
	app.Register(c, issueBatchMeta(summary, "issue",
		"gitlab-cli issue bulk "+verb+" --project acme/web --ids 2,6 --dry-run",
		"gitlab-cli issue bulk "+verb+" --project acme/web --ids 2,6 --confirm <confirm_token>"))
}

func addIssueBulkUpdate(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "update", RunE: func(cmd *cobra.Command, _ []string) error {
		return issueBatchWrite(app, cmd, func(project string) (issueChange, error) { return readIssueChange(app, cmd, project) }, "Update")
	}}
	addIssueBatchFlags(c)
	c.Flags().String("title", "", "New title for every issue")
	c.Flags().String("description", "", "New description for every issue (an empty value clears it)")
	c.Flags().String("description-file", "", "Read the new description from a file, or - for stdin")
	c.Flags().StringSlice("add-labels", nil, "Labels to add (comma-separated or repeated)")
	c.Flags().StringSlice("remove-labels", nil, "Labels to remove (comma-separated or repeated)")
	c.Flags().String("milestone", "", "New milestone: its iid, ref or title (an empty value removes it)")
	c.Flags().String("milestone-id", "", "New milestone ID (the global one)")
	c.MarkFlagsOneRequired("title", "description", "description-file", "add-labels", "remove-labels", "milestone", "milestone-id")
	c.MarkFlagsMutuallyExclusive("milestone", "milestone-id")
	parent.AddCommand(c)
	m := issueBatchMeta("Change the title, description, labels or milestone of many issues (only the fields given)", "issue",
		"gitlab-cli issue bulk update --project acme/web --ids 2,6 --add-labels needs-triage --milestone v1.1 --dry-run",
		"gitlab-cli issue bulk update --project acme/web --ids 2,6 --add-labels needs-triage --milestone v1.1 --confirm <confirm_token>")
	m.FileInputs = map[string]string{"description-file": "description"}
	m.DeprecatedFlags = map[string]string{"milestone-id": "use --milestone with the milestone's iid, ref or title"}
	app.Register(c, m)
}

func addIssueBulkLabel(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "label", RunE: func(cmd *cobra.Command, _ []string) error {
		return issueBatchWrite(app, cmd, func(project string) (issueChange, error) { return readIssueChange(app, cmd, project) }, "Label")
	}}
	addIssueBatchFlags(c)
	c.Flags().StringSlice("add", nil, "Labels to add (comma-separated or repeated)")
	c.Flags().StringSlice("remove", nil, "Labels to remove (comma-separated or repeated)")
	c.MarkFlagsOneRequired("add", "remove")
	parent.AddCommand(c)
	m := issueBatchMeta("Add labels to many issues or remove them; their other labels stay", "issue",
		"gitlab-cli issue bulk label --project acme/web --ids 2,6,10 --add needs-triage --dry-run",
		"gitlab-cli issue bulk label --project acme/web --ids 2,6,10 --add needs-triage --confirm <confirm_token>")
	m.Maturity, m.Evidence = aicli.Verified, append(m.Evidence, liveEvidence("issue")) // ran live
	app.Register(c, m)
}

func addIssueBulkAssign(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "assign <assignee>", RunE: func(cmd *cobra.Command, _ []string) error {
		return issueBatchWrite(app, cmd, func(string) (issueChange, error) {
			var w issueChange
			var err error
			w.assignees, w.setAssignees, err = issueUsernames(cmd, "assignee")
			return w, err
		}, "Assign")
	}}
	addIssueBatchFlags(c)
	c.Flags().StringSlice("assignee", nil, "Users to assign, replacing the current ones: carol, @carol or me (an empty value unassigns everyone)")
	_ = c.MarkFlagRequired("assignee")
	parent.AddCommand(c)
	m := issueBatchMeta("Assign many issues to users, replacing their assignees", "issue",
		"gitlab-cli issue bulk assign carol --project acme/web --ids 2,6 --dry-run",
		"gitlab-cli issue bulk assign carol --project acme/web --ids 2,6 --confirm <confirm_token>")
	m.Positional = "assignee"
	app.Register(c, m)
}

func addIssueBulkComment(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "comment", RunE: func(cmd *cobra.Command, _ []string) error {
		gl := gitlabFor(app)
		project, iids, err := issueBatchTargets(cmd, gl)
		if err != nil {
			return err
		}
		body, _, err := readIssueText(app, cmd, "body")
		if err != nil {
			return err
		}
		if strings.TrimSpace(body) == "" {
			e := aicli.Usage("missing_parameter", "--body", "The comment is empty: give --body or --body-file.")
			e.Details["next_action"] = "Add --body <text>."
			return e
		}
		reader := &issueBatchReader{gl: gl, project: project, iids: iids}
		items := make([]aicli.BatchItem, 0, len(iids))
		for _, iid := range iids {
			items = append(items, aicli.BatchItem{
				Target: aicli.Target{Key: issueRef(project, iid), Kind: "issue"},
				Preview: func(ctx context.Context) (aicli.Preview, error) {
					if _, err := reader.get(ctx, iid); err != nil {
						return aicli.Preview{}, err
					}
					pv := aicli.Preview{Action: "Comment on issues in " + project, CanApply: true,
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
					n, err := client.Issues.AddNote(ctx, project, iid, body)
					if err != nil {
						return aicli.Result{}, classifyWriteError(err, requiresRole("guest"), issueNotFound(project))
					}
					return aicli.Result{Observed: map[string]any{"body": n.Body, "ref": issueNoteRef(project, iid, n.ID)},
						Data: issueCommentOut(project, iid, *n), Untrusted: issueCommentUntrusted}, nil
				},
			})
		}
		return app.WriteBatch(cmd, aicli.BatchSpec{Items: items})
	}}
	addIssueBatchFlags(c)
	c.Flags().String("body", "", "Comment text (Markdown)")
	c.Flags().String("body-file", "", "Read the comment from a file, or - for stdin")
	parent.AddCommand(c)
	m := issueBatchMeta("Add the same comment to many issues", "issue_comment",
		"gitlab-cli issue bulk comment --project acme/web --ids 2,6 --body \"Triaged in the weekly sync.\" --dry-run",
		"gitlab-cli issue bulk comment --project acme/web --ids 2,6 --body \"Triaged in the weekly sync.\" --confirm <confirm_token>")
	m.FileInputs = map[string]string{"body-file": "body"}
	app.Register(c, m)
}
