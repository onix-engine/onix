//go:build cgo && windows && 386

package webview

import "unsafe"

func putWebViewControllerBounds(controller *webView2Controller, bounds webViewRect) uintptr {
	return invokeCOM(
		comMethod(controller, webView2ControllerPutBounds),
		uintptr(unsafe.Pointer(controller)),
		uintptr(bounds.left),
		uintptr(bounds.top),
		uintptr(bounds.right),
		uintptr(bounds.bottom),
	)
}
