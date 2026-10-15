package cmd

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
)

// gitlabCreds is what a request runs as: the GitLab host and the token, with
// where the token came from ("env" or "keyring") and, for a keyring token, the
// username `auth login` verified.
type gitlabCreds struct {
	Host, Token, Source, Account, Profile string
}

// envAccount is the account reported for a GITLAB_CLI_TOKEN token: nothing local
// names its user (CLI-035 forbids a network call in context), so the source stands
// in for it. `auth status` checks it with GitLab.
const envAccount = "env:GITLAB_CLI_TOKEN"

// resolveCredentials resolves the host (App.Setting: GITLAB_CLI_HOST, then the
// active profile, then https://gitlab.com) and the token: GITLAB_CLI_TOKEN, else the
// keyring entry of the active profile (App.Profile), used only when it was stored
// for that host. A settings file with no active profile (several profiles and no
// default_profile, as a person may write it) needs --profile.
func resolveCredentials(app *aicli.App) (gitlabCreds, error) {
	host, err := settingHost(app)
	if err != nil {
		return gitlabCreds{}, err
	}
	profile, _ := app.Profile()
	if tok, src, e := app.Setting(nil, "token"); e != nil {
		return gitlabCreds{}, e
	} else if strings.TrimSpace(tok) != "" && src == "env" {
		return gitlabCreds{Host: host, Token: strings.TrimSpace(tok), Source: "env", Account: envAccount, Profile: profile}, nil
	}
	if profile == "" && len(app.Profiles()) > 0 {
		return gitlabCreds{}, noActiveProfile("use")
	}
	stored, rerr := readCredentialFor(app, profile)
	switch {
	case errors.Is(rerr, errNoCredential):
		return gitlabCreds{}, noTokenError(host, profile, "")
	case rerr != nil:
		app.Debugf("keyring: %v", rerr)
		return gitlabCreds{}, noTokenError(host, profile, "the OS keyring could not be read")
	case !sameHost(stored.Host, host):
		e := aicli.NewError("E_AUTH", fmt.Sprintf("The token stored for profile `%s` belongs to another GitLab host, so it is not sent to this one.",
			profileLabel(profile)), map[string]any{
			"next_action": fmt.Sprintf("Ask the user to run `gitlab-cli auth login --host %s` in their terminal for this host, or to unset GITLAB_CLI_HOST if it overrides the profile.", host),
		})
		return gitlabCreds{}, e
	}
	return gitlabCreds{Host: host, Token: stored.Token, Source: "keyring", Account: stored.Username, Profile: profile}, nil
}

// noTokenError is E_AUTH for a host with no token. Agents never handle tokens
// (SEC-040): the next step is the user's.
func noTokenError(host, profile, why string) *aicli.Error {
	msg := fmt.Sprintf("No GitLab token is configured for profile `%s`.", profileLabel(profile))
	if why != "" {
		msg = fmt.Sprintf("No GitLab token is available: GITLAB_CLI_TOKEN is not set and %s.", why)
	}
	return aicli.NewError("E_AUTH", msg, map[string]any{
		"next_action": fmt.Sprintf("Ask the user to run `gitlab-cli auth login --host %s --token-file -` in their own terminal, or to set GITLAB_CLI_TOKEN; never handle the token yourself.", host),
	})
}

// settingHost is the host setting, checked: https, or http only for a loopback
// host (a token never crosses the network in clear), no user info, no query.
func settingHost(app *aicli.App) (string, error) {
	raw, source, e := app.Setting(nil, "host")
	if e != nil {
		return "", e
	}
	host, err := checkHost(raw)
	if err != nil {
		where := map[string]string{"env": "GITLAB_CLI_HOST", "profile": "the active profile's host", "default": "the default host"}[source]
		return "", aicli.NewError("E_CONFIG", fmt.Sprintf("The GitLab host from %s is not usable: %v.", where, err), map[string]any{
			"next_action": "Ask the user to set GITLAB_CLI_HOST, or the profile's host in settings.json, to the GitLab URL, like https://gitlab.example.com.",
		})
	}
	return host, nil
}

// checkHost validates and normalises a GitLab URL (no trailing slash).
func checkHost(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", errors.New("it is not a URL like https://gitlab.example.com")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("it must not carry user info, a query or a fragment")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopback(u.Hostname()) {
			return "", errors.New("http:// is allowed only for localhost; use https://")
		}
	default:
		return "", errors.New("it must start with https://")
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host+u.EscapedPath(), "/"), nil
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// sameHost compares two GitLab URLs after normalisation.
func sameHost(a, b string) bool {
	na, errA := checkHost(a)
	nb, errB := checkHost(b)
	return errA == nil && errB == nil && strings.EqualFold(na, nb)
}

// gitlabIdentity is App.Identity (CLI-035, CLI-054): the account and host a write
// acts as, from local state only. A token from GITLAB_CLI_TOKEN has no local
// account name, so it reports envAccount (aicli binds the token itself as the
// secret setting token, B1); a missing token reports no account. Under `auth login
// --profile new` (Meta.AllowUnknownProfile) App.Setting resolves without the profile, so the
// new profile acts as nobody yet.
func gitlabIdentity(app *aicli.App) (aicli.Identity, error) {
	raw, _, e := app.Setting(nil, "host")
	if e != nil {
		return aicli.Identity{}, e
	}
	host := strings.TrimRight(strings.TrimSpace(raw), "/")
	id := aicli.Identity{Host: host, Extra: map[string]string{"credentials_source": "none"}}
	if tok, src, _ := app.Setting(nil, "token"); strings.TrimSpace(tok) != "" && src == "env" {
		id.Account, id.Extra["credentials_source"] = envAccount, "env"
		return id, nil
	}
	profile, _ := app.Profile()
	if profile == "" && len(app.Profiles()) > 0 {
		return id, nil // no active profile, so no keyring entry applies
	}
	stored, err := readCredentialFor(app, profile)
	if err == nil && sameHost(stored.Host, host) {
		id.Account, id.Extra["credentials_source"] = stored.Username, "keyring"
		if id.Account == "" {
			id.Account = keyringAccount(profile)
		}
	}
	return id, nil
}
