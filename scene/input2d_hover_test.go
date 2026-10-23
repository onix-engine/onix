package scene

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
)

func TestDispatchInputEvents2DEmitsHoverMoveWithoutCapture(t *testing.T) {
	world := ecs.NewWorld()
	ecs.InsertResource(world, input.NewState())
	spawnInputCameraForTest(t, world, 200, 100)
	bundle := ecs.NewBundle(7)
	ecs.Put(&bundle, UITransformFromXY(10, 20))
	ecs.Put(&bundle, UIGlobalTransform{})
	ecs.Put(&bundle, Visibility{Visible: true})
	ecs.Put(&bundle, AnchorTopLeft)
	ecs.Put(&bundle, Sprite2D{Width: 100, Height: 40, Alpha: 1})
	ecs.Put(&bundle, InputTarget2D{})
	ecs.Put(&bundle, InputState2D{})
	target := world.Spawn(bundle)
	if err := SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	state, _ := ecs.GetResource[*input.State](world)
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
