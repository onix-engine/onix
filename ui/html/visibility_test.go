package html

import (
	"image"
	"testing"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
)

func TestHTMLVisibilityControlAndEditableDescendants(t *testing.T) {
	for _, tag := range []string{`button`, `div contenteditable`} {
		t.Run(tag, func(t *testing.T) {
			closeTag := "div"
			if tag == "button" {
				closeTag = "button"
			}
			world, owner := incrementalHTMLFixture(t, `<`+tag+` id="parent" style="visibility:hidden">Hidden<span id="shown" style="visibility:visible">Visible</span></`+closeTag+`>`)
			defer Dispose(world)
			state := world.GetRef[htmlRenderState](owner)
			seen := false
			for _, entity := range state.Children {
				if text, ok := world.Get[scene.Text2D](entity); ok && text.Value == "Visible" {
					seen = true
					if !scene.IsVisible(world, entity) {
						t.Fatal("hidden control suppressed a visible descendant")
					}
				}
			}
			if !seen {
				t.Fatal("visible descendant text was not emitted")
			}
		})
	}
}

func TestHTMLVisibilityDefersPaintTexturesAndImages(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>.hidden{visibility:hidden}</style><div id="target" class="hidden" style="width:40px;height:40px;background:linear-gradient(red,blue);border:2px solid black;box-shadow:0 0 2px black"><img id="image" src="test.png" width="10" height="10"></div><div style="visibility:hidden;opacity:.5;width:20px;height:20px;background:red">Hidden group</div>`)
	defer Dispose(world)
	texture := graphics.CreateTextureByImageNoMipmap(image.NewRGBA(image.Rect(0, 0, 10, 10)))
	defer texture.Dispose()
	world.InsertResource[ImageProvider](testHTMLImageProvider{texture: texture})
	incrementalSync(t, world)
	state := world.GetRef[htmlRenderState](owner)
	if len(state.Textures.entries) != 0 || len(state.Textures.rasters) != 0 {
		t.Fatal("fully hidden content generated paint textures or composite rasters")
	}
	target := incrementalHTMLNode(t, world, owner, "target")
	img := incrementalHTMLNode(t, world, owner, "image")
	if htmlCullRole(world, owner, img, htmlRoleImage) != 0 {
		t.Fatal("hidden image generated a visual")
	}
	target.SetAttribute("class", "")
	incrementalSync(t, world)
	if len(state.Textures.entries) == 0 || htmlCullRole(world, owner, img, htmlRoleImage) == 0 {
		t.Fatal("showing content did not restore textures and image")
	}
}

func TestHTMLVisibilityPreservesLayoutAndVisibleDescendants(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>.hidden{visibility:hidden}</style><div id="parent" class="hidden" tabindex="0" style="height:100px;overflow:auto;background:red"><div id="hidden" style="height:20px;background:blue;border:2px solid black;box-shadow:0 0 3px black">Hidden</div><div id="shown" style="visibility:visible;height:180px;background:#00ff00"><a id="link" href="#target">Visible</a></div></div><div id="after">After</div>`)
	defer Dispose(world)
	parent := incrementalHTMLNode(t, world, owner, "parent")
	hidden := incrementalHTMLNode(t, world, owner, "hidden")
	shown := incrementalHTMLNode(t, world, owner, "shown")
	after := incrementalHTMLNode(t, world, owner, "after")
	state := world.GetRef[htmlRenderState](owner)
	parentBox, afterRect := state.BoxesByNode[parent.node], state.BoxesByNode[after.node].BorderBox()
	clip := state.OverflowBindings[parent.node].entity
	scroll, _ := world.Get[scene.UIScroll](clip)
	if !scroll.DisableInput || !scene.IsVisible(world, clip) {
		t.Fatal("hidden scrollport must retain clipping and disable its own input")
	}
	if htmlCullRole(world, owner, hidden, htmlRoleSurface) != 0 || htmlCullRole(world, owner, hidden, htmlRoleShadow) != 0 {
		t.Fatal("hidden box generated painted surfaces")
	}
	if entity := htmlCullRole(world, owner, shown, htmlRoleSurface); entity == 0 || !scene.IsVisible(world, entity) {
		t.Fatal("explicitly visible descendant is missing or hidden")
	}
	for _, part := range state.OverflowBindings[parent.node].parts {
		if scene.IsVisible(world, part) {
			t.Fatal("hidden scrollport painted a scrollbar")
		}
	}
	for _, class := range []string{"", "hidden", ""} {
		parent.SetAttribute("class", class)
		incrementalSync(t, world)
		state = world.GetRef[htmlRenderState](owner)
		current, _ := world.Get[scene.UIScroll](clip)
		if state.SyncStats.Layouts != 0 || state.BoxesByNode[parent.node] != parentBox || state.BoxesByNode[after.node].BorderBox() != afterRect || current.ContentSize != scroll.ContentSize {
			t.Fatalf("visibility changed geometry or scroll extent: %+v", state.SyncStats)
		}
		if current.DisableInput != (class == "hidden") {
			t.Fatal("scroll input did not follow visibility")
		}
		if entity := htmlCullRole(world, owner, shown, htmlRoleSurface); entity == 0 || !scene.IsVisible(world, entity) {
			t.Fatal("visible descendant disappeared during parent toggle")
		}
	}
}

func TestHTMLVisibilityInlineFragmentsAndFocus(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="parent" tabindex="0" style="visibility:hidden">Hidden <span id="shown" style="visibility:visible">Visible</span><a id="link" href="#x" style="visibility:visible">Link</a><span id="hidden">Hidden too</span><img id="image" src="missing.png" width="20" height="20"></div>`)
	defer Dispose(world)
	state := world.GetRef[htmlRenderState](owner)
	seen := false
	for _, entity := range state.Children {
		child, _ := world.Get[htmlChild](entity)
		computed := state.Styles[child.Node]
		if text, ok := world.Get[scene.Text2D](entity); ok {
			if !htmlStyleVisible(computed) {
				t.Fatalf("hidden text emitted: %q", text.Value)
			}
			seen = true
			if route, ok := world.Get[FocusOwner](entity); ok && !scene.IsInteractive(world, route.Entity) {
				t.Fatal("visible fragment routed focus to its hidden ancestor")
			}
		}
	}
	if !seen {
		t.Fatal("visible inline fragments missing")
	}
	parent := incrementalHTMLNode(t, world, owner, "parent")
	focus := htmlCullRole(world, owner, parent, htmlRoleFocusable)
	if focus == 0 || scene.IsInteractive(world, focus) || SetControlFocus(world, focus, true) {
		t.Fatal("hidden focus target accepted focus")
	}
	link := incrementalHTMLNode(t, world, owner, "link")
	if entity := htmlCullRole(world, owner, link, htmlRoleFocusable); entity == 0 || !scene.IsInteractive(world, entity) {
		t.Fatal("visible inline link cannot receive focus")
	}
}

func TestHTMLVisibilityRetainsNativeControlState(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>.hidden{visibility:hidden}</style><div id="parent"><input id="field" value="initial"><input id="check" type="checkbox" checked><select id="select"><option>One</option></select></div>`)
	defer Dispose(world)
	parent := incrementalHTMLNode(t, world, owner, "parent")
	field := incrementalHTMLNode(t, world, owner, "field")
	control := htmlCullRole(world, owner, field, htmlRoleControl)
	value := world.GetMut[Control](control)
	value.Value, value.DirtyValue, value.SelectionAnchor, value.SelectionFocus = "edited", true, 1, 4
	for _, class := range []string{"hidden", "", "hidden", ""} {
		parent.SetAttribute("class", class)
		incrementalSync(t, world)
		if next := htmlCullRole(world, owner, field, htmlRoleControl); next != control {
			t.Fatal("visibility replaced the native control")
		}
		value := world.GetRef[Control](control)
		if value.Value != "edited" || value.SelectionAnchor != 1 || value.SelectionFocus != 4 || !value.DirtyValue {
			t.Fatalf("visibility lost live editing state: %+v", value)
		}
		if scene.IsInteractive(world, control) != (class == "") {
			t.Fatal("control interaction did not follow visibility")
		}
		for _, id := range []string{"check", "select"} {
			node := incrementalHTMLNode(t, world, owner, id)
			role := htmlRoleCheckbox
			if id == "select" {
				role = htmlRoleIndicator
			}
			if entity := htmlCullRole(world, owner, node, role); entity != 0 && scene.IsVisible(world, entity) != (class == "") {
				t.Fatalf("%s native visual ignored visibility", id)
			}
		}
	}
}

func TestHTMLVisibilityCompositeOnlyPaintsVisibleContent(t *testing.T) {
	for _, effect := range []string{"opacity:.5", "filter:invert(1)"} {
		t.Run(effect, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, `<div id="group" style="visibility:hidden;width:40px;height:40px;background:red;`+effect+`"><div id="child" style="visibility:visible;width:20px;height:20px;background:blue"></div></div>`)
			defer Dispose(world)
			group := incrementalHTMLNode(t, world, owner, "group")
			child := incrementalHTMLNode(t, world, owner, "child")
			state := world.GetRef[htmlRenderState](owner)
			raster := state.Textures.rasters[group.node]
			if raster == nil || raster.image.Rect.Dx() != 20 || raster.image.Rect.Dy() != 20 {
				t.Fatalf("hidden ancestor enlarged composite bounds: %+v", raster)
			}
			pixel := raster.image.RGBAAt(10, 10)
			if effect == "opacity:.5" && (pixel.B != 255 || pixel.R != 0 || pixel.A != 128) || effect == "filter:invert(1)" && (pixel.R != 255 || pixel.G != 255 || pixel.B != 0 || pixel.A != 255) {
				t.Fatalf("visible child lost its ancestor effect: %v", pixel)
			}
			if entity := htmlCullRole(world, owner, group, htmlRoleSurface); entity == 0 || !scene.IsVisible(world, entity) {
				t.Fatal("hidden ancestor suppressed its visible composite content")
			}
			child.SetAttribute("style", "width:20px;height:20px;background:blue")
			incrementalSync(t, world)
			if htmlCullRole(world, owner, group, htmlRoleSurface) != 0 {
				t.Fatal("fully hidden composite still emitted a surface")
			}
			child.SetAttribute("style", "visibility:visible;width:20px;height:20px;background:blue")
			incrementalSync(t, world)
			if htmlCullRole(world, owner, group, htmlRoleSurface) == 0 {
				t.Fatal("visible descendant did not restore composite")
			}
		})
	}
}

func TestHTMLVisibilityHiddenChildRestoresCoveredBackground(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>.hidden{visibility:hidden}</style><div id="parent" style="width:40px;height:40px;overflow:hidden;background:red"><div id="child" style="height:40px;background:blue"></div></div>`)
	defer Dispose(world)
	parent := incrementalHTMLNode(t, world, owner, "parent")
	child := incrementalHTMLNode(t, world, owner, "child")
	if htmlCullRole(world, owner, parent, htmlRoleSurface) != 0 {
		t.Fatal("fixture child did not cover parent background")
	}
	child.SetAttribute("class", "hidden")
	incrementalSync(t, world)
	entity := htmlCullRole(world, owner, parent, htmlRoleSurface)
	sprite, ok := world.Get[scene.Sprite2D](entity)
	if !ok || sprite.Shape.Fill != graphics.ColorFromRGB(255, 0, 0) || !scene.IsVisible(world, entity) {
		t.Fatal("hiding the covering child did not restore the parent background")
	}
}

func TestHTMLVisibilityOwnerHideShow(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<input id="hidden" style="visibility:hidden"><input id="shown"><div style="visibility:hidden;overflow:hidden;height:40px"><button id="child" style="visibility:visible">Visible</button></div>`)
	for _, visible := range []bool{false, true} {
		_ = world.Insert(owner, scene.Visibility{Visible: visible})
		incrementalSync(t, world)
		for _, id := range []string{"hidden", "shown", "child"} {
			node := incrementalHTMLNode(t, world, owner, id)
			entity := htmlCullRole(world, owner, node, htmlRoleControl)
			if scene.IsVisible(world, entity) != (visible && id != "hidden") {
				t.Fatalf("owner visible=%v changed CSS visibility of %s", visible, id)
			}
		}
	}
}

func TestHTMLVisibilityHoverDoesNotRunLayout(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>#parent:hover #target{visibility:hidden}</style><div id="parent"><button id="trigger">Hover</button><div id="target" style="height:20px;background:red">Target</div></div>`)
	defer Dispose(world)
	trigger := incrementalHTMLNode(t, world, owner, "trigger")
	target := incrementalHTMLNode(t, world, owner, "target")
	control := htmlCullRole(world, owner, trigger, htmlRoleControl)
	for _, hover := range []bool{true, false, true, false} {
		if !SetControlHover(world, control, hover) {
			t.Fatal("could not update hover")
		}
		incrementalSync(t, world)
		state := world.GetRef[htmlRenderState](owner)
		if state.SyncStats.Layouts != 0 {
			t.Fatalf("hover visibility ran layout: %+v", state.SyncStats)
		}
		if (htmlCullRole(world, owner, target, htmlRoleSurface) != 0) == hover {
			t.Fatalf("hover=%v target visibility did not update", hover)
		}
	}
}

func TestHTMLVisibilityHoverRefreshesCompositedInlineText(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>#trigger:hover + #group span{visibility:hidden}</style><button id="trigger">Hover</button><div id="group" style="opacity:.5;width:80px;height:30px"><span>Visible text</span></div>`)
	defer Dispose(world)
	trigger := incrementalHTMLNode(t, world, owner, "trigger")
	group := incrementalHTMLNode(t, world, owner, "group")
	control := htmlCullRole(world, owner, trigger, htmlRoleControl)
	for _, hover := range []bool{true, false, true, false} {
		SetControlHover(world, control, hover)
		incrementalSync(t, world)
		state := world.GetRef[htmlRenderState](owner)
		raster := state.Textures.rasters[group.node]
		if raster == nil || state.SyncStats.Layouts != 0 {
			t.Fatal("visibility lost the composite or ran layout")
		}
		painted := false
		for i := 3; i < len(raster.image.Pix); i += 4 {
			painted = painted || raster.image.Pix[i] != 0
		}
		if painted == hover {
			t.Fatalf("hover=%v reused stale inline text pixels", hover)
		}
	}
}

func TestHTMLVisibilityMutationMatchesFreshRender(t *testing.T) {
	for _, content := range []string{
		`<div style="height:25px;background:red">Hidden<span style="visibility:visible">Visible</span></div><input value="Value">`,
		`<button style="overflow:hidden"><span style="visibility:visible">Visible</span>Hidden</button>`,
		`<div style="opacity:.5"><span>Hidden</span><div style="visibility:visible;background:red;height:20px"></div></div>`,
	} {
		source := `<style>.hidden{visibility:hidden}</style><div id="target" class="hidden" style="overflow:auto;height:60px">` + content + `</div>`
		world, owner := incrementalHTMLFixture(t, source)
		defer Dispose(world)
		target := incrementalHTMLNode(t, world, owner, "target")
		for _, class := range []string{"", "hidden", ""} {
			target.SetAttribute("class", class)
			incrementalSync(t, world)
			freshSource := `<style>.hidden{visibility:hidden}</style><div id="target" class="` + class + `" style="overflow:auto;height:60px">` + content + `</div>`
			fresh, freshOwner := incrementalHTMLFixture(t, freshSource)
			got, _ := world.Get[htmlRenderState](owner)
			want, _ := fresh.Get[htmlRenderState](freshOwner)
			compareHTMLLayout(t, got.Boxes, want.Boxes)
			if !equalHTMLPrimitiveSnapshots(htmlPrimitiveSnapshot(world, got), htmlPrimitiveSnapshot(fresh, want)) {
				t.Fatalf("visibility=%q differs from fresh rendering for %s", class, content)
			}
			Dispose(fresh)
		}
	}
}

func TestHTMLVisibilityScrollCullRestoration(t *testing.T) {
	source := `<style>.hidden{visibility:hidden}</style><div id="scroll" style="height:50px;overflow:auto"><div style="height:500px"></div><div id="target" style="height:20px;background:red">Target</div><div style="height:500px"></div></div>`
	world, owner := incrementalHTMLFixture(t, source)
	defer Dispose(world)
	target := incrementalHTMLNode(t, world, owner, "target")
	for _, class := range []string{"hidden", ""} {
		target.SetAttribute("class", class)
		incrementalSync(t, world)
		htmlCullScroll(t, world, owner, "scroll", 0, -480)
		if got := htmlCullRole(world, owner, target, htmlRoleSurface) != 0; got != (class == "") {
			t.Fatalf("scroll restored wrong paint for class %q", class)
		}
		htmlCullScroll(t, world, owner, "scroll", 0, 0)
	}
}

func BenchmarkHTMLVisibilityNativeControlToggle(b *testing.B) {
	world, owner := incrementalHTMLFixture(b, `<style>.hidden{visibility:hidden}</style><input id="target" value="edited">`)
	target := incrementalHTMLNode(b, world, owner, "target")
	for range 3 {
		target.SetAttribute("class", "hidden")
		incrementalSync(b, world)
		target.SetAttribute("class", "")
		incrementalSync(b, world)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		class := ""
		if i%2 == 0 {
			class = "hidden"
		}
		target.SetAttribute("class", class)
		if err := Sync(world); err != nil {
			b.Fatal(err)
		}
	}
}
