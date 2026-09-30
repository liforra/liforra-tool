// Command installer is install.exe — embeds one specific version's
// liforra-tool.exe and uninstall.exe (see internal/installlogic), so it
// never needs network access to install: "meant for sharing the program
// not over the network" (project chat). Built with Wails, same as the
// main app, purely so it looks like the same product rather than a random
// generic installer.
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:web
var assets embed.FS

//go:embed payload/liforra-tool.exe
var appExe []byte

//go:embed payload/uninstall.exe
var uninstallExe []byte

func main() {
	// appExe/uninstallExe are only ever read from app.go's DoInstall, which
	// Wails invokes purely through JS-bridge reflection — no Go source calls
	// it directly. Without a real static reference here, the linker proved
	// that reachable and dead-code-eliminated the embedded payload entirely,
	// shipping an install.exe with nothing to install.
	if len(appExe) == 0 || len(uninstallExe) == 0 {
		panic("install.exe was built without its embedded payload")
	}

	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "liforra-tool installieren",
		Width:     460,
		Height:    520,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 15, B: 34, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
