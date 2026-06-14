package cmd

import (
	"strconv"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/spf13/cobra"
)

// mr bulk is a class-B batch (CLI-SPEC §15.7): GitLab has no public bulk REST for
// MR state/approval/merge, so the command loops client-side over --ids. bulk
// merge is irreversible/high-blast, so it is registered write-dangerous and
// requires the §15.4 two-step gate: --dangerous declares intent, the confirm
// token authorizes the resolved batch. Dangerous batches also default
// --continue-on-error to false (stop at first failure; no rollback).

var mrBulkCmd = &cobra.Command{
	Use:   "bulk",
	Short: "Run a write action across many merge requests in one call",
}

// bulkMRAction performs one MR mutation and returns optional per-item detail.
type bulkMRAction func(client *api.Client, project string, iid int) (map[string]any, error)

func runMRBulk(cmd *cobra.Command, action string, extra map[string]any, fn bulkMRAction) error {
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

// ─── mr bulk merge (dangerous) ───────────────────────────────────────────────

var mrBulkMergeCmd = &cobra.Command{
	Use:   "merge",
	Short: "Merge many merge requests (write-dangerous; requires --dangerous)",
	RunE: func(cmd *cobra.Command, args []string) error {
		squash, _ := cmd.Flags().GetBool("squash")
		removeSource, _ := cmd.Flags().GetBool("should-remove-source-branch")
		req := &api.MergeRequestMergeRequest{
			Squash:                   squash,
			ShouldRemoveSourceBranch: removeSource,
		}
		return runMRBulk(cmd, "mr bulk merge", nil, func(c *api.Client, project string, iid int) (map[string]any, error) {
			_, err := c.MergeRequests.Merge(apiCtx(), project, iid, req)
			return map[string]any{"action": "merge"}, err
		})
	},
}

// ─── mr bulk approve ─────────────────────────────────────────────────────────

var mrBulkApproveCmd = &cobra.Command{
	Use:   "approve",
	Short: "Approve many merge requests",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMRBulk(cmd, "mr bulk approve", nil, func(c *api.Client, project string, iid int) (map[string]any, error) {
			return map[string]any{"action": "approve"}, c.MergeRequests.Approve(apiCtx(), project, iid)
		})
	},
}

// ─── mr bulk close ───────────────────────────────────────────────────────────

var mrBulkCloseCmd = &cobra.Command{
	Use:   "close",
	Short: "Close many merge requests",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runMRBulk(cmd, "mr bulk close", nil, func(c *api.Client, project string, iid int) (map[string]any, error) {
			_, err := c.MergeRequests.Close(apiCtx(), project, iid)
			return map[string]any{"action": "close"}, err
		})
	},
}

// ─── mr bulk update ──────────────────────────────────────────────────────────

var mrBulkUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update many merge requests (title/description/labels/target-branch)",
	RunE: func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		desc, _ := cmd.Flags().GetString("description")
		addLabels, _ := cmd.Flags().GetString("add-labels")
		removeLabels, _ := cmd.Flags().GetString("remove-labels")
		targetBranch, _ := cmd.Flags().GetString("target-branch")
		if title == "" && desc == "" && addLabels == "" && removeLabels == "" && targetBranch == "" {
			return failArg("at least one of --title, --description, --add-labels, --remove-labels, --target-branch is required")
		}
		req := &api.MergeRequestUpdateRequest{
			Title:        title,
			Description:  desc,
			AddLabels:    addLabels,
			RemoveLabels: removeLabels,
			TargetBranch: targetBranch,
		}
		return runMRBulk(cmd, "mr bulk update", nil, func(c *api.Client, project string, iid int) (map[string]any, error) {
			_, err := c.MergeRequests.Update(apiCtx(), project, iid, req)
			return nil, err
		})
	},
}

func init() {
	mrCmd.AddCommand(mrBulkCmd)

	for _, c := range []*cobra.Command{mrBulkApproveCmd, mrBulkCloseCmd, mrBulkUpdateCmd} {
		c.Flags().String("project", "", "Project ID or path (required)")
		c.Flags().StringSlice("ids", nil, "MR IIDs (comma-separated or repeatable)")
		mrBulkCmd.AddCommand(c)
		markBatch(c, true)
	}

	// merge is dangerous: default --continue-on-error to false so a failed merge
	// stops the batch rather than charging ahead through the rest.
	mrBulkMergeCmd.Flags().String("project", "", "Project ID or path (required)")
	mrBulkMergeCmd.Flags().StringSlice("ids", nil, "MR IIDs (comma-separated or repeatable)")
	mrBulkMergeCmd.Flags().Bool("squash", false, "Squash commits")
	mrBulkMergeCmd.Flags().Bool("should-remove-source-branch", false, "Remove source branch after merge")
	mrBulkCmd.AddCommand(mrBulkMergeCmd)
	markBatch(mrBulkMergeCmd, false)
	markConfirm(mrBulkMergeCmd)
	markRiskLevel(mrBulkMergeCmd, "critical")

	mrBulkUpdateCmd.Flags().String("title", "", "New title")
	mrBulkUpdateCmd.Flags().String("description", "", "New description")
	mrBulkUpdateCmd.Flags().String("add-labels", "", "Labels to add (comma-separated)")
	mrBulkUpdateCmd.Flags().String("remove-labels", "", "Labels to remove (comma-separated)")
	mrBulkUpdateCmd.Flags().String("target-branch", "", "New target branch")
}
