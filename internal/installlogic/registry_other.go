//go:build !windows

package installlogic

import "errors"

// This tool only ever ships for Windows (see project memory) — these
// stubs exist purely so the package (and its tests) build on a non-Windows
// dev machine, same convention as internal/hwinfo's detect_linux.go.
const uninstallRegistryName = "liforra-tool"

func createStartMenuShortcut(appExePath, name string) (string, error) {
	return "", errors.New("Start Menu shortcuts are only supported on Windows")
}

func registerUninstall(installDir, appExePath, uninstallExePath string) error {
	return errors.New("registry uninstall entries are only supported on Windows")
}

func unregisterUninstall() error {
	return nil
}
