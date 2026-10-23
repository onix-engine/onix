#include "../linux_native.h"

#include <X11/Xlib.h>
#include <X11/extensions/XTest.h>
#include <assert.h>
#include <stdio.h>
#include <stdlib.h>

static void poll_window(OnixNativeWindow *window) {
    XSync(onix_window_native_display(window), False);
    onix_window_poll_events(window);
}

static int drain_capture(OnixNativeWindow *window) {
    OnixEvent event;
    int captured = -1;
    while (onix_window_pop_event(window, &event)) {
        if (event.kind == ONIX_EVENT_MOUSE_CAPTURE) captured = event.arg0;
    }
    return captured;
}

int main(void) {
    setenv("ONIX_LINUX_BACKEND", "x11", 1);
    OnixNativeWindow *window = onix_window_create(320, 240, "Onix capture test", 0, 0);
    assert(window != NULL);
    Display *display = onix_window_native_display(window);
    Window native = onix_window_native_surface(window);
    Display *observer = XOpenDisplay(NULL);
    assert(observer != NULL);
    XSetInputFocus(display, native, RevertToParent, CurrentTime);
    XWarpPointer(display, None, native, 0, 0, 0, 0, 160, 120);
    poll_window(window);
    drain_capture(window);
    assert(onix_window_set_mouse_captured(window, 1));
    assert(drain_capture(window) == 1);

    double dx = 0, dy = 0;
    for (int index = 0; index < 100; index++) {
        XTestFakeRelativeMotionEvent(display, 40, -20, CurrentTime);
        poll_window(window);
        OnixEvent event;
        while (onix_window_pop_event(window, &event)) {
            assert(event.kind != ONIX_EVENT_MOUSE_MOVE);
            if (event.kind == ONIX_EVENT_MOUSE_DELTA) {
                dx += event.x;
                dy += event.y;
            }
        }
    }
    assert(dx == 4000 && dy == -2000);

    for (int index = 0; index < 1024; index++) {
        XTestFakeRelativeMotionEvent(display, 1, 0, CurrentTime);
    }
    poll_window(window);
    OnixEvent event;
    dx = 0;
    while (onix_window_pop_event(window, &event)) {
        if (event.kind == ONIX_EVENT_MOUSE_DELTA) dx += event.x;
    }
    assert(dx == 1024);

    Window root, child;
    int root_x, root_y, x, y;
    unsigned int buttons;
    XWarpPointer(display, None, DefaultRootWindow(display), 0, 0, 0, 0, 0, 0);
    poll_window(window);
    XQueryPointer(display, native, &root, &child, &root_x, &root_y, &x, &y, &buttons);
    assert(x >= 0 && x < 320 && y >= 0 && y < 240);

    assert(onix_window_set_mouse_captured(window, 0));
    assert(drain_capture(window) == 0);
    assert(XGrabPointer(observer, native, False, 0, GrabModeAsync, GrabModeAsync, None, None, CurrentTime) == GrabSuccess);
    XUngrabPointer(observer, CurrentTime);
    XSync(observer, False);

    assert(onix_window_set_mouse_captured(window, 1));
    poll_window(window);
    assert(drain_capture(window) == 1);
    XSetInputFocus(display, DefaultRootWindow(display), RevertToParent, CurrentTime);
    poll_window(window);
    assert(drain_capture(window) == 0);
    assert(!onix_window_set_mouse_captured(window, 1));

    XSetInputFocus(display, native, RevertToParent, CurrentTime);
    poll_window(window);
    assert(onix_window_set_mouse_captured(window, 1));
    onix_window_destroy(window);
    free(window);
    assert(XGrabPointer(observer, DefaultRootWindow(observer), False, 0, GrabModeAsync, GrabModeAsync, None, None, CurrentTime) == GrabSuccess);
    XUngrabPointer(observer, CurrentTime);
    XCloseDisplay(observer);
    puts("X11 capture passed: unlimited relative motion, confinement, release, focus loss, teardown");
    return 0;
}
