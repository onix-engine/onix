//go:build cgo && darwin && !ios

package webview

/*
#cgo CFLAGS: -fblocks
#cgo LDFLAGS: -framework AppKit -framework Foundation -framework WebKit
#include <stdlib.h>
#include "webview_macos.h"
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"sync"
	"unsafe"

	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/window"
)

const macOSWebViewBridgeScript = `(function(){
  "use strict";
  var pending = new Map(), nextID = 0;
  function invoke(name, payload) {
    return new Promise(function(resolve, reject) {
      var id = ++nextID;
      pending.set(id, {resolve: resolve, reject: reject});
      window.webkit.messageHandlers.onix.postMessage(JSON.stringify({
        id: id,
        name: String(name),
        payload: String(payload == null ? "" : payload)
      }));
    });
  }
  globalThis.__onixWebViewResolve = function(response) {
    var callbacks = pending.get(response.id);
    if (!callbacks) return;
    pending.delete(response.id);
    if (response.ok) callbacks.resolve(response.result);
    else callbacks.reject(new Error(response.error || "webview invocation failed"));
  };
  globalThis.__onixInvoke = invoke;
  globalThis.onix = globalThis.onix || {};
  globalThis.onix.invoke = invoke;
})();`

var errMacOSWebViewThread = errors.New("window: WKWebView must be used on the macOS main thread")

type nativeWebViewBackend struct {
	mu      sync.Mutex
	handles map[*nativeWebViewHandle]struct{}
	parent  uintptr
	closed  bool
}

func newBackend(context *window.Context) (scene.WebViewBackend, error) {
	if context == nil || context.NativeHandle() == 0 {
		return nil, errors.New("window: WKWebView requires a native window")
	}
	if C.onix_macos_webview_is_main_thread() == 0 {
		return nil, errMacOSWebViewThread
	}
	return &nativeWebViewBackend{
		handles: make(map[*nativeWebViewHandle]struct{}),
		parent:  context.NativeHandle(),
	}, nil
}

func (b *nativeWebViewBackend) Create(options scene.WebViewOptions, invoke scene.WebViewInvoke, events func(scene.WebViewEvent)) (scene.WebViewHandle, error) {
	if b == nil || b.parent == 0 {
		return nil, errors.New("window: WKWebView backend is unavailable")
	}
	if C.onix_macos_webview_is_main_thread() == 0 {
		return nil, errMacOSWebViewThread
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, errors.New("window: WKWebView backend is closed")
	}
	handle := &nativeWebViewHandle{backend: b, invoke: invoke, events: events}
	b.handles[handle] = struct{}{}
	b.mu.Unlock()

	handle.token = cgo.NewHandle(handle)
	initScript := macOSWebViewBridgeScript
	if !options.Visible {
		initScript += `document.documentElement.style.visibility="hidden";`
	}
	initialization := C.CString(initScript)
	url := C.CString(options.URL)
	html := C.CString(options.HTML)
	view := C.onix_macos_webview_create(
		C.uintptr_t(handle.token),
		unsafe.Pointer(b.parent),
		C.int(macOSBoolInt(options.DevTools)),
		C.int(macOSBoolInt(options.Visible)),
		C.int(macOSBoolInt(options.Transparent)),
		C.double(options.Bounds.X),
		C.double(options.Bounds.Y),
		C.double(options.Bounds.Width),
		C.double(options.Bounds.Height),
		initialization,
		url,
		html,
	)
	C.free(unsafe.Pointer(initialization))
	C.free(unsafe.Pointer(url))
	C.free(unsafe.Pointer(html))
	if view == nil {
		handle.token.Delete()
		b.remove(handle)
		return nil, macOSWebViewError()
	}
	handle.view = view
	if events != nil {
		events(scene.WebViewEvent{Kind: scene.WebViewEventReady})
	}
	return handle, nil
}

func (b *nativeWebViewBackend) Close() error {
	if b == nil {
		return nil
	}
	if C.onix_macos_webview_is_main_thread() == 0 {
		return errMacOSWebViewThread
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.mu.Unlock()
	var first error
	for {
		b.mu.Lock()
		var handle *nativeWebViewHandle
		for candidate := range b.handles {
			handle = candidate
			break
		}
		b.mu.Unlock()
		if handle == nil {
			break
		}
		if err := handle.Destroy(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (b *nativeWebViewBackend) remove(handle *nativeWebViewHandle) {
	if b == nil {
		return
	}
	b.mu.Lock()
	delete(b.handles, handle)
	b.mu.Unlock()
}

type nativeWebViewHandle struct {
	backend *nativeWebViewBackend
	invoke  scene.WebViewInvoke
	events  func(scene.WebViewEvent)
	token   cgo.Handle
	view    *C.OnixMacWebView

	mu       sync.Mutex
	closed   bool
	response []byte
}

func (h *nativeWebViewHandle) withView(call func(*C.OnixMacWebView) error) error {
	if h == nil {
		return errors.New("window: webview is closed")
	}
	if C.onix_macos_webview_is_main_thread() == 0 {
		return errMacOSWebViewThread
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.view == nil {
		return errors.New("window: webview is closed")
	}
	return call(h.view)
}

func (h *nativeWebViewHandle) withString(value string, call func(*C.OnixMacWebView, *C.char) C.int) error {
	if C.onix_macos_webview_is_main_thread() == 0 {
		return errMacOSWebViewThread
	}
	text := C.CString(value)
	defer C.free(unsafe.Pointer(text))
	return h.withView(func(view *C.OnixMacWebView) error {
		if call(view, text) == 0 {
			return macOSWebViewError()
		}
		return nil
	})
}

func (h *nativeWebViewHandle) Navigate(url string) error {
	return h.withString(url, func(view *C.OnixMacWebView, value *C.char) C.int {
		return C.onix_macos_webview_navigate(view, value)
	})
}

func (h *nativeWebViewHandle) SetHTML(html string) error {
	return h.withString(html, func(view *C.OnixMacWebView, value *C.char) C.int {
		return C.onix_macos_webview_set_html(view, value)
	})
}

func (h *nativeWebViewHandle) Eval(script string) error {
	return h.withString(script, func(view *C.OnixMacWebView, value *C.char) C.int {
		return C.onix_macos_webview_eval(view, value)
	})
}

func (h *nativeWebViewHandle) SetBounds(bounds scene.WebViewBounds) error {
	return h.withView(func(view *C.OnixMacWebView) error {
		if C.onix_macos_webview_set_bounds(view, C.double(bounds.X), C.double(bounds.Y), C.double(bounds.Width), C.double(bounds.Height)) == 0 {
			return macOSWebViewError()
		}
		return nil
	})
}

func (h *nativeWebViewHandle) SetVisible(visible bool) error {
	return h.withView(func(view *C.OnixMacWebView) error {
		if C.onix_macos_webview_set_visible(view, C.int(macOSBoolInt(visible))) == 0 {
			return macOSWebViewError()
		}
		return nil
	})
}

func (h *nativeWebViewHandle) Destroy() error {
	if h == nil {
		return nil
	}
	if C.onix_macos_webview_is_main_thread() == 0 {
		return errMacOSWebViewThread
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	view := h.view
	h.view = nil
	h.mu.Unlock()
	if view != nil {
		C.onix_macos_webview_destroy(view)
	}
	h.token.Delete()
	h.backend.remove(h)
	return nil
}

func (h *nativeWebViewHandle) handleMessage(message string) {
	var request webViewBridgeRequest
	if err := decodeWebViewBridgeRequest(message, &request); err != nil {
		h.reportError(err)
		return
	}
	var result string
	var responseErr error
	if h.invoke == nil {
		responseErr = errors.New("window: webview bridge is not configured")
	} else {
		result, responseErr = h.invoke(request.Name, request.Payload)
	}
	h.response = appendWebViewBridgeResponse(h.response, request.ID, result, responseErr)
	_ = h.Eval("globalThis.__onixWebViewResolve(" + string(h.response) + ");")
}

func (h *nativeWebViewHandle) reportError(err error) {
	if err != nil && h.events != nil {
		h.events(scene.WebViewEvent{Kind: scene.WebViewEventError, Error: err.Error()})
	}
}

func macOSWebViewError() error {
	message := C.onix_macos_webview_last_error()
	if message == nil {
		return errors.New("window: WKWebView operation failed")
	}
	return errors.New(C.GoString(message))
}

func macOSBoolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

//export goOnixMacWebViewMessage
func goOnixMacWebViewMessage(token C.uintptr_t, message *C.char) {
	handle := cgo.Handle(token).Value().(*nativeWebViewHandle)
	handle.handleMessage(C.GoString(message))
}

//export goOnixMacWebViewNavigation
func goOnixMacWebViewNavigation(token C.uintptr_t, url *C.char) {
	handle := cgo.Handle(token).Value().(*nativeWebViewHandle)
	if handle.events != nil {
		handle.events(scene.WebViewEvent{Kind: scene.WebViewEventNavigation, URL: C.GoString(url)})
	}
}

//export goOnixMacWebViewError
func goOnixMacWebViewError(token C.uintptr_t, message *C.char) {
	handle := cgo.Handle(token).Value().(*nativeWebViewHandle)
	handle.reportError(errors.New(C.GoString(message)))
}
