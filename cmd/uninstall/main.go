//go:build windows

// Command uninstall removes a liforra-tool install — installed alongside
// liforra-tool.exe/install.exe by install.exe (see internal/installlogic
// and ../../installer). No GUI framework needed for this one: a single
// native confirm dialog, then delete everything the manifest lists.
//
// Deleting itself needs Windows' well-known trick (a running exe can't
// remove its own file — the handle is still open): spawn a detached,
// hidden helper that waits a moment for this process to exit and then
// deletes it, and exit immediately rather than wait around for that.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"liforra-tool/internal/installlogic"
)

var (
	user32          = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")
)

const (
	mbYesNo        = 0x00000004
	mbIconQuestion = 0x00000020
	mbIconError    = 0x00000010
	mbIconInfo     = 0x00000040
	idYes          = 6
)

func messageBox(text, caption string, flags uintptr) uintptr {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(caption)
	ret, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), flags)
	return ret
}

func main() {
	self, err := os.Executable()
	if err != nil {
		messageBox("Eigener Pfad konnte nicht ermittelt werden: "+err.Error(), "liforra-tool deinstallieren", mbIconError)
		os.Exit(1)
	}
	self, _ = filepath.EvalSymlinks(self)
	dir := filepath.Dir(self)

	manifest, err := installlogic.LoadManifest(dir)
	if err != nil {
		messageBox("Kein liforra-tool-Installations-Manifest in diesem Ordner gefunden:\n"+dir, "liforra-tool deinstallieren", mbIconError)
		os.Exit(1)
	}

	if messageBox("liforra-tool wird aus diesem Ordner entfernt:\n"+dir+"\n\nFortfahren?", "liforra-tool deinstallieren", mbYesNo|mbIconQuestion) != idYes {
		os.Exit(0)
	}

	errs := installlogic.Uninstall(manifest, self)
	if len(errs) > 0 {
		msg := "Einige Dateien konnten nicht entfernt werden:\n"
		for _, e := range errs {
			msg += "\n" + e.Error()
		}
		messageBox(msg, "liforra-tool deinstallieren", mbIconError)
	}

	scheduleSelfDelete(self)
	messageBox("liforra-tool wurde entfernt.", "liforra-tool deinstallieren", mbIconInfo)
}

// scheduleSelfDelete spawns a detached, hidden cmd.exe that waits briefly
// (long enough for this process to exit and release its file handle) and
// then deletes self. Errors are ignored — worst case, a leftover
// uninstall.exe sits in an otherwise-empty, already-removed folder, which
// is harmless.
func scheduleSelfDelete(self string) {
	cmd := exec.Command("cmd", "/C", fmt.Sprintf(`ping 127.0.0.1 -n 2 >nul & del "%s"`, self))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008 /* DETACHED_PROCESS */}
	_ = cmd.Start()
}
