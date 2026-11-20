package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const prefix = "enc:v1:"

// Box encrypts secrets stored next to the SQLite file.
type Box struct {
	key []byte
}

// Load uses dataDir/secret.key when it exists, then HARBOR_SECRET_KEY, and
// otherwise writes a new key file. An existing file wins so a later environment
// variable cannot make old rows undecryptable.
func Load(dataDir string) (*Box, error) {
	path := filepath.Join(dataDir, "secret.key")
	if raw, err := os.ReadFile(path); err == nil {
		if len(raw) != 32 {
			return nil, fmt.Errorf("secret.key must be 32 bytes")
		}
		return &Box{key: raw}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if env := strings.TrimSpace(os.Getenv("HARBOR_SECRET_KEY")); env != "" {
		key, err := parseKey(env)
		if err != nil {
			return nil, err
		}
		return &Box{key: key}, nil
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return &Box{key: key}, nil
}

func parseKey(raw string) ([]byte, error) {
	if len(raw) == 64 {
		if key, err := hex.DecodeString(raw); err == nil && len(key) == 32 {
			return key, nil
		}
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if key, err := enc.DecodeString(raw); err == nil && len(key) == 32 {
			return key, nil
		}
	}
	return nil, errors.New("HARBOR_SECRET_KEY must be 32 bytes, as 64 hex characters or base64")
}

// Sealed reports whether value was produced by Seal.
func Sealed(value string) bool {
	return strings.HasPrefix(value, prefix)
}

// Seal encrypts plain with AES-256-GCM. An empty string is stored as empty.
func (b *Box) Seal(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	block, err := aes.NewCipher(b.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return prefix + base64.RawStdEncoding.EncodeToString(out), nil
}

// Open decrypts a value from Seal. Values without the prefix are returned as
// they are so a migration can read rows written before encryption.
func (b *Box) Open(stored string) (string, error) {
	if stored == "" || !Sealed(stored) {
		return stored, nil
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(b.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("secret ciphertext is too short")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
