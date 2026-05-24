package cmd

import (
	"encoding/base64"
	"fmt"
	"os"
	"unicode/utf8"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

// 鈹€鈹€鈹€ Parent commands 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage repository files, branches, commits, and tree",
}

var repoFileCmd = &cobra.Command{
	Use:   "file",
	Short: "Manage repository files",
}

var repoBranchCmd = &cobra.Command{
	Use:   "branch",
	Short: "Manage repository branches",
}

var repoCommitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Browse repository commits",
}

// 鈹€鈹€鈹€ repo file get 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var repoFileGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a file from the repository (raw bytes)",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, _ := cmd.Flags().GetString("project")
		path, _ := cmd.Flags().GetString("path")
		ref, _ := cmd.Flags().GetString("ref")
		outPath, _ := cmd.Flags().GetString("output")

		if project == "" {
			return failArg("--project is required")
		}
		if path == "" {
			return failArg("--path is required")
		}

		if outPath != "" && outPath != "-" {
			if err := validateOutputPath(outPath); err != nil {
				output.Error(err.Error())
				setExitCode(ExitBadArgs)
				return ErrSilent
			}
		}

		data, err := client.Repos.GetFileRaw(apiCtx(), project, path, ref)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if outPath == "" || outPath == "-" {
			_, _ = os.Stdout.Write(data)
			return nil
		}
		if err := os.WriteFile(outPath, data, 0o644); err != nil {
			output.Error("writing file: " + err.Error())
			setExitCode(ExitNetwork)
			return ErrSilent
		}
		if !jsonMode {
			output.Success(fmt.Sprintf("Saved to %s (%d bytes)", outPath, len(data)))
		}
		return nil
	},
}

// 鈹€鈹€鈹€ repo file create 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var repoFileCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new file in the repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		path, _ := cmd.Flags().GetString("path")
		branch, _ := cmd.Flags().GetString("branch")
		content, _ := cmd.Flags().GetString("content")
		contentFile, _ := cmd.Flags().GetString("content-file")
		commitMsg, _ := cmd.Flags().GetString("commit-message")
		encoding, _ := cmd.Flags().GetString("encoding")

		if project == "" || path == "" || branch == "" || commitMsg == "" {
			return failArg("--project, --path, --branch, and --commit-message are required")
		}
		if content == "" && contentFile == "" {
			return failArg("--content or --content-file is required")
		}

		finalContent, finalEncoding, err := resolveContent(content, contentFile, encoding)
		if err != nil {
			output.Error(err.Error())
			setExitCode(ExitBadArgs)
			return ErrSilent
		}

		if dryRunOutput("repo file create", map[string]any{
			"project": project, "path": path, "branch": branch,
		}) {
			return nil
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		if err := client.Repos.CreateFile(apiCtx(), project, path, api.FileWriteBody{
			Branch:        branch,
			Content:       finalContent,
			CommitMessage: commitMsg,
			Encoding:      finalEncoding,
		}); err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			output.PrintJSON(map[string]any{"path": path, "branch": branch, "action": "created"})
			return nil
		}
		output.Success(fmt.Sprintf("Created %s on %s", path, branch))
		return nil
	},
}

// 鈹€鈹€鈹€ repo file update 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var repoFileUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update an existing file in the repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		path, _ := cmd.Flags().GetString("path")
		branch, _ := cmd.Flags().GetString("branch")
		content, _ := cmd.Flags().GetString("content")
		contentFile, _ := cmd.Flags().GetString("content-file")
		commitMsg, _ := cmd.Flags().GetString("commit-message")
		encoding, _ := cmd.Flags().GetString("encoding")

		if project == "" || path == "" || branch == "" || commitMsg == "" {
			return failArg("--project, --path, --branch, and --commit-message are required")
		}
		if content == "" && contentFile == "" {
			return failArg("--content or --content-file is required")
		}

		finalContent, finalEncoding, err := resolveContent(content, contentFile, encoding)
		if err != nil {
			output.Error(err.Error())
			setExitCode(ExitBadArgs)
			return ErrSilent
		}

		if dryRunOutput("repo file update", map[string]any{
			"project": project, "path": path, "branch": branch,
		}) {
			return nil
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		if err := client.Repos.UpdateFile(apiCtx(), project, path, api.FileWriteBody{
			Branch:        branch,
			Content:       finalContent,
			CommitMessage: commitMsg,
			Encoding:      finalEncoding,
		}); err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			output.PrintJSON(map[string]any{"path": path, "branch": branch, "action": "updated"})
			return nil
		}
		output.Success(fmt.Sprintf("Updated %s on %s", path, branch))
		return nil
	},
}

// 鈹€鈹€鈹€ repo file delete 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var repoFileDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a file from the repository",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		path, _ := cmd.Flags().GetString("path")
		branch, _ := cmd.Flags().GetString("branch")
		commitMsg, _ := cmd.Flags().GetString("commit-message")

		if project == "" || path == "" || branch == "" || commitMsg == "" {
			return failArg("--project, --path, --branch, and --commit-message are required")
		}

		if err := requireConfirm(cmd, fmt.Sprintf("Type the file path to confirm deletion of %q", path), path); err != nil {
			return err
		}

		if dryRunOutput("repo file delete", map[string]any{
			"project": project, "path": path, "branch": branch,
		}) {
			return nil
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		if err := client.Repos.DeleteFile(apiCtx(), project, path, branch, commitMsg); err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			output.PrintJSON(map[string]any{"path": path, "branch": branch, "action": "deleted"})
			return nil
		}
		output.Success(fmt.Sprintf("Deleted %s from %s", path, branch))
		return nil
	},
}

// 鈹€鈹€鈹€ repo branch list 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var repoBranchListCmd = &cobra.Command{
	Use:   "list",
	Short: "List branches",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, _ := cmd.Flags().GetString("project")
		search, _ := cmd.Flags().GetString("search")
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		if project == "" {
			return failArg("--project is required")
		}

		branches, err := client.Repos.ListBranches(apiCtx(), project, &api.BranchListOpts{Search: search, Limit: limit})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			fields := getFieldsFlag(cmd)
			flat := make([]map[string]any, len(branches))
			for i, b := range branches {
				b := b
				flat[i] = output.FilterMap(output.BranchToMap(output.ToFlatBranch(&b)), fields)
			}
			output.PrintJSON(flat)
			return nil
		}
		if len(branches) == 0 {
			output.Info("No branches found.")
			return nil
		}
		headers := []string{"NAME", "DEFAULT", "PROTECTED", "MERGED", "COMMIT"}
		rows := make([][]string, len(branches))
		for i, b := range branches {
			commitID := ""
			if b.Commit != nil {
				commitID = b.Commit.ShortID
			}
			rows[i] = []string{b.Name, boolStr(b.Default), boolStr(b.Protected), boolStr(b.Merged), commitID}
		}
		output.Table(headers, rows)
		return nil
	},
}

// 鈹€鈹€鈹€ repo branch create 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var repoBranchCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new branch",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		name, _ := cmd.Flags().GetString("name")
		ref, _ := cmd.Flags().GetString("ref")

		if project == "" || name == "" || ref == "" {
			return failArg("--project, --name, and --ref are required")
		}

		if dryRunOutput("repo branch create", map[string]any{"project": project, "name": name, "ref": ref}) {
			return nil
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		b, err := client.Repos.CreateBranch(apiCtx(), project, name, ref)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			output.PrintJSON(output.ToFlatBranch(b))
			return nil
		}
		output.Success(fmt.Sprintf("Created branch %s from %s", b.Name, ref))
		return nil
	},
}

// 鈹€鈹€鈹€ repo branch delete 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var repoBranchDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a branch",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		name, _ := cmd.Flags().GetString("name")

		if project == "" || name == "" {
			return failArg("--project and --name are required")
		}

		if err := requireConfirm(cmd, fmt.Sprintf("Type the branch name to confirm deletion of %q", name), name); err != nil {
			return err
		}

		if dryRunOutput("repo branch delete", map[string]any{"project": project, "name": name}) {
			return nil
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		if err := client.Repos.DeleteBranch(apiCtx(), project, name); err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			output.PrintJSON(map[string]any{"name": name, "action": "deleted"})
			return nil
		}
		output.Success(fmt.Sprintf("Deleted branch %s", name))
		return nil
	},
}

// 鈹€鈹€鈹€ repo commit list 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var repoCommitListCmd = &cobra.Command{
	Use:   "list",
	Short: "List commits",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, _ := cmd.Flags().GetString("project")
		refName, _ := cmd.Flags().GetString("ref-name")
		since, _ := cmd.Flags().GetString("since")
		until, _ := cmd.Flags().GetString("until")
		path, _ := cmd.Flags().GetString("path")
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		if project == "" {
			return failArg("--project is required")
		}

		commits, err := client.Repos.ListCommits(apiCtx(), project, &api.CommitListOpts{
			RefName: refName, Since: since, Until: until, Path: path, Limit: limit,
		})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			fields := getFieldsFlag(cmd)
			flat := make([]map[string]any, len(commits))
			for i, c := range commits {
				c := c
				flat[i] = output.FilterMap(output.CommitToMap(output.ToFlatCommit(&c)), fields)
			}
			output.PrintJSON(flat)
			return nil
		}
		if len(commits) == 0 {
			output.Info("No commits found.")
			return nil
		}
		headers := []string{"SHORT ID", "TITLE", "AUTHOR", "DATE"}
		rows := make([][]string, len(commits))
		for i, c := range commits {
			rows[i] = []string{c.ShortID, truncate(c.Title, 60), c.AuthorName, c.AuthoredDate}
		}
		output.Table(headers, rows)
		return nil
	},
}

// 鈹€鈹€鈹€ repo commit get 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var repoCommitGetCmd = &cobra.Command{
	Use:   "get <sha>",
	Short: "Get a commit by SHA",
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

		c, err := client.Repos.GetCommit(apiCtx(), project, args[0])
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			fields := getFieldsFlag(cmd)
			output.PrintJSON(output.FilterMap(output.CommitToMap(output.ToFlatCommit(c)), fields))
			return nil
		}
		output.Table([]string{"FIELD", "VALUE"}, [][]string{
			{"ID", c.ID},
			{"Short ID", c.ShortID},
			{"Title", c.Title},
			{"Author", c.AuthorName},
			{"Date", c.AuthoredDate},
			{"URL", c.WebURL},
		})
		return nil
	},
}

// 鈹€鈹€鈹€ repo tree 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

var repoTreeCmd = &cobra.Command{
	Use:   "tree",
	Short: "List repository tree entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, _ := cmd.Flags().GetString("project")
		path, _ := cmd.Flags().GetString("path")
		ref, _ := cmd.Flags().GetString("ref")
		recursive, _ := cmd.Flags().GetBool("recursive")
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		if project == "" {
			return failArg("--project is required")
		}

		entries, err := client.Repos.ListTree(apiCtx(), project, &api.TreeOpts{
			Path: path, Ref: ref, Recursive: recursive, Limit: limit,
		})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			fields := getFieldsFlag(cmd)
			flat := make([]map[string]any, len(entries))
			for i, e := range entries {
				e := e
				flat[i] = output.FilterMap(output.TreeEntryToMap(output.ToFlatTreeEntry(&e)), fields)
			}
			output.PrintJSON(flat)
			return nil
		}
		if len(entries) == 0 {
			output.Info("No entries found.")
			return nil
		}
		headers := []string{"TYPE", "NAME", "PATH", "MODE"}
		rows := make([][]string, len(entries))
		for i, e := range entries {
			rows[i] = []string{e.Type, e.Name, e.Path, e.Mode}
		}
		output.Table(headers, rows)
		return nil
	},
}

// 鈹€鈹€鈹€ init 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

func init() {
	rootCmd.AddCommand(repoCmd)

	// file subcommands
	repoCmd.AddCommand(repoFileCmd)
	repoFileCmd.AddCommand(repoFileGetCmd)
	repoFileGetCmd.Flags().String("project", "", "Project ID or path (required)")
	repoFileGetCmd.Flags().String("path", "", "File path in repository (required)")
	repoFileGetCmd.Flags().String("ref", "", "Branch/tag/commit (default: HEAD)")
	repoFileGetCmd.Flags().String("output", "", "Output file path (default: stdout)")
	markOutputType(repoFileGetCmd, "bytes")

	repoFileCmd.AddCommand(repoFileCreateCmd)
	repoFileCreateCmd.Flags().String("project", "", "Project ID or path (required)")
	repoFileCreateCmd.Flags().String("path", "", "File path in repository (required)")
	repoFileCreateCmd.Flags().String("branch", "", "Target branch (required)")
	repoFileCreateCmd.Flags().String("content", "", "File content as text")
	repoFileCreateCmd.Flags().String("content-file", "", "Read content from local file")
	repoFileCreateCmd.Flags().String("commit-message", "", "Commit message (required)")
	repoFileCreateCmd.Flags().String("encoding", "", "Encoding: text|base64 (auto-detected if omitted)")
	markWrite(repoFileCreateCmd)

	repoFileCmd.AddCommand(repoFileUpdateCmd)
	repoFileUpdateCmd.Flags().String("project", "", "Project ID or path (required)")
	repoFileUpdateCmd.Flags().String("path", "", "File path in repository (required)")
	repoFileUpdateCmd.Flags().String("branch", "", "Target branch (required)")
	repoFileUpdateCmd.Flags().String("content", "", "File content as text")
	repoFileUpdateCmd.Flags().String("content-file", "", "Read content from local file")
	repoFileUpdateCmd.Flags().String("commit-message", "", "Commit message (required)")
	repoFileUpdateCmd.Flags().String("encoding", "", "Encoding: text|base64 (auto-detected if omitted)")
	markWrite(repoFileUpdateCmd)

	repoFileCmd.AddCommand(repoFileDeleteCmd)
	repoFileDeleteCmd.Flags().String("project", "", "Project ID or path (required)")
	repoFileDeleteCmd.Flags().String("path", "", "File path in repository (required)")
	repoFileDeleteCmd.Flags().String("branch", "", "Target branch (required)")
	repoFileDeleteCmd.Flags().String("commit-message", "", "Commit message (required)")
	markWrite(repoFileDeleteCmd)
	markConfirm(repoFileDeleteCmd)

	// branch subcommands
	repoCmd.AddCommand(repoBranchCmd)
	repoBranchCmd.AddCommand(repoBranchListCmd)
	repoBranchListCmd.Flags().String("project", "", "Project ID or path (required)")
	repoBranchListCmd.Flags().String("search", "", "Search query")
	repoBranchListCmd.Flags().Int("limit", 20, "Max results (1-100)")
	repoBranchListCmd.Flags().String("fields", "", "Comma-separated fields for JSON output")

	repoBranchCmd.AddCommand(repoBranchCreateCmd)
	repoBranchCreateCmd.Flags().String("project", "", "Project ID or path (required)")
	repoBranchCreateCmd.Flags().String("name", "", "Branch name (required)")
	repoBranchCreateCmd.Flags().String("ref", "", "Source branch or SHA (required)")
	markWrite(repoBranchCreateCmd)

	repoBranchCmd.AddCommand(repoBranchDeleteCmd)
	repoBranchDeleteCmd.Flags().String("project", "", "Project ID or path (required)")
	repoBranchDeleteCmd.Flags().String("name", "", "Branch name (required)")
	markWrite(repoBranchDeleteCmd)
	markConfirm(repoBranchDeleteCmd)

	// commit subcommands
	repoCmd.AddCommand(repoCommitCmd)
	repoCommitCmd.AddCommand(repoCommitListCmd)
	repoCommitListCmd.Flags().String("project", "", "Project ID or path (required)")
	repoCommitListCmd.Flags().String("ref-name", "", "Branch/tag/commit")
	repoCommitListCmd.Flags().String("since", "", "ISO 8601 date (e.g. 2024-01-01T00:00:00Z)")
	repoCommitListCmd.Flags().String("until", "", "ISO 8601 date")
	repoCommitListCmd.Flags().String("path", "", "Filter by file path")
	repoCommitListCmd.Flags().Int("limit", 20, "Max results (1-100)")
	repoCommitListCmd.Flags().String("fields", "", "Comma-separated fields for JSON output")

	repoCommitCmd.AddCommand(repoCommitGetCmd)
	repoCommitGetCmd.Flags().String("project", "", "Project ID or path (required)")
	repoCommitGetCmd.Flags().String("fields", "", "Comma-separated fields for JSON output")

	// tree
	repoCmd.AddCommand(repoTreeCmd)
	repoTreeCmd.Flags().String("project", "", "Project ID or path (required)")
	repoTreeCmd.Flags().String("path", "", "Subdirectory path")
	repoTreeCmd.Flags().String("ref", "", "Branch/tag/commit")
	repoTreeCmd.Flags().Bool("recursive", false, "List recursively")
	repoTreeCmd.Flags().Int("limit", 20, "Max results (1-100)")
	repoTreeCmd.Flags().String("fields", "", "Comma-separated fields for JSON output")
}

// 鈹€鈹€鈹€ helpers 鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€鈹€

// resolveContent returns (content, encoding) from --content or --content-file.
// If the file is binary, it base64-encodes it and returns encoding="base64".
func resolveContent(content, contentFile, encoding string) (string, string, error) {
	if contentFile != "" {
		raw, err := os.ReadFile(contentFile)
		if err != nil {
			return "", "", fmt.Errorf("reading content file: %w", err)
		}
		if encoding == "base64" || !utf8.Valid(raw) {
			return base64.StdEncoding.EncodeToString(raw), "base64", nil
		}
		return string(raw), "text", nil
	}
	if encoding == "" {
		encoding = "text"
	}
	return content, encoding, nil
}

func boolStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Trim at rune boundary
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "\u2026"
}
