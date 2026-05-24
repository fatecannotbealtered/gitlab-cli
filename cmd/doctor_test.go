package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoctor_Help(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"doctor", "--help"})
	_ = rootCmd.Execute()
	rootCmd.SetOut(nil)
	// Reset the help flag so it doesn't persist into subsequent tests.
	if f := doctorCmd.Flags().Lookup("help"); f != nil {
		_ = f.Value.Set("false")
	}
	out := buf.String()
	if !strings.Contains(out, "doctor") {
		t.Errorf("doctor --help missing expected text\noutput:\n%s", out)
	}
}

func TestDoctor_NoConfig(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"doctor"})
	_ = rootCmd.Execute()

	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit when not configured, got %d", lastExit)
	}
}

func TestDoctor_WithMockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/user":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1,"username":"alice","name":"Alice"}`))
		case "/api/v4/version":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"version":"16.0","revision":"abc"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	rootCmd.SetArgs([]string{"doctor"})
	_ = rootCmd.Execute()

	if lastExit != ExitOK {
		t.Errorf("expected exit 0 with valid mock server, got %d", lastExit)
	}
}

func TestDoctor_NoConfig_JSON(t *testing.T) {
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("USERPROFILE", fakeHome)
	t.Setenv("GITLAB_CLI_HOST", "")
	t.Setenv("GITLAB_CLI_TOKEN", "")
	t.Setenv("GITLAB_HOST", "")
	t.Setenv("GITLAB_TOKEN", "")
	origJM := jsonMode
	origExit := lastExit
	defer func() { jsonMode = origJM; lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"doctor", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"configExists"`) {
		t.Errorf("expected configExists in JSON output, got: %s", out)
	}
}

func TestDoctor_WithMockServer_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":1,"username":"alice","name":"Alice"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "test")
	origJM := jsonMode
	origExit := lastExit
	defer func() { jsonMode = origJM; lastExit = origExit }()
	lastExit = 0
	out := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"doctor", "--json"})
		_ = rootCmd.Execute()
	})
	if !strings.Contains(out, `"authValid": true`) {
		t.Errorf("expected authValid true in JSON output, got: %s", out)
	}
}

func TestDoctor_AuthFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
	}))
	defer srv.Close()
	t.Setenv("GITLAB_CLI_HOST", srv.URL)
	t.Setenv("GITLAB_CLI_TOKEN", "bad-token")
	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0
	rootCmd.SetArgs([]string{"doctor"})
	_ = rootCmd.Execute()
	if lastExit == ExitOK {
		t.Errorf("expected non-zero exit for auth failure, got %d", lastExit)
	}
}
