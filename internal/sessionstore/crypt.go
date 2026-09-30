package sessionstore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

// buildKey is injected at build time via:
//
//	-ldflags "-X liforra-tool/internal/sessionstore.buildKey=<key>"
//
// so the real value never appears in any committed source file — see
// scripts/build-windows.sh, which reads it from a gitignored file. The
// value below is only ever used for local dev builds that skip that script,
// and is deliberately not secret.
//
// This is a shared key baked into every copy of the exe, not tied to a
// specific machine/account (unlike e.g. Windows DPAPI) — a deliberate
// tradeoff because this app's USB stick travels between many different
// prep-station machines and needs to decrypt its saved session on all of
// them. It protects the persisted OAuth refresh token against casual
// discovery (someone browsing a lost stick's files), not against a
// determined attacker with the binary who's willing to reverse-engineer
// it — that's an accepted limit, not an oversight, and is why only a
// revocable refresh token is stored here, never the actual password.
var buildKey = "dev-insecure-build-key-do-not-ship"

func deriveKey() []byte {
	sum := sha256.Sum256([]byte(buildKey))
	return sum[:]
}

// Protect/Unprotect expose this package's build-key-derived AES-GCM
// encryption for other packages persisting a secret the same way — see
// internal/localsettings, which owns the on-disk config (including the
// encrypted login) since config.toml replaced this package's own
// session.dat file.
func Protect(plain []byte) ([]byte, error) { return protect(plain) }
func Unprotect(data []byte) ([]byte, error) { return unprotect(data) }

func protect(plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(deriveKey())
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func unprotect(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(deriveKey())
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("session data too short")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
