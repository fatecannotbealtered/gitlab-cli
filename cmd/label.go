package cmd

import (
	"fmt"
	"strconv"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var labelCmd = &cobra.Command{
	Use:   "label",
	Short: "Manage project labels",
}

func init() {
	rootCmd.AddCommand(labelCmd)

	// list
	labelListCmd.Flags().String("project", "", "Project ID or path (required)")
	labelListCmd.Flags().Int("limit", 20, "Max results (1-100)")
	labelListCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	labelCmd.AddCommand(labelListCmd)

	// create
	labelCreateCmd.Flags().String("project", "", "Project ID or path (required)")
	labelCreateCmd.Flags().String("name", "", "Label name (required)")
	labelCreateCmd.Flags().String("color", "", "Label color (#hex or name, required)")
	labelCreateCmd.Flags().String("description", "", "Label description")
	labelCreateCmd.Flags().Int("priority", 0, "Label priority")
	labelCreateCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	labelCmd.AddCommand(labelCreateCmd)
	markWrite(labelCreateCmd)

	// update
	labelUpdateCmd.Flags().String("project", "", "Project ID or path (required)")
	labelUpdateCmd.Flags().Int("label-id", 0, "Label ID (required)")
	labelUpdateCmd.Flags().String("name", "", "New label name")
	labelUpdateCmd.Flags().String("color", "", "New color (#hex or name)")
	labelUpdateCmd.Flags().String("description", "", "New description")
	labelUpdateCmd.Flags().Int("priority", 0, "New priority")
	labelCmd.AddCommand(labelUpdateCmd)
	markWrite(labelUpdateCmd)

	// delete
	labelDeleteCmd.Flags().String("project", "", "Project ID or path (required)")
	labelDeleteCmd.Flags().Int("label-id", 0, "Label ID (required)")
	labelCmd.AddCommand(labelDeleteCmd)
	markWrite(labelDeleteCmd)
	markConfirm(labelDeleteCmd)
}

var labelListCmd = &cobra.Command{
	Use:   "list",
	Short: "List project labels",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}
		labels, err := client.Labels.List(apiCtx(), project, limit)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			out := make([]map[string]any, len(labels))
			for i, l := range labels {
				out[i] = output.FilterMap(output.LabelToMap(toFlatLabel(&l)), fields)
			}
			output.PrintJSON(out)
			return nil
		}
		if len(labels) == 0 {
			output.Info("No labels found.")
			return nil
		}
		headers := []string{"ID", "NAME", "COLOR", "DESCRIPTION"}
		rows := make([][]string, len(labels))
		for i, l := range labels {
			rows[i] = []string{
				fmt.Sprintf("%d", l.ID),
				l.Name,
				l.Color,
				l.Description,
			}
		}
		output.Table(headers, rows)
		return nil
	},
}

var labelCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a label",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		name, _ := cmd.Flags().GetString("name")
		if name == "" {
			return failArg("--name is required")
		}
		color, _ := cmd.Flags().GetString("color")
		if color == "" {
			return failArg("--color is required")
		}
		desc, _ := cmd.Flags().GetString("description")
		priority, _ := cmd.Flags().GetInt("priority")

		if dryRunOutput("create label", map[string]any{"project": project, "name": name, "color": color}) {
			return nil
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		opts := api.LabelCreateOpts{Name: name, Color: color, Description: desc}
		if priority != 0 {
			opts.Priority = &priority
		}
		label, err := client.Labels.Create(apiCtx(), project, opts)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			output.PrintJSON(output.FilterMap(output.LabelToMap(toFlatLabel(label)), fields))
			return nil
		}
		output.Success(fmt.Sprintf("Created label %q (ID %d)", label.Name, label.ID))
		return nil
	},
}

var labelUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update a label",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		labelID, _ := cmd.Flags().GetInt("label-id")
		if labelID == 0 {
			return failArg("--label-id is required")
		}
		name, _ := cmd.Flags().GetString("name")
		color, _ := cmd.Flags().GetString("color")
		desc, _ := cmd.Flags().GetString("description")
		priority, _ := cmd.Flags().GetInt("priority")

		if dryRunOutput("update label", map[string]any{"project": project, "labelId": labelID}) {
			return nil
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		opts := api.LabelUpdateOpts{NewName: name, Color: color, Description: desc}
		if priority != 0 {
			opts.Priority = &priority
		}
		label, err := client.Labels.Update(apiCtx(), project, labelID, opts)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.LabelToMap(toFlatLabel(label)))
			return nil
		}
		output.Success(fmt.Sprintf("Updated label %q (ID %d)", label.Name, label.ID))
		return nil
	},
}

var labelDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a label",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		labelID, _ := cmd.Flags().GetInt("label-id")
		if labelID == 0 {
			return failArg("--label-id is required")
		}
		if dryRunOutput("delete label", map[string]any{"project": project, "labelId": labelID}) {
			return nil
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		if err := requireConfirm(cmd, fmt.Sprintf("Type the label ID (%d) to confirm deletion", labelID), strconv.Itoa(labelID)); err != nil {
			return err
		}
		if err := client.Labels.Delete(apiCtx(), project, labelID); err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(map[string]any{"deleted": true, "labelId": labelID})
			return nil
		}
		output.Success(fmt.Sprintf("Deleted label ID %d", labelID))
		return nil
	},
}

func toFlatLabel(l *api.Label) output.FlatLabel {
	return output.FlatLabel{
		ID:          l.ID,
		Name:        l.Name,
		Color:       l.Color,
		Description: l.Description,
		Priority:    l.Priority,
	}
}
