package onix

import "testing"

func TestSetInitialWindowSizeStoresPrecreationOverride(t *testing.T) {
	app := NewApp()
	app.SetInitialWindowSize(1280, 720)
	if app.initialWindowWidth != 1280 || app.initialWindowHeight != 720 {
		t.Fatalf("initial window size = %dx%d", app.initialWindowWidth, app.initialWindowHeight)
	}
	app.SetInitialWindowSize(0, 720)
	if app.initialWindowWidth != 1280 || app.initialWindowHeight != 720 {
		t.Fatalf("invalid override changed initial window size = %dx%d", app.initialWindowWidth, app.initialWindowHeight)
	}
}
