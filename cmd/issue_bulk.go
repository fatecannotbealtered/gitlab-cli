package cmd

import (
	"strconv"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/spf13/cobra"
)

// issue bulk is a class-B batch (CLI-SPEC §15.7): GitLab has no public bulk REST
// for issue state/label/assignee changes, so the command loops client-side over
// --ids. The external contract is identical to a native bulk endpoint — plural
// input, one dry-run preview, one confirm token, aggregated items[]/summary,
// --continue-on-error — so an agent cannot tell A from B.

var issueBulkCmd = &cobra.Command{
	Use:   "bulk",
	Short: "Run a write action across many issues in one call",
}

// bulkIssueAction performs one issue mutation and returns the optional per-item
// extra detail to surface in items[]. The loop owns aggregation and error
// classification; this closure owns only the single-issue call.
type bulkIssueAction func(client *api.Client, project string, iid int) (map[string]any, error)

// runIssueBulk resolves the plural --ids, runs the §15 dry-run/confirm gate, then
// loops the action over every iid aggregating per-item results.
func runIssueBulk(cmd *cobra.Command, action string, extra map[string]any, fn bulkIssueAction) error {
	project, _ := cmd.Flags().GetString("project")
	if project == "" {
		return failArg("--project is required")
	}
	ids, err := parsePluralIntFlag(cmd, "ids")
	if err != nil {
		return err
	}
	targets := make([]string, len(ids))
	for i, id := range ids {
		targets[i] = strconv.Itoa(id)
	}

	previewExtra := map[string]any{"project": project}
	for k, v := range extra {
		previewExtra[k] = v
	}
	if done, err := prepareBatchWrite(cmd, action, targets, previewExtra); done || err != nil {
		return err
	}

	client, _, err := newClient()
	if err != nil {
		return err
	}

	keepGoing := continueOnError(cmd)
	items := make([]batchItem, 0, len(ids))
	var unattempted []string
	for i, iid := range ids {
		detail, err := fn(client, project, iid)
		if err != nil {
			items = append(items, batchItem{Target: targets[i], OK: false, Error: batchErrorFor(err)})
			setExitCode(ExitError)
			if !keepGoing {
				unattempted = targetsTail(targets, i+1)
				break
			}
			continue
		}
		items = append(items, batchItem{Target: targets[i], OK: true, Extra: detail})
	}
	emitBatchResult(items, unattempted)
	return nil
}

// targetsTail returns the remaining targets from index i, reported as skipped so
// the agent can resume after a --continue-on-error=false stop.
func targetsTail(targets []string, i int) []string {
	if i >= len(targets) {
		return nil
	}
	out := make([]string, len(targets)-i)
	copy(out, targets[i:])
	return out
}

// ─── issue bulk close / reopen ───────────────────────────────────────────────

var issueBulkCloseCmd = &cobra.Command{
	Use:   "close",
	Short: "Close many issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runIssueBulk(cmd, "issue bulk close", nil, func(c *api.Client, project string, iid int) (map[string]any, error) {
			_, err := c.Issues.Update(apiCtx(), project, iid, api.IssueUpdateOpts{StateEvent: "close"})
			return map[string]any{"action": "close"}, err
		})
	},
}

var issueBulkReopenCmd = &cobra.Command{
	Use:   "reopen",
	Short: "Reopen many issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runIssueBulk(cmd, "issue bulk reopen", nil, func(c *api.Client, project string, iid int) (map[string]any, error) {
			_, err := c.Issues.Update(apiCtx(), project, iid, api.IssueUpdateOpts{StateEvent: "reopen"})
			return map[string]any{"action": "reopen"}, err
		})
	},
}

// ─── issue bulk update ───────────────────────────────────────────────────────

var issueBulkUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update many issues (title/description/labels/milestone)",
	RunE: func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		desc, _ := cmd.Flags().GetString("description")
		addLabels, _ := cmd.Flags().GetString("add-labels")
		removeLabels, _ := cmd.Flags().GetString("remove-labels")
		milestoneID, _ := cmd.Flags().GetInt("milestone-id")
		if title == "" && desc == "" && addLabels == "" && removeLabels == "" && milestoneID == 0 {
			return failArg("at least one of --title, --description, --add-labels, --remove-labels, --milestone-id is required")
		}
		opts := api.IssueUpdateOpts{
			Title:        title,
			Description:  desc,
			AddLabels:    addLabels,
			RemoveLabels: removeLabels,
			MilestoneID:  milestoneID,
		}
		return runIssueBulk(cmd, "issue bulk update", nil, func(c *api.Client, project string, iid int) (map[string]any, error) {
			_, err := c.Issues.Update(apiCtx(), project, iid, opts)
			return nil, err
		})
	},
}

// ─── issue bulk label ────────────────────────────────────────────────────────

var issueBulkLabelCmd = &cobra.Command{
	Use:   "label",
	Short: "Add or remove labels on many issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		add, _ := cmd.Flags().GetString("add")
		remove, _ := cmd.Flags().GetString("remove")
		if add == "" && remove == "" {
			return failArg("at least one of --add or --remove is required")
		}
		return runIssueBulk(cmd, "issue bulk label", map[string]any{"add": add, "remove": remove}, func(c *api.Client, project string, iid int) (map[string]any, error) {
			_, err := c.Issues.Update(apiCtx(), project, iid, api.IssueUpdateOpts{AddLabels: add, RemoveLabels: remove})
			return nil, err
		})
	},
}

// ─── issue bulk assign ───────────────────────────────────────────────────────

var issueBulkAssignCmd = &cobra.Command{
	Use:   "assign <username|me>",
	Short: "Assign many issues to a user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		assignee := args[0]
		// Resolve the username to a numeric ID once: the batch shares one assignee,
		// so one /users lookup covers the whole loop.
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		ids, err := parsePluralIntFlag(cmd, "ids")
		if err != nil {
			return err
		}
		targets := make([]string, len(ids))
		for i, id := range ids {
			targets[i] = strconv.Itoa(id)
		}
		if done, err := prepareBatchWrite(cmd, "issue bulk assign", targets, map[string]any{"project": project, "assignee": assignee}); done || err != nil {
			return err
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		uid, err := resolveUserID(client, assignee)
		if err != nil {
			return err
		}
		keepGoing := continueOnError(cmd)
		items := make([]batchItem, 0, len(ids))
		var unattempted []string
		for i, iid := range ids {
			_, err := client.Issues.Update(apiCtx(), project, iid, api.IssueUpdateOpts{AssigneeIDs: []int{uid}})
			if err != nil {
				items = append(items, batchItem{Target: targets[i], OK: false, Error: batchErrorFor(err)})
				setExitCode(ExitError)
				if !keepGoing {
					unattempted = targetsTail(targets, i+1)
					break
				}
				continue
			}
			items = append(items, batchItem{Target: targets[i], OK: true})
		}
		emitBatchResult(items, unattempted)
		return nil
	},
}

// ─── issue bulk comment ──────────────────────────────────────────────────────

var issueBulkCommentCmd = &cobra.Command{
	Use:   "comment",
	Short: "Add the same comment to many issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		body, _ := cmd.Flags().GetString("body")
		if body == "" {
			return failArg("--body is required")
		}
		return runIssueBulk(cmd, "issue bulk comment", nil, func(c *api.Client, project string, iid int) (map[string]any, error) {
			note, err := c.Issues.AddNote(apiCtx(), project, iid, body)
			if err != nil {
				return nil, err
			}
			return map[string]any{"noteId": note.ID}, nil
		})
	},
}

func init() {
	issueCmd.AddCommand(issueBulkCmd)

	// Every bulk subcommand shares --project, --ids (plural), and --continue-on-error.
	for _, c := range []*cobra.Command{
		issueBulkCloseCmd, issueBulkReopenCmd, issueBulkUpdateCmd,
		issueBulkLabelCmd, issueBulkAssignCmd, issueBulkCommentCmd,
	} {
		c.Flags().String("project", "", "Project ID or path (required)")
		c.Flags().StringSlice("ids", nil, "Issue IIDs (comma-separated or repeatable)")
		issueBulkCmd.AddCommand(c)
		markBatch(c, true)
	}

	issueBulkUpdateCmd.Flags().String("title", "", "New title")
	issueBulkUpdateCmd.Flags().String("description", "", "New description")
	issueBulkUpdateCmd.Flags().String("add-labels", "", "Labels to add (comma-separated)")
	issueBulkUpdateCmd.Flags().String("remove-labels", "", "Labels to remove (comma-separated)")
	issueBulkUpdateCmd.Flags().Int("milestone-id", 0, "Milestone ID")

	issueBulkLabelCmd.Flags().String("add", "", "Labels to add (comma-separated)")
	issueBulkLabelCmd.Flags().String("remove", "", "Labels to remove (comma-separated)")

	issueBulkCommentCmd.Flags().String("body", "", "Comment body (required)")
}
