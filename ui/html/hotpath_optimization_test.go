package html

import (
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestHTMLScrollbarHoverAndCapturedDragUsePaintedTrack(t *testing.T) {
	for _, axis := range []uint8{ScrollbarHorizontal, ScrollbarVertical} {
		world, owner := incrementalHTMLFixture(t, `<div id="area" style="width:100px;height:100px;overflow:scroll;scrollbar-width:thin"><div style="width:300px;height:300px"></div></div>`)
		state := ecs.GetRef[htmlRenderState](world, owner)
		node := state.NodeByID["area"]
		binding := state.OverflowBindings[node]
		pointer := input.NewState()
		ecs.InsertResource(world, pointer)
		SyncScrollbars(world)
		if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		var thumb ecs.Entity
		for _, entity := range binding.parts {
			part, _ := ecs.Get[ScrollbarPart](world, entity)
			if part.Axis == axis && part.Kind == ScrollbarThumb {
				thumb = entity
			}
		}
		if thumb == 0 {
			t.Fatal("overflow thumb missing")
		}
		if target, ok := ecs.Get[scene.InputTarget2D](world, thumb); !ok || target.Width <= 0 || target.Height <= 0 {
			t.Fatalf("overflow thumb input target = %+v, ok=%v", target, ok)
		}
		size, _ := ecs.Get[scene.Size2D](world, thumb)
		origin, _ := ecs.Get[scene.UIGlobalTransform](world, thumb)
		x, y := float64(origin.Matrix[12]+size.Width/2), float64(origin.Matrix[13]+size.Height/2)
		thickness := func() float32 {
			size, _ := ecs.Get[scene.Size2D](world, thumb)
			if axis == ScrollbarHorizontal {
				return size.Height
			}
			return size.Width
		}
		if thickness() != 6 {
			t.Fatalf("normal thickness = %v, want 6", thickness())
		}
		pointer.MoveMouse(x, y)
		SyncScrollbars(world)
		if thickness() != 10 {
			t.Fatalf("hover thickness = %v, want 10", thickness())
		}
		pointer.PressMouse(input.MouseLeft, x, y)
		if err := scene.UIScrollSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		pointer.EndFrame()
		if axis == ScrollbarHorizontal {
			x, y = x+30, y-50
		} else {
			x, y = x-50, y+30
		}
		pointer.MoveMouse(x, y)
		if err := scene.UIScrollSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		SyncScrollbars(world)
		scroll, _ := ecs.Get[scene.UIScroll](world, binding.entity)
		if scroll.Position[axis] >= 0 || thickness() != 10 {
			t.Fatalf("captured drag: position=%v thickness=%v", scroll.Position, thickness())
		}
		if scroll.Position[1-axis] != 0 {
			t.Fatal("thumb drag moved the other axis")
		}
		pointer.ReleaseMouse(input.MouseLeft, x, y)
		if err := scene.UIScrollSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		SyncScrollbars(world)
		if thickness() != 6 {
			t.Fatalf("released thickness = %v, want 6", thickness())
		}
		if allocs := testing.AllocsPerRun(20, func() { SyncScrollbars(world) }); allocs != 0 {
			t.Fatalf("hover scrollbar sync allocations = %v", allocs)
		}
	}
}

func TestStableHTMLScrollbarSyncTracksScrollAndVisibility(t *testing.T) {
	world := ecs.NewWorld()
	areas := ecs.NewSpawner1[scene.UIScroll](world)
	area := areas.Spawn(scene.UIScroll{
		ViewportSize: mgl32.Vec2{100, 50}, ContentSize: mgl32.Vec2{200, 150},
		Horizontal: true, Vertical: true, ScrollbarSize: 10, ScrollbarMinThumbSize: 5,
	})
	parts := ecs.NewSpawner4[ScrollbarPart, scene.UITransform, scene.Size2D, scene.Sprite2D](world)
	var thumbs [2]ecs.Entity
	for axis := range uint8(2) {
		for kind := range uint8(2) {
			entity := parts.Spawn(ScrollbarPart{Area: area, Axis: axis, Kind: kind}, scene.NewUITransform(), scene.Size2D{}, scene.NewSprite2D())
			if kind == ScrollbarThumb {
				thumbs[axis] = entity
			}
		}
	}
	SyncScrollbars(world)
	version := world.MutationVersion()
	SyncScrollbars(world)
	if world.MutationVersion() != version {
		t.Fatal("unchanged scrollbars invalidated geometry")
	}
	if allocs := testing.AllocsPerRun(100, func() { SyncScrollbars(world) }); allocs != 0 {
		t.Fatalf("steady scrollbar sync allocations=%v", allocs)
	}
	scroll := ecs.GetMut[scene.UIScroll](world, area)
	scroll.Position = mgl32.Vec2{-100, -100}
	SyncScrollbars(world)
	horizontal, _ := ecs.Get[scene.UITransform](world, thumbs[0])
	vertical, _ := ecs.Get[scene.UITransform](world, thumbs[1])
	if horizontal.Translation.X.Value != 50 || math.Abs(float64(vertical.Translation.Y.Value-100.0/3)) > 1e-4 {
		t.Fatalf("thumb offsets=%v,%v", horizontal.Translation, vertical.Translation)
	}
	scroll = ecs.GetMut[scene.UIScroll](world, area)
	scroll.Horizontal, scroll.Vertical = false, false
	SyncScrollbars(world)
	for _, entity := range thumbs {
		size, _ := ecs.Get[scene.Size2D](world, entity)
		if size != (scene.Size2D{}) {
			t.Fatal("disabled scrollbar still has geometry")
		}
	}
}
