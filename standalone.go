// Two quick actions on the "Neues Gerät" screen that skip GLPI entirely —
// for a technician who just needs a TXT file, or just wants the reports off
// a stick, without going through device creation at all.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"liforra-tool/internal/usbscan"
)

// SaveTextFile opens a native "save as" dialog defaulting to defaultName and
// writes content to wherever the technician picks. Returns "" (no error) if
// the dialog was cancelled.
func (a *App) SaveTextFile(defaultName, content string) (string, error) {
	path, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Textdatei (*.txt)", Pattern: "*.txt"},
		},
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("datei konnte nicht gespeichert werden: %w", err)
	}
	return path, nil
}

// ChooseDestinationDirectory opens a native folder picker. Returns "" (no
// error) if cancelled.
func (a *App) ChooseDestinationDirectory() (string, error) {
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Zielordner auswählen",
	})
}

// CopyFilesResult is the outcome of CopyUSBFilesTo.
type CopyFilesResult struct {
	Copied   int      `json:"copied"`
	Warnings []string `json:"warnings,omitempty"`
}

// CopyUSBFilesTo scans every currently connected removable drive for
// toolstar files (same detection as the device-creation flow, but
// unfiltered — this is a standalone "grab everything off the stick"
// action, not tied to one device) and copies each one found into destDir.
// Returns how many files were copied; a per-file copy failure is collected
// as a warning rather than aborting the rest.
func (a *App) CopyUSBFilesTo(destDir string) (CopyFilesResult, error) {
	drives, err := usbscan.ListRemovableDrives()
	if err != nil {
		return CopyFilesResult{}, fmt.Errorf("usb-sticks konnten nicht gelistet werden: %w", err)
	}
	if len(drives) == 0 {
		return CopyFilesResult{}, fmt.Errorf("kein usb-stick gefunden")
	}

	var all []usbscan.FoundFile
	for _, d := range drives {
		found, err := usbscan.ScanDrive(d.Path)
		if err != nil {
			continue
		}
		all = append(all, found...)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return CopyFilesResult{}, fmt.Errorf("zielordner konnte nicht angelegt werden: %w", err)
	}

	result := CopyFilesResult{}
	used := map[string]bool{}
	for _, f := range all {
		name := filepath.Base(f.Path)
		dest := filepath.Join(destDir, name)
		for n := 2; used[dest]; n++ {
			ext := filepath.Ext(name)
			dest = filepath.Join(destDir, fmt.Sprintf("%s %d%s", name[:len(name)-len(ext)], n, ext))
		}
		used[dest] = true

		if err := copyFile(f.Path, dest); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		result.Copied++
	}
	return result, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
