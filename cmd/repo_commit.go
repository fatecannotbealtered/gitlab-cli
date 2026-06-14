package cmd

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

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
