//go:build android

package webview

import (
	"github.com/diiyw/onix/scene"
	"testing"
)

func TestAndroidWebViewCreatePrefersSpecifiedURL(t *testing.T) {
	resetAndroidWebViewTestState()
	t.Cleanup(resetAndroidWebViewTestState)

	backendValue, err := newBackend(nil)
	if err != nil {
		t.Fatal(err)
	}
	backend := backendValue.(*androidWebViewBackend)
	bounds := scene.WebViewBounds{X: 10, Y: 20, Width: 640, Height: 360}
	if _, err := backend.Create(scene.WebViewOptions{
		URL:     "https://example.com/app",
		HTML:    "<p>fallback</p>",
		Bounds:  bounds,
		Visible: true,
	}, nil, nil); err != nil {
		t.Fatal(err)
	}

	androidWebViews.Lock()
	command := *androidNativeCommandLocked()
	androidWebViews.Unlock()
	if command.kind != NativeCommandCreate {
		t.Fatalf("command = %d, want create", command.kind)
	}
	if command.text != "https://example.com/app" {
		t.Fatalf("URL = %q", command.text)
	}
	if command.flags&nativeFlagHTML != 0 {
		t.Fatal("URL creation command was marked as inline HTML")
	}
	if command.flags&nativeFlagVisible == 0 {
		t.Fatal("URL creation command lost visibility")
	}
	if command.bounds != bounds {
		t.Fatalf("bounds = %#v, want %#v", command.bounds, bounds)
	}
}

func resetAndroidWebViewTestState() {
	androidWebViews.Lock()
	androidWebViews.backend = nil
	androidWebViews.handles = nil
	androidWebViews.commands = nil
	androidWebViews.commandHead = 0
	androidWebViews.nextID = 0
	androidWebViews.nextCommand = 0
	androidWebViews.Unlock()
}
