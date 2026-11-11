package onix

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/input"
)

func BenchmarkInputHandlerPhases(b *testing.B) {
	app := NewApp()
	state := input.NewState()
	app.World.InsertResource(state)
	for phase := input.PhaseGlobal; phase <= input.PhaseUnhandled; phase++ {
		app.AddInputHandler(phase, func(*input.Dispatch) {})
	}
	context := &ecs.SystemContext{World: app.World}
	frame := func() {
		for range 32 {
			state.Enqueue(input.Event{Kind: input.EventMouseMove})
		}
		if err := app.processInputEvents(context); err != nil {
			b.Fatal(err)
		}
		state.EndFrame()
		app.World.AdvanceMessages()
	}
	frame()
	frame()
	b.ReportAllocs()
	for b.Loop() {
		frame()
	}
}
