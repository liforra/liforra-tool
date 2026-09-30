// Package buildserver is the private update/download server this app's
// internal/updater package talks to — matches its API exactly (see
// updater.go's package doc: that client's shape was this project's own
// design, never previously exercised against a real server; this is that
// server). Serves the version list/downloads the app itself needs, a
// higher-privilege publish endpoint for releasing a new version, and the
// public-facing download website.
package buildserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"liforra-tool/internal/updater"
)

// Meta is one published version's full record — updater.VersionInfo
// embedded so the exact fields the app's client decodes are guaranteed to
// match, plus the extra zip/installer info only the website needs (the
// app's client ignores unknown JSON fields, so embedding here is safe).
type Meta struct {
	updater.VersionInfo
	HasZip              bool   `json:"hasZip"`
	ZipSizeBytes        int64  `json:"zipSizeBytes,omitempty"`
	ZipHashSHA256       string `json:"zipHashSHA256,omitempty"`
	HasInstaller        bool   `json:"hasInstaller"`
	InstallerSizeBytes  int64  `json:"installerSizeBytes,omitempty"`
	InstallerHashSHA256 string `json:"installerHashSHA256,omitempty"`
}

// Store persists published versions on disk: data/<version>/{liforra-tool.exe,
// liforra-tool.zip, installer.exe, meta.json}.
type Store struct {
	dir string
}

var ErrNotFound = errors.New("version not found")

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) versionDir(version string) string { return filepath.Join(s.dir, version) }
func (s *Store) exePath(version string) string    { return filepath.Join(s.versionDir(version), "liforra-tool.exe") }
func (s *Store) zipPath(version string) string    { return filepath.Join(s.versionDir(version), "liforra-tool.zip") }
func (s *Store) installerPath(version string) string {
	return filepath.Join(s.versionDir(version), "installer.exe")
}
func (s *Store) metaPath(version string) string { return filepath.Join(s.versionDir(version), "meta.json") }

// List returns every published version, newest first — the order the
// app's client relies on (see update.go's runUpdateCheck: position in this
// list IS the "how many versions ahead" count).
func (s *Store) List() ([]Meta, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var all []Meta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := s.readMeta(e.Name())
		if err != nil {
			continue // a half-written/corrupt entry shouldn't break the whole list
		}
		all = append(all, *m)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].PublishedAt.After(all[j].PublishedAt) })
	return all, nil
}

func (s *Store) readMeta(version string) (*Meta, error) {
	data, err := os.ReadFile(s.metaPath(version))
	if err != nil {
		return nil, err
	}
	var m Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Get returns one version's metadata, or ErrNotFound.
func (s *Store) Get(version string) (*Meta, error) {
	m, err := s.readMeta(version)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

// ExePath/ZipPath/InstallerPath expose where a version's files live on
// disk, for the HTTP handlers to serve directly (http.ServeFile).
func (s *Store) ExePath(version string) string       { return s.exePath(version) }
func (s *Store) ZipPath(version string) string        { return s.zipPath(version) }
func (s *Store) InstallerPath(version string) string  { return s.installerPath(version) }

// Publish stores a newly built exe (and, if given, an installer exe —
// install.exe/uninstall.exe aren't built yet, so this is usually nil for
// now) under version, builds the zip bundle itself, and writes meta.json.
// Overwrites any existing publish of the same version.
func (s *Store) Publish(version string, exeData []byte, installerData []byte, changelog string, configChanged bool) (*Meta, error) {
	if version == "" {
		return nil, errors.New("version is required")
	}
	if err := os.MkdirAll(s.versionDir(version), 0o755); err != nil {
		return nil, err
	}

	exeHash := sha256.Sum256(exeData)
	if err := os.WriteFile(s.exePath(version), exeData, 0o644); err != nil {
		return nil, err
	}

	m := Meta{
		VersionInfo: updater.VersionInfo{
			Version:          version,
			PublishedAt:      time.Now().UTC(),
			Changelog:        changelog,
			ConfigChanged:    configChanged,
			BinaryHashSHA256: hex.EncodeToString(exeHash[:]),
			SizeBytes:        int64(len(exeData)),
		},
	}

	if len(installerData) > 0 {
		installerHash := sha256.Sum256(installerData)
		if err := os.WriteFile(s.installerPath(version), installerData, 0o644); err != nil {
			return nil, err
		}
		m.HasInstaller = true
		m.InstallerSizeBytes = int64(len(installerData))
		m.InstallerHashSHA256 = hex.EncodeToString(installerHash[:])
	}

	zipData, err := buildZip(version, exeData, installerData)
	if err != nil {
		return nil, err
	}
	zipHash := sha256.Sum256(zipData)
	if err := os.WriteFile(s.zipPath(version), zipData, 0o644); err != nil {
		return nil, err
	}
	m.HasZip = true
	m.ZipSizeBytes = int64(len(zipData))
	m.ZipHashSHA256 = hex.EncodeToString(zipHash[:])

	metaData, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.metaPath(version), metaData, 0o644); err != nil {
		return nil, err
	}
	return &m, nil
}

// Delete removes a published version entirely — for pulling a bad release.
func (s *Store) Delete(version string) error {
	err := os.RemoveAll(s.versionDir(version))
	if err != nil {
		return err
	}
	return nil
}
