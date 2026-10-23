package window

import "testing"

func TestDefaultOptionsLimitWebSurfaceCost(t *testing.T) {
	options := DefaultOptions()
	if options.RenderScale != 1 || options.MaxDevicePixelRatio != 2 || options.Antialias || options.Transparent || options.Decorations != DecorationSystem || options.TitleBarHeight != 0 || options.CornerRadius != DefaultCornerRadius || !options.Border || !options.Shadow || !options.Resizable {
		t.Fatalf("unexpected default window options: %+v", options)
	}
}

func TestOptionsNormalizeMissingScaleValues(t *testing.T) {
	options := (Options{Antialias: true, TitleBarHeight: -1}).normalized()
	if options.RenderScale != 1 || options.MaxDevicePixelRatio != 2 || !options.Antialias || options.Decorations != DecorationSystem || options.TitleBarHeight != 0 || options.CornerRadius != DefaultCornerRadius {
		t.Fatalf("unexpected normalized window options: %+v", options)
	}
}

func TestDecorationModeValidation(t *testing.T) {
	for _, mode := range []DecorationMode{"", DecorationSystem, DecorationNone} {
		if !mode.Valid() {
			t.Fatalf("expected decoration mode %q to be valid", mode)
		}
	}
	if DecorationMode("floating").Valid() {
		t.Fatal("unexpected custom decoration mode")
	}
}
