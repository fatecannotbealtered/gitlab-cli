package cmd

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/gitctx"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var mrCmd = &cobra.Command{
	Use:   "mr",
	Short: "Manage merge requests",
}

func init() {
	rootCmd.AddCommand(mrCmd)

	// list
	mrListCmd.Flags().String("project", "", "Project ID or path (required)")
	mrListCmd.Flags().String("state", "opened", "State: opened|closed|merged|all")
	mrListCmd.Flags().String("assignee", "", "Filter by assignee username")
	mrListCmd.Flags().String("author", "", "Filter by author username")
	mrListCmd.Flags().String("label", "", "Filter by labels (comma-separated)")
	mrListCmd.Flags().String("search", "", "Search query")
	mrListCmd.Flags().String("source-branch", "", "Filter by source branch")
	mrListCmd.Flags().String("target-branch", "", "Filter by target branch")
	registerListFlags(mrListCmd)
	mrListCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	mrCmd.AddCommand(mrListCmd)

	// get
	mrGetCmd.Flags().String("project", "", "Project ID or path (required)")
	mrGetCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	mrCmd.AddCommand(mrGetCmd)

	// current
	mrCmd.AddCommand(mrCurrentCmd)

	// create
	mrCreateCmd.Flags().String("project", "", "Project ID or path")
	mrCreateCmd.Flags().String("title", "", "MR title")
	mrCreateCmd.Flags().String("source-branch", "", "Source branch")
	mrCreateCmd.Flags().String("target-branch", "main", "Target branch")
	mrCreateCmd.Flags().String("description", "", "MR description")
	mrCreateCmd.Flags().String("assignee", "", "Assignee username")
	mrCreateCmd.Flags().String("label", "", "Labels (comma-separated)")
	mrCreateCmd.Flags().Bool("draft", false, "Mark as draft")
	mrCreateCmd.Flags().Bool("squash", false, "Squash commits on merge")
	mrCreateCmd.Flags().Bool("remove-source-branch", false, "Remove source branch after merge")
	mrCreateCmd.Flags().Bool("auto", false, "Derive project and source-branch from git context")
	mrCreateCmd.Flags().Bool("find-existing", false, "Return existing open MR for source branch instead of creating")
	mrCmd.AddCommand(mrCreateCmd)
	markWrite(mrCreateCmd)

	// update
	mrUpdateCmd.Flags().String("project", "", "Project ID or path (required)")
	mrUpdateCmd.Flags().String("title", "", "New title")
	mrUpdateCmd.Flags().String("description", "", "New description")
	mrUpdateCmd.Flags().String("assignee", "", "Assignee username")
	mrUpdateCmd.Flags().String("add-labels", "", "Labels to add (comma-separated)")
	mrUpdateCmd.Flags().String("remove-labels", "", "Labels to remove (comma-separated)")
	mrUpdateCmd.Flags().String("target-branch", "", "New target branch")
	mrCmd.AddCommand(mrUpdateCmd)
	markWrite(mrUpdateCmd)

	// merge
	mrMergeCmd.Flags().String("project", "", "Project ID or path (required)")
	mrMergeCmd.Flags().Bool("squash", false, "Squash commits")
	mrMergeCmd.Flags().Bool("should-remove-source-branch", false, "Remove source branch after merge")
	mrMergeCmd.Flags().String("merge-commit-message", "", "Custom merge commit message")
	mrMergeCmd.Flags().String("sha", "", "Expected SHA to merge")
	mrCmd.AddCommand(mrMergeCmd)
	markWrite(mrMergeCmd)
	markConfirm(mrMergeCmd)
	markRiskLevel(mrMergeCmd, "critical")

	// close
	mrCloseCmd.Flags().String("project", "", "Project ID or path (required)")
	mrCmd.AddCommand(mrCloseCmd)
	markWrite(mrCloseCmd)
	markConfirm(mrCloseCmd)
	markRiskLevel(mrCloseCmd, "high")

	// reopen
	mrReopenCmd.Flags().String("project", "", "Project ID or path (required)")
	mrCmd.AddCommand(mrReopenCmd)
	markWrite(mrReopenCmd)

	// approve
	mrApproveCmd.Flags().String("project", "", "Project ID or path (required)")
	mrCmd.AddCommand(mrApproveCmd)
	markWrite(mrApproveCmd)

	// unapprove
	mrUnapproveCmd.Flags().String("project", "", "Project ID or path (required)")
	mrCmd.AddCommand(mrUnapproveCmd)
	markWrite(mrUnapproveCmd)

	// diff
	mrDiffCmd.Flags().String("project", "", "Project ID or path (required)")
	mrCmd.AddCommand(mrDiffCmd)
	markOutputType(mrDiffCmd, "json")
	markOutputFormats(mrDiffCmd, formatJSON, formatText, formatRaw)
}

// 鈹€鈹€鈹€ list 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var mrListCmd = &cobra.Command{
	Use:   "list",
	Short: "List merge requests",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, err := requireFlagString(cmd, "project", "--project")
		if err != nil {
			return err
		}
		state, _ := cmd.Flags().GetString("state")
		assignee, _ := cmd.Flags().GetString("assignee")
		author, _ := cmd.Flags().GetString("author")
		label, _ := cmd.Flags().GetString("label")
		search, _ := cmd.Flags().GetString("search")
		srcBranch, _ := cmd.Flags().GetString("source-branch")
		tgtBranch, _ := cmd.Flags().GetString("target-branch")
		limit, fetchAll, err := listLimitFromFlags(cmd)
		if err != nil {
			return err
		}

		mrs, err := client.MergeRequests.List(apiCtx(), project, &api.MergeRequestListOpts{
			State:        state,
			AssigneeUser: assignee,
			AuthorUser:   author,
			Labels:       label,
			Search:       search,
			SourceBranch: srcBranch,
			TargetBranch: tgtBranch,
			Limit:        limit,
		})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			rows := make([]map[string]any, len(mrs))
			for i, m := range mrs {
				rows[i] = output.MRToMap(output.ToFlatMR(&m))
			}
			pag := api.Pagination{PerPage: limit}
			if len(mrs) >= limit && !fetchAll {
				pag.NextPage = 2
			}
			printListJSON(cmd, rows, pag, limit, fetchAll)
			return nil
		}
		if len(mrs) == 0 {
			output.Info("No merge requests found.")
			return nil
		}
		headers := []string{"IID", "TITLE", "STATE", "SOURCE", "TARGET", "AUTHOR"}
		rows := make([][]string, len(mrs))
		for i, m := range mrs {
			author := ""
			if m.Author != nil {
				author = m.Author.Username
			}
			rows[i] = []string{
				strconv.Itoa(m.IID),
				m.Title,
				output.StatusBadge(m.State),
				m.SourceBranch,
				m.TargetBranch,
				author,
			}
		}
		output.Table(headers, rows)
		return nil
	},
}

// 鈹€鈹€鈹€ get 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var mrGetCmd = &cobra.Command{
	Use:   "get <iid>",
	Short: "Get a merge request by IID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		iid, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("iid must be an integer")
		}
		mr, err := client.MergeRequests.Get(apiCtx(), project, iid)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			output.PrintJSON(output.FilterMap(output.MRToMap(output.ToFlatMR(mr)), fields))
			return nil
		}
		printMRDetail(mr)
		return nil
	},
}

// 鈹€鈹€鈹€ current 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var mrCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show the open MR for the current git branch",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, _ := gitctx.Detect("")
		if ctx.Remote == nil || ctx.CurrentBranch == "" {
			return failNotFound("not inside a git repo with a GitLab remote; run this command from a cloned GitLab project")
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		mrs, err := client.MergeRequests.List(apiCtx(), ctx.Remote.ProjectPath, &api.MergeRequestListOpts{
			State:        "opened",
			SourceBranch: ctx.CurrentBranch,
			Limit:        1,
		})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if len(mrs) == 0 {
			if jsonMode {
				return failNotFound(fmt.Sprintf("no open MR found for branch %q", ctx.CurrentBranch))
			}
			output.Info(fmt.Sprintf("No open MR found for branch %q", ctx.CurrentBranch))
			setExitCode(ExitNotFound)
			return ErrSilent
		}
		mr := &mrs[0]
		if jsonMode {
			output.PrintJSON(output.MRToMap(output.ToFlatMR(mr)))
			return nil
		}
		printMRDetail(mr)
		return nil
	},
}

// 鈹€鈹€鈹€ create 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var mrCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a merge request",
	RunE: func(cmd *cobra.Command, args []string) error {
		auto, _ := cmd.Flags().GetBool("auto")
		project, _ := cmd.Flags().GetString("project")
		srcBranch, _ := cmd.Flags().GetString("source-branch")
		title, _ := cmd.Flags().GetString("title")
		tgtBranch, _ := cmd.Flags().GetString("target-branch")
		description, _ := cmd.Flags().GetString("description")
		assignee, _ := cmd.Flags().GetString("assignee")
		label, _ := cmd.Flags().GetString("label")
		draft, _ := cmd.Flags().GetBool("draft")
		squash, _ := cmd.Flags().GetBool("squash")
		removeSrc, _ := cmd.Flags().GetBool("remove-source-branch")

		if auto {
			ctx, err := gitctx.Detect("")
			if err != nil || ctx.Remote == nil || ctx.CurrentBranch == "" {
				return failArg("--auto requires a git repo with a GitLab remote")
			}
			if project == "" {
				project = ctx.Remote.ProjectPath
			}
			if srcBranch == "" {
				srcBranch = ctx.CurrentBranch
			}
			if title == "" {
				title = gitCommitSubject()
			}
		}

		if project == "" {
			return failArg("--project is required (or use --auto)")
		}
		if title == "" {
			return failArg("--title is required")
		}
		if srcBranch == "" {
			return failArg("--source-branch is required (or use --auto)")
		}

		findExisting, _ := cmd.Flags().GetBool("find-existing")

		if dryRunOutput("create mr", map[string]any{
			"project":      project,
			"title":        title,
			"sourceBranch": srcBranch,
			"targetBranch": tgtBranch,
		}) {
			return nil
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		if findExisting {
			existing, listErr := client.MergeRequests.List(apiCtx(), project, &api.MergeRequestListOpts{
				State:        "opened",
				SourceBranch: srcBranch,
				TargetBranch: tgtBranch,
				Limit:        5,
			})
			if listErr != nil {
				return handleAPIError(listErr, jsonMode)
			}
			if len(existing) > 0 {
				mr := &existing[0]
				if jsonMode {
					out := output.MRToMap(output.ToFlatMR(mr))
					out["existing"] = true
					output.PrintJSON(out)
				} else {
					output.Info(fmt.Sprintf("Existing MR !%d: %s", mr.IID, mr.Title))
					output.Info(mr.WebURL)
				}
				return nil
			}
		}

		req := &api.MergeRequestCreateRequest{
			Title:              title,
			SourceBranch:       srcBranch,
			TargetBranch:       tgtBranch,
			Description:        description,
			Labels:             label,
			Draft:              draft,
			Squash:             squash,
			RemoveSourceBranch: removeSrc,
		}

		if assignee != "" {
			uid, err := resolveUserID(client, assignee)
			if err != nil {
				return err
			}
			req.AssigneeID = uid
		}

		mr, err := client.MergeRequests.Create(apiCtx(), project, req)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.MRToMap(output.ToFlatMR(mr)))
			return nil
		}
		output.Success(fmt.Sprintf("Created MR !%d: %s", mr.IID, mr.Title))
		output.Info(mr.WebURL)
		return nil
	},
}

// gitCommitSubject returns the subject of the latest git commit.
func gitCommitSubject() string {
	out, err := exec.Command("git", "log", "-1", "--pretty=%s").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// 鈹€鈹€鈹€ update 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var mrUpdateCmd = &cobra.Command{
	Use:   "update <iid>",
	Short: "Update a merge request",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		iid, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("iid must be an integer")
		}
		title, _ := cmd.Flags().GetString("title")
		desc, _ := cmd.Flags().GetString("description")
		assignee, _ := cmd.Flags().GetString("assignee")
		addLabels, _ := cmd.Flags().GetString("add-labels")
		removeLabels, _ := cmd.Flags().GetString("remove-labels")
		tgtBranch, _ := cmd.Flags().GetString("target-branch")

		if dryRunOutput("update mr", map[string]any{"project": project, "iid": iid}) {
			return nil
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		req := &api.MergeRequestUpdateRequest{
			Title:        title,
			Description:  desc,
			AddLabels:    addLabels,
			RemoveLabels: removeLabels,
			TargetBranch: tgtBranch,
		}
		if assignee != "" {
			u, err := client.Users.GetByUsername(apiCtx(), assignee)
			if err != nil {
				return handleAPIError(err, jsonMode)
			}
			if u != nil {
				req.AssigneeIDs = []int{u.ID}
			}
		}

		mr, err := client.MergeRequests.Update(apiCtx(), project, iid, req)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.MRToMap(output.ToFlatMR(mr)))
			return nil
		}
		output.Success(fmt.Sprintf("Updated MR !%d", mr.IID))
		return nil
	},
}

// 鈹€鈹€鈹€ merge 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var mrMergeCmd = &cobra.Command{
	Use:   "merge <iid>",
	Short: "Merge a merge request",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		iid, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("iid must be an integer")
		}
		squash, _ := cmd.Flags().GetBool("squash")
		removeSrc, _ := cmd.Flags().GetBool("should-remove-source-branch")
		msg, _ := cmd.Flags().GetString("merge-commit-message")
		sha, _ := cmd.Flags().GetString("sha")

		confirmPayload := map[string]any{
			"project":                  project,
			"iid":                      iid,
			"squash":                   squash,
			"shouldRemoveSourceBranch": removeSrc,
			"mergeCommitMessage":       msg,
			"sha":                      sha,
		}
		if dryRunOutput("merge mr", confirmPayload) {
			return nil
		}
		if err := requireConfirm(cmd, "merge mr", confirmPayload); err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}
		mr, err := client.MergeRequests.Merge(apiCtx(), project, iid, &api.MergeRequestMergeRequest{
			Squash:                   squash,
			ShouldRemoveSourceBranch: removeSrc,
			MergeCommitMessage:       msg,
			SHA:                      sha,
		})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.MRToMap(output.ToFlatMR(mr)))
			return nil
		}
		output.Success(fmt.Sprintf("Merged MR !%d", mr.IID))
		return nil
	},
}

// 鈹€鈹€鈹€ close 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var mrCloseCmd = &cobra.Command{
	Use:   "close <iid>",
	Short: "Close a merge request",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		iid, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("iid must be an integer")
		}
		confirmPayload := map[string]any{"project": project, "iid": iid}
		if dryRunOutput("close mr", confirmPayload) {
			return nil
		}
		if err := requireConfirm(cmd, "close mr", confirmPayload); err != nil {
			return err
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		mr, err := client.MergeRequests.Close(apiCtx(), project, iid)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.MRToMap(output.ToFlatMR(mr)))
			return nil
		}
		output.Success(fmt.Sprintf("Closed MR !%d", mr.IID))
		return nil
	},
}

// 鈹€鈹€鈹€ reopen 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var mrReopenCmd = &cobra.Command{
	Use:   "reopen <iid>",
	Short: "Reopen a merge request",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		iid, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("iid must be an integer")
		}
		if dryRunOutput("reopen mr", map[string]any{"project": project, "iid": iid}) {
			return nil
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		mr, err := client.MergeRequests.Reopen(apiCtx(), project, iid)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.MRToMap(output.ToFlatMR(mr)))
			return nil
		}
		output.Success(fmt.Sprintf("Reopened MR !%d", mr.IID))
		return nil
	},
}

// 鈹€鈹€鈹€ approve 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var mrApproveCmd = &cobra.Command{
	Use:   "approve <iid>",
	Short: "Approve a merge request",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		iid, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("iid must be an integer")
		}
		if dryRunOutput("approve mr", map[string]any{"project": project, "iid": iid}) {
			return nil
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		if err := client.MergeRequests.Approve(apiCtx(), project, iid); err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(map[string]any{"iid": iid, "approved": true})
			return nil
		}
		output.Success(fmt.Sprintf("Approved MR !%d", iid))
		return nil
	},
}

// 鈹€鈹€鈹€ unapprove 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var mrUnapproveCmd = &cobra.Command{
	Use:   "unapprove <iid>",
	Short: "Remove approval from a merge request",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		iid, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("iid must be an integer")
		}
		if dryRunOutput("unapprove mr", map[string]any{"project": project, "iid": iid}) {
			return nil
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		if err := client.MergeRequests.Unapprove(apiCtx(), project, iid); err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(map[string]any{"iid": iid, "approved": false})
			return nil
		}
		output.Success(fmt.Sprintf("Removed approval from MR !%d", iid))
		return nil
	},
}

// diff command

var mrDiffCmd = &cobra.Command{
	Use:   "diff <iid>",
	Short: "Show unified diff for a merge request",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		iid, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("iid must be an integer")
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		diff, err := client.MergeRequests.GetRawDiff(apiCtx(), project, iid)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(map[string]any{"diff": diff})
			return nil
		}
		if formatMode == formatRaw {
			fmt.Print(diff)
			return nil
		}
		if !quietMode {
			fmt.Println(diff)
		}
		return nil
	},
}

// 鈹€鈹€鈹€ helpers 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func printMRDetail(mr *api.MergeRequest) {
	author := ""
	if mr.Author != nil {
		author = mr.Author.Username
	}
	fmt.Printf("!%d  %s\n", mr.IID, mr.Title)
	fmt.Printf("State:  %s\n", output.StatusBadge(mr.State))
	fmt.Printf("Source: %s 鈫?%s\n", mr.SourceBranch, mr.TargetBranch)
	if author != "" {
		fmt.Printf("Author: %s\n", author)
	}
	if mr.WebURL != "" {
		fmt.Printf("URL:    %s\n", mr.WebURL)
	}
}
