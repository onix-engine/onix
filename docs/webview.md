# WebView

`scene.WebView` renders a page using the host browser control and keeps the
native handle synchronized with the ECS component. Add the component to an
entity and install the backend before the first frame:

```go
import (
	"github.com/diiyw/onix"
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	webview "github.com/diiyw/onix/window/webview"
)

app := onix.NewApp()
webview.Register()
if err := app.InstallSystems[onix.WebView](); err != nil {
	panic(err)
}
app.SetWindowSize(1024, 720)

page := scene.NewWebViewHTML(`<!doctype html>
<button onclick="onix.invoke('greet', JSON.stringify({name: 'Onix'}))">
  Greet
</button>`)
page.X, page.Y, page.Width, page.Height = 20, 20, 640, 360
page.Bind("greet", func(payload string) (string, error) {
	return `{"ok":true,"payload":` + payload + `}`, nil
})
entity := app.World.Spawn(ecs.NewBundle(0))
if err := app.World.Insert(entity, page); err != nil {
	panic(err)
}
```

To load a URL instead of inline HTML, construct the component with the target
URL. The URL is passed directly to the platform browser:

```go
page := scene.NewWebView("https://example.com/app")
page.Bounds = scene.WebViewBounds{X: 20, Y: 20, Width: 640, Height: 360}
```

Assigning a different non-empty `page.URL` and synchronizing navigates the
existing browser surface. `scene.NavigateWebView` does the same immediately for
an already materialized component. URL takes precedence if both `URL` and
`HTML` are set.

`app.InstallSystems[onix.WebView]` is opt-in. It registers the ECS synchronizer
and creates the built-in backend after the window context is ready, so projects
that do not use WebView do not retain the platform browser implementation in
their generated system installation. Applications that own a native view can
call `window.RegisterWebViewBackend` before creating the context, or pass an
`scene.WebViewBackend` through `App.SetWebViewBackend`.

When installing the systems manually, import and register the built-in adapter
for the target before installing the ECS system:

```go
import "github.com/diiyw/onix/window/webview"

webview.Register()
```

The bridge is Promise-based. Inline pages call `onix.invoke(name, payload)`;
`payload` is passed to the matching `WebView.Bind` handler as raw JSON and its
returned string resolves the Promise. `OnInvoke` can handle all names from one
component. Use `scene.EvalWebView` for fire-and-forget JavaScript and
`scene.NavigateWebView` for programmatic navigation.

Native events are delivered as `scene.WebViewEvent` messages. `Ready`,
`Navigation`, `Closed`, and `Error` are emitted by the backend; bridge calls
are handled synchronously so their return value can be sent back to the page.
The Windows, macOS, and Linux adapters embed each WebView as a native child of
the Onix window. Applications that need another embedding policy can provide it
through the backend factory. Native hosts can obtain the parent surface with
`Context.NativeHandle` where the platform exposes one.

Platform backends are selected at build time:

| Target | Backend |
| --- | --- |
| Windows | Microsoft WebView2 (Onix Win32/COM backend) |
| macOS | Built-in WKWebView backend |
| Linux | Built-in WebKitGTK backend (runtime-selected) |
| Browser/WASM | DOM iframe and `postMessage` |
| Android | Built-in Android WebView backend |
| iOS | Host-provided backend via `RegisterWebViewBackend` |
| WeChat | Unsupported (`scene.ErrWebViewUnavailable`) |

The Windows backend discovers the installed WebView2 Runtime through
`EdgeUpdate\ClientState` and loads it directly. It does not link or distribute
`WebView2Loader.dll`.

The Linux backend loads the WebKitGTK runtime dynamically and supports the
WebKitGTK 4.0 and 4.1 ABIs. It uses whichever compatible runtime is installed
on the host and does not require WebKitGTK development headers or pkg-config
metadata to build. Embedded WebViews use a GTK plug mapped directly into the
Onix X11 window, never a separately managed application window. WebKit uses its
shared-memory surface transport to avoid DMA-BUF stalls on drivers such as
vmwgfx. On a Wayland desktop Onix selects XWayland when WebView is installed.
