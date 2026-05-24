package cmd

import (
	"crypto/subtle"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var confirmFlag string

func initConfirmFlag() {
	rootCmd.PersistentFlags().StringVar(&confirmFlag, "confirm", "", "Non-interactive confirmation: value must match the expected token for this action")
}

// requireConfirm enforces typed confirmation for destructive or high-impact writes.
// Precedence: --confirm <expected> | --force (if allowed) | interactive stdin | fail.
func requireConfirm(cmd *cobra.Command, action, expected string) error {
	if subtle.ConstantTimeCompare([]byte(confirmFlag), []byte(expected)) == 1 {
		return nil
	}
	if forceMode {
		if err := allowForce(); err != nil {
			return err
		}
		return nil
	}
	if isTerminal(os.Stdin) {
		fmt.Printf("%s (type %q to confirm): ", action, expected)
		var input string
		if _, err := fmt.Fscanln(os.Stdin, &input); err != nil {
			return failCancelled("confirmation failed: " + err.Error())
		}
		if subtle.ConstantTimeCompare([]byte(input), []byte(expected)) == 1 {
			return nil
		}
		return failCancelled("confirmation rejected")
	}
	return failCancelled(fmt.Sprintf("confirmation required: re-run with --confirm %q", expected))
}

func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) != 0
}
