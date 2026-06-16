package cmd

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

// commitFanOutConcurrency bounds how many projects are queried in parallel
// during a multi-project commit list. Enough to be fast, low enough to stay
// polite to the instance (the client also backs off on 429).
const commitFanOutConcurrency = 8

// resolveCommitScope turns the scope flags into the concrete project set to
// query plus a scope label. Exactly one of --project / --group / --all-projects
// must be given; --all-projects must be bound to --author (CLI-SPEC: an
// instance-wide scan only makes sense for a specific person, never a bare dump).
func resolveCommitScope(cmd *cobra.Command, client *api.Client) ([]string, string, error) {
	projectsFlag := parsePluralFlag(cmd, "project")
	groupsFlag := parsePluralFlag(cmd, "group")
	allProjects, _ := cmd.Flags().GetBool("all-projects")
	author, _ := cmd.Flags().GetString("author")

	set := 0
	if len(projectsFlag) > 0 {
		set++
	}
	if len(groupsFlag) > 0 {
		set++
	}
	if allProjects {
		set++
	}
	switch {
	case set == 0:
		return nil, "", failArg("provide a scope: --project, --group, or --all-projects")
	case set > 1:
		return nil, "", failArg("--project, --group, and --all-projects are mutually exclusive")
	}

	switch {
	case len(projectsFlag) > 0:
		return projectsFlag, "project", nil
	case len(groupsFlag) > 0:
		projects, err := enumerateGroupProjects(client, groupsFlag)
		if err != nil {
			return nil, "", err
		}
		return projects, "group", nil
	default: // allProjects
		if author == "" {
			return nil, "", failArg("--all-projects requires --author: an instance-wide scan must be bound to a user")
		}
		projects, err := client.Projects.List(apiCtx(), &api.ProjectListOpts{Membership: true, Limit: listAllMax})
		if err != nil {
			return nil, "", handleAPIError(err, jsonMode)
		}
		return projectPaths(projects), "all-projects", nil
	}
}

// enumerateGroupProjects expands each group (subgroups included) into its
// project paths, de-duplicated with input order preserved.
func enumerateGroupProjects(client *api.Client, groups []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0)
	for _, g := range groups {
		projects, err := client.Projects.ListGroupProjects(apiCtx(), g, listAllMax)
		if err != nil {
			return nil, handleAPIError(err, jsonMode)
		}
		for _, p := range projects {
			if p.PathWithNamespace == "" || seen[p.PathWithNamespace] {
				continue
			}
			seen[p.PathWithNamespace] = true
			out = append(out, p.PathWithNamespace)
		}
	}
	return out, nil
}

// projectPaths returns the path_with_namespace for each project (the readable
// identifier used to annotate fan-out results and re-run per-project queries).
func projectPaths(projects []api.Project) []string {
	out := make([]string, 0, len(projects))
	for _, p := range projects {
		if p.PathWithNamespace != "" {
			out = append(out, p.PathWithNamespace)
		}
	}
	return out
}

// commitProjectResult is one project's outcome in a fan-out: either its commits
// or the error that project alone hit (a single failure never aborts the batch).
type commitProjectResult struct {
	project string
	commits []api.Commit
	err     error
}

// runCommitFanOut queries every project concurrently, aggregates the commits
// (each annotated with its project), and reports scanned/failed projects so an
// agent knows the coverage of the answer.
func runCommitFanOut(cmd *cobra.Command, client *api.Client, projects []string, scope string, opts *api.CommitListOpts, limit int) error {
	if len(projects) == 0 {
		if jsonMode {
			output.PrintJSON(map[string]any{
				"items": []map[string]any{}, "count": 0, "limit": limit, "hasMore": false,
				"scope": scope, "projectsScanned": 0,
			})
			return nil
		}
		output.Info("No projects in scope.")
		return nil
	}

	results := make([]commitProjectResult, len(projects))
	sem := make(chan struct{}, commitFanOutConcurrency)
	var wg sync.WaitGroup
	for i, project := range projects {
		wg.Add(1)
		go func(i int, project string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			commits, err := client.Repos.ListCommits(apiCtx(), project, opts)
			results[i] = commitProjectResult{project: project, commits: commits, err: err}
		}(i, project)
	}
	wg.Wait()

	fields := getFieldsFlag(cmd)
	items := make([]map[string]any, 0)
	projectErrors := make([]map[string]any, 0)
	scanned := 0
	for _, r := range results {
		if r.err != nil {
			be := batchErrorFor(r.err)
			projectErrors = append(projectErrors, map[string]any{
				"project": r.project,
				"error":   map[string]any{"code": be.Code, "retryable": be.Retryable},
			})
			continue
		}
		scanned++
		for _, c := range r.commits {
			c := c
			m := output.FilterMap(output.CommitToMap(output.ToFlatCommit(&c)), fields)
			m["project"] = r.project
			items = append(items, m)
		}
	}

	if !jsonMode {
		if len(items) == 0 {
			output.Info(fmt.Sprintf("No commits found across %d project(s).", scanned))
		} else {
			headers := []string{"PROJECT", "SHORT ID", "TITLE", "AUTHOR", "DATE"}
			rows := make([][]string, 0, len(items))
			for _, r := range results {
				for _, c := range r.commits {
					rows = append(rows, []string{r.project, c.ShortID, truncate(c.Title, 48), c.AuthorName, c.AuthoredDate})
				}
			}
			output.Table(headers, rows)
		}
		if len(projectErrors) > 0 {
			output.Warn(fmt.Sprintf("%d project(s) failed to scan; see JSON output for details.", len(projectErrors)))
		}
		return nil
	}

	out := map[string]any{
		"items":           items,
		"count":           len(items),
		"limit":           limit,
		"hasMore":         false,
		"scope":           scope,
		"projectsScanned": scanned,
	}
	if len(projectErrors) > 0 {
		out["projectErrors"] = projectErrors
	}
	output.PrintJSON(out)
	return nil
}

// renderCommitList renders a single project's commit[] in the original shape.
func renderCommitList(cmd *cobra.Command, commits []api.Commit, limit int) error {
	if jsonMode {
		fields := getFieldsFlag(cmd)
		flat := make([]map[string]any, len(commits))
		for i, c := range commits {
			c := c
			flat[i] = output.FilterMap(output.CommitToMap(output.ToFlatCommit(&c)), fields)
		}
		printSimpleListJSON(cmd, flat, limit)
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
}

// repoCommitDiffCmd shows the per-file diff for a single commit. It is a heavy
// sub-resource kept as its own command (never inlined into list): an agent
// triages with `commit list --with-stats`, then reads diffs only for the SHAs
// that matter. --fields projects each file entry (e.g. drop the patch text for a
// cheap inventory); --path narrows to one file.
var repoCommitDiffCmd = &cobra.Command{
	Use:   "diff <sha>",
	Short: "Show per-file diff for a commit",
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
		pathFilter, _ := cmd.Flags().GetString("path")

		files, err := client.Repos.GetCommitDiff(apiCtx(), project, args[0])
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			fields := getFieldsFlag(cmd)
			fileMaps := make([]map[string]any, 0, len(files))
			for _, f := range files {
				f := f
				if pathFilter != "" && f.NewPath != pathFilter && f.OldPath != pathFilter {
					continue
				}
				fileMaps = append(fileMaps, output.FilterMap(output.CommitDiffFileToMap(output.ToFlatCommitDiffFile(&f)), fields))
			}
			output.PrintJSON(map[string]any{
				"sha":          args[0],
				"filesChanged": len(fileMaps),
				"files":        fileMaps,
			})
			return nil
		}

		shown := 0
		headers := []string{"FILE", "+", "-", "CHANGE"}
		rows := make([][]string, 0, len(files))
		for _, f := range files {
			f := f
			if pathFilter != "" && f.NewPath != pathFilter && f.OldPath != pathFilter {
				continue
			}
			shown++
			flat := output.ToFlatCommitDiffFile(&f)
			rows = append(rows, []string{
				diffFilePath(flat),
				fmt.Sprintf("%d", flat.Additions),
				fmt.Sprintf("%d", flat.Deletions),
				diffChangeKind(flat),
			})
		}
		if shown == 0 {
			output.Info("No file changes in diff.")
			return nil
		}
		output.Table(headers, rows)
		return nil
	},
}

// diffFilePath picks the meaningful path for a changed file (new path, except a
// deletion where only the old path remains).
func diffFilePath(f output.FlatCommitDiffFile) string {
	if f.DeletedFile && f.NewPath == "" {
		return f.OldPath
	}
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

func diffChangeKind(f output.FlatCommitDiffFile) string {
	switch {
	case f.NewFile:
		return "added"
	case f.DeletedFile:
		return "deleted"
	case f.RenamedFile:
		return "renamed"
	default:
		return "modified"
	}
}

// repo commit is the class-A atomic multi-file commit (CLI-SPEC §15.7): one
// commit applies many create/update/delete/move actions via the native
// POST /repository/commits actions[] endpoint. Because it is server-side atomic,
// the per-item items[] either all succeed (one commit) or all fail together —
// the output schema says so rather than implying a partial-apply transaction.

const commitActionsMax = 1000

var repoCommitCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create one atomic commit applying multiple file actions",
	Long: "Create a single commit that applies multiple file actions atomically " +
		"(create/update/delete/move) in one upstream request.\n\n" +
		"Each --action is 'type:field=value;field=value':\n" +
		"  --action 'create:path=docs/a.md;content=hello'\n" +
		"  --action 'update:path=README.md;content_file=./README.md'\n" +
		"  --action 'delete:path=old.txt'\n" +
		"  --action 'move:path=new/x.go;previous_path=old/x.go'\n",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		branch, _ := cmd.Flags().GetString("branch")
		message, _ := cmd.Flags().GetString("message")
		startBranch, _ := cmd.Flags().GetString("start-branch")
		rawActions, _ := cmd.Flags().GetStringArray("action")

		if project == "" || branch == "" || message == "" {
			return failArg("--project, --branch, and --message are required")
		}
		if len(rawActions) == 0 {
			return failArg("at least one --action is required")
		}
		if len(rawActions) > commitActionsMax {
			return failArg(fmt.Sprintf("a single commit supports at most %d actions, got %d", commitActionsMax, len(rawActions)))
		}

		actions, targets, err := parseCommitActions(rawActions)
		if err != nil {
			return err
		}

		// The confirm token binds the whole action set (paths + action types), so
		// changing any action invalidates it — consistent with the §15.2 contract
		// that the token covers the whole resolved batch.
		extra := map[string]any{"project": project, "branch": branch}
		if done, err := prepareBatchWrite(cmd, "repo commit create", targets, extra); done || err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		commit, err := client.Repos.CreateCommit(idempotentCtx(cmd), project, api.CommitCreateOpts{
			Branch:        branch,
			CommitMessage: message,
			StartBranch:   startBranch,
			Actions:       actions,
		})
		if err != nil {
			// Atomic: the whole commit failed, so every action failed identically.
			// Report it as a per-item result so the agent reads it through the same
			// items[]/summary shape as any other batch.
			itemErr := batchErrorFor(err)
			items := make([]batchItem, 0, len(targets))
			for _, tgt := range targets {
				items = append(items, batchItem{Target: tgt, OK: false, Error: itemErr})
			}
			setExitCode(exitCodeForBatchError(err))
			emitBatchResult(items, nil)
			return nil
		}

		items := make([]batchItem, 0, len(targets))
		for i, tgt := range targets {
			items = append(items, batchItem{Target: tgt, OK: true, Extra: map[string]any{"action": actions[i].Action}})
		}
		if !jsonMode {
			output.Success(fmt.Sprintf("Created commit %s on %s (%d actions)", commit.ShortID, branch, len(actions)))
			return nil
		}
		// One atomic commit: emit the aggregated items[] plus the resulting commit.
		succeeded := len(items)
		out := make([]map[string]any, 0, len(items))
		for _, it := range items {
			out = append(out, map[string]any{"target": it.Target, "ok": it.OK, "action": it.Extra["action"]})
		}
		output.PrintJSON(map[string]any{
			"commitId": commit.ID,
			"shortId":  commit.ShortID,
			"branch":   branch,
			"webUrl":   commit.WebURL,
			"items":    out,
			"summary":  map[string]any{"total": len(items), "succeeded": succeeded, "failed": 0},
		})
		return nil
	},
}

// parseCommitActions parses repeatable --action specs into API actions plus the
// ordered target list (file paths). Each spec is 'type:field=value;field=value'.
func parseCommitActions(specs []string) ([]api.CommitAction, []string, error) {
	actions := make([]api.CommitAction, 0, len(specs))
	targets := make([]string, 0, len(specs))
	for _, spec := range specs {
		typePart, rest, found := strings.Cut(spec, ":")
		if !found {
			return nil, nil, failArgf("invalid --action %q: expected 'type:field=value;...'", spec)
		}
		actionType := strings.TrimSpace(typePart)
		switch actionType {
		case "create", "update", "delete", "move":
		default:
			return nil, nil, failArgf("invalid --action type %q: must be create|update|delete|move", actionType)
		}

		fields, err := parseActionFields(rest)
		if err != nil {
			return nil, nil, err
		}
		path := fields["path"]
		if path == "" {
			return nil, nil, failArgf("--action %q: path is required", spec)
		}

		act := api.CommitAction{Action: actionType, FilePath: path}
		if pp := fields["previous_path"]; pp != "" {
			act.PreviousPath = pp
		}
		if actionType == "move" && act.PreviousPath == "" {
			return nil, nil, failArgf("--action move %q: previous_path is required", path)
		}
		if actionType == "create" || actionType == "update" {
			content, encoding, err := resolveActionContent(fields)
			if err != nil {
				return nil, nil, err
			}
			act.Content = content
			act.Encoding = encoding
		}
		actions = append(actions, act)
		targets = append(targets, path)
	}
	return actions, targets, nil
}

// parseActionFields parses 'field=value;field=value' into a map. Values may not
// contain ';'; for arbitrary file content use content_file.
func parseActionFields(s string) (map[string]string, error) {
	out := map[string]string{}
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, val, found := strings.Cut(part, "=")
		if !found {
			return nil, failArgf("invalid action field %q: expected key=value", part)
		}
		out[strings.TrimSpace(key)] = val
	}
	return out, nil
}

// resolveActionContent returns (content, encoding) from the content / content_file
// fields of a create/update action, base64-encoding binary content_file input.
func resolveActionContent(fields map[string]string) (string, string, error) {
	if cf := fields["content_file"]; cf != "" {
		raw, err := os.ReadFile(cf)
		if err != nil {
			return "", "", failArg("reading content_file: " + err.Error())
		}
		if !utf8.Valid(raw) {
			return base64.StdEncoding.EncodeToString(raw), "base64", nil
		}
		return string(raw), "text", nil
	}
	return fields["content"], "text", nil
}

// exitCodeForBatchError maps a whole-batch failure to a semantic exit code,
// reusing the API-status mapping used by single writes.
func exitCodeForBatchError(err error) int {
	be := batchErrorFor(err)
	switch be.Code {
	case output.ErrNotFound:
		return ExitNotFound
	case output.ErrAuth:
		return ExitAuth
	case output.ErrForbidden:
		return ExitForbidden
	case output.ErrConflict:
		return ExitConflict
	case output.ErrRateLimit:
		return ExitRateLimit
	default:
		return ExitError
	}
}
