//go:build !linux || !gtk3 || !cgo

package host

import "github.com/wailsapp/wails/v3/pkg/application"

func PrepareTrailerPlayback(*application.WebviewWindowOptions) func(*application.WebviewWindow) {
	return func(*application.WebviewWindow) {}
}
