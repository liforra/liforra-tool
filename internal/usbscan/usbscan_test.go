package usbscan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A technician saving toolstar's output into a subfolder (rather than the
// stick's root) must not make the app silently report it as missing.
func TestScanDrive_FindsFilesInSubfolders(t *testing.T) {
	root := t.TempDir()

	sub := filepath.Join(root, "Berichte", "2026-09-29")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	report := "toolstar Start: 2026-09-29\nGesamtergebnis  bestanden\n"
	if err := os.WriteFile(filepath.Join(sub, "test.txt"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := ScanDrive(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("found %d files, want 1: %+v", len(found), found)
	}
	if found[0].Kind != KindTestReport {
		t.Errorf("kind = %q, want %q", found[0].Kind, KindTestReport)
	}
}

// The real layout: toolstar's own "testlx" folder at the stick's root,
// results nested anywhere inside it.
func TestScanDrive_ScopesToTestlxFolderWhenPresent(t *testing.T) {
	root := t.TempDir()

	sub := filepath.Join(root, "testlx", "2026-09-29", "run1")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	report := "toolstar Start: 2026-09-29\nGesamtergebnis  bestanden\n"
	if err := os.WriteFile(filepath.Join(sub, "test.txt"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}

	// Content outside testlx must not be picked up once that folder exists
	// — this is what makes the scoping worth doing (skip everything else
	// on the stick, e.g. a Ventoy partition's ISOs).
	if err := os.WriteFile(filepath.Join(root, "stray.txt"), []byte("Gesamtergebnis  bestanden\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := ScanDrive(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("found %d files, want 1: %+v", len(found), found)
	}
	if !strings.Contains(found[0].Path, "testlx") {
		t.Errorf("found file outside testlx: %+v", found[0])
	}
}

func TestScanDrive_SkipsSystemDirs(t *testing.T) {
	root := t.TempDir()

	sysDir := filepath.Join(root, "System Volume Information")
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A file that would classify as a report, but must not be reached
	// because the containing directory is skipped.
	if err := os.WriteFile(filepath.Join(sysDir, "test.txt"), []byte("Gesamtergebnis  bestanden\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := ScanDrive(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("found %+v, want none (System Volume Information should be skipped)", found)
	}
}

func TestScanDrive_IgnoresUnrelatedExtensions(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "readme.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, err := ScanDrive(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("found %+v, want none", found)
	}
}
