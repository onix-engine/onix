package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
)

func TestHTMLInlineBackgroundFollowsOverflowScroll(t *testing.T) {
	for name, text := range map[string]string{"single": "target", "wrapped": "one two three four five six"} {
		t.Run(name, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, `<div style="overflow:auto;width:180px;height:100px"><div style="height:20px"></div><div id="scroll" style="overflow:auto;width:90px;height:60px"><p id="text"><code id="code" style="background:#333;border-radius:3px"><span>`+text+`</span></code></p><div style="height:1200px"></div></div><div style="height:1200px"></div></div>`)
			defer Dispose(world)
			scroller := incrementalHTMLNode(t, world, owner, "scroll")
			code := incrementalHTMLNode(t, world, owner, "code")
			check := func(offset float32) {
				t.Helper()
				state := world.GetRef[htmlRenderState](owner)
				area := state.OverflowBindings[scroller.node].entity
				world.GetMut[scene.UIScroll](area).Position[1] = offset
				SyncScrollContent(world)
				role := htmlRoleInlineBackground
				if name == "wrapped" {
					role = htmlRoleInlineBackgroundList
				}
				background := htmlAnimationRole(t, world, owner, "code", role)
				for _, entity := range []ecs.Entity{background, state.RenderNodes[code.node.Children[0].Children[0]].Entities[0]} {
					parent, _ := world.Get[scene.Parent](entity)
					content, ok := world.Get[ScrollContent](entity)
					if parent.Entity != area || !ok || content.Area != area {
						t.Fatalf("inline paint lost its scroll and clip binding: entity=%v parent=%v content=%+v", entity, parent.Entity, content)
					}
					if y := world.GetRef[scene.UITransform](entity).Translation.Y.Value; y != content.Y+offset {
						t.Fatalf("inline paint did not follow scroll: y=%g, want %g", y, content.Y+offset)
					}
				}
			}
			check(-10)
			check(-25)
			// Re-layout re-emits paint through the local overflow routing path.
			incrementalHTMLNode(t, world, owner, "text").SetAttribute("style", "padding-left:1px")
			incrementalSync(t, world)
			if state := world.GetRef[htmlRenderState](owner); state.SyncStats.PartialReconciles != 1 {
				t.Fatalf("layout did not use local reconciliation: %+v", state.SyncStats)
			}
			check(-15)
			// Changing the clipping context clears and rebuilds inline routes.
			scroller.SetAttribute("style", "overflow:scroll;width:90px;height:60px")
			incrementalSync(t, world)
			if state := world.GetRef[htmlRenderState](owner); state.SyncStats.PartialReconciles != 1 {
				t.Fatalf("clip update did not use local reconciliation: %+v", state.SyncStats)
			}
			check(-15)
			// Scrolling out of the paint buffer and back mounts new fragments.
			htmlCullScroll(t, world, owner, "scroll", 0, -1000)
			htmlCullScroll(t, world, owner, "scroll", 0, 0)
			check(-20)
		})
	}
}

func TestHTMLInlineBackgroundPaintsExactTextInsideControl(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<button id="row" style="display:flex;flex-direction:column;width:280px"><span>file.go:1</span><span style="display:block;white-space:pre;color:white">before <span id="match" style="background:#ff800080">tar<span style="color:lime">get</span></span> after</span></button>`)
	match := incrementalHTMLNode(t, world, owner, "match")
	control, _ := ControlByID(world, owner, "row")
	state, _ := world.Get[htmlRenderState](owner)
	count := 0
	for _, access := range world.Query2[htmlChild, scene.Sprite2D]().All {
		child, sprite := access.A.Ref(), access.B.Ref()
		if child.Node != match.node || child.role != htmlRoleInlineBackground {
			continue
		}
		count++
		if sprite.Shape.Fill != graphics.ColorFromRGBA(255, 128, 0, 128) || sprite.Width <= 0 || sprite.Height <= 0 || sprite.Width >= 100 {
			t.Fatalf("incorrect match paint: %+v", sprite)
		}
	}
	if count != 1 {
		t.Fatalf("nested syntax fragments need one background, got %d", count)
	}
	background := htmlAnimationRole(t, world, owner, "match", htmlRoleInlineBackground)
	if parent, _ := world.Get[scene.Parent](background); parent.Entity != control {
		t.Fatal("inline paint lost its button/scroll transform")
	}
	for _, entity := range state.RenderNodes[match.node.Children[0]].Entities {
		if _, ok := world.Get[scene.Text2D](entity); ok {
			textStack, _ := world.Get[scene.UIStackIndex](entity)
			paintStack, _ := world.Get[scene.UIStackIndex](background)
			if textStack <= paintStack {
				t.Fatal("highlight paints over the glyphs")
			}
		}
	}
	match.SetAttribute("style", "background:#00ff0080")
	incrementalSync(t, world)
	if sprite, _ := world.Get[scene.Sprite2D](background); sprite.Shape.Fill != graphics.ColorFromRGBA(0, 255, 0, 128) {
		after := htmlAnimationRole(t, world, owner, "match", htmlRoleInlineBackground)
		next, _ := world.Get[scene.Sprite2D](after)
		t.Fatalf("inline background did not follow a paint-only update: old=%v new=%v paint=%+v next=%+v", background, after, sprite.Shape.Fill, next.Shape.Fill)
	}
	if allocations := testing.AllocsPerRun(100, func() { incrementalSync(t, world) }); allocations != 0 {
		t.Fatalf("idle inline background allocated %g times", allocations)
	}
}

func TestHTMLInlineBackgroundWrapsAndRetainsPaint(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="text" style="width:90px;line-height:22px"><span id="match" style="background:#ffaa0080">one <b>two three</b> four five six</span> outside</div>`)
	state, _ := world.Get[htmlRenderState](owner)
	text := incrementalHTMLNode(t, world, owner, "text")
	box := state.boxForNode(text.node)
	if len(box.InlineBackgrounds) < 2 {
		t.Fatalf("missing wrapped backgrounds: %+v", box.InlineBackgrounds)
	}
	for i, background := range box.InlineBackgrounds {
		if background.Rect.Width > box.Content.Width+.01 || i > 0 && background.Line <= box.InlineBackgrounds[i-1].Line {
			t.Fatalf("inline lines overlap or overpaint outside text: %+v", box.InlineBackgrounds)
		}
	}
	for range 2 {
		incrementalHTMLNode(t, world, owner, "match").SetAttribute("style", "background:#ffaa0080;color:blue")
		incrementalSync(t, world)
		incrementalHTMLNode(t, world, owner, "match").SetAttribute("style", "background:#ffaa0080;color:red")
		incrementalSync(t, world)
	}
	if allocations := testing.AllocsPerRun(100, func() { box.UpdateInlineBackgrounds(state.Styles) }); allocations != 0 {
		t.Fatalf("warm inline fragment collection allocated %g times", allocations)
	}
}

func TestHTMLWrappedInlineBackgroundUsesPaintList(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div style="width:72px;line-height:20px"><span id="match" style="background:#cc2200">one two three four five six</span></div>`)
	defer Dispose(world)
	node := incrementalHTMLNode(t, world, owner, "match")
	group := world.GetRef[htmlRenderState](owner).EntityGroups[htmlRenderIdentity{Node: node.node, Role: htmlRoleInlineBackgroundList}]
	if group == nil || len(group.Bindings) != 1 {
		t.Fatal("wrapped background did not produce one paint list")
	}
	entity := group.Bindings[0].Entity
	layers, ok := world.Get[scene.SpriteLayers2D](entity)
	if !ok || len(layers.Layers) < 2 {
		t.Fatalf("paint list has %d fragments", len(layers.Layers))
	}
	for _, layer := range layers.Layers {
		if layer.Sprite.Shape.Fill != graphics.ColorFromRGB(204, 34, 0) {
			t.Fatalf("wrong inline paint: %+v", layer.Sprite.Shape.Fill)
		}
	}
	node.SetAttribute("style", "background:#00aa33")
	incrementalSync(t, world)
	updated, ok := world.Get[scene.SpriteLayers2D](entity)
	if !ok || len(updated.Layers) != len(layers.Layers) || updated.Layers[0].Sprite.Shape.Fill != graphics.ColorFromRGB(0, 170, 51) {
		t.Fatalf("local paint update did not reuse and refresh the list: present=%v count=%d previous=%d color=%v", ok, len(updated.Layers), len(layers.Layers), updated.Layers[0].Sprite.Shape.Fill)
	}
	if allocations := testing.AllocsPerRun(50, func() { incrementalSync(t, world) }); allocations != 0 {
		t.Fatalf("idle paint list allocated %g times", allocations)
	}
	node.SetAttribute("style", "background:linear-gradient(red,blue)")
	incrementalSync(t, world)
	if world.IsAlive(entity) && world.Has[scene.SpriteLayers2D](entity) {
		t.Fatal("paint list survived an unsupported gradient style")
	}
}

func TestHTMLInlineBackgroundSurvivesOpacityComposite(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="group" style="width:160px;opacity:.5"><span id="match" style="background:#ff0000;color:#000000">target</span></div>`)
	defer Dispose(world)
	group := incrementalHTMLNode(t, world, owner, "group")
	state, _ := world.Get[htmlRenderState](owner)
	raster := state.Textures.rasters[group.node]
	if raster == nil {
		t.Fatal("missing opacity raster")
	}
	visual := htmlCullRole(world, owner, group, htmlRoleSurface)
	sprite, _ := world.Get[scene.Sprite2D](visual)
	if sprite.Alpha != .5 {
		t.Fatalf("group alpha = %v, want .5", sprite.Alpha)
	}
	check := func(red, green uint8) {
		t.Helper()
		found := false
		for i := 0; i < len(raster.image.Pix); i += 4 {
			if raster.image.Pix[i] == red && raster.image.Pix[i+1] == green && raster.image.Pix[i+3] == 255 {
				found = true
				break
			}
		}
		if !found {
			box := state.boxForNode(group.node)
			t.Fatalf("inline background missing from opacity raster: backgrounds=%+v rect=%+v pixels=%v", box.InlineBackgrounds, raster.rect, raster.image.Pix[:min(64, len(raster.image.Pix))])
		}
	}
	check(255, 0)
	version := raster.version
	incrementalHTMLNode(t, world, owner, "match").SetAttribute("style", "background:#00ff00;color:#000000")
	incrementalSync(t, world)
	if raster.version <= version {
		t.Fatal("inline paint mutation did not refresh the opacity group")
	}
	check(0, 255)
}
