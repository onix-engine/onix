package scene

import (
	"math"
	"testing"
	"unsafe"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/go-gl/mathgl/mgl32"
)

type previousInputEventLayout struct {
	Seq         uint64
	Entity      ecs.Entity
	Path        []ecs.Entity
	Handled     bool
	Kind        string
	Device      input.PointerDevice
	Key         input.KeyCode
	Button      input.MouseButton
	X           float32
	Y           float32
	DeltaX      float32
	DeltaY      float32
	WheelX      float32
	WheelY      float32
	File        string
	Text        string
	Composition input.TextComposition
	Command     input.TextCommand
	Selection   input.TextSelection
}

func TestInputEventUsesCompactLayout(t *testing.T) {
	if got, previous := unsafe.Sizeof(InputEvent{}), unsafe.Sizeof(previousInputEventLayout{}); got >= previous {
		t.Fatalf("input event size = %d, previous layout = %d", got, previous)
	}
}

func spawnInputCameraForTest(t *testing.T, world *ecs.World, width, height int) ecs.Entity {
	t.Helper()
	entity := SpawnCamera2D(world)
	camera := world.GetMut[Camera](entity)
	camera.UpdateViewport(width, height)
	if err := SyncGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	return entity
}

func TestInputEventFromPlatform(t *testing.T) {
	composition := input.TextComposition{Text: "ni", Active: true}
	selection := input.TextSelection{Start: 1, End: 2}
	tests := []struct {
		input input.Event
		kind  InputEventKind
	}{
		{input: input.Event{Seq: 1, Kind: input.EventKeyDown, Key: input.KeyA}, kind: InputEventKeyDown},
		{input: input.Event{Seq: 2, Kind: input.EventKeyUp, Key: input.KeyA}, kind: InputEventKeyUp},
		{input: input.Event{Seq: 3, Kind: input.EventTextInput, Text: "你"}, kind: InputEventTextInput},
		{input: input.Event{Seq: 4, Kind: input.EventTextComposition, Composition: composition}, kind: InputEventTextComposition},
		{input: input.Event{Seq: 5, Kind: input.EventTextCommand, Command: input.TextCommandEnter}, kind: InputEventTextCommand},
		{input: input.Event{Seq: 6, Kind: input.EventTextSelection, Selection: selection}, kind: InputEventTextSelection},
		{input: input.Event{Seq: 7, Kind: input.EventFilesDropped, File: "/tmp/input.txt", X: 8, Y: 12}, kind: InputEventFilesDropped},
	}
	for _, test := range tests {
		event, ok := InputEventFromPlatform(test.input)
		if !ok || event.Kind != test.kind || event.Seq != test.input.Seq || event.Key != test.input.Key ||
			event.Text != test.input.Text || event.Composition != test.input.Composition || event.Command != test.input.Command ||
			event.Selection != test.input.Selection || event.File != test.input.File || event.X != float32(test.input.X) || event.Y != float32(test.input.Y) {
			t.Fatalf("platform input conversion = %#v, %v, want kind %q from %#v", event, ok, test.kind, test.input)
		}
	}
	for _, kind := range []input.EventKind{input.EventMouseDown, input.EventMouseUp, input.EventMouseMove, input.EventMouseWheel, input.EventTouchStart, input.EventTouchMove, input.EventTouchEnd} {
		if event, ok := InputEventFromPlatform(input.Event{Kind: kind}); ok {
			t.Fatalf("pointer platform event %d bypassed hit testing: %#v", kind, event)
		}
	}
}

func TestDispatchInputEvents2DSystemEmitsTapForPickTarget(t *testing.T) {
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
		t.Fatalf("sync transform: %v", err)
	}
	state, _ := world.GetResource[*input.State]()
	state.PressMouse(input.MouseLeft, 24, 36)
	if err := DispatchInputEvents2DSystem("").Run(world); err != nil {
		t.Fatalf("input down: %v", err)
	}
	pointerState, _ := world.Get[InputState2D](target)
	if !pointerState.Hovered || !pointerState.Pressed {
		t.Fatalf("ordinary sprite input state after press = %+v", pointerState)
	}
	capture, ok := world.GetResource[*InputCapture2D]()
	if !ok || capture == nil || capture.Target != target || capture.Receiver != target {
		t.Fatalf("unexpected input capture after press: %#v", capture)
	}
	state.EndFrame()
	state.ReleaseMouse(input.MouseLeft, 24, 36)
	if err := DispatchInputEvents2DSystem("").Run(world); err != nil {
		t.Fatalf("input up: %v", err)
	}

	var reader ecs.MessageReader[InputEvent]
	events := reader.Read(world)
	if len(events) != 4 {
		t.Fatalf("expected 4 input events, got %d: %#v", len(events), events)
	}
	if events[0].Kind != InputEventEnter || events[1].Kind != InputEventPress || events[2].Kind != InputEventRelease || events[3].Kind != InputEventTap {
		t.Fatalf("unexpected input event kinds: %#v", events)
	}
	if events[1].Device != input.PointerDeviceMouse || events[1].Button != input.MouseLeft {
		t.Fatalf("pointer input metadata = %#v", events[1])
	}
	if len(events[1].Path) != 0 {
		t.Fatalf("unexpected root input event path: %#v", events[1].Path)
	}
	pointerState, _ = world.Get[InputState2D](target)
	if !pointerState.Hovered || pointerState.Pressed {
		t.Fatalf("ordinary sprite input state after release = %+v", pointerState)
	}
	if capture.Target != 0 || capture.Receiver != 0 || capture.Camera != 0 {
		t.Fatalf("input capture was not cleared: %#v", capture)
	}
}

func TestDispatchInputCacheInvalidatesWhenTargetComponentIsRemoved(t *testing.T) {
	world := ecs.NewWorld()
	state := input.NewState()
	world.InsertResource(state)
	spawnInputCameraForTest(t, world, 200, 100)
	bundle := ecs.NewBundle(6)
	bundle.Put(UITransformFromXY(10, 20))
	bundle.Put(UIGlobalTransform{})
	bundle.Put(Visibility{Visible: true})
	bundle.Put(AnchorTopLeft)
	bundle.Put(InputTarget2D{Width: 100, Height: 40})
	bundle.Put(InputState2D{})
	target := world.Spawn(bundle)
	if err := SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}

	state.MoveMouse(24, 36)
	inputSystem := DispatchInputEvents2DSystem("")
	if err := inputSystem.Run(world); err != nil {
		t.Fatal(err)
	}
	if pointer, _ := world.Get[InputState2D](target); !pointer.Hovered {
		t.Fatal("target was not hovered before removal")
	}
	state.EndFrame()
	if _, ok := world.Remove[InputTarget2D](target); !ok {
		t.Fatal("input target was not removed")
	}
	if err := inputSystem.Run(world); err != nil {
		t.Fatal(err)
	}
	if pointer, _ := world.Get[InputState2D](target); pointer.Hovered {
		t.Fatal("removed input target retained hover state")
	}
	events := (&ecs.MessageReader[InputEvent]{}).Read(world)
	if len(events) != 2 || events[0].Kind != InputEventEnter || events[1].Kind != InputEventLeave {
		t.Fatalf("events after removing hovered target = %#v", events)
	}
}

func TestRemapInputEntity2DPreservesReleaseAndTap(t *testing.T) {
	world := ecs.NewWorld()
	world.InsertResource(input.NewState())
	spawnInputCameraForTest(t, world, 200, 100)
	spawnTarget := func() ecs.Entity {
		bundle := ecs.NewBundle(7)
		bundle.Put(UITransformFromXY(10, 20))
		bundle.Put(UIGlobalTransform{})
		bundle.Put(Visibility{Visible: true})
		bundle.Put(AnchorTopLeft)
		bundle.Put(Sprite2D{Width: 100, Height: 40, Alpha: 1})
		bundle.Put(InputTarget2D{Width: 100, Height: 40})
		bundle.Put(InputState2D{})
		return world.Spawn(bundle)
	}
	previous := spawnTarget()
	if err := SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	inputState, _ := world.GetResource[*input.State]()
	inputState.PressMouse(input.MouseLeft, 24, 36)
	if err := DispatchInputEvents2DSystem("").Run(world); err != nil {
		t.Fatal(err)
	}

	current := spawnTarget()
	world.Despawn(previous)
	if err := SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	RemapInputEntity2D(world, previous, current)
	state, _ := world.Get[InputState2D](current)
	if !state.Hovered || !state.Pressed {
		t.Fatalf("remapped input state = %+v", state)
	}
	capture, _ := world.GetResource[*InputCapture2D]()
	if capture.Target != current || capture.Receiver != current {
		t.Fatalf("remapped capture = %#v", capture)
	}

	inputState.EndFrame()
	inputState.ReleaseMouse(input.MouseLeft, 24, 36)
	if err := DispatchInputEvents2DSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	events := (&ecs.MessageReader[InputEvent]{}).Read(world)
	var released, tapped bool
	for _, event := range events {
		if event.Entity != current {
			continue
		}
		released = released || event.Kind == InputEventRelease
		tapped = tapped || event.Kind == InputEventTap
	}
	if !released || !tapped {
		t.Fatalf("remapped release/tap events = %#v", events)
	}
	state, _ = world.Get[InputState2D](current)
	if !state.Hovered || state.Pressed {
		t.Fatalf("remapped state after release = %+v", state)
	}
}

func TestInputEventPathFollowsParents(t *testing.T) {
	world := ecs.NewWorld()
	spawnParent := func(parent ecs.Entity) ecs.Entity {
		bundle := ecs.NewBundle(1)
		bundle.Put(Parent{Entity: parent})
		return world.Spawn(bundle)
	}
	root := spawnParent(0)
	child := spawnParent(root)
	grandchild := spawnParent(child)

	path := inputEventPath2D(world, grandchild)
	if len(path) != 3 || path[0] != grandchild || path[1] != child || path[2] != root {
		t.Fatalf("unexpected input path: %#v", path)
	}
}

func TestInputHandlerBubblesAndConsumes(t *testing.T) {
	world := ecs.NewWorld()
	visited := []ecs.Entity{}
	rootBundle := ecs.NewBundle(2)
	rootBundle.Put(Parent{})
	rootBundle.Put(InputHandler2D{Handle: func(context *InputEventContext) {
		visited = append(visited, context.Current)
		context.Consume()
	}})
	root := world.Spawn(rootBundle)
	childBundle := ecs.NewBundle(3)
	childBundle.Put(Parent{Entity: root})
	childBundle.Put(InputTarget2D{InputFilter: InputFilterPass})
	childBundle.Put(InputHandler2D{Handle: func(context *InputEventContext) {
		visited = append(visited, context.Current)
	}})
	child := world.Spawn(childBundle)

	DispatchInputEvent(world, InputEvent{
		Entity: child,
		Path:   inputEventPath2D(world, child),
		Kind:   InputEventTap,
	})
	if len(visited) != 2 || visited[0] != child || visited[1] != root {
		t.Fatalf("unexpected bubble order: %#v", visited)
	}
	events := (&ecs.MessageReader[InputEvent]{}).Read(world)
	if len(events) != 1 || !events[0].Handled {
		t.Fatalf("consumed input event was not marked handled: %#v", events)
	}
}

func TestInputHandlerHonorsInheritedActivity(t *testing.T) {
	w := ecs.NewWorld()
	parentBundle := ecs.NewBundle(1)
	parentBundle.Put(Activity{Active: false})
	parent := w.Spawn(parentBundle)
	calls := 0
	b := ecs.NewBundle(2)
	b.Put(Parent{Entity: parent})
	b.Put(InputHandler2D{Handle: func(*InputEventContext) { calls++ }})
	child := w.Spawn(b)
	DispatchInputEvent(w, InputEvent{Entity: child, Kind: InputEventTap})
	if calls != 0 {
		t.Fatal("inactive descendant received synchronous input")
	}
	w.GetMut[Activity](parent).Active = true
	DispatchInputEvent(w, InputEvent{Entity: child, Kind: InputEventTap})
	if calls != 1 {
		t.Fatal("resumed descendant lost synchronous input")
	}
}

func TestInputEventArenaPreservesNestedEventContext(t *testing.T) {
	world := ecs.NewWorld()
	var nested bool
	var outerKind InputEventKind
	rootBundle := ecs.NewBundle(2)
	rootBundle.Put(InputHandler2D{Handle: func(context *InputEventContext) {
		if nested {
			return
		}
		nested = true
		outerKind = context.Event.Kind
		DispatchInputEvent(world, InputEvent{Entity: context.Current, Kind: InputEventMove})
		if context.Event.Kind != outerKind {
			t.Fatalf("nested input changed outer event from %q to %q", outerKind, context.Event.Kind)
		}
	}})
	root := world.Spawn(rootBundle)
	childBundle := ecs.NewBundle(1)
	childBundle.Put(Parent{Entity: root})
	child := world.Spawn(childBundle)

	DispatchInputEvent(world, InputEvent{
		Entity: child,
		Path:   inputEventPath2D(world, child),
		Kind:   InputEventTap,
	})
	if outerKind != InputEventTap {
		t.Fatalf("outer input kind = %q, want %q", outerKind, InputEventTap)
	}
	events := (&ecs.MessageReader[InputEvent]{}).Read(world)
	if len(events) != 2 || events[0].Kind != InputEventMove || events[1].Kind != InputEventTap {
		t.Fatalf("unexpected nested input messages: %#v", events)
	}
}

func TestInputEventArenaAvoidsStableFrameAllocations(t *testing.T) {
	world := ecs.NewWorld()
	DispatchInputEvent(world, InputEvent{Kind: InputEventMove})
	world.AdvanceMessages()
	allocations := testing.AllocsPerRun(100, func() {
		DispatchInputEvent(world, InputEvent{Kind: InputEventMove})
		world.AdvanceMessages()
	})
	if allocations != 0 {
		t.Fatalf("stable input event dispatch allocated %.0f objects, want 0", allocations)
	}
}

func TestInputBlockerPreventsEventsUnlessConfiguredToPassThrough(t *testing.T) {
	for _, passThrough := range []bool{false, true} {
		world := ecs.NewWorld()
		world.InsertResource(input.NewState())
		spawnInputCameraForTest(t, world, 200, 100)
		target := ecs.NewBundle(6)
		target.Put(NewUITransform())
		target.Put(UIGlobalTransform{})
		target.Put(Visibility{Visible: true})
		target.Put(AnchorTopLeft)
		target.Put(InputTarget2D{Width: 100, Height: 40})
		target.Put(UIStackIndex(0))
		world.Spawn(target)
		blocker := ecs.NewBundle(6)
		blocker.Put(NewUITransform())
		blocker.Put(UIGlobalTransform{})
		blocker.Put(Visibility{Visible: true})
		blocker.Put(AnchorTopLeft)
		blocker.Put(InputBlocker2D{Width: 100, Height: 40, PassThrough: passThrough})
		blocker.Put(UIStackIndex(1))
		world.Spawn(blocker)
		if err := SyncUIGlobalTransformSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		state, _ := world.GetResource[*input.State]()
		state.PressMouse(input.MouseLeft, 10, 10)
		if err := DispatchInputEvents2DSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		events := (&ecs.MessageReader[InputEvent]{}).Read(world)
		if passThrough && (len(events) != 2 || events[0].Kind != InputEventEnter || events[1].Kind != InputEventPress) {
			t.Fatalf("pass-through blocker should expose target, got %#v", events)
		}
		if !passThrough && len(events) != 0 {
			t.Fatalf("blocking surface leaked input events: %#v", events)
		}
	}
}

func TestInputFilterIgnoreSkipsTarget(t *testing.T) {
	world := ecs.NewWorld()
	world.InsertResource(input.NewState())
	spawnInputCameraForTest(t, world, 200, 100)
	bundle := ecs.NewBundle(6)
	bundle.Put(UITransformFromXY(0, 0))
	bundle.Put(UIGlobalTransform{})
	bundle.Put(Visibility{Visible: true})
	bundle.Put(AnchorTopLeft)
	bundle.Put(InputTarget2D{Width: 100, Height: 40, InputFilter: InputFilterIgnore})
	bundle.Put(InputState2D{})
	world.Spawn(bundle)
	if err := SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	state, _ := world.GetResource[*input.State]()
	state.PressMouse(input.MouseLeft, 10, 10)
	if err := DispatchInputEvents2DSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if events := (&ecs.MessageReader[InputEvent]{}).Read(world); len(events) != 0 {
		t.Fatalf("ignored target received input events: %#v", events)
	}
}

func TestWorldInputCoordinatesRejectOutsideCameraViewport(t *testing.T) {
	world := ecs.NewWorld()
	entity := SpawnCamera2D(world)
	camera := world.GetMut[Camera](entity)
	camera.UpdateViewport(800, 600)
	camera.Viewport = CameraViewport{X: 100, Y: 50, Width: 400, Height: 300}
	if err := SyncGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	cameras, _ := collectInputCameras2D(world, nil)
	if len(cameras) != 1 {
		t.Fatalf("input cameras = %d, want 1", len(cameras))
	}
	if _, _, ok := inputCoordsForCamera2D(cameras[0], false, 50, 100); ok {
		t.Fatal("world pointer outside the camera viewport was accepted")
	}
	if _, _, ok := inputCoordsForCamera2D(cameras[0], false, 200, 100); !ok {
		t.Fatal("world pointer inside the camera viewport was rejected")
	}
}

func TestInputTargetContainsUsesLocalQuadInsteadOfWorldAABB(t *testing.T) {
	target := inputPickTarget2D{
		anchor: AnchorTopLeft,
		width:  100,
		height: 20,
		ui:     true,
	}
	matrix := mgl32.HomogRotate3DZ(float32(math.Pi / 4))
	target.prepare(matrix)
	model := quadBaseTransform(matrix, target.anchor, target.width, target.height, target.ui).
		Mul4(mgl32.Scale3D(target.width, target.height, 1))
	inside := model.Mul4x1(mgl32.Vec4{0.5, 0.5, 0, 1})
	if !inputTargetContains2D(&target, inside.X(), inside.Y()) {
		t.Fatal("rotated target rejected its center")
	}
	corners := [...]mgl32.Vec4{
		model.Mul4x1(mgl32.Vec4{0, 0, 0, 1}),
		model.Mul4x1(mgl32.Vec4{1, 0, 0, 1}),
		model.Mul4x1(mgl32.Vec4{0, 1, 0, 1}),
		model.Mul4x1(mgl32.Vec4{1, 1, 0, 1}),
	}
	minX, maxY := corners[0].X(), corners[0].Y()
	for _, corner := range corners[1:] {
		minX = min(minX, corner.X())
		maxY = max(maxY, corner.Y())
	}
	if inputTargetContains2D(&target, minX+1, maxY-1) {
		t.Fatal("rotated target accepted a point only contained by its world AABB")
	}
}

func TestInputPickingUsesCameraViewportAndRenderLayers(t *testing.T) {
	world := ecs.NewWorld()
	world.InsertResource(input.NewState())
	spawnCamera := func(viewport CameraViewport, layer uint) ecs.Entity {
		entity := SpawnCamera2D(world)
		camera := world.GetMut[Camera](entity)
		camera.UpdateViewport(200, 100)
		camera.Viewport = viewport
		if err := world.Insert(entity, RenderLayer(layer)); err != nil {
			t.Fatal(err)
		}
		return entity
	}
	spawnCamera(CameraViewport{Width: 100, Height: 100}, 0)
	spawnCamera(CameraViewport{X: 100, Width: 100, Height: 100}, 1)
	if err := SyncGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}

	bundle := ecs.NewBundle(5)
	bundle.Put(InputTarget2D{Width: 20, Height: 20})
	bundle.Put(GlobalTransform{Matrix: mgl32.Ident4()})
	bundle.Put(Visibility{Visible: true})
	bundle.Put(AnchorCenter)
	bundle.Put(RenderLayer(1))
	target := world.Spawn(bundle)
	state, _ := world.GetResource[*input.State]()
	state.PressMouse(input.MouseLeft, 150, 50)
	if err := DispatchInputEvents2DSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	events := (&ecs.MessageReader[InputEvent]{}).Read(world)
	if len(events) != 2 || events[0].Entity != target || events[0].Kind != InputEventEnter || events[1].Entity != target || events[1].Kind != InputEventPress {
		t.Fatalf("secondary camera target was not picked: %+v", events)
	}
}

func BenchmarkCollectInputTargets2D10000(b *testing.B) {
	world := ecs.NewWorld()
	spawner := world.NewSpawner3[InputTarget2D, GlobalTransform, Visibility]()
	for range 10000 {
		spawner.Spawn(
			InputTarget2D{Width: 10, Height: 10},
			GlobalTransform{},
			Visibility{Visible: true},
		)
	}
	collectInputTargets2D(world, nil)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		benchmarkTotal += float32(len(collectInputTargets2D(world, nil)))
	}
}

func TestDispatchInputUsesPressPositionBeforeSameFrameMove(t *testing.T) {
	world := ecs.NewWorld()
	state := input.NewState()
	world.InsertResource(state)
	spawnInputCameraForTest(t, world, 200, 100)
	var first ecs.Entity
	for _, x := range []float32{10, 100} {
		bundle := ecs.NewBundle(7)
		bundle.Put(UITransformFromXY(x, 20))
		bundle.Put(UIGlobalTransform{})
		bundle.Put(Visibility{Visible: true})
		bundle.Put(AnchorTopLeft)
		bundle.Put(Sprite2D{Width: 80, Height: 40, Alpha: 1})
		bundle.Put(InputTarget2D{})
		bundle.Put(InputState2D{})
		entity := world.Spawn(bundle)
		if first == 0 {
			first = entity
		}
	}
	if err := SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	state.PressMouse(input.MouseLeft, 24, 36)
	state.MoveMouse(124, 36)
	if err := DispatchInputEvents2DSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	var reader ecs.MessageReader[InputEvent]
	pressed, moved := false, false
	for _, event := range reader.Read(world) {
		switch event.Kind {
		case InputEventPress:
			pressed = event.Entity == first && event.X == 24 && event.Y == 36
		case InputEventMove:
			moved = event.Entity == first && event.X == 124 && event.Y == 36
		}
	}
	if !pressed || !moved {
		t.Fatalf("same-frame press/move lost original capture: press=%v move=%v", pressed, moved)
	}
}
