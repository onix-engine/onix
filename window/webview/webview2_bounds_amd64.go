//go:build cgo && windows && amd64

package webview

import "unsafe"

func putWebViewControllerBounds(controller *webView2Controller, bounds webViewRect) uintptr {
	return comCall1(controller, webView2ControllerPutBounds, uintptr(unsafe.Pointer(&bounds)))
}
