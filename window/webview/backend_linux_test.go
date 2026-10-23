//go:build linux && cgo && !android && !onix_drm

package webview

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/window"
)

func TestLinuxWebViewLoadsInstalledRuntime(t *testing.T) {
	library, err := linuxWebViewLibrary()
	if err != nil {
		t.Skip(err)
	}
	if !strings.HasPrefix(library, "libwebkit2gtk-") {
		t.Fatalf("unexpected WebKitGTK library %q", library)
	}
}

func TestLinuxEmbeddedWebViewBridge(t *testing.T) {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("Linux display is unavailable")
	}
	context := window.NewContext(640, 480, "Onix WebView Test", window.DefaultOptions())
	defer context.Destroy()
	runtime, err := newBackend(context)
	if err != nil {
		t.Fatal(err)
	}
	backend := runtime.(*linuxWebViewBackend)
	defer func() {
		if err := backend.Close(); err != nil {
			t.Errorf("close backend: %v", err)
		}
	}()

	invoked := make(chan string, 1)
	handle, err := backend.Create(scene.WebViewOptions{
		HTML:    `<script>onix.invoke("ping", "from-webkit")</script>`,
		Bounds:  scene.WebViewBounds{Width: 320, Height: 200},
		Visible: false,
	}, func(name, payload string) (string, error) {
		invoked <- name + ":" + payload
		return "pong", nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-invoked:
		if message != "ping:from-webkit" {
			t.Fatalf("bridge message = %q", message)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for WebKitGTK bridge invocation")
	}
	if err := handle.SetBounds(scene.WebViewBounds{X: 30, Y: 40, Width: 280, Height: 160}); err != nil {
		t.Fatal(err)
	}
	if err := handle.SetVisible(true); err != nil {
		t.Fatal(err)
	}
	if err := handle.SetVisible(false); err != nil {
		t.Fatal(err)
	}
	if err := handle.Destroy(); err != nil {
		t.Fatal(err)
	}
}
