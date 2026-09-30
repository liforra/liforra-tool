//go:build windows

package installlogic

import (
	"fmt"
	"os"
	"path/filepath"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows/registry"
)

// uninstallRegistryName is both the HKCU uninstall subkey name and (for
// simplicity) the "Registered" marker stored in Manifest — one name to
// find it again by later, either to unregister or to know one exists.
const uninstallRegistryName = "liforra-tool"

const uninstallRegistryPath = `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + uninstallRegistryName

// createStartMenuShortcut adds a per-user (no admin needed) Start Menu
// entry pointing at appExePath, via the WScript.Shell COM automation
// object — the standard, well-documented way to write a .lnk file from Go
// without hand-rolling the IShellLink COM vtable.
func createStartMenuShortcut(appExePath, name string) (string, error) {
	startMenu, err := os.UserConfigDir() // %AppData% on Windows
	if err != nil {
		return "", err
	}
	dir := filepath.Join(startMenu, `Microsoft\Windows\Start Menu\Programs`)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	shortcutPath := filepath.Join(dir, name+".lnk")

	if err := ole.CoInitialize(0); err != nil {
		return "", err
	}
	defer ole.CoUninitialize()

	shell, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return "", fmt.Errorf("WScript.Shell: %w", err)
	}
	defer shell.Release()
	dispatch, err := shell.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return "", err
	}
	defer dispatch.Release()

	csDisp, err := oleutil.CallMethod(dispatch, "CreateShortcut", shortcutPath)
	if err != nil {
		return "", err
	}
	shortcut := csDisp.ToIDispatch()
	defer shortcut.Release()

	if _, err := oleutil.PutProperty(shortcut, "TargetPath", appExePath); err != nil {
		return "", err
	}
	_, _ = oleutil.PutProperty(shortcut, "WorkingDirectory", filepath.Dir(appExePath))
	_, _ = oleutil.PutProperty(shortcut, "Description", "liforra-tool")
	if _, err := oleutil.CallMethod(shortcut, "Save"); err != nil {
		return "", err
	}
	return shortcutPath, nil
}

// registerUninstall adds the standard HKCU uninstall entry so Windows'
// "Apps & Features" lists liforra-tool and offers to run uninstall.exe —
// HKCU (not HKLM), so no admin/elevation is needed, consistent with the
// rest of this tool.
func registerUninstall(installDir, appExePath, uninstallExePath string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallRegistryPath, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer key.Close()

	set := func(name, value string) {
		_ = key.SetStringValue(name, value)
	}
	set("DisplayName", "liforra-tool")
	set("UninstallString", uninstallExePath)
	set("DisplayIcon", appExePath)
	set("InstallLocation", installDir)
	set("Publisher", "liforra.de")
	_ = key.SetDWordValue("NoModify", 1)
	_ = key.SetDWordValue("NoRepair", 1)
	return nil
}

// unregisterUninstall removes the registry entry registerUninstall created.
func unregisterUninstall() error {
	err := registry.DeleteKey(registry.CURRENT_USER, uninstallRegistryPath)
	if err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}
