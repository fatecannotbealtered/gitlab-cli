package cmd

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/spf13/cobra"
)

// variable bulk-import is a class-B batch (CLI-SPEC §15.7): it parses a .env or
// JSON file and loops create-or-update per key over the existing single-variable
// endpoints. The external contract matches every other batch: plural input (the
// file is the target set), one dry-run preview, one confirm token, aggregated
// items[]/summary, --continue-on-error. Because it writes/overwrites secret CI
// variables it is also write-dangerous (§15.4): it requires the --dangerous
// two-step gate (declared intent) on top of dry-run → confirm (authorized batch).

// importVar is one parsed key/value pair to import. Order is preserved from the
// source file so items[] zips back to the input deterministically.
type importVar struct {
	Key   string
	Value string
}

var variableBulkImportCmd = &cobra.Command{
	Use:   "bulk-import",
	Short: "Import many CI/CD variables from a .env or JSON file (write-dangerous; requires --dangerous)",
	Long: "Import CI/CD variables from a file, creating new keys and updating " +
		"existing ones. --file is a .env (KEY=value lines) or a JSON object " +
		"({\"KEY\":\"value\"}). Format is auto-detected from the extension or content.",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		file, _ := cmd.Flags().GetString("file")
		envScope, _ := cmd.Flags().GetString("env-scope")
		if project == "" {
			return failArg("--project is required")
		}
		if file == "" {
			return failArg("--file is required")
		}

		vars, err := parseVariableImportFile(file)
		if err != nil {
			return err
		}
		if len(vars) == 0 {
			return failArg("no variables found in --file")
		}

		targets := make([]string, len(vars))
		for i, v := range vars {
			targets[i] = v.Key
		}
		// envScope is part of the confirm scope: importing into "*" vs "production"
		// is a different operation and must bind a different token.
		if done, err := prepareBatchWrite(cmd, "variable bulk-import", targets, map[string]any{"project": project, "envScope": envScope}); done || err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		keepGoing := continueOnError(cmd)
		items := make([]batchItem, 0, len(vars))
		var unattempted []string
		for i, v := range vars {
			action, err := importOneVariable(client, project, v, envScope)
			if err != nil {
				items = append(items, batchItem{Target: v.Key, OK: false, Error: batchErrorFor(err), Extra: map[string]any{"envScope": envScope}})
				setExitCode(ExitError)
				if !keepGoing {
					unattempted = targetsTail(targets, i+1)
					break
				}
				continue
			}
			items = append(items, batchItem{Target: v.Key, OK: true, Extra: map[string]any{"action": action, "envScope": envScope}})
		}
		emitBatchResult(items, unattempted)
		return nil
	},
}

// importOneVariable creates the variable, or updates it if it already exists.
// Returns "created" or "updated" for the per-item detail. A 400 with "already
// exists" from create is treated as an update path so re-running an import is
// idempotent.
func importOneVariable(client *api.Client, project string, v importVar, envScope string) (string, error) {
	_, err := client.Variables.Create(apiCtx(), project, &api.VariableCreateOpts{
		Key:              v.Key,
		Value:            v.Value,
		EnvironmentScope: envScope,
	})
	if err == nil {
		return "created", nil
	}
	// Already exists → update in place rather than failing the whole key.
	if isVariableExistsError(err) {
		_, uErr := client.Variables.Update(apiCtx(), project, v.Key, envScope, &api.VariableUpdateOpts{Value: v.Value})
		if uErr != nil {
			return "", uErr
		}
		return "updated", nil
	}
	return "", err
}

// isVariableExistsError reports whether a create failure is GitLab's "key
// already exists" conflict, which the importer maps to an update.
func isVariableExistsError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already") && strings.Contains(msg, "exist") ||
		strings.Contains(msg, "has already been taken")
}

// parseVariableImportFile reads and parses a .env or JSON variable file. JSON is
// detected from a .json extension or a leading '{'; everything else is .env.
func parseVariableImportFile(path string) ([]importVar, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, failArg("reading --file: " + err.Error())
	}
	trimmed := strings.TrimSpace(string(data))
	if strings.HasSuffix(strings.ToLower(path), ".json") || strings.HasPrefix(trimmed, "{") {
		return parseJSONVariables(trimmed)
	}
	return parseDotenvVariables(string(data)), nil
}

// parseJSONVariables parses a flat {"KEY":"value"} object, preserving key order
// via a streaming decoder (Go maps would lose it).
func parseJSONVariables(s string) ([]importVar, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, failArg("--file is not a flat JSON object of key/value pairs")
	}
	var out []importVar
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, failArg("malformed JSON in --file")
		}
		key, _ := keyTok.(string)
		var val string
		if err := dec.Decode(&val); err != nil {
			return nil, failArg("--file value for key " + key + " must be a string")
		}
		if key != "" {
			out = append(out, importVar{Key: key, Value: val})
		}
	}
	return out, nil
}

// parseDotenvVariables parses KEY=value lines, ignoring blanks, comments, and an
// optional leading "export ". Surrounding single/double quotes are stripped.
func parseDotenvVariables(s string) []importVar {
	var out []importVar
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key != "" {
			out = append(out, importVar{Key: key, Value: val})
		}
	}
	return out
}

func init() {
	variableCmd.AddCommand(variableBulkImportCmd)
	variableBulkImportCmd.Flags().String("project", "", "Project ID or path (required)")
	variableBulkImportCmd.Flags().String("file", "", "Path to a .env or JSON file (required)")
	variableBulkImportCmd.Flags().String("env-scope", "*", "Environment scope for all imported variables")
	// bulk-import writes/overwrites secret CI variables, so it is write-dangerous
	// (§15.4): default --continue-on-error to false so a failed write stops the
	// batch rather than charging through the rest of the secrets.
	markBatch(variableBulkImportCmd, false)
	markConfirm(variableBulkImportCmd)
	markRiskLevel(variableBulkImportCmd, "high")
}
