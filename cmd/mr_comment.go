package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var mrCommentCmd = &cobra.Command{
	Use:   "comment",
	Short: "Manage merge request comments",
}

func init() {
	mrCmd.AddCommand(mrCommentCmd)

	// comment add
	mrCommentAddCmd.Flags().String("project", "", "Project ID or path (required)")
	mrCommentAddCmd.Flags().String("body", "", "Comment body")
	mrCommentAddCmd.Flags().String("body-file", "", "Read comment body from file")
	mrCommentCmd.AddCommand(mrCommentAddCmd)
	markWrite(mrCommentAddCmd)

	// comment list
	mrCommentListCmd.Flags().String("project", "", "Project ID or path (required)")
	mrCommentListCmd.Flags().Int("limit", 20, "Max results (1-100)")
	mrCommentListCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	mrCommentCmd.AddCommand(mrCommentListCmd)

	// comment delete
	mrCommentDeleteCmd.Flags().String("project", "", "Project ID or path (required)")
	mrCommentDeleteCmd.Flags().Int("note-id", 0, "Note ID to delete (required)")
	mrCommentCmd.AddCommand(mrCommentDeleteCmd)
	markWrite(mrCommentDeleteCmd)
	markConfirm(mrCommentDeleteCmd)
}

var mrCommentAddCmd = &cobra.Command{
	Use:   "add <iid>",
	Short: "Add a comment to a merge request",
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
		body, _ := cmd.Flags().GetString("body")
		bodyFile, _ := cmd.Flags().GetString("body-file")
		if bodyFile != "" {
			data, err := os.ReadFile(bodyFile)
			if err != nil {
				return failArg("reading body-file: " + err.Error())
			}
			body = string(data)
		}
		body = strings.TrimSpace(body)
		if body == "" {
			return failArg("--body or --body-file is required")
		}

		if done, err := prepareWrite(cmd, "add mr comment", map[string]any{"project": project, "iid": iid, "body": body}); done || err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}
		note, err := client.MergeRequests.AddNote(apiCtx(), project, iid, body)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.MRNoteToMap(output.ToFlatMRNote(note)))
			return nil
		}
		output.Success(fmt.Sprintf("Added comment #%d to MR !%d", note.ID, iid))
		return nil
	},
}

var mrCommentListCmd = &cobra.Command{
	Use:   "list <iid>",
	Short: "List comments on a merge request",
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
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}
		notes, err := client.MergeRequests.ListNotes(apiCtx(), project, iid, limit)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			out := make([]map[string]any, 0, len(notes))
			for _, n := range notes {
				if !n.System {
					out = append(out, output.FilterMap(output.MRNoteToMap(output.ToFlatMRNote(&n)), fields))
				}
			}
			printSimpleListJSON(cmd, out, limit)
			return nil
		}
		if len(notes) == 0 {
			output.Info("No comments found.")
			return nil
		}
		headers := []string{"ID", "AUTHOR", "BODY", "CREATED"}
		rows := [][]string{}
		for _, n := range notes {
			if n.System {
				continue
			}
			author := ""
			if n.Author != nil {
				author = n.Author.Username
			}
			body := n.Body
			if len(body) > 60 {
				body = body[:57] + "..."
			}
			rows = append(rows, []string{strconv.Itoa(n.ID), author, body, n.CreatedAt})
		}
		output.Table(headers, rows)
		return nil
	},
}

var mrCommentDeleteCmd = &cobra.Command{
	Use:   "delete <iid>",
	Short: "Delete a comment from a merge request",
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
		noteID, _ := cmd.Flags().GetInt("note-id")
		if noteID == 0 {
			return failArg("--note-id is required")
		}

		confirmPayload := map[string]any{"project": project, "iid": iid, "noteId": noteID}
		if done, err := prepareWrite(cmd, "delete mr comment", confirmPayload); done || err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}
		if err := client.MergeRequests.DeleteNote(apiCtx(), project, iid, noteID); err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(map[string]any{"deleted": true, "noteId": output.ID(noteID)})
			return nil
		}
		output.Success(fmt.Sprintf("Deleted comment #%d from MR !%d", noteID, iid))
		return nil
	},
}
