package cmd

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var confirmFlag string
var confirmNow = time.Now

const confirmTokenTTL = 15 * time.Minute

// test hooks
var (
	stdinIsTerminalFunc  = isTerminal
	readConfirmInputFunc = func() (string, error) {
		var input string
		_, err := fmt.Fscanln(os.Stdin, &input)
		return input, err
	}
)

func initConfirmFlag() {
	rootCmd.PersistentFlags().StringVar(&confirmFlag, "confirm", "", "Non-interactive confirmation: value must match the expected token for this action")
}

func newConfirmToken(action string, payload any) (string, time.Time) {
	expires := confirmNow().UTC().Add(confirmTokenTTL).Truncate(time.Second)
	return buildConfirmToken(action, payload, expires), expires
}

func buildConfirmToken(action string, payload any, expires time.Time) string {
	seed := canonicalConfirmSeed(action, payload, expires.Unix())
	sum := sha256.Sum256(seed)
	return "ct_" + strconv.FormatInt(expires.Unix(), 10) + "_" + hex.EncodeToString(sum[:8])
}

func canonicalConfirmSeed(action string, payload any, expiresUnix int64) []byte {
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte(fmt.Sprintf("%v", payload))
	}
	return []byte(action + "\n" + strconv.FormatInt(expiresUnix, 10) + "\n" + string(body))
}

func validateConfirmToken(token, action string, payload any) error {
	if token == "" {
		return failConfirmRequired("confirmation required: run with --dry-run and retry with --confirm <confirm_token>")
	}
	parts := strings.Split(token, "_")
	if len(parts) != 3 || parts[0] != "ct" {
		return failWithCode("confirmation token is invalid; re-run with --dry-run", ExitConflict, output.ErrConflict)
	}
	expiresUnix, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return failWithCode("confirmation token is invalid; re-run with --dry-run", ExitConflict, output.ErrConflict)
	}
	expires := time.Unix(expiresUnix, 0).UTC()
	if !confirmNow().UTC().Before(expires) {
		return failWithCode("confirmation token expired; re-run with --dry-run", ExitConflict, output.ErrConflict)
	}
	expected := buildConfirmToken(action, payload, expires)
	if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1 {
		return nil
	}
	return failWithCode("confirmation token does not match this operation; re-run with --dry-run", ExitConflict, output.ErrConflict)
}

// requireConfirm enforces non-interactive confirmation for destructive or high-impact writes.
func requireConfirm(cmd *cobra.Command, action string, payload any) error {
	defer clearConfirmFlag()
	return validateConfirmToken(confirmFlag, action, payload)
}

func clearConfirmFlag() {
	confirmFlag = ""
	if flag := rootCmd.PersistentFlags().Lookup("confirm"); flag != nil {
		_ = flag.Value.Set("")
		flag.Changed = false
	}
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
