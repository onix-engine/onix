package main

import (
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/diiyw/onix"
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestProjectOnDemandFrames(t *testing.T) {
	source := os.DirFS(".")
	plan, err := project.CompileProject(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := OnixRegistry()
	if err != nil {
		t.Fatal(err)
	}
	app := onix.NewApp()
	t.Cleanup(app.Destroy)
	app.SetRenderMode(onix.RenderOnDemand)
	app.SetTargetFPS(0)
	if err := OnixInstallSystems(app); err != nil {
		t.Fatal(err)
	}
	if _, err := app.LoadModules(plan, registry, pelicanModuleOptions(source)); err != nil {
		t.Fatal(err)
	}
	step := func() bool {
		t.Helper()
		rendered, err := app.Step()
		if err != nil {
			t.Fatal(err)
		}
		return rendered
	}
	settle := func() {
		t.Helper()
		for range 8 {
			if !step() {
				return
			}
		}
		t.Fatal("static game kept rendering")
	}
	key := func(k input.KeyCode) {
		t.Helper()
		for _, kind := range []scene.InputEventKind{scene.InputEventKeyDown, scene.InputEventKeyUp} {
			if err := app.Modules().HandleInput(scene.InputEvent{Kind: kind, Key: k}); err != nil {
				t.Fatal(err)
			}
		}
		if err := app.Modules().Dispatch(); err != nil {
			t.Fatal(err)
		}
		if !step() {
			t.Fatal("game input did not publish a frame")
		}
	}
	if !step() {
		t.Fatal("missing initial frame")
	}
	settle()
	key(input.KeyEnter)
	for range 3 {
		if err := app.Modules().Update(1.0 / 64); err != nil {
			t.Fatal(err)
		}
		if !step() {
			t.Fatal("on-demand rendering froze the moving game")
		}
	}
	key(input.KeyP)
	settle()
	key(input.KeyP)
	key(input.KeyR)
}

// Exercise the generated registry, real SVG decoder, retained HUD and routed
// controls without opening a window. Visual acceptance remains manual.
func TestProjectResourcesAndControls(t *testing.T) {
	source := os.DirFS(".")
	plan, err := project.CompileProject(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := OnixRegistry()
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	ui.RegisterRuntimeFeature[ui.HTMLRenderer](world)
	state := input.NewState()
	ecs.InsertResource(world, state)
	camera := scene.SpawnCamera2D(world)
	ecs.GetMut[scene.Camera](world, camera).UpdateViewport(1280, 800)
	renders := 0
	options := pelicanModuleOptions(source)
	options.Host = &module.Host{RequestRender: func() { renders++ }}
	runtime, err := module.New(world, plan, registry, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := runtime.Manager().StartEntry(); err != nil {
		t.Fatal(err)
	}
	if err := htmlui.Sync(world); err != nil {
		t.Fatal(err)
	}
	var page, rider ecs.Entity
	textures := map[any]bool{}
	for e := range ecs.Query[htmlui.Surface](world).All {
		page = e
	}
	pageSurface, ok := ecs.Get[htmlui.Surface](world, page)
	if !ok || pageSurface.Font == nil || pageSurface.BoldFont == nil {
		t.Fatal("HUD must load bundled regular and bold fonts without system fonts")
	}
	if pageSurface.Font == pageSurface.BoldFont {
		t.Fatal("HUD regular and bold fonts must use separate faces")
	}
	for _, path := range []string{"resources/game/game.html", "resources/game/mod.go", "resources/game/state.go"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range string(data) {
			if unicode.IsSpace(r) {
				continue
			}
			if pageSurface.Font.GlyphIndex(r) == 0 || pageSurface.BoldFont.GlyphIndex(r) == 0 {
				t.Fatalf("bundled fonts lack %q (U+%04X) from %s; regenerate with tools/subset_fonts.py", r, r, path)
			}
		}
	}
	for e, a := range ecs.Query[scene.Sprite2D](world).All {
		sprite := a.Get()
		if sprite.Texture != nil {
			textures[sprite.Texture] = true
		}
		if sprite.Width == 230 && sprite.Height == 236 {
			if sprite.Texture.Width != 460 || sprite.Texture.Height != 472 {
				t.Fatal("rider lost its 2x raster resolution")
			}
			p, _ := ecs.Get[scene.Parent](world, e)
			rider = p.Entity
		}
		if sprite.Width == 1280 && sprite.Texture != nil {
			if sprite.Texture.Width != 1280 || sprite.Texture.LogicalWidth() != 1280 || !sprite.Texture.Mipmapped() {
				t.Fatal("background must use intrinsic resolution with mipmaps")
			}
		}
	}
	if page == 0 || rider == 0 || len(textures) < 15 {
		t.Fatalf("missing HTML, rider or SVG resources: page=%d rider=%d textures=%d", page, rider, len(textures))
	}
	node := func(id string) htmlui.Node {
		t.Helper()
		n, ok := htmlui.NodeByID(world, page, id)
		if !ok {
			t.Fatal("missing node", id)
		}
		return n
	}
	markup := func(id string) string {
		t.Helper()
		s, err := node(id).Serialize()
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	key := func(k input.KeyCode, down bool) {
		t.Helper()
		kind := scene.InputEventKeyUp
		if down {
			kind = scene.InputEventKeyDown
		}
		if err := runtime.HandleInput(scene.InputEvent{Kind: kind, Key: k}); err != nil {
			t.Fatal(err)
		}
		if err := runtime.Dispatch(); err != nil {
			t.Fatal(err)
		}
	}
	var inputEvents ecs.MessageReader[scene.InputEvent]
	var htmlEvents ecs.MessageReader[htmlui.Event]
	lastClick := ""
	pointerFrame := func() {
		t.Helper()
		for _, system := range []ecs.SystemConfig{scene.SyncGlobalTransformSystem(""), scene.SyncUIGlobalTransformSystem(""), scene.DispatchInputEvents2DSystem(""), ui.UpdateSystem("")} {
			if err := system.Run(world); err != nil {
				t.Fatal(err)
			}
		}
		for event := range inputEvents.All(world) {
			if err := runtime.HandleInput(*event); err != nil {
				t.Fatal(err)
			}
		}
		for event := range htmlEvents.All(world) {
			if event.Kind == htmlui.EventClick {
				lastClick = event.ID
			}
			if err := runtime.HandleHTMLEvent(*event); err != nil {
				t.Fatal(err)
			}
		}
		if err := runtime.Dispatch(); err != nil {
			t.Fatal(err)
		}
		if err := ui.SyncSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		state.EndFrame()
	}
	center := func(id string) (float64, float64) {
		t.Helper()
		// Present DOM changes before starting the next pointer interaction.
		pointerFrame()
		bounds, ok := node(id).Bounds()
		if !ok {
			t.Fatal("control has no bounds", id)
		}
		return float64(bounds.X + bounds.Width/2), float64(bounds.Y + bounds.Height/2)
	}
	click := func(id string) {
		t.Helper()
		x, y := center(id)
		lastClick = ""
		state.PressMouse(input.MouseLeft, x, y)
		pointerFrame()
		state.ReleaseMouse(input.MouseLeft, x, y)
		pointerFrame()
		if lastClick != id {
			t.Fatalf("pointer click on %s activated %q", id, lastClick)
		}
	}
	update := func(n int) {
		t.Helper()
		for range n {
			if err := runtime.Update(1.0 / 60); err != nil {
				t.Fatal(err)
			}
		}
		if err := htmlui.Sync(world); err != nil {
			t.Fatal(err)
		}
	}
	if b, ok := node("start").Bounds(); !ok || b.Width < 44 || b.Height < 44 {
		t.Fatal("start control is too small", b)
	}
	renders = 0
	update(4)
	if renders != 0 {
		t.Fatal("ready screen requested continuous rendering")
	}
	click("start")
	if renders == 0 {
		t.Fatal("starting did not request rendering")
	}
	if opening, _, _ := strings.Cut(markup("veil"), ">"); !strings.Contains(opening, " hidden=") {
		t.Fatal("start control did not hide the introduction")
	}
	click("deliver")
	if !strings.Contains(markup("notice"), "靠近灯塔小站时") {
		t.Fatal("delivery button did not explain why delivery is unavailable")
	}
	key(input.KeySpace, true)
	renders = 0
	update(12)
	if renders == 0 {
		t.Fatal("riding did not request animation frames")
	}
	transform, _ := ecs.Get[scene.UITransform](world, rider)
	jumping := transform.Translation
	if transform.Translation.Y.Value >= 580 {
		t.Fatal("keyboard jump did not move the native rider", transform.Translation)
	}
	key(input.KeyP, true)
	key(input.KeyP, false)
	renders = 0
	update(60)
	if renders != 0 {
		t.Fatal("paused game requested continuous rendering")
	}
	transform, _ = ecs.Get[scene.UITransform](world, rider)
	if transform.Translation != jumping || !strings.Contains(markup("sheet-title"), "鲜鱼可以等一下") {
		t.Fatal("pause did not freeze the rider and display its overlay")
	}
	key(input.KeySpace, false)
	click("start")
	update(70)
	// Hit-test touch coordinates too; pressing must jump before release/click.
	x, y := center("jump")
	state.TouchStart(x, y)
	pointerFrame()
	update(8)
	transform, _ = ecs.Get[scene.UITransform](world, rider)
	if transform.Translation.Y.Value >= 580 {
		t.Fatal("touch jump did not reach the game")
	}
	lastClick = ""
	state.TouchEnd(x, y)
	pointerFrame()
	if lastClick != "jump" {
		t.Fatal("touch release did not activate the jump button")
	}
	click("pause")
	if !strings.Contains(markup("sheet-title"), "鲜鱼可以等一下") {
		t.Fatal("pointer pause did not display its overlay")
	}
	click("restart")
	update(1)
	if !strings.Contains(markup("cargo"), ">0<") || !strings.Contains(markup("hearts"), ">3<") {
		t.Fatal("restart did not reset HUD state")
	}
	// CSS pointer-events:none keeps the riding area reachable behind the HUD.
	state.TouchStart(900, 400)
	pointerFrame()
	update(8)
	transform, _ = ecs.Get[scene.UITransform](world, rider)
	if transform.Translation.Y.Value >= 580 {
		t.Fatal("HUD blocked jumping from the riding area")
	}
	state.TouchEnd(900, 400)
	pointerFrame()
	for _, id := range []string{"pause", "jump", "deliver"} {
		bounds, ok := node(id).Bounds()
		if !ok || bounds.Width < 44 || bounds.Height < 44 {
			t.Fatal("control is too small", id, bounds)
		}
	}
}
