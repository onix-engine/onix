//go:build js && wasm && !wechat

package webview

import (
	"github.com/onix-engine/onix/scene"
	"github.com/onix-engine/onix/window"
)

func newBackend(context *window.Context) (scene.WebViewBackend, error) {
	return window.NewBrowserWebViewBackend(context)
}
