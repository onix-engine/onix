//go:build cgo && windows

package webview

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"github.com/diiyw/onix/scene"
	"github.com/ebitengine/purego"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	webViewWMSize              = 0x0005
	webViewWMSetFocus          = 0x0007
	webViewWMEraseBackground   = 0x0014
	webViewWMWindowPosition    = 0x0047
	webViewWMQuit              = 0x0012
	webViewWMDispatch          = 0x8000 + 0x157
	webViewWSChild             = 0x40000000
	webViewWSClipChildren      = 0x02000000
	webViewWSClipSiblings      = 0x04000000
	webViewSWShow              = 5
	webViewSWHide              = 0
	webViewSWPNoZOrder         = 0x0004
	webViewSWPNoActivate       = 0x0010
	webViewCOINITApartment     = 0x2
	webViewRuntimeInstalled    = 0
	webViewMoveFocusProgrammed = 0
)

const webView2ClientStateKey = `SOFTWARE\Microsoft\EdgeUpdate\ClientState\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

const (
	webView2IUnknownQueryInterface = iota
	webView2IUnknownAddRef
	webView2IUnknownRelease
)

const (
	webView2EnvironmentCreateController = 3
	webView2ControllerPutVisible        = 4
	webView2ControllerPutBounds         = 6
	webView2ControllerMoveFocus         = 12
	webView2ControllerNotifyPosition    = 23
	webView2ControllerClose             = 24
	webView2ControllerGetCore           = 25
	webView2ControllerPutBackground     = 27
	webView2CoreGetSettings             = 3
	webView2CoreGetSource               = 4
	webView2CoreNavigate                = 5
	webView2CoreNavigateToString        = 6
	webView2CoreAddNavigationCompleted  = 15
	webView2CoreAddDocumentScript       = 27
	webView2CoreExecuteScript           = 29
	webView2CorePostMessageString       = 33
	webView2CoreAddMessageReceived      = 34
	webView2SettingsPutMessagesEnabled  = 6
	webView2SettingsPutDevToolsEnabled  = 12
	webView2SettingsPutContextMenus     = 14
	webView2MessageArgsGetString        = 5
	webView2NavigationArgsGetSuccess    = 3
	webView2NavigationArgsGetError      = 4
)

type webViewRect struct {
	left, top, right, bottom int32
}

type webViewPoint struct {
	x, y int32
}

type webViewMessage struct {
	window  uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	point   webViewPoint
	private uint32
}

type webViewWndClass struct {
	size        uint32
	style       uint32
	wndProc     uintptr
	classExtra  int32
	windowExtra int32
	instance    uintptr
	icon        uintptr
	cursor      uintptr
	background  uintptr
	menuName    *uint16
	className   *uint16
	iconSmall   uintptr
}

var (
	webViewKernel32 = syscall.NewLazyDLL("kernel32.dll").Handle()
	webViewUser32   = syscall.NewLazyDLL("user32.dll").Handle()
	webViewOle32    = syscall.NewLazyDLL("ole32.dll").Handle()

	webViewGetModuleHandle    func(*uint16) uintptr
	webViewGetCurrentThreadID func() uint32
	webViewRegisterClass      func(*webViewWndClass) uint16
	webViewCreateWindow       func(uint32, *uint16, *uint16, uint32, int32, int32, int32, int32, uintptr, uintptr, uintptr, unsafe.Pointer) uintptr
	webViewDestroyWindow      func(uintptr) bool
	webViewDefWindowProc      func(uintptr, uint32, uintptr, uintptr) uintptr
	webViewShowWindow         func(uintptr, int32) bool
	webViewSetWindowPos       func(uintptr, uintptr, int32, int32, int32, int32, uint32) bool
	webViewGetClientRect      func(uintptr, *webViewRect) bool
	webViewPostMessage        func(uintptr, uint32, uintptr, uintptr) bool
	webViewGetMessage         func(*webViewMessage, uintptr, uint32, uint32) int32
	webViewTranslateMessage   func(*webViewMessage) bool
	webViewDispatchMessage    func(*webViewMessage) uintptr
	webViewPostQuitMessage    func(int32)
	webViewCoInitializeEx     func(unsafe.Pointer, uint32) uintptr
	webViewCoUninitialize     func()

	webViewHostClassName = syscall.StringToUTF16("OnixWebView2Host")
	webViewHostClassOnce sync.Once
	webViewHostClassErr  error

	webViewHostsMu sync.RWMutex
	webViewHosts   = make(map[uintptr]*windowsWebViewHandle)
)

func init() {
	purego.RegisterLibFunc(&webViewGetModuleHandle, webViewKernel32, "GetModuleHandleW")
	purego.RegisterLibFunc(&webViewGetCurrentThreadID, webViewKernel32, "GetCurrentThreadId")
	purego.RegisterLibFunc(&webViewRegisterClass, webViewUser32, "RegisterClassExW")
	purego.RegisterLibFunc(&webViewCreateWindow, webViewUser32, "CreateWindowExW")
	purego.RegisterLibFunc(&webViewDestroyWindow, webViewUser32, "DestroyWindow")
	purego.RegisterLibFunc(&webViewDefWindowProc, webViewUser32, "DefWindowProcW")
	purego.RegisterLibFunc(&webViewShowWindow, webViewUser32, "ShowWindow")
	purego.RegisterLibFunc(&webViewSetWindowPos, webViewUser32, "SetWindowPos")
	purego.RegisterLibFunc(&webViewGetClientRect, webViewUser32, "GetClientRect")
	purego.RegisterLibFunc(&webViewPostMessage, webViewUser32, "PostMessageW")
	purego.RegisterLibFunc(&webViewGetMessage, webViewUser32, "GetMessageW")
	purego.RegisterLibFunc(&webViewTranslateMessage, webViewUser32, "TranslateMessage")
	purego.RegisterLibFunc(&webViewDispatchMessage, webViewUser32, "DispatchMessageW")
	purego.RegisterLibFunc(&webViewPostQuitMessage, webViewUser32, "PostQuitMessage")
	purego.RegisterLibFunc(&webViewCoInitializeEx, webViewOle32, "CoInitializeEx")
	purego.RegisterLibFunc(&webViewCoUninitialize, webViewOle32, "CoUninitialize")
}

func currentWebViewThreadID() uint32 {
	return webViewGetCurrentThreadID()
}

func initializeWebViewCOM() error {
	result := webViewCoInitializeEx(nil, webViewCOINITApartment)
	if hresultFailed(result) {
		return hresultError("CoInitializeEx", result)
	}
	return nil
}

func uninitializeWebViewCOM() {
	webViewCoUninitialize()
}

func registerWebViewHostClass() error {
	webViewHostClassOnce.Do(func() {
		class := webViewWndClass{
			size:      uint32(unsafe.Sizeof(webViewWndClass{})),
			wndProc:   syscall.NewCallback(webViewHostWindowProc),
			instance:  webViewGetModuleHandle(nil),
			className: &webViewHostClassName[0],
		}
		if webViewRegisterClass(&class) == 0 {
			webViewHostClassErr = fmt.Errorf("window: register WebView2 host class: %w", syscall.GetLastError())
		}
	})
	return webViewHostClassErr
}

func webViewHostWindowProc(window uintptr, message uint32, wParam, lParam uintptr) uintptr {
	webViewHostsMu.RLock()
	handle := webViewHosts[window]
	webViewHostsMu.RUnlock()
	if handle == nil {
		return webViewDefWindowProc(window, message, wParam, lParam)
	}
	switch message {
	case webViewWMDispatch:
		handle.drainCommands()
		return 0
	case webViewWMSize:
		if err := handle.resizeNative(); err != nil {
			handle.reportError(err)
		}
		return 0
	case webViewWMSetFocus:
		if err := handle.focusNative(); err != nil {
			handle.reportError(err)
		}
		return 0
	case webViewWMWindowPosition:
		if err := handle.notifyPositionNative(); err != nil {
			handle.reportError(err)
		}
	case webViewWMEraseBackground:
		return 1
	}
	return webViewDefWindowProc(window, message, wParam, lParam)
}

func postWebViewCommand(window uintptr) bool {
	return webViewPostMessage(window, webViewWMDispatch, 0, 0)
}

type webView2Environment struct{ vtable unsafe.Pointer }
type webView2Controller struct{ vtable unsafe.Pointer }
type webView2Core struct{ vtable unsafe.Pointer }
type webView2Settings struct{ vtable unsafe.Pointer }
type webView2MessageArgs struct{ vtable unsafe.Pointer }
type webView2NavigationArgs struct{ vtable unsafe.Pointer }

type webView2EventToken struct {
	value int64
}

type webView2CallbackKind uint8

const (
	webView2EnvironmentCreated webView2CallbackKind = iota + 1
	webView2ControllerCreated
	webView2MessageReceived
	webView2NavigationCompleted
)

type webView2CallbackVTable struct {
	queryInterface uintptr
	addRef         uintptr
	release        uintptr
	invoke         uintptr
}

type webView2Callback struct {
	vtable *webView2CallbackVTable
	owner  *windowsWebViewHandle
	iid    windows.GUID
	refs   atomic.Uint32
	kind   webView2CallbackKind
}

var webView2CallbackMethods = webView2CallbackVTable{
	queryInterface: syscall.NewCallback(webView2CallbackQueryInterface),
	addRef:         syscall.NewCallback(webView2CallbackAddRef),
	release:        syscall.NewCallback(webView2CallbackRelease),
	invoke:         syscall.NewCallback(webView2CallbackInvoke),
}

var (
	webView2IIDUnknown             = windows.GUID{Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	webView2IIDEnvironmentCreated  = windows.GUID{Data1: 0x4e8a3389, Data2: 0xc9d8, Data3: 0x4bd2, Data4: [8]byte{0xb6, 0xb5, 0x12, 0x4f, 0xee, 0x6c, 0xc1, 0x4d}}
	webView2IIDControllerCreated   = windows.GUID{Data1: 0x6c4819f3, Data2: 0xc9b7, Data3: 0x4260, Data4: [8]byte{0x81, 0x27, 0xc9, 0xf5, 0xbd, 0xe7, 0xf6, 0x8c}}
	webView2IIDMessageReceived     = windows.GUID{Data1: 0x57213f19, Data2: 0x00e6, Data3: 0x49fa, Data4: [8]byte{0x8e, 0x07, 0x89, 0x8e, 0xa0, 0x1e, 0xcb, 0xd2}}
	webView2IIDNavigationCompleted = windows.GUID{Data1: 0xd33a35bf, Data2: 0x1c49, Data3: 0x4f98, Data4: [8]byte{0x93, 0xab, 0x00, 0x6e, 0x05, 0x33, 0xfe, 0x1c}}
	webView2IIDController2         = windows.GUID{Data1: 0xc979903e, Data2: 0xd4ca, Data3: 0x4228, Data4: [8]byte{0x92, 0xeb, 0x47, 0xee, 0x3f, 0xa9, 0x6e, 0xab}}
)

func (callback *webView2Callback) initialize(owner *windowsWebViewHandle, kind webView2CallbackKind, iid windows.GUID) {
	callback.vtable = &webView2CallbackMethods
	callback.owner = owner
	callback.kind = kind
	callback.iid = iid
	callback.refs.Store(1)
}

func webView2CallbackQueryInterface(callback *webView2Callback, iid *windows.GUID, object *unsafe.Pointer) uintptr {
	if callback == nil || iid == nil || object == nil {
		return uintptr(uint32(0x80004003))
	}
	*object = nil
	if *iid != webView2IIDUnknown && *iid != callback.iid {
		return uintptr(uint32(0x80004002))
	}
	*object = unsafe.Pointer(callback)
	webView2CallbackAddRef(callback)
	return 0
}

func webView2CallbackAddRef(callback *webView2Callback) uintptr {
	if callback == nil {
		return 0
	}
	return uintptr(callback.refs.Add(1))
}

func webView2CallbackRelease(callback *webView2Callback) uintptr {
	if callback == nil {
		return 0
	}
	for {
		current := callback.refs.Load()
		if current == 0 {
			return 0
		}
		if callback.refs.CompareAndSwap(current, current-1) {
			return uintptr(current - 1)
		}
	}
}

func webView2CallbackInvoke(callback *webView2Callback, first, second unsafe.Pointer) uintptr {
	if callback == nil || callback.owner == nil {
		return 0
	}
	switch callback.kind {
	case webView2EnvironmentCreated:
		callback.owner.environmentCreated(uintptr(first), second)
	case webView2ControllerCreated:
		callback.owner.controllerCreated(uintptr(first), second)
	case webView2MessageReceived:
		callback.owner.messageReceived(second)
	case webView2NavigationCompleted:
		callback.owner.navigationCompleted(first, second)
	}
	return 0
}

type windowsWebViewNative struct {
	window      uintptr
	environment *webView2Environment
	controller  *webView2Controller
	core        *webView2Core

	environmentCallback webView2Callback
	controllerCallback  webView2Callback
	messageCallback     webView2Callback
	navigationCallback  webView2Callback
	messageToken        webView2EventToken
	navigationToken     webView2EventToken

	pinner      runtime.Pinner
	pinned      bool
	initialized bool
	visible     bool
	transparent bool
	debug       bool
	initErr     error
	response    []byte
	wide        []uint16
}

func (h *windowsWebViewHandle) createNative(options scene.WebViewOptions) error {
	ratio := h.backend.context.DevicePixelRatio()
	x, y, width, height := nativeWebViewBounds(options.Bounds, ratio)
	style := uint32(webViewWSChild | webViewWSClipChildren | webViewWSClipSiblings)
	h.native.visible = options.Visible
	h.native.transparent = options.Transparent
	h.native.debug = options.DevTools
	h.native.window = webViewCreateWindow(
		0,
		&webViewHostClassName[0],
		nil,
		style,
		x,
		y,
		width,
		height,
		h.backend.parent,
		0,
		webViewGetModuleHandle(nil),
		nil,
	)
	if h.native.window == 0 {
		return fmt.Errorf("window: create WebView2 host: %w", syscall.GetLastError())
	}
	webViewHostsMu.Lock()
	webViewHosts[h.native.window] = h
	webViewHostsMu.Unlock()

	if err := h.initializeNative(); err != nil {
		return err
	}
	if err := h.addInitialScriptNative(windowsWebViewBridgeScript); err != nil {
		return err
	}
	if options.URL != "" {
		if err := h.navigateNative(options.URL); err != nil {
			return err
		}
	} else if err := h.setHTMLNative(options.HTML); err != nil {
		return err
	}
	if err := h.setVisibleNative(options.Visible); err != nil {
		return err
	}
	return nil
}

func (h *windowsWebViewHandle) initializeNative() error {
	h.native.environmentCallback.initialize(h, webView2EnvironmentCreated, webView2IIDEnvironmentCreated)
	h.native.controllerCallback.initialize(h, webView2ControllerCreated, webView2IIDControllerCreated)
	h.native.messageCallback.initialize(h, webView2MessageReceived, webView2IIDMessageReceived)
	h.native.navigationCallback.initialize(h, webView2NavigationCompleted, webView2IIDNavigationCompleted)
	h.native.pinner.Pin(h)
	h.native.pinner.Pin(&h.native.environmentCallback)
	h.native.pinner.Pin(&h.native.controllerCallback)
	h.native.pinner.Pin(&h.native.messageCallback)
	h.native.pinner.Pin(&h.native.navigationCallback)
	h.native.pinned = true

	procedure, err := loadWebView2Runtime()
	if err != nil {
		return err
	}
	userData, err := syscall.UTF16FromString(webView2UserDataDirectory())
	if err != nil {
		return fmt.Errorf("window: WebView2 user data path: %w", err)
	}
	result, _, _ := procedure.Call(
		1,
		webViewRuntimeInstalled,
		uintptr(unsafe.Pointer(&userData[0])),
		0,
		uintptr(unsafe.Pointer(&h.native.environmentCallback)),
	)
	runtime.KeepAlive(userData)
	if hresultFailed(result) {
		return hresultError("CreateWebViewEnvironmentWithOptionsInternal", result)
	}

	for !h.native.initialized && h.native.initErr == nil {
		var message webViewMessage
		result := webViewGetMessage(&message, 0, 0, 0)
		if result < 0 {
			return errors.New("window: WebView2 initialization message loop failed")
		}
		if result == 0 || message.message == webViewWMQuit {
			webViewPostQuitMessage(0)
			return errors.New("window: WebView2 initialization was interrupted")
		}
		webViewTranslateMessage(&message)
		webViewDispatchMessage(&message)
	}
	return h.native.initErr
}

func (h *windowsWebViewHandle) environmentCreated(result uintptr, environment unsafe.Pointer) {
	if hresultFailed(result) {
		h.native.initErr = hresultError("create WebView2 environment", result)
		return
	}
	if environment == nil {
		h.native.initErr = errors.New("window: WebView2 returned a nil environment")
		return
	}
	h.native.environment = (*webView2Environment)(environment)
	comAddRef(h.native.environment)
	result = comCall2(
		h.native.environment,
		webView2EnvironmentCreateController,
		h.native.window,
		uintptr(unsafe.Pointer(&h.native.controllerCallback)),
	)
	if hresultFailed(result) {
		h.native.initErr = hresultError("create WebView2 controller", result)
	}
}

func (h *windowsWebViewHandle) controllerCreated(result uintptr, controller unsafe.Pointer) {
	if hresultFailed(result) {
		h.native.initErr = hresultError("create WebView2 controller", result)
		return
	}
	if controller == nil {
		h.native.initErr = errors.New("window: WebView2 returned a nil controller")
		return
	}
	h.native.controller = (*webView2Controller)(controller)
	comAddRef(h.native.controller)
	if result = comCall1(h.native.controller, webView2ControllerGetCore, uintptr(unsafe.Pointer(&h.native.core))); hresultFailed(result) {
		h.native.initErr = hresultError("get WebView2 core", result)
		return
	}
	if h.native.core == nil {
		h.native.initErr = errors.New("window: WebView2 returned a nil core")
		return
	}
	if err := h.configureNative(); err != nil {
		h.native.initErr = err
		return
	}
	h.native.initialized = true
}

func (h *windowsWebViewHandle) configureNative() error {
	var settings *webView2Settings
	if result := comCall1(h.native.core, webView2CoreGetSettings, uintptr(unsafe.Pointer(&settings))); hresultFailed(result) {
		return hresultError("get WebView2 settings", result)
	}
	if settings == nil {
		return errors.New("window: WebView2 returned nil settings")
	}
	defer comRelease(settings)

	if result := comCall1(settings, webView2SettingsPutMessagesEnabled, 1); hresultFailed(result) {
		return hresultError("enable WebView2 messages", result)
	}
	debug := uintptr(0)
	if h.native.debug {
		debug = 1
	}
	if result := comCall1(settings, webView2SettingsPutDevToolsEnabled, debug); hresultFailed(result) {
		return hresultError("configure WebView2 developer tools", result)
	}
	if result := comCall1(settings, webView2SettingsPutContextMenus, debug); hresultFailed(result) {
		return hresultError("configure WebView2 context menus", result)
	}
	if result := comCall2(
		h.native.core,
		webView2CoreAddMessageReceived,
		uintptr(unsafe.Pointer(&h.native.messageCallback)),
		uintptr(unsafe.Pointer(&h.native.messageToken)),
	); hresultFailed(result) {
		return hresultError("register WebView2 message handler", result)
	}
	if result := comCall2(
		h.native.core,
		webView2CoreAddNavigationCompleted,
		uintptr(unsafe.Pointer(&h.native.navigationCallback)),
		uintptr(unsafe.Pointer(&h.native.navigationToken)),
	); hresultFailed(result) {
		return hresultError("register WebView2 navigation handler", result)
	}
	if h.native.transparent {
		var controller2 *webView2Controller
		if result := comCall2(
			h.native.controller,
			webView2IUnknownQueryInterface,
			uintptr(unsafe.Pointer(&webView2IIDController2)),
			uintptr(unsafe.Pointer(&controller2)),
		); hresultFailed(result) {
			return hresultError("get WebView2 transparent controller", result)
		}
		if controller2 == nil {
			return errors.New("window: WebView2 returned a nil transparent controller")
		}
		result := comCall1(controller2, webView2ControllerPutBackground, 0)
		comRelease(controller2)
		if hresultFailed(result) {
			return hresultError("set WebView2 transparent background", result)
		}
	}
	if err := h.resizeNative(); err != nil {
		return err
	}
	return h.setControllerVisibilityNative(h.native.visible)
}

func (h *windowsWebViewHandle) addInitialScriptNative(script string) error {
	return callCOMString2(h.native.core, webView2CoreAddDocumentScript, &h.native.wide, script, 0, "add WebView2 initialization script")
}

func (h *windowsWebViewHandle) navigateNative(url string) error {
	return callCOMString1(h.native.core, webView2CoreNavigate, &h.native.wide, url, "navigate WebView2")
}

func (h *windowsWebViewHandle) setHTMLNative(html string) error {
	return callCOMString1(h.native.core, webView2CoreNavigateToString, &h.native.wide, html, "set WebView2 HTML")
}

func (h *windowsWebViewHandle) evalNative(script string) error {
	return callCOMString2(h.native.core, webView2CoreExecuteScript, &h.native.wide, script, 0, "execute WebView2 script")
}

func (h *windowsWebViewHandle) setBoundsNative(bounds scene.WebViewBounds) error {
	if h.native.window == 0 {
		return errWindowsWebViewClosed
	}
	x, y, width, height := nativeWebViewBounds(bounds, h.backend.context.DevicePixelRatio())
	if !webViewSetWindowPos(
		h.native.window,
		0,
		x,
		y,
		width,
		height,
		webViewSWPNoZOrder|webViewSWPNoActivate,
	) {
		return fmt.Errorf("window: resize WebView2 host: %w", syscall.GetLastError())
	}
	return h.resizeNative()
}

func (h *windowsWebViewHandle) resizeNative() error {
	if h.native.controller == nil || h.native.window == 0 {
		return nil
	}
	var bounds webViewRect
	if !webViewGetClientRect(h.native.window, &bounds) {
		return fmt.Errorf("window: get WebView2 host bounds: %w", syscall.GetLastError())
	}
	result := putWebViewControllerBounds(h.native.controller, bounds)
	if hresultFailed(result) {
		return hresultError("resize WebView2 controller", result)
	}
	return nil
}

func (h *windowsWebViewHandle) setVisibleNative(visible bool) error {
	if h.native.window == 0 {
		return errWindowsWebViewClosed
	}
	command := int32(webViewSWHide)
	if visible {
		command = webViewSWShow
	}
	webViewShowWindow(h.native.window, command)
	h.native.visible = visible
	return h.setControllerVisibilityNative(visible)
}

func (h *windowsWebViewHandle) setControllerVisibilityNative(visible bool) error {
	if h.native.controller == nil {
		return nil
	}
	value := uintptr(0)
	if visible {
		value = 1
	}
	if result := comCall1(h.native.controller, webView2ControllerPutVisible, value); hresultFailed(result) {
		return hresultError("set WebView2 visibility", result)
	}
	return nil
}

func (h *windowsWebViewHandle) focusNative() error {
	if h.native.controller == nil {
		return nil
	}
	if result := comCall1(h.native.controller, webView2ControllerMoveFocus, webViewMoveFocusProgrammed); hresultFailed(result) {
		return hresultError("focus WebView2", result)
	}
	return nil
}

func (h *windowsWebViewHandle) notifyPositionNative() error {
	if h.native.controller == nil {
		return nil
	}
	if result := comCall0(h.native.controller, webView2ControllerNotifyPosition); hresultFailed(result) {
		return hresultError("notify WebView2 position", result)
	}
	return nil
}

func (h *windowsWebViewHandle) messageReceived(arguments unsafe.Pointer) {
	if arguments == nil {
		h.reportError(errors.New("window: WebView2 delivered nil message arguments"))
		return
	}
	var value *uint16
	if result := comCall1(
		(*webView2MessageArgs)(arguments),
		webView2MessageArgsGetString,
		uintptr(unsafe.Pointer(&value)),
	); hresultFailed(result) {
		h.reportError(hresultError("read WebView2 message", result))
		return
	}
	message := windows.UTF16PtrToString(value)
	windows.CoTaskMemFree(unsafe.Pointer(value))

	var request webViewBridgeRequest
	if err := decodeWebViewBridgeRequest(message, &request); err != nil {
		h.reportError(fmt.Errorf("window: decode WebView2 bridge request: %w", err))
		return
	}
	var result string
	var invokeErr error
	if h.invoke == nil {
		invokeErr = errWindowsWebViewBridge
	} else {
		result, invokeErr = h.invoke(request.Name, request.Payload)
	}
	h.native.response = appendWebViewBridgeResponse(h.native.response, request.ID, result, invokeErr)
	if err := h.postResponseNative(h.native.response); err != nil {
		h.reportError(err)
	}
}

func (h *windowsWebViewHandle) postResponseNative(response []byte) error {
	h.native.wide = appendUTF16Bytes(h.native.wide[:0], response)
	h.native.wide = append(h.native.wide, 0)
	result := comCall1(h.native.core, webView2CorePostMessageString, uintptr(unsafe.Pointer(&h.native.wide[0])))
	runtime.KeepAlive(h.native.wide)
	if hresultFailed(result) {
		return hresultError("post WebView2 bridge response", result)
	}
	return nil
}

func (h *windowsWebViewHandle) navigationCompleted(_ unsafe.Pointer, arguments unsafe.Pointer) {
	if arguments == nil {
		h.reportError(errors.New("window: WebView2 delivered nil navigation arguments"))
		return
	}
	args := (*webView2NavigationArgs)(arguments)
	var success int32
	if result := comCall1(args, webView2NavigationArgsGetSuccess, uintptr(unsafe.Pointer(&success))); hresultFailed(result) {
		h.reportError(hresultError("read WebView2 navigation result", result))
		return
	}
	if success == 0 {
		var status uint32
		if result := comCall1(args, webView2NavigationArgsGetError, uintptr(unsafe.Pointer(&status))); hresultFailed(result) {
			h.reportError(hresultError("read WebView2 navigation error", result))
		} else {
			h.reportError(fmt.Errorf("window: WebView2 navigation failed with status %d", status))
		}
		return
	}
	if h.events == nil {
		return
	}
	source, err := h.sourceNative()
	if err != nil {
		h.reportError(err)
		return
	}
	h.events(scene.WebViewEvent{Kind: scene.WebViewEventNavigation, URL: source})
}

func (h *windowsWebViewHandle) sourceNative() (string, error) {
	var value *uint16
	if result := comCall1(h.native.core, webView2CoreGetSource, uintptr(unsafe.Pointer(&value))); hresultFailed(result) {
		return "", hresultError("read WebView2 source", result)
	}
	source := windows.UTF16PtrToString(value)
	windows.CoTaskMemFree(unsafe.Pointer(value))
	return source, nil
}

func (h *windowsWebViewHandle) destroyNative() {
	window := h.native.window
	if window != 0 {
		webViewHostsMu.Lock()
		delete(webViewHosts, window)
		webViewHostsMu.Unlock()
		h.native.window = 0
	}
	if h.native.controller != nil {
		comCall0(h.native.controller, webView2ControllerClose)
	}
	if h.native.core != nil {
		comRelease(h.native.core)
		h.native.core = nil
	}
	if h.native.controller != nil {
		comRelease(h.native.controller)
		h.native.controller = nil
	}
	if h.native.environment != nil {
		comRelease(h.native.environment)
		h.native.environment = nil
	}
	h.native.environmentCallback.owner = nil
	h.native.controllerCallback.owner = nil
	h.native.messageCallback.owner = nil
	h.native.navigationCallback.owner = nil
	if h.native.pinned {
		h.native.pinner.Unpin()
		h.native.pinned = false
	}
	if window != 0 {
		webViewDestroyWindow(window)
	}
	if h.backend != nil {
		h.backend.remove(h)
	}
}

func nativeWebViewBounds(bounds scene.WebViewBounds, ratio float64) (int32, int32, int32, int32) {
	return nativeWebViewCoordinate(bounds.X, ratio),
		nativeWebViewCoordinate(bounds.Y, ratio),
		nativeWebViewDimension(bounds.Width, ratio),
		nativeWebViewDimension(bounds.Height, ratio)
}

func nativeWebViewCoordinate(value float32, ratio float64) int32 {
	scaled := math.Round(float64(value) * ratio)
	if scaled < math.MinInt32 {
		return math.MinInt32
	}
	if scaled > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(scaled)
}

func nativeWebViewDimension(value float32, ratio float64) int32 {
	scaled := math.Round(float64(value) * ratio)
	if scaled < 1 {
		return 1
	}
	if scaled > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(scaled)
}

func appendUTF16Bytes(destination []uint16, source []byte) []uint16 {
	for len(source) > 0 {
		r, size := utf8.DecodeRune(source)
		source = source[size:]
		if r <= 0xffff {
			destination = append(destination, uint16(r))
			continue
		}
		high, low := utf16.EncodeRune(r)
		destination = append(destination, uint16(high), uint16(low))
	}
	return destination
}

func appendUTF16String(destination []uint16, value string) ([]uint16, error) {
	if strings.IndexByte(value, 0) >= 0 {
		return destination[:0], syscall.EINVAL
	}
	for _, r := range value {
		if r <= 0xffff {
			destination = append(destination, uint16(r))
			continue
		}
		high, low := utf16.EncodeRune(r)
		destination = append(destination, uint16(high), uint16(low))
	}
	return append(destination, 0), nil
}

func callCOMString1[T any](object *T, slot int, scratch *[]uint16, value, operation string) error {
	wide, err := appendUTF16String((*scratch)[:0], value)
	if err != nil {
		return fmt.Errorf("window: %s: %w", operation, err)
	}
	*scratch = wide
	result := comCall1(object, slot, uintptr(unsafe.Pointer(&wide[0])))
	runtime.KeepAlive(wide)
	if hresultFailed(result) {
		return hresultError(operation, result)
	}
	return nil
}

func callCOMString2[T any](object *T, slot int, scratch *[]uint16, value string, second uintptr, operation string) error {
	wide, err := appendUTF16String((*scratch)[:0], value)
	if err != nil {
		return fmt.Errorf("window: %s: %w", operation, err)
	}
	*scratch = wide
	result := comCall2(object, slot, uintptr(unsafe.Pointer(&wide[0])), second)
	runtime.KeepAlive(wide)
	if hresultFailed(result) {
		return hresultError(operation, result)
	}
	return nil
}

func comMethod[T any](object *T, slot int) uintptr {
	vtable := *(*unsafe.Pointer)(unsafe.Pointer(object))
	return (*[64]uintptr)(vtable)[slot]
}

//go:uintptrescapes
func invokeCOM(procedure uintptr, arguments ...uintptr) uintptr {
	result, _, _ := syscall.SyscallN(procedure, arguments...)
	return result
}

func comCall0[T any](object *T, slot int) uintptr {
	return invokeCOM(comMethod(object, slot), uintptr(unsafe.Pointer(object)))
}

func comCall1[T any](object *T, slot int, first uintptr) uintptr {
	return invokeCOM(comMethod(object, slot), uintptr(unsafe.Pointer(object)), first)
}

func comCall2[T any](object *T, slot int, first, second uintptr) uintptr {
	return invokeCOM(comMethod(object, slot), uintptr(unsafe.Pointer(object)), first, second)
}

func comAddRef[T any](object *T) uintptr {
	return comCall0(object, webView2IUnknownAddRef)
}

func comRelease[T any](object *T) uintptr {
	return comCall0(object, webView2IUnknownRelease)
}

func hresultFailed(result uintptr) bool {
	return int32(uint32(result)) < 0
}

func hresultError(operation string, result uintptr) error {
	return fmt.Errorf("window: %s failed (HRESULT 0x%08X)", operation, uint32(result))
}

var webView2Runtime struct {
	once      sync.Once
	library   *syscall.DLL
	procedure *syscall.Proc
	err       error
}

func loadWebView2Runtime() (*syscall.Proc, error) {
	webView2Runtime.once.Do(func() {
		// WebView2Loader performs this same stable-runtime lookup before calling
		// the runtime export. Keeping it here removes the binary sidecar.
		directory, err := installedWebView2Directory()
		if err != nil {
			webView2Runtime.err = err
			return
		}
		var architecture string
		switch runtime.GOARCH {
		case "amd64":
			architecture = "x64"
		case "386":
			architecture = "x86"
		case "arm64":
			architecture = "arm64"
		default:
			webView2Runtime.err = fmt.Errorf("window: WebView2 does not support %s", runtime.GOARCH)
			return
		}
		path := filepath.Join(directory, "EBWebView", architecture, "EmbeddedBrowserWebView.dll")
		library, err := syscall.LoadDLL(path)
		if err != nil {
			webView2Runtime.err = fmt.Errorf("window: load WebView2 runtime %q: %w", path, err)
			return
		}
		procedure, err := library.FindProc("CreateWebViewEnvironmentWithOptionsInternal")
		if err != nil {
			library.Release()
			webView2Runtime.err = fmt.Errorf("window: load WebView2 entry point: %w", err)
			return
		}
		webView2Runtime.library = library
		webView2Runtime.procedure = procedure
	})
	return webView2Runtime.procedure, webView2Runtime.err
}

func installedWebView2Directory() (string, error) {
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		key, err := registry.OpenKey(root, webView2ClientStateKey, registry.QUERY_VALUE|registry.WOW64_32KEY)
		if err != nil {
			continue
		}
		directory, _, valueErr := key.GetStringValue("EBWebView")
		key.Close()
		if valueErr == nil && directory != "" {
			return directory, nil
		}
	}
	return "", errors.New("window: Microsoft WebView2 Runtime is not installed")
}

func webView2UserDataDirectory() string {
	cache, err := os.UserCacheDir()
	if err != nil || cache == "" {
		cache = os.TempDir()
	}
	executable, err := os.Executable()
	if err != nil {
		return filepath.Join(cache, "Onix", "WebView2")
	}
	name := filepath.Base(executable)
	return filepath.Join(cache, "Onix", name+".WebView2")
}
