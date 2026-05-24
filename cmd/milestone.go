package cmd

import (
	"fmt"
	"strconv"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var milestoneCmd = &cobra.Command{
	Use:   "milestone",
	Short: "Manage project milestones",
}

func init() {
	rootCmd.AddCommand(milestoneCmd)

	// list
	milestoneListCmd.Flags().String("project", "", "Project ID or path (required)")
	milestoneListCmd.Flags().String("state", "active", "State: active|closed|all")
	milestoneListCmd.Flags().Int("limit", 20, "Max results (1-100)")
	milestoneListCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	milestoneCmd.AddCommand(milestoneListCmd)

	// get
	milestoneGetCmd.Flags().String("project", "", "Project ID or path (required)")
	milestoneGetCmd.Flags().Int("milestone-id", 0, "Milestone ID (required)")
	milestoneGetCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	milestoneCmd.AddCommand(milestoneGetCmd)

	// create
	milestoneCreateCmd.Flags().String("project", "", "Project ID or path (required)")
	milestoneCreateCmd.Flags().String("title", "", "Milestone title (required)")
	milestoneCreateCmd.Flags().String("description", "", "Milestone description")
	milestoneCreateCmd.Flags().String("start-date", "", "Start date (YYYY-MM-DD)")
	milestoneCreateCmd.Flags().String("due-date", "", "Due date (YYYY-MM-DD)")
	milestoneCmd.AddCommand(milestoneCreateCmd)
	markWrite(milestoneCreateCmd)

	// update
	milestoneUpdateCmd.Flags().String("project", "", "Project ID or path (required)")
	milestoneUpdateCmd.Flags().Int("milestone-id", 0, "Milestone ID (required)")
	milestoneUpdateCmd.Flags().String("title", "", "New title")
	milestoneUpdateCmd.Flags().String("description", "", "New description")
	milestoneUpdateCmd.Flags().String("start-date", "", "New start date (YYYY-MM-DD)")
	milestoneUpdateCmd.Flags().String("due-date", "", "New due date (YYYY-MM-DD)")
	milestoneUpdateCmd.Flags().String("state-event", "", "State event: close|activate")
	milestoneCmd.AddCommand(milestoneUpdateCmd)
	markWrite(milestoneUpdateCmd)

	// close (alias for update --state-event close)
	milestoneCloseCmd.Flags().String("project", "", "Project ID or path (required)")
	milestoneCloseCmd.Flags().Int("milestone-id", 0, "Milestone ID (required)")
	milestoneCmd.AddCommand(milestoneCloseCmd)
	markWrite(milestoneCloseCmd)
	markConfirm(milestoneCloseCmd)
}

var milestoneListCmd = &cobra.Command{
	Use:   "list",
	Short: "List milestones",
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
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}
		ms, err := client.Milestones.List(apiCtx(), project, &api.MilestoneListOpts{State: state, Limit: limit})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			out := make([]map[string]any, len(ms))
			for i, m := range ms {
				out[i] = output.FilterMap(output.MilestoneToMap(toFlatMilestone(&m)), fields)
			}
			output.PrintJSON(out)
			return nil
		}
		if len(ms) == 0 {
			output.Info("No milestones found.")
			return nil
		}
		headers := []string{"ID", "TITLE", "STATE", "DUE DATE"}
		rows := make([][]string, len(ms))
		for i, m := range ms {
			rows[i] = []string{
				fmt.Sprintf("%d", m.ID),
				m.Title,
				output.StatusBadge(m.State),
				m.DueDate,
			}
		}
		output.Table(headers, rows)
		return nil
	},
}

var milestoneGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a milestone",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		milestoneID, _ := cmd.Flags().GetInt("milestone-id")
		if milestoneID == 0 {
			return failArg("--milestone-id is required")
		}
		m, err := client.Milestones.GetByID(apiCtx(), project, milestoneID)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			output.PrintJSON(output.FilterMap(output.MilestoneToMap(toFlatMilestone(m)), fields))
			return nil
		}
		fmt.Printf("Milestone #%d: %s\n", m.ID, m.Title)
		fmt.Printf("  State:    %s\n", output.StatusBadge(m.State))
		if m.StartDate != "" {
			fmt.Printf("  Start:    %s\n", m.StartDate)
		}
		if m.DueDate != "" {
			fmt.Printf("  Due:      %s\n", m.DueDate)
		}
		if m.Description != "" {
			fmt.Printf("  Description: %s\n", m.Description)
		}
		return nil
	},
}

var milestoneCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a milestone",
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
		startDate, _ := cmd.Flags().GetString("start-date")
		dueDate, _ := cmd.Flags().GetString("due-date")

		if dryRunOutput("create milestone", map[string]any{"project": project, "title": title}) {
			return nil
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		m, err := client.Milestones.Create(apiCtx(), project, api.MilestoneCreateOpts{
			Title:       title,
			Description: desc,
			StartDate:   startDate,
			DueDate:     dueDate,
		})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.MilestoneToMap(toFlatMilestone(m)))
			return nil
		}
		output.Success(fmt.Sprintf("Created milestone %q (ID %d)", m.Title, m.ID))
		return nil
	},
}

var milestoneUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update a milestone",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		milestoneID, _ := cmd.Flags().GetInt("milestone-id")
		if milestoneID == 0 {
			return failArg("--milestone-id is required")
		}
		title, _ := cmd.Flags().GetString("title")
		desc, _ := cmd.Flags().GetString("description")
		startDate, _ := cmd.Flags().GetString("start-date")
		dueDate, _ := cmd.Flags().GetString("due-date")
		stateEvent, _ := cmd.Flags().GetString("state-event")

		if dryRunOutput("update milestone", map[string]any{"project": project, "milestoneId": milestoneID}) {
			return nil
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		m, err := client.Milestones.Update(apiCtx(), project, milestoneID, api.MilestoneUpdateOpts{
			Title:       title,
			Description: desc,
			StartDate:   startDate,
			DueDate:     dueDate,
			StateEvent:  stateEvent,
		})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.MilestoneToMap(toFlatMilestone(m)))
			return nil
		}
		output.Success(fmt.Sprintf("Updated milestone %q (ID %d)", m.Title, m.ID))
		return nil
	},
}

var milestoneCloseCmd = &cobra.Command{
	Use:   "close",
	Short: "Close a milestone",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		milestoneID, _ := cmd.Flags().GetInt("milestone-id")
		if milestoneID == 0 {
			return failArg("--milestone-id is required")
		}
		if dryRunOutput("close milestone", map[string]any{"project": project, "milestoneId": milestoneID}) {
			return nil
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		if err := requireConfirm(cmd, fmt.Sprintf("Type the milestone ID (%d) to confirm close", milestoneID), strconv.Itoa(milestoneID)); err != nil {
			return err
		}
		m, err := client.Milestones.Update(apiCtx(), project, milestoneID, api.MilestoneUpdateOpts{StateEvent: "close"})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.MilestoneToMap(toFlatMilestone(m)))
			return nil
		}
		output.Success(fmt.Sprintf("Closed milestone %q (ID %d)", m.Title, m.ID))
		return nil
	},
}

func toFlatMilestone(m *api.Milestone) output.FlatMilestone {
	return output.FlatMilestone{
		ID:        m.ID,
		IID:       m.IID,
		Title:     m.Title,
		State:     m.State,
		StartDate: m.StartDate,
		DueDate:   m.DueDate,
		WebURL:    m.WebURL,
	}
}
