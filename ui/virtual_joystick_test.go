package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestVirtualJoystickTracksPointerAndResets(t *testing.T) {
	world := newUIWorld(t)
	entity := SpawnVirtualJoystick(world, VirtualJoystickOptions{
		X: 100, Y: 100, Size: 120, KnobSize: 40, DeadZone: .2,
	})
	if err := runUIFrame(world); err != nil {
		t.Fatalf("sync joystick: %v", err)
	}

	joystick, ok := world.Get[VirtualJoystick](entity)
	if !ok || joystick.Track == 0 || joystick.Knob == 0 {
		t.Fatalf("generated joystick is incomplete: %#v", joystick)
	}
	track, trackOK := world.Get[scene.UIBackground2D](joystick.Track)
	knob, knobOK := world.Get[scene.UIBackground2D](joystick.Knob)
	if !trackOK || !knobOK || track.Shape.Radius != 60 || knob.Shape.Radius != 20 {
		t.Fatalf("circle visuals = track %#v knob %#v", track, knob)
	}

	var changes ecs.MessageReader[ChangedEvent[mgl32.Vec2]]
	var activeEvents ecs.MessageReader[VirtualJoystickActiveEvent]
	state, _ := world.GetResource[*input.State]()
	state.PressMouse(input.MouseLeft, 220, 160)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("press joystick: %v", err)
	}
	joystick, _ = world.Get[VirtualJoystick](entity)
	if !joystick.Active || joystick.RawValue != (mgl32.Vec2{1, 0}) || joystick.Value != (mgl32.Vec2{1, 0}) {
		t.Fatalf("pressed joystick = %#v", joystick)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("sync pressed joystick: %v", err)
	}
	knobTransform, _ := world.Get[scene.UITransform](joystick.Knob)
	if knobTransform.Translation.X.Value != 80 || knobTransform.Translation.Y.Value != 40 {
		t.Fatalf("knob position after press = %#v", knobTransform.Translation)
	}
	if got := changes.Read(world); len(got) != 1 || got[0].Entity != entity || got[0].Value != (mgl32.Vec2{1, 0}) {
		t.Fatalf("changed events after press = %#v", got)
	}
	if got := activeEvents.Read(world); len(got) != 1 || got[0] != (VirtualJoystickActiveEvent{Entity: entity, Active: true}) {
		t.Fatalf("active events after press = %#v", got)
	}

	state.EndFrame()
	state.ReleaseMouse(input.MouseLeft, 220, 160)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("release joystick: %v", err)
	}
	joystick, _ = world.Get[VirtualJoystick](entity)
	if joystick.Active || joystick.RawValue != (mgl32.Vec2{}) || joystick.Value != (mgl32.Vec2{}) {
		t.Fatalf("released joystick = %#v", joystick)
	}
	if got := changes.Read(world); len(got) != 1 || got[0].Value != (mgl32.Vec2{}) {
		t.Fatalf("changed events after release = %#v", got)
	}
	if got := activeEvents.Read(world); len(got) != 1 || got[0] != (VirtualJoystickActiveEvent{Entity: entity, Active: false}) {
		t.Fatalf("active events after release = %#v", got)
	}
}

func TestVirtualJoystickAppliesAxisAndDeadZone(t *testing.T) {
	world := newUIWorld(t)
	entity := SpawnVirtualJoystick(world, VirtualJoystickOptions{
		Size: 100, DeadZone: .25, Axis: VirtualJoystickHorizontal,
	})
	if err := runUIFrame(world); err != nil {
		t.Fatalf("sync joystick: %v", err)
	}
	state, _ := world.GetResource[*input.State]()
	state.PressMouse(input.MouseLeft, 60, 75)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("press joystick: %v", err)
	}
	joystick, _ := world.Get[VirtualJoystick](entity)
	if joystick.RawValue != (mgl32.Vec2{.2, 0}) || joystick.Value != (mgl32.Vec2{}) {
		t.Fatalf("dead-zone joystick = %#v", joystick)
	}

	state.EndFrame()
	state.MoveMouse(160, 50)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("move joystick: %v", err)
	}
	joystick, _ = world.Get[VirtualJoystick](entity)
	if joystick.RawValue != (mgl32.Vec2{1, 0}) || joystick.Value != (mgl32.Vec2{1, 0}) {
		t.Fatalf("axis-constrained joystick = %#v", joystick)
	}
}

func TestSetVirtualJoystickRawValueClampsAndPublishes(t *testing.T) {
	world := newUIWorld(t)
	entity := SpawnVirtualJoystick(world, VirtualJoystickOptions{Size: 100, DeadZone: .25})
	var changes ecs.MessageReader[ChangedEvent[mgl32.Vec2]]
	if !SetVirtualJoystickRawValue(world, entity, mgl32.Vec2{3, 4}) {
		t.Fatal("set raw value returned false")
	}
	joystick, _ := world.Get[VirtualJoystick](entity)
	if joystick.RawValue != (mgl32.Vec2{.6, .8}) || joystick.Value != (mgl32.Vec2{.6, .8}) {
		t.Fatalf("clamped joystick = %#v", joystick)
	}
	if got := changes.Read(world); len(got) != 1 || got[0].Entity != entity || got[0].Value != joystick.Value {
		t.Fatalf("changed events = %#v", got)
	}
	if !ResetVirtualJoystick(world, entity) {
		t.Fatal("reset returned false")
	}
	joystick, _ = world.Get[VirtualJoystick](entity)
	if joystick.Value != (mgl32.Vec2{}) || joystick.RawValue != (mgl32.Vec2{}) {
		t.Fatalf("reset joystick = %#v", joystick)
	}
}

func TestBindVirtualJoystickUsesAuthoredRoot(t *testing.T) {
	world := newUIWorld(t)
	entity := spawnAnchor(world, 30, 40)
	track := spawnAuthoredVirtualJoystickPart(world, entity, 0, 0, 100, 100)
	knob := spawnAuthoredVirtualJoystickPart(world, entity, 30, 30, 40, 40)
	if err := BindVirtualJoystick(world, entity, ControlOptions{Width: 100, Height: 100}, VirtualJoystick{
		Axis: VirtualJoystickVertical, Track: track, Knob: knob,
	}); err != nil {
		t.Fatalf("bind virtual joystick: %v", err)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("sync bound joystick: %v", err)
	}
	state, _ := world.GetResource[*input.State]()
	state.PressMouse(input.MouseLeft, 100, 140)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("press bound joystick: %v", err)
	}
	joystick, _ := world.Get[VirtualJoystick](entity)
	if !joystick.Active || joystick.RawValue != (mgl32.Vec2{0, 1}) || joystick.Value != (mgl32.Vec2{0, 1}) {
		t.Fatalf("bound joystick = %#v", joystick)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("sync authored joystick: %v", err)
	}
	knobTransform, _ := world.Get[scene.UITransform](knob)
	if knobTransform.Translation.X.Value != 30 || knobTransform.Translation.Y.Value != 60 {
		t.Fatalf("authored knob position = %#v", knobTransform.Translation)
	}
}

func TestFloatingVirtualJoystickShowsAtPressPosition(t *testing.T) {
	world := newUIWorld(t)
	entity := spawnAnchor(world, 0, 0)
	track := spawnAuthoredVirtualJoystickPart(world, entity, 0, 0, 120, 120)
	knob := spawnAuthoredVirtualJoystickPart(world, entity, 40, 40, 40, 40)
	if err := BindVirtualJoystick(world, entity, ControlOptions{Width: 400, Height: 300}, VirtualJoystick{
		DeadZone: .2, Floating: true, Track: track, Knob: knob,
	}); err != nil {
		t.Fatalf("bind floating joystick: %v", err)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("sync floating joystick: %v", err)
	}
	for _, part := range [...]ecs.Entity{track, knob} {
		visibility, _ := world.Get[scene.Visibility](part)
		if visibility.Visible {
			t.Fatalf("floating joystick part %d is visible before press", part)
		}
	}

	state, _ := world.GetResource[*input.State]()
	state.PressMouse(input.MouseLeft, 200, 150)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("press floating joystick: %v", err)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("sync pressed floating joystick: %v", err)
	}
	joystick, _ := world.Get[VirtualJoystick](entity)
	if !joystick.Active || joystick.RawValue != (mgl32.Vec2{}) {
		t.Fatalf("pressed floating joystick = %#v", joystick)
	}
	trackTransform, _ := world.Get[scene.UITransform](track)
	knobTransform, _ := world.Get[scene.UITransform](knob)
	if trackTransform.Translation.X.Value != 140 || trackTransform.Translation.Y.Value != 90 {
		t.Fatalf("floating track position = %#v", trackTransform.Translation)
	}
	if knobTransform.Translation.X.Value != 180 || knobTransform.Translation.Y.Value != 130 {
		t.Fatalf("floating knob position = %#v", knobTransform.Translation)
	}

	state.EndFrame()
	state.MoveMouse(260, 150)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("move floating joystick: %v", err)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("sync moved floating joystick: %v", err)
	}
	joystick, _ = world.Get[VirtualJoystick](entity)
	if joystick.RawValue != (mgl32.Vec2{1, 0}) || joystick.Value != (mgl32.Vec2{1, 0}) {
		t.Fatalf("moved floating joystick = %#v", joystick)
	}
	knobTransform, _ = world.Get[scene.UITransform](knob)
	if knobTransform.Translation.X.Value != 220 || knobTransform.Translation.Y.Value != 130 {
		t.Fatalf("moved floating knob position = %#v", knobTransform.Translation)
	}

	state.EndFrame()
	state.ReleaseMouse(input.MouseLeft, 260, 150)
	if err := runUIUpdate(world); err != nil {
		t.Fatalf("release floating joystick: %v", err)
	}
	joystick, _ = world.Get[VirtualJoystick](entity)
	if joystick.Active || joystick.Value != (mgl32.Vec2{}) {
		t.Fatalf("released floating joystick = %#v", joystick)
	}
	for _, part := range [...]ecs.Entity{track, knob} {
		visibility, _ := world.Get[scene.Visibility](part)
		if visibility.Visible {
			t.Fatalf("floating joystick part %d is visible after release", part)
		}
	}
}

func TestFloatingVirtualJoystickMoveDoesNotAllocateAfterWarmup(t *testing.T) {
	world := newUIWorld(t)
	entity := spawnAnchor(world, 0, 0)
	track := spawnAuthoredVirtualJoystickPart(world, entity, 0, 0, 120, 120)
	knob := spawnAuthoredVirtualJoystickPart(world, entity, 40, 40, 40, 40)
	if err := BindVirtualJoystick(world, entity, ControlOptions{Width: 400, Height: 300}, VirtualJoystick{
		Floating: true, Track: track, Knob: knob,
	}); err != nil {
		t.Fatalf("bind floating joystick: %v", err)
	}
	if err := runUIFrame(world); err != nil {
		t.Fatalf("sync floating joystick: %v", err)
	}
	updateVirtualJoystickInput(world, &scene.InputEvent{Entity: entity, Kind: scene.InputEventPress, X: 200, Y: 150})
	world.AdvanceMessages()

	x := float32(260)
	allocations := testing.AllocsPerRun(100, func() {
		x = 400 - x
		updateVirtualJoystickInput(world, &scene.InputEvent{Entity: entity, Kind: scene.InputEventMove, X: x, Y: 150})
		world.AdvanceMessages()
	})
	if allocations != 0 {
		t.Fatalf("floating joystick move allocated %.0f objects, want 0", allocations)
	}
}

func spawnAuthoredVirtualJoystickPart(world *ecs.World, parent ecs.Entity, x, y, width, height float32) ecs.Entity {
	transform := scene.UITransformFromXY(x, y)
	bundle := ecs.NewBundle(6)
	bundle.Put(transform)
	bundle.Put(scene.UIGlobalTransform{})
	bundle.Put(scene.Visibility{Visible: true})
	bundle.Put(scene.AnchorTopLeft)
	bundle.Put(scene.Parent{Entity: parent})
	bundle.Put(scene.Sprite2D{Width: width, Height: height, Alpha: 1})
	return world.Spawn(bundle)
}

func TestVirtualJoystickSetDoesNotAllocateAfterWarmup(t *testing.T) {
	world := newUIWorld(t)
	entity := SpawnVirtualJoystick(world, VirtualJoystickOptions{Size: 100})
	SetVirtualJoystickRawValue(world, entity, mgl32.Vec2{1, 0})
	world.AdvanceMessages()
	SetVirtualJoystickRawValue(world, entity, mgl32.Vec2{-1, 0})
	world.AdvanceMessages()

	direction := float32(1)
	allocations := testing.AllocsPerRun(100, func() {
		direction = -direction
		SetVirtualJoystickRawValue(world, entity, mgl32.Vec2{direction, 0})
		world.AdvanceMessages()
	})
	if allocations != 0 {
		t.Fatalf("virtual joystick update allocated %.0f objects, want 0", allocations)
	}
}

func BenchmarkSetVirtualJoystickRawValue(b *testing.B) {
	world := ecs.NewWorld()
	entity := SpawnVirtualJoystick(world, VirtualJoystickOptions{Size: 100})
	SetVirtualJoystickRawValue(world, entity, mgl32.Vec2{1, 0})
	world.AdvanceMessages()
	SetVirtualJoystickRawValue(world, entity, mgl32.Vec2{-1, 0})
	world.AdvanceMessages()

	direction := float32(1)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		direction = -direction
		SetVirtualJoystickRawValue(world, entity, mgl32.Vec2{direction, 0})
		world.AdvanceMessages()
	}
}
