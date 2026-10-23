//go:build (!android && !(cgo && windows) && !(cgo && (darwin || linux) && !ios) && !(js && wasm && !wechat)) || (linux && onix_drm)

package webview

import (
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/window"
)

func newBackend(_ *window.Context) (scene.WebViewBackend, error) {
	return nil, scene.ErrWebViewUnavailable
}
