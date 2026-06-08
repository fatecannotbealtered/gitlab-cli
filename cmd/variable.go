package cmd

import (
	"fmt"
	"strings"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var variableCmd = &cobra.Command{
	Use:   "variable",
	Short: "Manage project CI/CD variables",
}

func init() {
	rootCmd.AddCommand(variableCmd)

	// All variable subcommands share --show-values: secret values are redacted
	// from JSON output by default. Pass --show-values to expose them.
	variableCmd.PersistentFlags().Bool("show-values", false, "Include the secret value in --json output (default: redact)")

	// list
	variableListCmd.Flags().String("project", "", "Project ID or path (required)")
	variableListCmd.Flags().Int("limit", 20, "Max results (1-100)")
	variableListCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	variableCmd.AddCommand(variableListCmd)

	// get
	variableGetCmd.Flags().String("project", "", "Project ID or path (required)")
	variableGetCmd.Flags().String("key", "", "Variable key (required)")
	variableGetCmd.Flags().String("filter", "", "Filter by env_scope (e.g. env_scope=production)")
	variableGetCmd.Flags().String("fields", "", "Comma-separated fields to include in JSON output")
	variableCmd.AddCommand(variableGetCmd)

	// create
	variableCreateCmd.Flags().String("project", "", "Project ID or path (required)")
	variableCreateCmd.Flags().String("key", "", "Variable key (required)")
	variableCreateCmd.Flags().String("value", "", "Variable value (required)")
	variableCreateCmd.Flags().String("type", "env_var", "Variable type: env_var|file")
	variableCreateCmd.Flags().Bool("protected", false, "Protect the variable")
	variableCreateCmd.Flags().Bool("masked", false, "Mask the variable in job logs")
	variableCreateCmd.Flags().Bool("raw", false, "Disable variable expansion")
	variableCreateCmd.Flags().String("env-scope", "*", "Environment scope")
	variableCreateCmd.Flags().String("description", "", "Variable description")
	variableCmd.AddCommand(variableCreateCmd)
	markWrite(variableCreateCmd)

	// update
	variableUpdateCmd.Flags().String("project", "", "Project ID or path (required)")
	variableUpdateCmd.Flags().String("key", "", "Variable key (required)")
	variableUpdateCmd.Flags().String("value", "", "New value")
	variableUpdateCmd.Flags().String("type", "", "Variable type: env_var|file")
	variableUpdateCmd.Flags().String("protected", "", "Set protected: true|false")
	variableUpdateCmd.Flags().String("masked", "", "Set masked: true|false")
	variableUpdateCmd.Flags().String("raw", "", "Set raw: true|false")
	variableUpdateCmd.Flags().String("env-scope", "", "Filter by environment scope (also sets new scope)")
	variableUpdateCmd.Flags().String("description", "", "Variable description")
	variableCmd.AddCommand(variableUpdateCmd)
	markWrite(variableUpdateCmd)

	// delete
	variableDeleteCmd.Flags().String("project", "", "Project ID or path (required)")
	variableDeleteCmd.Flags().String("key", "", "Variable key (required)")
	variableDeleteCmd.Flags().String("filter", "", "Filter by env_scope (e.g. env_scope=production)")
	variableCmd.AddCommand(variableDeleteCmd)
	markWrite(variableDeleteCmd)
	markConfirm(variableDeleteCmd)
}

var variableListCmd = &cobra.Command{
	Use:   "list",
	Short: "List CI/CD variables for a project",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkShowValues(cmd); err != nil {
			return err
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, _ := cmd.Flags().GetString("project")
		if project == "" {
			return failArg("--project is required")
		}
		limit, err := requireLimit(cmd)
		if err != nil {
			return err
		}

		vars, err := client.Variables.List(apiCtx(), project, limit)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			showValues, _ := cmd.Flags().GetBool("show-values")
			fields := getFieldsFlag(cmd)
			if showValues {
				out := make([]map[string]any, len(vars))
				for i := range vars {
					out[i] = output.FilterMap(output.VariableWithValueToMap(output.ToFlatVariableWithValue(&vars[i])), fields)
				}
				printSimpleListJSON(cmd, out, limit)
				return nil
			}
			// Default: redact secret values
			out := make([]map[string]any, len(vars))
			for i := range vars {
				out[i] = output.FilterMap(output.VariableToMap(output.ToFlatVariable(&vars[i])), fields)
			}
			printSimpleListJSON(cmd, out, limit)
			return nil
		}
		if len(vars) == 0 {
			output.Info("No variables found.")
			return nil
		}
		headers := []string{"KEY", "TYPE", "PROTECTED", "MASKED", "ENV_SCOPE"}
		rows := make([][]string, len(vars))
		for i, v := range vars {
			rows[i] = []string{
				v.Key,
				v.VariableType,
				fmt.Sprintf("%v", v.Protected),
				fmt.Sprintf("%v", v.Masked),
				v.EnvironmentScope,
			}
		}
		output.Table(headers, rows)
		return nil
	},
}

var variableGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get a single CI/CD variable",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := checkShowValues(cmd); err != nil {
			return err
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}
		project, _ := cmd.Flags().GetString("project")
		key, _ := cmd.Flags().GetString("key")
		if project == "" {
			return failArg("--project is required")
		}
		if key == "" {
			return failArg("--key is required")
		}
		envScope := parseEnvScopeFilter(cmd)

		v, err := client.Variables.Get(apiCtx(), project, key, envScope)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			showValues, _ := cmd.Flags().GetBool("show-values")
			fields := getFieldsFlag(cmd)
			if showValues {
				output.PrintJSON(output.FilterMap(output.VariableWithValueToMap(output.ToFlatVariableWithValue(v)), fields))
			} else {
				output.PrintJSON(output.FilterMap(output.VariableToMap(output.ToFlatVariable(v)), fields))
			}
			return nil
		}
		// Table mode: hide value
		headers := []string{"KEY", "TYPE", "VALUE", "PROTECTED", "MASKED", "ENV_SCOPE"}
		rows := [][]string{{
			v.Key,
			v.VariableType,
			"***",
			fmt.Sprintf("%v", v.Protected),
			fmt.Sprintf("%v", v.Masked),
			v.EnvironmentScope,
		}}
		output.Table(headers, rows)
		return nil
	},
}

var variableCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a CI/CD variable",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		key, _ := cmd.Flags().GetString("key")
		value, _ := cmd.Flags().GetString("value")
		if project == "" {
			return failArg("--project is required")
		}
		if key == "" {
			return failArg("--key is required")
		}
		if value == "" {
			return failArg("--value is required")
		}

		varType, _ := cmd.Flags().GetString("type")
		protected, _ := cmd.Flags().GetBool("protected")
		masked, _ := cmd.Flags().GetBool("masked")
		raw, _ := cmd.Flags().GetBool("raw")
		envScope, _ := cmd.Flags().GetString("env-scope")
		description, _ := cmd.Flags().GetString("description")

		if done, err := prepareWrite(cmd, "create variable", map[string]any{
			"project":  project,
			"key":      key,
			"type":     varType,
			"envScope": envScope,
		}); done || err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		v, err := client.Variables.Create(apiCtx(), project, &api.VariableCreateOpts{
			Key:              key,
			Value:            value,
			VariableType:     varType,
			Protected:        protected,
			Masked:           masked,
			Raw:              raw,
			EnvironmentScope: envScope,
			Description:      description,
		})
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			showValues, _ := cmd.Flags().GetBool("show-values")
			if showValues {
				output.PrintJSON(output.VariableWithValueToMap(output.ToFlatVariableWithValue(v)))
			} else {
				output.PrintJSON(output.VariableToMap(output.ToFlatVariable(v)))
			}
			return nil
		}
		output.Success(fmt.Sprintf("Variable %q created (scope: %s)", v.Key, v.EnvironmentScope))
		return nil
	},
}

var variableUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update a CI/CD variable",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		key, _ := cmd.Flags().GetString("key")
		if project == "" {
			return failArg("--project is required")
		}
		if key == "" {
			return failArg("--key is required")
		}
		envScope, _ := cmd.Flags().GetString("env-scope")

		opts := &api.VariableUpdateOpts{}
		if v, _ := cmd.Flags().GetString("value"); v != "" {
			opts.Value = v
		}
		if v, _ := cmd.Flags().GetString("type"); v != "" {
			opts.VariableType = v
		}
		if v, _ := cmd.Flags().GetString("description"); v != "" {
			opts.Description = v
		}
		if v, _ := cmd.Flags().GetString("protected"); v != "" {
			b := v == "true"
			opts.Protected = &b
		}
		if v, _ := cmd.Flags().GetString("masked"); v != "" {
			b := v == "true"
			opts.Masked = &b
		}
		if v, _ := cmd.Flags().GetString("raw"); v != "" {
			b := v == "true"
			opts.Raw = &b
		}
		if envScope != "" {
			opts.EnvironmentScope = envScope
		}

		if done, err := prepareWrite(cmd, "update variable", map[string]any{
			"project":  project,
			"key":      key,
			"envScope": envScope,
		}); done || err != nil {
			return err
		}

		client, _, err := newClient()
		if err != nil {
			return err
		}

		v, err := client.Variables.Update(apiCtx(), project, key, envScope, opts)
		if err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			showValues, _ := cmd.Flags().GetBool("show-values")
			if showValues {
				output.PrintJSON(output.VariableWithValueToMap(output.ToFlatVariableWithValue(v)))
			} else {
				output.PrintJSON(output.VariableToMap(output.ToFlatVariable(v)))
			}
			return nil
		}
		output.Success(fmt.Sprintf("Variable %q updated", v.Key))
		return nil
	},
}

var variableDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a CI/CD variable",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := cmd.Flags().GetString("project")
		key, _ := cmd.Flags().GetString("key")
		if project == "" {
			return failArg("--project is required")
		}
		if key == "" {
			return failArg("--key is required")
		}
		envScope := parseEnvScopeFilter(cmd)

		confirmPayload := map[string]any{
			"project":  project,
			"key":      key,
			"envScope": envScope,
		}
		if done, err := prepareWrite(cmd, "delete variable", confirmPayload); done || err != nil {
			return err
		}
		client, _, err := newClient()
		if err != nil {
			return err
		}

		if err := client.Variables.Delete(apiCtx(), project, key, envScope); err != nil {
			return handleAPIError(err, jsonMode)
		}

		if jsonMode {
			output.PrintJSON(output.MarkUntrusted(map[string]any{"deleted": true, "key": key}, "key"))
			return nil
		}
		output.Success(fmt.Sprintf("Variable %q deleted", key))
		return nil
	},
}

// parseEnvScopeFilter parses --filter env_scope=<value> and returns the scope value.
func parseEnvScopeFilter(cmd *cobra.Command) string {
	filter, _ := cmd.Flags().GetString("filter")
	if filter == "" {
		return ""
	}
	// Accept "env_scope=production" or just "production"
	if strings.HasPrefix(filter, "env_scope=") {
		return strings.TrimPrefix(filter, "env_scope=")
	}
	return filter
}
