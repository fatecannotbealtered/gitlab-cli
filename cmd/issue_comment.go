package cmd

import (
	"fmt"
	"strconv"

	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var issueCommentCmd = &cobra.Command{
	Use:   "comment",
	Short: "Manage issue comments",
}

func init() {
	issueCmd.AddCommand(issueCommentCmd)

	// comment add
	issueCommentAddCmd.Flags().String("project", "", "Project ID or path (required)")
	issueCommentAddCmd.Flags().String("body", "", "Comment body")
	issueCommentAddCmd.Flags().String("body-file", "", "Read comment body from file")
	issueCommentCmd.AddCommand(issueCommentAddCmd)
	markWrite(issueCommentAddCmd)

	// comment list
	issueCommentListCmd.Flags().String("project", "", "Project ID or path (required)")
	issueCommentListCmd.Flags().Int("limit", 20, "Max results (1-100)")
	issueCommentListCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	issueCommentCmd.AddCommand(issueCommentListCmd)

	// comment delete
	issueCommentDeleteCmd.Flags().String("project", "", "Project ID or path (required)")
	issueCommentDeleteCmd.Flags().Int("note-id", 0, "Note ID (required)")
	issueCommentCmd.AddCommand(issueCommentDeleteCmd)
	markWrite(issueCommentDeleteCmd)
	markConfirm(issueCommentDeleteCmd)
}

var issueCommentAddCmd = &cobra.Command{
	Use:   "add <iid>",
	Short: "Add a comment to an issue",
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
		body, _ := cmd.Flags().GetString("body")
		bodyFile, _ := cmd.Flags().GetString("body-file")
		if bodyFile != "" {
			content, err := readBodyFile(bodyFile)
			if err != nil {
				return failArg(err.Error())
			}
			body = content
		}
		if body == "" {
			return failArg("--body or --body-file is required")
		}
		if done, err := prepareWrite(cmd, "add comment", map[string]any{"project": project, "iid": iid, "body": body}); done || err != nil {
			return err
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		note, err := client.Issues.AddNote(apiCtx(), project, iid, body)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.MarkUntrusted(map[string]any{
				"id":   output.ID(note.ID),
				"body": note.Body,
			}, "body"))
			return nil
		}
		output.Success(fmt.Sprintf("Added comment #%d to issue #%d", note.ID, iid))
		return nil
	},
}

var issueCommentListCmd = &cobra.Command{
	Use:   "list <iid>",
	Short: "List comments on an issue",
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
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}
		notes, err := client.Issues.ListNotes(apiCtx(), project, iid, limit)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			out := make([]map[string]any, 0, len(notes))
			for _, n := range notes {
				if n.System {
					continue
				}
				m := output.MarkUntrusted(map[string]any{"id": output.ID(n.ID), "body": n.Body}, "body")
				if n.Author != nil {
					m["author"] = n.Author.Username
				}
				out = append(out, output.FilterMap(m, fields))
			}
			printSimpleListJSON(cmd, out, limit)
			return nil
		}
		if len(notes) == 0 {
			output.Info("No comments found.")
			return nil
		}
		headers := []string{"ID", "AUTHOR", "BODY"}
		rows := [][]string{}
		for _, n := range notes {
			if n.System {
				continue
			}
			body := n.Body
			if len(body) > 80 {
				body = body[:77] + "..."
			}
			rows = append(rows, []string{
				fmt.Sprintf("%d", n.ID),
				usernameOf(n.Author),
				body,
			})
		}
		output.Table(headers, rows)
		return nil
	},
}

var issueCommentDeleteCmd = &cobra.Command{
	Use:   "delete <iid>",
	Short: "Delete a comment from an issue",
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
		noteID, _ := cmd.Flags().GetInt("note-id")
		if noteID == 0 {
			return failArg("--note-id is required")
		}
		confirmPayload := map[string]any{"project": project, "iid": iid, "noteId": noteID}
		if done, err := prepareWrite(cmd, "delete comment", confirmPayload); done || err != nil {
			return err
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		if err := client.Issues.DeleteNote(apiCtx(), project, iid, noteID); err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(map[string]any{"deleted": true, "noteId": output.ID(noteID)})
			return nil
		}
		output.Success(fmt.Sprintf("Deleted comment #%d", noteID))
		return nil
	},
}
