package host

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestFitWindowToWorkAreaClampsSizeAndMinimums(t *testing.T) {
	width, height, minWidth, minHeight := fitWindowToWorkArea(
		application.Rect{Width: 1024, Height: 600},
		1280, 800, 960, 620,
	)
	if width != 1000 || height != 576 {
		t.Fatalf("tamanho = %dx%d, quer 1000x576", width, height)
	}
	if minWidth != 960 || minHeight != 576 {
		t.Fatalf("mínimo = %dx%d, quer 960x576", minWidth, minHeight)
	}
}

func TestFitWindowToWorkAreaKeepsNormalWindow(t *testing.T) {
	width, height, minWidth, minHeight := fitWindowToWorkArea(
		application.Rect{Width: 1920, Height: 1080},
		1280, 800, 960, 620,
	)
	if width != 1280 || height != 800 || minWidth != 960 || minHeight != 620 {
		t.Fatalf("tamanho = %dx%d mínimo = %dx%d", width, height, minWidth, minHeight)
	}
}
