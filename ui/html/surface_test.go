package html

import (
	"image"
	"image/color"
	"slices"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	onixfont "github.com/diiyw/onix/font"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/scene"
	notosanssc "github.com/diiyw/onix/testdata/fonts/Noto_Sans_SC"
	onixtext "github.com/diiyw/onix/text"
)

type testHTMLImageProvider struct {
	texture *graphics.Texture
}

func (p testHTMLImageProvider) ResolveHTMLImage(string) (*graphics.Texture, bool) {
	return p.texture, p.texture != nil
}

func TestHTMLNodePointerAndClickCallbacks(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	component := New(`<div id="target" style="width:120px;height:40px"></div>`)
	component.Width, component.Height = 160, 80
	component.Font = notosanssc.NotoSansSCRegular
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	node, ok := NodeByID(world, owner, "target")
	if !ok {
		t.Fatal("target node is missing")
	}
	pointerCount, clickCount := 0, 0
	if !node.OnPointer(func(event *PointerEvent) {
		pointerCount++
		if event.Target.node != node.node {
			t.Error("pointer callback target changed")
		}
	}) || !node.OnClick(func(event *PointerEvent) {
		clickCount++
		if event.Kind != scene.InputEventTap {
			t.Errorf("click event kind = %q", event.Kind)
		}
	}) {
		t.Fatal("binding HTML node callbacks failed")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	var target ecs.Entity
	for entity, access := range ecs.Query[htmlChild](world).All {
		child := access.Get()

		if child.Owner == owner && child.Node == node.node && ecs.Has[scene.InputTarget2D](world, entity) {
			target = entity
			break
		}
	}
	if target == 0 {
		t.Fatal("bound node has no input target")
	}
	scene.DispatchInputEvent(world, scene.InputEvent{Entity: target, Kind: scene.InputEventPress, X: 10, Y: 12})
	scene.DispatchInputEvent(world, scene.InputEvent{Entity: target, Kind: scene.InputEventTap, X: 10, Y: 12})
	if pointerCount != 2 || clickCount != 1 {
		t.Fatalf("callback counts = pointer %d, click %d", pointerCount, clickCount)
	}
	callbackEvent := scene.InputEvent{Entity: target, Kind: scene.InputEventTap, X: 10, Y: 12}
	callbackContext := scene.InputEventContext{Event: &callbackEvent, Target: target, Current: target}
	dispatchHTMLNodePointer(world, owner, node.node, &callbackContext)
	allocations := testing.AllocsPerRun(100, func() {
		dispatchHTMLNodePointer(world, owner, node.node, &callbackContext)
	})
	if allocations != 0 {
		t.Fatalf("HTML node callback dispatch allocated %.0f objects, want 0", allocations)
	}
}

func TestHTMLLinearGradientBackgroundCreatesSpriteTexture(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="gradient" style="width:40px;height:20px;background:linear-gradient(to right,#ef4444,#3b82f6)"></div>`)
	found := false
	for _, access := range ecs.Query2[htmlChild, scene.Sprite2D](world).All {
		child := access.A.Get()
		sprite := access.B.Get()

		if child.Owner != owner || child.Node == nil {
			continue
		}
		id, _ := child.Node.Attr("id")
		if id != "gradient" {
			continue
		}
		found = true
		if sprite.Texture == nil || !sprite.Texture.Valid() {
			t.Errorf("gradient texture = %#v, want valid texture", sprite.Texture)
		}
		if sprite.Shape.Kind != 0 {
			t.Errorf("gradient shape = %#v, want texture-only sprite", sprite.Shape)
		}
	}
	if !found {
		t.Fatal("gradient surface missing")
	}
}

func TestHTMLLinearGradientParsesDirectionAndStops(t *testing.T) {
	stops, direction, ok := parseHTMLLinearGradient(`linear-gradient(45deg, #ef4444 0%, rgba(0, 128, 0, .5) 50%, transparent 100%)`)
	if !ok || len(stops) != 3 {
		t.Fatalf("gradient parse = ok:%v stops:%#v", ok, stops)
	}
	if direction.X < .7 || direction.Y > -.7 || stops[0].Position != 0 || stops[1].Position != .5 || stops[2].Position != 1 {
		t.Fatalf("gradient direction/stops = %#v %#v", direction, stops)
	}
	if stops[1].Color.G != 128 || stops[1].Color.A != 128 || stops[2].Color.A != 0 {
		t.Fatalf("gradient colors = %#v", stops)
	}
}

func TestHTMLLinearGradientColorMix(t *testing.T) {
	stops, _, ok := parseHTMLLinearGradient(`linear-gradient(to right, color-mix(in srgb, red, blue) 10%, color-mix(in srgb, rgba(0, 255, 0, .5) 40%, transparent) 90%)`)
	if !ok || len(stops) != 2 {
		t.Fatalf("mixed gradient parse = %v, %+v", ok, stops)
	}
	if stops[0].Color != (color.RGBA{R: 128, B: 128, A: 255}) || stops[1].Color != (color.RGBA{G: 255, A: 51}) || stops[0].Position != .1 || stops[1].Position != .9 {
		t.Fatalf("mixed gradient stops = %+v", stops)
	}
}

func TestHTMLNodeEventChangesReuseVisualTree(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	component := New(`<div id="target" style="width:120px;height:40px">Target</div>`)
	component.Width, component.Height = 160, 80
	component.Font = notosanssc.NotoSansSCRegular
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state, ok := ecs.Get[htmlRenderState](world, owner)
	if !ok || len(state.Children) == 0 {
		t.Fatal("missing initial HTML children")
	}
	initial := slices.Clone(state.Children)
	node, ok := NodeByID(world, owner, "target")
	clicks := 0
	if !ok || !node.OnClick(func(*PointerEvent) { clicks++ }) {
		t.Fatal("bind click callback")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state, _ = ecs.Get[htmlRenderState](world, owner)
	for _, entity := range initial {
		if !world.IsAlive(entity) {
			t.Fatalf("event-only sync despawned visual entity %d", entity)
		}
	}
	if len(state.Children) < len(initial) {
		t.Fatalf("event-only sync lost children: before=%d after=%d", len(initial), len(state.Children))
	}
	if !node.OnClick(func(*PointerEvent) { clicks++ }) {
		t.Fatal("rebind click callback")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	var target ecs.Entity
	for entity, access := range ecs.Query[htmlChild](world).All {
		child := access.Get()

		if child.Owner == owner && child.Node == node.node && ecs.Has[scene.InputTarget2D](world, entity) {
			target = entity
			break
		}
	}
	scene.DispatchInputEvent(world, scene.InputEvent{Entity: target, Kind: scene.InputEventTap})
	if clicks != 1 {
		t.Fatalf("rebound click callback count = %d, want 1", clicks)
	}
	if !node.OnClick(nil) {
		t.Fatal("remove click callback")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	scene.DispatchInputEvent(world, scene.InputEvent{Entity: target, Kind: scene.InputEventTap})
	if clicks != 1 {
		t.Fatalf("removed click callback still fired: %d", clicks)
	}
	for entity, _ := range ecs.Query[htmlNodeEventTarget](world).All {
		child, childOK := ecs.Get[htmlChild](world, entity)
		if childOK && child.Owner == owner {
			t.Errorf("stale event target %d remained after callback removal", entity)
		}
	}
}

func TestHTMLNodeIDIndexTracksMutations(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, New(`<div id="root"><span id="first"></span></div>`))
	owner := world.Spawn(bundle)
	root, err := Root(world, owner)
	if err != nil {
		t.Fatal(err)
	}
	first, ok := root.FindByID("first")
	if !ok {
		t.Fatal("initial id lookup failed")
	}
	if !first.SetAttribute("id", "renamed") {
		t.Fatal("rename id failed")
	}
	if _, ok := NodeByID(world, owner, "first"); ok {
		t.Fatal("old id remained indexed")
	}
	if _, ok := NodeByID(world, owner, "renamed"); !ok {
		t.Fatal("renamed id was not indexed")
	}
	child := first.Clone()
	if !child.SetAttribute("id", "appended") {
		t.Fatal("set detached clone id failed")
	}
	if !root.Append(child) {
		t.Fatal("append clone failed")
	}
	if _, ok := NodeByID(world, owner, "appended"); !ok {
		t.Fatal("appended id was not indexed")
	}
	if !first.RemoveAttribute("id") {
		t.Fatal("remove id failed")
	}
	if _, ok := NodeByID(world, owner, "renamed"); ok {
		t.Fatal("removed id remained indexed")
	}
}

func TestHTMLRendersImagesWhenProviderIsInstalledAfterFirstSync(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	component := New(`<button><img src="icon.svg" width="16" height="16"></button>`)
	component.Width, component.Height = 120, 60
	component.Font = notosanssc.NotoSansSCRegular
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	firstState, ok := ecs.Get[htmlRenderState](world, owner)
	if !ok {
		t.Fatal("missing initial HTML render state")
	}
	firstChildren := slices.Clone(firstState.Children)

	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 16, 16)))
	t.Cleanup(func() { _ = texture.Dispose() })
	ecs.InsertResource[ImageProvider](world, testHTMLImageProvider{texture: texture})
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	secondState, _ := ecs.Get[htmlRenderState](world, owner)
	if slices.Equal(firstChildren, secondState.Children) {
		t.Fatal("HTML image provider change reused stale visual tree")
	}
	foundImage := false
	for entity, access := range ecs.Query[htmlChild](world).All {
		child := access.Get()

		if child.Owner != owner {
			continue
		}
		sprite, exists := ecs.Get[scene.Sprite2D](world, entity)
		if exists && sprite.Texture == texture {
			foundImage = true
			break
		}
	}
	if !foundImage {
		t.Fatal("HTML image provider texture was not rendered")
	}
}

func TestHTMLComponentBuildsAndCachesVisualTree(t *testing.T) {
	world := ecs.NewWorld()
	ecs.InsertResource(world, scene.ViewportSize{Width: 320, Height: 180})
	bundle := ecs.NewBundle(6)
	ecs.Put(&bundle, scene.NewUITransform())
	ecs.Put(&bundle, scene.UIGlobalTransform{})
	ecs.Put(&bundle, scene.Visibility{Visible: true})
	ecs.Put(&bundle, scene.AnchorTopLeft)
	ecs.Put(&bundle, Surface{
		Value: `<style>.card { color: #123456; background-color: #abcdef; padding: 8px; border: 2px solid #010203; border-radius: 4px; }</style><div class="card">Hello <span>Onix</span></div>`,
		Width: 220, Height: 120, Font: notosanssc.NotoSansSCRegular,
	})
	owner := world.Spawn(bundle)

	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state, ok := ecs.Get[htmlRenderState](world, owner)
	if !ok || len(state.Children) < 2 {
		t.Fatalf("html state = %#v", state)
	}
	firstChildren := slices.Clone(state.Children)

	texts, surfaces := 0, 0
	for entity, access := range ecs.Query[htmlChild](world).All {
		child := access.Get()

		if child.Owner != owner {
			continue
		}
		if text, exists := ecs.Get[scene.Text2D](world, entity); exists {
			texts++
			if strings.Contains(text.Value, ".card") {
				t.Fatal("style element was rendered as visible text")
			}
		}
		if ecs.Has[scene.Sprite2D](world, entity) {
			surfaces++
		}
	}
	if texts == 0 || surfaces == 0 {
		t.Fatalf("html visuals: texts=%d surfaces=%d", texts, surfaces)
	}
	if bounds, ok := Bounds(world, owner); !ok || bounds.Width != 220 || bounds.Height <= 0 {
		t.Fatalf("html bounds = %#v, ok=%v", bounds, ok)
	}

	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state, _ = ecs.Get[htmlRenderState](world, owner)
	if !slices.Equal(firstChildren, state.Children) {
		t.Fatalf("unchanged html rebuilt children: before=%v after=%v", firstChildren, state.Children)
	}

	if !Set(world, owner, `<p>Replacement</p>`) {
		t.Fatal("SetHTML did not find component")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state, _ = ecs.Get[htmlRenderState](world, owner)
	if slices.Equal(firstChildren, state.Children) {
		t.Fatal("changed html reused stale visual entities")
	}
	for _, child := range firstChildren {
		if world.IsAlive(child) {
			t.Fatalf("stale html child %d is still alive", child)
		}
	}
}

func TestHTMLNodeOffsetMovesDescendantTextWithElementSurface(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	component := New(`<div id="notice" style="background:#123456;padding:4px"><span>Notice</span></div>`)
	component.Width, component.Height = 220, 80
	component.Font = notosanssc.NotoSansSCRegular
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatalf("sync offset test HTML: %v", err)
	}

	wantFill := graphics.ColorFromRGB(0x12, 0x34, 0x56)
	var textEntity, surfaceEntity ecs.Entity
	for entity, access := range ecs.Query[htmlChild](world).All {
		child := access.Get()

		if child.Owner != owner {
			continue
		}
		if text, ok := ecs.Get[scene.Text2D](world, entity); ok && text.Value == "Notice" {
			textEntity = entity
		}
		if sprite, ok := ecs.Get[scene.Sprite2D](world, entity); ok && sprite.Shape.Fill == wantFill {
			surfaceEntity = entity
		}
	}
	if textEntity == 0 || surfaceEntity == 0 {
		t.Fatalf("notice visuals: text=%v surface=%v", textEntity, surfaceEntity)
	}
	textBefore, _ := ecs.Get[scene.UITransform](world, textEntity)
	surfaceBefore, _ := ecs.Get[scene.UITransform](world, surfaceEntity)
	if !SetNodeOffset(world, owner, "notice", 0, 12) {
		t.Fatal("notice offset did not move retained visuals")
	}
	textAfter, _ := ecs.Get[scene.UITransform](world, textEntity)
	surfaceAfter, _ := ecs.Get[scene.UITransform](world, surfaceEntity)
	if textAfter.Translation.Y.Value != textBefore.Translation.Y.Value+12 {
		t.Fatalf("notice text y = %v, want %v", textAfter.Translation.Y.Value, textBefore.Translation.Y.Value+12)
	}
	if surfaceAfter.Translation.Y.Value != surfaceBefore.Translation.Y.Value+12 {
		t.Fatalf("notice surface y = %v, want %v", surfaceAfter.Translation.Y.Value, surfaceBefore.Translation.Y.Value+12)
	}
}

func TestHTMLBoldTextUsesBoldAdvanceForFollowingText(t *testing.T) {
	world := ecs.NewWorld()
	component := New(`<p>prefix <strong>bold</strong> suffix</p>`)
	component.Width = 320
	component.Font = notosanssc.NotoSansSCRegular
	boldFace, err := onixfont.Parse(append([]byte(nil), component.Font.Data...))
	if err != nil {
		t.Fatalf("clone font: %v", err)
	}
	boldGlyph := boldFace.GlyphIndex('b')
	boldFace.Metrics[boldGlyph].AdvanceWidth += 1000
	component.BoldFont = onixtext.NewFont(boldFace)
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)

	if err := Sync(world); err != nil {
		t.Fatal(err)
	}

	var bold, suffix *scene.Text2D
	var boldTransform, suffixTransform scene.UITransform
	for entity, access := range ecs.Query2[scene.Parent, scene.Text2D](world).All {
		parent := access.A.Get()
		text := access.B.Get()

		if parent.Entity != owner {
			continue
		}
		transform, ok := ecs.Get[scene.UITransform](world, entity)
		if !ok {
			continue
		}
		switch text.Value {
		case "bold":
			bold, boldTransform = &text, transform
		case "suffix":
			suffix, suffixTransform = &text, transform
		}
	}
	if bold == nil || suffix == nil {
		t.Fatalf("bold/suffix text visuals were not found: bold=%#v suffix=%#v", bold, suffix)
	}
	measured, _, _ := bold.Font.MeasureString(bold.Font.FontFace(bold.FontSize, bold.DPI), bold.Value)
	actualAdvance := suffixTransform.Translation.X.Value - boldTransform.Translation.X.Value
	if actualAdvance+0.01 < float32(measured) {
		t.Fatalf("suffix starts inside bold text: advance=%v bold width=%v", actualAdvance, measured)
	}
}

func TestHTMLNodesMutateRetainedDocument(t *testing.T) {
	world := ecs.NewWorld()
	component := New(`<div id="templates" hidden><button id="row-template"><span id="label-template">Template</span></button></div><div id="rows"></div>`)
	component.Width, component.Height = 240, 120
	component.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)

	rows, ok := NodeByID(world, owner, "rows")
	if !ok {
		t.Fatal("rows node was not found")
	}
	template, ok := NodeByID(world, owner, "row-template")
	if !ok {
		t.Fatal("row template was not found")
	}
	row := template.Clone()
	row.SetAttribute("id", "row-1")
	label, ok := row.FindByID("label-template")
	if !ok || !label.SetText("First row") || !rows.Append(row) {
		t.Fatal("cloned row could not be populated")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}

	control, found := ecs.Entity(0), false
	for entity, access := range ecs.Query[Control](world).All {
		value := access.Get()

		if value.Owner == owner && value.ID == "row-1" {
			control, found = entity, true
		}
		if !(!found) {
			break
		}
	}
	if !found || control == 0 {
		t.Fatal("mutated document did not render cloned control")
	}
	firstState, _ := ecs.Get[htmlRenderState](world, owner)
	firstChildren := slices.Clone(firstState.Children)

	if !rows.Clear() {
		t.Fatal("rows node could not be cleared")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	secondState, _ := ecs.Get[htmlRenderState](world, owner)
	if slices.Equal(firstChildren, secondState.Children) {
		t.Fatal("node mutation reused stale visual entities")
	}
}

func TestHTMLViewportUnitsFollowWorldViewport(t *testing.T) {
	world := ecs.NewWorld()
	ecs.InsertResource(world, scene.ViewportSize{Width: 320, Height: 200})
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, Surface{
		Value: `<div id="panel" style="width:50vw;height:25vh"></div>`,
		Font:  notosanssc.NotoSansSCRegular,
	})
	owner := world.Spawn(bundle)

	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state, ok := ecs.Get[htmlRenderState](world, owner)
	if !ok || state.Styles == nil {
		t.Fatal("missing HTML render state")
	}
	panel := state.NodeByID["panel"]
	if panel == nil {
		t.Fatal("missing panel node")
	}
	if got := state.Styles[panel].Width; got != style.LengthPx(160) {
		t.Fatalf("initial vw width = %#v, want 160px", got)
	}
	if got := state.Styles[panel].Height; got != style.LengthPx(50) {
		t.Fatalf("initial vh height = %#v, want 50px", got)
	}

	ecs.InsertResource(world, scene.ViewportSize{Width: 640, Height: 400})
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state, _ = ecs.Get[htmlRenderState](world, owner)
	if got := state.Styles[panel].Width; got != style.LengthPx(320) {
		t.Fatalf("resized vw width = %#v, want 320px", got)
	}
	if got := state.Styles[panel].Height; got != style.LengthPx(100) {
		t.Fatalf("resized vh height = %#v, want 100px", got)
	}
}

func TestHTMLUniversalSelectorCSS(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, Surface{
		Value: `<div class="card">Universal selector</div>`,
		CSS:   `*{box-sizing:border-box}.card{padding:8px}`,
		Width: 180, Height: 80, Font: notosanssc.NotoSansSCRegular,
	})
	world.Spawn(bundle)

	if err := Sync(world); err != nil {
		t.Fatalf("sync HTML with universal selector: %v", err)
	}
}

func TestHTMLInputPlaceholderKeepsDefaultMutedColor(t *testing.T) {
	world := ecs.NewWorld()
	component := New(`<body style="color:#1f2937"><input id="field" placeholder="输入你的名字"></body>`)
	component.Width, component.Height = 240, 60
	component.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)

	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	count := 0
	for entity, access := range ecs.Query2[ControlText, scene.Text2D](world).All {
		text := access.B.Get()

		child, ok := ecs.Get[htmlChild](world, entity)
		if !ok || child.Owner != owner {
			continue
		}
		count++
		if text.Value != "输入你的名字" {
			t.Fatalf("placeholder text = %q", text.Value)
		}
		if text.Color != (graphics.ColorFromRGB(117, 117, 117)) {
			t.Fatalf("placeholder color = %#v", text.Color)
		}
	}
	if count != 1 {
		t.Fatalf("placeholder text visuals = %d, want 1", count)
	}
}

func TestHTMLSemanticBlockElementsRenderNestedContent(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	component := New(`<main><header><div>levon</div></header><section><p>Welcome</p></section></main>`)
	component.Width, component.Height = 320, 180
	component.Font = notosanssc.NotoSansSCRegular
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)

	if err := Sync(world); err != nil {
		t.Fatalf("sync semantic HTML: %v", err)
	}
	state, ok := ecs.Get[htmlRenderState](world, owner)
	if !ok || len(state.Children) == 0 {
		t.Fatalf("semantic HTML visual tree = %#v", state)
	}
	textCount := 0
	for entity, access := range ecs.Query[htmlChild](world).All {
		child := access.Get()

		if child.Owner == owner && ecs.Has[scene.Text2D](world, entity) {
			textCount++
		}
	}
	if textCount != 2 {
		t.Fatalf("semantic HTML text visuals = %d, want 2", textCount)
	}
	if bounds, ok := Bounds(world, owner); !ok || bounds.Height <= 0 {
		t.Fatalf("semantic HTML bounds = %#v, ok=%v", bounds, ok)
	}
}

func TestHTMLFlexButtonRendersItsSizeAndLabel(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	component := New(`<div style="display:flex;flex-direction:column;width:160px;height:80px"><button id="save" style="display:flex;align-items:center;justify-content:center;width:80px;height:32px">Save</button></div>`)
	component.Width, component.Height = 200, 100
	component.Font = notosanssc.NotoSansSCRegular
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatalf("sync flex button: %v", err)
	}
	var control ecs.Entity
	for entity, access := range ecs.Query[Control](world).All {
		value := access.Get()

		if value.Owner == owner && value.ID == "save" {
			control = entity
			break
		}
	}
	if control == 0 {
		t.Fatal("flex button control was not generated")
	}
	size, ok := ecs.Get[scene.Size2D](world, control)
	if !ok || size.Width != 94 || size.Height != 38 {
		t.Fatalf("flex button size = %#v, ok=%v; want 94x38", size, ok)
	}
	foundLabel := false
	for _, access := range ecs.Query2[ControlText, scene.Text2D](world).All {
		link := access.A.Get()
		text := access.B.Get()

		if link.Control == control && text.Value == "Save" {
			if text.LineHeight != 19.2 {
				t.Errorf("flex button line height = %v, want 19.2", text.LineHeight)
			}
			foundLabel = true
			break
		}
	}
	if !foundLabel {
		t.Fatal("flex button label was not rendered")
	}
}

func TestHTMLComponentCleansOrphansAndRecoversMalformedSource(t *testing.T) {
	world := ecs.NewWorld()
	component := New(`<div>valid</div>`)
	component.Width, component.Height = 160, 80
	component.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	state, _ := ecs.Get[htmlRenderState](world, owner)
	children := slices.Clone(state.Children)
	world.Despawn(owner)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	for _, child := range children {
		if world.IsAlive(child) {
			t.Fatalf("orphan html child %d is still alive", child)
		}
	}

	invalidBundle := ecs.NewBundle(1)
	ecs.Put(&invalidBundle, Surface{Value: `<div>`, Width: 100, Height: 100, Font: notosanssc.NotoSansSCRegular})
	recovered := world.Spawn(invalidBundle)
	if err := Sync(world); err != nil {
		t.Fatalf("recover malformed HTML: %v", err)
	}
	recoveredState, ok := ecs.Get[htmlRenderState](world, recovered)
	if !ok || len(recoveredState.Root.Children) != 1 || recoveredState.Root.Children[0].Data != "div" {
		t.Fatalf("recovered HTML tree = %#v", recoveredState.Root)
	}
}

func TestHTMLContainerQueryUsesLaidOutContainerMetrics(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	component := New(`<div id="container"><div id="child"></div></div>`)
	component.CSS = `#container { width: 200px; container-type: inline-size; } @container (min-width: 150px) { #child { height: 42px; } }`
	component.Width, component.Height = 240, 120
	component.Font = notosanssc.NotoSansSCRegular
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatalf("render with container query: %v", err)
	}
	state, ok := ecs.Get[htmlRenderState](world, owner)
	if !ok {
		t.Fatal("missing html render state")
	}
	child := lookupHTMLNode(&state, "child")
	if child == nil || state.Styles[child].Height != style.LengthPx(42) {
		t.Fatalf("container query child height = %#v", state.Styles[child])
	}
	container := lookupHTMLNode(&state, "container")
	if state.StyleCache.ContainerMetrics[container].Width < 150 {
		t.Fatalf("container metrics = %#v", state.StyleCache.ContainerMetrics[container])
	}
}

func TestHTMLParagraphHasDefaultPreviousSiblingSpacing(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	component := New(`<div>before</div><p>after</p>`)
	component.Width, component.Height = 200, 100
	component.Font = notosanssc.NotoSansSCRegular
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}

	var beforeBottom, afterTop float32
	for entity, access := range ecs.Query2[scene.Text2D, scene.UITransform](world).All {
		text := access.A.Get()
		transform := access.B.Get()

		child, ok := ecs.Get[htmlChild](world, entity)
		if !ok || child.Owner != owner {
			continue
		}
		switch strings.TrimSpace(text.Value) {
		case "before":
			size, _ := ecs.Get[scene.Size2D](world, entity)
			beforeBottom = transform.Translation.Y.Value + size.Height
		case "after":
			afterTop = transform.Translation.Y.Value
		}
	}
	if gap := afterTop - beforeBottom; gap < 16 {
		t.Fatalf("paragraph sibling gap = %v, want at least 16", gap)
	}
}

func TestHTMLUsesBoldFontForBoldText(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, Surface{
		Value: `<div>regular <span style="font-weight:bold">bold</span></div>`, Width: 240, Height: 80,
		Font: notosanssc.NotoSansSCRegular, BoldFont: notosanssc.NotoSansSCBold,
	})
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	regular, bold := false, false
	for entity, access := range ecs.Query[htmlChild](world).All {
		child := access.Get()

		if child.Owner != owner {
			continue
		}
		text, ok := ecs.Get[scene.Text2D](world, entity)
		if !ok {
			continue
		}
		regular = regular || text.Font == notosanssc.NotoSansSCRegular
		bold = bold || text.Font == notosanssc.NotoSansSCBold
	}
	if !regular || !bold {
		t.Fatalf("text fonts = regular:%v bold:%v", regular, bold)
	}
}

func TestHTMLRendersLineThroughDecoration(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, Surface{
		Value: `<span style="color:#123456;text-decoration-line:line-through">deleted</span>`,
		Width: 160, Height: 60, Font: notosanssc.NotoSansSCRegular,
	})
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}

	var textTransform scene.UITransform
	var textSize scene.Size2D
	textFound := false
	for entity, access := range ecs.Query[htmlChild](world).All {
		child := access.Get()

		if child.Owner != owner {
			continue
		}
		text, ok := ecs.Get[scene.Text2D](world, entity)
		if !ok || text.Value != "deleted" {
			continue
		}
		textTransform, _ = ecs.Get[scene.UITransform](world, entity)
		textSize, _ = ecs.Get[scene.Size2D](world, entity)
		textFound = true
		break
	}
	if !textFound {
		t.Fatal("deleted text was not rendered")
	}

	lineFound := false
	for entity, access := range ecs.Query[htmlChild](world).All {
		child := access.Get()

		if child.Owner != owner {
			continue
		}
		sprite, spriteOK := ecs.Get[scene.Sprite2D](world, entity)
		lineTransform, transformOK := ecs.Get[scene.UITransform](world, entity)
		lineSize, sizeOK := ecs.Get[scene.Size2D](world, entity)
		if !spriteOK || !transformOK || !sizeOK || sprite.Shape.Fill != graphics.ColorFromRGB(0x12, 0x34, 0x56) {
			continue
		}
		textTop := textTransform.Translation.Y.Value
		lineTop := lineTransform.Translation.Y.Value
		lineFound = lineSize.Width == textSize.Width && lineSize.Height > 0 && lineSize.Height < textSize.Height && lineTop > textTop && lineTop+lineSize.Height < textTop+textSize.Height
		if !(!lineFound) {
			break
		}
	}
	if !lineFound {
		t.Fatal("line-through decoration was not rendered across the text fragment")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if err := Sync(world); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("stable decorated HTML sync allocated %.0f objects, want 0", allocations)
	}
}

func TestHTMLSyncDoesNotAllocateAfterWarmup(t *testing.T) {
	world := ecs.NewWorld()
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, Surface{Value: `<div style="color:#123456">Stable HTML</div>`, Width: 180, Height: 80, Font: notosanssc.NotoSansSCRegular})
	world.Spawn(bundle)
	system := SyncSystem("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("HTML sync allocated %.0f objects, want 0", allocations)
	}
}

func TestHTMLPaintStateUpdatesDoNotAllocateAfterWarmup(t *testing.T) {
	world := ecs.NewWorld()
	component := New(`<button id="button">Button</button><style>#button{background:#112233}#button:hover{background:#445566}</style>`)
	component.Width, component.Height = 160, 60
	component.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	var button ecs.Entity
	for entity, access := range ecs.Query[Control](world).All {
		control := access.Get()

		if control.Owner == owner && control.ID == "button" {
			button = entity
			break
		}
	}
	if button == 0 || !SetControlHover(world, button, true) || !SetControlHover(world, button, false) {
		t.Fatal("failed to warm HTML hover state")
	}
	allocations := testing.AllocsPerRun(100, func() {
		if !SetControlHover(world, button, true) || !SetControlHover(world, button, false) {
			panic("HTML hover update failed")
		}
	})
	// Hover restyles the control in each direction, so interning allocates one
	// instance per direction; everything else stays shared.
	if allocations > 2 {
		t.Fatalf("HTML hover state update allocated %.0f objects, want one instance per restyled direction", allocations)
	}
}

func TestHTMLUniformBorderHoverDoesNotRebuild(t *testing.T) {
	world := ecs.NewWorld()
	component := New(`<button id="button">Button</button><style>#button{border:1px solid #3b404b}#button:hover{border-color:#84ccff}</style>`)
	component.Width, component.Height = 160, 60
	component.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	var button ecs.Entity
	for entity, access := range ecs.Query[Control](world).All {
		control := access.Get()

		if control.Owner == owner && control.ID == "button" {
			button = entity
			break
		}
	}
	state, ok := ecs.Get[htmlRenderState](world, owner)
	if button == 0 || !ok {
		t.Fatal("button HTML control was not rendered")
	}
	revision := state.Revision
	if !SetControlHover(world, button, true) {
		t.Fatal("failed to apply button hover state")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if state, _ = ecs.Get[htmlRenderState](world, owner); state.Revision != revision {
		t.Fatalf("uniform border hover revision = %d, want %d", state.Revision, revision)
	}
	if !world.IsAlive(button) {
		t.Fatal("uniform border hover rebuilt the HTML control")
	}
}

func TestHTMLLiveFormStateDoesNotRewriteAuthoredAttributes(t *testing.T) {
	world := ecs.NewWorld()
	component := New(`<input id="check" type="checkbox" checked><select id="select"><option id="first" selected>First</option><option id="second">Second</option></select>`)
	component.Width, component.Height = 240, 60
	component.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	var checkbox, selectControl ecs.Entity
	for entity, access := range ecs.Query[Control](world).All {
		control := access.Get()

		if control.Owner != owner {
			continue
		}
		switch control.ID {
		case "check":
			checkbox = entity
		case "select":
			selectControl = entity
		}
	}
	if !SetControlChecked(world, checkbox, false) || !SetControlSelected(world, selectControl, 1) {
		t.Fatal("live form state update failed")
	}
	state, _ := ecs.Get[htmlRenderState](world, owner)
	check := findHTMLNodeByID(state.Root, "check")
	first := findHTMLNodeByID(state.Root, "first")
	second := findHTMLNodeByID(state.Root, "second")
	if _, authored := check.Attr("checked"); !authored || dom.IsChecked(check) {
		t.Fatalf("checkbox authored/live state = attr:%v checked:%v", authored, dom.IsChecked(check))
	}
	if _, authored := first.Attr("selected"); !authored || dom.IsChecked(first) || !dom.IsChecked(second) {
		t.Fatalf("option authored/live state = authored:%v first:%v second:%v", authored, dom.IsChecked(first), dom.IsChecked(second))
	}
}

func TestHTMLDirtyFormStateSurvivesDefaultAttributeChanges(t *testing.T) {
	world := ecs.NewWorld()
	component := New(`<input id="field" value="default" placeholder=""><input id="pristine" value="A"><textarea id="notes">initial</textarea><input id="check" type="checkbox" checked><select id="mode"><option id="first">A</option><option id="second">B</option></select>`)
	component.Width, component.Height = 480, 160
	component.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	controlByID := func(id string) ecs.Entity {
		var result ecs.Entity
		for entity, access := range ecs.Query[Control](world).All {
			control := access.Get()

			if control.Owner == owner && control.ID == id {
				result = entity
				break
			}
		}
		return result
	}
	field := controlByID("field")
	pristine := controlByID("pristine")
	notes := controlByID("notes")
	check := controlByID("check")
	mode := controlByID("mode")
	if field == 0 || pristine == 0 || notes == 0 || check == 0 || mode == 0 {
		t.Fatal("missing generated form control")
	}
	if !SetControlValue(world, field, "user") || !SetControlValue(world, notes, "edited") ||
		!SetControlChecked(world, check, false) || !SetControlSelected(world, mode, 1) {
		t.Fatal("failed to set live form state")
	}
	fieldNode, _ := NodeByID(world, owner, "field")
	pristineNode, _ := NodeByID(world, owner, "pristine")
	notesNode, _ := NodeByID(world, owner, "notes")
	checkNode, _ := NodeByID(world, owner, "check")
	firstNode, _ := NodeByID(world, owner, "first")
	if !fieldNode.SetAttribute("value", "new default") || !pristineNode.SetAttribute("value", "B") ||
		!notesNode.SetText("new textarea default") || !checkNode.RemoveAttribute("checked") ||
		!firstNode.SetAttribute("selected", "") {
		t.Fatal("failed to change authored defaults")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}

	fieldControl, _ := ecs.Get[Control](world, controlByID("field"))
	if fieldControl.Value != "user" || fieldControl.DefaultValue != "new default" || !fieldControl.DirtyValue {
		t.Fatalf("dirty input state = %#v", fieldControl)
	}
	pristineControl, _ := ecs.Get[Control](world, controlByID("pristine"))
	if pristineControl.Value != "B" || pristineControl.DefaultValue != "B" || pristineControl.DirtyValue {
		t.Fatalf("pristine input state = %#v", pristineControl)
	}
	notesControl, _ := ecs.Get[Control](world, controlByID("notes"))
	if notesControl.Value != "edited" || notesControl.DefaultValue != "new textarea default" || !notesControl.DirtyValue {
		t.Fatalf("dirty textarea state = %#v", notesControl)
	}
	checkControl, _ := ecs.Get[Control](world, controlByID("check"))
	if checkControl.Checked || checkControl.DefaultChecked || !checkControl.DirtyChecked {
		t.Fatalf("dirty checked state = %#v", checkControl)
	}
	modeControl, _ := ecs.Get[Control](world, controlByID("mode"))
	if modeControl.Selected != 0 || modeControl.DefaultSelected != 0 || !modeControl.DirtySelected {
		t.Fatalf("selection after non-dirty option attribute = %#v", modeControl)
	}

	secondNode, _ := NodeByID(world, owner, "second")
	if !secondNode.SetAttribute("selected", "") {
		t.Fatal("failed to change dirty option default")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	modeControl, _ = ecs.Get[Control](world, controlByID("mode"))
	if modeControl.Selected != 0 || modeControl.DefaultSelected != 1 {
		t.Fatalf("dirty option attribute replaced live selection: %#v", modeControl)
	}
	fieldDOM, _ := NodeByID(world, owner, "field")
	dirtyValue, _ := fieldDOM.node.States.Get(dom.StateDirtyValue)
	if !dirtyValue {
		t.Fatal("dirty value state was not retained on the DOM node")
	}
}

func TestHTMLMultipleSelectionAndFormReset(t *testing.T) {
	world := ecs.NewWorld()
	component := New(`<form id="settings"><input id="field" value="default"><input id="check" type="checkbox" checked><select id="mode" multiple><option selected>A</option><option>B</option><option selected>C</option></select></form>`)
	component.Width, component.Height = 360, 120
	component.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	controlByID := func(id string) ecs.Entity {
		var result ecs.Entity
		for entity, access := range ecs.Query[Control](world).All {
			control := access.Get()

			if control.Owner == owner && control.ID == id {
				result = entity
				break
			}
		}
		return result
	}
	field := controlByID("field")
	check := controlByID("check")
	mode := controlByID("mode")
	if !SetControlValue(world, field, "edited") || !SetControlChecked(world, check, false) ||
		!SetControlOptionSelected(world, mode, 1, true) || !SetControlOptionSelected(world, mode, 2, false) {
		t.Fatal("failed to set live form state")
	}
	control, _ := ecs.Get[Control](world, mode)
	if !control.Multiple || control.Selected != 0 || control.SelectedCount != 2 ||
		!control.Options[0].Selected || !control.Options[1].Selected || control.Options[2].Selected {
		t.Fatalf("multiple selected state = %#v", control)
	}

	if !ResetForm(world, owner, "settings") {
		t.Fatal("form reset failed")
	}
	fieldState, _ := ecs.Get[Control](world, field)
	checkState, _ := ecs.Get[Control](world, check)
	control, _ = ecs.Get[Control](world, mode)
	if fieldState.Value != "default" || fieldState.DirtyValue || !checkState.Checked || checkState.DirtyChecked {
		t.Fatalf("reset input state = field:%#v check:%#v", fieldState, checkState)
	}
	if control.Selected != 0 || control.SelectedCount != 2 || control.DirtySelected ||
		!control.Options[0].Selected || control.Options[1].Selected || !control.Options[2].Selected {
		t.Fatalf("reset multiple state = %#v", control)
	}
	if !SetControlOptionSelected(world, mode, 1, true) || !SetControlOptionSelected(world, mode, 1, false) {
		t.Fatal("failed to warm multiple option updates")
	}
	optionAllocations := testing.AllocsPerRun(100, func() {
		if !SetControlOptionSelected(world, mode, 1, true) || !SetControlOptionSelected(world, mode, 1, false) {
			panic("multiple option update failed")
		}
	})
	if optionAllocations != 0 {
		t.Fatalf("multiple option update allocated %.0f objects, want 0", optionAllocations)
	}

	allocations := testing.AllocsPerRun(100, func() {
		if !ResetForm(world, owner, "settings") {
			panic("form reset failed")
		}
	})
	if allocations != 0 {
		t.Fatalf("HTML form reset allocated %.0f objects, want 0", allocations)
	}
}

func TestHTMLMultipleSelectionRangeUpdatesOnceWithoutAllocation(t *testing.T) {
	world := ecs.NewWorld()
	component := New(`<select id="mode" multiple><option>A</option><option>B</option><option disabled>C</option><option>D</option></select>`)
	component.Width, component.Height = 240, 100
	component.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	var mode ecs.Entity
	for entity, access := range ecs.Query[Control](world).All {
		control := access.Get()

		if control.Owner == owner && control.ID == "mode" {
			mode = entity
			break
		}
	}
	if !SetControlSelectionRange(world, mode, 0, 3) {
		t.Fatal("set HTML selection range failed")
	}
	control, _ := ecs.Get[Control](world, mode)
	if control.SelectionAnchor != 0 || control.SelectionFocus != 3 || control.SelectedCount != 3 || control.Options[2].Selected {
		t.Fatalf("range selection = %#v", control)
	}
	if !SetControlSelectionRange(world, mode, 0, 1) || !SetControlSelectionRange(world, mode, 3, 0) {
		t.Fatal("warm range update failed")
	}
	allocations := testing.AllocsPerRun(100, func() {
		if !SetControlSelectionRange(world, mode, 0, 1) || !SetControlSelectionRange(world, mode, 0, 3) {
			panic("range update failed")
		}
	})
	if allocations != 0 {
		t.Fatalf("range update allocated %.0f objects, want 0", allocations)
	}
}

func TestHTMLActionDispatchDoesNotAllocateAfterWarmup(t *testing.T) {
	world := ecs.NewWorld()
	owner := world.Spawn(ecs.NewBundle(0))
	prevent := false
	if err := ecs.Insert(world, owner, ActionHandler{Handle: func(context *ActionContext) {
		if prevent {
			context.PreventDefault()
		}
	}}); err != nil {
		t.Fatal(err)
	}
	event := Event{Entity: owner, Kind: EventReset, Cancelable: true}
	DispatchAction(world, event)
	allocations := testing.AllocsPerRun(100, func() {
		result := DispatchAction(world, event)
		if result.DefaultPrevented {
			panic("unexpected cancellation")
		}
	})
	if allocations != 0 {
		t.Fatalf("HTML action dispatch allocated %.0f objects, want 0", allocations)
	}
}

func TestHTMLFormControlAppearanceDisablesNativeVisuals(t *testing.T) {
	world := ecs.NewWorld()
	component := New(`<input id="default-check" type="checkbox" checked><input id="custom-check" type="checkbox" checked style="appearance:none;background:#112233;border:1px solid #445566"><select id="default-select"><option>One</option></select><select id="custom-select" style="appearance:none;background:#223344;border:1px solid #556677"><option>One</option></select>`)
	component.Width, component.Height = 400, 120
	component.Font = notosanssc.NotoSansSCRegular
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	if err := Sync(world); err != nil {
		t.Fatalf("sync HTML: %v", err)
	}

	defaultMarks, customMarks := 0, 0
	defaultIndicators, customIndicators := 0, 0
	for _, access := range ecs.Query[htmlCheckboxMark](world).All {
		mark := access.Get()

		control, ok := ecs.Get[Control](world, mark.Control)
		if !ok || control.Owner != owner {
			continue
		}
		switch control.ID {
		case "default-check":
			defaultMarks++
		case "custom-check":
			customMarks++
		}
	}
	for _, access := range ecs.Query[htmlSelectIndicator](world).All {
		indicator := access.Get()

		control, ok := ecs.Get[Control](world, indicator.Control)
		if !ok || control.Owner != owner {
			continue
		}
		switch control.ID {
		case "default-select":
			defaultIndicators++
		case "custom-select":
			customIndicators++
		}
	}
	if defaultMarks != 1 || defaultIndicators != 1 {
		t.Fatalf("default native visuals = mark:%d indicator:%d, want one each", defaultMarks, defaultIndicators)
	}
	if customMarks != 0 || customIndicators != 0 {
		t.Fatalf("appearance:none native visuals = mark:%d indicator:%d, want none", customMarks, customIndicators)
	}

	customCheck := ecs.Entity(0)
	customSelect := ecs.Entity(0)
	for entity, access := range ecs.Query[Control](world).All {
		control := access.Get()

		switch control.ID {
		case "custom-check":
			customCheck = entity
		case "custom-select":
			customSelect = entity
		}
	}
	checkSprite, checkOK := ecs.Get[scene.Sprite2D](world, customCheck)
	selectSprite, selectOK := ecs.Get[scene.Sprite2D](world, customSelect)
	if !checkOK || !selectOK ||
		checkSprite.Shape.Fill != graphics.ColorFromRGB(0x11, 0x22, 0x33) ||
		checkSprite.Shape.Border != graphics.ColorFromRGB(0x44, 0x55, 0x66) ||
		checkSprite.Shape.BorderWidth != 1 ||
		selectSprite.Shape.Fill != graphics.ColorFromRGB(0x22, 0x33, 0x44) ||
		selectSprite.Shape.Border != graphics.ColorFromRGB(0x55, 0x66, 0x77) ||
		selectSprite.Shape.BorderWidth != 1 {
		t.Fatalf("appearance:none CSS fills = checkbox:%#v select:%#v", checkSprite.Shape.Fill, selectSprite.Shape.Fill)
	}
}

func TestHTMLNodeBoundsFollowCSSLayout(t *testing.T) {
	world := ecs.NewWorld()
	component := New(`<div style="position:relative;width:100%;height:40px"><button id="target" style="position:absolute;right:0;top:0;width:46px;height:38px;box-sizing:border-box;padding:0;border:0">X</button></div>`)
	component.Width, component.Height = 800, 100
	bundle := ecs.NewBundle(1)
	ecs.Put(&bundle, component)
	owner := world.Spawn(bundle)
	node, ok := NodeByID(world, owner, "target")
	if !ok {
		t.Fatal("missing target")
	}
	if _, ok := node.Bounds(); ok {
		t.Fatal("bounds available before layout")
	}
	for _, width := range []float32{800, 640} {
		html := ecs.GetMut[Surface](world, owner)
		html.Width = width
		if err := Sync(world); err != nil {
			t.Fatal(err)
		}
		r, ok := node.Bounds()
		if !ok || r.X != width-46 || r.Y != 0 || r.Width != 46 || r.Height != 38 {
			t.Fatalf("bounds at width %v: %+v, %v", width, r, ok)
		}
	}
	if allocations := testing.AllocsPerRun(100, func() { node.Bounds() }); allocations != 0 {
		t.Fatalf("bounds allocations = %v", allocations)
	}
	node.SetHidden(true)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if _, ok := node.Bounds(); ok {
		t.Fatal("hidden node has bounds")
	}
	node.SetHidden(false)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	node.Remove()
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if _, ok := node.Bounds(); ok {
		t.Fatal("removed node has bounds")
	}
}
