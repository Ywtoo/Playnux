//go:build linux && gtk3 && cgo

package host

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

static WebKitWebView *find_webview(GtkWidget *widget) {
    if (WEBKIT_IS_WEB_VIEW(widget)) return WEBKIT_WEB_VIEW(widget);
    if (!GTK_IS_CONTAINER(widget)) return NULL;
    GList *children = gtk_container_get_children(GTK_CONTAINER(widget));
    WebKitWebView *view = NULL;
    for (GList *child = children; child && !view; child = child->next) {
        view = find_webview(GTK_WIDGET(child->data));
    }
    g_list_free(children);
    return view;
}

static gboolean allow_trailer_autoplay(WebKitWebView *view, WebKitPolicyDecision *decision,
                                       WebKitPolicyDecisionType type, gpointer data) {
    if (type != WEBKIT_POLICY_DECISION_TYPE_NAVIGATION_ACTION) return FALSE;
    WebKitNavigationAction *action = webkit_navigation_policy_decision_get_navigation_action(
        WEBKIT_NAVIGATION_POLICY_DECISION(decision));
    const char *uri = webkit_uri_request_get_uri(webkit_navigation_action_get_request(action));
    if (!uri || !g_str_has_prefix(uri, "wails://")) return FALSE;

    WebKitWebsitePolicies *policies = webkit_website_policies_new_with_policies(
        "autoplay", WEBKIT_AUTOPLAY_ALLOW, NULL);
    webkit_policy_decision_use_with_policies(decision, policies);
    g_object_unref(policies);
    return TRUE;
}

static void configure_trailer_playback(GtkWidget *window) {
    WebKitWebView *view = find_webview(window);
    if (!view) return;
    g_signal_connect(view, "decide-policy", G_CALLBACK(allow_trailer_autoplay), NULL);
}
*/
import "C"

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// PrepareTrailerPlayback installs the app's WebKit policy before the library loads.
func PrepareTrailerPlayback(options *application.WebviewWindowOptions) func(*application.WebviewWindow) {
	startURL := options.URL
	options.URL = "about:blank"
	return func(window *application.WebviewWindow) {
		var once sync.Once
		window.OnWindowEvent(events.Linux.WindowLoadFinished, func(*application.WindowEvent) {
			once.Do(func() {
				application.InvokeSync(func() {
					if native := window.NativeWindow(); native != nil {
						C.configure_trailer_playback((*C.GtkWidget)(native))
					}
				})
				window.SetURL(startURL)
			})
		})
	}
}
