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

	// create
	projectCreateCmd.Flags().String("name", "", "Project name (required)")
	projectCreateCmd.Flags().String("path", "", "Project path (slug); defaults from name")
	projectCreateCmd.Flags().String("description", "", "Project description")
	projectCreateCmd.Flags().String("visibility", "", "Visibility: private|internal|public")
	projectCreateCmd.Flags().Int("namespace-id", 0, "Namespace (group/user) ID to create the project under")
	projectCreateCmd.Flags().String("idempotency-key", "", "Idempotency-Key sent to GitLab so a retried create cannot duplicate the project")
	projectCmd.AddCommand(projectCreateCmd)
	markWrite(projectCreateCmd)

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

// ─── create ───────────────────────────────────────────────────────────────────

var projectCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new project",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		if name == "" {
			return failArg("--name is required")
		}
		path, _ := cmd.Flags().GetString("path")
		description, _ := cmd.Flags().GetString("description")
		visibility, _ := cmd.Flags().GetString("visibility")
		namespaceID, _ := cmd.Flags().GetInt("namespace-id")

		// Validate visibility at the boundary so a typo fails fast with E_VALIDATION
		// instead of surfacing as a GitLab 400 after the confirm round-trip.
		if visibility != "" && visibility != "private" && visibility != "internal" && visibility != "public" {
			return failArg("--visibility must be one of private|internal|public")
		}

		confirmDetail := map[string]any{"name": name}
		if path != "" {
			confirmDetail["path"] = path
		}
		if visibility != "" {
			confirmDetail["visibility"] = visibility
		}
		if namespaceID != 0 {
			confirmDetail["namespaceId"] = namespaceID
		}
		if key := idempotencyKeyOf(cmd); key != "" {
			confirmDetail["idempotencyKey"] = key
		}
		if done, err := prepareWrite(cmd, "create project", confirmDetail); done || err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		p, err := client.Projects.Create(idempotentCtx(cmd), &api.ProjectCreateRequest{
			Name:        name,
			Path:        path,
			Description: description,
			Visibility:  visibility,
			NamespaceID: namespaceID,
		})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.ProjectToMap(output.ToFlatProject(p)))
			return nil
		}
		output.Success(fmt.Sprintf("Created project %s (#%d)", p.PathWithNamespace, p.ID))
		if p.WebURL != "" {
			output.Info(p.WebURL)
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
