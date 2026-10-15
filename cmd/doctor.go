package cmd

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
	"github.com/fatecannotbealtered/gitlab-cli/internal/gitctx"
)

// doctorTimeout bounds doctor's one request to GitLab.
const doctorTimeout = 5 * time.Second

// gitlabDoctorChecks is App.DoctorChecks (CLI-036): it replaces aicli's
// credentials and connectivity checks with a real one, GET /api/v4/user with the
// configured token. A network failure fails only connectivity; a certificate
// failure names GITLAB_CLI_CA_FILE (CLI-110).
func gitlabDoctorChecks(ctx context.Context, app *aicli.App) []aicli.Check {
	cred, cerr := resolveCredentials(app)
	var ae *aicli.Error
	if cerr != nil && errors.As(cerr, &ae) && ae.Code == "E_CONFIG" {
		return []aicli.Check{{Check: "connectivity", Status: "warn", Message: "Not checked: " + ae.Message,
			Fix: "Fix the configuration the settings check names, then run `gitlab-cli doctor` again."}}
	}
	host := cred.Host
	if cerr != nil {
		host, _ = settingHost(app)
	}
	c, err := clientFor(app, host, cred.Token)
	if err != nil {
		msg := "The GitLab client cannot be built."
		if errors.As(err, &ae) {
			msg = ae.Message
		}
		return []aicli.Check{{Check: "connectivity", Status: "fail", Message: msg, Fix: "Fix the configuration it names, then run `gitlab-cli doctor` again."}}
	}
	cctx, cancel := context.WithTimeout(ctx, doctorTimeout)
	defer cancel()
	start := time.Now()
	var checks []aicli.Check
	if cerr != nil {
		// No token: say so, and still check that GitLab answers (GET /version
		// answers 401 without one, which proves the connection).
		checks = append(checks, aicli.Check{Check: "credentials", Status: "fail", Message: errMessage(cerr),
			Fix: fmt.Sprintf("Ask the user to run `gitlab-cli auth login --host %s --token-file -` in their terminal, or to set GITLAB_CLI_TOKEN.", host)})
		_, err = c.Get(cctx, c.APIPath("/version"))
		var apiErr *api.APIError
		if err == nil || errors.As(err, &apiErr) {
			return append(checks, connectivityPass(host, start))
		}
		return append(checks, connectivityFail(host, err))
	}
	me, err := c.Users.Me(cctx)
	var apiErr *api.APIError
	switch {
	case err == nil:
		checks = append(checks, aicli.Check{Check: "credentials", Status: "pass",
			Message: fmt.Sprintf("The token from the %s signs in as %s.", sourceName(cred.Source), me.Username)}, connectivityPass(host, start))
	case errors.As(err, &apiErr) && apiErr.StatusCode == 401:
		checks = append(checks, aicli.Check{Check: "credentials", Status: "fail",
			Message: fmt.Sprintf("GitLab rejected the token from the %s: it is invalid, expired or revoked.", sourceName(cred.Source)),
			Fix:     fmt.Sprintf("Ask the user to create a new token and run `gitlab-cli auth login --host %s --token-file -`, or to update GITLAB_CLI_TOKEN.", host)},
			connectivityPass(host, start))
	case errors.As(err, &apiErr):
		checks = append(checks, aicli.Check{Check: "connectivity", Status: "warn",
			Message: fmt.Sprintf("%s answered GET /user with HTTP %d.", host, apiErr.StatusCode),
			Fix:     "Run `gitlab-cli auth status` for the details, then `gitlab-cli doctor` again."})
	default:
		checks = append(checks, connectivityFail(host, err))
	}
	return checks
}

func sourceName(source string) string {
	if source == "env" {
		return "GITLAB_CLI_TOKEN variable"
	}
	return "OS keyring"
}

func connectivityPass(host string, start time.Time) aicli.Check {
	return aicli.Check{Check: "connectivity", Status: "pass",
		Message: fmt.Sprintf("%s answered in %d ms.", host, time.Since(start).Milliseconds())}
}

func connectivityFail(host string, err error) aicli.Check {
	var ne *api.NetworkError
	switch {
	case errors.As(err, &ne) && ne.TLS:
		return aicli.Check{Check: "connectivity", Status: "fail",
			Message: fmt.Sprintf("The TLS certificate of %s is not trusted.", host),
			Fix:     "If GitLab uses a company CA, point GITLAB_CLI_CA_FILE at its PEM file, then run `gitlab-cli doctor` again."}
	case errors.As(err, &ne) && ne.Timeout:
		return aicli.Check{Check: "connectivity", Status: "fail",
			Message: fmt.Sprintf("%s did not answer within %d s.", host, int(doctorTimeout.Seconds())),
			Fix:     "Check the VPN, HTTPS_PROXY and NO_PROXY, then run `gitlab-cli doctor` again."}
	}
	return aicli.Check{Check: "connectivity", Status: "fail",
		Message: fmt.Sprintf("%s could not be reached.", host),
		Fix:     "Check the host setting, the VPN, HTTPS_PROXY and NO_PROXY, then run `gitlab-cli doctor` again."}
}

func errMessage(err error) string {
	var ae *aicli.Error
	if errors.As(err, &ae) {
		return ae.Message
	}
	return "The credentials cannot be resolved."
}

var (
	safeHost    = regexp.MustCompile(`^[A-Za-z0-9.-]+(:[0-9]+)?$`)
	safeProject = regexp.MustCompile(`^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)+$`)
)

// gitlabContextExtra is App.ContextExtra (CLI-035): the GitLab remote of the
// current directory's git repository, from `git remote -v` (local, no network), so
// an agent working in a checkout learns its --project. Only a host and a path that
// look like ones are reported; anything else in a remote URL (user info, a token)
// never is.
func gitlabContextExtra() map[string]any {
	r, err := gitctx.DetectGitLabRemote("")
	if err != nil || r == nil || !safeHost.MatchString(r.Host) || !safeProject.MatchString(r.ProjectPath) {
		return nil
	}
	return map[string]any{"git_remote": map[string]any{"host": r.Host, "project": r.ProjectPath}}
}
