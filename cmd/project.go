package cmd

import (
	"fmt"
	"strconv"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage GitLab projects",
}

func init() {
	rootCmd.AddCommand(projectCmd)

	// list
	projectListCmd.Flags().Bool("owned", false, "Only show owned projects")
	projectListCmd.Flags().Bool("membership", false, "Only show projects you are a member of")
	projectListCmd.Flags().String("search", "", "Search query")
	projectListCmd.Flags().String("visibility", "", "Visibility: public|internal|private")
	projectListCmd.Flags().Int("limit", 20, "Max results (1-100)")
	projectListCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	projectCmd.AddCommand(projectListCmd)

	// get
	projectGetCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	projectCmd.AddCommand(projectGetCmd)

	// members
	projectMembersCmd.Flags().String("query", "", "Search query for members")
	projectMembersCmd.Flags().Int("limit", 20, "Max results (1-100)")
	projectCmd.AddCommand(projectMembersCmd)
}

// ─── list ─────────────────────────────────────────────────────────────────────

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		owned, _ := cmd.Flags().GetBool("owned")
		membership, _ := cmd.Flags().GetBool("membership")
		search, _ := cmd.Flags().GetString("search")
		visibility, _ := cmd.Flags().GetString("visibility")
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		projects, err := client.Projects.List(apiCtx(), &api.ProjectListOpts{
			Owned:      owned,
			Membership: membership,
			Search:     search,
			Visibility: visibility,
			Limit:      limit,
		})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			fields := getFieldsFlag(cmd)
			out := make([]map[string]any, len(projects))
			for i, p := range projects {
				out[i] = output.FilterMap(output.ProjectToMap(output.ToFlatProject(&p)), fields)
			}
			printSimpleListJSON(cmd, out, limit)
			return nil
		}
		if len(projects) == 0 {
			output.Info("No projects found.")
			return nil
		}
		headers := []string{"ID", "PATH", "VISIBILITY", "DEFAULT BRANCH"}
		rows := make([][]string, len(projects))
		for i, p := range projects {
			rows[i] = []string{
				strconv.Itoa(p.ID),
				p.PathWithNamespace,
				p.Visibility,
				p.DefaultBranch,
			}
		}
		output.Table(headers, rows)
		return nil
	},
}

// ─── get ──────────────────────────────────────────────────────────────────────

var projectGetCmd = &cobra.Command{
	Use:   "get <id-or-path>",
	Short: "Get a project by ID or path",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		p, err := client.Projects.Get(apiCtx(), args[0])
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			output.PrintJSON(output.FilterMap(output.ProjectToMap(output.ToFlatProject(p)), fields))
			return nil
		}
		fmt.Printf("ID:             %d\n", p.ID)
		fmt.Printf("Name:           %s\n", p.Name)
		fmt.Printf("Path:           %s\n", p.PathWithNamespace)
		fmt.Printf("Visibility:     %s\n", p.Visibility)
		fmt.Printf("Default Branch: %s\n", p.DefaultBranch)
		if p.WebURL != "" {
			fmt.Printf("URL:            %s\n", p.WebURL)
		}
		return nil
	},
}

// ─── members ──────────────────────────────────────────────────────────────────

var projectMembersCmd = &cobra.Command{
	Use:   "members <id-or-path>",
	Short: "List members of a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		query, _ := cmd.Flags().GetString("query")
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		members, err := client.Projects.Members(apiCtx(), args[0], query, limit)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			out := make([]map[string]any, len(members))
			for i, m := range members {
				out[i] = output.ProjectMemberToMap(output.ToFlatProjectMember(&m))
			}
			printSimpleListJSON(cmd, out, limit)
			return nil
		}
		if len(members) == 0 {
			output.Info("No members found.")
			return nil
		}
		headers := []string{"ID", "USERNAME", "NAME", "STATE", "ACCESS LEVEL"}
		rows := make([][]string, len(members))
		for i, m := range members {
			rows[i] = []string{
				strconv.Itoa(m.ID),
				m.Username,
				m.Name,
				m.State,
				strconv.Itoa(m.AccessLevel),
			}
		}
		output.Table(headers, rows)
		return nil
	},
}
