//go:build windows

package localsettings

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// isRemovableDrive reports whether path sits on a removable drive (a USB
// stick) — fast, no subprocess, since this runs on every config access via
// portableDir(). GetDriveType needs a drive root ("D:\\"), not an arbitrary
// path.
func isRemovableDrive(path string) bool {
	vol := filepath.VolumeName(path)
	if vol == "" {
		return false
	}
	root, err := windows.UTF16PtrFromString(vol + `\`)
	if err != nil {
		return false
	}
	return windows.GetDriveType(root) == windows.DRIVE_REMOVABLE
}
