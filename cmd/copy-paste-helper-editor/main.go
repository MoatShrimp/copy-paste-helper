// Command copy-paste-helper-editor is a Wails + Svelte GUI for creating and
// editing copy-paste-helper's YAML templates. It's a separate process from
// the copy-paste-helper daemon (normally launched from its tray menu), and
// only ever touches files in the templates directory — the running daemon
// picks up saved changes on its own via internal/template.Watch.
package main

import (
	"embed"
	"flag"
	"log"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	templatesDirFlag := flag.String("templates", "templates", "directory containing template YAML files")
	flag.Parse()

	templatesDir, err := filepath.Abs(*templatesDirFlag)
	if err != nil {
		log.Fatalf("resolving templates directory: %v", err)
	}

	app := NewApp(templatesDir)

	err = wails.Run(&options.App{
		Title:  "copy-paste-helper — Templates",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		log.Fatalf("running editor: %v", err)
	}
}
