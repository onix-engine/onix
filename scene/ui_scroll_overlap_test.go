package scene

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/go-gl/mathgl/mgl32"
)

func TestUIScrollPopupTakesWheelBeforeDeeperBackground(t *testing.T) {
	world := ecs.NewWorld()
	in := input.NewState()
	ecs.InsertResource(world, in)
	spawn := func(parent ecs.Entity, stack UIStackIndex, horizontal, vertical bool) ecs.Entity {
		bundle := ecs.NewBundle(6)
		ecs.Put(&bundle, UITransformFromXY(0, 0))
		ecs.Put(&bundle, Size2D{Width: 100, Height: 100})
		ecs.Put(&bundle, Visibility{Visible: true})
		ecs.Put(&bundle, Parent{Entity: parent})
		ecs.Put(&bundle, stack)
		ecs.Put(&bundle, UIScroll{ViewportSize: mgl32.Vec2{100, 100}, ContentSize: mgl32.Vec2{300, 300}, WheelStep: 48, Horizontal: horizontal, Vertical: vertical, ConsumeWheel: true})
		return world.Spawn(bundle)
	}
	background := spawn(spawn(0, 0, true, true), 0, true, true)
	popup := spawn(0, 60, false, true)
	in.MoveMouse(50, 50)
	in.ScrollMouse(-1, -1)
	if err := UIScrollSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	front, _ := ecs.Get[UIScroll](world, popup)
	back, _ := ecs.Get[UIScroll](world, background)
	if front.Position != (mgl32.Vec2{0, -48}) || back.Position != (mgl32.Vec2{}) {
		t.Fatalf("popup wheel leaked to background: popup %v, background %v", front.Position, back.Position)
	}
	if in.Pointer().WheelX != 0 || in.Pointer().WheelY != 0 {
		t.Fatal("popup did not consume wheel input")
	}
	// Even a short menu blocks the scrollable panel underneath it.
	ecs.GetMut[UIScroll](world, popup).ContentSize = mgl32.Vec2{100, 100}
	in.EndFrame()
	in.ScrollMouse(0, -1)
	if err := UIScrollSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	back, _ = ecs.Get[UIScroll](world, background)
	if back.Position != (mgl32.Vec2{}) || in.Pointer().WheelY != 0 {
		t.Fatal("short popup allowed the background to scroll")
	}
}

func TestUIScrollHitTestingUsesResolvedPopupClip(t *testing.T) {
	for _, exempt := range []bool{false, true} {
		t.Run(map[bool]string{false: "clipped", true: "popup"}[exempt], func(t *testing.T) {
			world := ecs.NewWorld()
			in := input.NewState()
			ecs.InsertResource(world, in)
			parentBundle := ecs.NewBundle(3)
			ecs.Put(&parentBundle, UITransformFromXY(10, 20))
			ecs.Put(&parentBundle, Size2D{Width: 100, Height: 20})
			ecs.Put(&parentBundle, ClipRect2D{Width: 100, Height: 20})
			parent := world.Spawn(parentBundle)
			bundle := ecs.NewBundle(7)
			ecs.Put(&bundle, UITransformFromXY(0, 20))
			ecs.Put(&bundle, Parent{Entity: parent})
			ecs.Put(&bundle, Size2D{Width: 100, Height: 80})
			ecs.Put(&bundle, Visibility{Visible: true})
			ecs.Put(&bundle, ClipRect2D{Width: 100, Height: 80})
			ecs.Put(&bundle, UIScroll{ViewportSize: mgl32.Vec2{100, 80}, ContentSize: mgl32.Vec2{100, 200}, WheelStep: 20, Vertical: true, ConsumeWheel: true})
			if exempt {
				ecs.Put(&bundle, ClipExempt2D{})
			}
			popup := world.Spawn(bundle)
			if err := SyncUIGlobalTransformSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			in.MoveMouse(30, 70)
			in.ScrollMouse(0, -1)
			if err := UIScrollSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			want := float32(0)
			if exempt {
				want = -20
			}
			if scroll := ecs.GetRef[UIScroll](world, popup); scroll.Position.Y() != want {
				t.Fatalf("exempt=%v scroll=%g, want %g", exempt, scroll.Position.Y(), want)
			}
			in.EndFrame()
			in.MoveMouse(30, 140)
			in.ScrollMouse(0, -1)
			if err := UIScrollSystem("").Run(world); err != nil {
				t.Fatal(err)
			}
			if scroll := ecs.GetRef[UIScroll](world, popup); scroll.Position.Y() != want || in.Pointer().WheelY == 0 {
				t.Fatal("pointer outside the popup viewport scrolled it")
			}
		})
	}
}
