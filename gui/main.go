//go:build desktop || bindings

// FolderWatch desktop is a thin Wails adapter. The default Go build deliberately
// excludes native WebView dependencies, keeping CLI/core builds portable.
package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"runtime"

	"github.com/StevenWinsir/FolderWatch/gui/backend"
	"github.com/StevenWinsir/FolderWatch/gui/host"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

var version = "dev"
var commit = "unknown"
var buildDate = "unknown"

func main() {
	info := backend.AppInfo{Version: version, Commit: commit, BuildDate: buildDate}
	facade := backend.New(context.Background(), info)
	desktop := host.New(facade, func(ctx context.Context, event backend.Event) { wruntime.EventsEmit(ctx, event.Name, event) })
	defer desktop.Close()
	api := backend.NewAPI(facade, func() (string, error) {
		ctx := desktop.Context()
		if ctx == nil {
			return "", fmt.Errorf("native folder picker is unavailable while the app is closing")
		}
		return wruntime.OpenDirectoryDialog(ctx, wruntime.OpenDialogOptions{Title: "Choose a folder to watch"})
	})
	appMenu := menu.NewMenu()
	if runtime.GOOS == "darwin" {
		appMenu.Append(menu.AppMenu())
	}
	fileMenu := appMenu.AddSubmenu("File")
	fileMenu.AddText("Quit FolderWatch", keys.CmdOrCtrl("q"), func(_ *menu.CallbackData) {
		if ctx := desktop.Context(); ctx != nil {
			wruntime.Quit(ctx)
		}
	})
	appMenu.Append(menu.EditMenu())
	help := appMenu.AddSubmenu("Help")
	help.AddText("About FolderWatch", nil, func(_ *menu.CallbackData) {
		ctx := desktop.Context()
		if ctx == nil {
			return
		}
		_, _ = wruntime.MessageDialog(ctx, wruntime.MessageDialogOptions{Type: wruntime.InfoDialog, Title: "FolderWatch", Message: fmt.Sprintf("Version %s\nCommit %s\nBuilt %s\n\nLocal folder monitoring. Your files stay on your Mac.", version, commit, buildDate)})
	})
	err := wails.Run(&options.App{
		Title: "FolderWatch", Width: 1040, Height: 720, MinWidth: 680, MinHeight: 480,
		BackgroundColour: options.NewRGB(248, 249, 251),
		AssetServer:      &assetserver.Options{Assets: assets},
		Menu:             appMenu,
		Bind:             []interface{}{api},
		OnStartup:        desktop.Start,
		OnShutdown:       func(context.Context) { desktop.Close() },
		// Default false: WebView fraud scanning must not upload local URLs/content.
		EnableFraudulentWebsiteDetection: false,
	})
	if err != nil {
		desktop.Close()
		log.Fatal(err)
	}
}
