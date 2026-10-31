package html

import (
	"testing"

	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestHTMLReservedScrollbarLayoutAndPainting(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="outer" style="width:98px;height:65px;overflow:hidden"><div id="area" style="width:100px;height:80px;overflow:auto;scrollbar-width:thin"><div id="child" style="width:100px;height:200px"></div></div></div>`)
	world.GetMut[Surface](owner).ReserveScrollbarSpace = true
	incrementalSync(t, world)
	SyncScrollbars(world)
	if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	state := world.GetRef[htmlRenderState](owner)
	area := incrementalHTMLNode(t, world, owner, "area")
	box := state.BoxesByNode[area.node]
	binding := state.OverflowBindings[area.node]
	scroll, _ := world.Get[scene.UIScroll](binding.entity)
	if scroll.ScrollbarOverlay || scroll.ViewportSize != (mgl32.Vec2{90, 70}) || !scroll.Horizontal || !scroll.Vertical {
		t.Fatalf("scroll=%+v", scroll)
	}
	if border := box.BorderBox(); border.Width != 100 || border.Height != 80 {
		t.Fatalf("outer size changed: %+v", border)
	}
	clip, _ := world.Get[scene.ClipRect2D](binding.entity)
	if clip.Width != 90 || clip.Height != 70 {
		t.Fatalf("content clip=%+v", clip)
	}
	outer := incrementalHTMLNode(t, world, owner, "outer")
	outerGlobal, _ := world.Get[scene.UIGlobalTransform](state.OverflowBindings[outer.node].entity)
	outerClip, _ := outerGlobal.ClipRect()
	for _, entity := range binding.parts {
		part, _ := world.Get[ScrollbarPart](entity)
		position, _ := world.Get[scene.UITransform](entity)
		size, _ := world.Get[scene.Size2D](entity)
		if part.Kind == ScrollbarTrack && ((part.Axis == ScrollbarHorizontal && position.Translation.Y.Value != 70) || (part.Axis == ScrollbarVertical && position.Translation.X.Value != 90)) {
			t.Fatalf("scrollbar did not occupy its gutter: %+v", position)
		}
		if part.Kind == ScrollbarTrack && ((part.Axis == ScrollbarHorizontal && size.Width != 90) || (part.Axis == ScrollbarVertical && size.Height != 70)) {
			t.Fatalf("tracks overlap in the shared corner: %+v", size)
		}
		global, _ := world.Get[scene.UIGlobalTransform](entity)
		if got, ok := global.ClipRect(); !ok || got != outerClip {
			t.Fatalf("bar must retain outer clip, got %+v want %+v", got, outerClip)
		}
	}
	// Reconciliation must update the mode and thickness on retained UIScroll.
	area.SetAttribute("style", "width:100px;height:80px;overflow:auto;scrollbar-width:none")
	incrementalSync(t, world)
	state = world.GetRef[htmlRenderState](owner)
	binding = state.OverflowBindings[area.node]
	scroll, _ = world.Get[scene.UIScroll](binding.entity)
	if scroll.ScrollbarSize != 0 || scroll.ViewportSize != (mgl32.Vec2{100, 80}) || len(binding.parts) != 0 {
		t.Fatalf("hidden bars retained their gutters: %+v", scroll)
	}
	world.GetMut[Surface](owner).ReserveScrollbarSpace = false
	area.SetAttribute("style", "width:100px;height:80px;overflow:auto;scrollbar-width:thin")
	incrementalSync(t, world)
	state = world.GetRef[htmlRenderState](owner)
	scroll, _ = world.Get[scene.UIScroll](state.OverflowBindings[area.node].entity)
	if !scroll.ScrollbarOverlay || scroll.ScrollbarSize != 10 || scroll.ViewportSize != (mgl32.Vec2{100, 80}) {
		t.Fatalf("overlay mode=%+v", scroll)
	}
}

func TestHTMLReservedScrollbarRelayoutAndIncremental(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="area" style="width:100px;height:80px;overflow:auto;scrollbar-width:thin"><div id="child" style="height:200px">wrap this text across several lines</div></div>`)
	world.GetMut[Surface](owner).ReserveScrollbarSpace = true
	incrementalSync(t, world)
	area := incrementalHTMLNode(t, world, owner, "area")
	child := incrementalHTMLNode(t, world, owner, "child")
	state := world.GetRef[htmlRenderState](owner)
	if width := state.BoxesByNode[child.node].Content.Width; width != 90 {
		t.Fatalf("auto-width child=%v, want 90", width)
	}
	for _, height := range []string{"20px", "200px", "80px", "81px"} {
		child.SetAttribute("style", "height:"+height)
		incrementalSync(t, world)
		state = world.GetRef[htmlRenderState](owner)
		fresh := layout.Cache{ReserveScrollbarSpace: true}
		_, err := fresh.Update(state.Root, state.Styles, state.Key.Font.Face, nil, 320, 200, nil, nil, true, true)
		if err != nil {
			t.Fatal(err)
		}
		if state.Boxes.GeometryHash() != fresh.Root.GeometryHash() {
			t.Fatalf("incremental layout differs at child height %s", height)
		}
		box := state.BoxesByNode[area.node]
		scroll, _ := world.Get[scene.UIScroll](state.OverflowBindings[area.node].entity)
		if scroll.ViewportSize != (mgl32.Vec2{float32(box.ScrollportBox().Width), float32(box.ScrollportBox().Height)}) {
			t.Fatalf("incremental scrollport=%v, box=%+v", scroll.ViewportSize, box.ScrollportBox())
		}
	}
	SyncScrollbars(world)
	if allocs := testing.AllocsPerRun(30, func() { SyncScrollbars(world) }); allocs != 0 {
		t.Fatalf("scrollbar sync allocations=%v", allocs)
	}
}
