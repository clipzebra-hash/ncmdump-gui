package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	"git.taurusxin.com/taurusxin/ncmdump-gui/utils"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()

	config_manager, err := utils.NewConfigManager("config.json")
	if err != nil {
		return
	}

	// Create application with options
	err = wails.Run(&options.App{
		Title:  "ncmdump-gui · 离线优化版 · SMB修复",
		Width:  750,
		Height: 500,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
			config_manager,
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
		},
		Mac: &mac.Options{
			About: &mac.AboutInfo{
				Title:   "ncmdump-gui",
				Message: "Copyright © 2025 TaurusXin",
			},
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
