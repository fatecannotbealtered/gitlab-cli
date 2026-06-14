package cmd

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
	"github.com/fatecannotbealtered/gitlab-cli/internal/output"
	"github.com/spf13/cobra"
)

var confirmFlag string
var confirmNow = time.Now

const confirmTokenTTL = 15 * time.Minute

func initConfirmFlag() {
	rootCmd.PersistentFlags().StringVar(&confirmFlag, "confirm", "", "Non-interactive confirmation: value must match the expected token for this action")
}

func newConfirmToken(action string, payload any) (string, time.Time) {
	expires := confirmNow().UTC().Add(confirmTokenTTL).Truncate(time.Second)
	return buildConfirmToken(action, payload, expires), expires
}

func buildConfirmToken(action string, payload any, expires time.Time) string {
	seed := canonicalConfirmSeed(action, payload, expires.Unix())
	sum := confirmDigest32(seed)
	return "ct_" + strconv.FormatInt(expires.Unix(), 10) + "_" + hex.EncodeToString(sum[:8])
}

func canonicalConfirmSeed(action string, payload any, expiresUnix int64) []byte {
	body, err := json.Marshal(map[string]any{
		"action":       action,
		"command_path": confirmCommandPath(action),
		"account":      confirmAccountContext(),
		"payload":      normalizeConfirmPayload(payload),
		"expires_unix": expiresUnix,
	})
	if err != nil {
		body = []byte(fmt.Sprintf("%s\n%d\n%v", action, expiresUnix, payload))
	}
	return body
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
	token := confirmFlag
	if err := validateConfirmToken(token, action, payload); err != nil {
		return err
	}
	// Single-use: a confirm token may drive exactly one write. A replay (e.g. an
	// agent retrying a confirmed write that timed out) is rejected so it cannot
	// duplicate the operation; the agent must re-run --dry-run to see current state.
	now := confirmNow().UTC()
	if isConfirmTokenConsumed(token, now) {
		return failWithCode("confirm token already used; the operation may have completed — re-run --dry-run to see current state", ExitConflict, output.ErrConflict)
	}
	markConfirmTokenConsumed(token, confirmTokenExpiryUnix(token), now)
	return nil
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

func confirmCommandPath(action string) string {
	mapped := commandPathForConfirmAction(action)
	if activeCmd != nil {
		path := activeCmd.CommandPath()
		if mapped == "" || path == mapped {
			return path
		}
	}
	return mapped
}

func commandPathForConfirmAction(action string) string {
	switch action {
	case "save credentials":
		return "gitlab-cli auth login"
	case "delete credentials":
		return "gitlab-cli auth logout"
	case "use profile":
		return "gitlab-cli auth profile use"
	case "remove profile":
		return "gitlab-cli auth profile remove"
	case "create issue":
		return "gitlab-cli issue create"
	case "update issue":
		return "gitlab-cli issue update"
	case "reopen issue":
		return "gitlab-cli issue reopen"
	case "assign issue":
		return "gitlab-cli issue assign"
	case "label issue":
		return "gitlab-cli issue label"
	case "issue bulk close":
		return "gitlab-cli issue bulk close"
	case "issue bulk reopen":
		return "gitlab-cli issue bulk reopen"
	case "issue bulk update":
		return "gitlab-cli issue bulk update"
	case "issue bulk label":
		return "gitlab-cli issue bulk label"
	case "issue bulk assign":
		return "gitlab-cli issue bulk assign"
	case "issue bulk comment":
		return "gitlab-cli issue bulk comment"
	case "add comment":
		return "gitlab-cli issue comment add"
	case "delete comment":
		return "gitlab-cli issue comment delete"
	case "retry job":
		return "gitlab-cli job retry"
	case "cancel job":
		return "gitlab-cli job cancel"
	case "create label":
		return "gitlab-cli label create"
	case "update label":
		return "gitlab-cli label update"
	case "delete mr comment":
		return "gitlab-cli mr comment delete"
	case "close issue":
		return "gitlab-cli issue close"
	case "delete label":
		return "gitlab-cli label delete"
	case "create milestone":
		return "gitlab-cli milestone create"
	case "update milestone":
		return "gitlab-cli milestone update"
	case "close milestone":
		return "gitlab-cli milestone close"
	case "create mr":
		return "gitlab-cli mr create"
	case "update mr":
		return "gitlab-cli mr update"
	case "close mr":
		return "gitlab-cli mr close"
	case "reopen mr":
		return "gitlab-cli mr reopen"
	case "approve mr":
		return "gitlab-cli mr approve"
	case "unapprove mr":
		return "gitlab-cli mr unapprove"
	case "merge mr":
		return "gitlab-cli mr merge"
	case "mr bulk merge":
		return "gitlab-cli mr bulk merge"
	case "mr bulk approve":
		return "gitlab-cli mr bulk approve"
	case "mr bulk close":
		return "gitlab-cli mr bulk close"
	case "mr bulk update":
		return "gitlab-cli mr bulk update"
	case "add mr comment":
		return "gitlab-cli mr comment add"
	case "create pipeline":
		return "gitlab-cli pipeline create"
	case "retry pipeline":
		return "gitlab-cli pipeline retry"
	case "cancel pipeline":
		return "gitlab-cli pipeline cancel"
	case "release create":
		return "gitlab-cli release create"
	case "release update":
		return "gitlab-cli release update"
	case "release delete":
		return "gitlab-cli release delete"
	case "repo file create":
		return "gitlab-cli repo file create"
	case "repo file update":
		return "gitlab-cli repo file update"
	case "repo file delete":
		return "gitlab-cli repo file delete"
	case "repo commit create":
		return "gitlab-cli repo commit create"
	case "repo branch create":
		return "gitlab-cli repo branch create"
	case "repo branch delete":
		return "gitlab-cli repo branch delete"
	case "update gitlab-cli":
		return "gitlab-cli update"
	case "create variable":
		return "gitlab-cli variable create"
	case "update variable":
		return "gitlab-cli variable update"
	case "delete variable":
		return "gitlab-cli variable delete"
	case "variable bulk-import":
		return "gitlab-cli variable bulk-import"
	default:
		return ""
	}
}

func confirmAccountContext() map[string]any {
	ctx := map[string]any{}
	if cfg, err := config.Load(); err == nil && cfg != nil {
		ctx["host"] = cfg.Host
		ctx["configured"] = strings.TrimSpace(cfg.Token) != ""
	}
	if source, err := authStatusSourceHook(); err == nil {
		ctx["source"] = source
	}
	return ctx
}

func normalizeConfirmPayload(v any) any {
	return normalizeConfirmValue("", v)
}

func normalizeConfirmValue(key string, v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = normalizeConfirmValue(k, val)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(x))
		for i, item := range x {
			out[i] = normalizeConfirmValue(key, item).(map[string]any)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = normalizeConfirmValue(key, item)
		}
		return out
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		if isIdentifierPayloadKey(key) {
			return output.ID(x)
		}
		return v
	default:
		rv := reflect.ValueOf(v)
		if !rv.IsValid() {
			return v
		}
		if isIdentifierPayloadKey(key) {
			switch rv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				return output.ID(v)
			}
		}
		return v
	}
}

func isIdentifierPayloadKey(key string) bool {
	k := strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
	return k == "id" || k == "iid" || strings.HasSuffix(k, "id")
}
