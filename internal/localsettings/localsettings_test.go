package localsettings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"liforra-tool/internal/installlogic"
	"liforra-tool/internal/sessionstore"
)

// dropPortableManifest writes a minimal install manifest next to the
// running test binary's own exe path (os.Executable() inside `go test` is
// the compiled test binary — a real, writable path) and removes it when
// the test ends, so portableDir() has something real to find rather than
// needing os.Executable() itself to be mocked.
func dropPortableManifest(t *testing.T, portable bool) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(filepath.Dir(exe), installlogic.ManifestName)
	data := []byte(`{"installDir":"x","files":[],"portable":` + map[bool]string{true: "true", false: "false"}[portable] + `}`)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(p) })
}

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

func TestPortableDir_FalseWithNoManifest(t *testing.T) {
	// No manifest dropped — the ordinary `go test` case, and the ordinary
	// case for a dev build / `wails build` output run directly.
	if _, ok := portableDir(); ok {
		t.Error("portableDir() = _, true, want false with no install manifest present")
	}
}

func TestPortableDir_FalseWhenManifestSaysNotPortable(t *testing.T) {
	dropPortableManifest(t, false)
	if _, ok := portableDir(); ok {
		t.Error("portableDir() = _, true, want false when the manifest says portable=false")
	}
}

func TestPortableDir_TrueAndConfigLivesNextToExeWhenPortable(t *testing.T) {
	// Deliberately NOT sandboxed via AppData/XDG_CONFIG_HOME — the whole
	// point is confirming this ignores the host's config dir entirely.
	dropPortableManifest(t, true)

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Dir(exe)

	d, ok := portableDir()
	if !ok || d != wantDir {
		t.Fatalf("portableDir() = %q, %v, want %q, true", d, ok, wantDir)
	}

	p, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(p) != wantDir {
		t.Errorf("configPath() = %q, want it inside %q", p, wantDir)
	}
	t.Cleanup(func() { _ = os.Remove(p) })

	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("config.toml was not created next to the exe: %v", err)
	}
}
