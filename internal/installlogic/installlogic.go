// Package installlogic is the actual file-level work behind install.exe/
// uninstall.exe — kept separate from both GUI/CLI wrappers so it's testable
// without needing to drive an actual window (installer/ has no headless
// test story) or hand-verify a console prompt.
//
// install.exe embeds a specific version's liforra-tool.exe (plus a
// standalone uninstall.exe) via go:embed at build time, so it never needs
// network access — "meant for sharing the program not over the network"
// (see project chat). It writes a manifest.json alongside the installed
// files; uninstall.exe reads that manifest to know exactly what it's
// responsible for removing, including itself.
package installlogic

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const ManifestName = "liforra-tool-install.json"

// Manifest lists every file this install put down, so Uninstall can remove
// precisely those and nothing else — never a wildcard delete of the
// install directory, in case the technician later added other files there.
type Manifest struct {
	InstallDir string   `json:"installDir"`
	Files      []string `json:"files"` // absolute paths, includes the manifest's own eventual location's siblings — and the Start Menu shortcut, if any
	// Portable, when true (the default the UI pre-checks), means this
	// install never touched the registry or Start Menu — pure files, safe
	// to delete by hand without an uninstaller at all. When false, a
	// shortcut and an HKCU uninstall registry entry (so Windows' "Apps &
	// Features" lists it) were created too, and Uninstall removes both.
	Portable bool `json:"portable"`
	// Registered is the HKCU uninstall registry subkey name to remove on
	// uninstall — empty when Portable (nothing was ever registered).
	Registered string `json:"registered,omitempty"`
}

// Payload is what install.exe has embedded — the exact bytes of one
// version's liforra-tool.exe and of uninstall.exe.
type Payload struct {
	AppExe       []byte
	UninstallExe []byte
}

// Install copies the payload into targetDir (created if missing) and
// writes the manifest uninstall.exe will later read. Refuses to run
// against a non-empty directory that isn't already a liforra-tool install
// (no manifest present) — so it can't silently dump files into something
// unrelated a technician pointed it at by mistake. portable controls
// whether a Start Menu shortcut + registry uninstall entry get created
// (see Manifest.Portable).
func Install(targetDir string, payload Payload, portable bool) (*Manifest, error) {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, fmt.Errorf("Zielordner konnte nicht angelegt werden: %w", err)
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return nil, err
	}
	if len(entries) > 0 {
		if _, err := os.Stat(filepath.Join(targetDir, ManifestName)); err != nil {
			return nil, fmt.Errorf("Zielordner ist nicht leer und keine vorhandene Installation (%s fehlt)", ManifestName)
		}
		// Already a liforra-tool install — overwrite in place (an update
		// via this same install.exe), not an error.
	}

	appPath := filepath.Join(targetDir, "liforra-tool.exe")
	uninstallPath := filepath.Join(targetDir, "uninstall.exe")
	installPath := filepath.Join(targetDir, "install.exe")

	if err := os.WriteFile(appPath, payload.AppExe, 0o755); err != nil {
		return nil, fmt.Errorf("liforra-tool.exe konnte nicht geschrieben werden: %w", err)
	}
	if err := os.WriteFile(uninstallPath, payload.UninstallExe, 0o755); err != nil {
		return nil, fmt.Errorf("uninstall.exe konnte nicht geschrieben werden: %w", err)
	}
	// install.exe alongside the rest — a copy of the running installer
	// itself, so the install can be repaired/repeated later without
	// needing the original download again.
	self, err := os.Executable()
	if err == nil {
		if data, err := os.ReadFile(self); err == nil {
			_ = os.WriteFile(installPath, data, 0o755) // best-effort; not fatal if this one copy fails
		}
	}

	m := &Manifest{
		InstallDir: targetDir,
		Files:      []string{appPath, uninstallPath, installPath, filepath.Join(targetDir, ManifestName)},
		Portable:   portable,
	}

	if !portable {
		// Best-effort, same spirit as the install.exe self-copy above — a
		// technician still gets a working portable install even if a
		// shortcut or the registry entry couldn't be created for some
		// reason (e.g. a locked-down Group Policy), rather than failing
		// the whole install over a nice-to-have.
		if shortcutPath, err := createStartMenuShortcut(appPath, "liforra-tool"); err == nil {
			m.Files = append(m.Files, shortcutPath)
		}
		if err := registerUninstall(targetDir, appPath, uninstallPath); err == nil {
			m.Registered = uninstallRegistryName
		}
	}

	manifestData, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(targetDir, ManifestName), manifestData, 0o644); err != nil {
		return nil, fmt.Errorf("Installations-Manifest konnte nicht geschrieben werden: %w", err)
	}
	return m, nil
}

// LoadManifest reads the manifest from dir (uninstall.exe's own directory).
func LoadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Uninstall removes every file the manifest lists except selfPath (the
// running uninstall.exe can't delete itself directly on Windows — see
// cmd/uninstall, which handles that separately after this returns), plus
// the registry uninstall entry if one was created.
func Uninstall(m *Manifest, selfPath string) []error {
	var errs []error
	for _, f := range m.Files {
		if samePath(f, selfPath) {
			continue
		}
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("%s: %w", f, err))
		}
	}
	if m.Registered != "" {
		if err := unregisterUninstall(); err != nil {
			errs = append(errs, fmt.Errorf("Registry-Eintrag: %w", err))
		}
	}
	// The directory itself, if now empty.
	_ = os.Remove(m.InstallDir)
	return errs
}

func samePath(a, b string) bool {
	ca, err1 := filepath.Abs(a)
	cb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	return ca == cb
}
