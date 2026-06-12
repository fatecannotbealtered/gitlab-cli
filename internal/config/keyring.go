package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/zalando/go-keyring"
)

// Keyring three-part pattern (SEC-SPEC §4), whole-file-envelope variant: the
// AES master key is a random 32-byte secret held by the OS keyring (Windows
// Credential Manager / macOS Keychain / Linux Secret Service), so an
// exfiltrated config/profiles file carries nothing decryptable. The legacy
// machine-bound derivation remains as the fallback when no keyring service is
// available, and for decrypting files written before this scheme.
const (
	keyringService       = "gitlab-cli"
	keyringMasterAccount = "credential-master-key"
	encryptedKDFKeyring  = "keyring-master-key-v1"
)

// Seams: tests override these to avoid touching the real OS keyring.
var (
	keyringSet    = keyring.Set
	keyringGet    = keyring.Get
	keyringDelete = keyring.Delete
)

// masterKeySource returns the encryption secret and the KDF marker recorded
// in the envelope. Preference order: existing keyring key, new keyring key,
// machine-bound fallback.
func masterKeySource() ([]byte, string) {
	if hexKey, err := keyringGet(keyringService, keyringMasterAccount); err == nil {
		if key, decErr := hex.DecodeString(hexKey); decErr == nil && len(key) == 32 {
			return key, encryptedKDFKeyring
		}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err == nil {
		if err := keyringSet(keyringService, keyringMasterAccount, hex.EncodeToString(key)); err == nil {
			return key, encryptedKDFKeyring
		}
	}
	return machineBoundSecret(), encryptedKDF
}

// masterKeyForKDF resolves the decryption secret for a stored envelope.
func masterKeyForKDF(kdf string) ([]byte, error) {
	switch kdf {
	case encryptedKDFKeyring:
		hexKey, err := keyringGet(keyringService, keyringMasterAccount)
		if err != nil {
			return nil, fmt.Errorf("credential master key not found in OS keyring (re-run 'gitlab-cli auth login'): %w", err)
		}
		key, err := hex.DecodeString(hexKey)
		if err != nil || len(key) != 32 {
			return nil, fmt.Errorf("credential master key in OS keyring is corrupt; re-run 'gitlab-cli auth login'")
		}
		return key, nil
	case encryptedKDF:
		return machineBoundSecret(), nil
	default:
		return nil, fmt.Errorf("unsupported credential kdf %q", kdf)
	}
}

// CredentialStorageBackend reports the at-rest backend of stored credential
// files: "keyring", "encrypted-file", or "" when nothing is stored.
func CredentialStorageBackend() string {
	for _, path := range []string{FilePath(), profilesPath()} {
		env, ok := readEnvelopeHeader(path)
		if !ok {
			continue
		}
		if env.KDF == encryptedKDFKeyring {
			return "keyring"
		}
		return "encrypted-file"
	}
	return ""
}
