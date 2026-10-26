//go:build windows && cgo

package window

import (
	"testing"

	"github.com/diiyw/onix/input"
)

// The pump talks to user32 through a stack argument array; the registered
// wrappers it replaces cost six allocations per pumped event.
func TestPollEventsDoesNotAllocate(t *testing.T) {
	var c Context
	poll := func() bool { return c.pollEvents() }
	if poll() {
		t.Fatal("unexpected quit")
	}
	if n := testing.AllocsPerRun(1000, func() { poll() }); n != 0 {
		t.Fatalf("pollEvents allocations per call=%g", n)
	}
}

func TestMapVirtualModifierKeyUsesMessageBits(t *testing.T) {
	tests := []struct {
		name   string
		key    uintptr
		params uintptr
		want   input.KeyCode
	}{
		{name: "left shift", key: vkShift, params: 0x2a << 16, want: input.KeyLeftShift},
		{name: "right shift", key: vkShift, params: 0x36 << 16, want: input.KeyRightShift},
		{name: "left control", key: vkControl, want: input.KeyLeftControl},
		{name: "right control", key: vkControl, params: 1 << 24, want: input.KeyRightControl},
		{name: "left alt", key: vkMenu, want: input.KeyLeftAlt},
		{name: "right alt", key: vkMenu, params: 1 << 24, want: input.KeyRightAlt},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := mapVirtualKey(test.key, test.params); got != test.want {
				t.Fatalf("mapVirtualKey(%#x, %#x) = %d, want %d", test.key, test.params, got, test.want)
			}
		})
	}
}

func TestContextReleasesHeldKeysWhenFocusIsLost(t *testing.T) {
	context := &Context{}
	state := input.NewState()
	context.OnKeydown(state.PressKey)
	context.OnKeyup(state.ReleaseKey)

	context.pressKey(input.KeyLeftControl)
	context.pressKey(input.KeyLeftShift)
	context.releaseKeys()

	if state.KeyHeld(input.KeyLeftControl) || state.KeyHeld(input.KeyLeftShift) {
		t.Fatal("expected focus loss to release all held keys")
	}
	if !state.KeyReleased(input.KeyLeftControl) || !state.KeyReleased(input.KeyLeftShift) {
		t.Fatal("expected focus loss to report released keys")
	}
	if len(context.downKeys) != 0 {
		t.Fatalf("expected tracked keys to be cleared, got %v", context.downKeys)
	}
}

func TestContextReleasesHeldMouseButtonsWhenFocusIsLost(t *testing.T) {
	context := &Context{}
	state := input.NewState()
	context.OnMouseDown(func(x, y float64, button input.MouseButton) {
		state.PressMouse(button, x, y)
	})
	context.OnMouseUp(func(x, y float64, button input.MouseButton) {
		state.ReleaseMouse(button, x, y)
	})

	context.pressMouse(input.MouseLeft, 12, 34)
	state.EndFrame()
	context.releaseMouseButtons()

	if state.MouseHeld(input.MouseLeft) || state.PointerHeld() {
		t.Fatal("expected focus loss to release the held mouse button")
	}
	if !state.MouseReleased(input.MouseLeft) || !state.PointerReleased() {
		t.Fatal("expected focus loss to report the released mouse button")
	}
	if len(context.downMouse) != 0 {
		t.Fatalf("expected tracked mouse buttons to be cleared, got %v", context.downMouse)
	}

	state.EndFrame()
	context.pressMouse(input.MouseLeft, 12, 34)
	if !state.MousePressed(input.MouseLeft) || !state.PointerPressed() {
		t.Fatal("expected the first click after focus returns to be pressed")
	}
}

func TestContextKeepsExplicitMouseCaptureDuringCancelMode(t *testing.T) {
	originalContext := windowContexts[1]
	originalDefWindowProcW := defWindowProcW
	t.Cleanup(func() {
		delete(windowContexts, 1)
		if originalContext != nil {
			windowContexts[1] = originalContext
		}
		defWindowProcW = originalDefWindowProcW
	})

	defWindowProcW = func(hwnd hwnd, message uint32, wParam, lParam uintptr) uintptr { return 0 }
	context := &Context{window: 1}
	context.updateMouseCapture(true)
	windowContexts[context.window] = context
	wndProc(context.window, wmCancelMode, 0, 0)

	if !context.mouseCaptured {
		t.Fatal("WM_CANCELMODE unexpectedly released explicit mouse capture")
	}
}

func TestContextDoesNotRecaptureMouseButtonsWhileCaptured(t *testing.T) {
	originalContext := windowContexts[1]
	originalSetCapture := setCapture
	t.Cleanup(func() {
		delete(windowContexts, 1)
		if originalContext != nil {
			windowContexts[1] = originalContext
		}
		setCapture = originalSetCapture
	})

	calls := 0
	setCapture = func(window hwnd) hwnd {
		calls++
		return window
	}
	context := &Context{window: 1}
	context.updateMouseCapture(true)
	windowContexts[context.window] = context
	wndProc(context.window, wmRButtonDown, 0, 0)

	if calls != 0 {
		t.Fatalf("captured mouse button recaptured the window %d times", calls)
	}
}

func TestContextMinimizeReleasesHeldMouseButtons(t *testing.T) {
	originalShowWindow := showWindow
	t.Cleanup(func() { showWindow = originalShowWindow })
	showWindow = func(_ hwnd, command int32) bool {
		if command != swMinimize {
			t.Fatalf("ShowWindow command = %d, want %d", command, swMinimize)
		}
		return true
	}

	context := &Context{window: 1}
	state := input.NewState()
	context.OnMouseDown(func(x, y float64, button input.MouseButton) {
		state.PressMouse(button, x, y)
	})
	context.OnMouseUp(func(x, y float64, button input.MouseButton) {
		state.ReleaseMouse(button, x, y)
	})

	context.pressMouse(input.MouseLeft, 12, 34)
	state.EndFrame()
	context.Minimize()

	if state.MouseHeld(input.MouseLeft) || !state.MouseReleased(input.MouseLeft) {
		t.Fatal("expected minimizing to release the held mouse button")
	}
	state.EndFrame()
	context.pressMouse(input.MouseLeft, 12, 34)
	if !state.MousePressed(input.MouseLeft) {
		t.Fatal("expected the first click after minimizing to be pressed")
	}
}

func TestContextWaitsForEventsOnlyWhileMinimized(t *testing.T) {
	originalWaitMessage := waitMessage
	t.Cleanup(func() { waitMessage = originalWaitMessage })
	waits := 0
	waitMessage = func() bool {
		waits++
		return true
	}

	context := &Context{}
	context.waitForEvents()
	context.minimized = true
	context.waitForEvents()

	if waits != 1 {
		t.Fatalf("WaitMessage calls = %d, want 1", waits)
	}
}

func TestContextShowsWindowOnlyOnce(t *testing.T) {
	originalShowWindow := showWindow
	t.Cleanup(func() { showWindow = originalShowWindow })
	shows := 0
	showWindow = func(window hwnd, command int32) bool {
		if command != swShow {
			t.Fatalf("ShowWindow command = %d, want %d", command, swShow)
		}
		shows++
		return true
	}

	context := &Context{window: 1}
	context.show()
	context.show()

	if shows != 1 || !context.shown {
		t.Fatalf("ShowWindow calls = %d, shown = %v; want 1, true", shows, context.shown)
	}
}

func TestTaskbarStyleDoesNotShowWindowBeforeFirstFrame(t *testing.T) {
	originalGetWindowLongPtrW := getWindowLongPtrW
	originalSetWindowLongPtrW := setWindowLongPtrW
	originalSetWindowPos := setWindowPos
	originalShowWindow := showWindow
	t.Cleanup(func() {
		getWindowLongPtrW = originalGetWindowLongPtrW
		setWindowLongPtrW = originalSetWindowLongPtrW
		setWindowPos = originalSetWindowPos
		showWindow = originalShowWindow
	})
	getWindowLongPtrW = func(_ hwnd, _ int32) uintptr { return wsExAppWindow }
	setWindowLongPtrW = func(_ hwnd, _ int32, _ uintptr) uintptr { return 0 }
	setWindowPos = func(_ hwnd, _ hwnd, _, _, _, _ int32, _ uint32) bool { return true }
	shows := 0
	showWindow = func(_ hwnd, _ int32) bool {
		shows++
		return true
	}

	context := &Context{window: 1}
	context.SetTaskbarVisible(false)

	if shows != 0 {
		t.Fatalf("ShowWindow calls before first frame = %d, want 0", shows)
	}
}

func TestUndecoratedHitTestKeepsTopEdgeInClientArea(t *testing.T) {
	windowRect := rect{Left: 100, Top: 100, Right: 900, Bottom: 700}
	if got := undecoratedHitTestRect(windowRect, 500, 101, 8); got != htClient {
		t.Fatalf("top edge should remain draggable client area, got %d", got)
	}
	if got := undecoratedHitTestRect(windowRect, 101, 400, 8); got != htLeft {
		t.Fatalf("left edge should resize, got %d", got)
	}
	if got := undecoratedHitTestRect(windowRect, 899, 699, 8); got != htBottomRight {
		t.Fatalf("bottom-right corner should resize, got %d", got)
	}
}

func TestTaskbarExtendedStyle(t *testing.T) {
	const base = uintptr(0x20)
	hidden := taskbarExtendedStyle(base|wsExAppWindow, false)
	if hidden&wsExToolWindow == 0 || hidden&wsExAppWindow != 0 {
		t.Fatalf("unexpected hidden taskbar style: %#x", hidden)
	}
	visible := taskbarExtendedStyle(hidden, true)
	if visible&wsExToolWindow != 0 || visible&wsExAppWindow == 0 {
		t.Fatalf("unexpected visible taskbar style: %#x", visible)
	}
}

func TestDecorationControlsWindowBorder(t *testing.T) {
	if got := windowBorderColor(DecorationNone, false); got != dwmColorNone {
		t.Fatalf("undecorated window color = %#x, want DWM no-border color", got)
	}
	if got := windowBorderColor(DecorationNone, true); got != dwmColorDefault {
		t.Fatalf("outlined undecorated window color = %#x, want DWM default color", got)
	}
	if got := windowBorderColor(DecorationSystem, false); got != dwmColorDefault {
		t.Fatalf("framed window color = %#x, want DWM default color", got)
	}
}

func TestWindowShadowControlsNonClientRendering(t *testing.T) {
	if got := windowNonClientPolicy(false); got != dwmNCRenderingDisabled {
		t.Fatalf("hidden shadow policy = %d, want disabled", got)
	}
	if got := windowNonClientPolicy(true); got != dwmNCRenderingUseStyle {
		t.Fatalf("visible shadow policy = %d, want window style", got)
	}
}

func TestUndecoratedFixedWindowUsesPopupStyleOnly(t *testing.T) {
	if got := nativeWindowStyle(DecorationNone, false); got != wsPopup {
		t.Fatalf("fixed transparent window style = %#x, want WS_POPUP", got)
	}
	if got := nativeWindowStyle(DecorationNone, true); got&wsThickFrame == 0 {
		t.Fatalf("resizable undecorated window style = %#x, want WS_THICKFRAME", got)
	}
	if got := nativeWindowStyle(DecorationSystem, false); got&wsThickFrame != 0 || got&wsMaximizeBox != 0 {
		t.Fatalf("fixed decorated window style = %#x, want resize styles removed", got)
	}
}

func TestNativeWindowStylePreservesRuntimeState(t *testing.T) {
	const wsCaption = 0x00c00000
	current := uintptr(wsOverlappedWindow | wsVisible | wsDisabled | wsMinimize | wsMaximize)
	got := nativeWindowStyleWithState(current, DecorationNone, true)
	wantState := uint32(wsVisible | wsDisabled | wsMinimize | wsMaximize)
	if got&wantState != wantState {
		t.Fatalf("runtime window state = %#x, want %#x in style %#x", got&wantState, wantState, got)
	}
	if got&wsCaption != 0 {
		t.Fatalf("old system decorations survived undecorated style: %#x", got)
	}
	if got&wsPopup == 0 || got&wsThickFrame == 0 {
		t.Fatalf("undecorated resizable style = %#x", got)
	}
}

func TestCenteredWindowPositionUsesMonitorWorkArea(t *testing.T) {
	x, y := centeredWindowPosition(rect{Left: 1920, Top: 40, Right: 3840, Bottom: 1080}, 1200, 800)
	if x != 2280 || y != 160 {
		t.Fatalf("unexpected centered position: %d,%d", x, y)
	}
}

func TestApplyMaximizedWorkAreaIncludesNonClientBounds(t *testing.T) {
	info := monitorInfo{
		Monitor: rect{Left: 1920, Top: 0, Right: 3840, Bottom: 1080},
		Work:    rect{Left: 1920, Top: 40, Right: 3840, Bottom: 1040},
	}
	target := minMaxInfo{}
	applyMaximizedWorkArea(&target, info)
	if target.MaxPosition != (point{X: 0, Y: 40}) {
		t.Fatalf("unexpected maximum position: %+v", target.MaxPosition)
	}
	if target.MaxSize != (point{X: 1920, Y: 1000}) || target.MaxTrackSize != target.MaxSize {
		t.Fatalf("unexpected maximum size: %+v track=%+v", target.MaxSize, target.MaxTrackSize)
	}
}
