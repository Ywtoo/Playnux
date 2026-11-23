//go:build linux && gtk3 && cgo

#include <stdint.h>
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include "_cgo_export.h"

static gboolean on_policy(WebKitWebView *view, WebKitPolicyDecision *decision,
                           WebKitPolicyDecisionType type, gpointer handle) {
    WebKitURIRequest *request = NULL;
    if (type == WEBKIT_POLICY_DECISION_TYPE_NAVIGATION_ACTION) {
        WebKitNavigationAction *action = webkit_navigation_policy_decision_get_navigation_action(WEBKIT_NAVIGATION_POLICY_DECISION(decision));
        request = webkit_navigation_action_get_request(action);
    } else if (type == WEBKIT_POLICY_DECISION_TYPE_RESPONSE) {
        request = webkit_response_policy_decision_get_request(WEBKIT_RESPONSE_POLICY_DECISION(decision));
    }
    if (request && playnuxOAuthNavigation((uintptr_t) handle, (char *) webkit_uri_request_get_uri(request))) {
        webkit_policy_decision_ignore(decision);
        return TRUE;
    }
    return FALSE;
}

static void on_load(WebKitWebView *view, WebKitLoadEvent event, gpointer handle) {
    if (event == WEBKIT_LOAD_STARTED || event == WEBKIT_LOAD_REDIRECTED || event == WEBKIT_LOAD_COMMITTED) {
        const char *uri = webkit_web_view_get_uri(view);
        if (uri) {
            playnuxOAuthNavigation((uintptr_t) handle, (char *) uri);
        }
    }
}

static void on_message(WebKitUserContentManager *manager, WebKitJavascriptResult *message, gpointer handle) {
    char *json = jsc_value_to_string(webkit_javascript_result_get_js_value(message));
    if (json) {
        playnuxOAuthMessage((uintptr_t) handle, json);
        g_free(json);
    }
}

static void on_root_destroy(GtkWidget *window, gpointer handle) {
    WebKitUserContentManager *manager = g_object_get_data(G_OBJECT(window), "playnux-oauth-manager");
    g_signal_handlers_disconnect_by_data(manager, handle);
    playnuxOAuthClosed((uintptr_t) handle);
}

static GtkWidget *login_window(GtkWindow *parent, const char *title) {
    GtkWidget *window = gtk_window_new(GTK_WINDOW_TOPLEVEL);
    gtk_window_set_title(GTK_WINDOW(window), title);
    int width = 920;
    int height = 700;
    GdkDisplay *display = gtk_widget_get_display(window);
    GdkWindow *parent_window = parent ? gtk_widget_get_window(GTK_WIDGET(parent)) : NULL;
    GdkMonitor *monitor = parent_window ? gdk_display_get_monitor_at_window(display, parent_window)
                                        : gdk_display_get_primary_monitor(display);
    if (monitor) {
        GdkRectangle workarea;
        gdk_monitor_get_workarea(monitor, &workarea);
        width = MIN(width, MAX(1, workarea.width - 48));
        height = MIN(height, MAX(1, workarea.height - 64));
    }
    gtk_window_set_default_size(GTK_WINDOW(window), width, height);
    if (parent) {
        gtk_window_set_transient_for(GTK_WINDOW(window), parent);
        gtk_window_set_destroy_with_parent(GTK_WINDOW(window), TRUE);
        gtk_window_set_position(GTK_WINDOW(window), GTK_WIN_POS_CENTER_ON_PARENT);
    }
    gtk_window_set_modal(GTK_WINDOW(window), TRUE);
    return window;
}

static void present_popup(WebKitWebView *view, GtkWidget *window) {
    gtk_widget_show_all(window);
    gtk_window_present(GTK_WINDOW(window));
}

static void attach_view(GtkWidget *window, WebKitWebView *view, gpointer handle);

static GtkWidget *on_popup(WebKitWebView *parent_view, WebKitNavigationAction *action, gpointer handle) {
    GtkWindow *parent = GTK_WINDOW(gtk_widget_get_toplevel(GTK_WIDGET(parent_view)));
    GtkWidget *window = login_window(parent, gtk_window_get_title(parent));
    WebKitWebView *view = g_object_new(WEBKIT_TYPE_WEB_VIEW,
        "related-view", parent_view,
        "user-content-manager", webkit_web_view_get_user_content_manager(parent_view),
        NULL);
    attach_view(window, view, handle);
    g_signal_connect(view, "ready-to-show", G_CALLBACK(present_popup), window);
    return GTK_WIDGET(view);
}

static void on_view_destroy(GtkWidget *view, gpointer handle) {
    g_signal_handlers_disconnect_by_data(view, handle);
}

static void attach_view(GtkWidget *window, WebKitWebView *view, gpointer handle) {
    gtk_container_add(GTK_CONTAINER(window), GTK_WIDGET(view));
    g_signal_connect(view, "decide-policy", G_CALLBACK(on_policy), handle);
    g_signal_connect(view, "load-changed", G_CALLBACK(on_load), handle);
    g_signal_connect(view, "create", G_CALLBACK(on_popup), handle);
    g_signal_connect(view, "destroy", G_CALLBACK(on_view_destroy), handle);
    g_signal_connect_swapped(view, "close", G_CALLBACK(gtk_widget_destroy), window);
}

GtkWidget *playnux_oauth_window(GtkWindow *parent, const char *title, const char *url,
                                const char *script, uintptr_t handle) {
    GtkWidget *window = login_window(parent, title);
    WebKitWebContext *context = webkit_web_context_new_ephemeral();
    WebKitUserContentManager *manager = webkit_user_content_manager_new();
    gpointer data = (gpointer) handle;

    g_signal_connect(manager, "script-message-received::playnuxOAuth", G_CALLBACK(on_message), data);
    webkit_user_content_manager_register_script_message_handler(manager, "playnuxOAuth");
    WebKitUserScript *hook = webkit_user_script_new(script, WEBKIT_USER_CONTENT_INJECT_TOP_FRAME,
        WEBKIT_USER_SCRIPT_INJECT_AT_DOCUMENT_END, NULL, NULL);
    webkit_user_content_manager_add_script(manager, hook);
    webkit_user_script_unref(hook);

    WebKitWebView *view = g_object_new(WEBKIT_TYPE_WEB_VIEW,
        "web-context", context, "user-content-manager", manager, NULL);
    g_object_unref(context);
    g_object_set_data_full(G_OBJECT(window), "playnux-oauth-manager", manager, g_object_unref);
    g_signal_connect(window, "destroy", G_CALLBACK(on_root_destroy), data);
    attach_view(window, view, data);
    webkit_web_view_load_uri(view, url);
    gtk_widget_show_all(window);
    gtk_window_present(GTK_WINDOW(window));
    return window;
}
