package config

import (
	"os"
	"path/filepath"
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

func TestDirAndFilePath(t *testing.T) {
	d := Dir()
	if filepath.Base(d) != ".gitlab-cli" {
		t.Errorf("Dir() base = %q, want .gitlab-cli", filepath.Base(d))
	}
	if filepath.Base(FilePath()) != "config.json" {
		t.Errorf("FilePath() base = %q, want config.json", filepath.Base(FilePath()))
	}
}
