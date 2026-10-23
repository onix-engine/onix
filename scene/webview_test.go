package scene

import (
	"github.com/diiyw/onix/ecs"
	"testing"
)

type webViewTestHandle struct {
	navigate []string
	html     []string
	bounds   []WebViewBounds
	visible  []bool
	destroy  int
}

func (h *webViewTestHandle) Navigate(url string) error {
	h.navigate = append(h.navigate, url)
	return nil
}

func (h *webViewTestHandle) SetHTML(html string) error {
	h.html = append(h.html, html)
	return nil
}

func (*webViewTestHandle) Eval(string) error { return nil }

func (h *webViewTestHandle) SetBounds(bounds WebViewBounds) error {
	h.bounds = append(h.bounds, bounds)
	return nil
}

func (h *webViewTestHandle) SetVisible(visible bool) error {
	h.visible = append(h.visible, visible)
	return nil
}

func (h *webViewTestHandle) Destroy() error {
	h.destroy++
	return nil
}

type webViewTestBackend struct {
	options []WebViewOptions
	invoke  []WebViewInvoke
	events  []func(WebViewEvent)
	handle  *webViewTestHandle
}

func (b *webViewTestBackend) Create(options WebViewOptions, invoke WebViewInvoke, events func(WebViewEvent)) (WebViewHandle, error) {
	b.options = append(b.options, options)
	b.invoke = append(b.invoke, invoke)
	b.events = append(b.events, events)
	if b.handle == nil {
		b.handle = &webViewTestHandle{}
	}
	return b.handle, nil
}

func TestWebViewRuntimeSynchronizesChangesAndDestroys(t *testing.T) {
	world := ecs.NewWorld()
	backend := &webViewTestBackend{}
	runtime := NewWebViewRuntime(backend)
	ecs.InsertResource(world, runtime)
	entity := world.Spawn(ecs.NewBundle(0))
	reader := &ecs.MessageReader[WebViewEvent]{}
	if err := ecs.Insert(world, entity, NewWebView("https://example.test")); err != nil {
		t.Fatal(err)
	}
	if err := SyncWebViews(world); err != nil {
		t.Fatal(err)
	}
	if len(backend.options) != 1 || len(backend.invoke) != 1 {
		t.Fatalf("create calls = %d, invoke callbacks = %d", len(backend.options), len(backend.invoke))
	}
	backend.events[0](WebViewEvent{Kind: WebViewEventReady})
	if events := reader.Read(world); len(events) != 0 {
		t.Fatalf("events were delivered before the next sync: %#v", events)
	}
	if err := SyncWebViews(world); err != nil {
		t.Fatal(err)
	}
	events := reader.Read(world)
	if len(events) != 1 || events[0].Entity != entity || events[0].Kind != WebViewEventReady {
		t.Fatalf("events after sync = %#v", events)
	}
	if got := backend.options[0].URL; got != "https://example.test" {
		t.Fatalf("url = %q", got)
	}
	if err := SyncWebViews(world); err != nil {
		t.Fatal(err)
	}
	if len(backend.handle.navigate) != 0 {
		t.Fatalf("unexpected initial navigation calls: %v", backend.handle.navigate)
	}
	value := ecs.GetMut[WebView](world, entity)
	value.URL = ""
	value.HTML = "<strong>ready</strong>"
	value.X, value.Y, value.Width, value.Height = 2, 3, 40, 20
	if err := SyncWebViews(world); err != nil {
		t.Fatal(err)
	}
	if len(backend.handle.html) != 1 || backend.handle.html[0] != value.HTML {
		t.Fatalf("html updates = %v", backend.handle.html)
	}
	if got := backend.handle.bounds; len(got) != 1 || got[0] != (WebViewBounds{X: 2, Y: 3, Width: 40, Height: 20}) {
		t.Fatalf("bounds updates = %v", got)
	}
	if err := NavigateWebView(world, entity, "https://next.example"); err != nil {
		t.Fatal(err)
	}
	if err := SyncWebViews(world); err != nil {
		t.Fatal(err)
	}
	if len(backend.handle.navigate) != 1 || backend.handle.navigate[0] != "https://next.example" {
		t.Fatalf("navigate calls = %v", backend.handle.navigate)
	}
	value.DevTools = true
	if err := SyncWebViews(world); err != nil {
		t.Fatal(err)
	}
	if len(backend.options) != 2 || backend.options[1].DevTools != true || backend.handle.destroy != 1 {
		t.Fatalf("recreate options=%d devtools=%v destroys=%d", len(backend.options), backend.options[1].DevTools, backend.handle.destroy)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if backend.handle.destroy != 2 {
		t.Fatalf("destroy calls = %d", backend.handle.destroy)
	}
}

func TestWebViewBindingUsesCurrentComponent(t *testing.T) {
	world := ecs.NewWorld()
	backend := &webViewTestBackend{}
	ecs.InsertResource(world, NewWebViewRuntime(backend))
	entity := world.Spawn(ecs.NewBundle(0))
	reader := &ecs.MessageReader[WebViewEvent]{}
	value := NewWebViewHTML("<button></button>")
	if !value.Bind("add", func(payload string) (string, error) { return payload + "!", nil }) {
		t.Fatal("bind failed")
	}
	if err := ecs.Insert(world, entity, value); err != nil {
		t.Fatal(err)
	}
	if err := SyncWebViews(world); err != nil {
		t.Fatal(err)
	}
	result, err := backend.invoke[0]("add", "ok")
	if err != nil || result != "ok!" {
		t.Fatalf("invoke = %q, %v", result, err)
	}
	if err := SyncWebViews(world); err != nil {
		t.Fatal(err)
	}
	events := reader.Read(world)
	if len(events) != 1 || events[0].Kind != WebViewEventMessage || events[0].Name != "add" || events[0].Message != "ok" {
		t.Fatalf("bridge events = %#v", events)
	}
	if _, err := backend.invoke[0]("missing", ""); err == nil || err.Error() != `ecs: webview binding "missing" is not registered` {
		t.Fatalf("missing binding error = %v", err)
	}
}

func TestWebViewRuntimeUnavailableEmitsError(t *testing.T) {
	world := ecs.NewWorld()
	ecs.InsertResource(world, NewWebViewRuntime(nil))
	entity := world.Spawn(ecs.NewBundle(0))
	if err := ecs.Insert(world, entity, NewWebView("https://example.test")); err != nil {
		t.Fatal(err)
	}
	if err := SyncWebViews(world); err != nil {
		t.Fatal(err)
	}
	events := (&ecs.MessageReader[WebViewEvent]{}).Read(world)
	if len(events) != 1 || events[0].Entity != entity || events[0].Kind != WebViewEventError || events[0].Error != ErrWebViewUnavailable.Error() {
		t.Fatalf("events = %#v", events)
	}
}
