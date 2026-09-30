//go:build windows

package usbscan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type psVolume struct {
	DriveLetter     string `json:"DriveLetter"`
	FileSystemLabel string `json:"FileSystemLabel"`
	DriveType       string `json:"DriveType"`
}

// ListRemovableDrives asks PowerShell for currently mounted removable
// volumes (i.e. USB sticks), so no extra host dependency beyond what
// Windows already ships.
func ListRemovableDrives() ([]Drive, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		"Get-Volume | Where-Object { $_.DriveType -eq 'Removable' -and $_.DriveLetter } | Select-Object DriveLetter, FileSystemLabel, DriveType | ConvertTo-Json")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	// No removable volume means no output at all, not "[]".
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, nil
	}

	var vols []psVolume
	// ConvertTo-Json returns a single object (not an array) when there's
	// exactly one result, so try both shapes.
	if err := json.Unmarshal(out, &vols); err != nil {
		var single psVolume
		if err2 := json.Unmarshal(out, &single); err2 != nil {
			return nil, err
		}
		vols = []psVolume{single}
	}

	drives := make([]Drive, 0, len(vols))
	for _, v := range vols {
		if v.DriveLetter == "" {
			continue
		}
		drives = append(drives, Drive{
			Path:  v.DriveLetter + `:\`,
			Label: v.FileSystemLabel,
		})
	}
	return drives, nil
}

// EjectDrive safely ejects a removable drive by path (e.g. "F:\") — the
// same "Eject" shell verb as right-click → Eject in File Explorer, so it's
// safe to physically pull the stick right after this returns without
// risking a half-written file.
func EjectDrive(path string) error {
	letter := strings.TrimRight(path, `:\`)
	if letter == "" {
		return fmt.Errorf("no drive letter in %q", path)
	}
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$item = (New-Object -ComObject Shell.Application).Namespace(17).ParseName('%s:')
if (-not $item) { throw "drive %s: not found" }
$item.InvokeVerb('Eject')
`, letter, letter)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
