#ifndef ONIX_WEBVIEW_MACOS_H
#define ONIX_WEBVIEW_MACOS_H

#include <stdint.h>

typedef struct OnixMacWebView OnixMacWebView;

const char *onix_macos_webview_last_error(void);
int onix_macos_webview_is_main_thread(void);

OnixMacWebView *onix_macos_webview_create(
    uintptr_t handle,
    void *parent,
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
);
int onix_macos_webview_navigate(OnixMacWebView *view, const char *url);
int onix_macos_webview_set_html(OnixMacWebView *view, const char *html);
int onix_macos_webview_eval(OnixMacWebView *view, const char *script);
int onix_macos_webview_set_bounds(OnixMacWebView *view, double x, double y, double width, double height);
int onix_macos_webview_set_visible(OnixMacWebView *view, int visible);
void onix_macos_webview_destroy(OnixMacWebView *view);

#endif
