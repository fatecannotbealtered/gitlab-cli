package cmd

import (
	"fmt"
	"strings"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Manage project releases",
}

// ─── release list ─────────────────────────────────────────────────────────────

var releaseListCmd = &cobra.Command{
	Use:   "list",
	Short: "List releases",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, _ := cmd.Flags().GetString("project")
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		if project == "" {
			return failArg("--project is required")
		}

		releases, err := client.Releases.List(apiCtx(), project, limit)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			fields := getFieldsFlag(cmd)
			flat := make([]map[string]any, len(releases))
			for i, r := range releases {
				r := r
				flat[i] = output.FilterMap(output.ReleaseToMap(output.ToFlatRelease(&r)), fields)
			}
			output.PrintJSON(flat)
			return nil
		}
		if len(releases) == 0 {
			output.Info("No releases found.")
			return nil
		}
		headers := []string{"TAG", "NAME", "RELEASED AT", "AUTHOR"}
		rows := make([][]string, len(releases))
		for i, r := range releases {
			author := ""
			if r.Author != nil {
				author = r.Author.Username
			}
			rows[i] = []string{r.TagName, r.Name, r.ReleasedAt, author}
		}
		output.Table(headers, rows)
		return nil
	},
}

// ─── release get ──────────────────────────────────────────────────────────────

var releaseGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a release by tag name",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, _ := cmd.Flags().GetString("project")
		tag, _ := cmd.Flags().GetString("tag")

		if project == "" || tag == "" {
			return failArg("--project and --tag are required")
		}

		r, err := client.Releases.Get(apiCtx(), project, tag)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			output.PrintJSON(output.ToFlatRelease(r))
			return nil
		}
		author := ""
		if r.Author != nil {
			author = r.Author.Username
		}
		output.Table([]string{"FIELD", "VALUE"}, [][]string{
			{"Tag", r.TagName},
			{"Name", r.Name},
			{"Released At", r.ReleasedAt},
			{"Author", author},
			{"Description", truncate(r.Description, 80)},
		})
		return nil
	},
}

// ─── release create ───────────────────────────────────────────────────────────

var releaseCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new release",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		tag, _ := cmd.Flags().GetString("tag")
		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")
		ref, _ := cmd.Flags().GetString("ref")
		milestoneStr, _ := cmd.Flags().GetString("milestone")

		if project == "" || tag == "" || name == "" {
			return failArg("--project, --tag, and --name are required")
		}

		milestones := parseMilestones(milestoneStr)

		if dryRunOutput("release create", map[string]any{
			"project": project, "tag": tag, "name": name,
		}) {
			return nil
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		r, err := client.Releases.Create(apiCtx(), project, api.ReleaseCreateBody{
			TagName:     tag,
			Name:        name,
			Description: description,
			Ref:         ref,
			Milestones:  milestones,
		})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			output.PrintJSON(output.ToFlatRelease(r))
			return nil
		}
		output.Success(fmt.Sprintf("Created release %s (%s)", r.Name, r.TagName))
		return nil
	},
}

// ─── release update ───────────────────────────────────────────────────────────

var releaseUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update an existing release",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		tag, _ := cmd.Flags().GetString("tag")
		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")
		milestoneStr, _ := cmd.Flags().GetString("milestone")

		if project == "" || tag == "" {
			return failArg("--project and --tag are required")
		}

		milestones := parseMilestones(milestoneStr)

		if dryRunOutput("release update", map[string]any{
			"project": project, "tag": tag,
		}) {
			return nil
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		r, err := client.Releases.Update(apiCtx(), project, tag, api.ReleaseUpdateBody{
			Name:        name,
			Description: description,
			Milestones:  milestones,
		})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			output.PrintJSON(output.ToFlatRelease(r))
			return nil
		}
		output.Success(fmt.Sprintf("Updated release %s (%s)", r.Name, r.TagName))
		return nil
	},
}

// ─── release delete ───────────────────────────────────────────────────────────

var releaseDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a release",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		tag, _ := cmd.Flags().GetString("tag")

		if project == "" || tag == "" {
			return failArg("--project and --tag are required")
		}

		if dryRunOutput("release delete", map[string]any{"project": project, "tag": tag}) {
			return nil
		}

		if err := requireConfirm(cmd, fmt.Sprintf("Type the tag name to confirm deletion of release %q", tag), tag); err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		if err := client.Releases.Delete(apiCtx(), project, tag); err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			output.PrintJSON(map[string]any{"tag": tag, "action": "deleted"})
			return nil
		}
		output.Success(fmt.Sprintf("Deleted release %s", tag))
		return nil
	},
}

// ─── init ─────────────────────────────────────────────────────────────────────

func init() {
	rootCmd.AddCommand(releaseCmd)

	releaseCmd.AddCommand(releaseListCmd)
	releaseListCmd.Flags().String("project", "", "Project ID or path (required)")
	releaseListCmd.Flags().Int("limit", 20, "Max results (1-100)")
	releaseListCmd.Flags().String("fields", "", "Comma-separated fields for JSON output")

	releaseCmd.AddCommand(releaseGetCmd)
	releaseGetCmd.Flags().String("project", "", "Project ID or path (required)")
	releaseGetCmd.Flags().String("tag", "", "Tag name (required)")

	releaseCmd.AddCommand(releaseCreateCmd)
	releaseCreateCmd.Flags().String("project", "", "Project ID or path (required)")
	releaseCreateCmd.Flags().String("tag", "", "Tag name (required)")
	releaseCreateCmd.Flags().String("name", "", "Release name (required)")
	releaseCreateCmd.Flags().String("description", "", "Release description")
	releaseCreateCmd.Flags().String("ref", "", "Branch or SHA to tag")
	releaseCreateCmd.Flags().String("milestone", "", "Comma-separated milestone titles")
	markWrite(releaseCreateCmd)

	releaseCmd.AddCommand(releaseUpdateCmd)
	releaseUpdateCmd.Flags().String("project", "", "Project ID or path (required)")
	releaseUpdateCmd.Flags().String("tag", "", "Tag name (required)")
	releaseUpdateCmd.Flags().String("name", "", "New release name")
	releaseUpdateCmd.Flags().String("description", "", "New release description")
	releaseUpdateCmd.Flags().String("milestone", "", "Comma-separated milestone titles")
	markWrite(releaseUpdateCmd)

	releaseCmd.AddCommand(releaseDeleteCmd)
	releaseDeleteCmd.Flags().String("project", "", "Project ID or path (required)")
	releaseDeleteCmd.Flags().String("tag", "", "Tag name (required)")
	markWrite(releaseDeleteCmd)
	markConfirm(releaseDeleteCmd)
}

// parseMilestones splits a comma-separated milestone string into a slice.
// Returns nil (not empty slice) when the input is empty so omitempty works.
func parseMilestones(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
