//go:build android

package webview

import (
	"errors"
	"sync"

	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/window"
)

type NativeCommand uint8

const (
	NativeCommandCreate NativeCommand = iota + 1
	NativeCommandNavigate
	NativeCommandSetHTML
	NativeCommandEval
	NativeCommandSetBounds
	NativeCommandSetVisible
	NativeCommandDestroy
)

type NativeEvent uint8

const (
	NativeEventReady NativeEvent = iota + 1
	NativeEventNavigation
	NativeEventError
	NativeEventClosed
)

const (
	nativeFlagVisible uint8 = 1 << iota
	nativeFlagDevTools
	nativeFlagTransparent
	nativeFlagHTML
)

type androidWebViewCommand struct {
	sequence uint64
	id       uint64
	kind     NativeCommand
	flags    uint8
	text     string
	bounds   scene.WebViewBounds
}

type androidWebViewBackend struct {
	closed bool
}

type androidWebViewHandle struct {
	backend *androidWebViewBackend
	id      uint64
	invoke  scene.WebViewInvoke
	events  func(scene.WebViewEvent)
	result  string
	err     string
	closed  bool
}

var androidWebViews struct {
	sync.Mutex
	backend     *androidWebViewBackend
	handles     map[uint64]*androidWebViewHandle
	commands    []androidWebViewCommand
	commandHead int
	nextID      uint64
	nextCommand uint64
}

func newBackend(_ *window.Context) (scene.WebViewBackend, error) {
	androidWebViews.Lock()
	defer androidWebViews.Unlock()
	if androidWebViews.backend != nil && !androidWebViews.backend.closed {
		return nil, errors.New("window: Android WebView backend is already active")
	}
	backend := &androidWebViewBackend{}
	androidWebViews.backend = backend
	if androidWebViews.handles == nil {
		androidWebViews.handles = make(map[uint64]*androidWebViewHandle)
	}
	if androidWebViews.commands == nil {
		androidWebViews.commands = make([]androidWebViewCommand, 0, 8)
	}
	return backend, nil
}

func (b *androidWebViewBackend) Create(options scene.WebViewOptions, invoke scene.WebViewInvoke, events func(scene.WebViewEvent)) (scene.WebViewHandle, error) {
	androidWebViews.Lock()
	defer androidWebViews.Unlock()
	if b == nil || b.closed || androidWebViews.backend != b {
		return nil, errors.New("window: Android WebView backend is closed")
	}
	androidWebViews.nextID++
	handle := &androidWebViewHandle{backend: b, id: androidWebViews.nextID, invoke: invoke, events: events}
	androidWebViews.handles[handle.id] = handle
	flags := uint8(0)
	if options.Visible {
		flags |= nativeFlagVisible
	}
	if options.DevTools {
		flags |= nativeFlagDevTools
	}
	if options.Transparent {
		flags |= nativeFlagTransparent
	}
	text := options.URL
	if text == "" {
		flags |= nativeFlagHTML
		text = options.HTML
	}
	androidEnqueueCommandLocked(androidWebViewCommand{id: handle.id, kind: NativeCommandCreate, flags: flags, text: text, bounds: options.Bounds})
	return handle, nil
}

func (b *androidWebViewBackend) Close() error {
	androidWebViews.Lock()
	defer androidWebViews.Unlock()
	if b == nil || b.closed {
		return nil
	}
	for id, handle := range androidWebViews.handles {
		if handle.backend != b {
			continue
		}
		handle.closed = true
		androidEnqueueCommandLocked(androidWebViewCommand{id: id, kind: NativeCommandDestroy})
		delete(androidWebViews.handles, id)
	}
	b.closed = true
	if androidWebViews.backend == b {
		androidWebViews.backend = nil
	}
	return nil
}

func (h *androidWebViewHandle) Navigate(url string) error {
	return h.enqueue(androidWebViewCommand{kind: NativeCommandNavigate, text: url})
}

func (h *androidWebViewHandle) SetHTML(html string) error {
	return h.enqueue(androidWebViewCommand{kind: NativeCommandSetHTML, text: html})
}

func (h *androidWebViewHandle) Eval(script string) error {
	return h.enqueue(androidWebViewCommand{kind: NativeCommandEval, text: script})
}

func (h *androidWebViewHandle) SetBounds(bounds scene.WebViewBounds) error {
	return h.enqueue(androidWebViewCommand{kind: NativeCommandSetBounds, bounds: bounds})
}

func (h *androidWebViewHandle) SetVisible(visible bool) error {
	var flags uint8
	if visible {
		flags = nativeFlagVisible
	}
	return h.enqueue(androidWebViewCommand{kind: NativeCommandSetVisible, flags: flags})
}

func (h *androidWebViewHandle) Destroy() error {
	androidWebViews.Lock()
	defer androidWebViews.Unlock()
	if h == nil || h.closed {
		return nil
	}
	h.closed = true
	delete(androidWebViews.handles, h.id)
	androidEnqueueCommandLocked(androidWebViewCommand{id: h.id, kind: NativeCommandDestroy})
	return nil
}

func (h *androidWebViewHandle) enqueue(command androidWebViewCommand) error {
	androidWebViews.Lock()
	defer androidWebViews.Unlock()
	if h == nil || h.closed || h.backend == nil || h.backend.closed || androidWebViews.handles[h.id] != h {
		return errors.New("window: Android WebView is closed")
	}
	command.id = h.id
	androidEnqueueCommandLocked(command)
	return nil
}

func androidEnqueueCommandLocked(command androidWebViewCommand) {
	androidWebViews.nextCommand++
	command.sequence = androidWebViews.nextCommand
	androidWebViews.commands = append(androidWebViews.commands, command)
}

func androidNativeCommandLocked() *androidWebViewCommand {
	if androidWebViews.commandHead >= len(androidWebViews.commands) {
		return nil
	}
	return &androidWebViews.commands[androidWebViews.commandHead]
}

func NativeCommandValue() uint64 {
	androidWebViews.Lock()
	defer androidWebViews.Unlock()
	command := androidNativeCommandLocked()
	if command == nil {
		return 0
	}
	return command.sequence<<8 | uint64(command.kind)
}

func NativeCommandID() uint64 {
	androidWebViews.Lock()
	defer androidWebViews.Unlock()
	if command := androidNativeCommandLocked(); command != nil {
		return command.id
	}
	return 0
}

func NativeCommandFlags() uint8 {
	androidWebViews.Lock()
	defer androidWebViews.Unlock()
	if command := androidNativeCommandLocked(); command != nil {
		return command.flags
	}
	return 0
}

func NativeCommandText() string {
	androidWebViews.Lock()
	defer androidWebViews.Unlock()
	if command := androidNativeCommandLocked(); command != nil {
		return command.text
	}
	return ""
}

func NativeCommandBounds() scene.WebViewBounds {
	androidWebViews.Lock()
	defer androidWebViews.Unlock()
	if command := androidNativeCommandLocked(); command != nil {
		return command.bounds
	}
	return scene.WebViewBounds{}
}

func CompleteNativeCommand(sequence uint64) {
	androidWebViews.Lock()
	defer androidWebViews.Unlock()
	command := androidNativeCommandLocked()
	if command == nil || command.sequence != sequence {
		return
	}
	*command = androidWebViewCommand{}
	androidWebViews.commandHead++
	if androidWebViews.commandHead == len(androidWebViews.commands) {
		androidWebViews.commands = androidWebViews.commands[:0]
		androidWebViews.commandHead = 0
	}
}

func DispatchNativeEvent(id uint64, event NativeEvent, value string) {
	androidWebViews.Lock()
	handle := androidWebViews.handles[id]
	if handle != nil && event == NativeEventClosed {
		handle.closed = true
		delete(androidWebViews.handles, id)
	}
	androidWebViews.Unlock()
	if handle == nil || handle.events == nil {
		return
	}
	switch event {
	case NativeEventReady:
		handle.events(scene.WebViewEvent{Kind: scene.WebViewEventReady})
	case NativeEventNavigation:
		handle.events(scene.WebViewEvent{Kind: scene.WebViewEventNavigation, URL: value})
	case NativeEventError:
		handle.events(scene.WebViewEvent{Kind: scene.WebViewEventError, Error: value})
	case NativeEventClosed:
		handle.events(scene.WebViewEvent{Kind: scene.WebViewEventClosed})
	}
}

func DispatchNativeInvoke(id uint64, name, payload string) {
	androidWebViews.Lock()
	handle := androidWebViews.handles[id]
	androidWebViews.Unlock()
	if handle == nil {
		return
	}
	var result string
	var err error
	if handle.invoke == nil {
		err = errors.New("window: Android WebView bridge is not configured")
	} else {
		result, err = handle.invoke(name, payload)
	}
	androidWebViews.Lock()
	if androidWebViews.handles[id] == handle {
		handle.result = result
		handle.err = ""
		if err != nil {
			handle.result = ""
			handle.err = err.Error()
		}
	}
	androidWebViews.Unlock()
}

func NativeInvokeResult(id uint64) string {
	androidWebViews.Lock()
	defer androidWebViews.Unlock()
	if handle := androidWebViews.handles[id]; handle != nil {
		return handle.result
	}
	return ""
}

func NativeInvokeError(id uint64) string {
	androidWebViews.Lock()
	defer androidWebViews.Unlock()
	if handle := androidWebViews.handles[id]; handle != nil {
		return handle.err
	}
	return "window: Android WebView is closed"
}
