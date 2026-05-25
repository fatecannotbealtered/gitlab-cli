package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileNames_Sorted(t *testing.T) {
	// Keys chosen so map iteration order is not already sorted (exercises swap branch).
	pf := &ProfilesFile{
		Profiles: map[string]*Config{
			"personal": {Host: "https://p.example.com", Token: "p"},
			"work":     {Host: "https://w.example.com", Token: "w"},
			"default":  {Host: "https://d.example.com", Token: "d"},
		},
	}
	got := ProfileNames(pf)
	want := []string{"default", "personal", "work"}
	if len(got) != len(want) {
		t.Fatalf("ProfileNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ProfileNames() = %v, want %v", got, want)
		}
	}
}

func TestProfileNames_Empty(t *testing.T) {
	got := ProfileNames(&ProfilesFile{Profiles: map[string]*Config{}})
	if len(got) != 0 {
		t.Errorf("ProfileNames(empty) = %v, want []", got)
	}
}

func TestLoadProfiles_MissingFile(t *testing.T) {
	withTempHome(t, func(_ string) {
		pf, err := LoadProfiles()
		if err != nil {
			t.Fatalf("LoadProfiles: %v", err)
		}
		if pf.Profiles == nil || len(pf.Profiles) != 0 {
			t.Errorf("expected empty profiles map, got %+v", pf)
		}
	})
}

func TestLoadProfiles_ParseError(t *testing.T) {
	withTempHome(t, func(_ string) {
		if err := os.MkdirAll(Dir(), 0700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(profilesPath(), []byte("{not json"), 0600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		_, err := LoadProfiles()
		if err == nil {
			t.Fatal("expected parse error")
		}
		if !strings.Contains(err.Error(), "parsing profiles") {
			t.Fatalf("LoadProfiles() = %v, want parsing profiles error", err)
		}
	})
}

func TestLoadProfiles_ReadError(t *testing.T) {
	withTempHome(t, func(_ string) {
		blockProfilesJSONAsDir(t)
		if _, err := LoadProfiles(); err == nil {
			t.Fatal("expected read error")
		}
	})
}

func TestLoadProfiles_NilProfilesMap(t *testing.T) {
	withTempHome(t, func(_ string) {
		if err := os.MkdirAll(Dir(), 0700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(profilesPath(), []byte(`{"active":"","profiles":null}`), 0600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		pf, err := LoadProfiles()
		if err != nil {
			t.Fatalf("LoadProfiles: %v", err)
		}
		if pf.Profiles == nil {
			t.Fatal("expected non-nil Profiles map")
		}
	})
}

func TestSaveProfiles_MarshalIndentError(t *testing.T) {
	withTempHome(t, func(_ string) {
		orig := jsonMarshalIndent
		jsonMarshalIndent = func(any, string, string) ([]byte, error) {
			return nil, errors.New("marshal indent failed")
		}
		t.Cleanup(func() { jsonMarshalIndent = orig })

		err := SaveProfiles(&ProfilesFile{Profiles: map[string]*Config{}})
		if err == nil {
			t.Fatal("expected MarshalIndent error")
		}
		if !strings.Contains(err.Error(), "encoding profiles") {
			t.Fatalf("SaveProfiles() = %v, want encoding profiles error", err)
		}
	})
}

func TestSaveProfiles_MkdirAllError(t *testing.T) {
	withTempHome(t, func(home string) {
		blockConfigDirAsFile(t, home)
		err := SaveProfiles(&ProfilesFile{Profiles: map[string]*Config{}})
		if err == nil {
			t.Fatal("expected MkdirAll error")
		}
		if !strings.Contains(err.Error(), "creating config dir") {
			t.Fatalf("SaveProfiles() = %v, want creating config dir error", err)
		}
	})
}

func TestSaveProfiles_WriteFileError(t *testing.T) {
	withTempHome(t, func(_ string) {
		if err := os.MkdirAll(Dir(), 0700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.Mkdir(profilesPath(), 0700); err != nil {
			t.Fatalf("Mkdir profiles path: %v", err)
		}
		err := SaveProfiles(&ProfilesFile{Profiles: map[string]*Config{}})
		if err == nil {
			t.Fatal("expected WriteFile error")
		}
		if !strings.Contains(err.Error(), "writing profiles") {
			t.Fatalf("SaveProfiles() = %v, want writing profiles error", err)
		}
	})
}

func TestSetProfile_DefaultNameWhenEmpty(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		cfg := &Config{Host: "https://gitlab.example.com", Token: "tok"}
		if err := SetProfile("  ", cfg); err != nil {
			t.Fatalf("SetProfile: %v", err)
		}
		pf, err := LoadProfiles()
		if err != nil {
			t.Fatalf("LoadProfiles: %v", err)
		}
		if pf.Active != "default" {
			t.Errorf("Active = %q, want default", pf.Active)
		}
		if pf.Profiles["default"] == nil {
			t.Fatal("expected default profile")
		}
	})
}

func TestSetProfile_LoadProfilesError(t *testing.T) {
	withTempHome(t, func(_ string) {
		blockProfilesJSONAsDir(t)
		if err := SetProfile("work", &Config{Host: "https://work.example.com", Token: "tok"}); err == nil {
			t.Fatal("expected LoadProfiles error")
		}
	})
}

func TestUseProfile_NotFound(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		if err := SetProfile("default", &Config{Host: "https://a.example.com", Token: "tok"}); err != nil {
			t.Fatalf("SetProfile: %v", err)
		}
		err := UseProfile("missing")
		if err == nil {
			t.Fatal("expected not found error")
		}
		if !strings.Contains(err.Error(), `profile "missing" not found`) {
			t.Fatalf("UseProfile() = %v, want not found", err)
		}
	})
}

func TestUseProfile_LoadProfilesError(t *testing.T) {
	withTempHome(t, func(_ string) {
		blockProfilesJSONAsDir(t)
		if err := UseProfile("default"); err == nil {
			t.Fatal("expected LoadProfiles error")
		}
	})
}

func TestUseProfile_SaveProfilesError(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		if err := SetProfile("a", &Config{Host: "https://a.example.com", Token: "tok-a"}); err != nil {
			t.Fatalf("SetProfile a: %v", err)
		}
		if err := SetProfile("b", &Config{Host: "https://b.example.com", Token: "tok-b"}); err != nil {
			t.Fatalf("SetProfile b: %v", err)
		}
		if err := os.Chmod(profilesPath(), 0444); err != nil {
			t.Fatalf("Chmod profiles: %v", err)
		}
		if err := os.Chmod(Dir(), 0555); err != nil {
			t.Fatalf("Chmod dir: %v", err)
		}
		t.Cleanup(func() {
			_ = os.Chmod(Dir(), 0755)
			_ = os.Chmod(profilesPath(), 0644)
		})

		err := UseProfile("a")
		if err == nil {
			t.Fatal("expected SaveProfiles error")
		}
		if !strings.Contains(err.Error(), "writing profiles") {
			t.Fatalf("UseProfile() = %v, want writing profiles error", err)
		}
	})
}

func TestUseProfile_SaveLegacyFileError(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		if err := SetProfile("default", &Config{Host: "https://a.example.com", Token: "tok"}); err != nil {
			t.Fatalf("SetProfile: %v", err)
		}
		blockConfigJSONAsDir(t)
		err := UseProfile("default")
		if err == nil {
			t.Fatal("expected saveLegacyFile error")
		}
		if !strings.Contains(err.Error(), "writing config") {
			t.Fatalf("UseProfile() = %v, want writing config error", err)
		}
	})
}

func TestDeleteProfile_NotFound(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		if err := SetProfile("default", &Config{Host: "https://a.example.com", Token: "tok"}); err != nil {
			t.Fatalf("SetProfile: %v", err)
		}
		err := DeleteProfile("missing")
		if err == nil {
			t.Fatal("expected not found error")
		}
		if !strings.Contains(err.Error(), `profile "missing" not found`) {
			t.Fatalf("DeleteProfile() = %v, want not found", err)
		}
	})
}

func TestDeleteProfile_LoadProfilesError(t *testing.T) {
	withTempHome(t, func(_ string) {
		blockProfilesJSONAsDir(t)
		if err := DeleteProfile("default"); err == nil {
			t.Fatal("expected LoadProfiles error")
		}
	})
}

func TestDeleteProfile_RemovesActiveAndReassigns(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		if err := SetProfile("alpha", &Config{Host: "https://a.example.com", Token: "tok-a"}); err != nil {
			t.Fatalf("SetProfile alpha: %v", err)
		}
		if err := SetProfile("beta", &Config{Host: "https://b.example.com", Token: "tok-b"}); err != nil {
			t.Fatalf("SetProfile beta: %v", err)
		}
		if err := DeleteProfile("beta"); err != nil {
			t.Fatalf("DeleteProfile: %v", err)
		}
		pf, err := LoadProfiles()
		if err != nil {
			t.Fatalf("LoadProfiles: %v", err)
		}
		if _, ok := pf.Profiles["beta"]; ok {
			t.Error("beta profile should be removed")
		}
		if pf.Active == "beta" || pf.Active == "" {
			t.Errorf("Active = %q, want reassigned profile name", pf.Active)
		}
		if pf.Profiles[pf.Active] == nil {
			t.Errorf("active profile %q missing", pf.Active)
		}
	})
}

func TestDeleteProfile_RemovesNonActive(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		if err := SetProfile("keep", &Config{Host: "https://keep.example.com", Token: "tok-k"}); err != nil {
			t.Fatalf("SetProfile keep: %v", err)
		}
		if err := SetProfile("drop", &Config{Host: "https://drop.example.com", Token: "tok-d"}); err != nil {
			t.Fatalf("SetProfile drop: %v", err)
		}
		if err := UseProfile("keep"); err != nil {
			t.Fatalf("UseProfile keep: %v", err)
		}
		if err := DeleteProfile("drop"); err != nil {
			t.Fatalf("DeleteProfile drop: %v", err)
		}
		pf, err := LoadProfiles()
		if err != nil {
			t.Fatalf("LoadProfiles: %v", err)
		}
		if pf.Active != "keep" {
			t.Errorf("Active = %q, want keep", pf.Active)
		}
		if _, ok := pf.Profiles["drop"]; ok {
			t.Error("drop profile should be removed")
		}
	})
}

func TestDeleteProfile_SaveProfilesError(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		if err := SetProfile("only", &Config{Host: "https://only.example.com", Token: "tok"}); err != nil {
			t.Fatalf("SetProfile: %v", err)
		}
		if err := os.Chmod(profilesPath(), 0444); err != nil {
			t.Fatalf("Chmod profiles: %v", err)
		}
		if err := os.Chmod(Dir(), 0555); err != nil {
			t.Fatalf("Chmod dir: %v", err)
		}
		t.Cleanup(func() {
			_ = os.Chmod(Dir(), 0755)
			_ = os.Chmod(profilesPath(), 0644)
		})

		err := DeleteProfile("only")
		if err == nil {
			t.Fatal("expected SaveProfiles error")
		}
		if !strings.Contains(err.Error(), "writing profiles") {
			t.Fatalf("DeleteProfile() = %v, want writing profiles error", err)
		}
	})
}

func TestActiveProfileConfig_LoadProfilesError(t *testing.T) {
	withTempHome(t, func(_ string) {
		blockProfilesJSONAsDir(t)
		cfg, name, err := activeProfileConfig()
		if err == nil {
			t.Fatal("expected LoadProfiles error")
		}
		if cfg != nil || name != "" {
			t.Errorf("got cfg=%v name=%q, want nil", cfg, name)
		}
	})
}

func TestActiveProfileConfig_NoActive(t *testing.T) {
	withTempHome(t, func(_ string) {
		writeProfilesFile(t, &ProfilesFile{Active: "", Profiles: map[string]*Config{}})
		cfg, name, err := activeProfileConfig()
		if err != nil {
			t.Fatalf("activeProfileConfig: %v", err)
		}
		if cfg != nil || name != "" {
			t.Errorf("got cfg=%v name=%q, want nil", cfg, name)
		}
	})
}

func TestActiveProfileConfig_MissingProfileEntry(t *testing.T) {
	withTempHome(t, func(_ string) {
		if err := os.MkdirAll(Dir(), 0700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		body := []byte(`{"active":"ghost","profiles":{"default":{"host":"https://x.com","token":"t"}}}`)
		if err := os.WriteFile(profilesPath(), body, 0600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		cfg, name, err := activeProfileConfig()
		if err != nil {
			t.Fatalf("activeProfileConfig: %v", err)
		}
		if cfg != nil || name != "" {
			t.Errorf("got cfg=%v name=%q, want nil for missing active entry", cfg, name)
		}
	})
}

func writeProfilesFile(t *testing.T, pf *ProfilesFile) {
	t.Helper()
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(profilesPath(), data, 0600); err != nil {
		t.Fatalf("WriteFile profiles: %v", err)
	}
}

func blockConfigDirAsFile(t *testing.T, home string) {
	t.Helper()
	p := filepath.Join(home, ".gitlab-cli")
	if err := os.WriteFile(p, []byte("block"), 0600); err != nil {
		t.Fatalf("WriteFile block dir: %v", err)
	}
}
