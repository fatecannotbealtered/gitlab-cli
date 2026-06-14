package cmd

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

// This file is the shared batch contract (CLI-SPEC §15): every batch command
// presents one command, one envelope, one confirm token, and one aggregated
// result regardless of whether the work is a native upstream bulk endpoint
// (class A) or a client-side loop (class B). Keeping the plumbing here is what
// guarantees both classes look identical to the agent.

// batchItem is one entry of an aggregated batch result (CLI-SPEC §15.5). target
// is the natural key the caller passed in (id, iid, key, …), never an array
// index, so an agent can zip results back to inputs.
type batchItem struct {
	Target string          `json:"target"`
	OK     bool            `json:"ok"`
	Error  *batchItemError `json:"error,omitempty"`
	// Extra carries optional per-item detail (e.g. action, envScope) merged into
	// the emitted map. It is never part of the failure taxonomy.
	Extra map[string]any `json:"-"`
}

// batchItemError mirrors the top-level envelope error taxonomy (§6): the same
// {code, retryable} shape so an agent reads per-item failures exactly as it
// reads a whole-command failure.
type batchItemError struct {
	Code      output.ErrorCode `json:"code"`
	Retryable bool             `json:"retryable"`
}

// parsePluralFlag reads a repeatable + comma-separated string slice flag and
// returns it de-duplicated with input order preserved (CLI-SPEC §15.1). A single
// value degrades to a batch of one. The flag must be registered with
// StringSlice so both `--ids 1,2` and `--ids 1 --ids 2` are accepted.
func parsePluralFlag(cmd *cobra.Command, name string) []string {
	raw, _ := cmd.Flags().GetStringSlice(name)
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// parsePluralIntFlag is parsePluralFlag plus per-value integer parsing. It
// returns the validated ints in input order (de-duplicated). A non-numeric value
// is a usage error (§15.1: empty/invalid input is E_VALIDATION, not a no-op).
func parsePluralIntFlag(cmd *cobra.Command, name string) ([]int, error) {
	strs := parsePluralFlag(cmd, name)
	out := make([]int, 0, len(strs))
	for _, s := range strs {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, failArg("--" + name + " values must be integers, got " + strconv.Quote(s))
		}
		out = append(out, n)
	}
	return out, nil
}

// continueOnError reads the --continue-on-error flag. Default true (best-effort,
// finish the batch); dangerous batches register the flag with a false default.
func continueOnError(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("continue-on-error")
	return v
}

// markBatch registers the flags every batch write command shares:
//   - --continue-on-error (default true; pass false in defaultContinue for
//     dangerous batches that should stop at the first failure).
//
// It also marks the command as a write so audit logging and the format gate
// treat it like any other mutation.
func markBatch(cmd *cobra.Command, defaultContinue bool) {
	cmd.Flags().Bool("continue-on-error", defaultContinue, "Keep going after a per-item failure (already-applied items are not rolled back)")
	markWrite(cmd)
}

// prepareBatchWrite runs the §15 gate for a batch write: it enforces the
// --dangerous second gate, emits the batch dry-run preview (total + full target
// set + per-target changes + one confirm token), or consumes the single confirm
// token that authorizes the whole resolved batch.
//
// targets is the resolved, de-duplicated, order-preserved natural-key set. The
// token binds the whole set, so adding/removing a target invalidates it with
// E_CONFLICT (the confirm payload carries the full ordered list). Returns
// done=true when the dry-run preview was printed and the caller should stop.
func prepareBatchWrite(cmd *cobra.Command, action string, targets []string, extra map[string]any) (bool, error) {
	if len(targets) == 0 {
		return false, failArg("no targets: provide at least one via the plural flag")
	}
	if isDangerousCommand(cmd) && !dangerousMode {
		return false, failConfirmRequired(cmd.CommandPath() + " is write-dangerous and requires --dangerous in both the --dry-run and --confirm steps")
	}

	// The confirm payload binds the whole resolved target set plus any extra
	// scope (project, action-specific fields). Order matters: a different order
	// is a different batch and must invalidate the token.
	payload := map[string]any{"action": action, "targets": targets}
	for k, v := range extra {
		payload[k] = v
	}

	if dryRun {
		emitBatchPreview(cmd, action, targets, extra)
		return true, nil
	}
	if err := requireConfirm(cmd, action, payload); err != nil {
		return false, err
	}
	return false, nil
}

// emitBatchPreview prints the §15.2 dry-run summary: what will happen to N
// objects, the full target list, and one confirm token covering the batch.
func emitBatchPreview(cmd *cobra.Command, action string, targets []string, extra map[string]any) {
	if !jsonMode {
		output.Info("[dry-run] " + action + " (" + strconv.Itoa(len(targets)) + " targets)")
		return
	}
	changes := make([]map[string]any, 0, len(targets))
	for _, tgt := range targets {
		changes = append(changes, map[string]any{"action": action, "target": tgt})
	}
	preview := map[string]any{
		"action":  action,
		"total":   len(targets),
		"targets": targets,
		"changes": changes,
	}
	for k, v := range extra {
		preview[k] = v
	}
	payload := map[string]any{"action": action, "targets": targets}
	for k, v := range extra {
		payload[k] = v
	}
	token, expires := newConfirmToken(action, payload)
	output.PrintJSON(map[string]any{
		"preview":       preview,
		"confirm_token": token,
		"expires_at":    expires.Format(time.RFC3339),
	})
}

// emitBatchResult prints the §15.5 aggregated result: per-item items[] plus a
// {total, succeeded, failed} summary. Top-level ok is always true here — the
// batch executed and produced a result; per-item status lives in items[].
// unattempted is the tail that --continue-on-error=false skipped, reported so an
// agent can resume.
func emitBatchResult(items []batchItem, unattempted []string) {
	succeeded, failed := 0, 0
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		entry := map[string]any{"target": it.Target, "ok": it.OK}
		for k, v := range it.Extra {
			entry[k] = v
		}
		if it.OK {
			succeeded++
		} else {
			failed++
			if it.Error != nil {
				entry["error"] = map[string]any{
					"code":      it.Error.Code,
					"retryable": it.Error.Retryable,
				}
			}
		}
		out = append(out, entry)
	}
	summary := map[string]any{
		"total":     len(items),
		"succeeded": succeeded,
		"failed":    failed,
	}
	result := map[string]any{"items": out, "summary": summary}
	if len(unattempted) > 0 {
		result["skipped"] = unattempted
	}
	if !jsonMode {
		output.Success("Batch done: " + strconv.Itoa(succeeded) + " succeeded, " + strconv.Itoa(failed) + " failed")
		return
	}
	output.PrintJSON(result)
}

// batchErrorFor maps an API error to the per-item {code, retryable} taxonomy.
// It mirrors handleAPIError's classification without emitting the top-level
// envelope, since the failure belongs inside items[]. A non-API error (e.g. a
// network failure with no HTTP status) maps to E_NETWORK.
func batchErrorFor(err error) *batchItemError {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		code := output.ErrorCodeFromStatus(apiErr.StatusCode)
		return &batchItemError{Code: code, Retryable: output.RetryableErrorCode(code)}
	}
	return &batchItemError{Code: output.ErrNetwork, Retryable: output.RetryableErrorCode(output.ErrNetwork)}
}
