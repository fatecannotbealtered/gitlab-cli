package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// jsonMarshalIndent marshals config JSON; overridden in tests. // test hook
var jsonMarshalIndent = json.MarshalIndent

// Config stores GitLab authentication information.
type Config struct {
	Host  string `json:"host"`
	Token string `json:"token"`
}

// Dir returns the configuration directory path ~/.gitlab-cli/
func Dir() string {
	// Prefer HOME when set (tests and cross-platform overrides); fall back to OS user home.
	if home := strings.TrimSpace(os.Getenv("HOME")); home != "" {
		return filepath.Join(home, ".gitlab-cli")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".gitlab-cli"
	}
	return filepath.Join(home, ".gitlab-cli")
}

// FilePath returns the configuration file path ~/.gitlab-cli/config.json
func FilePath() string {
	return filepath.Join(Dir(), "config.json")
}

// firstNonEmpty returns the first non-empty trimmed string from candidates.
func firstNonEmpty(candidates ...string) string {
	for _, c := range candidates {
		if v := strings.TrimSpace(c); v != "" {
			return v
		}
	}
	return ""
}

// Load reads the configuration with three-tier precedence:
//
//	GITLAB_CLI_HOST  / GITLAB_CLI_TOKEN   (highest, isolates this CLI from glab)
//	GITLAB_HOST      / GITLAB_TOKEN       (compatible with glab and other GitLab tooling)
//	~/.gitlab-cli/config.json             (saved by `gitlab-cli auth login`)
//
// Returns an empty Config (no error) if no source has values.
// Corrupt config JSON returns an explicit error.
func Load() (*Config, error) {
	cfg := &Config{}

	// 0. Active profile (overrides legacy file when set)
	if prof, _, err := activeProfileConfig(); err != nil {
		return nil, err
	} else if prof != nil && prof.Host != "" && strings.TrimSpace(prof.Token) != "" {
		cfg.Host = prof.Host
		cfg.Token = prof.Token
	}

	// 1. Stored config file (lowest precedence among stored creds; encrypted
	// when written by current versions, plaintext legacy files still read).
	fileCfg, exists, err := LoadStoredConfig()
	if err != nil {
		return nil, err
	}
	if exists {
		if cfg.Host == "" {
			cfg.Host = fileCfg.Host
		}
		if cfg.Token == "" {
			cfg.Token = fileCfg.Token
		}
	}

	// 2. GITLAB_* env vars (compatible with glab) — overrides file
	if v := firstNonEmpty(os.Getenv("GITLAB_HOST")); v != "" {
		cfg.Host = v
	}
	if v := firstNonEmpty(os.Getenv("GITLAB_TOKEN")); v != "" {
		cfg.Token = v
	}

	// 3. GITLAB_CLI_* env vars — highest precedence (isolates from glab)
	if v := firstNonEmpty(os.Getenv("GITLAB_CLI_HOST")); v != "" {
		cfg.Host = v
	}
	if v := firstNonEmpty(os.Getenv("GITLAB_CLI_TOKEN")); v != "" {
		cfg.Token = v
	}

	return cfg, nil
}

// saveLegacyFile writes cfg to ~/.gitlab-cli/config.json only.
func saveLegacyFile(cfg *Config) error {
	dir := Dir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := encodeEncryptedJSON(cfg)
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	if err := os.WriteFile(FilePath(), data, 0600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// LoadStoredConfig reads ~/.gitlab-cli/config.json directly. Current files are
// encrypted; plaintext legacy files are still accepted and rewritten encrypted
// on the next save.
func LoadStoredConfig() (*Config, bool, error) {
	data, err := os.ReadFile(FilePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, false, nil
		}
		return nil, false, fmt.Errorf("reading config: %w", err)
	}
	fileCfg := &Config{}
	if err := decodeMaybeEncryptedJSON(data, fileCfg); err != nil {
		return nil, true, fmt.Errorf("parsing config %s: %w", FilePath(), err)
	}
	return fileCfg, true, nil
}

// Save writes config.json and updates the default profile (legacy save path).
func Save(cfg *Config) error {
	if err := saveLegacyFile(cfg); err != nil {
		return err
	}
	return SetProfile("default", cfg)
}

// MustLoad reads the configuration and validates required fields.
func MustLoad() (*Config, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}

	if cfg.Host == "" || strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("not logged in: run 'gitlab-cli auth login' or set GITLAB_HOST and GITLAB_TOKEN environment variables")
	}

	if !strings.HasPrefix(cfg.Host, "https://") && !strings.HasPrefix(cfg.Host, "http://") {
		return nil, errors.New("host must start with https:// (or http:// for local development)")
	}

	// http:// is only allowed for loopback hosts (local dev).
	if strings.HasPrefix(cfg.Host, "http://") {
		host := strings.TrimPrefix(cfg.Host, "http://")
		// Strip path / port / userinfo for the host check.
		if i := strings.IndexAny(host, "/?#"); i >= 0 {
			host = host[:i]
		}
		if at := strings.LastIndex(host, "@"); at >= 0 {
			host = host[at+1:]
		}
		if i := strings.LastIndex(host, ":"); i >= 0 {
			host = host[:i]
		}
		host = strings.ToLower(host)
		if host != "localhost" && host != "127.0.0.1" && host != "[::1]" && host != "::1" {
			return nil, fmt.Errorf("http:// is only allowed for loopback hosts (localhost, 127.0.0.1, [::1]); got %q", cfg.Host)
		}
	}

	return cfg, nil
}

// Delete removes the configuration file.
func Delete() error {
	err := os.Remove(FilePath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deleting config: %w", err)
	}
	return nil
}

// ClearStoredCredentials removes config.json, profiles.json, and the keyring
// master key (logout).
func ClearStoredCredentials() error {
	if err := Delete(); err != nil {
		return err
	}
	err := os.Remove(profilesPath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deleting profiles: %w", err)
	}
	// Best-effort: no key exists for machine-bound or env-only setups.
	_ = keyringDelete(keyringService, keyringMasterAccount)
	return nil
}

// IsConfigured reports whether credentials are available (file or env vars).
func IsConfigured() bool {
	cfg, err := Load()
	if err != nil {
		return false
	}
	return cfg.Host != "" && strings.TrimSpace(cfg.Token) != ""
}
