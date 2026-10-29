package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/diiyw/onix"

	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
)

type browserHandle struct {
	visible   bool
	bounds    scene.WebViewBounds
	destroyed bool
}

func (*browserHandle) Navigate(string) error                       { return nil }
func (*browserHandle) SetHTML(string) error                        { return nil }
func (*browserHandle) Eval(string) error                           { return nil }
func (h *browserHandle) SetBounds(value scene.WebViewBounds) error { h.bounds = value; return nil }
func (h *browserHandle) SetVisible(value bool) error               { h.visible = value; return nil }
func (h *browserHandle) Destroy() error                            { h.destroyed = true; return nil }

type browserBackend struct {
	handles []*browserHandle
	invokes []scene.WebViewInvoke
	pages   []string
}

func (b *browserBackend) Create(value scene.WebViewOptions, invoke scene.WebViewInvoke, _ func(scene.WebViewEvent)) (scene.WebViewHandle, error) {
	handle := &browserHandle{visible: value.Visible, bounds: value.Bounds}
	b.handles = append(b.handles, handle)
	b.invokes = append(b.invokes, invoke)
	b.pages = append(b.pages, value.HTML)
	return handle, nil
}

func TestWebViewModuleOwnsPageBoundsBridgeAndLifecycle(t *testing.T) {
	plan, err := project.CompileProjectDirectory(".")
	if err != nil {
		t.Fatal(err)
	}
	plan.Scenes = append(plan.Scenes, project.ScenePlan{ID: "pause.json", Transform: project.ModTransform{Scale: [3]float32{1, 1, 1}}})
	registry, err := OnixRegistry()
	if err != nil {
		t.Fatal(err)
	}
	app := onix.NewApp()
	t.Cleanup(app.Destroy)
	backend := &browserBackend{}
	app.World.InsertResource(scene.NewWebViewRuntime(backend))
	if err := OnixInstallSystems(app); err != nil {
		t.Fatal(err)
	}
	entry, err := app.LoadModules[OnixContentProfile](plan, registry, OnixModuleOptions(os.DirFS(".")))
	if err != nil {
		t.Fatal(err)
	}
	sync := func() {
		t.Helper()
		if err := scene.SyncWebViews(app.World); err != nil {
			t.Fatal(err)
		}
	}
	sync()
	if len(backend.handles) != 1 || !strings.Contains(backend.pages[0], "onix.invoke") || backend.handles[0].bounds != (scene.WebViewBounds{X: 20, Y: 20, Width: 760, Height: 500}) {
		t.Fatalf("private page or bounds missing: handles=%+v pages=%v", backend.handles, backend.pages)
	}
	payload := `{"source":"browser","value":"a\\b\"c"}`
	result, err := backend.invokes[0]("ping", payload)
	if err != nil {
		t.Fatal(err)
	}
	var reply struct {
		Message string
		Payload json.RawMessage
	}
	if err := json.Unmarshal([]byte(result), &reply); err != nil || reply.Message != "hello from Go" || string(reply.Payload) != payload {
		t.Fatalf("bridge result %s (%v)", result, err)
	}
	if _, err := backend.invokes[0]("ping", "{"); err == nil {
		t.Fatal("malformed payload accepted")
	}
	writer, err := entry.SlotWriter("browser", "bounds")
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Write(project.SlotValue{Type: "BrowserBounds", Fields: []project.SlotFieldValue{
		{Name: "height", Value: project.SlotValue{Type: project.NumberValue, Number: 240}},
		{Name: "width", Value: project.SlotValue{Type: project.NumberValue, Number: 320}},
		{Name: "x", Value: project.SlotValue{Type: project.NumberValue, Number: 12}},
		{Name: "y", Value: project.SlotValue{Type: project.NumberValue, Number: 24}},
	}}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := app.Modules().Dispatch(); err != nil {
			t.Fatal(err)
		}
	}
	sync()
	if backend.handles[0].bounds != (scene.WebViewBounds{X: 12, Y: 24, Width: 320, Height: 240}) {
		t.Fatal("bounds slot not applied")
	}
	second, err := app.Modules().MountEntry()
	if err != nil {
		t.Fatal(err)
	}
	sync()
	if len(backend.handles) != 2 || backend.handles[1].bounds.Width != 760 {
		t.Fatal("reused browser instance shares bounds")
	}
	if err := second.Dispose(); err != nil {
		t.Fatal(err)
	}
	sync()
	if !backend.handles[1].destroyed || backend.handles[0].destroyed {
		t.Fatal("browser lifetime escaped owning scene")
	}
	if err := app.Modules().Manager().Switch(module.SceneSwitchRequest{Scene: "pause.json", Retain: true}); err != nil {
		t.Fatal(err)
	}
	if err := app.Modules().Dispatch(); err != nil {
		t.Fatal(err)
	}
	sync()
	if backend.handles[0].visible {
		t.Fatal("suspended browser remained visible")
	}
	if _, err := backend.invokes[0]("ping", "{}"); err == nil {
		t.Fatal("suspended browser invoked its module")
	}
	if err := app.Modules().Manager().Switch(module.SceneSwitchRequest{Scene: "main.json"}); err != nil {
		t.Fatal(err)
	}
	if err := app.Modules().Dispatch(); err != nil {
		t.Fatal(err)
	}
	sync()
	if !backend.handles[0].visible {
		t.Fatal("browser did not restore visibility")
	}
	step := func() {
		if err := scene.SyncWebViews(app.World); err != nil {
			panic(err)
		}
	}
	step()
	if allocations := testing.AllocsPerRun(100, step); allocations != 0 {
		t.Fatalf("stable browser allocations=%g", allocations)
	}
	if err := entry.Dispose(); err != nil {
		t.Fatal(err)
	}
	sync()
	if !backend.handles[0].destroyed {
		t.Fatal("browser surface survived scene disposal")
	}
	if _, err := backend.invokes[0]("ping", "{}"); err == nil {
		t.Fatal("disposed browser still invokes module")
	}
}
