package window

import (
	"math"
	"testing"
)

func TestComputeViewportLayoutContainCentersLetterbox(t *testing.T) {
	layout, ok := computeViewportLayout(720, 1280, ViewportModeContain, 1920, 1080, 960, 540)
	if !ok {
		t.Fatal("expected viewport layout to be computed")
	}

	if layout.ViewWidth != 720 || layout.ViewHeight != 1280 {
		t.Fatalf("unexpected logical viewport: %dx%d", layout.ViewWidth, layout.ViewHeight)
	}
	if !almostEqual(layout.RenderViewport.X, 656.25) || !almostEqual(layout.RenderViewport.Width, 607.5) {
		t.Fatalf("unexpected contain render viewport: %+v", layout.RenderViewport)
	}
	if !almostEqual(layout.InputViewport.X, 328.125) || !almostEqual(layout.InputViewport.Width, 303.75) {
		t.Fatalf("unexpected contain input viewport: %+v", layout.InputViewport)
	}
}

func TestComputeViewportLayoutCoverCropsOverflow(t *testing.T) {
	layout, ok := computeViewportLayout(720, 1280, ViewportModeCover, 1920, 1080, 1920, 1080)
	if !ok {
		t.Fatal("expected viewport layout to be computed")
	}

	if !almostEqual(layout.RenderViewport.X, 0) || !almostEqual(layout.RenderViewport.Y, -1166.6666667) {
		t.Fatalf("unexpected cover viewport origin: %+v", layout.RenderViewport)
	}
	if !almostEqual(layout.RenderViewport.Width, 1920) || !almostEqual(layout.RenderViewport.Height, 3413.3333333) {
		t.Fatalf("unexpected cover viewport size: %+v", layout.RenderViewport)
	}
}

func TestComputeViewportLayoutStretchUsesFullSurface(t *testing.T) {
	layout, ok := computeViewportLayout(720, 1280, ViewportModeStretch, 1920, 1080, 1920, 1080)
	if !ok {
		t.Fatal("expected viewport layout to be computed")
	}

	if !almostEqual(layout.RenderViewport.X, 0) || !almostEqual(layout.RenderViewport.Y, 0) {
		t.Fatalf("unexpected stretch origin: %+v", layout.RenderViewport)
	}
	if !almostEqual(layout.RenderViewport.Width, 1920) || !almostEqual(layout.RenderViewport.Height, 1080) {
		t.Fatalf("unexpected stretch viewport: %+v", layout.RenderViewport)
	}
}

func TestComputeViewportLayoutFixedWidthAdjustsLogicalHeight(t *testing.T) {
	layout, ok := computeViewportLayout(720, 1280, ViewportModeFixedWidth, 1920, 1080, 960, 540)
	if !ok {
		t.Fatal("expected viewport layout to be computed")
	}

	if layout.ViewWidth != 720 || layout.ViewHeight != 405 {
		t.Fatalf("unexpected fixed-width viewport: %dx%d", layout.ViewWidth, layout.ViewHeight)
	}
	if !almostEqual(layout.RenderViewport.Width, 1920) || !almostEqual(layout.RenderViewport.Height, 1080) {
		t.Fatalf("unexpected fixed-width render viewport: %+v", layout.RenderViewport)
	}
}

func TestComputeViewportLayoutFixedHeightAdjustsLogicalWidth(t *testing.T) {
	layout, ok := computeViewportLayout(720, 1280, ViewportModeFixedHeight, 1920, 1080, 960, 540)
	if !ok {
		t.Fatal("expected viewport layout to be computed")
	}

	if layout.ViewWidth != 2276 || layout.ViewHeight != 1280 {
		t.Fatalf("unexpected fixed-height viewport: %dx%d", layout.ViewWidth, layout.ViewHeight)
	}
}

func TestComputeViewportLayoutExpandGrowsVisibleArea(t *testing.T) {
	layout, ok := computeViewportLayout(720, 1280, ViewportModeExpand, 1920, 1080, 960, 540)
	if !ok {
		t.Fatal("expected viewport layout to be computed")
	}

	if layout.ViewWidth != 2276 || layout.ViewHeight != 1280 {
		t.Fatalf("unexpected expand viewport: %dx%d", layout.ViewWidth, layout.ViewHeight)
	}
	if !almostEqual(layout.RenderViewport.Width, 1920) || !almostEqual(layout.RenderViewport.Height, 1080) {
		t.Fatalf("unexpected expand render viewport: %+v", layout.RenderViewport)
	}
}

func TestComputeViewportLayoutAdaptiveUsesInputSurfaceAsMinimum(t *testing.T) {
	layout, ok := computeViewportLayout(1600, 960, ViewportModeAdaptive, 3840, 2160, 1920, 1080)
	if !ok {
		t.Fatal("expected viewport layout to be computed")
	}

	if layout.ViewWidth != 1920 || layout.ViewHeight != 1080 {
		t.Fatalf("unexpected adaptive viewport: %dx%d", layout.ViewWidth, layout.ViewHeight)
	}
	if !almostEqual(layout.RenderViewport.Width, 3840) || !almostEqual(layout.RenderViewport.Height, 2160) {
		t.Fatalf("unexpected adaptive render viewport: %+v", layout.RenderViewport)
	}
	if !almostEqual(layout.InputViewport.Width, 1920) || !almostEqual(layout.InputViewport.Height, 1080) {
		t.Fatalf("unexpected adaptive input viewport: %+v", layout.InputViewport)
	}

	layout, ok = computeViewportLayout(1600, 960, ViewportModeAdaptive, 2400, 1440, 1200, 720)
	if !ok {
		t.Fatal("expected small adaptive viewport layout to be computed")
	}
	if layout.ViewWidth != 1600 || layout.ViewHeight != 960 {
		t.Fatalf("expected adaptive viewport to keep minimum size, got %dx%d", layout.ViewWidth, layout.ViewHeight)
	}
}

func TestComputeViewportLayoutIntegerScaleKeepsWholePixels(t *testing.T) {
	layout, ok := computeViewportLayout(320, 180, ViewportModeInteger, 1920, 1080, 960, 540)
	if !ok {
		t.Fatal("expected viewport layout to be computed")
	}

	if layout.ViewWidth != 320 || layout.ViewHeight != 180 {
		t.Fatalf("unexpected integer viewport: %dx%d", layout.ViewWidth, layout.ViewHeight)
	}
	if !almostEqual(layout.RenderViewport.X, 0) || !almostEqual(layout.RenderViewport.Y, 0) {
		t.Fatalf("unexpected integer origin: %+v", layout.RenderViewport)
	}
	if !almostEqual(layout.RenderViewport.Width, 1920) || !almostEqual(layout.RenderViewport.Height, 1080) {
		t.Fatalf("unexpected integer render viewport: %+v", layout.RenderViewport)
	}
}

func TestLogicalXYFromViewportUsesVisibleArea(t *testing.T) {
	viewport := viewportRect{X: -320, Y: 0, Width: 1920, Height: 1080}
	x, y := logicalXYFromViewport(0, 540, 720, 1280, viewport)

	if !almostEqual(x, 120) {
		t.Fatalf("expected logical x 120, got %f", x)
	}
	if !almostEqual(y, 640) {
		t.Fatalf("expected logical y 640, got %f", y)
	}
}

func TestInputRectMapsLogicalCaretToInputSurface(t *testing.T) {
	state := newViewportState(720, 1280)
	if _, ok := state.Update(1920, 1080, 960, 540); !ok {
		t.Fatal("expected viewport layout to be computed")
	}
	rect, ok := state.InputRect(120, 640, 1, 24)
	if !ok {
		t.Fatal("expected input rect to be computed")
	}
	if !almostEqual(rect.X, 378.75) || !almostEqual(rect.Y, 270) {
		t.Fatalf("unexpected caret origin: %+v", rect)
	}
	if !almostEqual(rect.Width, 0.421875) || !almostEqual(rect.Height, 10.125) {
		t.Fatalf("unexpected caret size: %+v", rect)
	}
}

func TestNormalizeDevicePixelRatio(t *testing.T) {
	if got := normalizeDevicePixelRatio(2); got != 2 {
		t.Fatalf("expected ratio 2, got %f", got)
	}
	if got := normalizeDevicePixelRatio(0); got != 1 {
		t.Fatalf("expected invalid ratio to normalize to 1, got %f", got)
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.0001
}
