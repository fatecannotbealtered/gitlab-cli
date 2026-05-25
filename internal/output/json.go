package output

import (
	"encoding/json"
	"fmt"
	"os"
)

// Compact controls whether JSON is emitted without indentation (set by --compact).
var Compact bool

// marshalJSON encodes v according to the global Compact setting.
func marshalJSON(v any) ([]byte, error) {
	if Compact {
		return json.Marshal(v)
	}
	return json.MarshalIndent(v, "", "  ")
}

// emitJSONMarshal is the encoder for emitErrorPayload (overridable in tests).
var emitJSONMarshal = marshalJSON

// PrintJSONErr outputs v as JSON to stdout. On marshal failure it writes
// to stderr and returns the error (callers may map this to exit code 2).
func PrintJSONErr(v any) error {
	data, err := marshalJSON(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "json marshal error: %v\n", err)
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
	ErrConfig     ErrorCode = "CONFIG_ERROR"
	ErrAuth       ErrorCode = "AUTH_REQUIRED"
	ErrForbidden  ErrorCode = "FORBIDDEN"
	ErrNotFound   ErrorCode = "NOT_FOUND"
	ErrConflict   ErrorCode = "CONFLICT"
	ErrRateLimit  ErrorCode = "RATE_LIMITED"
	ErrServer     ErrorCode = "SERVER_ERROR"
	ErrValidation ErrorCode = "VALIDATION_ERROR"
	ErrCancelled  ErrorCode = "CANCELLED"
	ErrNetwork    ErrorCode = "NETWORK_ERROR"
	ErrUnknown    ErrorCode = "UNKNOWN_ERROR"
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
		return "Operation was cancelled or confirmation was not provided; use --confirm <token> for non-interactive runs"
	case ErrNetwork:
		return "Check host URL and network connectivity"
	default:
		return ""
	}
}

type errorPayload struct {
	Error      string    `json:"error"`
	StatusCode int       `json:"statusCode"`
	ErrorCode  ErrorCode `json:"errorCode"`
	Hint       string    `json:"hint,omitempty"`
}

// PrintErrorJSON outputs an error message as JSON to stderr.
func PrintErrorJSON(msg string, statusCode int) {
	code := ErrorCodeFromStatus(statusCode)
	if statusCode == 0 {
		code = ErrUnknown
	}
	emitErrorPayload(msg, statusCode, code)
}

// PrintErrorJSONWithCode outputs an error with an explicit error code.
func PrintErrorJSONWithCode(msg string, statusCode int, code ErrorCode) {
	emitErrorPayload(msg, statusCode, code)
}

func emitErrorPayload(msg string, statusCode int, code ErrorCode) {
	payload := errorPayload{
		Error:      msg,
		StatusCode: statusCode,
		ErrorCode:  code,
		Hint:       HintForErrorCode(code),
	}
	data, err := emitJSONMarshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, `{"error": %q, "statusCode": %d, "errorCode": %q}`+"\n", msg, statusCode, code)
		return
	}
	fmt.Fprintln(os.Stderr, string(data))
}
