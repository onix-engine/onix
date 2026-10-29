package scene

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/go-gl/mathgl/mgl32"
)

// Both readers process the same payloads; setup is outside the timed region.
func BenchmarkMessageReaderInputEvents(b *testing.B) {
	for _, borrow := range []bool{false, true} {
		name := "Read"
		if borrow {
			name = "All"
		}
		b.Run(name, func(b *testing.B) {
			world := ecs.NewWorld()
			for i := range 100 {
				world.WriteMessage(InputEvent{X: float32(i), Kind: InputEventMove})
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				var reader ecs.MessageReader[InputEvent]
				if borrow {
					for event := range reader.All(world) {
						benchmarkTotal += event.X
					}
				} else {
					for _, event := range reader.Read(world) {
						benchmarkTotal += event.X
					}
				}
			}
		})
	}
}

func BenchmarkDispatchInputSamePosition1000(b *testing.B) {
	world := ecs.NewWorld()
	state := input.NewState()
	world.InsertResource(state)
	camera := SpawnCamera2D(world)
	component := world.GetMut[Camera](camera)
	component.UpdateViewport(200, 100)
	if err := SyncGlobalTransformSystem("").Run(world); err != nil {
		b.Fatal(err)
	}
	spawner := world.NewSpawner3[InputTarget2D, UIGlobalTransform, Visibility]()
	for range 1000 {
		spawner.Spawn(InputTarget2D{Width: 10, Height: 10}, UIGlobalTransform{Matrix: mgl32.Ident4()}, Visibility{Visible: true})
	}
	context := &ecs.SystemContext{World: world}
	step := func() {
		state.ReleaseMouse(input.MouseLeft, 100, 50)
		state.EndFrame()
		state.PressMouse(input.MouseLeft, 100, 50)
		if err := dispatchInputEvents2D(context); err != nil {
			b.Fatal(err)
		}
		world.AdvanceMessages()
	}
	step()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		step()
	}
}

func BenchmarkDispatchInputDrag1000(b *testing.B) {
	world := ecs.NewWorld()
	state := input.NewState()
	world.InsertResource(state)
	camera := SpawnCamera2D(world)
	component := world.GetMut[Camera](camera)
	component.UpdateViewport(200, 100)
	if err := SyncGlobalTransformSystem("").Run(world); err != nil {
		b.Fatal(err)
	}
	spawner := world.NewSpawner3[InputTarget2D, UIGlobalTransform, Visibility]()
	for range 1000 {
		spawner.Spawn(InputTarget2D{Width: 20, Height: 20}, UIGlobalTransform{Matrix: mgl32.Ident4()}, Visibility{Visible: true})
	}
	context := &ecs.SystemContext{World: world}
	state.PressMouse(input.MouseLeft, 1, 1)
	if err := dispatchInputEvents2D(context); err != nil {
		b.Fatal(err)
	}
	tracker, _ := world.GetResource[*inputTracker2D]()
	if !tracker.hasActive {
		b.Fatal("pointer was not captured")
	}
	x := 1.0
	step := func() {
		state.EndFrame()
		x = 3 - x
		state.MoveMouse(x, 1)
		if err := dispatchInputEvents2D(context); err != nil {
			b.Fatal(err)
		}
		world.AdvanceMessages()
	}
	step()
	step()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		step()
	}
}

// A normal function lets the compiler optimize iteration as in a system body.
func BenchmarkMessageReaderInputEventsSystem(b *testing.B) {
	world := ecs.NewWorld()
	for i := range 100 {
		world.WriteMessage(InputEvent{X: float32(i), Kind: InputEventMove})
	}
	b.ReportAllocs()
	for b.Loop() {
		benchmarkTotal += sumBenchmarkInputMessages(world)
	}
}

func sumBenchmarkInputMessages(world *ecs.World) float32 {
	var reader ecs.MessageReader[InputEvent]
	var total float32
	for event := range reader.All(world) {
		total += event.X
	}
	return total
}
