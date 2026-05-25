//go:build unix

package cmd

import (
	"io"
	"os"
	"testing"

	"github.com/fatecannotbealtered/gitlab-cli/internal/config"
)

func TestAuth_Login_Interactive_ReadPassword_Unix(t *testing.T) {
	srv := mockUserServer(t)
	defer srv.Close()

	isolateConfigHome(t)
	clearAuthEnv(t)
	resetAuthLoginFlags(t)

	master, slave := openPTY(t)
	origStdin := os.Stdin
	defer func() { os.Stdin = origStdin }()
	os.Stdin = slave

	go func() {
		_, _ = io.WriteString(master, "secret-token\n")
	}()

	origExit := lastExit
	defer func() { lastExit = origExit }()
	lastExit = 0

	captureCombinedOutput(t, func() {
		rootCmd.SetArgs([]string{"auth", "login", "--host", srv.URL})
		_ = rootCmd.Execute()
	})

	if lastExit != ExitOK {
		t.Fatalf("expected exit 0, got %d", lastExit)
	}
	pf, err := config.LoadProfiles()
	if err != nil {
		t.Fatalf("LoadProfiles: %v", err)
	}
	if pf.Profiles["default"].Token != "secret-token" {
		t.Fatalf("token not saved: %+v", pf.Profiles["default"])
	}
}

func TestAuth_Login_Interactive_ReadPasswordError_Unix(t *testing.T) {
	srv := mockUserServer(t)
	defer srv.Close()

	resetAuthLoginFlags(t)
	clearAuthEnv(t)

	_, slave := openPTY(t)
	origStdin := os.Stdin
	origExit := lastExit
	defer func() {
		os.Stdin = origStdin
		lastExit = origExit
	}()
	os.Stdin = slave
	lastExit = 0
	_ = slave.Close()

	rootCmd.SetArgs([]string{"auth", "login", "--host", srv.URL})
	_ = rootCmd.Execute()

	if lastExit != ExitNetwork {
		t.Fatalf("expected exit %d, got %d", lastExit, ExitNetwork)
	}
}
