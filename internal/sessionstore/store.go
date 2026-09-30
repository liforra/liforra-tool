// Package sessionstore holds the build-key-derived AES-GCM encryption (see
// crypt.go) used to keep a technician logged in across app restarts for a
// few days — the OAuth refresh token (v2 API) and, deliberately, the
// technician's GLPI password (needed to silently restore v1 API access,
// which has no refresh mechanism of its own; GLPI's alternative — a
// personal API token — requires manual per-technician setup in GLPI's web
// UI that in practice nobody will do, so it isn't relied on for this). Not
// tied to a specific machine/account, because the same USB stick this app
// runs from travels between many different prep-station machines and
// needs to decrypt its saved session on all of them. See project memory
// for the full reasoning and the accepted tradeoff.
//
// The Saved/Save/Load/Clear file-based API below is what used to persist
// this on its own (session.dat) before config.toml existed — kept only so
// internal/localsettings can migrate an old session.dat into the new
// config on first run. New code should use localsettings.*Credentials
// instead, which encrypts with the same key via Protect/Unprotect below
// but stores the result inside config.toml alongside everything else.
package sessionstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Saved struct {
	Username     string    `json:"username"`
	Password     string    `json:"password"`
	RefreshToken string    `json:"refreshToken"`
	SavedAt      time.Time `json:"savedAt"`
}

func storePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "liforra-tool")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.dat"), nil
}

// Save encrypts and persists the session. Call after every successful login
// or token refresh (refresh tokens can rotate).
func Save(s Saved) error {
	s.SavedAt = time.Now()
	plain, err := json.Marshal(s)
	if err != nil {
		return err
	}
	cipherBytes, err := protect(plain)
	if err != nil {
		return err
	}
	path, err := storePath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, cipherBytes, 0o600)
}

// Load returns the saved session, or (nil, nil) if there isn't one.
func Load() (*Saved, error) {
	path, err := storePath()
	if err != nil {
		return nil, err
	}
	cipherBytes, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	plain, err := unprotect(cipherBytes)
	if err != nil {
		return nil, err
	}
	var s Saved
	if err := json.Unmarshal(plain, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Clear removes any persisted session (used on explicit logout).
func Clear() error {
	path, err := storePath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
