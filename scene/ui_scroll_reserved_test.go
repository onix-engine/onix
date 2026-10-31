package scene

import (
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/go-gl/mathgl/mgl32"
)

func TestClassicScrollbarDraggingOutsideContentClip(t *testing.T) {
	for _, axis := range []int{0, 1} {
		world := ecs.NewWorld()
		state := input.NewState()
		world.InsertResource(state)
		parentBundle := ecs.NewBundle(4)
		parentBundle.Put(NewUITransform())
		parentBundle.Put(Size2D{Width: 100, Height: 100})
		parentBundle.Put(ClipRect2D{Width: 100, Height: 100})
		parent := world.Spawn(parentBundle)
		bundle := ecs.NewBundle(7)
		bundle.Put(NewUITransform())
		bundle.Put(AnchorTopLeft)
		bundle.Put(Parent{Entity: parent})
		bundle.Put(Size2D{Width: 100, Height: 100})
		bundle.Put(ClipRect2D{Width: 90, Height: 90})
		bundle.Put(Visibility{Visible: true})
		bundle.Put(UIScroll{
			ViewportSize: mgl32.Vec2{90, 90}, ContentSize: mgl32.Vec2{300, 300},
			Horizontal: true, Vertical: true, ScrollbarSize: 10, ScrollbarMinThumbSize: 20,
		})
		area := world.Spawn(bundle)
		if err := SyncUIGlobalTransformSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		point := mgl32.Vec2{95, 95}
		point[axis] = 20
		state.PressMouse(input.MouseLeft, float64(point.X()), float64(point.Y()))
		if err := UIScrollSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		state.EndFrame()
		point[axis] += 30
		state.MoveMouse(float64(point.X()), float64(point.Y()))
		if err := UIScrollSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		scroll, _ := world.Get[UIScroll](area)
		if math.Abs(float64(scroll.Position[axis]+100)) > .001 || scroll.Position[1-axis] != 0 {
			t.Fatalf("classic bar drag axis %d=%v, want -100", axis, scroll.Position)
		}
		// The same gutter must reject input when hidden by an ancestor.
		world.GetMut[ClipRect2D](parent).Width = 80
		world.GetMut[ClipRect2D](parent).Height = 80
		if err := SyncUIGlobalTransformSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		if pointInUIScrollContainer(world, area, 100, 100, 1, point.X(), point.Y()) {
			t.Fatal("classic scrollbar accepted a point outside its ancestor clip")
		}
	}
}
