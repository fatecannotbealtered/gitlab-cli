package cmd

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/zalando/go-keyring"

	"github.com/fatecannotbealtered/gitlab-cli/internal/aicli"
)

// Credentials at rest (SEC-040): the token of each profile is one OS keyring entry
// (Windows Credential Manager, macOS Keychain, Linux Secret Service), service
// "gitlab-cli", account "profile:<name>" ("profile:default" when no profile is
// active). The entry also records the host the token was issued for, so a token is
// never sent to another host, and the username `auth login` verified, which
// `context` reports without a network call (CLI-035). The settings file holds no
// secret.

const keyringService = "gitlab-cli"

// credentialBackend is where tokens are kept. Production uses the OS keyring; tests
// swap in a memory store (cmd/main_test.go) that also counts writes, since a login
// writes to no upstream (the conformance harness counts them as writes).
type credentialBackend interface {
	Get(service, user string) (string, error)
	Set(service, user, secret string) error
	Delete(service, user string) error
}

type osKeyring struct{}

func (osKeyring) Get(service, user string) (string, error) { return keyring.Get(service, user) }
func (osKeyring) Set(service, user, secret string) error   { return keyring.Set(service, user, secret) }
func (osKeyring) Delete(service, user string) error        { return keyring.Delete(service, user) }

// keyringBackend is the credential store in use (a test seam, not configuration).
var keyringBackend credentialBackend = osKeyring{}

// errNoCredential is a profile with no keyring entry.
var errNoCredential = errors.New("no keyring entry")

// storedCredential is the JSON value of one keyring entry.
type storedCredential struct {
	Version  int    `json:"v"`
	Host     string `json:"host"`
	Token    string `json:"token"`
	Username string `json:"username,omitempty"`
	UserID   string `json:"user_id,omitempty"`
	SavedAt  string `json:"saved_at,omitempty"`
}

// keyringAccount names the keyring entry of a profile.
func keyringAccount(profile string) string {
	if profile == "" {
		profile = "default"
	}
	return "profile:" + profile
}

// profileLabel is the profile name a person reads ("default" when none is active).
func profileLabel(profile string) string {
	if profile == "" {
		return "default"
	}
	return profile
}

// readCredential returns the stored credential of a profile: errNoCredential when
// there is none, another error when the keyring cannot be read or the entry is not
// one gitlab-cli wrote.
func readCredential(profile string) (*storedCredential, error) {
	raw, err := keyringBackend.Get(keyringService, keyringAccount(profile))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, errNoCredential
	}
	if err != nil {
		return nil, err
	}
	var c storedCredential
	if err := json.Unmarshal([]byte(raw), &c); err != nil || strings.TrimSpace(c.Token) == "" || c.Host == "" {
		return nil, errors.New("the keyring entry is not a gitlab-cli credential")
	}
	return &c, nil
}

// readCredentialFor is readCredential that also registers the stored token with
// App.AddSecret, so debug lines and errors show it as *** (CLI-102, D2): aicli
// redacts the secret settings it resolves, and this token is not one of them.
func readCredentialFor(app *aicli.App, profile string) (*storedCredential, error) {
	c, err := readCredential(profile)
	if err == nil {
		app.AddSecret(c.Token)
	}
	return c, err
}

// writeCredential stores a profile's credential.
func writeCredential(profile string, c storedCredential) error {
	c.Version = 1
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return keyringBackend.Set(keyringService, keyringAccount(profile), string(b))
}

// deleteCredential removes a profile's credential; a missing entry is
// errNoCredential.
func deleteCredential(profile string) error {
	err := keyringBackend.Delete(keyringService, keyringAccount(profile))
	if errors.Is(err, keyring.ErrNotFound) {
		return errNoCredential
	}
	return err
}
