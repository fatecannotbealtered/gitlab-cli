package config

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestMain(m *testing.M) {
	// Tests must never touch the real OS keyring.
	keyring.MockInit()
	os.Exit(m.Run())
}

func disableKeyringForTest(t *testing.T) {
	t.Helper()
	origSet, origGet := keyringSet, keyringGet
	keyringSet = func(string, string, string) error { return errors.New("no keyring service") }
	keyringGet = func(string, string) (string, error) { return "", errors.New("no keyring service") }
	t.Cleanup(func() { keyringSet, keyringGet = origSet, origGet })
}

// TestEnvelopeUsesKeyringMasterKey: with a keyring available, the envelope
// records the keyring KDF marker and round-trips through the keyring-held key.
func TestEnvelopeUsesKeyringMasterKey(t *testing.T) {
	_ = keyringDelete(keyringService, keyringMasterAccount)

	data, err := encodeEncryptedJSON(map[string]string{"token": "glpat-secret"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.Contains(string(data), encryptedKDFKeyring) {
		t.Fatalf("envelope should record the keyring KDF marker: %s", data)
	}
	if strings.Contains(string(data), "glpat-secret") {
		t.Fatalf("envelope must not contain plaintext: %s", data)
	}

	var out map[string]string
	if err := decodeMaybeEncryptedJSON(data, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["token"] != "glpat-secret" {
		t.Fatalf("round-trip = %q", out["token"])
	}
}

// TestEnvelopeFallsBackToMachineBound: without a keyring service the envelope
// degrades to the machine-bound KDF and still round-trips.
func TestEnvelopeFallsBackToMachineBound(t *testing.T) {
	disableKeyringForTest(t)

	data, err := encodeEncryptedJSON(map[string]string{"token": "fallback-secret"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !strings.Contains(string(data), encryptedKDF) {
		t.Fatalf("envelope should record the machine-bound KDF marker: %s", data)
	}

	var out map[string]string
	if err := decodeMaybeEncryptedJSON(data, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["token"] != "fallback-secret" {
		t.Fatalf("round-trip = %q", out["token"])
	}
}

// TestKeyringEnvelopeUnreadableWithoutKey: a keyring-encrypted file must be
// undecryptable when the keyring entry is gone (the exfiltration property).
func TestKeyringEnvelopeUnreadableWithoutKey(t *testing.T) {
	_ = keyringDelete(keyringService, keyringMasterAccount)
	data, err := encodeEncryptedJSON(map[string]string{"token": "gone"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	disableKeyringForTest(t)
	var out map[string]string
	if err := decodeMaybeEncryptedJSON(data, &out); err == nil {
		t.Fatal("decode should fail without the keyring master key")
	}
}
