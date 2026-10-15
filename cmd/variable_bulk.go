package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// variable bulk-import is a batch (CLI-063): one variable per item, created when
// the key is new in the scope and its value replaced when it exists, through
// App.WriteBatch, so one preview and one token cover the file. The file holds
// secrets, so it is a secret file input: the token binds a keyed digest of its
// content, previews show it as ***, and no output names a value.

// importVar is one key and value of the import file, in file order.
type importVar struct {
	key, value string
	line       int
}

// parseVariableImport reads a .env file (KEY=value lines; blank lines, # comments
// and a leading "export " are skipped; one pair of surrounding quotes is removed) or
// a flat JSON object of strings (a .json file, or content that starts with "{").
// Errors name a line number, never a key or a value.
func parseVariableImport(path string, content []byte) ([]importVar, error) {
	var vars []importVar
	var err error
	if strings.HasSuffix(strings.ToLower(path), ".json") || bytes.HasPrefix(bytes.TrimSpace(content), []byte("{")) {
		vars, err = parseVariableJSON(content)
	} else {
		vars, err = parseVariableDotenv(string(content))
	}
	if err != nil {
		return nil, err
	}
	seen := map[string]int{}
	for _, v := range vars {
		if !ciVariableKey.MatchString(v.key) {
			return nil, aicli.Validation("--file", "KEY=value lines (or a JSON object) whose keys are letters, digits and _",
				fmt.Sprintf("Entry %d of --file has a key that is not letters, digits and _.", v.line))
		}
		if first, dup := seen[v.key]; dup {
			return nil, aicli.Validation("--file", "each key once", fmt.Sprintf("Entries %d and %d of --file set the same key.", first, v.line))
		}
		seen[v.key] = v.line
	}
	if len(vars) == 0 {
		e := aicli.Usage("missing_parameter", "--file", "--file holds no variables.")
		e.Details["next_action"] = "Pass a .env file with KEY=value lines, or a JSON object of strings."
		return nil, e
	}
	return vars, nil
}

func parseVariableJSON(content []byte) ([]importVar, error) {
	invalid := func(why string) error {
		return aicli.Validation("--file", `a flat JSON object of strings: {"KEY": "value"}`, why)
	}
	dec := json.NewDecoder(bytes.NewReader(content))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, invalid("--file is not a JSON object.")
	}
	var out []importVar
	for n := 1; dec.More(); n++ {
		tok, err := dec.Token()
		if err != nil {
			return nil, invalid("--file is not valid JSON.")
		}
		key, _ := tok.(string)
		var value string
		if err := dec.Decode(&value); err != nil {
			return nil, invalid(fmt.Sprintf("Entry %d of --file is not a string value.", n))
		}
		out = append(out, importVar{key: key, value: value, line: n})
	}
	if _, err := dec.Token(); err != nil {
		return nil, invalid("--file is not valid JSON.")
	}
	return out, nil
}

func parseVariableDotenv(s string) ([]importVar, error) {
	var out []importVar
	for i, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, found := strings.Cut(line, "=")
		if !found {
			return nil, aicli.Validation("--file", "KEY=value lines", fmt.Sprintf("Line %d of --file is not KEY=value.", i+1))
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		out = append(out, importVar{key: strings.TrimSpace(key), value: value, line: i + 1})
	}
	return out, nil
}

func addVariableBulkImport(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "bulk-import", RunE: func(cmd *cobra.Command, _ []string) error {
		content, err := app.ReadFileInput(cmd, "file")
		if err != nil {
			return err
		}
		path, _ := cmd.Flags().GetString("file")
		vars, err := parseVariableImport(path, content)
		if err != nil {
			return err
		}
		scope, _ := cmd.Flags().GetString("env-scope")
		if scope = strings.TrimSpace(scope); !variableScopeOK(scope) {
			return aicli.Validation("--env-scope", "an environment name or pattern such as production, review/* or *",
				"--env-scope is not an environment scope.")
		}
		gl := gitlabFor(app)
		project, err := projectFlag(cmd, "", gl)
		if err != nil {
			return err
		}
		// Each item's Preview decides create or update; at confirm the gate runs every
		// Preview again before the first Apply, and a decision that changed is drift.
		creates := make([]bool, len(vars))
		projectSeen := false
		items := make([]aicli.BatchItem, 0, len(vars))
		for i, v := range vars {
			ref := variableRef(project, v.key, scope)
			items = append(items, aicli.BatchItem{
				Target: aicli.Target{Key: ref, Kind: "variable"},
				Preview: func(ctx context.Context) (aicli.Preview, error) {
					client, err := gl.get()
					if err != nil {
						return aicli.Preview{}, err
					}
					var apiErr *api.APIError
					switch _, err := client.Variables.Get(ctx, project, v.key, scope); {
					case err == nil:
						creates[i] = false
						return aicli.Preview{Action: fmt.Sprintf("Replace the value of CI/CD variable %s", variableName(v.key, scope)),
							CanApply: true, Changes: variableChanges(&api.Variable{}, variableWant{value: &v.value})}, nil
					case errors.As(err, &apiErr) && apiErr.StatusCode == 404:
						if !projectSeen {
							if err := projectExists(ctx, client, project); err != nil {
								return aicli.Preview{}, err
							}
							projectSeen = true
						}
						creates[i] = true
						return aicli.Preview{Action: fmt.Sprintf("Create CI/CD variable %s", variableName(v.key, scope)), CanApply: true,
							Changes: variableChanges(nil, variableWant{create: true, key: v.key, scope: scope, value: &v.value})}, nil
					default:
						return aicli.Preview{}, readError(err, projectNotFound(), requiresRole("maintainer"))
					}
				},
				Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
					client, err := gl.get()
					if err != nil {
						return aicli.Result{}, aicli.NotDispatched(err)
					}
					var got *api.Variable
					want := variableWant{value: &v.value}
					if creates[i] {
						want = variableWant{create: true, key: v.key, scope: scope, value: &v.value}
						got, err = client.Variables.Create(ctx, project, &api.VariableCreateOpts{Key: v.key, Value: v.value,
							EnvironmentScope: scope})
					} else {
						got, err = client.Variables.Update(ctx, project, v.key, scope, &api.VariableUpdateOpts{Value: &v.value})
					}
					if err != nil {
						return aicli.Result{}, classifyWriteError(err, requiresRole("maintainer"), variableNotFound(project))
					}
					obs, mismatched := variableObserved(*got, want)
					return aicli.Result{Observed: obs, Mismatched: mismatched, Data: variableOut(project, *got),
						Untrusted: variableUntrusted}, nil
				},
			})
		}
		return app.WriteBatch(cmd, aicli.BatchSpec{Items: items})
	}}
	c.Flags().String("project", "", "Project path (acme/web), web URL or numeric ID")
	_ = c.MarkFlagRequired("project")
	c.Flags().String("file", "", "A .env file (KEY=value lines) or a JSON object of strings; - reads stdin. Its values are secret")
	_ = c.MarkFlagRequired("file")
	c.Flags().String("env-scope", "*", "Environment scope of every variable: * (every environment), production, review/*")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Create or update many CI/CD variables from a .env or JSON file in one batch (configure: dangerous)",
		Effects: []aicli.Effect{aicli.Configure}, Output: "variable", BatchFlag: "file",
		FileInputs: map[string]string{"file": "variables"}, Secrets: []string{"file"},
		ErrorCodes: gitlabWriteCodes, Maturity: aicli.Experimental, Evidence: []string{"simulated:cmd/variable_bulk_test.go"},
		Examples: []string{"gitlab-cli variable bulk-import --project acme/web --file ci.env --dangerous --dry-run",
			"gitlab-cli variable bulk-import --project acme/web --file ci.env --dangerous --confirm <confirm_token>"}})
}
