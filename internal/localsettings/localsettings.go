// Package localsettings is this app's single on-disk config file
// (config.toml) — auto-created the first time there's nothing to read,
// with every option filled in from a real default rather than left for
// some other layer to fall back on, so there's never a "field missing"
// case once the file exists. The one field that isn't plain text is
// EncryptedCredentials: the technician's GLPI login, needed to silently
// resume a session across restarts (v1 API has no refresh mechanism of
// its own — see internal/sessionstore's package doc for the full
// reasoning), encrypted with the same build-time key sessionstore always
// used, so a lost USB stick doesn't hand out a live password in plain
// text sitting next to the rest of the settings.
package localsettings

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"

	"liforra-tool/internal/config"
	"liforra-tool/internal/installlogic"
	"liforra-tool/internal/sessionstore"
)

// Settings is the full config — every field always has a real value, never
// "unset, use the compiled default", since Load() writes real defaults to
// disk the first time the file doesn't exist.
type Settings struct {
	UpdateServerURL string `toml:"updateServerURL"`
	AutoUpdate      bool   `toml:"autoUpdate"`
	// Administrator lets "Neues Gerät" create GLPI entries (manufacturer,
	// model, type, OS, component catalog) that don't exist yet. A local
	// convenience toggle, not an access control — GLPI's own permissions
	// still decide what the logged-in account may actually write.
	Administrator bool `toml:"administrator"`
	// BetterPDFNames renames attached USB files after the device
	// ("#3303 Testbericht.pdf") instead of keeping toolstar's names.
	BetterPDFNames bool `toml:"betterPDFNames"`
	// EncryptedCredentials is base64(AES-GCM(JSON(Credentials))) — see
	// package doc. Empty until a technician logs in.
	EncryptedCredentials string `toml:"encryptedCredentials,omitempty"`
}

// Defaults is what a freshly created config.toml contains, before any
// technician has changed a setting or logged in.
func Defaults() Settings {
	return Settings{
		UpdateServerURL: config.DefaultUpdateServerURL,
		AutoUpdate:      config.DefaultAutoUpdate,
		Administrator:   false,
		BetterPDFNames:  false,
	}
}

// Credentials is the technician's saved login — never persisted except
// inside Settings.EncryptedCredentials.
type Credentials struct {
	Username     string    `json:"username"`
	Password     string    `json:"password"`
	RefreshToken string    `json:"refreshToken"`
	SavedAt      time.Time `json:"savedAt"`
}

// portableDir returns the directory config.toml belongs in when this binary
// was installed "portable" (see cmd/installer) — the entire point of that
// install mode is that the USB stick carries its own state from PC to PC,
// which only works if the config lives next to the exe, not in whatever
// machine happens to be running it. Returns ("", false) for anything else:
// installed non-portable (registered in Start Menu/registry), or just run
// directly with no install manifest at all (dev builds, `wails build`
// output) — those fall back to the host's per-user config dir below.
func portableDir() (string, bool) {
	exe, err := os.Executable()
	if err != nil {
		return "", false
	}
	m, err := installlogic.LoadManifest(filepath.Dir(exe))
	if err != nil || !m.Portable {
		return "", false
	}
	return filepath.Dir(exe), true
}

func dir() (string, error) {
	if d, ok := portableDir(); ok {
		return d, nil
	}

	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	d = filepath.Join(d, "liforra-tool")
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	return d, nil
}

func configPath() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.toml"), nil
}

// Load reads config.toml. The first time there's nothing to read, it
// creates one — folding in a pre-existing settings.json/session.dat from
// before this file existed (see migrateOrDefault), or just the compiled
// defaults — and writes it to disk immediately, so "no config present"
// only ever happens once per install.
func Load() (Settings, error) {
	p, err := configPath()
	if err != nil {
		return Settings{}, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		s := migrateOrDefault()
		return s, Save(s)
	}
	if err != nil {
		return Settings{}, err
	}
	var s Settings
	if err := toml.Unmarshal(data, &s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

func Save(s Settings) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	data, err := toml.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// migrateOrDefault folds in a pre-existing settings.json (this app's old
// plain-JSON settings file) and/or session.dat (its old separate encrypted
// session file) so upgrading to config.toml doesn't silently drop a
// technician's settings or saved login. Best-effort throughout — any
// read/parse failure on the old files is treated as "nothing to migrate
// there", never fatal to starting the app.
func migrateOrDefault() Settings {
	s := Defaults()

	d, err := dir()
	if err != nil {
		return s
	}

	oldSettingsPath := filepath.Join(d, "settings.json")
	if data, err := os.ReadFile(oldSettingsPath); err == nil {
		var old struct {
			UpdateServerURL string `json:"updateServerURL"`
			AutoUpdate      *bool  `json:"autoUpdate"`
			Administrator   bool   `json:"administrator"`
			BetterPDFNames  bool   `json:"betterPDFNames"`
		}
		if json.Unmarshal(data, &old) == nil {
			if old.UpdateServerURL != "" {
				s.UpdateServerURL = old.UpdateServerURL
			}
			if old.AutoUpdate != nil {
				s.AutoUpdate = *old.AutoUpdate
			}
			s.Administrator = old.Administrator
			s.BetterPDFNames = old.BetterPDFNames
		}
		_ = os.Rename(oldSettingsPath, oldSettingsPath+".migrated")
	}

	// The old session.dat is already encrypted with the exact same
	// build-time key — sessionstore.Load does the decrypt, so this just
	// relocates the result into the new file rather than re-deriving
	// anything.
	if saved, err := sessionstore.Load(); err == nil && saved != nil {
		blob, err := encodeCredentials(Credentials{
			Username:     saved.Username,
			Password:     saved.Password,
			RefreshToken: saved.RefreshToken,
			SavedAt:      saved.SavedAt,
		})
		if err == nil {
			s.EncryptedCredentials = blob
		}
		_ = sessionstore.Clear()
	}

	return s
}

func encodeCredentials(c Credentials) (string, error) {
	plain, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	cipherBytes, err := sessionstore.Protect(plain)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(cipherBytes), nil
}

// LoadCredentials returns the saved login, or (nil, nil) if there isn't one.
func LoadCredentials() (*Credentials, error) {
	s, err := Load()
	if err != nil {
		return nil, err
	}
	if s.EncryptedCredentials == "" {
		return nil, nil
	}
	cipherBytes, err := base64.StdEncoding.DecodeString(s.EncryptedCredentials)
	if err != nil {
		return nil, err
	}
	plain, err := sessionstore.Unprotect(cipherBytes)
	if err != nil {
		return nil, err
	}
	var c Credentials
	if err := json.Unmarshal(plain, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// SaveCredentials encrypts and persists the login, without touching any
// other setting. Call after every successful login or token refresh
// (refresh tokens can rotate).
func SaveCredentials(c Credentials) error {
	c.SavedAt = time.Now()
	blob, err := encodeCredentials(c)
	if err != nil {
		return err
	}
	s, err := Load()
	if err != nil {
		return err
	}
	s.EncryptedCredentials = blob
	return Save(s)
}

// ClearCredentials removes the saved login (explicit logout) without
// touching the rest of the config.
func ClearCredentials() error {
	s, err := Load()
	if err != nil {
		return err
	}
	s.EncryptedCredentials = ""
	return Save(s)
}
