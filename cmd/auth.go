package cmd

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
	"github.com/fatecannotbealtered/gitlab-cli/internal/api"
)

// The auth group (CLI-101, SEC-040): `auth login` stores a token in the OS keyring
// and the profile's host in settings.json; `auth status` checks the token with
// GitLab; `auth logout` removes the token from the keyring. Login and logout change
// the tool's own credentials, so they declare configure: dangerous by default and
// gated like any write (CLI-025). A person sets credentials up; agents never handle
// a token (SEC-040), so the documented input is --token-file - (stdin) or
// GITLAB_CLI_TOKEN, and --token is for one-off use.

const authEvidence = "simulated:cmd/auth_test.go"

func registerAuth(app *aicli.App) {
	group := addGroup(app, "auth", "Sign in to GitLab with a token, check it, and sign out")
	app.Schema("auth_login", aicli.SchemaDef{Shape: "object",
		Fields: []string{"profile", "host", "account", "credentials_source", "settings_file"}, Untrusted: []string{"account"}})
	app.Schema("auth_status", aicli.SchemaDef{Shape: "object",
		Fields:    []string{"profile", "host", "account", "name", "user_id", "credentials_source", "token_name", "scopes", "expires_at"},
		Untrusted: []string{"account", "name", "token_name"}})
	app.Schema("auth_logout", aicli.SchemaDef{Shape: "object", Fields: []string{"profile", "host", "token_stored"}})
	addAuthLogin(app, group)
	addAuthStatus(app, group)
	addAuthLogout(app, group)
}

// profileTarget is the scope key of a profile's credentials (SEC-012).
func profileTarget(profile string) string { return "profiles/" + profileLabel(profile) }

// profileNamePattern is CLI-101's profile name. auth checks a requested name before
// the gate, so a bad --profile fails at once rather than in Apply
// (App.SetProfileSettings checks it again).
var profileNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// authProfile is the profile `auth login` and `auth logout` act on: App.Profile,
// which for these Meta.AllowUnknownProfile commands is also a profile --profile or
// GITLAB_CLI_PROFILE names that settings.json lacks (login creates it, logout removes
// a token left behind), else "default" when there is no settings file. With a
// settings file and no active profile (several profiles, no default_profile:
// App.Profiles lists them), the profile must be named.
func authProfile(app *aicli.App, verb string) (string, error) {
	name, source := app.Profile()
	if name != "" {
		if (source == "flag" || source == "env") && !profileNamePattern.MatchString(name) {
			param := map[string]string{"flag": "--profile", "env": "GITLAB_CLI_PROFILE"}[source]
			e := aicli.Usage("invalid_value", param, "A profile name uses a-z, 0-9, _ and -, up to 64 characters.")
			e.Details["expected"] = "^[a-z0-9][a-z0-9_-]{0,63}$"
			return "", e
		}
		return name, nil
	}
	// No profile: either no settings file, or one aicli refuses (reported here), or
	// one with no active profile.
	if _, _, e := app.Setting(nil, "host"); e != nil {
		return "", e
	}
	if len(app.Profiles()) > 0 {
		return "", noActiveProfile(verb)
	}
	return "default", nil
}

// noActiveProfile is E_USAGE for a settings file with profiles but no active one:
// several profiles and no default_profile. auth login makes its profile the default
// only when it writes the file's first profile (App.SetProfileSettings, Q3, RC2-1),
// so this is a file a person wrote that way, and a later login keeps it so.
func noActiveProfile(verb string) *aicli.Error {
	e := aicli.Usage("missing_parameter", "--profile", fmt.Sprintf("No profile is active (settings.json has several profiles and no default_profile), so name the profile to %s.", verb))
	e.Details["next_action"] = "Add --profile <name> (`gitlab-cli context` lists the profiles), or ask the user to set default_profile in settings.json."
	return e
}

func addAuthLogin(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "login", RunE: func(cmd *cobra.Command, _ []string) error {
		profile, err := authProfile(app, "sign in to")
		if err != nil {
			return err
		}
		host, storedHost, err := loginHost(app, cmd)
		if err != nil {
			return err
		}
		token, err := loginToken(app, cmd)
		if err != nil {
			return err
		}
		var me *api.User // the account the token belongs to, checked by Preview
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: profileTarget(profile), Kind: "profile", Label: profile},
			ReadBack: "auth status --profile " + profile,
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				client, err := clientFor(app, host, token)
				if err != nil {
					return aicli.Preview{}, err
				}
				if me, err = client.Users.Me(ctx); err != nil {
					return aicli.Preview{}, readError(err)
				}
				var prevHost, prevAccount any
				if storedHost != "" {
					prevHost = storedHost
				}
				if prev, err := readCredentialFor(app, profile); err == nil {
					prevAccount = prev.Username
				}
				return aicli.Preview{Action: fmt.Sprintf("Store a GitLab token for profile %s (%s)", profile, host), CanApply: true,
					Changes: []aicli.FieldChange{
						{Field: "account", Before: prevAccount, After: me.Username},
						{Field: "host", Before: prevHost, After: host},
						{Field: "token", Note: fmt.Sprintf("stored in the OS keyring as %s / %s, never in settings.json", keyringService, keyringAccount(profile))},
					}, Untrusted: []string{"changes[].before", "changes[].after"}}, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				// The gate ran Preview at this confirm, so me is the token's account now.
				if me == nil {
					return aicli.Result{}, aicli.NotDispatched(errors.New("the token was not checked"))
				}
				if err := storeLogin(app, profile, host, !sameHost(storedHost, host), storedCredential{Host: host, Token: token,
					Username: me.Username, UserID: idString(me.ID), SavedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
					return aicli.Result{}, err
				}
				// Read back the keyring entry: the account and the host the token is now
				// stored for (settings.json is written by aicli, which reports failure).
				obs := map[string]any{}
				if stored, err := readCredentialFor(app, profile); err == nil {
					obs["account"], obs["host"] = stored.Username, stored.Host
				}
				return aicli.Result{Observed: obs, Data: map[string]any{"profile": profile, "host": host, "account": me.Username,
					"credentials_source": "keyring", "settings_file": app.SettingsPath()}, Untrusted: []string{"account"}}, nil
			},
		})
	}}
	c.Flags().String("host", "", "GitLab URL, like https://gitlab.example.com (default: the profile's host)")
	c.Flags().String("token-file", "", "Read the token from this file, or - for stdin (the documented way)")
	c.Flags().String("token", "", "The token itself, for one-off use only: it shows in process lists; prefer --token-file - or GITLAB_CLI_TOKEN")
	c.MarkFlagsMutuallyExclusive("token", "token-file")
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Store a GitLab token in the OS keyring for a profile, after checking it with GitLab",
		Effects: []aicli.Effect{aicli.Configure}, Output: "auth_login", AllowUnknownProfile: true,
		Secrets: []string{"token"}, FileInputs: map[string]string{"token-file": "token"},
		ErrorCodes: []string{"E_AUTH", "E_CONFIG", "E_FORBIDDEN", "E_IO", "E_NETWORK", "E_RATE_LIMITED", "E_SERVER", "E_TIMEOUT", "E_VALIDATION"},
		Maturity:   aicli.Verified, Evidence: []string{authEvidence, liveEvidence("auth")},
		Examples: []string{"gitlab-cli auth login --host https://gitlab.example.com --token-file - --dangerous --dry-run",
			"gitlab-cli auth login --host https://gitlab.example.com --token-file - --dangerous --confirm <confirm_token>"}})
}

// loginHost is the host `auth login` signs in to, and the host the profile has in
// settings.json now ("" when it has none, or when GITLAB_CLI_HOST hides it): --host,
// else the host the profile resolves to (GITLAB_CLI_HOST, then its settings; for a
// new profile App.Setting skips the profile, Meta.AllowUnknownProfile). Once a settings file
// exists, a profile without a host of its own needs --host rather than the default
// https://gitlab.com.
func loginHost(app *aicli.App, cmd *cobra.Command) (host, stored string, err error) {
	raw, source, e := app.Setting(nil, "host")
	if e != nil {
		return "", "", e
	}
	if source == "profile" {
		stored = strings.TrimSpace(raw)
	}
	if cmd.Flags().Changed("host") {
		flag, _ := cmd.Flags().GetString("host")
		host, err := checkHost(flag)
		if err != nil {
			return "", "", aicli.Validation("--host", "an https URL such as https://gitlab.example.com (http only for localhost)",
				fmt.Sprintf("--host is not usable: %v.", err))
		}
		return host, stored, nil
	}
	if source == "default" && len(app.Profiles()) > 0 {
		e := aicli.Usage("missing_parameter", "--host", "The profile has no host yet, so it needs --host.")
		e.Details["next_action"] = "Add --host <GitLab URL>, like --host https://gitlab.example.com."
		return "", "", e
	}
	host, err = settingHost(app)
	return host, stored, err
}

// loginToken is the token to store: --token-file (stdin with -), else --token,
// else GITLAB_CLI_TOKEN, which is then bound like the flag (CLI-054), as a keyed
// digest because it is secret.
func loginToken(app *aicli.App, cmd *cobra.Command) (string, error) {
	var token string
	b, err := app.ReadFileInput(cmd, "token-file")
	switch {
	case err != nil:
		return "", err
	case b != nil:
		token = string(b)
	case cmd.Flags().Changed("token"):
		token, _ = cmd.Flags().GetString("token")
	default:
		env := app.Env("GITLAB_CLI_TOKEN")
		if strings.TrimSpace(env) == "" {
			e := aicli.Usage("missing_parameter", "--token-file", "`auth login` needs the token: --token-file - reads it from stdin.")
			e.Details["next_action"] = "Ask the user to run `gitlab-cli auth login --token-file -` in their own terminal and paste the token there; never handle the token yourself."
			return "", e
		}
		if err := aicli.SetResolved(cmd, "token", env); err != nil {
			return "", err
		}
		token = env
	}
	if token = strings.TrimSpace(token); token == "" {
		return "", aicli.Validation("--token", "a GitLab personal access token", "The token is empty.")
	}
	return token, nil
}

// storeLogin stores the token in the keyring, then, when writeHost, the profile's
// host in settings.json through App.SetProfileSettings (which also makes the profile
// the default when the file has none, Q3). If the settings file cannot be written,
// the keyring entry is put
// back as it was, so the outcome is all or nothing; only a failed restore leaves it
// unknown.
func storeLogin(app *aicli.App, profile, host string, writeHost bool, cred storedCredential) error {
	account := keyringAccount(profile)
	before, gerr := keyringBackend.Get(keyringService, account)
	existed := gerr == nil
	if gerr != nil && !errors.Is(gerr, keyring.ErrNotFound) {
		return aicli.NotDispatched(ioFailure("The OS keyring cannot be read, so nothing was stored.",
			"Use GITLAB_CLI_TOKEN in the environment instead (the keyring is unavailable here), or ask the user to unlock the keyring."))
	}
	if err := writeCredential(profile, cred); err != nil {
		return aicli.NotDispatched(ioFailure("The OS keyring refused the token, so nothing was stored.",
			"Use GITLAB_CLI_TOKEN in the environment instead (the keyring is unavailable here), or ask the user to unlock the keyring."))
	}
	if !writeHost {
		return nil
	}
	if e := app.SetProfileSettings(profile, map[string]string{"host": host}); e != nil {
		var rerr error
		if existed {
			rerr = keyringBackend.Set(keyringService, account, before)
		} else {
			rerr = keyringBackend.Delete(keyringService, account)
		}
		if rerr != nil {
			return fmt.Errorf("settings.json could not be written (%s) and the keyring entry could not be put back: %w", e.Message, rerr)
		}
		return aicli.NotDispatched(e)
	}
	return nil
}

// ioFailure is E_IO for a local file or keyring failure; nothing was sent.
func ioFailure(message, next string) *aicli.Error {
	return aicli.NewError("E_IO", message, map[string]any{"next_action": next})
}

func addAuthStatus(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "status", RunE: func(cmd *cobra.Command, _ []string) error {
		cred, err := resolveCredentials(app)
		if err != nil {
			return err
		}
		client, err := clientFor(app, cred.Host, cred.Token)
		if err != nil {
			return err
		}
		me, err := client.Users.Me(cmd.Context())
		if err != nil {
			return readError(err)
		}
		out := map[string]any{"profile": profileLabel(cred.Profile), "host": cred.Host, "account": me.Username, "name": me.Name,
			"user_id": idString(me.ID), "credentials_source": cred.Source, "token_name": nil, "scopes": []string{}, "expires_at": nil}
		// The token's own record; GitLab answers it for personal access tokens only.
		if t, err := tokenSelf(cmd.Context(), client); err == nil {
			out["token_name"], out["scopes"] = t.Name, nonNilStrings(t.Scopes)
			if t.ExpiresAt != "" {
				if d, perr := time.Parse("2006-01-02", t.ExpiresAt); perr == nil {
					out["expires_at"] = d.UTC().Format(time.RFC3339)
				}
			}
		} else {
			app.Debugf("personal_access_tokens/self: %v", err)
		}
		return app.OK(cmd, out)
	}}
	parent.AddCommand(c)
	app.Register(c, aicli.Meta{Summary: "Check the token with GitLab: the account, the host, the token's scopes and expiry",
		Effects: []aicli.Effect{aicli.Observe}, Output: "auth_status", ErrorCodes: gitlabReadCodes,
		Maturity: aicli.Verified, Evidence: []string{authEvidence, liveEvidence("auth")},
		Examples: []string{"gitlab-cli auth status", "gitlab-cli auth status --profile work"}})
}

// accessToken is GitLab's record of the token in use.
type accessToken struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt string   `json:"expires_at"`
}

func tokenSelf(ctx context.Context, c *api.Client) (*accessToken, error) {
	data, err := c.Get(ctx, c.APIPath("/personal_access_tokens/self"))
	if err != nil {
		return nil, err
	}
	var t accessToken
	if err := jsonUnmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func addAuthLogout(app *aicli.App, parent *cobra.Command) {
	c := &cobra.Command{Use: "logout", RunE: func(cmd *cobra.Command, _ []string) error {
		profile, err := authProfile(app, "sign out of")
		if err != nil {
			return err
		}
		label := profileLabel(profile)
		return app.Write(cmd, aicli.WriteSpec{
			Target:   aicli.Target{Key: profileTarget(profile), Kind: "profile", Label: label},
			ReadBack: "auth status --profile " + label,
			Preview: func(ctx context.Context) (aicli.Preview, error) {
				stored, err := readCredentialFor(app, profile)
				switch {
				case errors.Is(err, errNoCredential):
					next := "Nothing to remove; `gitlab-cli context` shows which credentials are in use."
					if strings.TrimSpace(app.Env("GITLAB_CLI_TOKEN")) != "" {
						next = "The token in use comes from GITLAB_CLI_TOKEN; ask the user to unset it to stop using it."
					}
					return aicli.Preview{}, aicli.NotFound(fmt.Sprintf("No token is stored in the OS keyring for profile `%s`.", label), next)
				case err != nil:
					return aicli.Preview{}, ioFailure("The OS keyring cannot be read; nothing was changed.", "Ask the user to unlock the keyring, then run the command again.")
				}
				return aicli.Preview{Action: fmt.Sprintf("Remove the GitLab token of profile %s from the OS keyring", label), CanApply: true,
					Changes: []aicli.FieldChange{{Field: "token_stored", Before: true, After: false,
						Note: fmt.Sprintf("the token for %s; settings.json keeps the profile's host", stored.Host)}}}, nil
			},
			Apply: func(ctx context.Context, op aicli.Op) (aicli.Result, error) {
				stored, _ := readCredentialFor(app, profile)
				switch err := deleteCredential(profile); {
				case errors.Is(err, errNoCredential):
					return aicli.Result{}, aicli.NotDispatched(aicli.NotFound("The token was already gone from the keyring; nothing was removed.", ""))
				case err != nil:
					return aicli.Result{}, aicli.NotDispatched(ioFailure("The OS keyring refused to remove the token; nothing was changed.",
						"Ask the user to unlock the keyring, then run --dry-run again."))
				}
				_, rerr := readCredential(profile)
				out := map[string]any{"profile": label, "host": nil, "token_stored": rerr == nil}
				if stored != nil {
					out["host"] = stored.Host
				}
				return aicli.Result{Observed: map[string]any{"token_stored": rerr == nil}, Data: out}, nil
			},
		})
	}}
	parent.AddCommand(c)
	// AllowUnknownProfile: logout also removes the token of a profile already deleted from
	// settings.json, which D4 would otherwise stop up front.
	app.Register(c, aicli.Meta{Summary: "Remove the profile's GitLab token from the OS keyring",
		Effects: []aicli.Effect{aicli.Configure}, Output: "auth_logout", AllowUnknownProfile: true,
		ErrorCodes: []string{"E_CONFIG", "E_IO", "E_NOT_FOUND"},
		Maturity:   aicli.Verified, Evidence: []string{authEvidence, liveEvidence("auth")},
		Examples: []string{"gitlab-cli auth logout --dangerous --dry-run", "gitlab-cli auth logout --dangerous --confirm <confirm_token>"}})
}
