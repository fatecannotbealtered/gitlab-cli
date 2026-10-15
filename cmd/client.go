package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// The GitLab client (CLI-110): aicli's transport (TLS 1.2+, always verified, the
// system roots plus GITLAB_CLI_CA_FILE, dial and handshake timeouts) and User-Agent,
// a per-request timeout of at most 30 s (GITLAB_CLI_TIMEOUT lowers it), reads that
// retry at most twice and writes sent once (internal/api). Nothing turns TLS
// verification off.

// newClient builds the client of this run from the App's settings and credentials.
// Read commands call it in RunE. Write commands use gitlabFor(app) instead, so that
// nothing is read or resolved before the gate has bound the request.
func newClient(app *aicli.App) (*api.Client, error) {
	cred, err := resolveCredentials(app)
	if err != nil {
		return nil, err
	}
	return clientFor(app, cred.Host, cred.Token)
}

// clientFor builds a client for an explicit host and token (auth login checks a
// token before it is stored).
func clientFor(app *aicli.App, host, token string) (*api.Client, error) {
	tr, e := app.HTTPTransport() // proxy, CA file and TLS from App.Env (CLI-110, Q2)
	if e != nil {
		return nil, e
	}
	timeout, err := requestTimeout(app)
	if err != nil {
		return nil, err
	}
	return api.NewClient(&api.Config{Host: host, Token: token, UserAgent: app.UserAgent(), Transport: tr,
		Timeout: timeout, Debugf: app.Debugf}), nil
}

// lazyClient builds the client on first use and keeps it for the run. A write's
// Preview and Apply call get(); the first call happens inside the gate.
type lazyClient struct {
	app  *aicli.App
	c    *api.Client
	err  error
	done bool
}

// gitlabFor returns the run's lazy client.
func gitlabFor(app *aicli.App) *lazyClient { return &lazyClient{app: app} }

func (l *lazyClient) get() (*api.Client, error) {
	if !l.done {
		l.c, l.err = newClient(l.app)
		l.done = true
	}
	return l.c, l.err
}

// requestTimeout is the timeout setting: a Go duration above 0 and at most 30 s.
func requestTimeout(app *aicli.App) (time.Duration, error) {
	raw, source, e := app.Setting(nil, "timeout")
	if e != nil {
		return 0, e
	}
	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || d <= 0 || d > api.DefaultTimeout {
		where := map[string]string{"env": "GITLAB_CLI_TIMEOUT", "profile": "the active profile's timeout", "default": "the default timeout"}[source]
		return 0, aicli.NewError("E_CONFIG", fmt.Sprintf("The request timeout from %s is not a duration between 1ms and 30s.", where), map[string]any{
			"next_action": "Ask the user to set GITLAB_CLI_TIMEOUT to a duration such as 10s, at most 30s, or to unset it.",
		})
	}
	return d, nil
}
