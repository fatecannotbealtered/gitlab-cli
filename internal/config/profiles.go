package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProfilesFile stores named GitLab credentials and the active profile name.
type ProfilesFile struct {
	Active   string             `json:"active"`
	Profiles map[string]*Config `json:"profiles"`
}

func profilesPath() string {
	return filepath.Join(Dir(), "profiles.json")
}

// LoadProfiles reads ~/.gitlab-cli/profiles.json.
func LoadProfiles() (*ProfilesFile, error) {
	data, err := os.ReadFile(profilesPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &ProfilesFile{Profiles: map[string]*Config{}}, nil
		}
		return nil, fmt.Errorf("reading profiles: %w", err)
	}
	var pf ProfilesFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parsing profiles: %w", err)
	}
	if pf.Profiles == nil {
		pf.Profiles = map[string]*Config{}
	}
	return &pf, nil
}

// SaveProfiles writes profiles.json (mode 0600).
func SaveProfiles(pf *ProfilesFile) error {
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := jsonMarshalIndent(pf, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding profiles: %w", err)
	}
	if err := os.WriteFile(profilesPath(), data, 0600); err != nil {
		return fmt.Errorf("writing profiles: %w", err)
	}
	return nil
}

// ProfileNames returns sorted profile names.
func ProfileNames(pf *ProfilesFile) []string {
	names := make([]string, 0, len(pf.Profiles))
	for name := range pf.Profiles {
		names = append(names, name)
	}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return names
}

// SetProfile saves credentials under name and sets it active.
func SetProfile(name string, cfg *Config) error {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	pf, err := LoadProfiles()
	if err != nil {
		return err
	}
	pf.Profiles[name] = &Config{Host: cfg.Host, Token: cfg.Token}
	pf.Active = name
	return SaveProfiles(pf)
}

// UseProfile switches the active profile.
func UseProfile(name string) error {
	pf, err := LoadProfiles()
	if err != nil {
		return err
	}
	if _, ok := pf.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	pf.Active = name
	if err := SaveProfiles(pf); err != nil {
		return err
	}
	// Keep legacy config.json in sync for tools that only read it.
	return saveLegacyFile(pf.Profiles[name])
}

// DeleteProfile removes a named profile.
func DeleteProfile(name string) error {
	pf, err := LoadProfiles()
	if err != nil {
		return err
	}
	if _, ok := pf.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	delete(pf.Profiles, name)
	if pf.Active == name {
		pf.Active = ""
		for n := range pf.Profiles {
			pf.Active = n
			break
		}
	}
	return SaveProfiles(pf)
}

// activeProfileConfig returns credentials from the active profile, if any.
func activeProfileConfig() (*Config, string, error) {
	pf, err := LoadProfiles()
	if err != nil {
		return nil, "", err
	}
	if pf.Active == "" {
		return nil, "", nil
	}
	c, ok := pf.Profiles[pf.Active]
	if !ok || c == nil {
		return nil, "", nil
	}
	return &Config{Host: c.Host, Token: c.Token}, pf.Active, nil
}
