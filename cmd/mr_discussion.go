package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var mrDiscussionCmd = &cobra.Command{
	Use:   "discussion",
	Short: "Manage merge request discussion threads",
}

func init() {
	mrCmd.AddCommand(mrDiscussionCmd)

	// discussion list
	mrDiscussionListCmd.Flags().String("project", "", "Project ID or path (required)")
	mrDiscussionListCmd.Flags().Int("limit", 20, "Max results (1-100)")
	mrDiscussionListCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	mrDiscussionCmd.AddCommand(mrDiscussionListCmd)

	// discussion reply
	mrDiscussionReplyCmd.Flags().String("project", "", "Project ID or path (required)")
	mrDiscussionReplyCmd.Flags().String("discussion-id", "", "Discussion thread ID to reply to (required)")
	mrDiscussionReplyCmd.Flags().String("body", "", "Reply body")
	mrDiscussionReplyCmd.Flags().String("body-file", "", "Read reply body from file")
	mrDiscussionCmd.AddCommand(mrDiscussionReplyCmd)
	markWrite(mrDiscussionReplyCmd)
}

// ─── discussion list ──────────────────────────────────────────────────────────

var mrDiscussionListCmd = &cobra.Command{
	Use:   "list <iid>",
	Short: "List discussion threads on a merge request",
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
		discussions, err := client.MergeRequests.ListDiscussions(apiCtx(), project, iid, limit)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			fields := getFieldsFlag(cmd)
			out := make([]map[string]any, 0, len(discussions))
			for i := range discussions {
				out = append(out, output.FilterMap(output.MRDiscussionToMap(output.ToFlatMRDiscussion(&discussions[i])), fields))
			}
			printSimpleListJSON(cmd, out, limit)
			return nil
		}
		if len(discussions) == 0 {
			output.Info("No discussions found.")
			return nil
		}
		headers := []string{"DISCUSSION ID", "NOTES", "AUTHOR", "BODY"}
		rows := [][]string{}
		for i := range discussions {
			d := &discussions[i]
			author, body := "", ""
			if len(d.Notes) > 0 {
				first := d.Notes[0]
				if first.Author != nil {
					author = first.Author.Username
				}
				body = first.Body
				if len(body) > 50 {
					body = body[:47] + "..."
				}
			}
			rows = append(rows, []string{d.ID, strconv.Itoa(len(d.Notes)), author, body})
		}
		output.Table(headers, rows)
		return nil
	},
}

// ─── discussion reply ─────────────────────────────────────────────────────────

var mrDiscussionReplyCmd = &cobra.Command{
	Use:   "reply <iid>",
	Short: "Reply to a merge request discussion thread",
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
		discussionID, _ := cmd.Flags().GetString("discussion-id")
		if discussionID == "" {
			return failArg("--discussion-id is required")
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

		confirmDetail := map[string]any{"project": project, "iid": iid, "discussionId": discussionID, "body": body}
		if done, err := prepareWrite(cmd, "reply mr discussion", confirmDetail); done || err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}
		note, err := client.MergeRequests.ReplyDiscussion(apiCtx(), project, iid, discussionID, body)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.MRNoteToMap(output.ToFlatMRNote(note)))
			return nil
		}
		output.Success(fmt.Sprintf("Replied to discussion %s on MR !%d (note #%d)", discussionID, iid, note.ID))
		return nil
	},
}
