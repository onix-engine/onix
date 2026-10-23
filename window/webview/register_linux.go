//go:build linux && cgo && !android && !onix_drm

package webview

import "github.com/diiyw/onix/window"

func registerBackend() {
	window.RegisterX11WebViewBackendIfUnset(newBackend)
}
