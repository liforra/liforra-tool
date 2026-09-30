package main

import (
	"context"
	"os"
	"path/filepath"

	"liforra-tool/internal/installlogic"
	"liforra-tool/internal/usbscan"
)

// Target is one place install.exe could put the program — either the one
// "this computer" entry or one detected removable drive.
type Target struct {
	Path            string `json:"path"`
	Label           string `json:"label"`
	IsRemovable     bool   `json:"isRemovable"`
	// HasPortableApps flags a drive that already has a "PortableApps"
	// folder at its root — a strong signal this stick is already used to
	// carry installed portable software, so it's worth pre-selecting/
	// highlighting rather than making the technician hunt for the right
	// one among several plugged in.
	HasPortableApps bool `json:"hasPortableApps"`
}

type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// ListTargets checks every currently connected removable drive (not just
// the first one — a technician's stick is sometimes the second or third
// partition Windows enumerates, same lesson learned in usbscan) plus the
// one "install onto this computer" option.
func (a *App) ListTargets() ([]Target, error) {
	targets := []Target{{
		Path:  filepath.Join(userProgramsDir(), "liforra-tool"),
		Label: "Dieser PC",
	}}

	drives, err := usbscan.ListRemovableDrives()
	if err != nil {
		return targets, nil // a scan failure still leaves "this computer" usable
	}
	for _, d := range drives {
		_, err := os.Stat(filepath.Join(d.Path, "PortableApps"))
		hasPortableApps := err == nil
		// Drives already used the PortableApps.com way install alongside the
		// other portable apps there (X:\PortableApps\<name>), not loose at
		// the drive root — that's the whole point of pre-flagging them.
		installPath := filepath.Join(d.Path, "liforra-tool")
		if hasPortableApps {
			installPath = filepath.Join(d.Path, "PortableApps", "liforra-tool")
		}
		targets = append(targets, Target{
			Path:            installPath,
			Label:           labelFor(d),
			IsRemovable:     true,
			HasPortableApps: hasPortableApps,
		})
	}
	return targets, nil
}

func labelFor(d usbscan.Drive) string {
	if d.Label != "" {
		return d.Path + " (" + d.Label + ")"
	}
	return d.Path
}

func userProgramsDir() string {
	d, err := os.UserCacheDir() // %LocalAppData% on Windows
	if err != nil {
		return `C:\Program Files\liforra-tool`
	}
	return filepath.Join(d, "Programs")
}

// InstallResult is what the frontend shows once DoInstall finishes.
type InstallResult struct {
	InstallDir string `json:"installDir"`
}

// DoInstall writes the embedded payload (this exact install.exe's own
// version of liforra-tool.exe, plus uninstall.exe) into targetDir. portable
// mirrors the UI's pre-checked "Portable" checkbox — true skips the Start
// Menu shortcut and registry uninstall entry entirely.
func (a *App) DoInstall(targetDir string, portable bool) (*InstallResult, error) {
	m, err := installlogic.Install(targetDir, installlogic.Payload{
		AppExe:       appExe,
		UninstallExe: uninstallExe,
	}, portable)
	if err != nil {
		return nil, err
	}
	return &InstallResult{InstallDir: m.InstallDir}, nil
}
