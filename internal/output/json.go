package output

import (
	"encoding/json"
	"fmt"
	"os"
)

const SchemaVersion = "1.0"

// Compact controls whether JSON is emitted without indentation (set by --compact).
var Compact bool

// DurationMS returns the current command duration for response metadata.
// The cmd package sets this hook; package-level tests and helper use default to 0.
var DurationMS func() int64

// UpdateNoticesProvider returns the cached update-available notices to attach to
// every response's meta.notices (CLI-SPEC §3, §14). The cmd package wires this
// hook to a read-only, TTL-bounded local cache reader; it must perform NO network
// I/O. When nil (e.g. in package-level tests) or when it returns an empty slice,
// meta.notices is omitted. This lives behind a func pointer so internal/output
// does not import cmd (which would create an import cycle).
var UpdateNoticesProvider func() []any

// cachedNotices invokes the provider hook if set, returning the cached notices to
// attach to meta. A nil hook or empty result yields nil so meta.notices is omitted.
func cachedNotices() []any {
	if UpdateNoticesProvider == nil {
		return nil
	}
	notices := UpdateNoticesProvider()
	if len(notices) == 0 {
		return nil
	}
	return notices
}

// marshalJSON encodes v according to the global Compact setting.
func marshalJSON(v any) ([]byte, error) {
	if Compact {
		return json.Marshal(v)
	}
	return json.MarshalIndent(v, "", "  ")
}

// emitJSONMarshal is the encoder for emitErrorPayload (overridable in tests).
var emitJSONMarshal = marshalJSON

type Meta struct {
	DurationMS int64 `json:"duration_ms"`
	// Notices carries the cached update-available notice (CLI-SPEC §3, §14),
	// read only from the local cache via UpdateNoticesProvider. omitempty: present
	// only when the cache currently holds an available-update notice.
	Notices []any `json:"notices,omitempty"`
}

type Envelope struct {
	OK            bool   `json:"ok"`
	SchemaVersion string `json:"schema_version"`
	Data          any    `json:"data,omitempty"`
	// Meta is always emitted (no omitempty): every response carries meta, and
	// duration_ms:0 is a valid value an agent should always see.
	Meta  Meta           `json:"meta"`
	Error *EnvelopeError `json:"error,omitempty"`
}

type EnvelopeError struct {
	Code      ErrorCode      `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details"`
	Retryable bool           `json:"retryable"`
}

func commandDurationMS() int64 {
	if DurationMS == nil {
		return 0
	}
	return DurationMS()
}

func SuccessEnvelope(v any) Envelope {
	return Envelope{
		OK:            true,
		SchemaVersion: SchemaVersion,
		Data:          v,
		Meta:          Meta{DurationMS: commandDurationMS(), Notices: cachedNotices()},
	}
}

func ErrorEnvelope(msg string, statusCode int, code ErrorCode) Envelope {
	details := map[string]any{}
	if statusCode != 0 {
		details["status_code"] = statusCode
	}
	return Envelope{
		OK:            false,
		SchemaVersion: SchemaVersion,
		Meta:          Meta{DurationMS: commandDurationMS(), Notices: cachedNotices()},
		Error: &EnvelopeError{
			Code:      code,
			Message:   msg,
			Details:   details,
			Retryable: RetryableErrorCode(code),
		},
	}
}

// PrintJSONErr outputs v as JSON to stdout. On marshal failure it writes
// to stderr and returns the error (callers may map this to exit code 2).
func PrintJSONErr(v any) error {
	data, err := marshalJSON(SuccessEnvelope(v))
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "json marshal error: %v\n", err)
		return err
	}
	fmt.Println(string(data))
	return nil
}

// PrintJSON outputs v as indented JSON to stdout. Marshal errors are written to
// stderr and terminate the process with exit code 2.
func PrintJSON(v any) {
	if err := PrintJSONErr(v); err != nil {
		os.Exit(2)
	}
}

// NDJSONLine is one line of a streaming NDJSON response (CLI-SPEC §5). Each line
// is an independent envelope carrying ok, schema_version, and a type tag; the
// final line of a stream uses type "summary".
type NDJSONLine struct {
	OK            bool   `json:"ok"`
	SchemaVersion string `json:"schema_version"`
	Type          string `json:"type"`
	Data          any    `json:"data,omitempty"`
}

// PrintNDJSON writes one NDJSON line (always compact, one object per line) to
// stdout. typ is the line type (e.g. "chunk", "summary").
func PrintNDJSON(typ string, data any) error {
	line := NDJSONLine{OK: true, SchemaVersion: SchemaVersion, Type: typ, Data: data}
	encoded, err := json.Marshal(line)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "json marshal error: %v\n", err)
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

// ErrorCode classifies errors for machine consumption.
type ErrorCode string

const (
	ErrConfig          ErrorCode = "E_CONFIG"
	ErrAuth            ErrorCode = "E_AUTH"
	ErrForbidden       ErrorCode = "E_FORBIDDEN"
	ErrNotFound        ErrorCode = "E_NOT_FOUND"
	ErrConflict        ErrorCode = "E_CONFLICT"
	ErrRateLimit       ErrorCode = "E_RATE_LIMITED"
	ErrServer          ErrorCode = "E_SERVER"
	ErrValidation      ErrorCode = "E_VALIDATION"
	ErrConfirmRequired ErrorCode = "E_CONFIRMATION_REQUIRED"
	ErrCancelled       ErrorCode = "E_CANCELLED"
	ErrTimeout         ErrorCode = "E_TIMEOUT"
	ErrNetwork         ErrorCode = "E_NETWORK"
	ErrIntegrity       ErrorCode = "E_INTEGRITY"
	ErrIO              ErrorCode = "E_IO"
	ErrInterrupted     ErrorCode = "E_INTERRUPTED"
	ErrUnknown         ErrorCode = "E_UNKNOWN"
)

// ErrorCodeFromStatus maps HTTP status codes to error codes.
func ErrorCodeFromStatus(statusCode int) ErrorCode {
	switch statusCode {
	case 401:
		return ErrAuth
	case 403:
		return ErrForbidden
	case 404:
		return ErrNotFound
	case 409:
		return ErrConflict
	case 429:
		return ErrRateLimit
	default:
		if statusCode >= 500 {
			return ErrServer
		}
		if statusCode >= 400 {
			return ErrValidation
		}
		return ErrUnknown
	}
}

// HintForErrorCode returns an actionable hint for the given error code.
func HintForErrorCode(code ErrorCode) string {
	switch code {
	case ErrConfig:
		return "Run 'gitlab-cli auth login' or set GITLAB_HOST and GITLAB_TOKEN environment variables"
	case ErrAuth:
		return "Check your PAT; create one at <host>/-/user_settings/personal_access_tokens with at least 'api' scope"
	case ErrForbidden:
		return "Check your PAT scope and project membership/permissions"
	case ErrNotFound:
		return "Verify the resource (project path, IID, ID) exists and you have permission to view it"
	case ErrConflict:
		return "Resource conflict; another change may have happened concurrently. Re-fetch and retry"
	case ErrRateLimit:
		return "Wait and retry; reduce request frequency"
	case ErrServer:
		return "GitLab server error; try again later"
	case ErrValidation:
		return "Check command arguments and flags"
	case ErrCancelled:
		return "Operation was cancelled or confirmation was not provided; use --confirm <confirm_token> for non-interactive runs"
	case ErrConfirmRequired:
		return "Run the same command with --dry-run, inspect the preview, then retry with --confirm <confirm_token>"
	case ErrTimeout:
		return "The operation timed out; retry with backoff"
	case ErrNetwork:
		return "Check host URL and network connectivity"
	case ErrIntegrity:
		return "Release integrity verification failed (signature or checksum); do not retry. Re-run update to fetch the current release, or report a possible supply-chain issue"
	case ErrIO:
		return "Local filesystem failure (disk space, file locked, or partial write); fix the environment, then re-run"
	case ErrInterrupted:
		return "Operation cancelled by signal; staged work left nothing half-applied. Re-run update, it is idempotent"
	default:
		return ""
	}
}

func RetryableErrorCode(code ErrorCode) bool {
	switch code {
	case ErrRateLimit, ErrServer, ErrNetwork, ErrTimeout, ErrInterrupted:
		return true
	default:
		return false
	}
}

// PrintErrorJSON outputs the machine-readable error envelope as the single
// JSON document on stdout (CLI-SPEC §4): agents always parse stdout.
func PrintErrorJSON(msg string, statusCode int) {
	code := ErrorCodeFromStatus(statusCode)
	if statusCode == 0 {
		code = ErrUnknown
	}
	emitErrorPayload(msg, statusCode, code, nil)
}

// PrintErrorJSONWithCode outputs an error envelope with an explicit error code.
func PrintErrorJSONWithCode(msg string, statusCode int, code ErrorCode) {
	emitErrorPayload(msg, statusCode, code, nil)
}

// PrintErrorJSONWithDetails outputs an error envelope with an explicit error
// code and extra structured details merged into error.details (e.g. the update
// stage invariant: stage, current_version, binary_replaced, skill_sync_status).
func PrintErrorJSONWithDetails(msg string, statusCode int, code ErrorCode, details map[string]any) {
	emitErrorPayload(msg, statusCode, code, details)
}

func emitErrorPayload(msg string, statusCode int, code ErrorCode, extra map[string]any) {
	payload := ErrorEnvelope(msg, statusCode, code)
	for k, v := range extra {
		payload.Error.Details[k] = v
	}
	if hint := HintForErrorCode(code); hint != "" {
		payload.Error.Details["hint"] = hint
	}
	data, err := emitJSONMarshal(payload)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stdout, `{"ok":false,"schema_version":%q,"meta":{"duration_ms":%d},"error":{"code":%q,"message":%q,"details":{},"retryable":false}}`+"\n", SchemaVersion, commandDurationMS(), code, msg)
		return
	}
	fmt.Println(string(data))
}
