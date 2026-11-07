#include "../macos_native.h"

#import <AppKit/AppKit.h>
#import <objc/runtime.h>
#include <assert.h>
#include <stdatomic.h>
#include <string.h>

static int dispatched;
static atomic_bool woke;
static NSUInteger mouse_buttons;
static NSEvent *drag_event;

static NSUInteger pressed_mouse_buttons(id object, SEL selector) {
    (void)object;
    (void)selector;
    return mouse_buttons;
}

static void perform_window_drag(id object, SEL selector, NSEvent *event) {
    (void)object;
    (void)selector;
    drag_event = event;
}

@interface OnixIdleTestApplication : NSApplication
@end

@implementation OnixIdleTestApplication
- (void)sendEvent:(NSEvent *)event {
    if (event.type == NSEventTypeApplicationDefined && event.subtype == 73) {
        dispatched++;
    } else {
        [super sendEvent:event];
    }
}
@end

static int drain(OnixMacWindow *state) {
    OnixMacEvent event;
    int redraws = 0;
    while (onix_macos_pop_event(state, &event)) {
        if (event.kind == ONIX_MAC_EVENT_REDRAW) redraws++;
        onix_macos_release_event(&event);
    }
    return redraws;
}

static void test_dispatch(OnixMacWindow *state) {
    NSEvent *event = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
        location:NSZeroPoint modifierFlags:0 timestamp:0 windowNumber:0
        context:nil subtype:73 data1:0 data2:0];
    [NSApp postEvent:event atStart:YES];
    onix_macos_wait(state, 1000);
    assert(dispatched == 1); // The wait must dispatch, not just peek.
    onix_macos_poll(state);
    assert(dispatched == 1); // Polling must not dispatch the same event twice.
}

static void test_wake(OnixMacWindow *state) {
    for (int i = 0; i < 8; i++) {
        onix_macos_poll(state);
        drain(state);
        atomic_store(&woke, false);
        dispatch_group_t group = dispatch_group_create();
        dispatch_group_async(group, dispatch_get_global_queue(QOS_CLASS_DEFAULT, 0), ^{
            [NSThread sleepForTimeInterval:0.02];
            atomic_store(&woke, true);
            onix_macos_wake();
        });
        while (!atomic_load(&woke)) {
            onix_macos_wait(state, -1);
            onix_macos_poll(state);
            drain(state);
        }
        dispatch_group_wait(group, DISPATCH_TIME_FOREVER);
        dispatch_release(group);
    }
}

static void test_callback(OnixMacWindow *state, NSWindow *window) {
    __block BOOL exposed = NO;
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 20 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
        // A run-loop callback can enqueue Onix work without producing an NSEvent.
        [window.delegate windowDidExpose:[NSNotification notificationWithName:NSWindowDidExposeNotification object:window]];
        exposed = YES;
    });
    int redraws = 0;
    while (!exposed) {
        onix_macos_wait(state, -1);
        redraws += drain(state);
    }
    assert(redraws > 0);
    [window.delegate windowDidBecomeKey:[NSNotification notificationWithName:NSWindowDidBecomeKeyNotification object:window]];
    assert(drain(state) > 0);
    [window.delegate windowDidDeminiaturize:[NSNotification notificationWithName:NSWindowDidDeminiaturizeNotification object:window]];
    assert(drain(state) > 0);
    [NSApp.delegate applicationDidBecomeActive:[NSNotification notificationWithName:NSApplicationDidBecomeActiveNotification object:NSApp]];
    assert(drain(state) > 0);
}

static void test_reopen(OnixMacWindow *state, NSWindow *window) {
    assert([NSApp.delegate respondsToSelector:@selector(applicationShouldHandleReopen:hasVisibleWindows:)]);
    [window orderOut:nil];
    onix_macos_poll(state);
    drain(state);
    assert(!window.isVisible);
    [NSApp.delegate applicationShouldHandleReopen:NSApp hasVisibleWindows:NO];
    assert(window.isVisible);
    assert(drain(state) > 0);

    [window miniaturize:nil];
    onix_macos_poll(state);
    drain(state);
    assert(window.isMiniaturized);
    // AppKit reports miniaturized windows as visible to the reopen delegate.
    [NSApp.delegate applicationShouldHandleReopen:NSApp hasVisibleWindows:YES];
    assert(!window.isMiniaturized);
    assert(window.isVisible);
    assert(drain(state) > 0);
}

static void test_drag(OnixMacWindow *state, NSWindow *window) {
    Method buttons = class_getClassMethod([NSEvent class], @selector(pressedMouseButtons));
    Method drag = class_getInstanceMethod([NSWindow class], @selector(performWindowDragWithEvent:));
    IMP original_buttons = method_setImplementation(buttons, (IMP)pressed_mouse_buttons);
    IMP original_drag = method_setImplementation(drag, (IMP)perform_window_drag);
    OnixMacEvent events[2];
    // Queue two different presses, then let their autorelease pool expire before
    // dispatching either, just as the Go event batch does after native polling.
    @autoreleasepool {
        for (int i = 0; i < 2; i++) {
            NSEvent *press = [NSEvent mouseEventWithType:NSEventTypeLeftMouseDown
                location:NSMakePoint(100 + i * 80, 220) modifierFlags:0 timestamp:i + 1
                windowNumber:window.windowNumber context:nil eventNumber:i + 1 clickCount:1 pressure:1];
            [window.contentView mouseDown:press];
        }
    }
    for (int i = 0; i < 2; i++) {
        assert(onix_macos_pop_event(state, &events[i]));
        assert(events[i].kind == ONIX_MAC_EVENT_MOUSE_DOWN);
    }
    mouse_buttons = 1;
    for (int i = 0; i < 2; i++) {
        drag_event = nil;
        onix_macos_start_drag(state, &events[i]);
        assert(drag_event == events[i].mouse_down);
        assert(drag_event.eventNumber == i + 1);
        assert(drag_event.locationInWindow.x == 100 + i * 80);
    }
    // A completed click must not initiate a late drag from an old position.
    mouse_buttons = 0;
    drag_event = nil;
    onix_macos_start_drag(state, &events[0]);
    assert(drag_event == nil);
    mouse_buttons = 1;
    onix_macos_start_drag(state, NULL);
    assert(drag_event == nil);
    for (int i = 0; i < 2; i++) {
        onix_macos_release_event(&events[i]);
        assert(events[i].mouse_down == NULL);
        onix_macos_start_drag(state, &events[i]);
        assert(drag_event == nil);
        onix_macos_release_event(&events[i]);
    }
    method_setImplementation(buttons, original_buttons);
    method_setImplementation(drag, original_drag);
}

int main(int argc, const char **argv) {
    assert(argc == 2);
    @autoreleasepool {
        [OnixIdleTestApplication sharedApplication];
        OnixMacWindow *state = onix_macos_create(320, 240, "Onix idle test", 0, 1, 40, 1, 1);
        assert(state != NULL);
        NSWindow *window = (NSWindow *)onix_macos_native_window(state);
        onix_macos_poll(state);
        drain(state);
        if (strcmp(argv[1], "dispatch") == 0) test_dispatch(state);
        else if (strcmp(argv[1], "wake") == 0) test_wake(state);
        else if (strcmp(argv[1], "callback") == 0) test_callback(state, window);
        else if (strcmp(argv[1], "reopen") == 0) test_reopen(state, window);
        else if (strcmp(argv[1], "drag") == 0) test_drag(state, window);
        else assert(false);
        onix_macos_destroy(state);
        assert([NSApp.delegate applicationShouldHandleReopen:NSApp hasVisibleWindows:NO]);
    }
    return 0;
}
