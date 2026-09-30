package localsettings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"liforra-tool/internal/sessionstore"
)

// sandboxConfigDir points os.UserConfigDir() (AppData on Windows, XDG_CONFIG_HOME
// elsewhere) at a fresh temp directory, so each test gets its own isolated
// config.toml instead of touching the real one.
func sandboxConfigDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AppData", dir)         // windows
	t.Setenv("XDG_CONFIG_HOME", dir) // linux (dev-only Detect(), see project memory)
}

func TestLoad_CreatesFileWithDefaultsWhenMissing(t *testing.T) {
	sandboxConfigDir(t)

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s != Defaults() {
		t.Errorf("Load() = %+v, want defaults %+v", s, Defaults())
	}

	p, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("config.toml was not created: %v", err)
	}

	// A second Load() must read back exactly what got written, not
	// silently regenerate different defaults.
	s2, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s2 != s {
		t.Errorf("second Load() = %+v, want %+v", s2, s)
	}
}

func TestSaveLoad_RoundTripsSettings(t *testing.T) {
	sandboxConfigDir(t)

	want := Settings{UpdateServerURL: "https://example.test", AutoUpdate: false, Administrator: true, BetterPDFNames: true}
	if err := Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestCredentials_RoundTripAndClear(t *testing.T) {
	sandboxConfigDir(t)

	want := Credentials{Username: "leon.ankert", Password: "hunter2", RefreshToken: "rt-abc123"}
	if err := SaveCredentials(want); err != nil {
		t.Fatal(err)
	}

	got, err := LoadCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("LoadCredentials() = nil, want a value")
	}
	if got.Username != want.Username || got.Password != want.Password || got.RefreshToken != want.RefreshToken {
		t.Errorf("got %+v, want %+v", *got, want)
	}
	if got.SavedAt.IsZero() {
		t.Error("SavedAt was not stamped")
	}

	// Saving credentials must not disturb the rest of the config.
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.UpdateServerURL != Defaults().UpdateServerURL {
		t.Errorf("unrelated setting changed: %+v", s)
	}

	if err := ClearCredentials(); err != nil {
		t.Fatal(err)
	}
	got, err = LoadCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("LoadCredentials() after clear = %+v, want nil", got)
	}
}

func TestLoad_MigratesOldSettingsJSONAndSessionDat(t *testing.T) {
	sandboxConfigDir(t)

	d, err := dir()
	if err != nil {
		t.Fatal(err)
	}

	// A technician had already changed settings under the old JSON file...
	autoUpdate := false
	oldSettings, _ := json.Marshal(struct {
		UpdateServerURL string `json:"updateServerURL"`
		AutoUpdate      *bool  `json:"autoUpdate"`
		Administrator   bool   `json:"administrator"`
		BetterPDFNames  bool   `json:"betterPDFNames"`
	}{UpdateServerURL: "https://old-server.example", AutoUpdate: &autoUpdate, Administrator: true, BetterPDFNames: false})
	if err := os.WriteFile(filepath.Join(d, "settings.json"), oldSettings, 0o600); err != nil {
		t.Fatal(err)
	}

	// ...and was already logged in under the old encrypted session.dat.
	if err := sessionstore.Save(sessionstore.Saved{
		Username: "old.user", Password: "oldpass", RefreshToken: "old-refresh-token", SavedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.UpdateServerURL != "https://old-server.example" || s.AutoUpdate != false || !s.Administrator {
		t.Errorf("settings not migrated: %+v", s)
	}
	if s.EncryptedCredentials == "" {
		t.Fatal("credentials not migrated")
	}

	creds, err := LoadCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if creds == nil || creds.Username != "old.user" || creds.RefreshToken != "old-refresh-token" {
		t.Errorf("migrated credentials = %+v", creds)
	}

	// The old session.dat must be gone (migrated, not duplicated) and the
	// old settings.json renamed aside rather than silently left in place.
	if _, err := os.Stat(filepath.Join(d, "session.dat")); !os.IsNotExist(err) {
		t.Errorf("old session.dat still present: err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(d, "settings.json.migrated")); err != nil {
		t.Errorf("old settings.json was not renamed aside: %v", err)
	}
}

func TestLoad_NoOldFilesMeansPlainDefaults(t *testing.T) {
	sandboxConfigDir(t)

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.EncryptedCredentials != "" {
		t.Errorf("expected no credentials with nothing to migrate, got %q", s.EncryptedCredentials)
	}
	creds, err := LoadCredentials()
	if err != nil {
		t.Fatal(err)
	}
	if creds != nil {
		t.Errorf("LoadCredentials() = %+v, want nil", creds)
	}
}
