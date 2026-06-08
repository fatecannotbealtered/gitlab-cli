package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
)

const (
	encryptedFileVersion = 2
	encryptedCipher      = "AES-256-GCM"
	encryptedKDF         = "PBKDF2-SHA256-machine-bound-v1"
	encryptedIterations  = 150000
)

type encryptedFile struct {
	Version    int    `json:"version"`
	Encrypted  bool   `json:"encrypted"`
	Cipher     string `json:"cipher"`
	KDF        string `json:"kdf"`
	Iterations int    `json:"iterations"`
	Salt       string `json:"salt"`
	Nonce      string `json:"nonce"`
	Data       string `json:"data"`
}

func encodeEncryptedJSON(v any) ([]byte, error) {
	plain, err := jsonMarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}

	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}
	key := pbkdf2SHA256(machineBoundSecret(), salt, encryptedIterations, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	env := encryptedFile{
		Version:    encryptedFileVersion,
		Encrypted:  true,
		Cipher:     encryptedCipher,
		KDF:        encryptedKDF,
		Iterations: encryptedIterations,
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Nonce:      base64.StdEncoding.EncodeToString(nonce),
		Data:       base64.StdEncoding.EncodeToString(gcm.Seal(nil, nonce, plain, nil)),
	}
	return jsonMarshalIndent(env, "", "  ")
}

func decodeMaybeEncryptedJSON(data []byte, target any) error {
	var probe struct {
		Encrypted bool `json:"encrypted"`
	}
	if err := json.Unmarshal(data, &probe); err == nil && probe.Encrypted {
		plain, err := decryptEnvelope(data)
		if err != nil {
			return err
		}
		return json.Unmarshal(plain, target)
	}
	return json.Unmarshal(data, target)
}

// CredentialStorageEncrypted reports whether existing on-disk credential files
// use the encrypted envelope format. Missing files are treated as compliant.
func CredentialStorageEncrypted() bool {
	for _, path := range []string{FilePath(), profilesPath()} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var probe struct {
			Encrypted bool `json:"encrypted"`
		}
		if err := json.Unmarshal(data, &probe); err != nil || !probe.Encrypted {
			return false
		}
	}
	return true
}

func decryptEnvelope(data []byte) ([]byte, error) {
	var env encryptedFile
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	if env.Cipher != encryptedCipher {
		return nil, fmt.Errorf("unsupported credential cipher %q", env.Cipher)
	}
	if env.KDF != encryptedKDF {
		return nil, fmt.Errorf("unsupported credential kdf %q", env.KDF)
	}
	if env.Iterations <= 0 {
		return nil, fmt.Errorf("invalid credential kdf iterations")
	}
	salt, err := base64.StdEncoding.DecodeString(env.Salt)
	if err != nil {
		return nil, fmt.Errorf("decoding salt: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, fmt.Errorf("decoding nonce: %w", err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(env.Data)
	if err != nil {
		return nil, fmt.Errorf("decoding encrypted data: %w", err)
	}

	key := pbkdf2SHA256(machineBoundSecret(), salt, env.Iterations, 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating gcm: %w", err)
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid credential nonce size")
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting credentials: %w", err)
	}
	return plain, nil
}

func machineBoundSecret() []byte {
	hostname, _ := os.Hostname()
	home, _ := os.UserHomeDir()
	parts := []string{
		"gitlab-cli credential key",
		runtime.GOOS,
		runtime.GOARCH,
		hostname,
		home,
		os.Getenv("USERDOMAIN"),
		os.Getenv("USERNAME"),
		os.Getenv("USER"),
		os.Getenv("COMPUTERNAME"),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return sum[:]
}

func pbkdf2SHA256(password, salt []byte, iter, keyLen int) []byte {
	hashLen := sha256.Size
	numBlocks := (keyLen + hashLen - 1) / hashLen
	out := make([]byte, 0, numBlocks*hashLen)
	for block := 1; block <= numBlocks; block++ {
		mac := hmac.New(sha256.New, password)
		mac.Write(salt)
		var idx [4]byte
		binary.BigEndian.PutUint32(idx[:], uint32(block))
		mac.Write(idx[:])
		u := mac.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iter; i++ {
			mac = hmac.New(sha256.New, password)
			mac.Write(u)
			u = mac.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}
