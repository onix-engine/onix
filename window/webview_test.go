package window

import (
	"errors"
	"github.com/diiyw/onix/scene"
	"testing"
)

type testWebViewBackend struct{}

func (testWebViewBackend) Create(scene.WebViewOptions, scene.WebViewInvoke, func(scene.WebViewEvent)) (scene.WebViewHandle, error) {
	return nil, errors.New("test backend should not create views")
}

func TestWebViewBackendFactoryOverridesPlatform(t *testing.T) {
	previous := webViewBackendFactory
	previousRequiresX11 := webViewBackendRequiresX11
	defer func() {
		webViewBackendFactory = previous
		webViewBackendRequiresX11 = previousRequiresX11
	}()
	want := testWebViewBackend{}
	RegisterWebViewBackend(func(*Context) (scene.WebViewBackend, error) { return want, nil })
	got, err := NewWebViewBackend(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.(testWebViewBackend); !ok {
		t.Fatalf("backend type = %T", got)
	}
	if RegisterWebViewBackendIfUnset(func(*Context) (scene.WebViewBackend, error) { return nil, nil }) {
		t.Fatal("backend factory was replaced despite an existing registration")
	}
}

func TestX11WebViewBackendMarksWindowRequirement(t *testing.T) {
	previous := webViewBackendFactory
	previousRequiresX11 := webViewBackendRequiresX11
	defer func() {
		webViewBackendFactory = previous
		webViewBackendRequiresX11 = previousRequiresX11
	}()
	webViewBackendFactory = nil
	webViewBackendRequiresX11 = false
	if !RegisterX11WebViewBackendIfUnset(func(*Context) (scene.WebViewBackend, error) { return testWebViewBackend{}, nil }) {
		t.Fatal("X11 backend registration failed")
	}
	if !webViewBackendRequiresX11 {
		t.Fatal("X11 requirement was not retained")
	}
	RegisterWebViewBackend(nil)
	if webViewBackendRequiresX11 {
		t.Fatal("custom backend registration retained the X11 requirement")
	}
}
