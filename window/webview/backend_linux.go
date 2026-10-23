//go:build linux && cgo && !android && !onix_drm

package webview

/*
#cgo linux LDFLAGS: -ldl
#include <stdlib.h>
#include "webview_linux.h"
*/
import "C"

import (
	"errors"
	"runtime"
	"runtime/cgo"
	"sync"
	"unsafe"

	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/window"
)

const linuxWebViewBridgeScript = `(function(){
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

type linuxWebViewBackend struct {
	mu      sync.Mutex
	handles map[*linuxWebViewHandle]struct{}
	closed  bool
	done    chan struct{}
	parent  uintptr
}

var (
	linuxWebViewLoadOnce sync.Once
	linuxWebViewLoadErr  error
)

func newBackend(context *window.Context) (scene.WebViewBackend, error) {
	if context == nil || context.NativeX11Window() == 0 {
		return nil, errors.New("window: embedded WebKitGTK requires an X11 parent window")
	}
	if err := linuxWebViewLoad(); err != nil {
		return nil, err
	}
	backend := &linuxWebViewBackend{
		handles: make(map[*linuxWebViewHandle]struct{}),
		done:    make(chan struct{}),
		parent:  context.NativeX11Window(),
	}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		err := linuxWebViewError(C.onix_linux_webview_init())
		ready <- err
		if err != nil {
			close(backend.done)
			return
		}
		C.onix_linux_webview_run()
		close(backend.done)
	}()
	if err := <-ready; err != nil {
		return nil, err
	}
	return backend, nil
}

func linuxWebViewLoad() error {
	linuxWebViewLoadOnce.Do(func() {
		linuxWebViewLoadErr = linuxWebViewError(C.onix_linux_webview_load())
	})
	return linuxWebViewLoadErr
}

func linuxWebViewLibrary() (string, error) {
	if err := linuxWebViewLoad(); err != nil {
		return "", err
	}
	return C.GoString(C.onix_linux_webview_library()), nil
}

func linuxWebViewError(message *C.char) error {
	if message == nil {
		return nil
	}
	return errors.New(C.GoString(message))
}

func (b *linuxWebViewBackend) Create(options scene.WebViewOptions, invoke scene.WebViewInvoke, events func(scene.WebViewEvent)) (scene.WebViewHandle, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, errors.New("window: WebKitGTK backend is closed")
	}
	b.mu.Unlock()

	handle := &linuxWebViewHandle{
		backend: b,
		invoke:  invoke,
		events:  events,
		created: make(chan linuxWebViewCreateResult, 1),
	}
	handle.token = cgo.NewHandle(handle)

	initScript := linuxWebViewBridgeScript
	if !options.Visible {
		initScript += `document.documentElement.style.visibility="hidden";`
	}
	initValue := C.CString(initScript)
	url := C.CString(options.URL)
	html := C.CString(options.HTML)
	message := C.onix_linux_webview_create(
		C.uintptr_t(handle.token),
		C.uintptr_t(b.parent),
		C.int(boolInt(options.DevTools)),
		C.int(boolInt(options.Visible)),
		C.int(boolInt(options.Transparent)),
		C.double(options.Bounds.X),
		C.double(options.Bounds.Y),
		C.double(options.Bounds.Width),
		C.double(options.Bounds.Height),
		initValue,
		url,
		html,
	)
	C.free(unsafe.Pointer(initValue))
	C.free(unsafe.Pointer(url))
	C.free(unsafe.Pointer(html))
	if err := linuxWebViewError(message); err != nil {
		handle.token.Delete()
		return nil, err
	}
	result := <-handle.created
	if result.err != nil {
		handle.token.Delete()
		return nil, result.err
	}
	handle.mu.Lock()
	handle.view = result.view
	handle.mu.Unlock()

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		_ = handle.Destroy()
		return nil, errors.New("window: WebKitGTK backend is closed")
	}
	b.handles[handle] = struct{}{}
	b.mu.Unlock()
	if events != nil {
		events(scene.WebViewEvent{Kind: scene.WebViewEventReady})
	}
	return handle, nil
}

func (b *linuxWebViewBackend) remove(handle *linuxWebViewHandle) {
	b.mu.Lock()
	delete(b.handles, handle)
	b.mu.Unlock()
}

func (b *linuxWebViewBackend) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	if b.closed {
		done := b.done
		b.mu.Unlock()
		<-done
		return nil
	}
	b.closed = true
	handles := make([]*linuxWebViewHandle, 0, len(b.handles))
	for handle := range b.handles {
		handles = append(handles, handle)
	}
	b.mu.Unlock()

	var first error
	for _, handle := range handles {
		if err := handle.Destroy(); err != nil && first == nil {
			first = err
		}
	}
	C.onix_linux_webview_quit()
	<-b.done
	return first
}

type linuxWebViewCreateResult struct {
	view uintptr
	err  error
}

type linuxWebViewDestroyRequest struct {
	handle *linuxWebViewHandle
	done   chan struct{}
}

type linuxWebViewHandle struct {
	backend *linuxWebViewBackend
	invoke  scene.WebViewInvoke
	events  func(scene.WebViewEvent)
	created chan linuxWebViewCreateResult
	token   cgo.Handle

	mu             sync.Mutex
	responseMu     sync.Mutex
	view           uintptr
	closed         bool
	destroyStarted bool
	destroyDone    chan struct{}
	response       []byte
}

func (h *linuxWebViewHandle) queueString(value string, call func(C.uintptr_t, *C.char) *C.char) error {
	h.mu.Lock()
	if h.closed || h.view == 0 {
		h.mu.Unlock()
		return errors.New("window: webview is closed")
	}
	view := C.uintptr_t(h.view)
	h.mu.Unlock()
	text := C.CString(value)
	message := call(view, text)
	C.free(unsafe.Pointer(text))
	return linuxWebViewError(message)
}

func (h *linuxWebViewHandle) Navigate(url string) error {
	return h.queueString(url, func(view C.uintptr_t, value *C.char) *C.char {
		return C.onix_linux_webview_navigate(view, value)
	})
}

func (h *linuxWebViewHandle) SetHTML(html string) error {
	return h.queueString(html, func(view C.uintptr_t, value *C.char) *C.char {
		return C.onix_linux_webview_set_html(view, value)
	})
}

func (h *linuxWebViewHandle) Eval(script string) error {
	return h.queueString(script, func(view C.uintptr_t, value *C.char) *C.char {
		return C.onix_linux_webview_eval(view, value)
	})
}

func (h *linuxWebViewHandle) SetBounds(bounds scene.WebViewBounds) error {
	h.mu.Lock()
	if h.closed || h.view == 0 {
		h.mu.Unlock()
		return errors.New("window: webview is closed")
	}
	view := C.uintptr_t(h.view)
	h.mu.Unlock()
	return linuxWebViewError(C.onix_linux_webview_set_bounds(
		view,
		C.double(bounds.X),
		C.double(bounds.Y),
		C.double(bounds.Width),
		C.double(bounds.Height),
	))
}

func (h *linuxWebViewHandle) SetVisible(visible bool) error {
	h.mu.Lock()
	if h.closed || h.view == 0 {
		h.mu.Unlock()
		return errors.New("window: webview is closed")
	}
	view := C.uintptr_t(h.view)
	h.mu.Unlock()
	return linuxWebViewError(C.onix_linux_webview_set_visible(view, C.int(boolInt(visible))))
}

func (h *linuxWebViewHandle) Destroy() error {
	h.mu.Lock()
	if h.destroyStarted {
		done := h.destroyDone
		h.mu.Unlock()
		<-done
		return nil
	}
	h.destroyStarted = true
	h.closed = true
	h.destroyDone = make(chan struct{})
	done := h.destroyDone
	view := h.view
	h.mu.Unlock()
	if view == 0 {
		close(done)
		return nil
	}
	request := &linuxWebViewDestroyRequest{handle: h, done: done}
	token := cgo.NewHandle(request)
	if err := linuxWebViewError(C.onix_linux_webview_destroy(C.uintptr_t(view), C.uintptr_t(token))); err != nil {
		token.Delete()
		close(done)
		return err
	}
	<-done
	return nil
}

func (h *linuxWebViewHandle) handleMessage(message string) {
	var request webViewBridgeRequest
	if err := decodeWebViewBridgeRequest(message, &request); err != nil {
		return
	}
	var result string
	var responseErr error
	if h.invoke == nil {
		responseErr = errors.New("window: webview bridge is not configured")
	} else {
		result, responseErr = h.invoke(request.Name, request.Payload)
	}
	h.responseMu.Lock()
	h.response = appendWebViewBridgeResponse(h.response, request.ID, result, responseErr)
	script := "globalThis.__onixWebViewResolve(" + string(h.response) + ");"
	h.responseMu.Unlock()
	_ = h.Eval(script)
}

func (h *linuxWebViewHandle) handleClosed() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	events := h.events
	h.mu.Unlock()
	if events != nil {
		events(scene.WebViewEvent{Kind: scene.WebViewEventClosed})
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

//export goOnixLinuxWebViewCreated
func goOnixLinuxWebViewCreated(token C.uintptr_t, view C.uintptr_t, message *C.char) {
	handle := cgo.Handle(token).Value().(*linuxWebViewHandle)
	handle.created <- linuxWebViewCreateResult{
		view: uintptr(view),
		err:  linuxWebViewError(message),
	}
}

//export goOnixLinuxWebViewMessage
func goOnixLinuxWebViewMessage(token C.uintptr_t, message *C.char) {
	handle := cgo.Handle(token).Value().(*linuxWebViewHandle)
	handle.handleMessage(C.GoString(message))
}

//export goOnixLinuxWebViewClosed
func goOnixLinuxWebViewClosed(token C.uintptr_t) {
	handle := cgo.Handle(token).Value().(*linuxWebViewHandle)
	handle.handleClosed()
}

//export goOnixLinuxWebViewDestroyed
func goOnixLinuxWebViewDestroyed(token C.uintptr_t) {
	handle := cgo.Handle(token)
	request := handle.Value().(*linuxWebViewDestroyRequest)
	handle.Delete()
	request.handle.token.Delete()
	request.handle.backend.remove(request.handle)
	close(request.done)
}
