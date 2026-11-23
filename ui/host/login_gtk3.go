//go:build linux && gtk3 && cgo

package host

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1
#include <stdint.h>
#include <stdlib.h>
#include <gtk/gtk.h>

GtkWidget *playnux_oauth_window(GtkWindow *parent, const char *title,
    const char *url, const char *script, uintptr_t handle);
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"runtime/cgo"
	"time"
	"unsafe"

	"github.com/playnux/core/auth"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type embeddedLogin struct {
	redirect *url.URL
	result   chan auth.LoginResult
	closed   chan struct{}
	window   *C.GtkWidget // Accessed only on the GTK main thread.
}

func embeddedStoreLogin(ctx context.Context, parent *application.WebviewWindow, req auth.LoginRequest) (auth.LoginResult, error, bool) {
	redirect, err := url.Parse(req.RedirectURLPrefix)
	if err != nil || redirect.Host == "" {
		return auth.LoginResult{}, fmt.Errorf("redirecionamento de login inválido"), true
	}
	login := &embeddedLogin{
		redirect: redirect,
		result:   make(chan auth.LoginResult, 1),
		closed:   make(chan struct{}),
	}
	handle := cgo.NewHandle(login)
	defer handle.Delete()
	title := "Entrar na Epic Games"
	if req.PluginID == "gog" {
		title = "Entrar na GOG"
	}

	// Epic returns an authorization code in the response body. The script
	// reports only that callback page and has no access to app bindings.
	origin, _ := json.Marshal(redirect.Scheme + "://" + redirect.Host)
	path, _ := json.Marshal(redirect.Path)
	script := fmt.Sprintf(`(() => {
		if (location.origin !== %s || location.pathname !== %s) return;
		window.webkit.messageHandlers.playnuxOAuth.postMessage(JSON.stringify({
			url: location.href,
			body: (document.body?.innerText || "").slice(0, 32768)
		}));
	})()`, origin, path)

	application.InvokeSync(func() {
		name, loginURL, bridge := C.CString(title), C.CString(req.URL), C.CString(script)
		defer C.free(unsafe.Pointer(name))
		defer C.free(unsafe.Pointer(loginURL))
		defer C.free(unsafe.Pointer(bridge))
		login.window = C.playnux_oauth_window((*C.GtkWindow)(parent.NativeWindow()), name, loginURL, bridge, C.uintptr_t(handle))
	})
	defer application.InvokeSync(func() {
		if login.window != nil {
			C.gtk_widget_destroy(login.window)
		}
	})
	timeout := time.NewTimer(15 * time.Minute)
	defer timeout.Stop()
	select {
	case result := <-login.result:
		return result, nil, true
	case <-login.closed:
		return auth.LoginResult{}, fmt.Errorf("login cancelado"), true
	case <-ctx.Done():
		return auth.LoginResult{}, ctx.Err(), true
	case <-timeout.C:
		return auth.LoginResult{}, fmt.Errorf("login expirado"), true
	}
}

func (login *embeddedLogin) complete(rawURL, body string) bool {
	current, err := url.Parse(rawURL)
	if err != nil || current.User != nil || current.Scheme != login.redirect.Scheme ||
		current.Host != login.redirect.Host || current.Path != login.redirect.Path {
		return false
	}
	if current.Query().Get("code") == "" {
		var payload struct {
			AuthorizationCode string `json:"authorizationCode"`
		}
		if json.Unmarshal([]byte(body), &payload) != nil || payload.AuthorizationCode == "" {
			return false
		}
	}
	select {
	case login.result <- auth.LoginResult{RedirectURL: rawURL, Body: body}:
	default:
	}
	return true
}

//export playnuxOAuthNavigation
func playnuxOAuthNavigation(handle C.uintptr_t, uri *C.char) C.int {
	login := cgo.Handle(handle).Value().(*embeddedLogin)
	if login.complete(C.GoString(uri), "") {
		return 1
	}
	return 0
}

//export playnuxOAuthMessage
func playnuxOAuthMessage(handle C.uintptr_t, message *C.char) {
	var payload struct{ URL, Body string }
	if err := json.Unmarshal([]byte(C.GoString(message)), &payload); err == nil {
		cgo.Handle(handle).Value().(*embeddedLogin).complete(payload.URL, payload.Body)
	}
}

//export playnuxOAuthClosed
func playnuxOAuthClosed(handle C.uintptr_t) {
	login := cgo.Handle(handle).Value().(*embeddedLogin)
	login.window = nil
	close(login.closed)
}
