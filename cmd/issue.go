package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var issueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Manage GitLab issues",
}

func init() {
	rootCmd.AddCommand(issueCmd)

	// list
	issueListCmd.Flags().String("project", "", "Project ID or path (required)")
	issueListCmd.Flags().String("state", "opened", "State: opened|closed|all")
	issueListCmd.Flags().String("assignee", "", "Filter by assignee username")
	issueListCmd.Flags().String("author", "", "Filter by author username")
	issueListCmd.Flags().String("label", "", "Filter by labels (comma-separated)")
	issueListCmd.Flags().String("search", "", "Search query")
	issueListCmd.Flags().String("milestone", "", "Filter by milestone title")
	issueListCmd.Flags().Int("limit", 20, "Max results (1-100)")
	issueListCmd.Flags().String("fields", "", "Comma-separated fields for JSON output")
	issueCmd.AddCommand(issueListCmd)

	// get
	issueGetCmd.Flags().String("project", "", "Project ID or path (required)")
	issueGetCmd.Flags().String("fields", "", "Comma-separated fields for JSON output")
	issueCmd.AddCommand(issueGetCmd)

	// create
	issueCreateCmd.Flags().String("project", "", "Project ID or path (required)")
	issueCreateCmd.Flags().String("title", "", "Issue title (required)")
	issueCreateCmd.Flags().String("description", "", "Issue description")
	issueCreateCmd.Flags().String("assignee", "", "Assignee username")
	issueCreateCmd.Flags().String("label", "", "Labels (comma-separated)")
	issueCreateCmd.Flags().Int("milestone-id", 0, "Milestone ID")
	issueCreateCmd.Flags().Bool("confidential", false, "Mark as confidential")
	issueCreateCmd.Flags().String("fields", "", "Comma-separated fields for JSON output")
	issueCmd.AddCommand(issueCreateCmd)
	markWrite(issueCreateCmd)

	// update
	issueUpdateCmd.Flags().String("project", "", "Project ID or path (required)")
	issueUpdateCmd.Flags().String("title", "", "New title")
	issueUpdateCmd.Flags().String("description", "", "New description")
	issueUpdateCmd.Flags().String("assignee", "", "New assignee username")
	issueUpdateCmd.Flags().String("add-labels", "", "Labels to add (comma-separated)")
	issueUpdateCmd.Flags().String("remove-labels", "", "Labels to remove (comma-separated)")
	issueUpdateCmd.Flags().Int("milestone-id", 0, "Milestone ID")
	issueCmd.AddCommand(issueUpdateCmd)
	markWrite(issueUpdateCmd)

	// close
	issueCloseCmd.Flags().String("project", "", "Project ID or path (required)")
	issueCmd.AddCommand(issueCloseCmd)
	markWrite(issueCloseCmd)
	markConfirm(issueCloseCmd)
	markRiskLevel(issueCloseCmd, "high")

	// reopen
	issueReopenCmd.Flags().String("project", "", "Project ID or path (required)")
	issueCmd.AddCommand(issueReopenCmd)
	markWrite(issueReopenCmd)

	// assign
	issueAssignCmd.Flags().String("project", "", "Project ID or path (required)")
	issueCmd.AddCommand(issueAssignCmd)
	markWrite(issueAssignCmd)

	// label (convenience)
	issueLabelCmd.Flags().String("project", "", "Project ID or path (required)")
	issueLabelCmd.Flags().String("add", "", "Labels to add (comma-separated)")
	issueLabelCmd.Flags().String("remove", "", "Labels to remove (comma-separated)")
	issueCmd.AddCommand(issueLabelCmd)
	markWrite(issueLabelCmd)
}

// ─── list ────────────────────────────────────────────────────────────────────

var issueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		state, _ := cmd.Flags().GetString("state")
		assignee, _ := cmd.Flags().GetString("assignee")
		author, _ := cmd.Flags().GetString("author")
		label, _ := cmd.Flags().GetString("label")
		search, _ := cmd.Flags().GetString("search")
		milestone, _ := cmd.Flags().GetString("milestone")
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		issues, err := client.Issues.List(apiCtx(), project, &api.IssueListOpts{
			State:            state,
			AssigneeUsername: assignee,
			AuthorUsername:   author,
			Labels:           label,
			Search:           search,
			Milestone:        milestone,
			Limit:            limit,
		})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			fields := getFieldsFlag(cmd)
			out := make([]map[string]any, len(issues))
			for i, iss := range issues {
				out[i] = output.FilterMap(output.IssueToMap(toFlatIssue(&iss)), fields)
			}
			printSimpleListJSON(cmd, out, limit)
			return nil
		}
		if len(issues) == 0 {
			output.Info("No issues found.")
			return nil
		}
		headers := []string{"IID", "TITLE", "STATE", "AUTHOR", "ASSIGNEE", "LABELS"}
		rows := make([][]string, len(issues))
		for i, iss := range issues {
			rows[i] = []string{
				fmt.Sprintf("%d", iss.IID),
				iss.Title,
				output.StatusBadge(iss.State),
				usernameOf(iss.Author),
				usernameOf(iss.Assignee),
				strings.Join(iss.Labels, ","),
			}
		}
		output.Table(headers, rows)
		return nil
	},
}

// ─── get ─────────────────────────────────────────────────────────────────────

var issueGetCmd = &cobra.Command{
	Use:   "get <iid>",
	Short: "Get a single issue",
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
			return failArg("iid must be a number")
		}
		iss, err := client.Issues.Get(apiCtx(), project, iid)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			output.PrintJSON(output.FilterMap(output.IssueToMap(toFlatIssue(iss)), fields))
			return nil
		}
		printIssueSummary(iss)
		return nil
	},
}

// ─── create ──────────────────────────────────────────────────────────────────

var issueCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create an issue",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		title, _ := cmd.Flags().GetString("title")
		if title == "" {
			return failArg("--title is required")
		}
		desc, _ := cmd.Flags().GetString("description")
		assigneeUser, _ := cmd.Flags().GetString("assignee")
		label, _ := cmd.Flags().GetString("label")
		milestoneID, _ := cmd.Flags().GetInt("milestone-id")
		confidential, _ := cmd.Flags().GetBool("confidential")

		if done, err := prepareWrite(cmd, "create issue", map[string]any{"project": project, "title": title}); done || err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		opts := api.IssueCreateOpts{
			Title:        title,
			Description:  desc,
			Labels:       label,
			MilestoneID:  milestoneID,
			Confidential: confidential,
		}
		if assigneeUser != "" {
			uid, err := resolveUserID(client, assigneeUser)
			if err != nil {
				return err
			}
			opts.AssigneeIDs = []int{uid}
		}

		iss, err := client.Issues.Create(apiCtx(), project, opts)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			output.PrintJSON(output.FilterMap(output.IssueToMap(toFlatIssue(iss)), fields))
			return nil
		}
		output.Success(fmt.Sprintf("Created issue #%d: %s", iss.IID, iss.Title))
		return nil
	},
}

// ─── update ──────────────────────────────────────────────────────────────────

var issueUpdateCmd = &cobra.Command{
	Use:   "update <iid>",
	Short: "Update an issue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		iid, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("iid must be a number")
		}
		title, _ := cmd.Flags().GetString("title")
		desc, _ := cmd.Flags().GetString("description")
		assigneeUser, _ := cmd.Flags().GetString("assignee")
		addLabels, _ := cmd.Flags().GetString("add-labels")
		removeLabels, _ := cmd.Flags().GetString("remove-labels")
		milestoneID, _ := cmd.Flags().GetInt("milestone-id")

		if done, err := prepareWrite(cmd, "update issue", map[string]any{"project": project, "iid": iid}); done || err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		opts := api.IssueUpdateOpts{
			Title:        title,
			Description:  desc,
			AddLabels:    addLabels,
			RemoveLabels: removeLabels,
			MilestoneID:  milestoneID,
		}
		if assigneeUser != "" {
			uid, err := resolveUserID(client, assigneeUser)
			if err != nil {
				return err
			}
			opts.AssigneeIDs = []int{uid}
		}

		iss, err := client.Issues.Update(apiCtx(), project, iid, opts)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.IssueToMap(toFlatIssue(iss)))
			return nil
		}
		output.Success(fmt.Sprintf("Updated issue #%d", iss.IID))
		return nil
	},
}

// ─── close ───────────────────────────────────────────────────────────────────

var issueCloseCmd = &cobra.Command{
	Use:   "close <iid>",
	Short: "Close an issue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		iid, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("iid must be a number")
		}
		confirmPayload := map[string]any{"project": project, "iid": iid}
		if done, err := prepareWrite(cmd, "close issue", confirmPayload); done || err != nil {
			return err
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		iss, err := client.Issues.Update(apiCtx(), project, iid, api.IssueUpdateOpts{StateEvent: "close"})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.IssueToMap(toFlatIssue(iss)))
			return nil
		}
		output.Success(fmt.Sprintf("Closed issue #%d", iss.IID))
		return nil
	},
}

// ─── reopen ──────────────────────────────────────────────────────────────────

var issueReopenCmd = &cobra.Command{
	Use:   "reopen <iid>",
	Short: "Reopen a closed issue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		iid, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("iid must be a number")
		}
		if done, err := prepareWrite(cmd, "reopen issue", map[string]any{"project": project, "iid": iid}); done || err != nil {
			return err
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		iss, err := client.Issues.Update(apiCtx(), project, iid, api.IssueUpdateOpts{StateEvent: "reopen"})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.IssueToMap(toFlatIssue(iss)))
			return nil
		}
		output.Success(fmt.Sprintf("Reopened issue #%d", iss.IID))
		return nil
	},
}

// ─── assign ──────────────────────────────────────────────────────────────────

var issueAssignCmd = &cobra.Command{
	Use:   "assign <iid> <username|me>",
	Short: "Assign an issue to a user",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		iid, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("iid must be a number")
		}
		if done, err := prepareWrite(cmd, "assign issue", map[string]any{"project": project, "iid": iid, "assignee": args[1]}); done || err != nil {
			return err
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		uid, err := resolveUserID(client, args[1])
		if err != nil {
			return err
		}
		iss, err := client.Issues.Update(apiCtx(), project, iid, api.IssueUpdateOpts{AssigneeIDs: []int{uid}})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.IssueToMap(toFlatIssue(iss)))
			return nil
		}
		output.Success(fmt.Sprintf("Assigned issue #%d to %s", iss.IID, args[1]))
		return nil
	},
}

// ─── label (convenience) ─────────────────────────────────────────────────────

var issueLabelCmd = &cobra.Command{
	Use:   "label <iid>",
	Short: "Add or remove labels on an issue",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		iid, err := strconv.Atoi(args[0])
		if err != nil {
			return failArg("iid must be a number")
		}
		add, _ := cmd.Flags().GetString("add")
		remove, _ := cmd.Flags().GetString("remove")
		if add == "" && remove == "" {
			return failArg("at least one of --add or --remove is required")
		}
		if done, err := prepareWrite(cmd, "label issue", map[string]any{"project": project, "iid": iid, "add": add, "remove": remove}); done || err != nil {
			return err
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		iss, err := client.Issues.Update(apiCtx(), project, iid, api.IssueUpdateOpts{AddLabels: add, RemoveLabels: remove})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.IssueToMap(toFlatIssue(iss)))
			return nil
		}
		output.Success(fmt.Sprintf("Updated labels on issue #%d", iss.IID))
		return nil
	},
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func toFlatIssue(iss *api.Issue) output.FlatIssue {
	f := output.FlatIssue{
		IID:    iss.IID,
		Title:  iss.Title,
		State:  iss.State,
		Labels: strings.Join(iss.Labels, ","),
		WebURL: iss.WebURL,
	}
	if iss.Author != nil {
		f.Author = iss.Author.Username
	}
	if iss.Assignee != nil {
		f.Assignee = iss.Assignee.Username
	}
	if iss.Milestone != nil {
		f.Milestone = iss.Milestone.Title
	}
	return f
}

func usernameOf(u *api.User) string {
	if u == nil {
		return ""
	}
	return u.Username
}

func printIssueSummary(iss *api.Issue) {
	fmt.Printf("Issue #%d: %s\n", iss.IID, iss.Title)
	fmt.Printf("  State:  %s\n", output.StatusBadge(iss.State))
	fmt.Printf("  Author: %s\n", usernameOf(iss.Author))
	if iss.Assignee != nil {
		fmt.Printf("  Assignee: %s\n", iss.Assignee.Username)
	}
	if len(iss.Labels) > 0 {
		fmt.Printf("  Labels: %s\n", strings.Join(iss.Labels, ", "))
	}
	if iss.Milestone != nil {
		fmt.Printf("  Milestone: %s\n", iss.Milestone.Title)
	}
	if iss.WebURL != "" {
		fmt.Printf("  URL: %s\n", iss.WebURL)
	}
	if iss.Description != "" {
		fmt.Printf("  Description:\n    %s\n", iss.Description)
	}
}

// readBodyFile reads a file and returns its content as a string.
func readBodyFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading body file: %w", err)
	}
	return strings.TrimRight(string(data), "\n"), nil
}
