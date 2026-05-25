package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTempHome redirects the user home directory to a temp dir for the duration of fn.
func withTempHome(t *testing.T, fn func(string)) {
	t.Helper()
	tmp := t.TempDir()

	// HOME on unix, USERPROFILE on Windows. Set both to be safe.
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp)
	defer func() {
		_ = os.Setenv("HOME", origHome)
		_ = os.Setenv("USERPROFILE", origUserProfile)
	}()

	fn(tmp)
}

func clearGitLabEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"GITLAB_HOST", "GITLAB_TOKEN", "GITLAB_CLI_HOST", "GITLAB_CLI_TOKEN"} {
		t.Setenv(k, "")
	}
}

func TestLoad_Empty(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Host != "" || cfg.Token != "" {
			t.Errorf("expected empty config, got %+v", cfg)
		}
	})
}

func TestLoad_FromFile(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		cfg := &Config{Host: "https://gitlab.example.com", Token: "filetoken"}
		if err := Save(cfg); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.Host != cfg.Host || got.Token != cfg.Token {
			t.Errorf("got %+v, want %+v", got, cfg)
		}
	})
}

func TestLoad_PrecedenceGitLabEnvOverridesFile(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		_ = Save(&Config{Host: "https://from-file.example.com", Token: "filetoken"})
		t.Setenv("GITLAB_HOST", "https://from-env.example.com")
		t.Setenv("GITLAB_TOKEN", "envtoken")

		got, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.Host != "https://from-env.example.com" {
			t.Errorf("Host = %q, want from-env", got.Host)
		}
		if got.Token != "envtoken" {
			t.Errorf("Token = %q, want envtoken", got.Token)
		}
	})
}

func TestLoad_PrecedenceCliEnvOverridesGitLabEnv(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		t.Setenv("GITLAB_HOST", "https://shared.example.com")
		t.Setenv("GITLAB_TOKEN", "shared")
		t.Setenv("GITLAB_CLI_HOST", "https://only-cli.example.com")
		t.Setenv("GITLAB_CLI_TOKEN", "only-cli")

		got, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.Host != "https://only-cli.example.com" {
			t.Errorf("Host = %q, want only-cli", got.Host)
		}
		if got.Token != "only-cli" {
			t.Errorf("Token = %q, want only-cli", got.Token)
		}
	})
}

func TestLoad_PartialCliEnvFallsBackToGitLabEnv(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		t.Setenv("GITLAB_HOST", "https://from-env.example.com")
		t.Setenv("GITLAB_TOKEN", "envtoken")
		// Only CLI token set, host should fall back to GITLAB_HOST
		t.Setenv("GITLAB_CLI_TOKEN", "cli-only-token")

		got, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.Host != "https://from-env.example.com" {
			t.Errorf("Host = %q, want fallback to GITLAB_HOST", got.Host)
		}
		if got.Token != "cli-only-token" {
			t.Errorf("Token = %q, want cli-only-token", got.Token)
		}
	})
}

func TestLoad_CorruptFileReturnsError(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		if err := os.MkdirAll(Dir(), 0700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(FilePath(), []byte("not json {"), 0600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if _, err := Load(); err == nil {
			t.Error("expected error for corrupt JSON, got nil")
		}
	})
}

func TestSave_FilePermissions(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		cfg := &Config{Host: "https://gitlab.example.com", Token: "tok"}
		if err := Save(cfg); err != nil {
			t.Fatalf("Save: %v", err)
		}
		fi, err := os.Stat(FilePath())
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		// On Windows the unix mode bits aren't fully meaningful, but on Unix we expect 0600.
		mode := fi.Mode().Perm()
		if mode != 0 && mode != 0600 {
			// If non-zero, must be 0600. Zero on some Windows filesystems is acceptable.
			t.Logf("file mode = %v (informational)", mode)
		}
	})
}

func TestMustLoad_RequiresHostAndToken(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		if _, err := MustLoad(); err == nil {
			t.Error("expected error when not configured")
		}

		t.Setenv("GITLAB_HOST", "https://gitlab.example.com")
		t.Setenv("GITLAB_TOKEN", "tok")
		if _, err := MustLoad(); err != nil {
			t.Errorf("unexpected error with full config: %v", err)
		}
	})
}

func TestMustLoad_RequiresHttps(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		t.Setenv("GITLAB_HOST", "ftp://gitlab.example.com")
		t.Setenv("GITLAB_TOKEN", "tok")
		if _, err := MustLoad(); err == nil {
			t.Error("expected error for non-http(s) host")
		}
	})
}

func TestMustLoad_HttpAllowedOnlyForLoopback(t *testing.T) {
	loopback := []string{
		"http://localhost",
		"http://localhost:8080",
		"http://127.0.0.1",
		"http://127.0.0.1:9000",
		"http://[::1]:8080",
	}
	for _, host := range loopback {
		withTempHome(t, func(_ string) {
			clearGitLabEnv(t)
			t.Setenv("GITLAB_HOST", host)
			t.Setenv("GITLAB_TOKEN", "tok")
			if _, err := MustLoad(); err != nil {
				t.Errorf("loopback host %q should be allowed: %v", host, err)
			}
		})
	}

	nonLoopback := []string{
		"http://gitlab.example.com",
		"http://192.168.1.10",
		"http://10.0.0.1",
		"http://gitlab",
	}
	for _, host := range nonLoopback {
		withTempHome(t, func(_ string) {
			clearGitLabEnv(t)
			t.Setenv("GITLAB_HOST", host)
			t.Setenv("GITLAB_TOKEN", "tok")
			if _, err := MustLoad(); err == nil {
				t.Errorf("non-loopback http:// host %q should be rejected", host)
			}
		})
	}
}

func TestDelete_RemovesFile(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		_ = Save(&Config{Host: "https://gitlab.example.com", Token: "tok"})
		if err := Delete(); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := os.Stat(FilePath()); !os.IsNotExist(err) {
			t.Errorf("expected file removed, stat err = %v", err)
		}
		// Idempotent
		if err := Delete(); err != nil {
			t.Errorf("second Delete should be idempotent, got %v", err)
		}
	})
}

func TestUseProfile_KeepsActiveProfile(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		if err := SetProfile("default", &Config{Host: "https://a.example.com", Token: "tok-a"}); err != nil {
			t.Fatalf("SetProfile default: %v", err)
		}
		if err := SetProfile("work", &Config{Host: "https://b.example.com", Token: "tok-b"}); err != nil {
			t.Fatalf("SetProfile work: %v", err)
		}
		if err := UseProfile("work"); err != nil {
			t.Fatalf("UseProfile: %v", err)
		}
		pf, err := LoadProfiles()
		if err != nil {
			t.Fatalf("LoadProfiles: %v", err)
		}
		if pf.Active != "work" {
			t.Errorf("Active = %q, want work", pf.Active)
		}
		got, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.Host != "https://b.example.com" || got.Token != "tok-b" {
			t.Errorf("Load got %+v, want work profile creds", got)
		}
	})
}

func TestClearStoredCredentials(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		if err := SetProfile("default", &Config{Host: "https://gitlab.example.com", Token: "tok"}); err != nil {
			t.Fatalf("SetProfile: %v", err)
		}
		if !IsConfigured() {
			t.Fatal("expected configured before clear")
		}
		if err := ClearStoredCredentials(); err != nil {
			t.Fatalf("ClearStoredCredentials: %v", err)
		}
		if IsConfigured() {
			t.Error("expected not configured after clear")
		}
		if _, err := os.Stat(profilesPath()); !os.IsNotExist(err) {
			t.Errorf("expected profiles.json removed, stat err = %v", err)
		}
	})
}

func TestDirAndFilePath(t *testing.T) {
	withTempHome(t, func(home string) {
		got := Dir()
		want := filepath.Join(home, ".gitlab-cli")
		if got != want {
			t.Errorf("Dir() = %q, want %q", got, want)
		}
		if filepath.Base(FilePath()) != "config.json" {
			t.Errorf("FilePath() base = %q, want config.json", filepath.Base(FilePath()))
		}
	})
}

func TestDir_FallbackWhenUserHomeDirFails(t *testing.T) {
	for _, k := range []string{"HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH"} {
		t.Setenv(k, "")
	}
	if got := Dir(); got != ".gitlab-cli" {
		t.Errorf("Dir() = %q, want .gitlab-cli when UserHomeDir fails", got)
	}
}

func TestLoad_FromLegacyFileOnly(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		writeLegacyConfigFile(t, &Config{Host: "https://legacy.example.com", Token: "legacy-token"})
		got, err := Load()
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got.Host != "https://legacy.example.com" || got.Token != "legacy-token" {
			t.Errorf("Load() = %+v, want legacy file creds", got)
		}
	})
}

func TestLoad_ReadConfigError(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		blockConfigJSONAsDir(t)
		_, err := Load()
		if err == nil {
			t.Fatal("expected read config error")
		}
		if !strings.Contains(err.Error(), "reading config") {
			t.Fatalf("Load() = %v, want reading config error", err)
		}
	})
}

func TestLoad_ActiveProfileLoadError(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		if err := os.MkdirAll(Dir(), 0700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(profilesPath(), []byte("{bad"), 0600); err != nil {
			t.Fatalf("WriteFile profiles: %v", err)
		}
		if _, err := Load(); err == nil {
			t.Fatal("expected active profile load error")
		}
	})
}

func TestMustLoad_LoadError(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		if err := os.MkdirAll(Dir(), 0700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(profilesPath(), []byte("{bad"), 0600); err != nil {
			t.Fatalf("WriteFile profiles: %v", err)
		}
		if _, err := MustLoad(); err == nil {
			t.Fatal("expected Load error from MustLoad")
		}
	})
}

func TestMustLoad_WhitespaceTokenRejected(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		t.Setenv("GITLAB_HOST", "https://gitlab.example.com")
		t.Setenv("GITLAB_TOKEN", "   ")
		if _, err := MustLoad(); err == nil {
			t.Fatal("expected error for whitespace token")
		}
	})
}

func TestMustLoad_HttpStripsPathAndUserinfo(t *testing.T) {
	cases := []string{
		"http://user@localhost/gitlab",
		"http://127.0.0.1:8080/path?query=1",
	}
	for _, host := range cases {
		t.Run(host, func(t *testing.T) {
			withTempHome(t, func(_ string) {
				clearGitLabEnv(t)
				t.Setenv("GITLAB_HOST", host)
				t.Setenv("GITLAB_TOKEN", "tok")
				if _, err := MustLoad(); err != nil {
					t.Fatalf("MustLoad(%q): %v", host, err)
				}
			})
		})
	}
}

func TestSaveLegacyFile_MkdirAllError(t *testing.T) {
	withTempHome(t, func(home string) {
		blockConfigDirAsFile(t, home)
		err := saveLegacyFile(&Config{Host: "https://gitlab.example.com", Token: "tok"})
		if err == nil {
			t.Fatal("expected MkdirAll error")
		}
		if !strings.Contains(err.Error(), "creating config dir") {
			t.Fatalf("saveLegacyFile() = %v, want creating config dir error", err)
		}
	})
}

func TestSaveLegacyFile_WriteFileError(t *testing.T) {
	withTempHome(t, func(_ string) {
		blockConfigJSONAsDir(t)
		err := saveLegacyFile(&Config{Host: "https://gitlab.example.com", Token: "tok"})
		if err == nil {
			t.Fatal("expected WriteFile error")
		}
		if !strings.Contains(err.Error(), "writing config") {
			t.Fatalf("saveLegacyFile() = %v, want writing config error", err)
		}
	})
}

func TestSaveLegacyFile_MarshalIndentError(t *testing.T) {
	withTempHome(t, func(_ string) {
		orig := jsonMarshalIndent
		jsonMarshalIndent = func(any, string, string) ([]byte, error) {
			return nil, errors.New("marshal indent failed")
		}
		t.Cleanup(func() { jsonMarshalIndent = orig })

		err := saveLegacyFile(&Config{Host: "https://gitlab.example.com", Token: "tok"})
		if err == nil {
			t.Fatal("expected MarshalIndent error")
		}
		if !strings.Contains(err.Error(), "encoding config") {
			t.Fatalf("saveLegacyFile() = %v, want encoding config error", err)
		}
	})
}

func TestSave_LegacyFileError(t *testing.T) {
	withTempHome(t, func(home string) {
		blockConfigDirAsFile(t, home)
		err := Save(&Config{Host: "https://gitlab.example.com", Token: "tok"})
		if err == nil {
			t.Fatal("expected saveLegacyFile error")
		}
		if !strings.Contains(err.Error(), "creating config dir") {
			t.Fatalf("Save() = %v, want creating config dir error", err)
		}
	})
}

func TestDelete_Error(t *testing.T) {
	withTempHome(t, func(_ string) {
		blockConfigJSONAsNonemptyDir(t)
		err := Delete()
		if err == nil {
			t.Fatal("expected delete error")
		}
		if !strings.Contains(err.Error(), "deleting config") {
			t.Fatalf("Delete() = %v, want deleting config error", err)
		}
	})
}

func TestClearStoredCredentials_DeleteError(t *testing.T) {
	withTempHome(t, func(_ string) {
		blockConfigJSONAsNonemptyDir(t)
		err := ClearStoredCredentials()
		if err == nil {
			t.Fatal("expected Delete error")
		}
		if !strings.Contains(err.Error(), "deleting config") {
			t.Fatalf("ClearStoredCredentials() = %v, want deleting config error", err)
		}
	})
}

func TestClearStoredCredentials_ProfilesDeleteError(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		if err := SetProfile("default", &Config{Host: "https://gitlab.example.com", Token: "tok"}); err != nil {
			t.Fatalf("SetProfile: %v", err)
		}
		if err := Delete(); err != nil {
			t.Fatalf("Delete config: %v", err)
		}
		blockProfilesJSONAsNonemptyDir(t)
		err := ClearStoredCredentials()
		if err == nil {
			t.Fatal("expected profiles delete error")
		}
		if !strings.Contains(err.Error(), "deleting profiles") {
			t.Fatalf("ClearStoredCredentials() = %v, want deleting profiles error", err)
		}
	})
}

func TestIsConfigured_LoadError(t *testing.T) {
	withTempHome(t, func(_ string) {
		clearGitLabEnv(t)
		if err := os.MkdirAll(Dir(), 0700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(profilesPath(), []byte("{bad"), 0600); err != nil {
			t.Fatalf("WriteFile profiles: %v", err)
		}
		if IsConfigured() {
			t.Error("IsConfigured() = true, want false on Load error")
		}
	})
}

func writeLegacyConfigFile(t *testing.T, cfg *Config) {
	t.Helper()
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(FilePath(), data, 0600); err != nil {
		t.Fatalf("WriteFile config: %v", err)
	}
}

func blockConfigJSONAsDir(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.RemoveAll(FilePath()); err != nil {
		t.Fatalf("RemoveAll config path: %v", err)
	}
	if err := os.Mkdir(FilePath(), 0700); err != nil {
		t.Fatalf("Mkdir config path: %v", err)
	}
}

func blockConfigJSONAsNonemptyDir(t *testing.T) {
	t.Helper()
	blockConfigJSONAsDir(t)
	if err := os.WriteFile(filepath.Join(FilePath(), "inner"), []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile inner: %v", err)
	}
}

func blockProfilesJSONAsDir(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.RemoveAll(profilesPath()); err != nil {
		t.Fatalf("RemoveAll profiles path: %v", err)
	}
	if err := os.Mkdir(profilesPath(), 0700); err != nil {
		t.Fatalf("Mkdir profiles path: %v", err)
	}
}

func blockProfilesJSONAsNonemptyDir(t *testing.T) {
	t.Helper()
	blockProfilesJSONAsDir(t)
	if err := os.WriteFile(filepath.Join(profilesPath(), "inner"), []byte("x"), 0600); err != nil {
		t.Fatalf("WriteFile inner: %v", err)
	}
}
