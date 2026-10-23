package html

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func htmlCullRole(world *ecs.World, owner ecs.Entity, node Node, role htmlRenderRole) ecs.Entity {
	state := ecs.GetRef[htmlRenderState](world, owner)
	if rendered := state.RenderNodes[node.node]; rendered != nil {
		for _, entity := range rendered.Entities {
			if child := ecs.GetRef[htmlChild](world, entity); child != nil && child.role == role {
				return entity
			}
		}
	}
	return 0
}

func htmlCullScroll(t testing.TB, world *ecs.World, owner ecs.Entity, id string, x, y float32) {
	t.Helper()
	node := incrementalHTMLNode(t, world, owner, id)
	state := ecs.GetRef[htmlRenderState](world, owner)
	scroll := ecs.GetMut[scene.UIScroll](world, state.OverflowBindings[node.node].entity)
	if scroll == nil {
		t.Fatal("missing scroll container")
	}
	scroll.Position[0], scroll.Position[1] = x, y
	incrementalSync(t, world)
}

func TestHTMLPaintCullSurfacesAndScrollBuffer(t *testing.T) {
	var source strings.Builder
	source.WriteString(`<div id="scroll" style="width:180px;height:100px;overflow:auto;background:#fff">`)
	for i := range 400 {
		fmt.Fprintf(&source, `<div id="row%d" style="height:20px;background:linear-gradient(red,blue);border-top:1px solid red;border-bottom:1px solid blue">Row %d</div>`, i, i)
	}
	source.WriteString(`</div>`)
	// An unrelated branch must not be visited by the scroll cull walk.
	source.WriteString(strings.Repeat(`<div>Unrelated</div>`, 1000))
	world, owner := incrementalHTMLFixture(t, source.String())
	state := ecs.GetRef[htmlRenderState](world, owner)
	root := state.Boxes
	scrollNode := incrementalHTMLNode(t, world, owner, "scroll")
	head := incrementalHTMLNode(t, world, owner, "row0")
	tail := incrementalHTMLNode(t, world, owner, "row399")
	background := htmlCullRole(world, owner, scrollNode, htmlRoleSurface)
	if background == 0 || htmlCullRole(world, owner, head, htmlRoleSurface) == 0 || htmlCullRole(world, owner, tail, htmlRoleSurface) != 0 {
		t.Fatal("initial surface window did not cull the tail")
	}
	if state.Textures.paint[htmlTextureKey{node: tail.node, variant: "background"}] != nil {
		t.Fatal("offscreen background was rasterized")
	}
	initial := slices.Clone(state.Children)
	htmlCullScroll(t, world, owner, "scroll", 0, -64)
	state = ecs.GetRef[htmlRenderState](world, owner)
	if state.SyncStats.PaintCullBoxes != 0 || state.SyncStats.ReconciledBoxes != 0 || !slices.Equal(initial, state.Children) {
		t.Fatalf("small scroll rebuilt the buffer: %+v", state.SyncStats)
	}
	clip := state.OverflowBindings[scrollNode.node].entity
	if allocations := testing.AllocsPerRun(100, func() {
		scroll := ecs.GetMut[scene.UIScroll](world, clip)
		scroll.Position[1] = -32
		if err := Sync(world); err != nil {
			panic(err)
		}
		scroll = ecs.GetMut[scene.UIScroll](world, clip)
		scroll.Position[1] = -64
		if err := Sync(world); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("buffered scroll allocated %v objects", allocations)
	}
	htmlCullScroll(t, world, owner, "scroll", 0, -8700)
	state = ecs.GetRef[htmlRenderState](world, owner)
	if htmlCullRole(world, owner, head, htmlRoleSurface) != 0 || htmlCullRole(world, owner, tail, htmlRoleSurface) == 0 {
		t.Fatal("scroll did not retire the head and mount the tail")
	}
	paint := state.Textures.paint[htmlTextureKey{node: tail.node, variant: "background"}]
	if paint == nil {
		t.Fatal("visible gradient background and borders were not rasterized")
	}
	if pixel := paint.image.RGBAAt(1, paint.image.Rect.Dy()-1); pixel.B != 255 || pixel.R != 0 {
		t.Fatalf("bottom border was not restored: %v", pixel)
	}

	if state.SyncStats.PaintCullBoxes > 401 || state.SyncStats.Layouts != 0 || state.SyncStats.Styled != 0 || state.Boxes != root {
		t.Fatalf("scroll visited unrelated boxes or ran layout: %+v", state.SyncStats)
	}
	if htmlCullRole(world, owner, scrollNode, htmlRoleSurface) != background {
		t.Fatal("scroll moved or retired its own background")
	}
	htmlCullScroll(t, world, owner, "scroll", 0, 0)
	if htmlCullRole(world, owner, head, htmlRoleSurface) == 0 || htmlCullRole(world, owner, tail, htmlRoleSurface) != 0 {
		t.Fatal("returning to the start did not restore the head")
	}
}

func TestHTMLPaintCullHorizontalAndVisibleOverflow(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroll" style="width:120px;height:50px;overflow:auto"><div style="position:relative;width:8000px;height:20px"><div id="head" style="position:absolute;left:0;width:20px;height:20px;background:red">A</div><div id="tail" style="position:absolute;left:7800px;width:20px;height:20px;background:blue">B</div></div></div><div id="unclipped" style="position:absolute;left:9000px;top:9000px;width:20px;height:20px;background:red">Overflow</div>`)
	head := incrementalHTMLNode(t, world, owner, "head")
	tail := incrementalHTMLNode(t, world, owner, "tail")
	unclipped := incrementalHTMLNode(t, world, owner, "unclipped")
	if htmlCullRole(world, owner, head, htmlRoleSurface) == 0 || htmlCullRole(world, owner, tail, htmlRoleSurface) != 0 || htmlCullRole(world, owner, unclipped, htmlRoleSurface) == 0 {
		t.Fatal("horizontal clip or visible overflow was not respected")
	}
	htmlCullScroll(t, world, owner, "scroll", -7800, 0)
	if htmlCullRole(world, owner, head, htmlRoleSurface) != 0 || htmlCullRole(world, owner, tail, htmlRoleSurface) == 0 {
		t.Fatal("horizontal scroll did not move the paint window")
	}
}

func TestHTMLPaintCullRetainsNestedClipScrollRoute(t *testing.T) {
	for _, mode := range []string{"hidden", "clip", "auto"} {
		t.Run(mode, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, `<div id="scroll" style="width:180px;height:100px;overflow:auto"><div style="height:1200px"></div><button id="row" style="display:flex;width:160px;height:28px;overflow:`+mode+`"><span id="label" style="overflow:hidden">File.go</span></button><div style="height:200px"></div></div>`)
			state := ecs.GetRef[htmlRenderState](world, owner)
			scroll := incrementalHTMLNode(t, world, owner, "scroll")
			row := incrementalHTMLNode(t, world, owner, "row")
			label := incrementalHTMLNode(t, world, owner, "label")
			text := Node{world: world, owner: owner, node: label.node.Children[0]}
			area := state.OverflowBindings[scroll.node].entity
			clip := state.OverflowBindings[row.node].entity
			initial, ok := ecs.Get[ScrollContent](world, clip)
			if !ok || initial.Area != area {
				t.Fatal("nested clip has no initial scroll route")
			}
			for _, y := range []float32{-1200, -1100, 0, -1200} {
				htmlCullScroll(t, world, owner, "scroll", 0, y)
				content, ok := ecs.Get[ScrollContent](world, clip)
				if !ok || content != initial {
					t.Fatalf("scroll %g lost the nested clip route: %+v, want %+v", y, content, initial)
				}
				if y == -1200 {
					if htmlCullRole(world, owner, text, htmlRoleText) == 0 {
						t.Fatal("visible label was not remounted")
					}
				}
			}
		})
	}
}

func TestHTMLPaintCullShadowAndOverflowingChild(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroll" style="position:relative;width:150px;height:100px;overflow:auto"><div style="height:1000px"></div><div id="shadow" style="position:absolute;top:400px;width:30px;height:20px;background:red;box-shadow:0 -100px 0 blue"></div><div id="parent" style="position:absolute;top:1200px;height:0;width:20px;background:red"><div id="child" style="position:absolute;top:-1150px;width:20px;height:20px;background:blue">Child</div></div></div>`)
	shadow := incrementalHTMLNode(t, world, owner, "shadow")
	parent := incrementalHTMLNode(t, world, owner, "parent")
	child := incrementalHTMLNode(t, world, owner, "child")
	if htmlCullRole(world, owner, shadow, htmlRoleShadow) == 0 || htmlCullRole(world, owner, shadow, htmlRoleSurface) != 0 {
		t.Fatal("shadow overlapping the buffer was culled with its offscreen border box")
	}
	if htmlCullRole(world, owner, parent, htmlRoleSurface) != 0 || htmlCullRole(world, owner, child, htmlRoleSurface) == 0 {
		t.Fatal("offscreen parent hid an overflowing visible child")
	}
	// The shadow still intersects while the border enters the window. Check
	// both extents independently so its background cannot remain missing.
	htmlCullScroll(t, world, owner, "scroll", 0, -150)
	if htmlCullRole(world, owner, shadow, htmlRoleSurface) == 0 {
		t.Fatal("background did not mount while its shadow stayed visible")
	}
}

func TestHTMLPaintCullDefersCompositeRasterAndRestoresLatestPaint(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroll" style="width:120px;height:80px;overflow:auto"><div style="height:1000px"></div><div id="group" style="width:40px;height:20px;opacity:.5;background:red"><div id="child" style="width:20px;height:20px;background:blue"></div></div></div>`)
	group := incrementalHTMLNode(t, world, owner, "group")
	child := incrementalHTMLNode(t, world, owner, "child")
	state := ecs.GetRef[htmlRenderState](world, owner)
	if state.Textures.rasters[group.node] != nil || state.Textures.rasters[child.node] != nil || htmlCullRole(world, owner, group, htmlRoleSurface) != 0 {
		t.Fatal("offscreen opacity group allocated a raster")
	}
	child.SetAttribute("style", "width:20px;height:20px;background:#00ff00")
	incrementalSync(t, world)
	state = ecs.GetRef[htmlRenderState](world, owner)
	if state.Textures.rasters[group.node] != nil {
		t.Fatal("offscreen mutation rasterized the group")
	}
	htmlCullScroll(t, world, owner, "scroll", 0, -1000)
	state = ecs.GetRef[htmlRenderState](world, owner)
	raster := state.Textures.rasters[group.node]
	if raster == nil || htmlCullRole(world, owner, group, htmlRoleSurface) == 0 || htmlCullRole(world, owner, child, htmlRoleSurface) != 0 {
		t.Fatal("visible opacity group was not restored as one composite")
	}
	if pixel := raster.image.RGBAAt(1, 1); pixel.G != 255 || pixel.R != 0 || pixel.A != 128 {
		t.Fatalf("composite restored stale paint: %v", pixel)
	}
}

func TestHTMLPaintCullCompositeIncludesFilterAndChildBounds(t *testing.T) {
	for _, test := range []struct{ name, source string }{
		{"filter", `<div id="group" style="position:absolute;top:400px;width:20px;height:20px;background:red;filter:drop-shadow(0 -100px 0 blue)"></div>`},
		{"child", `<div id="group" style="position:absolute;top:1200px;width:20px;height:0;opacity:.5"><div style="position:absolute;top:-1150px;width:20px;height:20px;background:blue"></div></div>`},
	} {
		t.Run(test.name, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, `<div style="position:relative;width:120px;height:100px;overflow:hidden">`+test.source+`</div>`)
			group := incrementalHTMLNode(t, world, owner, "group")
			if htmlCullRole(world, owner, group, htmlRoleSurface) == 0 {
				t.Fatal("effect with visible output was culled by its border box")
			}
		})
	}
}

func TestHTMLPaintCullRetainsControlsAndTransformedContent(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroll" style="position:relative;width:120px;height:80px;overflow:auto"><div style="height:1000px"></div><input id="field" value="initial"><div id="moved" style="transform:translateY(-1000px);width:20px;height:20px;background:red">Moved</div><div id="dynamic" style="width:20px;height:20px;background:blue"><div id="nested" style="width:10px;height:10px;background:red">Nested</div></div></div>`)
	field := incrementalHTMLNode(t, world, owner, "field")
	moved := incrementalHTMLNode(t, world, owner, "moved")
	dynamic := incrementalHTMLNode(t, world, owner, "dynamic")
	nested := incrementalHTMLNode(t, world, owner, "nested")
	control := htmlCullRole(world, owner, field, htmlRoleControl)
	if control == 0 || htmlCullRole(world, owner, moved, htmlRoleSurface) == 0 {
		t.Fatal("offscreen control or transformed paint was discarded")
	}
	SetControlValue(world, control, "typed")
	live := ecs.GetMut[Control](world, control)
	live.SelectionAnchor, live.SelectionFocus = 1, 3
	if htmlCullRole(world, owner, nested, htmlRoleSurface) != 0 {
		t.Fatal("initial dynamic subtree was not culled")
	}
	dynamic.SetAttribute("style", "transform:translateY(-1000px);width:20px;height:20px;background:blue")
	incrementalSync(t, world)
	if htmlCullRole(world, owner, dynamic, htmlRoleSurface) == 0 || htmlCullRole(world, owner, nested, htmlRoleSurface) == 0 {
		t.Fatal("transform style update did not restore unmounted descendants")
	}
	htmlCullScroll(t, world, owner, "scroll", 0, -1000)
	htmlCullScroll(t, world, owner, "scroll", 0, 0)
	live = ecs.GetMut[Control](world, control)
	if live == nil || htmlCullRole(world, owner, field, htmlRoleControl) != control || live.Value != "typed" || live.SelectionAnchor != 1 || live.SelectionFocus != 3 {
		t.Fatal("scroll discarded live control state")
	}
}

func TestHTMLPaintCullCoalescesNestedScrollRoots(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="outer" style="height:100px;overflow:auto"><div id="inner" style="height:80px;overflow:auto">`+strings.Repeat(`<div style="height:20px;background:red">Row</div>`, 100)+`</div><div style="height:2000px"></div></div>`)
	state := ecs.GetRef[htmlRenderState](world, owner)
	outer := incrementalHTMLNode(t, world, owner, "outer")
	inner := incrementalHTMLNode(t, world, owner, "inner")
	ecs.GetMut[scene.UIScroll](world, state.OverflowBindings[outer.node].entity).Position[1] = -200
	ecs.GetMut[scene.UIScroll](world, state.OverflowBindings[inner.node].entity).Position[1] = -600
	incrementalSync(t, world)
	state = ecs.GetRef[htmlRenderState](world, owner)
	if state.SyncStats.PaintCullBoxes != 103 || state.SyncStats.Layouts != 0 {
		t.Fatalf("nested scroll roots were visited more than once: %+v", state.SyncStats)
	}
	if len(state.PaintCullRoots) != 0 {
		t.Fatal("scroll work retained roots")
	}
}

// A layout query must still report the full document, including culled rows.
func TestHTMLPaintCullKeepsLayoutBounds(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div style="height:50px;overflow:hidden"><div style="height:1000px"></div><div id="tail" style="height:40px;background:red"></div></div>`)
	tail := incrementalHTMLNode(t, world, owner, "tail")
	bounds, ok := tail.Bounds()
	if !ok || bounds.Height != 40 || bounds.Y < 1000 {
		t.Fatalf("offscreen layout bounds were discarded: %+v, %v", bounds, ok)
	}
}

func TestHTMLPaintCullResizeRefreshesDescendants(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroll" style="height:50px;overflow:auto"><div style="height:500px"></div><div id="tail" style="height:20px;background:red">Tail</div></div>`)
	scroll := incrementalHTMLNode(t, world, owner, "scroll")
	tail := incrementalHTMLNode(t, world, owner, "tail")
	if htmlCullRole(world, owner, tail, htmlRoleSurface) != 0 {
		t.Fatal("initial tail was not culled")
	}
	scroll.SetAttribute("style", "height:600px;overflow:auto")
	incrementalSync(t, world)
	if htmlCullRole(world, owner, tail, htmlRoleSurface) == 0 {
		t.Fatal("growing the viewport did not restore unchanged descendants")
	}
	scroll.SetAttribute("style", "height:50px;overflow:auto")
	incrementalSync(t, world)
	if htmlCullRole(world, owner, tail, htmlRoleSurface) != 0 {
		t.Fatal("shrinking the viewport did not cull unchanged descendants")
	}
}

func TestHTMLPaintCullLocalPaintPreservesScrollAnchor(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroll" style="height:80px;overflow:auto;background:red">`+strings.Repeat(`<div style="height:20px;background:blue">Row</div>`, 100)+`</div>`)
	scroll := incrementalHTMLNode(t, world, owner, "scroll")
	htmlCullScroll(t, world, owner, "scroll", 0, -64)
	scroll.SetAttribute("style", "height:80px;overflow:auto;background:linear-gradient(red,blue)")
	incrementalSync(t, world)
	state := ecs.GetRef[htmlRenderState](world, owner)
	if state.PaintCullOffsets[scroll.node][1] != 0 {
		t.Fatal("local paint reset the scroll anchor without re-windowing descendants")
	}
	htmlCullScroll(t, world, owner, "scroll", 0, -200)
	state = ecs.GetRef[htmlRenderState](world, owner)
	if state.SyncStats.PaintCullBoxes == 0 || state.PaintCullOffsets[scroll.node][1] != -200 {
		t.Fatal("scroll failed to refresh the buffered window")
	}
}

func TestHTMLPaintCullNewOffsetBindingRestoresPaint(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div style="height:80px;overflow:auto"><div style="height:1000px"></div><div id="target" style="width:20px;height:20px;background:red">Target</div></div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	if htmlCullRole(world, owner, target, htmlRoleSurface) != 0 {
		t.Fatal("initial target was not culled")
	}
	SetNodeOffset(world, owner, "target", 0, -1000)
	incrementalSync(t, world)
	if htmlCullRole(world, owner, target, htmlRoleSurface) == 0 {
		t.Fatal("new offset binding left moved paint unmounted")
	}
}

func BenchmarkHTMLPaintCullBufferedScroll(b *testing.B) {
	world, owner := incrementalHTMLFixture(b, `<div id="scroll" style="height:80px;overflow:auto">`+strings.Repeat(`<div style="height:20px;background:red">Row</div>`, 1000)+`</div>`)
	node := incrementalHTMLNode(b, world, owner, "scroll")
	state := ecs.GetRef[htmlRenderState](world, owner)
	clip := state.OverflowBindings[node.node].entity
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ecs.GetMut[scene.UIScroll](world, clip).Position[1] = -float32(i % 100)
		if err := Sync(world); err != nil {
			b.Fatal(err)
		}
	}
}

func TestHTMLPaintCullTransparentGroupRetainsFocus(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target" tabindex="0" style="width:30px;height:20px;opacity:0;background:red">Target</div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	state := ecs.GetRef[htmlRenderState](world, owner)
	if state.Textures.rasters[target.node] != nil || htmlCullRole(world, owner, target, htmlRoleSurface) != 0 || htmlCullRole(world, owner, target, htmlRoleFocusable) == 0 {
		t.Fatal("transparent group rasterized or lost its focus target")
	}
	target.SetAttribute("style", "width:30px;height:20px;opacity:.5;background:red")
	incrementalSync(t, world)
	if htmlCullRole(world, owner, target, htmlRoleSurface) == 0 {
		t.Fatal("changing opacity did not restore the deferred surface")
	}
}

func BenchmarkHTMLPaintCullMovedScroll(b *testing.B) {
	world, owner := incrementalHTMLFixture(b, `<div id="scroll" style="height:80px;overflow:auto">`+strings.Repeat(`<div style="height:20px;background:red">Row</div>`, 1000)+`</div>`)
	node := incrementalHTMLNode(b, world, owner, "scroll")
	state := ecs.GetRef[htmlRenderState](world, owner)
	clip := state.OverflowBindings[node.node].entity
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ecs.GetMut[scene.UIScroll](world, clip).Position[1] = -float32(400 + i%2*400)
		if err := Sync(world); err != nil {
			b.Fatal(err)
		}
	}
}

func TestHTMLPaintCullRestoresSeparateBorders(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroll" style="height:80px;overflow:auto"><div style="height:1000px"></div><div id="target" style="width:30px;height:20px;background:red;border-top:1px solid blue;border-left:2px solid black">Target</div></div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	if htmlCullRole(world, owner, target, htmlRoleBorderTop) != 0 || htmlCullRole(world, owner, target, htmlRoleBorderLeft) != 0 {
		t.Fatal("offscreen borders were mounted")
	}
	htmlCullScroll(t, world, owner, "scroll", 0, -1000)
	if htmlCullRole(world, owner, target, htmlRoleBorderTop) == 0 || htmlCullRole(world, owner, target, htmlRoleBorderLeft) == 0 {
		t.Fatal("separate border surfaces were not restored")
	}
}
