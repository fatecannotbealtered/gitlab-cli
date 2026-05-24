package cmd

import (
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

// failArg reports a validation error (exit 2).
func failArg(msg string) error {
	emitError(msg, ExitBadArgs, output.ErrValidation)
	return ErrSilent
}

// failNotFound reports a missing resource (exit 4).
func failNotFound(msg string) error {
	emitError(msg, ExitNotFound, output.ErrNotFound)
	return ErrSilent
}

// failCancelled reports user-aborted or missing confirmation (exit 10).
func failCancelled(msg string) error {
	emitError(msg, ExitCancelled, output.ErrCancelled)
	return ErrSilent
}

// failWithCode reports an error with a custom exit code and error code.
func failWithCode(msg string, exit int, code output.ErrorCode) error {
	emitError(msg, exit, code)
	return ErrSilent
}

func emitError(msg string, exit int, code output.ErrorCode) {
	if jsonMode {
		output.PrintErrorJSONWithCode(msg, 0, code)
	} else {
		output.Error(msg)
	}
	setExitCode(exit)
}

// requireFlagString returns the flag value or fails if empty.
func requireFlagString(cmd *cobra.Command, name, label string) (string, error) {
	v, err := cmd.Flags().GetString(name)
	if err != nil {
		return "", failArg(err.Error())
	}
	if v == "" {
		return "", failArg(label + " is required")
	}
	return v, nil
}
