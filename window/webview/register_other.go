//go:build !(linux && cgo && !android && !onix_drm)

package webview

import "github.com/onix-engine/onix/window"

func registerBackend() {
	window.RegisterWebViewBackendIfUnset(newBackend)
}
