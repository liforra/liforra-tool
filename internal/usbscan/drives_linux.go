//go:build linux

package usbscan

import (
	"encoding/json"
	"os/exec"
)

// ListRemovableDrives is a dev-only convenience for testing on Linux — the
// shipped app only ever runs on Windows (see project memory), so this uses
// lsblk purely so USB-detection logic can be exercised without a Windows
// box. Not meant to be robust across every Linux distro/desktop.
func ListRemovableDrives() ([]Drive, error) {
	cmd := exec.Command("lsblk", "-J", "-o", "NAME,MOUNTPOINT,RM,LABEL")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var result struct {
		BlockDevices []struct {
			Name       string `json:"name"`
			MountPoint string `json:"mountpoint"`
			RM         bool   `json:"rm"`
			Label      string `json:"label"`
			Children   []struct {
				Name       string `json:"name"`
				MountPoint string `json:"mountpoint"`
				RM         bool   `json:"rm"`
				Label      string `json:"label"`
			} `json:"children"`
		} `json:"blockdevices"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, err
	}

	var drives []Drive
	for _, dev := range result.BlockDevices {
		if !dev.RM {
			continue
		}
		if dev.MountPoint != "" {
			drives = append(drives, Drive{Path: dev.MountPoint, Label: dev.Label})
		}
		for _, child := range dev.Children {
			if child.MountPoint != "" {
				drives = append(drives, Drive{Path: child.MountPoint, Label: child.Label})
			}
		}
	}
	return drives, nil
}

// EjectDrive is a dev-only no-op on Linux — see ListRemovableDrives.
func EjectDrive(path string) error {
	return nil
}
