//go:build windows && cgo

package window

import (
	"testing"
	"unsafe"
)

func TestRoundedCornersUseDWMWithoutWindowRegion(t *testing.T) {
	originalDWM, originalSetRegion := dwmSetWindowAttribute, setWindowRgn
	t.Cleanup(func() {
		dwmSetWindowAttribute, setWindowRgn = originalDWM, originalSetRegion
	})
	var preference uint32
	calls := 0
	dwmSetWindowAttribute = func(window hwnd, attribute uint32, value unsafe.Pointer, size uint32) int32 {
		if window != 1 || attribute != dwmwaWindowCornerPreference || size != 4 {
			t.Fatalf("unexpected DWM request: window=%d attribute=%d size=%d", window, attribute, size)
		}
		preference = *(*uint32)(value)
		calls++
		return 0
	}
	setWindowRgn = func(hwnd, handle, bool) int32 {
		t.Fatal("rounded corners must not install a hard window region")
		return 0
	}
	context := &Context{window: 1}
	for _, enabled := range []bool{true, false, true} {
		context.SetRoundedCorners(enabled)
		want := uint32(dwmWindowCornerDoNotRound)
		if enabled {
			want = dwmWindowCornerRound
		}
		if preference != want {
			t.Fatalf("enabled=%v: preference=%d, want %d", enabled, preference, want)
		}
	}
	(*Context)(nil).SetRoundedCorners(true)
	(&Context{}).SetRoundedCorners(true)
	if calls != 3 {
		t.Fatalf("DWM calls=%d, want 3", calls)
	}
}

func TestCustomSurfaceCornersFollowDPIAndWindowState(t *testing.T) {
	originalDWM, originalZoomed := dwmSetWindowAttribute, isZoomed
	t.Cleanup(func() {
		dwmSetWindowAttribute, isZoomed = originalDWM, originalZoomed
	})
	dwmSetWindowAttribute = func(_ hwnd, attribute uint32, value unsafe.Pointer, _ uint32) int32 {
		if attribute != dwmwaWindowCornerPreference || *(*uint32)(value) != dwmWindowCornerDoNotRound {
			t.Fatal("custom corners must disable the smaller native DWM outline")
		}
		return 0
	}
	maximized := false
	isZoomed = func(hwnd) bool { return maximized }
	context := &Context{window: 1, decorations: DecorationNone, cornerRadius: 12, customCorners: true}
	context.SetRoundedCorners(true)
	for _, test := range []struct {
		dpi  float64
		want int
	}{{1, 12}, {1.25, 15}, {1.5, 18}, {2, 24}} {
		context.devicePixelRatio = test.dpi
		if got := context.surfaceCornerRadius(); got != test.want {
			t.Fatalf("DPI %g: radius=%d, want %d", test.dpi, got, test.want)
		}
	}
	maximized = true
	if context.surfaceCornerRadius() != 0 {
		t.Fatal("maximized windows must have square corners")
	}
	maximized = false
	context.fullscreen = true
	if context.surfaceCornerRadius() != 0 {
		t.Fatal("fullscreen windows must have square corners")
	}
	context.fullscreen = false
	if context.surfaceCornerRadius() != 24 {
		t.Fatal("restoring the window lost its custom radius")
	}
	context.SetRoundedCorners(false)
	if context.surfaceCornerRadius() != 0 {
		t.Fatal("disabled corners still have a surface mask")
	}
	context.SetRoundedCorners(true)
	if allocations := testing.AllocsPerRun(100, func() { context.surfaceCornerRadius() }); allocations != 0 {
		t.Fatalf("surface corner geometry allocated %g times", allocations)
	}
}

func TestSurfaceAlphaCompositionDoesNotClipWindow(t *testing.T) {
	originalCreate, originalDelete := createRectRgn, deleteObject
	originalBlur, originalSet := dwmEnableBlurBehind, setWindowRgn
	t.Cleanup(func() {
		createRectRgn, deleteObject = originalCreate, originalDelete
		dwmEnableBlurBehind, setWindowRgn = originalBlur, originalSet
	})
	createRectRgn = func(left, top, right, bottom int32) handle {
		if left != 0 || top != 0 || right != -1 || bottom != -1 {
			t.Fatal("composition must use an empty blur region")
		}
		return 7
	}
	deleted := 0
	deleteObject = func(region handle) bool {
		if region != 7 {
			t.Fatalf("deleted unexpected region %d", region)
		}
		deleted++
		return true
	}
	setWindowRgn = func(hwnd, handle, bool) int32 {
		t.Fatal("surface alpha must not install a hard window region")
		return 0
	}
	result := int32(0)
	dwmEnableBlurBehind = func(window hwnd, blur *dwmBlurBehind) int32 {
		if window != 1 || blur.Flags != dwmBBEnable|dwmBBBlurRegion || blur.Enable != 1 || blur.Region != 7 {
			t.Fatalf("unexpected composition request: window=%d blur=%+v", window, blur)
		}
		return result
	}
	context := &Context{window: 1}
	if err := context.enableSurfaceAlpha(); err != nil {
		t.Fatal(err)
	}
	result = -1
	if err := context.enableSurfaceAlpha(); err == nil {
		t.Fatal("DWM composition failure was ignored")
	}
	if deleted != 2 {
		t.Fatalf("composition leaked blur regions: deleted=%d, want 2", deleted)
	}
}

func TestWindowRegionOnlyUsesExplicitInputRects(t *testing.T) {
	originalCreate, originalCombine := createRectRgn, combineRgn
	originalSet, originalDelete := setWindowRgn, deleteObject
	t.Cleanup(func() {
		createRectRgn, combineRgn = originalCreate, originalCombine
		setWindowRgn, deleteObject = originalSet, originalDelete
	})
	var bounds [3][4]int32
	created, combined, deleted := 0, 0, 0
	createRectRgn = func(left, top, right, bottom int32) handle {
		bounds[created] = [4]int32{left, top, right, bottom}
		created++
		return handle(created)
	}
	combineRgn = func(destination, source1, source2 handle, mode int32) int32 {
		if destination != 1 || source1 != 1 || source2 != handle(combined+2) || mode != rgnOr {
			t.Fatalf("unexpected input region union: %d %d %d mode=%d", destination, source1, source2, mode)
		}
		combined++
		return 1
	}
	deleteObject = func(region handle) bool {
		if region != handle(deleted+2) {
			t.Fatalf("unexpected deleted region: %d", region)
		}
		deleted++
		return true
	}
	var applied handle
	setWindowRgn = func(window hwnd, region handle, redraw bool) int32 {
		if window != 1 || !redraw {
			t.Fatalf("unexpected window region request: window=%d redraw=%v", window, redraw)
		}
		applied = region
		return 1
	}
	context := &Context{window: 1, viewport: newViewportState(100, 100)}
	context.viewport.Update(200, 200, 200, 200)
	context.SetInputRegion([]InputRegionRect{
		{X: 10, Y: 20, Width: 30, Height: 40},
		{X: 60, Y: 70, Width: 10, Height: 20},
	})
	want := [3][4]int32{{}, {20, 40, 80, 120}, {120, 140, 140, 180}}
	if bounds != want || combined != 2 || deleted != 2 || applied != 1 {
		t.Fatalf("input region: bounds=%v unions=%d deleted=%d applied=%d", bounds, combined, deleted, applied)
	}
	context.SetInputRegion(nil)
	if applied != 0 || created != 3 || deleted != 2 {
		t.Fatalf("clearing input region must restore DWM corners: applied=%d created=%d deleted=%d", applied, created, deleted)
	}
	if allocations := testing.AllocsPerRun(100, context.applyWindowRegion); allocations != 0 {
		t.Fatalf("unrestricted window region allocated %g times", allocations)
	}
}
