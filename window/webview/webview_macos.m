//go:build darwin && cgo && !ios

#include "webview_macos.h"

#import <AppKit/AppKit.h>
#import <WebKit/WebKit.h>

#include <math.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>

extern void goOnixMacWebViewMessage(uintptr_t handle, const char *message);
extern void goOnixMacWebViewNavigation(uintptr_t handle, const char *url);
extern void goOnixMacWebViewError(uintptr_t handle, const char *message);

struct OnixMacWebView {
    uintptr_t handle;
    WKWebView *webview;
    NSObject<WKScriptMessageHandler> *message_handler;
    NSObject<WKNavigationDelegate> *navigation_delegate;
};

static _Thread_local char onix_macos_webview_error[512];

static void onix_macos_webview_set_error(const char *message) {
    snprintf(
        onix_macos_webview_error,
        sizeof(onix_macos_webview_error),
        "window: %s",
        message == NULL ? "WKWebView operation failed" : message
    );
}

static int onix_macos_webview_main_thread(void) {
    if (pthread_main_np() != 0) return 1;
    onix_macos_webview_set_error("WKWebView must be used on the macOS main thread");
    return 0;
}

static CGFloat onix_macos_webview_dimension(double value) {
    return isfinite(value) && value > 0 ? (CGFloat)value : 1;
}

static CGFloat onix_macos_webview_coordinate(double value) {
    return isfinite(value) ? (CGFloat)value : 0;
}

static NSString *onix_macos_webview_string(const char *value) {
    return value == NULL ? @"" : [NSString stringWithUTF8String:value];
}

@interface OnixMacWebViewMessageHandler : NSObject <WKScriptMessageHandler>
@property(nonatomic, assign) uintptr_t handle;
@end

@implementation OnixMacWebViewMessageHandler
- (void)userContentController:(WKUserContentController *)controller didReceiveScriptMessage:(WKScriptMessage *)message {
    (void)controller;
    if (![message.body isKindOfClass:[NSString class]]) return;
    goOnixMacWebViewMessage(self.handle, [(NSString *)message.body UTF8String]);
}
@end

@interface OnixMacWebViewNavigationDelegate : NSObject <WKNavigationDelegate>
@property(nonatomic, assign) uintptr_t handle;
@end

@implementation OnixMacWebViewNavigationDelegate
- (void)webView:(WKWebView *)webview didFinishNavigation:(WKNavigation *)navigation {
    (void)navigation;
    const char *url = webview.URL.absoluteString.UTF8String;
    goOnixMacWebViewNavigation(self.handle, url == NULL ? "" : url);
}
- (void)webView:(WKWebView *)webview didFailNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    (void)webview;
    (void)navigation;
    const char *message = error.localizedDescription.UTF8String;
    goOnixMacWebViewError(self.handle, message == NULL ? "WKWebView navigation failed" : message);
}
- (void)webView:(WKWebView *)webview didFailProvisionalNavigation:(WKNavigation *)navigation withError:(NSError *)error {
    (void)webview;
    (void)navigation;
    const char *message = error.localizedDescription.UTF8String;
    goOnixMacWebViewError(self.handle, message == NULL ? "WKWebView navigation failed" : message);
}
- (void)webViewWebContentProcessDidTerminate:(WKWebView *)webview {
    (void)webview;
    goOnixMacWebViewError(self.handle, "WKWebView content process terminated");
}
@end

const char *onix_macos_webview_last_error(void) {
    return onix_macos_webview_error[0] == '\0' ? NULL : onix_macos_webview_error;
}

int onix_macos_webview_is_main_thread(void) {
    return pthread_main_np() != 0;
}

void onix_macos_webview_destroy(OnixMacWebView *view) {
    if (view == NULL || !onix_macos_webview_main_thread()) return;
    @autoreleasepool {
        [view->webview.configuration.userContentController removeScriptMessageHandlerForName:@"onix"];
        view->webview.navigationDelegate = nil;
        [view->webview stopLoading];
        [view->webview removeFromSuperview];
        [view->navigation_delegate release];
        [view->message_handler release];
        [view->webview release];
        free(view);
    }
}

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
) {
    onix_macos_webview_error[0] = '\0';
    if (!onix_macos_webview_main_thread()) return NULL;
    @autoreleasepool {
        NSWindow *window = (NSWindow *)parent;
        if (window == nil || window.contentView == nil) {
            onix_macos_webview_set_error("WKWebView parent window is unavailable");
            return NULL;
        }
        NSString *initialization = onix_macos_webview_string(init_script);
        NSString *initialURL = onix_macos_webview_string(url);
        NSString *initialHTML = onix_macos_webview_string(html);
        if (initialization == nil || initialURL == nil || initialHTML == nil) {
            onix_macos_webview_set_error("WKWebView input is not valid UTF-8");
            return NULL;
        }

        OnixMacWebView *view = calloc(1, sizeof(*view));
        if (view == NULL) {
            onix_macos_webview_set_error("failed to allocate WKWebView state");
            return NULL;
        }
        view->handle = handle;
        WKWebViewConfiguration *configuration = [[WKWebViewConfiguration alloc] init];
        OnixMacWebViewMessageHandler *messageHandler = [[OnixMacWebViewMessageHandler alloc] init];
        messageHandler.handle = handle;
        view->message_handler = messageHandler;
        [configuration.userContentController addScriptMessageHandler:messageHandler name:@"onix"];
        WKUserScript *userScript = [[WKUserScript alloc]
            initWithSource:initialization
            injectionTime:WKUserScriptInjectionTimeAtDocumentStart
            forMainFrameOnly:YES];
        [configuration.userContentController addUserScript:userScript];
        [userScript release];
        if (debug) {
            [configuration.preferences setValue:@YES forKey:@"developerExtrasEnabled"];
        }

        NSRect frame = NSMakeRect(
            onix_macos_webview_coordinate(x),
            onix_macos_webview_coordinate(y),
            onix_macos_webview_dimension(width),
            onix_macos_webview_dimension(height)
        );
        view->webview = [[WKWebView alloc] initWithFrame:frame configuration:configuration];
        [configuration release];
        if (view->webview == nil) {
            [messageHandler release];
            free(view);
            onix_macos_webview_set_error("failed to create WKWebView");
            return NULL;
        }
        OnixMacWebViewNavigationDelegate *navigationDelegate = [[OnixMacWebViewNavigationDelegate alloc] init];
        navigationDelegate.handle = handle;
        view->navigation_delegate = navigationDelegate;
        view->webview.navigationDelegate = navigationDelegate;
        view->webview.hidden = visible ? NO : YES;
        if (transparent) {
            [view->webview setValue:@NO forKey:@"drawsBackground"];
        }
        if (debug && [view->webview respondsToSelector:NSSelectorFromString(@"setInspectable:")]) {
            [view->webview setValue:@YES forKey:@"inspectable"];
        }
        [window.contentView addSubview:view->webview];

        if (initialURL.length > 0) {
            NSURL *target = [NSURL URLWithString:initialURL];
            if (target == nil) {
                onix_macos_webview_set_error("WKWebView URL is invalid");
                onix_macos_webview_destroy(view);
                return NULL;
            }
            [view->webview loadRequest:[NSURLRequest requestWithURL:target]];
        } else {
            [view->webview loadHTMLString:initialHTML baseURL:nil];
        }
        return view;
    }
}

int onix_macos_webview_navigate(OnixMacWebView *view, const char *url) {
    onix_macos_webview_error[0] = '\0';
    if (view == NULL || !onix_macos_webview_main_thread()) return 0;
    @autoreleasepool {
        NSString *value = onix_macos_webview_string(url);
        NSURL *target = value == nil ? nil : [NSURL URLWithString:value];
        if (target == nil) {
            onix_macos_webview_set_error("WKWebView URL is invalid");
            return 0;
        }
        [view->webview loadRequest:[NSURLRequest requestWithURL:target]];
        return 1;
    }
}

int onix_macos_webview_set_html(OnixMacWebView *view, const char *html) {
    onix_macos_webview_error[0] = '\0';
    if (view == NULL || !onix_macos_webview_main_thread()) return 0;
    @autoreleasepool {
        NSString *value = onix_macos_webview_string(html);
        if (value == nil) {
            onix_macos_webview_set_error("WKWebView HTML is not valid UTF-8");
            return 0;
        }
        [view->webview loadHTMLString:value baseURL:nil];
        return 1;
    }
}

int onix_macos_webview_eval(OnixMacWebView *view, const char *script) {
    onix_macos_webview_error[0] = '\0';
    if (view == NULL || !onix_macos_webview_main_thread()) return 0;
    @autoreleasepool {
        NSString *value = onix_macos_webview_string(script);
        if (value == nil) {
            onix_macos_webview_set_error("WKWebView script is not valid UTF-8");
            return 0;
        }
        [view->webview evaluateJavaScript:value completionHandler:nil];
        return 1;
    }
}

int onix_macos_webview_set_bounds(OnixMacWebView *view, double x, double y, double width, double height) {
    onix_macos_webview_error[0] = '\0';
    if (view == NULL || !onix_macos_webview_main_thread()) return 0;
    view->webview.frame = NSMakeRect(
        onix_macos_webview_coordinate(x),
        onix_macos_webview_coordinate(y),
        onix_macos_webview_dimension(width),
        onix_macos_webview_dimension(height)
    );
    return 1;
}

int onix_macos_webview_set_visible(OnixMacWebView *view, int visible) {
    onix_macos_webview_error[0] = '\0';
    if (view == NULL || !onix_macos_webview_main_thread()) return 0;
    view->webview.hidden = visible ? NO : YES;
    return 1;
}
