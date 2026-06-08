package cmd

import (
	"fmt"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Query GitLab users",
}

func init() {
	rootCmd.AddCommand(userCmd)

	// me
	userMeCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	userCmd.AddCommand(userMeCmd)

	// search
	userSearchCmd.Flags().String("query", "", "Search query (required)")
	userSearchCmd.Flags().Bool("active", false, "Only show active users")
	userSearchCmd.Flags().Int("limit", 20, "Max results (1-100)")
	userSearchCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	userCmd.AddCommand(userSearchCmd)

	// get
	userGetCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	userCmd.AddCommand(userGetCmd)
}

// ─── me ───────────────────────────────────────────────────────────────────────

var userMeCmd = &cobra.Command{
	Use:   "me",
	Short: "Show the currently authenticated user",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		u, err := client.Users.Me(apiCtx())
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			output.PrintJSON(output.FilterMap(output.UserToMap(toFlatUser(u)), fields))
			return nil
		}
		fmt.Printf("ID:       %d\n", u.ID)
		fmt.Printf("Username: %s\n", u.Username)
		fmt.Printf("Name:     %s\n", u.Name)
		if u.Email != "" {
			fmt.Printf("Email:    %s\n", u.Email)
		}
		fmt.Printf("State:    %s\n", u.State)
		return nil
	},
}

// ─── search ───────────────────────────────────────────────────────────────────

var userSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search for users",
	RunE: func(cmd *cobra.Command, args []string) error {
		query, _ := cmd.Flags().GetString("query")
		if query == "" {
			return failArg("--query is required")
		}
		active, _ := cmd.Flags().GetBool("active")
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		users, err := client.Users.Search(apiCtx(), query, limit)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		// Apply active filter and limit client-side (UserAPI.Search uses per_page=20 default)
		filtered := filterUsers(users, active, limit)

		if jsonMode {
			fields := getFieldsFlag(cmd)
			out := make([]map[string]any, len(filtered))
			for i, u := range filtered {
				out[i] = output.FilterMap(output.UserToMap(toFlatUser(&u)), fields)
			}
			printSimpleListJSON(cmd, out, limit)
			return nil
		}
		if len(filtered) == 0 {
			output.Info("No users found.")
			return nil
		}
		headers := []string{"ID", "USERNAME", "NAME", "STATE"}
		rows := make([][]string, len(filtered))
		for i, u := range filtered {
			rows[i] = []string{
				fmt.Sprintf("%d", u.ID),
				u.Username,
				u.Name,
				u.State,
			}
		}
		output.Table(headers, rows)
		return nil
	},
}

// ─── get ──────────────────────────────────────────────────────────────────────

var userGetCmd = &cobra.Command{
	Use:   "get <username>",
	Short: "Get a user by username",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		u, err := client.Users.GetByUsername(apiCtx(), args[0])
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if u == nil {
			return failNotFound("user not found: " + args[0])
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			output.PrintJSON(output.FilterMap(output.UserToMap(toFlatUser(u)), fields))
			return nil
		}
		fmt.Printf("ID:       %d\n", u.ID)
		fmt.Printf("Username: %s\n", u.Username)
		fmt.Printf("Name:     %s\n", u.Name)
		if u.Email != "" {
			fmt.Printf("Email:    %s\n", u.Email)
		}
		fmt.Printf("State:    %s\n", u.State)
		return nil
	},
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func toFlatUser(u *api.User) output.FlatUser {
	return output.FlatUser{
		ID:       u.ID,
		Username: u.Username,
		Name:     u.Name,
		Email:    u.Email,
		State:    u.State,
		WebURL:   u.WebURL,
	}
}

func filterUsers(users []api.User, activeOnly bool, limit int) []api.User {
	out := make([]api.User, 0, len(users))
	for _, u := range users {
		if activeOnly && u.State != "active" {
			continue
		}
		out = append(out, u)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}
