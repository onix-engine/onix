//go:build linux && !android && !onix_drm

#include "webview_linux.h"

#include <dlfcn.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef int gboolean;
typedef long gssize;
typedef unsigned long gulong;

typedef struct {
    double red;
    double green;
    double blue;
    double alpha;
} OnixGdkRGBA;

typedef gboolean (*OnixGSourceFunc)(void *data);

extern void goOnixLinuxWebViewCreated(uintptr_t handle, uintptr_t view, const char *error);
extern void goOnixLinuxWebViewMessage(uintptr_t handle, const char *message);
extern void goOnixLinuxWebViewClosed(uintptr_t handle);
extern void goOnixLinuxWebViewDestroyed(uintptr_t completion);

typedef struct {
    gboolean (*gtk_init_check)(int *argc, char ***argv);
    void *(*gtk_plug_new)(unsigned long socket_id);
    void (*gtk_window_set_default_size)(void *window, int width, int height);
    void (*gtk_window_resize)(void *window, int width, int height);
    void (*gtk_window_set_decorated)(void *window, gboolean decorated);
    void (*gtk_window_set_skip_taskbar_hint)(void *window, gboolean setting);
    void (*gtk_container_add)(void *container, void *widget);
    void (*gtk_widget_set_size_request)(void *widget, int width, int height);
    void (*gtk_widget_realize)(void *widget);
    void *(*gtk_widget_get_window)(void *widget);
    void (*gtk_widget_show_all)(void *widget);
    void (*gtk_widget_hide)(void *widget);
    void (*gtk_widget_destroy)(void *widget);
    void (*gdk_set_allowed_backends)(const char *backends);
    void *(*gdk_window_get_display)(void *window);
    void *(*gdk_x11_display_get_xdisplay)(void *display);
    unsigned long (*gdk_x11_window_get_xid)(void *window);
    void (*gdk_window_move_resize)(void *window, int x, int y, int width, int height);
    int (*x_query_tree)(void *display, unsigned long window, unsigned long *root, unsigned long *parent, unsigned long **children, unsigned int *count);
    int (*x_free)(void *data);
    int (*x_map_window)(void *display, unsigned long window);
    int (*x_unmap_window)(void *display, unsigned long window);
    int (*x_flush)(void *display);

    void *(*webkit_user_content_manager_new)(void);
    gboolean (*webkit_user_content_manager_register_script_message_handler)(void *manager, const char *name);
    void (*webkit_user_content_manager_unregister_script_message_handler)(void *manager, const char *name);
    void (*webkit_user_content_manager_add_script)(void *manager, void *script);
    void *(*webkit_user_script_new)(const char *source, int frames, int time, const char *const *allow_list, const char *const *block_list);
    void (*webkit_user_script_unref)(void *script);
    void *(*webkit_web_view_new_with_user_content_manager)(void *manager);
    void (*webkit_web_view_load_uri)(void *webview, const char *url);
    void (*webkit_web_view_load_html)(void *webview, const char *html, const char *base_uri);
    void *(*webkit_web_view_get_settings)(void *webview);
    void (*webkit_settings_set_enable_developer_extras)(void *settings, gboolean enabled);
    void (*webkit_settings_set_enable_write_console_messages_to_stdout)(void *settings, gboolean enabled);
    void (*webkit_web_view_set_background_color)(void *webview, const OnixGdkRGBA *color);
    void (*webkit_web_view_evaluate_javascript)(void *webview, const char *script, gssize length, const char *world_name, const char *source_uri, void *cancellable, void *callback, void *user_data);
    void (*webkit_web_view_run_javascript)(void *webview, const char *script, void *cancellable, void *callback, void *user_data);
    void *(*webkit_javascript_result_get_js_value)(void *result);
    char *(*jsc_value_to_string)(void *value);

    gulong (*g_signal_connect_data)(void *instance, const char *signal, void (*handler)(void), void *data, void *destroy_data, int flags);
    void (*g_signal_handler_disconnect)(void *instance, gulong handler_id);
    void (*g_object_unref)(void *object);
    void (*g_free)(void *memory);
    void *(*g_main_loop_new)(void *context, gboolean running);
    void (*g_main_loop_run)(void *loop);
    void (*g_main_loop_quit)(void *loop);
    void (*g_main_loop_unref)(void *loop);
    void (*g_main_context_invoke)(void *context, OnixGSourceFunc function, void *data);
} OnixLinuxWebViewAPI;

typedef struct OnixLinuxWebView {
    uintptr_t handle;
    uintptr_t parent;
    void *window;
    void *webview;
    void *manager;
    void *gdk_window;
    void *xdisplay;
    unsigned long xid;
    gulong window_destroy_handler;
    gulong message_handler;
    int owns_manager;
    int destroying;
} OnixLinuxWebView;

typedef struct {
    OnixLinuxWebView *view;
    uintptr_t handle;
    uintptr_t parent;
    uintptr_t completion;
    int debug;
    int visible;
    int transparent;
    double x;
    double y;
    double width;
    double height;
    char *script;
    char *url;
    char *html;
} OnixLinuxWebViewOperation;

static OnixLinuxWebViewAPI onix_webview_api;
static void *onix_webkit_library;
static void *onix_gtk_library;
static void *onix_main_loop;
static const char *onix_webkit_library_name;
static _Thread_local char onix_webview_error[512];
static int onix_webview_loaded;

static const char *onix_set_error(const char *message) {
    snprintf(onix_webview_error, sizeof(onix_webview_error), "window: %s", message);
    return onix_webview_error;
}

static const char *onix_set_symbol_error(const char *symbol) {
    snprintf(onix_webview_error, sizeof(onix_webview_error), "window: WebKitGTK runtime is missing symbol %s", symbol);
    return onix_webview_error;
}

static void *onix_open_first(const char *const *names, const char **selected) {
    for (size_t index = 0; names[index] != NULL; index++) {
        void *library = dlopen(names[index], RTLD_NOW | RTLD_GLOBAL);
        if (library != NULL) {
            if (selected != NULL) *selected = names[index];
            return library;
        }
    }
    return NULL;
}

static void *onix_symbol(void *library, const char *name) {
    void *symbol = dlsym(library, name);
    if (symbol == NULL && library != RTLD_DEFAULT) symbol = dlsym(RTLD_DEFAULT, name);
    return symbol;
}

#define ONIX_LOAD_REQUIRED(library, field, name)                                  \
    do {                                                                          \
        *(void **)(&onix_webview_api.field) = onix_symbol((library), (name));       \
        if (onix_webview_api.field == NULL) return onix_set_symbol_error((name));   \
    } while (0)

#define ONIX_LOAD_OPTIONAL(library, field, name) \
    *(void **)(&onix_webview_api.field) = onix_symbol((library), (name))

const char *onix_linux_webview_load(void) {
    if (onix_webview_loaded) return NULL;
    // Shared-memory transport avoids DMA-BUF stalls on drivers such as
    // vmwgfx while preserving WebKit's accelerated backing store.
    setenv("WEBKIT_DMABUF_RENDERER_FORCE_SHM", "1", 0);
    static const char *const webkit_names[] = {
        "libwebkit2gtk-4.1.so.0",
        "libwebkit2gtk-4.1.so",
        "libwebkit2gtk-4.0.so.37",
        "libwebkit2gtk-4.0.so",
        NULL,
    };
    static const char *const gtk_names[] = {
        "libgtk-3.so.0",
        "libgtk-3.so",
        NULL,
    };
    onix_webkit_library = onix_open_first(webkit_names, &onix_webkit_library_name);
    if (onix_webkit_library == NULL) {
        return onix_set_error("no compatible WebKitGTK runtime found (supported ABIs: 4.0, 4.1)");
    }
    onix_gtk_library = onix_open_first(gtk_names, NULL);
    if (onix_gtk_library == NULL) return onix_set_error("WebKitGTK requires the GTK 3 runtime");

    ONIX_LOAD_REQUIRED(onix_gtk_library, gtk_init_check, "gtk_init_check");
    ONIX_LOAD_REQUIRED(onix_gtk_library, gtk_plug_new, "gtk_plug_new");
    ONIX_LOAD_REQUIRED(onix_gtk_library, gtk_window_set_default_size, "gtk_window_set_default_size");
    ONIX_LOAD_REQUIRED(onix_gtk_library, gtk_window_resize, "gtk_window_resize");
    ONIX_LOAD_REQUIRED(onix_gtk_library, gtk_window_set_decorated, "gtk_window_set_decorated");
    ONIX_LOAD_REQUIRED(onix_gtk_library, gtk_window_set_skip_taskbar_hint, "gtk_window_set_skip_taskbar_hint");
    ONIX_LOAD_REQUIRED(onix_gtk_library, gtk_container_add, "gtk_container_add");
    ONIX_LOAD_REQUIRED(onix_gtk_library, gtk_widget_set_size_request, "gtk_widget_set_size_request");
    ONIX_LOAD_REQUIRED(onix_gtk_library, gtk_widget_realize, "gtk_widget_realize");
    ONIX_LOAD_REQUIRED(onix_gtk_library, gtk_widget_get_window, "gtk_widget_get_window");
    ONIX_LOAD_REQUIRED(onix_gtk_library, gtk_widget_show_all, "gtk_widget_show_all");
    ONIX_LOAD_REQUIRED(onix_gtk_library, gtk_widget_hide, "gtk_widget_hide");
    ONIX_LOAD_REQUIRED(onix_gtk_library, gtk_widget_destroy, "gtk_widget_destroy");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, gdk_set_allowed_backends, "gdk_set_allowed_backends");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, gdk_window_get_display, "gdk_window_get_display");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, gdk_x11_display_get_xdisplay, "gdk_x11_display_get_xdisplay");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, gdk_x11_window_get_xid, "gdk_x11_window_get_xid");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, gdk_window_move_resize, "gdk_window_move_resize");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, x_query_tree, "XQueryTree");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, x_free, "XFree");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, x_map_window, "XMapWindow");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, x_unmap_window, "XUnmapWindow");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, x_flush, "XFlush");

    ONIX_LOAD_REQUIRED(onix_webkit_library, webkit_user_content_manager_new, "webkit_user_content_manager_new");
    ONIX_LOAD_REQUIRED(onix_webkit_library, webkit_user_content_manager_register_script_message_handler, "webkit_user_content_manager_register_script_message_handler");
    ONIX_LOAD_REQUIRED(onix_webkit_library, webkit_user_content_manager_unregister_script_message_handler, "webkit_user_content_manager_unregister_script_message_handler");
    ONIX_LOAD_REQUIRED(onix_webkit_library, webkit_user_content_manager_add_script, "webkit_user_content_manager_add_script");
    ONIX_LOAD_REQUIRED(onix_webkit_library, webkit_user_script_new, "webkit_user_script_new");
    ONIX_LOAD_REQUIRED(onix_webkit_library, webkit_user_script_unref, "webkit_user_script_unref");
    ONIX_LOAD_REQUIRED(onix_webkit_library, webkit_web_view_new_with_user_content_manager, "webkit_web_view_new_with_user_content_manager");
    ONIX_LOAD_REQUIRED(onix_webkit_library, webkit_web_view_load_uri, "webkit_web_view_load_uri");
    ONIX_LOAD_REQUIRED(onix_webkit_library, webkit_web_view_load_html, "webkit_web_view_load_html");
    ONIX_LOAD_REQUIRED(onix_webkit_library, webkit_web_view_get_settings, "webkit_web_view_get_settings");
    ONIX_LOAD_REQUIRED(onix_webkit_library, webkit_settings_set_enable_developer_extras, "webkit_settings_set_enable_developer_extras");
    ONIX_LOAD_OPTIONAL(onix_webkit_library, webkit_settings_set_enable_write_console_messages_to_stdout, "webkit_settings_set_enable_write_console_messages_to_stdout");
    ONIX_LOAD_OPTIONAL(onix_webkit_library, webkit_web_view_set_background_color, "webkit_web_view_set_background_color");
    ONIX_LOAD_OPTIONAL(onix_webkit_library, webkit_web_view_evaluate_javascript, "webkit_web_view_evaluate_javascript");
    ONIX_LOAD_OPTIONAL(onix_webkit_library, webkit_web_view_run_javascript, "webkit_web_view_run_javascript");
    if (onix_webview_api.webkit_web_view_evaluate_javascript == NULL && onix_webview_api.webkit_web_view_run_javascript == NULL) {
        return onix_set_error("WebKitGTK runtime has no JavaScript evaluation entry point");
    }
    ONIX_LOAD_REQUIRED(onix_webkit_library, webkit_javascript_result_get_js_value, "webkit_javascript_result_get_js_value");
    ONIX_LOAD_REQUIRED(onix_webkit_library, jsc_value_to_string, "jsc_value_to_string");

    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, g_signal_connect_data, "g_signal_connect_data");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, g_signal_handler_disconnect, "g_signal_handler_disconnect");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, g_object_unref, "g_object_unref");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, g_free, "g_free");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, g_main_loop_new, "g_main_loop_new");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, g_main_loop_run, "g_main_loop_run");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, g_main_loop_quit, "g_main_loop_quit");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, g_main_loop_unref, "g_main_loop_unref");
    ONIX_LOAD_REQUIRED(RTLD_DEFAULT, g_main_context_invoke, "g_main_context_invoke");
    onix_webview_loaded = 1;
    return NULL;
}

const char *onix_linux_webview_library(void) {
    return onix_webkit_library_name == NULL ? "" : onix_webkit_library_name;
}

const char *onix_linux_webview_init(void) {
    const char *error = onix_linux_webview_load();
    if (error != NULL) return error;
    onix_webview_api.gdk_set_allowed_backends("x11");
    if (!onix_webview_api.gtk_init_check(NULL, NULL)) {
        return onix_set_error("GTK could not connect to a display");
    }
    onix_main_loop = onix_webview_api.g_main_loop_new(NULL, 0);
    if (onix_main_loop == NULL) return onix_set_error("failed to create the WebKitGTK event loop");
    return NULL;
}

void onix_linux_webview_run(void) {
    if (onix_main_loop == NULL) return;
    onix_webview_api.g_main_loop_run(onix_main_loop);
    onix_webview_api.g_main_loop_unref(onix_main_loop);
    onix_main_loop = NULL;
}

static gboolean onix_quit_operation(void *data) {
    (void)data;
    if (onix_main_loop != NULL) onix_webview_api.g_main_loop_quit(onix_main_loop);
    return 0;
}

void onix_linux_webview_quit(void) {
    if (onix_main_loop != NULL) onix_webview_api.g_main_context_invoke(NULL, onix_quit_operation, NULL);
}

static char *onix_copy_string(const char *value) {
    if (value == NULL) value = "";
    size_t length = strlen(value) + 1;
    char *copy = malloc(length);
    if (copy != NULL) memcpy(copy, value, length);
    return copy;
}

static void onix_free_operation(OnixLinuxWebViewOperation *operation) {
    if (operation == NULL) return;
    free(operation->script);
    free(operation->url);
    free(operation->html);
    free(operation);
}

static void onix_webview_message(void *manager, void *result, void *data) {
    (void)manager;
    OnixLinuxWebView *view = data;
    if (view == NULL || view->destroying || result == NULL) return;
    void *value = onix_webview_api.webkit_javascript_result_get_js_value(result);
    if (value == NULL) return;
    char *message = onix_webview_api.jsc_value_to_string(value);
    if (message == NULL) return;
    goOnixLinuxWebViewMessage(view->handle, message);
    onix_webview_api.g_free(message);
}

static void onix_webview_window_destroyed(void *window, void *data) {
    (void)window;
    OnixLinuxWebView *view = data;
    if (view == NULL || view->destroying) return;
    view->window = NULL;
    view->webview = NULL;
    view->manager = NULL;
    view->window_destroy_handler = 0;
    view->message_handler = 0;
    goOnixLinuxWebViewClosed(view->handle);
}

static void onix_destroy_view(OnixLinuxWebView *view) {
    if (view == NULL) return;
    view->destroying = 1;
    if (view->manager != NULL) {
        onix_webview_api.webkit_user_content_manager_unregister_script_message_handler(view->manager, "onix");
        if (view->message_handler != 0) {
            onix_webview_api.g_signal_handler_disconnect(view->manager, view->message_handler);
        }
        if (view->owns_manager) onix_webview_api.g_object_unref(view->manager);
    }
    if (view->window != NULL) {
        if (view->window_destroy_handler != 0) {
            onix_webview_api.g_signal_handler_disconnect(view->window, view->window_destroy_handler);
        }
        onix_webview_api.gtk_widget_destroy(view->window);
    }
    free(view);
}

static gboolean onix_create_operation(void *data) {
    OnixLinuxWebViewOperation *operation = data;
    OnixLinuxWebView *view = calloc(1, sizeof(*view));
    if (view == NULL) {
        goOnixLinuxWebViewCreated(operation->handle, 0, "window: failed to allocate WebKitGTK view");
        onix_free_operation(operation);
        return 0;
    }
    view->handle = operation->handle;
    view->parent = operation->parent;
    view->window = onix_webview_api.gtk_plug_new((unsigned long)view->parent);
    view->manager = onix_webview_api.webkit_user_content_manager_new();
    view->owns_manager = view->manager != NULL;
    if (view->window == NULL || view->manager == NULL) {
        onix_destroy_view(view);
        goOnixLinuxWebViewCreated(operation->handle, 0, "window: failed to create WebKitGTK window");
        onix_free_operation(operation);
        return 0;
    }
    if (!onix_webview_api.webkit_user_content_manager_register_script_message_handler(view->manager, "onix")) {
        onix_destroy_view(view);
        goOnixLinuxWebViewCreated(operation->handle, 0, "window: failed to register WebKitGTK message bridge");
        onix_free_operation(operation);
        return 0;
    }
    view->message_handler = onix_webview_api.g_signal_connect_data(
        view->manager,
        "script-message-received::onix",
        (void (*)(void))onix_webview_message,
        view,
        NULL,
        0
    );
    void *user_script = onix_webview_api.webkit_user_script_new(operation->script, 1, 0, NULL, NULL);
    if (user_script == NULL) {
        onix_destroy_view(view);
        goOnixLinuxWebViewCreated(operation->handle, 0, "window: failed to create WebKitGTK initialization script");
        onix_free_operation(operation);
        return 0;
    }
    onix_webview_api.webkit_user_content_manager_add_script(view->manager, user_script);
    onix_webview_api.webkit_user_script_unref(user_script);
    view->webview = onix_webview_api.webkit_web_view_new_with_user_content_manager(view->manager);
    onix_webview_api.g_object_unref(view->manager);
    view->owns_manager = 0;
    if (view->webview == NULL) {
        view->manager = NULL;
        onix_destroy_view(view);
        goOnixLinuxWebViewCreated(operation->handle, 0, "window: failed to create WebKitGTK browser");
        onix_free_operation(operation);
        return 0;
    }
    onix_webview_api.gtk_container_add(view->window, view->webview);
    onix_webview_api.gtk_window_set_decorated(view->window, 0);
    onix_webview_api.gtk_window_set_skip_taskbar_hint(view->window, 1);
    view->window_destroy_handler = onix_webview_api.g_signal_connect_data(
        view->window,
        "destroy",
        (void (*)(void))onix_webview_window_destroyed,
        view,
        NULL,
        0
    );
    int width = operation->width > 0 ? (int)operation->width : 1;
    int height = operation->height > 0 ? (int)operation->height : 1;
    onix_webview_api.gtk_widget_set_size_request(view->webview, width, height);
    onix_webview_api.gtk_window_set_default_size(view->window, width, height);
    onix_webview_api.gtk_widget_realize(view->window);
    view->gdk_window = onix_webview_api.gtk_widget_get_window(view->window);
    void *gdk_display = view->gdk_window == NULL ? NULL : onix_webview_api.gdk_window_get_display(view->gdk_window);
    view->xdisplay = gdk_display == NULL ? NULL : onix_webview_api.gdk_x11_display_get_xdisplay(gdk_display);
    view->xid = view->gdk_window == NULL ? 0 : onix_webview_api.gdk_x11_window_get_xid(view->gdk_window);
    if (view->xdisplay == NULL || view->xid == 0) {
        onix_destroy_view(view);
        goOnixLinuxWebViewCreated(operation->handle, 0, "window: failed to resolve WebKitGTK X11 window");
        onix_free_operation(operation);
        return 0;
    }
    onix_webview_api.gdk_window_move_resize(view->gdk_window, (int)operation->x, (int)operation->y, width, height);
    onix_webview_api.x_flush(view->xdisplay);
    unsigned long root_xid = 0;
    unsigned long actual_parent_xid = 0;
    unsigned long *children = NULL;
    unsigned int child_count = 0;
    int queried = onix_webview_api.x_query_tree(view->xdisplay, view->xid, &root_xid, &actual_parent_xid, &children, &child_count);
    if (children != NULL) onix_webview_api.x_free(children);
    if (!queried || actual_parent_xid != (unsigned long)view->parent) {
        onix_destroy_view(view);
        goOnixLinuxWebViewCreated(operation->handle, 0, "window: WebKitGTK window could not be embedded into Onix");
        onix_free_operation(operation);
        return 0;
    }
    void *settings = onix_webview_api.webkit_web_view_get_settings(view->webview);
    if (operation->debug && settings != NULL) {
        onix_webview_api.webkit_settings_set_enable_developer_extras(settings, 1);
        if (onix_webview_api.webkit_settings_set_enable_write_console_messages_to_stdout != NULL) {
            onix_webview_api.webkit_settings_set_enable_write_console_messages_to_stdout(settings, 1);
        }
    }
    if (operation->transparent && onix_webview_api.webkit_web_view_set_background_color != NULL) {
        const OnixGdkRGBA transparent = {0, 0, 0, 0};
        onix_webview_api.webkit_web_view_set_background_color(view->webview, &transparent);
    }
    onix_webview_api.gtk_widget_show_all(view->window);
    if (operation->visible) {
        onix_webview_api.x_map_window(view->xdisplay, view->xid);
    } else {
        onix_webview_api.gtk_widget_hide(view->window);
        onix_webview_api.x_unmap_window(view->xdisplay, view->xid);
    }
    onix_webview_api.x_flush(view->xdisplay);
    if (operation->url[0] != '\0') {
        onix_webview_api.webkit_web_view_load_uri(view->webview, operation->url);
    } else {
        onix_webview_api.webkit_web_view_load_html(view->webview, operation->html, NULL);
    }
    goOnixLinuxWebViewCreated(operation->handle, (uintptr_t)view, NULL);
    onix_free_operation(operation);
    return 0;
}

const char *onix_linux_webview_create(
    uintptr_t handle,
    uintptr_t parent,
    int debug,
    int visible,
    int transparent,
    double x,
    double y,
    double width,
    double height,
    const char *init_script,
    const char *url,
    const char *html
) {
    OnixLinuxWebViewOperation *operation = calloc(1, sizeof(*operation));
    if (operation == NULL) return onix_set_error("failed to allocate WebKitGTK create operation");
    operation->handle = handle;
    operation->parent = parent;
    operation->debug = debug;
    operation->visible = visible;
    operation->transparent = transparent;
    operation->x = x;
    operation->y = y;
    operation->width = width;
    operation->height = height;
    operation->script = onix_copy_string(init_script);
    operation->url = onix_copy_string(url);
    operation->html = onix_copy_string(html);
    if (operation->script == NULL || operation->url == NULL || operation->html == NULL) {
        onix_free_operation(operation);
        return onix_set_error("failed to copy WebKitGTK create data");
    }
    onix_webview_api.g_main_context_invoke(NULL, onix_create_operation, operation);
    return NULL;
}

static gboolean onix_navigate_operation(void *data) {
    OnixLinuxWebViewOperation *operation = data;
    if (operation->view->webview != NULL) {
        onix_webview_api.webkit_web_view_load_uri(operation->view->webview, operation->url);
    }
    onix_free_operation(operation);
    return 0;
}

static gboolean onix_set_html_operation(void *data) {
    OnixLinuxWebViewOperation *operation = data;
    if (operation->view->webview != NULL) {
        onix_webview_api.webkit_web_view_load_html(operation->view->webview, operation->html, NULL);
    }
    onix_free_operation(operation);
    return 0;
}

static gboolean onix_eval_operation(void *data) {
    OnixLinuxWebViewOperation *operation = data;
    if (operation->view->webview != NULL) {
        if (onix_webview_api.webkit_web_view_evaluate_javascript != NULL) {
            onix_webview_api.webkit_web_view_evaluate_javascript(
                operation->view->webview,
                operation->script,
                (gssize)strlen(operation->script),
                NULL,
                NULL,
                NULL,
                NULL,
                NULL
            );
        } else {
            onix_webview_api.webkit_web_view_run_javascript(operation->view->webview, operation->script, NULL, NULL, NULL);
        }
    }
    onix_free_operation(operation);
    return 0;
}

static OnixLinuxWebViewOperation *onix_string_operation(uintptr_t view, const char *value) {
    if (view == 0) return NULL;
    OnixLinuxWebViewOperation *operation = calloc(1, sizeof(*operation));
    if (operation == NULL) return NULL;
    operation->view = (OnixLinuxWebView *)view;
    operation->script = onix_copy_string(value);
    if (operation->script == NULL) {
        onix_free_operation(operation);
        return NULL;
    }
    return operation;
}

const char *onix_linux_webview_navigate(uintptr_t view, const char *url) {
    OnixLinuxWebViewOperation *operation = onix_string_operation(view, url);
    if (operation == NULL) return onix_set_error("failed to allocate WebKitGTK navigation operation");
    operation->url = operation->script;
    operation->script = NULL;
    onix_webview_api.g_main_context_invoke(NULL, onix_navigate_operation, operation);
    return NULL;
}

const char *onix_linux_webview_set_html(uintptr_t view, const char *html) {
    OnixLinuxWebViewOperation *operation = onix_string_operation(view, html);
    if (operation == NULL) return onix_set_error("failed to allocate WebKitGTK HTML operation");
    operation->html = operation->script;
    operation->script = NULL;
    onix_webview_api.g_main_context_invoke(NULL, onix_set_html_operation, operation);
    return NULL;
}

const char *onix_linux_webview_eval(uintptr_t view, const char *script) {
    OnixLinuxWebViewOperation *operation = onix_string_operation(view, script);
    if (operation == NULL) return onix_set_error("failed to allocate WebKitGTK evaluation operation");
    onix_webview_api.g_main_context_invoke(NULL, onix_eval_operation, operation);
    return NULL;
}

static gboolean onix_bounds_operation(void *data) {
    OnixLinuxWebViewOperation *operation = data;
    if (operation->view->window != NULL) {
        int width = operation->width > 0 ? (int)operation->width : 1;
        int height = operation->height > 0 ? (int)operation->height : 1;
        if (operation->view->webview != NULL) {
            onix_webview_api.gtk_widget_set_size_request(operation->view->webview, width, height);
        }
        onix_webview_api.gtk_window_resize(operation->view->window, width, height);
        if (operation->view->gdk_window != NULL) {
            onix_webview_api.gdk_window_move_resize(
                operation->view->gdk_window,
                (int)operation->x,
                (int)operation->y,
                width,
                height
            );
            onix_webview_api.x_flush(operation->view->xdisplay);
        }
    }
    onix_free_operation(operation);
    return 0;
}

const char *onix_linux_webview_set_bounds(uintptr_t view, double x, double y, double width, double height) {
    if (view == 0) return onix_set_error("WebKitGTK view is closed");
    OnixLinuxWebViewOperation *operation = calloc(1, sizeof(*operation));
    if (operation == NULL) return onix_set_error("failed to allocate WebKitGTK bounds operation");
    operation->view = (OnixLinuxWebView *)view;
    operation->x = x;
    operation->y = y;
    operation->width = width;
    operation->height = height;
    onix_webview_api.g_main_context_invoke(NULL, onix_bounds_operation, operation);
    return NULL;
}

static gboolean onix_visible_operation(void *data) {
    OnixLinuxWebViewOperation *operation = data;
    if (operation->view->window != NULL) {
        if (operation->visible) {
            onix_webview_api.gtk_widget_show_all(operation->view->window);
            onix_webview_api.x_map_window(operation->view->xdisplay, operation->view->xid);
        } else {
            onix_webview_api.gtk_widget_hide(operation->view->window);
            onix_webview_api.x_unmap_window(operation->view->xdisplay, operation->view->xid);
        }
        onix_webview_api.x_flush(operation->view->xdisplay);
    }
    onix_free_operation(operation);
    return 0;
}

const char *onix_linux_webview_set_visible(uintptr_t view, int visible) {
    if (view == 0) return onix_set_error("WebKitGTK view is closed");
    OnixLinuxWebViewOperation *operation = calloc(1, sizeof(*operation));
    if (operation == NULL) return onix_set_error("failed to allocate WebKitGTK visibility operation");
    operation->view = (OnixLinuxWebView *)view;
    operation->visible = visible;
    onix_webview_api.g_main_context_invoke(NULL, onix_visible_operation, operation);
    return NULL;
}

static gboolean onix_destroy_operation(void *data) {
    OnixLinuxWebViewOperation *operation = data;
    uintptr_t completion = operation->completion;
    onix_destroy_view(operation->view);
    onix_free_operation(operation);
    goOnixLinuxWebViewDestroyed(completion);
    return 0;
}

const char *onix_linux_webview_destroy(uintptr_t view, uintptr_t completion) {
    if (view == 0) return onix_set_error("WebKitGTK view is closed");
    OnixLinuxWebViewOperation *operation = calloc(1, sizeof(*operation));
    if (operation == NULL) return onix_set_error("failed to allocate WebKitGTK destroy operation");
    operation->view = (OnixLinuxWebView *)view;
    operation->completion = completion;
    onix_webview_api.g_main_context_invoke(NULL, onix_destroy_operation, operation);
    return NULL;
}
