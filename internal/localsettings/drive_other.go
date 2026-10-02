//go:build !windows

package localsettings

// This tool only ever ships for Windows (see project memory) — this stub
// exists purely so the package (and its tests) build on a non-Windows dev
// machine, same convention as internal/hwinfo's detect_linux.go.
func isRemovableDrive(path string) bool {
	return false
}
