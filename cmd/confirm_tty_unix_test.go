//go:build unix

package cmd

import (
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

func openPTY(t *testing.T) (master, slave *os.File) {
	t.Helper()
	ptmx, tty, err := unix.Openpty(nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("Openpty: %v", err)
	}
	master = os.NewFile(uintptr(ptmx), "ptmx")
	slave = os.NewFile(uintptr(tty), "tty")
	t.Cleanup(func() {
		_ = master.Close()
		_ = slave.Close()
	})
	return master, slave
}

func TestRequireConfirm_TTY_Accepted(t *testing.T) {
	master, slave := openPTY(t)
	origStdin := os.Stdin
	origConfirm := confirmFlag
	origForce := forceMode
	defer func() {
		os.Stdin = origStdin
		confirmFlag = origConfirm
		forceMode = origForce
	}()
	os.Stdin = slave
	confirmFlag = ""
	forceMode = false

	go func() {
		_, _ = io.WriteString(master, "secret\n")
	}()

	if err := requireConfirm(&cobra.Command{}, "delete branch", "secret"); err != nil {
		t.Fatalf("requireConfirm() = %v", err)
	}
}

func TestRequireConfirm_TTY_Rejected(t *testing.T) {
	master, slave := openPTY(t)
	origStdin := os.Stdin
	origConfirm := confirmFlag
	origForce := forceMode
	origExit := lastExit
	defer func() {
		os.Stdin = origStdin
		confirmFlag = origConfirm
		forceMode = origForce
		lastExit = origExit
	}()
	os.Stdin = slave
	confirmFlag = ""
	forceMode = false
	lastExit = 0

	go func() {
		_, _ = io.WriteString(master, "wrong\n")
	}()

	err := requireConfirm(&cobra.Command{}, "delete branch", "secret")
	if err == nil {
		t.Fatal("expected confirmation rejection")
	}
	if lastExit != ExitCancelled {
		t.Fatalf("exit=%d want=%d", lastExit, ExitCancelled)
	}
}

func TestRequireConfirm_TTY_ReadErrorPTY(t *testing.T) {
	_, slave := openPTY(t)
	origStdin := os.Stdin
	origConfirm := confirmFlag
	origForce := forceMode
	origExit := lastExit
	defer func() {
		os.Stdin = origStdin
		confirmFlag = origConfirm
		forceMode = origForce
		lastExit = origExit
	}()
	os.Stdin = slave
	confirmFlag = ""
	forceMode = false
	lastExit = 0

	_ = slave.Close()

	err := requireConfirm(&cobra.Command{}, "delete branch", "secret")
	if err == nil {
		t.Fatal("expected read error")
	}
	if lastExit != ExitCancelled {
		t.Fatalf("exit=%d want=%d", lastExit, ExitCancelled)
	}
}
