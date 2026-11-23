package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"

	coreapp "github.com/playnux/core/app"
	"github.com/playnux/ui/host"
	"github.com/playnux/ui/internal/release"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// O frontend compilado pelo Vite é embutido no binário.
//
//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/window-icon.png
var appIcon []byte

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("Playnux " + release.Version())
		return
	}
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

	programName := os.Getenv("PLAYNUX_RUN_DESKTOP_ID")
	if programName == "" {
		programName = "playnux"
	}
	app := application.New(application.Options{
		Name:        "Playnux",
		Description: "Your game library on Linux",
		Icon:        appIcon,
		Linux: application.LinuxOptions{
			ProgramName: programName,
		},
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
	options := application.WebviewWindowOptions{
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
	}
	preparePlayback := host.PrepareTrailerPlayback(&options)
	window := app.Window.NewWithOptions(options)
	preparePlayback(window)
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
	if err := host.RelaunchAfterShutdown(service); err != nil {
		log.Printf("could not restart Playnux after updating: %v", err)
	}
}
