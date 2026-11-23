//go:build !linux || !gtk3 || !cgo

package host

import (
	"context"

	"github.com/playnux/core/auth"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func embeddedStoreLogin(context.Context, *application.WebviewWindow, auth.LoginRequest) (auth.LoginResult, error, bool) {
	return auth.LoginResult{}, nil, false
}
