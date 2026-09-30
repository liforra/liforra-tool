package installlogic

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestInstall_WritesAllFilesAndManifest(t *testing.T) {
	target := filepath.Join(t.TempDir(), "liforra-tool")
	payload := Payload{AppExe: []byte("app exe bytes"), UninstallExe: []byte("uninstall exe bytes")}

	m, err := Install(target, payload, true)
	if err != nil {
		t.Fatal(err)
	}

	appData, err := os.ReadFile(filepath.Join(target, "liforra-tool.exe"))
	if err != nil || string(appData) != "app exe bytes" {
		t.Errorf("liforra-tool.exe = %q, err=%v", appData, err)
	}
	uData, err := os.ReadFile(filepath.Join(target, "uninstall.exe"))
	if err != nil || string(uData) != "uninstall exe bytes" {
		t.Errorf("uninstall.exe = %q, err=%v", uData, err)
	}
	if _, err := os.Stat(filepath.Join(target, ManifestName)); err != nil {
		t.Errorf("manifest not written: %v", err)
	}
	if len(m.Files) != 4 {
		t.Errorf("manifest.Files = %+v, want 4 entries", m.Files)
	}
	if !m.Portable || m.Registered != "" {
		t.Errorf("portable install should have no registry entry: %+v", m)
	}
}

func TestInstall_RefusesNonEmptyUnrelatedDirectory(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "something-else.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Install(target, Payload{AppExe: []byte("a"), UninstallExe: []byte("b")}, true)
	if err == nil {
		t.Fatal("expected an error installing into a non-empty, unrelated directory")
	}
}

func TestInstall_OverwritesAnExistingInstall(t *testing.T) {
	target := t.TempDir()
	if _, err := Install(target, Payload{AppExe: []byte("v1"), UninstallExe: []byte("u1")}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(target, Payload{AppExe: []byte("v2"), UninstallExe: []byte("u2")}, true); err != nil {
		t.Fatalf("re-installing over an existing install should succeed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(target, "liforra-tool.exe"))
	if err != nil || string(data) != "v2" {
		t.Errorf("liforra-tool.exe = %q, want v2", data)
	}
}

func TestUninstall_RemovesEverythingExceptSelf(t *testing.T) {
	target := t.TempDir()
	m, err := Install(target, Payload{AppExe: []byte("a"), UninstallExe: []byte("b")}, true)
	if err != nil {
		t.Fatal(err)
	}
	self := filepath.Join(target, "uninstall.exe")

	if errs := Uninstall(m, self); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}

	if _, err := os.Stat(filepath.Join(target, "liforra-tool.exe")); !os.IsNotExist(err) {
		t.Errorf("liforra-tool.exe should be gone, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "install.exe")); !os.IsNotExist(err) {
		t.Errorf("install.exe should be gone, err=%v", err)
	}
	if _, err := os.Stat(self); err != nil {
		t.Error("uninstall.exe (self) must be left for the caller to remove separately")
	}
}

func TestLoadManifest_RoundTrips(t *testing.T) {
	target := t.TempDir()
	if _, err := Install(target, Payload{AppExe: []byte("a"), UninstallExe: []byte("b")}, true); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(target)
	if err != nil {
		t.Fatal(err)
	}
	if m.InstallDir != target {
		t.Errorf("InstallDir = %q, want %q", m.InstallDir, target)
	}
	if len(m.Files) != 4 {
		t.Errorf("Files = %+v", m.Files)
	}
}

// Real Start Menu shortcut + HKCU registry entry creation and cleanup —
// only meaningful on Windows (see registry_other.go's stubs for why this
// package still builds elsewhere).
func TestInstall_NonPortable_RegistersAndUnregisters(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("registry/shortcut behavior is Windows-only")
	}
	target := t.TempDir()

	m, err := Install(target, Payload{AppExe: []byte("a"), UninstallExe: []byte("b")}, false)
	if err != nil {
		t.Fatal(err)
	}
	if m.Portable || m.Registered == "" {
		t.Fatalf("non-portable install should register: %+v", m)
	}

	key, err := registry.OpenKey(registry.CURRENT_USER, uninstallRegistryPath, registry.QUERY_VALUE)
	if err != nil {
		t.Fatalf("registry key not created: %v", err)
	}
	name, _, err := key.GetStringValue("DisplayName")
	key.Close()
	if err != nil || name != "liforra-tool" {
		t.Errorf("DisplayName = %q, err=%v", name, err)
	}

	var shortcutFound bool
	for _, f := range m.Files {
		if filepath.Ext(f) == ".lnk" {
			shortcutFound = true
			if _, err := os.Stat(f); err != nil {
				t.Errorf("shortcut listed in manifest but missing on disk: %v", err)
			}
		}
	}
	if !shortcutFound {
		t.Error("expected a .lnk shortcut in the manifest's file list")
	}

	self := filepath.Join(target, "uninstall.exe")
	if errs := Uninstall(m, self); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if _, err := registry.OpenKey(registry.CURRENT_USER, uninstallRegistryPath, registry.QUERY_VALUE); err == nil {
		t.Error("registry key should be removed after uninstall")
	}
}
