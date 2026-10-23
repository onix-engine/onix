package scene

import (
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/go-gl/mathgl/mgl32"
)

func TestUIScrollOverlayThumbDragUsesPaintedTrack(t *testing.T) {
	for axis := range 2 {
		world := ecs.NewWorld()
		state := input.NewState()
		ecs.InsertResource(world, state)
		bundle := ecs.NewBundle(6)
		ecs.Put(&bundle, NewUITransform())
		ecs.Put(&bundle, UIGlobalTransform{Matrix: mgl32.Ident4()})
		ecs.Put(&bundle, Visibility{Visible: true})
		ecs.Put(&bundle, AnchorTopLeft)
		ecs.Put(&bundle, Size2D{Width: 100, Height: 100})
		ecs.Put(&bundle, UIScroll{
			ViewportSize: mgl32.Vec2{100, 100}, ContentSize: mgl32.Vec2{300, 300},
			ScrollbarSize: 10, ScrollbarMinThumbSize: 20, ScrollbarOverlay: true,
			Horizontal: true, Vertical: true,
		})
		area := world.Spawn(bundle)
		// Both tracks are 90px, with a 30px thumb. A 30px drag must
		// scroll halfway through the 200px range, even outside the track.
		point := mgl32.Vec2{15, 95}
		if axis == 1 {
			point = mgl32.Vec2{95, 15}
		}
		state.PressMouse(input.MouseLeft, float64(point.X()), float64(point.Y()))
		if err := UIScrollSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		state.EndFrame()
		point[axis] += 30
		point[1-axis] -= 50
		state.MoveMouse(float64(point.X()), float64(point.Y()))
		if err := UIScrollSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		scroll, _ := ecs.Get[UIScroll](world, area)
		if math.Abs(float64(scroll.Position[axis]+100)) > 0.001 || scroll.Position[1-axis] != 0 {
			t.Fatalf("axis %d overlay drag = %v, want -100 on the dragged axis", axis, scroll.Position)
		}
		if uiScrollScrollbarAxis(&scroll, scroll.ViewportSize, scroll.ContentSize, 100, 100, 95, 95) != uiScrollDragContent {
			t.Fatal("shared scrollbar corner accepted a thumb drag")
		}
	}
}

func TestUIScrollScrollbarDragFollowsPointerDirection(t *testing.T) {
	world := ecs.NewWorld()
	state := input.NewState()
	ecs.InsertResource(world, state)
	request := &RenderRequest{}
	ecs.InsertResource(world, request)
	ecs.InsertResource(world, ViewportSize{Width: 320, Height: 240, ScaleFactor: 1})

	bundle := ecs.NewBundle(7)
	ecs.Put(&bundle, UITransformFromXY(10, 20))
	ecs.Put(&bundle, UIGlobalTransform{Matrix: mgl32.Translate3D(10, 20, 0)})
	ecs.Put(&bundle, Visibility{Visible: true})
	ecs.Put(&bundle, AnchorTopLeft)
	ecs.Put(&bundle, Size2D{Width: 100, Height: 100})
	ecs.Put(&bundle, UIScroll{
		Position:              mgl32.Vec2{0, -40},
		ViewportSize:          mgl32.Vec2{100, 100},
		ContentSize:           mgl32.Vec2{100, 300},
		ScrollbarSize:         10,
		ScrollbarMinThumbSize: 24,
		Vertical:              true,
	})
	area := world.Spawn(bundle)

	state.PressMouse(input.MouseLeft, 105, 45)
	if err := UIScrollSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if request.Take() {
		t.Fatal("stationary scrollbar press requested a render")
	}
	state.EndFrame()
	state.MoveMouse(105, 75)
	if err := UIScrollSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if !request.Take() {
		t.Fatal("scrollbar drag did not request a render")
	}

	scroll, _ := ecs.Get[UIScroll](world, area)
	if offset := -scroll.Position.Y(); offset <= 40 {
		t.Fatalf("downward scrollbar drag offset = %.1f, want greater than 40", offset)
	}
	if offset := -scroll.Position.Y(); offset < 120 || offset > 140 {
		t.Fatalf("downward scrollbar drag offset = %.1f, want about 130", offset)
	}
}

func TestUIScrollRequestsRenderOnlyWhenPositionChanges(t *testing.T) {
	world := ecs.NewWorld()
	state := input.NewState()
	state.MoveMouse(50, 50)
	ecs.InsertResource(world, state)
	request := &RenderRequest{}
	ecs.InsertResource(world, request)
	bundle := ecs.NewBundle(2)
	ecs.Put(&bundle, UITransformFromXY(0, 0))
	ecs.Put(&bundle, Size2D{Width: 100, Height: 100})
	ecs.Put(&bundle, Visibility{Visible: true})
	ecs.Put(&bundle, UIScroll{
		ViewportSize: mgl32.Vec2{100, 100},
		ContentSize:  mgl32.Vec2{100, 300},
		Vertical:     true,
		ConsumeWheel: true,
	})
	area := world.Spawn(bundle)
	system := UIScrollSystem("")
	for _, test := range []struct {
		name     string
		position float32
		momentum float32
		wheel    float64
		want     float32
	}{
		{name: "idle", position: -40, want: -40},
		{name: "momentum", position: -40, momentum: -5, want: -45},
		{name: "clamped momentum", position: -200, momentum: -5, want: -200},
		{name: "wheel", position: -40, wheel: -1, want: -50},
		{name: "clamped wheel", position: -200, wheel: -1, want: -200},
		{name: "content shrunk", position: -250, want: -200},
	} {
		t.Run(test.name, func(t *testing.T) {
			scroll := ecs.GetMut[UIScroll](world, area)
			scroll.Position[1], scroll.Momentum[1] = test.position, test.momentum
			state.ScrollMouse(0, test.wheel)
			if err := system.Run(world); err != nil {
				t.Fatal(err)
			}
			state.EndFrame()
			if scroll.Position.Y() != test.want {
				t.Fatalf("position=%v, want %v", scroll.Position.Y(), test.want)
			}
			if rendered := request.Take(); rendered != (test.position != test.want) {
				t.Fatalf("render requested=%v for position %v -> %v", rendered, test.position, test.want)
			}
		})
	}
	if allocations := testing.AllocsPerRun(50, func() {
		scroll := ecs.GetMut[UIScroll](world, area)
		scroll.Position[1], scroll.Momentum[1] = -40, -5
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
		request.Take()
	}); allocations != 0 {
		t.Fatalf("scroll render request allocations=%v", allocations)
	}
}

// The middle button pans a scroll container without the left button's press
// semantics, and releases without momentum, matching VS Code's editor pan.
func TestUIScrollMiddleDragPansContent(t *testing.T) {
	world := ecs.NewWorld()
	state := input.NewState()
	ecs.InsertResource(world, state)
	ecs.InsertResource(world, &RenderRequest{})
	bundle := ecs.NewBundle(5)
	ecs.Put(&bundle, UITransformFromXY(0, 0))
	ecs.Put(&bundle, UIGlobalTransform{Matrix: mgl32.Translate3D(0, 0, 0)})
	ecs.Put(&bundle, Visibility{Visible: true})
	ecs.Put(&bundle, Size2D{Width: 100, Height: 100})
	ecs.Put(&bundle, UIScroll{
		ViewportSize: mgl32.Vec2{100, 100},
		ContentSize:  mgl32.Vec2{100, 300},
		Vertical:     true,
		DisableDrag:  true,
	})
	area := world.Spawn(bundle)
	system := UIScrollSystem("")

	state.MoveMouse(50, 50)
	state.PressMouse(input.MouseMiddle, 50, 50)
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if state.Pointer().Down {
		t.Fatal("middle button must not read as a pointer press")
	}
	state.EndFrame()
	state.MoveMouse(50, 20)
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	scroll, _ := ecs.Get[UIScroll](world, area)
	if scroll.Position.Y() != -30 {
		t.Fatalf("middle drag position = %v, want -30", scroll.Position)
	}

	state.ReleaseMouse(input.MouseMiddle, 50, 20)
	for range 2 {
		state.EndFrame()
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	scroll, _ = ecs.Get[UIScroll](world, area)
	if scroll.Position.Y() != -30 {
		t.Fatalf("position after release = %v, want it held at -30 with no momentum", scroll.Position)
	}
}

func TestUIScrollConsumesWheelWhenHoveredWithoutOverflow(t *testing.T) {
	world := ecs.NewWorld()
	state := input.NewState()
	state.MoveMouse(20, 20)
	ecs.InsertResource(world, state)
	bundle := ecs.NewBundle(4)
	ecs.Put(&bundle, UITransformFromXY(0, 0))
	ecs.Put(&bundle, Visibility{Visible: true})
	ecs.Put(&bundle, UIScroll{
		ViewportSize: mgl32.Vec2{100, 100},
		ContentSize:  mgl32.Vec2{100, 100},
		Vertical:     true,
		ConsumeWheel: true,
	})
	world.Spawn(bundle)

	state.ScrollMouse(0, -1)
	if err := UIScrollSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if wheel := state.Pointer().WheelY; wheel != 0 {
		t.Fatalf("hovered non-scrollable container left wheel=%v, want consumed", wheel)
	}
}
