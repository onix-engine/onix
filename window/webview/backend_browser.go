//go:build js && wasm && !wechat

package webview

import (
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/window"
)

func newBackend(context *window.Context) (scene.WebViewBackend, error) {
	return window.NewBrowserWebViewBackend(context)
}
