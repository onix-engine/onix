package onix

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/physics"
	"github.com/diiyw/onix/resource"
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
	spawner := app.World.NewSpawner1[scene.Transform]()
	entity := spawner.Spawn(scene.NewTransform())
	step(true)
	pose := app.World.GetMut[scene.Transform](entity)
	pose.Translation[0]++
	step(true)
	state, _ := app.World.GetResource[*input.State]()
	state.Enqueue(input.Event{Kind: input.EventKeyDown, Key: input.KeyA})
	step(true)
	if !state.KeyHeld(input.KeyA) {
		t.Fatal("wake lost input")
	}
	app.World.InsertResource(42)
	step(true)
	app.World.WriteMessage("wake")
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
	defer app.Destroy()
	app.SetRenderMode(RenderOnDemand)
	app.SetIdleEnabled(true)
	if err := app.InstallSystems[Physics2D](); err != nil {
		t.Fatal(err)
	}
	if !app.idleEligible() {
		t.Fatal("empty physics feature prevented suspension")
	}
	s := app.World.NewSpawner2[physics.RigidBody2D, scene.Transform]()
	e := s.Spawn(physics.NewRigidBody2D(physics.BodyDynamic), scene.NewTransform())
	if app.idleEligible() {
		t.Fatal("active physics body allowed suspension")
	}
	if err := app.World.Insert(e, scene.Activity{}); err != nil {
		t.Fatal(err)
	}
	if !app.idleEligible() {
		t.Fatal("paused physics body prevented suspension")
	}
}

func TestIdleWithInstalledEffectsAndCompletedAnimation(t *testing.T) {
	app := NewApp()
	defer app.Destroy()
	app.SetRenderMode(RenderOnDemand)
	app.SetIdleEnabled(true)
	app.SetTargetFPS(0)
	for _, install := range []func(*App) error{
		(*App).InstallSystems[Animations2D], (*App).InstallSystems[SpriteTrails2D],
		(*App).InstallSystems[Particles2D], (*App).InstallSystems[Spine2D],
		(*App).InstallSystems[Physics2D], (*App).InstallSystems[Physics3D],
	} {
		if err := install(app); err != nil {
			t.Fatal(err)
		}
	}
	if !app.idleEligible() {
		t.Fatal("empty capabilities prevented idle")
	}
	spawner := app.World.NewSpawner1[scene.Animator]()
	entity := spawner.Spawn(scene.NewAnimator(&scene.AnimationClip{Duration: .01}, false))
	if app.idleEligible() {
		t.Fatal("playing animation allowed idle")
	}
	app.ensureECSResources()
	clock, _ := app.World.GetResource[*scene.Time]()
	clock.Delta = time.Second
	if err := scene.UpdateWorldAnimatorSystem("").Run(app.World); err != nil {
		t.Fatal(err)
	}
	if !app.idleEligible() {
		t.Fatal("completed animation prevented idle")
	}
	if _, err := app.Step(); err != nil {
		t.Fatal(err)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if rendered, err := app.Step(); rendered || err != nil {
			panic("idle schedules ran")
		}
	}); allocations != 0 {
		t.Fatalf("idle with installed capabilities allocated %g times", allocations)
	}
	app.World.GetMut[scene.Animator](entity).Play()
	if app.idleEligible() {
		t.Fatal("restarted animation failed to wake simulation")
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
	if err := app.InstallSystems[UIWidgets](); err != nil {
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
	request, _ := app.World.GetResource[*scene.RenderRequest]()
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

func TestIdleWorkerStartedBeforeFirstFrameCanWakeApp(t *testing.T) {
	app := NewApp()
	defer app.Destroy()
	app.SetRenderMode(RenderOnDemand)
	app.SetIdleEnabled(true)
	app.SetTargetFPS(0)
	request, ok := app.World.GetResource[*scene.RenderRequest]()
	if !ok || request == nil {
		t.Fatal("loaders cannot capture a wake request before the first frame")
	}
	release := make(chan struct{})
	p := resource.BeginPreparation(func(ctx context.Context) (int, error) {
		select {
		case <-release:
			return 1, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}, nil, request.Request)
	defer p.Cancel()
	if rendered, err := app.Step(); !rendered || err != nil {
		t.Fatal("first frame failed", err)
	}
	if rendered, err := app.Step(); rendered || err != nil {
		t.Fatal("waiting worker prevented idle", err)
	}
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := p.WaitCPU(ctx); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.Step(); !rendered || err != nil {
		t.Fatal("CPU completion did not wake the idle application", err)
	}
}

func TestIdleConsumesFinalPhysicsChanges(t *testing.T) {
	for _, sleep := range []bool{false, true} {
		for _, change := range []string{"static", "remove-body", "remove-transform", "inactive", "despawn", "scope"} {
			name := change
			if sleep {
				name += "/sleep"
			}
			t.Run(name, func(t *testing.T) {
				app := NewApp()
				defer app.Destroy()
				app.SetRenderMode(RenderOnDemand)
				app.SetIdleEnabled(true)
				app.SetTargetFPS(0)
				now := time.Unix(10, 0)
				app.now = func() time.Time { return now }
				if err := app.InstallSystems[Physics2D](); err != nil {
					t.Fatal(err)
				}
				if err := app.InstallSystems[Physics3D](); err != nil {
					t.Fatal(err)
				}
				r2, _ := app.World.GetResource[*physics.Runtime2D]()
				r3, _ := app.World.GetResource[*physics.Runtime3D]()
				r2.Simulation.Gravity, r3.Simulation.Gravity = physics.Vector{}, physics.Vector3D{}
				r2.Simulation.Sleep.Enabled = sleep
				spawner := app.World.NewSpawner3[physics.RigidBody2D, physics.RigidBody3D, scene.Transform]()
				e := spawner.Spawn(physics.NewRigidBody2D(physics.BodyDynamic), physics.NewRigidBody3D(physics.BodyDynamic), scene.NewTransform())
				step := func(delta time.Duration) {
					t.Helper()
					now = now.Add(delta)
					if _, err := app.Step(); err != nil {
						t.Fatal(err)
					}
				}
				step(0)
				step(time.Second / 60)
				body2 := app.World.GetRef[physics.RigidBody2D](e).Handle()
				body3 := app.World.GetRef[physics.RigidBody3D](e).Handle()
				if !body2.Valid() || !body3.Valid() {
					t.Fatal("initial physics step did not create bodies")
				}
				switch change {
				case "static":
					app.World.GetMut[physics.RigidBody2D](e).Type = physics.BodyStatic
					app.World.GetMut[physics.RigidBody3D](e).Type = physics.BodyStatic
				case "remove-body":
					app.World.Remove[physics.RigidBody2D](e)
					app.World.Remove[physics.RigidBody3D](e)
				case "remove-transform":
					app.World.Remove[scene.Transform](e)
				case "inactive":
					if err := app.World.Insert(e, scene.Activity{}); err != nil {
						t.Fatal(err)
					}
				case "despawn":
					app.World.Despawn(e)
				case "scope":
					var scope ecs.QueryScope
					scope.Reset(nil)
					app.World.SetQueryScope(&scope)
				}
				step(time.Millisecond)
				if app.idleEligible() {
					t.Fatal("unconsumed physics change allowed idle before the next fixed step")
				}
				step(time.Second / 60)
				if change == "static" {
					if body2.Type() != physics.BodyStatic || body3.Type() != physics.BodyStatic {
						t.Fatal("type change was not consumed")
					}
				} else if body2.Valid() || body3.Valid() {
					t.Fatal("removed or excluded bodies remained in the simulation")
				}
				if !app.idleEligible() {
					t.Fatal("consumed physics change kept the app awake")
				}
			})
		}
	}
}

func TestIdleInitializesStaticPhysicsBeforeSleeping(t *testing.T) {
	app := NewApp()
	defer app.Destroy()
	app.SetRenderMode(RenderOnDemand)
	app.SetIdleEnabled(true)
	app.SetTargetFPS(0)
	now := time.Unix(10, 0)
	app.now = func() time.Time { return now }
	if err := app.InstallSystems[Physics2D](); err != nil {
		t.Fatal(err)
	}
	if err := app.InstallSystems[Physics3D](); err != nil {
		t.Fatal(err)
	}
	s := app.World.NewSpawner3[physics.RigidBody2D, physics.RigidBody3D, scene.Transform]()
	e := s.Spawn(physics.NewRigidBody2D(physics.BodyStatic), physics.NewRigidBody3D(physics.BodyStatic), scene.NewTransform())
	if _, err := app.Step(); err != nil {
		t.Fatal(err)
	}
	if app.idleEligible() {
		t.Fatal("static scene idled before its first physics tick")
	}
	now = now.Add(time.Second / 60)
	if _, err := app.Step(); err != nil {
		t.Fatal(err)
	}
	if !app.World.GetRef[physics.RigidBody2D](e).Handle().Valid() || !app.World.GetRef[physics.RigidBody3D](e).Handle().Valid() {
		t.Fatal("static bodies were not created")
	}
	if !app.idleEligible() {
		t.Fatal("initialized static scene did not idle")
	}
}
