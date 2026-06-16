package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
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

	// discussion create (inline / diff-anchored comment)
	mrDiscussionCreateCmd.Flags().String("project", "", "Project ID or path (required)")
	mrDiscussionCreateCmd.Flags().String("body", "", "Comment body")
	mrDiscussionCreateCmd.Flags().String("body-file", "", "Read comment body from file")
	mrDiscussionCreateCmd.Flags().String("new-path", "", "File path in the new version of the diff")
	mrDiscussionCreateCmd.Flags().String("old-path", "", "File path in the old version (defaults to --new-path)")
	mrDiscussionCreateCmd.Flags().Int("new-line", 0, "Line number in the new version to anchor the comment")
	mrDiscussionCreateCmd.Flags().Int("old-line", 0, "Line number in the old version to anchor the comment")
	mrDiscussionCreateCmd.Flags().String("base-sha", "", "Override diff base SHA (default: MR diff_refs)")
	mrDiscussionCreateCmd.Flags().String("start-sha", "", "Override diff start SHA (default: MR diff_refs)")
	mrDiscussionCreateCmd.Flags().String("head-sha", "", "Override diff head SHA (default: MR diff_refs)")
	mrDiscussionCmd.AddCommand(mrDiscussionCreateCmd)
	markWrite(mrDiscussionCreateCmd)

	// discussion resolve / unresolve
	mrDiscussionResolveCmd.Flags().String("project", "", "Project ID or path (required)")
	mrDiscussionResolveCmd.Flags().String("discussion-id", "", "Discussion thread ID to resolve (required)")
	mrDiscussionResolveCmd.Flags().Bool("unresolve", false, "Reopen the thread instead of resolving it")
	mrDiscussionCmd.AddCommand(mrDiscussionResolveCmd)
	markWrite(mrDiscussionResolveCmd)
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

// ─── discussion create (inline / diff-anchored comment) ───────────────────────

var mrDiscussionCreateCmd = &cobra.Command{
	Use:   "create <iid>",
	Short: "Start an inline (diff-anchored) discussion on a merge request",
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

		// Resolve the diff position. This command is for inline review, so a path
		// and a line are mandatory — a plain (non-diff) thread is `mr comment add`.
		newPath, _ := cmd.Flags().GetString("new-path")
		oldPath, _ := cmd.Flags().GetString("old-path")
		newLine, _ := cmd.Flags().GetInt("new-line")
		oldLine, _ := cmd.Flags().GetInt("old-line")
		if newPath == "" && oldPath == "" {
			return failArg("--new-path or --old-path is required (inline comments anchor to a file)")
		}
		if newLine == 0 && oldLine == 0 {
			return failArg("--new-line or --old-line is required (inline comments anchor to a line)")
		}
		// A single --new-path covers the common non-renamed case: GitLab wants both
		// paths, so mirror whichever the agent omitted.
		if oldPath == "" {
			oldPath = newPath
		}
		if newPath == "" {
			newPath = oldPath
		}

		baseSHA, _ := cmd.Flags().GetString("base-sha")
		startSHA, _ := cmd.Flags().GetString("start-sha")
		headSHA, _ := cmd.Flags().GetString("head-sha")

		client, _, err := newClient()
		if err != nil {
			return err
		}

		// Auto-fill the three diff SHAs from the MR's diff_refs unless the agent
		// pinned them explicitly (e.g. to comment on an older diff version). The
		// SHAs are a deterministic property of (project, iid), so the lookup is
		// exact — the agent only supplies the file and line it decided to flag.
		if baseSHA == "" || startSHA == "" || headSHA == "" {
			mr, err := client.MergeRequests.Get(apiCtx(), project, iid)
			if err != nil {
				return handleAPIError(err, jsonMode)
			}
			if mr.DiffRefs == nil {
				return failArg(fmt.Sprintf("MR !%d has no diff_refs; cannot anchor an inline comment", iid))
			}
			if baseSHA == "" {
				baseSHA = mr.DiffRefs.BaseSHA
			}
			if startSHA == "" {
				startSHA = mr.DiffRefs.StartSHA
			}
			if headSHA == "" {
				headSHA = mr.DiffRefs.HeadSHA
			}
		}

		// Boundary guard: GitLab leaves base_sha/head_sha null while an MR's diff is
		// still being computed (or when it has no diff at all). Sending such a
		// position yields a cryptic "doesn't support new-style diff notes" 400, so
		// fail early with an actionable message instead.
		if baseSHA == "" || headSHA == "" {
			return failArg(fmt.Sprintf("MR !%d has incomplete diff_refs (base/head SHA missing); its diff may still be computing or the MR has no diff — retry shortly or pass --base-sha/--head-sha explicitly", iid))
		}

		position := &api.DiffPosition{
			BaseSHA:      baseSHA,
			StartSHA:     startSHA,
			HeadSHA:      headSHA,
			PositionType: "text",
			NewPath:      newPath,
			OldPath:      oldPath,
		}
		if newLine > 0 {
			position.NewLine = &newLine
		}
		if oldLine > 0 {
			position.OldLine = &oldLine
		}

		// Bind the resolved head SHA into the confirm scope so a force-push between
		// dry-run and confirm invalidates the token rather than landing the comment
		// on a stale diff version.
		confirmDetail := map[string]any{
			"project": project, "iid": iid, "body": body,
			"newPath": newPath, "oldPath": oldPath,
			"newLine": newLine, "oldLine": oldLine, "headSha": headSHA,
		}
		if done, err := prepareWrite(cmd, "create mr discussion", confirmDetail); done || err != nil {
			return err
		}

		discussion, err := client.MergeRequests.CreateDiscussion(apiCtx(), project, iid, body, position)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.MRDiscussionToMap(output.ToFlatMRDiscussion(discussion)))
			return nil
		}
		output.Success(fmt.Sprintf("Created inline discussion %s on MR !%d (%s:%d)", discussion.ID, iid, newPath, newLine))
		return nil
	},
}

// ─── discussion resolve / unresolve ───────────────────────────────────────────

var mrDiscussionResolveCmd = &cobra.Command{
	Use:   "resolve <iid>",
	Short: "Resolve or reopen a merge request discussion thread",
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
		unresolve, _ := cmd.Flags().GetBool("unresolve")
		resolved := !unresolve

		confirmDetail := map[string]any{"project": project, "iid": iid, "discussionId": discussionID, "resolved": resolved}
		if done, err := prepareWrite(cmd, "resolve mr discussion", confirmDetail); done || err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}
		discussion, err := client.MergeRequests.ResolveDiscussion(apiCtx(), project, iid, discussionID, resolved)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}
		if jsonMode {
			output.PrintJSON(output.MRDiscussionToMap(output.ToFlatMRDiscussion(discussion)))
			return nil
		}
		verb := "Resolved"
		if unresolve {
			verb = "Reopened"
		}
		output.Success(fmt.Sprintf("%s discussion %s on MR !%d", verb, discussionID, iid))
		return nil
	},
}
