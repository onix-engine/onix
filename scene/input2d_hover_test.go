package scene

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/input"
)

func TestDispatchInputEvents2DHoversOnFirstMousePosition(t *testing.T) {
	world := ecs.NewWorld()
	state := input.NewState()
	world.InsertResource(state)
	spawnInputCameraForTest(t, world, 200, 100)
	spawner := world.NewSpawner6[UITransform, UIGlobalTransform, Visibility, InputTarget2D, InputState2D, Anchor]()
	target := spawner.Spawn(UITransformFromXY(10, 20), UIGlobalTransform{}, Visibility{Visible: true}, InputTarget2D{Width: 100, Height: 40}, InputState2D{}, AnchorTopLeft)
	if err := SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	dispatch := DispatchInputEvents2DSystem("")
	if err := dispatch.Run(world); err != nil {
		t.Fatal(err)
	}
	state.EndFrame()
	state.MoveMouse(24, 36)
	if pointer := state.Pointer(); pointer.DeltaX != 0 || pointer.DeltaY != 0 {
		t.Fatal("first mouse position must not invent a movement delta")
	}
	if err := dispatch.Run(world); err != nil {
		t.Fatal(err)
	}
	if pointer := world.GetRef[InputState2D](target); !pointer.Hovered {
		t.Fatal("first mouse position did not update hover with unchanged geometry")
	}
	state.EndFrame()
	if allocations := testing.AllocsPerRun(100, func() {
		if err := dispatch.Run(world); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("idle hover allocated %g times", allocations)
	}
}

func TestDispatchInputEvents2DEmitsHoverMoveWithoutCapture(t *testing.T) {
	world := ecs.NewWorld()
	world.InsertResource(input.NewState())
	spawnInputCameraForTest(t, world, 200, 100)
	bundle := ecs.NewBundle(7)
	bundle.Put(UITransformFromXY(10, 20))
	bundle.Put(UIGlobalTransform{})
	bundle.Put(Visibility{Visible: true})
	bundle.Put(AnchorTopLeft)
	bundle.Put(Sprite2D{Width: 100, Height: 40, Alpha: 1})
	bundle.Put(InputTarget2D{})
	bundle.Put(InputState2D{})
	target := world.Spawn(bundle)
	if err := SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	state, _ := world.GetResource[*input.State]()
	state.MoveMouse(0, 0)
	state.EndFrame()
	state.MoveMouse(24, 36)
	if err := DispatchInputEvents2DSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	events := (&ecs.MessageReader[InputEvent]{}).Read(world)
	if len(events) != 2 || events[0].Kind != InputEventEnter || events[1].Kind != InputEventMove || events[1].Entity != target {
		t.Fatalf("hover movement events = %#v", events)
	}
	if events[1].DeltaX == 0 && events[1].DeltaY == 0 {
		t.Fatal("hover movement lost pointer delta")
	}
}
