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
}

type Envelope struct {
	OK            bool           `json:"ok"`
	SchemaVersion string         `json:"schema_version"`
	Data          any            `json:"data,omitempty"`
	Meta          Meta           `json:"meta,omitempty"`
	Error         *EnvelopeError `json:"error,omitempty"`
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
		Meta:          Meta{DurationMS: commandDurationMS()},
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
		Meta:          Meta{DurationMS: commandDurationMS()},
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
	default:
		return ""
	}
}

func RetryableErrorCode(code ErrorCode) bool {
	switch code {
	case ErrRateLimit, ErrServer, ErrNetwork, ErrTimeout:
		return true
	default:
		return false
	}
}

// PrintErrorJSON outputs a machine-readable error envelope as JSON to stderr.
func PrintErrorJSON(msg string, statusCode int) {
	code := ErrorCodeFromStatus(statusCode)
	if statusCode == 0 {
		code = ErrUnknown
	}
	emitErrorPayload(msg, statusCode, code)
}

// PrintErrorJSONWithCode outputs an error envelope with an explicit error code.
func PrintErrorJSONWithCode(msg string, statusCode int, code ErrorCode) {
	emitErrorPayload(msg, statusCode, code)
}

func emitErrorPayload(msg string, statusCode int, code ErrorCode) {
	payload := ErrorEnvelope(msg, statusCode, code)
	if hint := HintForErrorCode(code); hint != "" {
		payload.Error.Details["hint"] = hint
	}
	data, err := emitJSONMarshal(payload)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, `{"ok":false,"schema_version":%q,"meta":{"duration_ms":%d},"error":{"code":%q,"message":%q,"details":{},"retryable":false}}`+"\n", SchemaVersion, commandDurationMS(), code, msg)
		return
	}
	_, _ = fmt.Fprintln(os.Stderr, string(data))
}
