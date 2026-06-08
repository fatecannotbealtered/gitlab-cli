package cmd

import (
	"fmt"

	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search GitLab resources",
}

func init() {
	rootCmd.AddCommand(searchCmd)

	// projects
	searchProjectsCmd.Flags().String("query", "", "Search query (required)")
	searchProjectsCmd.Flags().Int("limit", 20, "Max results (1-100)")
	searchProjectsCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	searchCmd.AddCommand(searchProjectsCmd)

	// issues
	searchIssuesCmd.Flags().String("query", "", "Search query (required)")
	searchIssuesCmd.Flags().String("project", "", "Project ID or path (scopes search to project)")
	searchIssuesCmd.Flags().Int("limit", 20, "Max results (1-100)")
	searchIssuesCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	searchCmd.AddCommand(searchIssuesCmd)

	// mrs
	searchMRsCmd.Flags().String("query", "", "Search query (required)")
	searchMRsCmd.Flags().String("project", "", "Project ID or path (scopes search to project)")
	searchMRsCmd.Flags().Int("limit", 20, "Max results (1-100)")
	searchMRsCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	searchCmd.AddCommand(searchMRsCmd)

	// code
	searchCodeCmd.Flags().String("query", "", "Search query (required)")
	searchCodeCmd.Flags().String("project", "", "Project ID or path (required for code search)")
	searchCodeCmd.Flags().Int("limit", 20, "Max results (1-100)")
	searchCodeCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	searchCmd.AddCommand(searchCodeCmd)

	// commits
	searchCommitsCmd.Flags().String("query", "", "Search query (required)")
	searchCommitsCmd.Flags().String("project", "", "Project ID or path (scopes search to project)")
	searchCommitsCmd.Flags().Int("limit", 20, "Max results (1-100)")
	searchCommitsCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	searchCmd.AddCommand(searchCommitsCmd)
}

// ─── projects ─────────────────────────────────────────────────────────────────

var searchProjectsCmd = &cobra.Command{
	Use:   "projects",
	Short: "Search for projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		query, _ := cmd.Flags().GetString("query")
		if query == "" {
			return failArg("--query is required")
		}
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}
		results, err := client.Search.Projects(apiCtx(), query, limit)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			fields := getFieldsFlag(cmd)
			out := make([]map[string]any, len(results))
			for i, r := range results {
				out[i] = output.FilterMap(output.SearchProjectToMap(output.ToFlatSearchProject(&r)), fields)
			}
			printSimpleListJSON(cmd, out, limit)
			return nil
		}
		if len(results) == 0 {
			output.Info("No projects found.")
			return nil
		}
		headers := []string{"ID", "PATH", "VISIBILITY"}
		rows := make([][]string, len(results))
		for i, r := range results {
			rows[i] = []string{fmt.Sprintf("%d", r.ID), r.PathWithNamespace, r.Visibility}
		}
		output.Table(headers, rows)
		return nil
	},
}

// ─── issues ───────────────────────────────────────────────────────────────────

var searchIssuesCmd = &cobra.Command{
	Use:   "issues",
	Short: "Search for issues",
	RunE: func(cmd *cobra.Command, args []string) error {
		query, _ := cmd.Flags().GetString("query")
		if query == "" {
			return failArg("--query is required")
		}
		project, _ := cmd.Flags().GetString("project")
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}
		results, err := client.Search.Issues(apiCtx(), query, project, limit)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			fields := getFieldsFlag(cmd)
			out := make([]map[string]any, len(results))
			for i, r := range results {
				out[i] = output.FilterMap(output.SearchIssueToMap(output.ToFlatSearchIssue(&r)), fields)
			}
			printSimpleListJSON(cmd, out, limit)
			return nil
		}
		if len(results) == 0 {
			output.Info("No issues found.")
			return nil
		}
		headers := []string{"ID", "IID", "TITLE", "STATE"}
		rows := make([][]string, len(results))
		for i, r := range results {
			rows[i] = []string{
				fmt.Sprintf("%d", r.ID),
				fmt.Sprintf("%d", r.IID),
				r.Title,
				output.StatusBadge(r.State),
			}
		}
		output.Table(headers, rows)
		return nil
	},
}

// ─── mrs ──────────────────────────────────────────────────────────────────────

var searchMRsCmd = &cobra.Command{
	Use:   "mrs",
	Short: "Search for merge requests",
	RunE: func(cmd *cobra.Command, args []string) error {
		query, _ := cmd.Flags().GetString("query")
		if query == "" {
			return failArg("--query is required")
		}
		project, _ := cmd.Flags().GetString("project")
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}
		results, err := client.Search.MergeRequests(apiCtx(), query, project, limit)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			fields := getFieldsFlag(cmd)
			out := make([]map[string]any, len(results))
			for i, r := range results {
				out[i] = output.FilterMap(output.SearchMRToMap(output.ToFlatSearchMR(&r)), fields)
			}
			printSimpleListJSON(cmd, out, limit)
			return nil
		}
		if len(results) == 0 {
			output.Info("No merge requests found.")
			return nil
		}
		headers := []string{"ID", "IID", "TITLE", "STATE"}
		rows := make([][]string, len(results))
		for i, r := range results {
			rows[i] = []string{
				fmt.Sprintf("%d", r.ID),
				fmt.Sprintf("%d", r.IID),
				r.Title,
				output.StatusBadge(r.State),
			}
		}
		output.Table(headers, rows)
		return nil
	},
}

// ─── code ─────────────────────────────────────────────────────────────────────

var searchCodeCmd = &cobra.Command{
	Use:   "code",
	Short: "Search for code (blobs) within a project",
	RunE: func(cmd *cobra.Command, args []string) error {
		query, _ := cmd.Flags().GetString("query")
		if query == "" {
			return failArg("--query is required")
		}
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required for code search")
		}
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}
		results, err := client.Search.Code(apiCtx(), query, project, limit)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			fields := getFieldsFlag(cmd)
			out := make([]map[string]any, len(results))
			for i, r := range results {
				out[i] = output.FilterMap(output.SearchBlobToMap(output.ToFlatSearchBlob(&r)), fields)
			}
			printSimpleListJSON(cmd, out, limit)
			return nil
		}
		if len(results) == 0 {
			output.Info("No code results found.")
			return nil
		}
		headers := []string{"FILE", "PATH", "REF", "LINE"}
		rows := make([][]string, len(results))
		for i, r := range results {
			rows[i] = []string{r.Filename, r.Path, r.Ref, fmt.Sprintf("%d", r.StartLine)}
		}
		output.Table(headers, rows)
		return nil
	},
}

// ─── commits ──────────────────────────────────────────────────────────────────

var searchCommitsCmd = &cobra.Command{
	Use:   "commits",
	Short: "Search for commits",
	RunE: func(cmd *cobra.Command, args []string) error {
		query, _ := cmd.Flags().GetString("query")
		if query == "" {
			return failArg("--query is required")
		}
		project, _ := cmd.Flags().GetString("project")
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}
		results, err := client.Search.Commits(apiCtx(), query, project, limit)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			fields := getFieldsFlag(cmd)
			out := make([]map[string]any, len(results))
			for i, r := range results {
				out[i] = output.FilterMap(output.SearchCommitToMap(output.ToFlatSearchCommit(&r)), fields)
			}
			printSimpleListJSON(cmd, out, limit)
			return nil
		}
		if len(results) == 0 {
			output.Info("No commits found.")
			return nil
		}
		headers := []string{"SHORT ID", "TITLE", "AUTHOR", "DATE"}
		rows := make([][]string, len(results))
		for i, r := range results {
			rows[i] = []string{r.ShortID, r.Title, r.AuthorName, r.CreatedAt}
		}
		output.Table(headers, rows)
		return nil
	},
}
