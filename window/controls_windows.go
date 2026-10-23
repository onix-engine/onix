//go:build windows && cgo

package window

import (
	"math"
	"syscall"
	"unsafe"

	"github.com/diiyw/onix/render"
	"github.com/ebitengine/purego"
)

const (
	gwlExStyle                  = -20
	gwlStyle                    = -16
	wsExToolWindow              = 0x00000080
	wsExAppWindow               = 0x00040000
	wsPopup                     = 0x80000000
	wsThickFrame                = 0x00040000
	wsMinimizeBox               = 0x00020000
	wsMaximizeBox               = 0x00010000
	wsSysMenu                   = 0x00080000
	wsMaximize                  = 0x01000000
	wsDisabled                  = 0x08000000
	wsVisible                   = 0x10000000
	wsMinimize                  = 0x20000000
	swHide                      = 0
	swMinimize                  = 6
	swMaximize                  = 3
	swRestore                   = 9
	swpNoMove                   = 0x0002
	swpNoSize                   = 0x0001
	swpFrameChanged             = 0x0020
	wmNCLButtonDown             = 0x00A1
	htCaption                   = 2
	htClient                    = 1
	htLeft                      = 10
	htRight                     = 11
	htBottom                    = 15
	htBottomLeft                = 16
	htBottomRight               = 17
	dwmwaWindowCornerPreference = 33
	dwmwaBorderColor            = 34
	dwmwaNCRenderingPolicy      = 2
	dwmWindowCornerDefault      = 0
	dwmWindowCornerDoNotRound   = 1
	dwmWindowCornerRound        = 2
	dwmNCRenderingUseStyle      = 0
	dwmNCRenderingDisabled      = 1
	dwmColorDefault             = 0xffffffff
	dwmColorNone                = 0xfffffffe
	hwndTopmost                 = ^hwnd(0)
	hwndNotTopmost              = ^hwnd(1)
	rgnOr                       = 2
	rgnAnd                      = 1
)

var (
	getWindowLongPtrW     func(window hwnd, index int32) uintptr
	setWindowLongPtrW     func(window hwnd, index int32, value uintptr) uintptr
	releaseCapture        func() bool
	isZoomed              func(window hwnd) bool
	getWindowRect         func(window hwnd, rect *rect) bool
	dwmSetWindowAttribute func(window hwnd, attribute uint32, value unsafe.Pointer, size uint32) int32
	createRectRgn         func(left, top, right, bottom int32) handle
	createRoundRectRgn    func(left, top, right, bottom, ellipseWidth, ellipseHeight int32) handle
	combineRgn            func(destination, source1, source2 handle, mode int32) int32
	setWindowRgn          func(window hwnd, region handle, redraw bool) int32
)

func init() {
	purego.RegisterLibFunc(&getWindowLongPtrW, user32, "GetWindowLongPtrW")
	purego.RegisterLibFunc(&setWindowLongPtrW, user32, "SetWindowLongPtrW")
	purego.RegisterLibFunc(&releaseCapture, user32, "ReleaseCapture")
	purego.RegisterLibFunc(&isZoomed, user32, "IsZoomed")
	purego.RegisterLibFunc(&getWindowRect, user32, "GetWindowRect")
	dwmapi := syscall.NewLazyDLL("dwmapi.dll").Handle()
	purego.RegisterLibFunc(&dwmSetWindowAttribute, dwmapi, "DwmSetWindowAttribute")
	purego.RegisterLibFunc(&createRectRgn, gdi32, "CreateRectRgn")
	purego.RegisterLibFunc(&createRoundRectRgn, gdi32, "CreateRoundRectRgn")
	purego.RegisterLibFunc(&combineRgn, gdi32, "CombineRgn")
	purego.RegisterLibFunc(&setWindowRgn, user32, "SetWindowRgn")
}

func (c *Context) SetTaskbarVisible(visible bool) {
	if c == nil || c.window == 0 {
		return
	}
	style := taskbarExtendedStyle(getWindowLongPtrW(c.window, gwlExStyle), visible)
	if c.shown {
		showWindow(c.window, swHide)
	}
	setWindowLongPtrW(c.window, gwlExStyle, style)
	setWindowPos(c.window, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged)
	if c.shown {
		showWindow(c.window, swShow)
	}
}

func taskbarExtendedStyle(style uintptr, visible bool) uintptr {
	if visible {
		return style&^wsExToolWindow | wsExAppWindow
	}
	return style&^wsExAppWindow | wsExToolWindow
}

func (c *Context) SetAlwaysOnTop(enabled bool) {
	if c == nil || c.window == 0 {
		return
	}
	insertAfter := hwndNotTopmost
	if enabled {
		insertAfter = hwndTopmost
	}
	setWindowPos(c.window, insertAfter, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
}

func (c *Context) SetInputRegion(rects []InputRegionRect) {
	if c == nil || inputRegionsEqual(c.inputRegion, rects) {
		return
	}
	c.inputRegion = cloneInputRegion(rects)
	c.applyWindowRegion()
}

func (c *Context) applyWindowRegion() {
	if c == nil || c.window == 0 {
		return
	}
	// Win10 ignores the DWM corner preference, so keep the native window
	// region in sync with the renderer's rounded surface.
	var region handle
	if c.roundedCorners && c.decorations == DecorationNone && !c.transparent && !c.fullscreen && !isZoomed(c.window) {
		radius := int32(math.Round(float64(c.cornerRadius) * c.devicePixelRatio))
		if radius < 1 {
			radius = 1
		}
		// SetWindowRgn clips the outer edge of the region; include the final
		// client pixel so the right and bottom border remain visible.
		region = createRoundRectRgn(0, 0, int32(c.Width+1), int32(c.Height+1), radius*2, radius*2)
	}
	if c.inputRegion != nil {
		inputRegion := createRectRgn(0, 0, 0, 0)
		if inputRegion == 0 {
			if region != 0 {
				deleteObject(region)
			}
			return
		}
		for _, rect := range c.viewport.nativeInputRegion(c.inputRegion) {
			part := createRectRgn(
				int32(rect.X),
				int32(rect.Y),
				int32(rect.X+rect.Width),
				int32(rect.Y+rect.Height),
			)
			if part == 0 {
				continue
			}
			combineRgn(inputRegion, inputRegion, part, rgnOr)
			deleteObject(part)
		}
		if region == 0 {
			region = inputRegion
		} else {
			combineRgn(region, region, inputRegion, rgnAnd)
			deleteObject(inputRegion)
		}
	}
	if region == 0 {
		setWindowRgn(c.window, 0, true)
		return
	}
	if setWindowRgn(c.window, region, true) == 0 {
		deleteObject(region)
	}
}

func (c *Context) SetDecorations(mode DecorationMode) {
	if c == nil || c.window == 0 {
		return
	}
	c.decorations = mode
	c.applyWindowStyle()
	c.SetRoundedCorners(mode == DecorationNone && !c.transparent)
	c.applyNonClientAppearance()
	setWindowPos(c.window, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged)
}

func (c *Context) SetTitleBarHeight(float64) {}

func (c *Context) SetShadowVisible(visible bool) {
	if c == nil {
		return
	}
	c.shadow = visible
	c.applyNonClientAppearance()
}

func (c *Context) SetBorderVisible(visible bool) {
	if c == nil {
		return
	}
	c.border = visible
	c.applyNonClientAppearance()
}

func (c *Context) UsesClientWindowBorder() bool { return false }

func (c *Context) SetResizable(enabled bool) {
	if c == nil || c.window == 0 {
		return
	}
	c.resizable = enabled
	c.applyWindowStyle()
	setWindowPos(c.window, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged)
}

func (c *Context) applyWindowStyle() {
	current := getWindowLongPtrW(c.window, gwlStyle)
	setWindowLongPtrW(c.window, gwlStyle, uintptr(nativeWindowStyleWithState(current, c.decorations, c.resizable)))
}

func nativeWindowStyleWithState(current uintptr, decorations DecorationMode, resizable bool) uint32 {
	const runtimeState = wsMaximize | wsDisabled | wsVisible | wsMinimize
	return nativeWindowStyle(decorations, resizable) | uint32(current)&runtimeState
}

func nativeWindowStyle(decorations DecorationMode, resizable bool) uint32 {
	if decorations == DecorationNone {
		style := uint32(wsPopup)
		if resizable {
			style |= wsThickFrame | wsMinimizeBox | wsMaximizeBox | wsSysMenu
		}
		return style
	}
	style := uint32(wsOverlappedWindow)
	if !resizable {
		style &^= wsThickFrame | wsMaximizeBox
	}
	return style
}

func (c *Context) applyNonClientAppearance() {
	if c.window == 0 {
		return
	}
	color := windowBorderColor(c.decorations, c.border)
	dwmSetWindowAttribute(c.window, dwmwaBorderColor, unsafe.Pointer(&color), uint32(unsafe.Sizeof(color)))
	policy := windowNonClientPolicy(c.shadow)
	dwmSetWindowAttribute(c.window, dwmwaNCRenderingPolicy, unsafe.Pointer(&policy), uint32(unsafe.Sizeof(policy)))
}

func windowBorderColor(decorations DecorationMode, border bool) uint32 {
	if decorations == DecorationNone && !border {
		return dwmColorNone
	}
	return dwmColorDefault
}

func windowNonClientPolicy(shadow bool) uint32 {
	if !shadow {
		return dwmNCRenderingDisabled
	}
	return dwmNCRenderingUseStyle
}

func (c *Context) undecoratedHitTest(lParam uintptr) uintptr {
	windowRect := rect{}
	if !getWindowRect(c.window, &windowRect) {
		return htClient
	}
	x := int32(int16(lParam & 0xffff))
	y := int32(int16((lParam >> 16) & 0xffff))
	margin := int32(math.Ceil(6 * c.devicePixelRatio))
	return undecoratedHitTestRect(windowRect, x, y, margin)
}

func (c *Context) SetRoundedCorners(enabled bool) {
	if c == nil || c.window == 0 {
		return
	}
	c.roundedCorners = enabled
	preference := uint32(dwmWindowCornerDoNotRound)
	if enabled {
		preference = dwmWindowCornerRound
	}
	dwmSetWindowAttribute(c.window, dwmwaWindowCornerPreference, unsafe.Pointer(&preference), uint32(unsafe.Sizeof(preference)))
	c.applyRoundedCorners()
}

func (c *Context) applyRoundedCorners() {
	radius := 0
	if c.roundedCorners && c.decorations == DecorationNone && !c.transparent && !c.fullscreen && !isZoomed(c.window) {
		// The swapchain and the native region use device pixels. Keeping the
		// same scaled radius on both sides preserves the renderer's antialiased
		// edge instead of clipping it at a smaller logical radius.
		radius = int(math.Round(float64(c.cornerRadius) * c.devicePixelRatio))
	}
	render.SetSurfaceCornerRadius(radius)
	c.applyWindowRegion()
}

func undecoratedHitTestRect(windowRect rect, x, y, margin int32) uintptr {
	left := x < windowRect.Left+margin
	right := x >= windowRect.Right-margin
	bottom := y >= windowRect.Bottom-margin
	switch {
	case bottom && left:
		return htBottomLeft
	case bottom && right:
		return htBottomRight
	case left:
		return htLeft
	case right:
		return htRight
	case bottom:
		return htBottom
	default:
		return htClient
	}
}

func (c *Context) Minimize() {
	if c != nil && c.window != 0 {
		c.releaseMouseButtons()
		showWindow(c.window, swMinimize)
	}
}

func (c *Context) ToggleMaximize() {
	if c == nil || c.window == 0 {
		return
	}
	command := int32(swMaximize)
	if isZoomed(c.window) {
		command = swRestore
	}
	showWindow(c.window, command)
}

func (c *Context) ToggleFullscreen() {
	if c == nil || c.window == 0 {
		return
	}
	if c.fullscreen {
		setWindowLongPtrW(c.window, gwlStyle, c.fullscreenStyle)
		c.fullscreen = false
		if c.fullscreenMaximized {
			showWindow(c.window, swMaximize)
		} else {
			setWindowPos(
				c.window,
				0,
				c.fullscreenRect.Left,
				c.fullscreenRect.Top,
				c.fullscreenRect.Right-c.fullscreenRect.Left,
				c.fullscreenRect.Bottom-c.fullscreenRect.Top,
				swpNoZOrder|swpNoActivate|swpFrameChanged,
			)
		}
		c.SetRoundedCorners(c.decorations == DecorationNone && !c.transparent)
		return
	}
	if !getWindowRect(c.window, &c.fullscreenRect) {
		return
	}
	monitor := monitorFromWindow(c.window, monitorDefaultNearest)
	if monitor == 0 {
		return
	}
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if !getMonitorInfoW(monitor, &info) {
		return
	}
	c.fullscreenMaximized = isZoomed(c.window)
	c.fullscreenStyle = getWindowLongPtrW(c.window, gwlStyle)
	setWindowLongPtrW(c.window, gwlStyle, uintptr(wsPopup))
	setWindowPos(
		c.window,
		0,
		info.Monitor.Left,
		info.Monitor.Top,
		info.Monitor.Right-info.Monitor.Left,
		info.Monitor.Bottom-info.Monitor.Top,
		swpNoZOrder|swpNoActivate|swpFrameChanged,
	)
	c.fullscreen = true
	c.SetRoundedCorners(false)
}

func (c *Context) StartWindowDrag() {
	if c == nil || c.window == 0 {
		return
	}
	releaseCapture()
	sendMessageW(c.window, wmNCLButtonDown, htCaption, 0)
}
