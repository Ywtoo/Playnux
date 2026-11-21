package main

import (
	"context"
	"embed"
	"log"

	"github.com/playnux/ui/host"
	coreapp "github.com/playnux/core/app"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// O frontend compilado pelo Vite é embutido no binário.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	dbPath, err := coreapp.DefaultDBPath()
	if err != nil {
		log.Fatalf("descobrir diretório de dados: %v", err)
	}

	service := host.New(nil)
	core, err := coreapp.New(dbPath, service, service)
	if err != nil {
		log.Fatalf("abrir banco do Playnux em %s: %v", dbPath, err)
	}
	host.SetCore(service, core)

	app := application.New(application.Options{
		Name:        "Playnux",
		Description: "Sua biblioteca de jogos no Linux",
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: core.ServeArtwork(application.AssetFileServerFS(assets)),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	uiSettings, err := core.UISettings(context.Background())
	if err != nil {
		log.Printf("ler preferências de aparência: %v", err)
		uiSettings = coreapp.DefaultUISettings()
	}
	gpuPolicy := application.WebviewGpuPolicyAlways
	if !uiSettings.GPUAcceleration {
		gpuPolicy = application.WebviewGpuPolicyNever
	}

	width, height, minWidth, minHeight := host.FitWindowToScreen(app, 1280, 800, 960, 620)
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:                      "Playnux",
		Width:                      width,
		Height:                     height,
		MinWidth:                   minWidth,
		MinHeight:                  minHeight,
		BackgroundColour:           application.NewRGB(11, 13, 18),
		URL:                        "/",
		DefaultContextMenuDisabled: true,
		Linux: application.LinuxWindow{
			WebviewGpuPolicy: gpuPolicy,
		},
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 42,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
	})
	host.Attach(service, app, window)
	host.RegisterGameMenu(app)

	// A tela só é conhecida depois que o loop GTK inicia. Reaplica o limite no
	// primeiro carregamento para o caso de a janela ter sido criada antes disso.
	window.OnWindowEvent(events.Linux.WindowLoadFinished, func(*application.WindowEvent) {
		width, height, minWidth, minHeight := host.FitWindowToScreen(app, 1280, 800, 960, 620)
		window.SetSize(width, height)
		window.SetMinSize(minWidth, minHeight)
		window.Center()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
