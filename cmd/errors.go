package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// GitLab failures become contract errors here, by HTTP status (aicli.FromHTTP) and
// never by message text (CLI-016). Messages are written for the model (CLI-017):
// they name what failed and what to do, and GitLab's own text goes to
// details.upstream_message, which aicli marks untrusted.
//
//	readError(err, opts...)          for reads and for a write's Preview: nothing changed
//	classifyWriteError(err, opts...) for a write's Apply: what reached GitLab (CLI-015)
//
// Options: onNotFound(message, next) for a precise 404, requiresRole(role) for the
// role on the project a 403 lacks (details.required, CLI-014), requiresRoleOn(role,
// resource) when the role is on something else (a create in a group).

type errOpts struct {
	notFoundMessage, notFoundNext string
	required, requiredOn          string
}

// errOpt adjusts how a GitLab failure is reported.
type errOpt func(*errOpts)

// onNotFound replaces the generic 404 message and next step.
func onNotFound(message, next string) errOpt {
	return func(o *errOpts) { o.notFoundMessage, o.notFoundNext = message, next }
}

// requiresRole names the GitLab role on the project a 403 means the account lacks.
func requiresRole(role string) errOpt { return requiresRoleOn(role, "the project") }

// requiresRoleOn names the role and what it is held on ("the group" for a create in
// a group).
func requiresRoleOn(role, resource string) errOpt {
	return func(o *errOpts) { o.required, o.requiredOn = role, resource }
}

func collect(opts []errOpt) errOpts {
	var o errOpts
	for _, fn := range opts {
		fn(&o)
	}
	return o
}

const doctorHint = "`gitlab-cli doctor` checks the host, the token and the connection."

// The error codes a GitLab read or write can return besides the ones aicli adds
// itself (Meta.ErrorCodes, CLI-026). E_CONFIG: an invalid settings file, host or
// timeout; E_AUTH: no token or a rejected one; E_AMBIGUOUS: a name that matches
// several projects or users (projectFlag, CLI-048). Writes whose read-back can
// disagree with the request add E_VERIFY_FAILED.
var (
	gitlabReadCodes = []string{"E_AMBIGUOUS", "E_AUTH", "E_CONFIG", "E_FORBIDDEN", "E_INTERRUPTED", "E_NETWORK", "E_NOT_FOUND",
		"E_RATE_LIMITED", "E_SERVER", "E_TIMEOUT", "E_VALIDATION"}
	gitlabWriteCodes = append(append([]string{}, gitlabReadCodes...), "E_VERIFY_FAILED")
)

// readError maps a failed GitLab read (or a write's Preview) to a contract error;
// nothing was changed, so dispatch_state is not_started (CLI-011).
func readError(err error, opts ...errOpt) error {
	if err == nil {
		return nil
	}
	o := collect(opts)
	var ae *aicli.Error
	if errors.As(err, &ae) {
		return ae
	}
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		return httpError(apiErr, o, false)
	}
	var ne *api.NetworkError
	if errors.As(err, &ne) {
		switch {
		case errors.Is(err, context.Canceled):
			return aicli.NewError("E_INTERRUPTED", "The command was interrupted; this read changed nothing.", map[string]any{
				"next_action": "Run the command again if it is still needed."})
		case ne.Timeout:
			return aicli.NewError("E_TIMEOUT", "GitLab did not answer in time; this read changed nothing.", map[string]any{
				"next_action": "Retry; if it keeps timing out, ask for less (a smaller --limit) or check the network. " + doctorHint})
		}
		return networkError(ne, "GitLab could not be reached; this read changed nothing.")
	}
	return err
}

// classifyWriteError tells the gate what a failed Apply did (CLI-015): GitLab
// answered with a 4xx (429 included), so nothing changed (Rejected); no connection
// was made, so nothing was sent (NotDispatched); anything else (a 5xx, a timeout or
// a dropped connection after sending) stays unwrapped, so the gate reports
// E_OUTCOME_UNKNOWN and the operation can be looked up.
func classifyWriteError(err error, opts ...errOpt) error {
	if err == nil {
		return nil
	}
	o := collect(opts)
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode < 500 {
		return aicli.Rejected(httpError(apiErr, o, true))
	}
	var ne *api.NetworkError
	if errors.As(err, &ne) && !ne.Sent {
		if errors.Is(err, context.Canceled) {
			return aicli.NotDispatched(aicli.NewError("E_INTERRUPTED", "The command was interrupted before the change was sent; nothing was applied.",
				map[string]any{"next_action": "Run --dry-run again if the change is still needed."}))
		}
		return aicli.NotDispatched(networkError(ne, "GitLab could not be reached, so nothing was sent."))
	}
	return err
}

// networkError is E_NETWORK for a request that got no answer (FromHTTP status 0);
// a certificate failure names GITLAB_CLI_CA_FILE (CLI-110).
func networkError(ne *api.NetworkError, message string) *aicli.Error {
	if ne.TLS {
		e := aicli.FromHTTP(0, "GitLab's TLS certificate is not trusted, so nothing was sent.", ne.Err.Error(), 0)
		e.Details["next_action"] = "If GitLab uses a company CA, ask the user to point GITLAB_CLI_CA_FILE at its PEM file; never turn verification off."
		return e
	}
	e := aicli.FromHTTP(0, message, ne.Err.Error(), 0)
	e.Details["next_action"] = "Check the GitLab host setting, the VPN, HTTPS_PROXY and NO_PROXY, then retry. " + doctorHint
	return e
}

// httpError maps a GitLab HTTP error by status.
func httpError(apiErr *api.APIError, o errOpts, write bool) *aicli.Error {
	status := apiErr.StatusCode
	msg := readMessage(status)
	if write {
		msg = writeMessage(status)
	}
	if status == 404 && o.notFoundMessage != "" {
		msg = o.notFoundMessage
	}
	e := aicli.FromHTTP(status, msg, apiErr.Upstream(), apiErr.RetryAfter)
	switch status {
	case 401:
		e.Details["next_action"] = "Ask the user to sign in again (`gitlab-cli auth login` in their terminal) or to set GITLAB_CLI_TOKEN; `gitlab-cli auth status` checks the token."
	case 403:
		if o.required != "" {
			e.Details["required"] = o.required
			e.Details["next_action"] = fmt.Sprintf("Tell the user this needs at least the %s role on %s; do not retry with other credentials.", o.required, o.requiredOn)
		} else {
			e.Details["next_action"] = "Tell the user this account lacks the permission; do not retry with other credentials."
		}
	case 404:
		next := "Check the identifiers; list the resources to find the right one."
		if o.notFoundNext != "" {
			next = o.notFoundNext
		}
		e.Details["next_action"] = next
	case 429:
		e.Details["next_action"] = "Wait details.retry_after_ms (or a few seconds when absent), then run the command again."
	}
	if _, has := e.Details["next_action"]; !has && status < 500 {
		if write {
			e.Details["next_action"] = "GitLab refused the change and nothing was applied; fix what details.upstream_message names, then run --dry-run again."
		} else {
			e.Details["next_action"] = "Fix what details.upstream_message names, then run the command again."
		}
	}
	return e
}

func readMessage(status int) string {
	switch {
	case status == 401:
		return "GitLab rejected the token: it is missing, invalid, expired or revoked."
	case status == 403:
		return "GitLab refused this request for this account."
	case status == 404, status == 410:
		return "GitLab has no such resource, or this account cannot see it."
	case status == 409, status == 412:
		return "GitLab refused the request because of the resource's current state."
	case status == 429:
		return "GitLab's rate limit was hit; this read changed nothing."
	case status >= 500:
		return "GitLab failed while answering; this read changed nothing."
	}
	return "GitLab rejected the request's values; details.upstream_message says which."
}

func writeMessage(status int) string {
	switch status {
	case 401:
		return "GitLab rejected the token, so nothing was applied."
	case 403:
		return "GitLab refused the change for this account, so nothing was applied."
	case 404, 410:
		return "GitLab has no such resource, or this account cannot see it; nothing was applied."
	case 409, 412:
		return "GitLab refused the change because of the resource's current state; nothing was applied."
	case 429:
		return "GitLab's rate limit refused the change; nothing was applied."
	}
	return "GitLab rejected the change's values, so nothing was applied; details.upstream_message says which."
}
