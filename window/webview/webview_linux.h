#ifndef ONIX_WEBVIEW_LINUX_H
#define ONIX_WEBVIEW_LINUX_H

#include <stdint.h>

const char *onix_linux_webview_load(void);
const char *onix_linux_webview_library(void);
const char *onix_linux_webview_init(void);
void onix_linux_webview_run(void);
void onix_linux_webview_quit(void);

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
);
const char *onix_linux_webview_navigate(uintptr_t view, const char *url);
const char *onix_linux_webview_set_html(uintptr_t view, const char *html);
const char *onix_linux_webview_eval(uintptr_t view, const char *script);
const char *onix_linux_webview_set_bounds(
    uintptr_t view,
    double x,
    double y,
    double width,
    double height
);
const char *onix_linux_webview_set_visible(uintptr_t view, int visible);
const char *onix_linux_webview_destroy(uintptr_t view, uintptr_t completion);

#endif
