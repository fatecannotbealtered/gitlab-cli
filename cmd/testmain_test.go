package cmd

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestMain(m *testing.M) {
	// Tests must never touch the real OS keyring.
	keyring.MockInit()
	os.Exit(m.Run())
}
