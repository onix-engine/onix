package webview

// Register enables Onix's built-in platform WebView adapter. Keeping the
// registration package separate prevents unused browser dependencies from
// entering applications that do not opt into WebView.
func Register() {
	registerBackend()
}
