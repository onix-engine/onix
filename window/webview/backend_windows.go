//go:build cgo && windows

package webview

import (
	"errors"
	"fmt"
	"sync"

	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/window"
)

var (
	errWindowsWebViewClosed = errors.New("window: webview is closed")
	errWindowsWebViewBridge = errors.New("window: webview bridge is not configured")
)

func newBackend(context *window.Context) (scene.WebViewBackend, error) {
	if context == nil || context.NativeHandle() == 0 {
		return nil, errors.New("window: WebView2 requires a native window")
	}
	if err := registerWebViewHostClass(); err != nil {
		return nil, err
	}
	if err := initializeWebViewCOM(); err != nil {
		return nil, err
	}
	return &windowsWebViewBackend{
		context: context,
		parent:  context.NativeHandle(),
		thread:  currentWebViewThreadID(),
	}, nil
}

type windowsWebViewBackend struct {
	context *window.Context
	parent  uintptr
	thread  uint32

	mu      sync.Mutex
	handles map[*windowsWebViewHandle]struct{}
	closed  bool
}

func (b *windowsWebViewBackend) Create(options scene.WebViewOptions, invoke scene.WebViewInvoke, events func(scene.WebViewEvent)) (scene.WebViewHandle, error) {
	if b == nil || b.parent == 0 {
		return nil, errors.New("window: WebView2 backend is unavailable")
	}
	if currentWebViewThreadID() != b.thread {
		return nil, errors.New("window: WebView2 must be created on the window thread")
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, errors.New("window: WebView2 backend is closed")
	}
	handle := &windowsWebViewHandle{backend: b, invoke: invoke, events: events}
	if b.handles == nil {
		b.handles = make(map[*windowsWebViewHandle]struct{})
	}
	b.handles[handle] = struct{}{}
	b.mu.Unlock()

	if err := handle.createNative(options); err != nil {
		handle.closed = true
		handle.destroyNative()
		return nil, err
	}
	if events != nil {
		events(scene.WebViewEvent{Kind: scene.WebViewEventReady})
	}
	return handle, nil
}

func (b *windowsWebViewBackend) Close() error {
	if b == nil {
		return nil
	}
	if currentWebViewThreadID() != b.thread {
		return errors.New("window: WebView2 backend must be closed on the window thread")
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.mu.Unlock()

	for {
		b.mu.Lock()
		var handle *windowsWebViewHandle
		for candidate := range b.handles {
			handle = candidate
			break
		}
		b.mu.Unlock()
		if handle == nil {
			break
		}
		if err := handle.Destroy(); err != nil {
			return err
		}
	}
	uninitializeWebViewCOM()
	return nil
}

func (b *windowsWebViewBackend) remove(handle *windowsWebViewHandle) {
	if b == nil {
		return
	}
	b.mu.Lock()
	delete(b.handles, handle)
	b.mu.Unlock()
}

type windowsWebViewCommandKind uint8

const (
	windowsWebViewNavigate windowsWebViewCommandKind = iota + 1
	windowsWebViewSetHTML
	windowsWebViewEval
	windowsWebViewSetBounds
	windowsWebViewSetVisible
	windowsWebViewDestroy
)

type windowsWebViewCommand struct {
	kind    windowsWebViewCommandKind
	text    string
	bounds  scene.WebViewBounds
	visible bool
}

type windowsWebViewHandle struct {
	backend *windowsWebViewBackend
	invoke  scene.WebViewInvoke
	events  func(scene.WebViewEvent)
	native  windowsWebViewNative

	mu     sync.Mutex
	closed bool
	queue  []windowsWebViewCommand
	work   []windowsWebViewCommand
}

func (h *windowsWebViewHandle) Navigate(url string) error {
	return h.dispatch(windowsWebViewCommand{kind: windowsWebViewNavigate, text: url})
}

func (h *windowsWebViewHandle) SetHTML(html string) error {
	return h.dispatch(windowsWebViewCommand{kind: windowsWebViewSetHTML, text: html})
}

func (h *windowsWebViewHandle) Eval(script string) error {
	return h.dispatch(windowsWebViewCommand{kind: windowsWebViewEval, text: script})
}

func (h *windowsWebViewHandle) SetBounds(bounds scene.WebViewBounds) error {
	return h.dispatch(windowsWebViewCommand{kind: windowsWebViewSetBounds, bounds: bounds})
}

func (h *windowsWebViewHandle) SetVisible(visible bool) error {
	return h.dispatch(windowsWebViewCommand{kind: windowsWebViewSetVisible, visible: visible})
}

func (h *windowsWebViewHandle) Destroy() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	if currentWebViewThreadID() == h.backend.thread {
		h.mu.Unlock()
		h.destroyNative()
		return nil
	}
	h.queue = append(h.queue, windowsWebViewCommand{kind: windowsWebViewDestroy})
	window := h.native.window
	h.mu.Unlock()
	if window == 0 || !postWebViewCommand(window) {
		return errors.New("window: failed to dispatch WebView2 destruction")
	}
	return nil
}

func (h *windowsWebViewHandle) dispatch(command windowsWebViewCommand) error {
	if h == nil {
		return errWindowsWebViewClosed
	}
	h.mu.Lock()
	if h.closed || h.native.window == 0 {
		h.mu.Unlock()
		return errWindowsWebViewClosed
	}
	if currentWebViewThreadID() == h.backend.thread {
		h.mu.Unlock()
		return h.execute(command)
	}
	h.queue = append(h.queue, command)
	window := h.native.window
	h.mu.Unlock()
	if !postWebViewCommand(window) {
		return errors.New("window: failed to dispatch WebView2 command")
	}
	return nil
}

func (h *windowsWebViewHandle) drainCommands() {
	h.mu.Lock()
	pending := h.queue
	h.queue = h.work[:0]
	h.work = nil
	h.mu.Unlock()

	for index := range pending {
		if err := h.execute(pending[index]); err != nil {
			h.reportError(err)
		}
	}
	clear(pending)
	h.mu.Lock()
	h.work = pending[:0]
	h.mu.Unlock()
}

func (h *windowsWebViewHandle) execute(command windowsWebViewCommand) error {
	switch command.kind {
	case windowsWebViewNavigate:
		return h.navigateNative(command.text)
	case windowsWebViewSetHTML:
		return h.setHTMLNative(command.text)
	case windowsWebViewEval:
		return h.evalNative(command.text)
	case windowsWebViewSetBounds:
		return h.setBoundsNative(command.bounds)
	case windowsWebViewSetVisible:
		return h.setVisibleNative(command.visible)
	case windowsWebViewDestroy:
		h.destroyNative()
		return nil
	default:
		return fmt.Errorf("window: unknown WebView2 command %d", command.kind)
	}
}

func (h *windowsWebViewHandle) reportError(err error) {
	if err != nil && h.events != nil {
		h.events(scene.WebViewEvent{Kind: scene.WebViewEventError, Error: err.Error()})
	}
}
