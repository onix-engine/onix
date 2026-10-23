//go:build cgo && windows && arm64

package webview

import "unsafe"

func putWebViewControllerBounds(controller *webView2Controller, bounds webViewRect) uintptr {
	words := (*[2]uintptr)(unsafe.Pointer(&bounds))
	return comCall2(controller, webView2ControllerPutBounds, words[0], words[1])
}
