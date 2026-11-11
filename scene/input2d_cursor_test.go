package scene

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/input"
)

func TestPointerCursorFollowsHitAndOcclusion(t *testing.T) {
	world := ecs.NewWorld()
	state := input.NewState()
	world.InsertResource(state)
	spawnInputCameraForTest(t, world, 200, 100)
	spawner := world.NewSpawner6[UITransform, UIGlobalTransform, Visibility, InputTarget2D, InputState2D, Anchor]()
	editor := spawner.Spawn(UITransformFromXY(10, 20), UIGlobalTransform{}, Visibility{Visible: true}, InputTarget2D{Width: 100, Height: 40, Cursor: input.CursorText}, InputState2D{}, AnchorTopLeft)
	dispatch := DispatchInputEvents2DSystem("")
	step := func(want input.Cursor) {
		t.Helper()
		if err := SyncUIGlobalTransformSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		if err := dispatch.Run(world); err != nil {
			t.Fatal(err)
		}
		if got := state.Cursor(); got != want {
			t.Fatalf("cursor = %s, want %s", got, want)
		}
		state.EndFrame()
	}
	state.MoveMouse(24, 36)
	step(input.CursorText)
	// No movement or focus is needed when a cursor style changes.
	world.GetMut[InputTarget2D](editor).Cursor = input.CursorPointer
	step(input.CursorPointer)
	blockerSpawner := world.NewSpawner6[UITransform, UIGlobalTransform, Visibility, InputBlocker2D, UIStackIndex, Anchor]()
	overlay := blockerSpawner.Spawn(UITransformFromXY(10, 20), UIGlobalTransform{}, Visibility{Visible: true}, InputBlocker2D{Width: 100, Height: 40, Cursor: input.CursorEWResize}, UIStackIndex(100), AnchorTopLeft)
	step(input.CursorEWResize)
	if world.GetRef[InputState2D](editor).Hovered {
		t.Fatal("occluded editor retained hover")
	}
	world.GetMut[InputBlocker2D](overlay).PassThrough = true
	step(input.CursorPointer)
	world.GetMut[Visibility](editor).Visible = false
	step(input.CursorDefault)
	world.GetMut[Visibility](editor).Visible = true
	step(input.CursorPointer)
	state.MoveMouse(180, 80)
	step(input.CursorDefault)
	state.MoveMouse(24, 36)
	step(input.CursorPointer)
	// Rebuilding a retained control keeps the cursor correct before the next pick.
	replacement := spawner.Spawn(UITransformFromXY(10, 20), UIGlobalTransform{}, Visibility{Visible: true}, InputTarget2D{Width: 100, Height: 40, Cursor: input.CursorText}, InputState2D{}, AnchorTopLeft)
	RemapInputEntity2D(world, editor, replacement)
	world.Despawn(editor)
	SyncPointerCursor2D(world)
	if state.Cursor() != input.CursorText {
		t.Fatal("replacing a retained control lost its cursor")
	}
	step(input.CursorText)
	if allocations := testing.AllocsPerRun(100, func() {
		if err := dispatch.Run(world); err != nil {
			panic(err)
		}
		SyncPointerCursor2D(world)
	}); allocations != 0 {
		t.Fatalf("idle cursor allocated %g times", allocations)
	}
}
