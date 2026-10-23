package onix

import (
	"sync"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/ui"
)

func TestIdleSkipsSchedulesAndWakesForChangesInputAndDeadline(t *testing.T) {
	app := NewApp()
	app.SetRenderMode(RenderOnDemand)
	app.SetIdleEnabled(true)
	app.SetTargetFPS(0)
	now := time.Unix(10, 0)
	app.now = func() time.Time { return now }
	updates := 0
	if err := app.AddSystem(StageUpdate, ecs.SystemConfig{Name: "update", System: func(*ecs.SystemContext) error { updates++; return nil }}); err != nil {
		t.Fatal(err)
	}
	step := func(want bool) {
		t.Helper()
		rendered, err := app.Step()
		if err != nil || rendered != want {
			t.Fatalf("rendered=%v want=%v err=%v", rendered, want, err)
		}
	}
	step(true)
	step(false)
	if updates != 1 {
		t.Fatalf("idle ran updates: %d", updates)
	}
	spawner := ecs.NewSpawner1[scene.Transform](app.World)
	entity := spawner.Spawn(scene.NewTransform())
	step(true)
	pose := ecs.GetMut[scene.Transform](app.World, entity)
	pose.Translation[0]++
	step(true)
	state, _ := ecs.GetResource[*input.State](app.World)
	state.Enqueue(input.Event{Kind: input.EventKeyDown, Key: input.KeyA})
	step(true)
	if !state.KeyHeld(input.KeyA) {
		t.Fatal("wake lost input")
	}
	ecs.InsertResource(app.World, 42)
	step(true)
	ecs.WriteMessage(app.World, "wake")
	step(true)
	deadline := now.Add(time.Second)
	app.WakeAt(deadline)
	app.WakeAt(deadline.Add(time.Second))
	if got, idle := app.idleDeadline(); !idle || got != deadline {
		t.Fatalf("idle=%v deadline=%v", idle, got)
	}
	step(false)
	now = deadline
	step(true)
	step(false)
	if _, idle := app.idleDeadline(); !idle {
		t.Fatal("deadline kept scheduler active")
	}
	if allocations := testing.AllocsPerRun(100, func() { step(false) }); allocations != 0 {
		t.Fatalf("idle allocations=%v", allocations)
	}
}

func TestIdlePreservesContinuousSimulation(t *testing.T) {
	app := NewApp()
	app.SetRenderMode(RenderOnDemand)
	app.SetIdleEnabled(true)
	if err := InstallSystems[Physics2D](app); err != nil {
		t.Fatal(err)
	}
	if app.idleEligible() {
		t.Fatal("physics app allowed suspension")
	}
}

func TestIdleWakesOnlyAtCaretBlinkDeadline(t *testing.T) {
	app := NewApp()
	defer app.Destroy()
	app.SetRenderMode(RenderOnDemand)
	app.SetIdleEnabled(true)
	app.SetTargetFPS(0)
	now := time.Unix(10, 0)
	app.now = func() time.Time { return now }
	if err := InstallSystems[UIWidgets](app); err != nil {
		t.Fatal(err)
	}
	field := ui.SpawnInput(app.World, ui.TextOptions{Width: 160, Height: 36, Value: "stable"})
	ui.FocusControl(app.World, field)
	step := func(want bool) {
		t.Helper()
		rendered, err := app.Step()
		if err != nil || rendered != want {
			t.Fatalf("rendered=%v want=%v err=%v", rendered, want, err)
		}
	}
	step(true)
	deadline, idle := app.idleDeadline()
	if !idle || deadline != now.Add(500*time.Millisecond) {
		t.Fatalf("idle=%v deadline=%v", idle, deadline)
	}
	now = now.Add(499 * time.Millisecond)
	step(false)
	now = now.Add(time.Millisecond)
	step(true)
	step(false)
	if deadline, idle := app.idleDeadline(); !idle || deadline != now.Add(500*time.Millisecond) {
		t.Fatalf("next blink: idle=%v deadline=%v", idle, deadline)
	}
	// User deadlines take priority and do not erase the pending caret phase.
	app.WakeAt(now.Add(50 * time.Millisecond))
	now = now.Add(50 * time.Millisecond)
	step(true)
	if deadline, idle := app.idleDeadline(); !idle || deadline != time.Unix(11, 0) {
		t.Fatalf("blink after user wake: idle=%v deadline=%v", idle, deadline)
	}
	ui.FocusControl(app.World, 0)
	step(true)
	if deadline, idle := app.idleDeadline(); !idle || !deadline.IsZero() {
		t.Fatalf("unfocused field still wakes: idle=%v deadline=%v", idle, deadline)
	}
}

func TestIdleRenderRequestsAreConcurrent(t *testing.T) {
	app := NewApp()
	app.ensureECSResources()
	request, _ := ecs.GetResource[*scene.RenderRequest](app.World)
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() {
			for range 100 {
				request.Request()
				app.RequestRender()
				app.WakeAt(time.Now().Add(time.Second))
			}
		})
	}
	group.Wait()
	if !request.Take() || !app.renderRequested.Load() {
		t.Fatal("concurrent request lost")
	}
}
