package onix

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
)

func appModulePlan(t *testing.T, mod string) *project.ProjectPlan {
	t.Helper()
	plan, err := project.CompileProject(fstest.MapFS{
		"resources/project.json":     &fstest.MapFile{Data: []byte(`{"entry":"main.json","scenes":["main.json"],"clearColor":"#123456"}`)},
		"resources/scenes/main.json": &fstest.MapFile{Data: []byte(`{"modules":[{"id":"a","mod":"mod"}]}`)},
		"mod/mod.json":               &fstest.MapFile{Data: []byte(mod)},
	}, ".")
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestAppModuleFrameUpdateCanIdleAndRunsOncePerWake(t *testing.T) {
	app := NewApp()
	defer app.Destroy()
	app.SetRenderMode(RenderOnDemand)
	app.SetIdleEnabled(true)
	app.SetTargetFPS(0)
	now := time.Unix(10, 0)
	app.now = func() time.Time { return now }
	frames := 0
	registry := module.Registry{}
	if err := module.RegisterMod(&registry, "mod", func() struct{} { return struct{}{} }, module.Lifecycle[struct{}]{
		FrameUpdate: func(*module.Context, *struct{}, module.Tick) error { frames++; return nil },
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.LoadModules(appModulePlan(t, `{}`), registry, module.Options{}); err != nil {
		t.Fatal(err)
	}
	if err := app.Frame(); err != nil {
		t.Fatal(err)
	}
	if frames != 1 || !app.idleEligible() {
		t.Fatalf("frames=%d idle=%v", frames, app.idleEligible())
	}
	if err := app.Frame(); err != nil {
		t.Fatal(err)
	}
	if frames != 1 {
		t.Fatal("idle ran the frame callback")
	}
	// A wake before the next fixed tick still runs application synchronization.
	now = now.Add(time.Millisecond)
	app.RequestRender()
	if err := app.Frame(); err != nil {
		t.Fatal(err)
	}
	if frames != 2 {
		t.Fatal("frame callback waited for a fixed tick")
	}
	// Fixed catch-up must not multiply UI/application work.
	now = now.Add(time.Second)
	app.RequestRender()
	if err := app.Frame(); err != nil {
		t.Fatal(err)
	}
	if frames != 3 {
		t.Fatal("fixed catch-up repeated the frame callback")
	}
}

func TestAppSceneScopesSeparateSimulationAndOverlayRendering(t *testing.T) {
	restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(320, 240))
	defer restore()
	app := NewApp()
	defer app.Destroy()
	plan := appModulePlan(t, `{"components":{"sprite":{"color":"#ffffff","size":[10,10]}}}`)
	game := plan.Scenes[0]
	game.ID = "game.json"
	plan.Scenes = append(plan.Scenes, game)
	main, err := app.LoadModules(plan, module.Registry{}, module.Options{})
	if err != nil {
		t.Fatal(err)
	}
	manager := app.Modules().Manager()
	ready, err := manager.Preload(module.ScenePreloadRequest{Scene: "game.json"})
	if err != nil {
		t.Fatal(err)
	}
	updated, rendered := 0, 0
	count := func(world *ecs.World) int {
		n := 0
		for range ecs.Query[scene.Sprite2D](world).All {
			n++
		}
		return n
	}
	if err := app.AddSystem(StageUpdate, ecs.SystemConfig{Name: "observe_running_scenes", System: func(c *ecs.SystemContext) error { updated = count(c.World); return nil }}); err != nil {
		t.Fatal(err)
	}
	if err := app.AddSystem(StageRender, ecs.SystemConfig{Name: "observe_rendered_scenes", System: func(c *ecs.SystemContext) error { rendered = count(c.World); return nil }}); err != nil {
		t.Fatal(err)
	}
	check := func(wantRendered int) {
		t.Helper()
		if err := app.runStages([]Stage{StageUpdate}); err != nil {
			t.Fatal(err)
		}
		if err := app.renderFrame(); err != nil {
			t.Fatal(err)
		}
		if updated != 1 || rendered != wantRendered || count(app.World) != 1 {
			t.Fatalf("update/render/restored scope = %d/%d/%d", updated, rendered, count(app.World))
		}
	}
	check(1)
	if err := manager.Activate(ready, module.SceneActivationOptions{Overlay: true}); err != nil {
		t.Fatal(err)
	}
	if err := app.Modules().Dispatch(); err != nil {
		t.Fatal(err)
	}
	check(2)
	if main.State() != module.SceneSuspended {
		t.Fatal("overlay did not retain the background")
	}
	if err := manager.Activate(main, module.SceneActivationOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := app.Modules().Dispatch(); err != nil {
		t.Fatal(err)
	}
	check(1)
}

func TestAppModuleAuthoredCameraSettingsSurviveSystemInstallation(t *testing.T) {
	app := NewApp()
	defer app.Destroy()
	authored := scene.SpawnCamera2D(app.World)
	camera := ecs.GetMut[scene.Camera](app.World, authored)
	camera.Order, camera.Clear, camera.ClearColor = 17, false, graphics.ColorFromRGB(3, 4, 5)
	before := *camera
	ecs.InsertResource(app.World, scene.RenderSettings2D{ClearColor: graphics.White})
	if err := InstallSystems[Render2D](app); err != nil {
		t.Fatal(err)
	}
	if _, err := app.LoadModules(appModulePlan(t, `{}`), module.Registry{}, module.Options{}); err != nil {
		t.Fatal(err)
	}
	after, _ := ecs.Get[scene.Camera](app.World, authored)
	if before.Order != after.Order || before.Clear != after.Clear || before.ClearColor != after.ClearColor || after.Fallback {
		t.Fatal("host installation overwrote the authored camera")
	}
	fallbacks := 0
	for _, camera := range ecs.Query[scene.Camera](app.World).All {
		if camera.Get().Fallback {
			fallbacks++
		}
	}
	if fallbacks != 1 {
		t.Fatal("engine fallback was not prepared", fallbacks)
	}
}

func TestAppModuleClearColorAppliesToFallbackCamera(t *testing.T) {
	for _, preinstall := range []bool{false, true} {
		name := "automatic_system_installation"
		if preinstall {
			name = "preinstalled_systems"
		}
		t.Run(name, func(t *testing.T) {
			app := NewApp()
			defer app.Destroy()
			if preinstall {
				if err := InstallSystems[Render2D](app); err != nil {
					t.Fatal(err)
				}
			}
			plan := appModulePlan(t, `{"components":{"sprite":{"color":"#ffffff","size":[10,10]}}}`)
			plan.Spec.ClearColor = "#092a38"
			if _, err := app.LoadModules(plan, module.Registry{}, module.Options{}); err != nil {
				t.Fatal(err)
			}
			want := graphics.ColorFromRGB(9, 42, 56)
			settings, ok := ecs.GetResource[scene.RenderSettings2D](app.World)
			if !ok || settings.ClearColor != want {
				t.Fatalf("project clear color resource = %+v, present=%t", settings, ok)
			}
			app.Modules().SyncScopes()
			fallbacks := 0
			for _, camera := range ecs.Query[scene.Camera](app.World).All {
				value := camera.Get()
				if !value.Fallback {
					continue
				}
				fallbacks++
				if !value.Clear || value.ClearColor != want {
					t.Fatalf("fallback camera clear=%t color=%v, want %v", value.Clear, value.ClearColor, want)
				}
			}
			if fallbacks != 1 {
				t.Fatalf("fallback camera count = %d, want 1", fallbacks)
			}
		})
	}
}

func TestAppModulesDispatchPerFixedTickAndDestroy(t *testing.T) {
	restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(320, 240))
	defer restore()
	app := NewApp()
	defer app.Destroy()
	now := time.Unix(0, 0)
	app.now = func() time.Time { return now }
	app.SetTargetFPS(DefaultTargetTPS * 2)
	plan := appModulePlan(t, `{}`)
	type state struct{ emit module.Emitter[int64] }
	updates, received, destroyed := 0, 0, 0
	var emitters []module.Emitter[int64]
	registry := module.Registry{}
	err := module.RegisterMod(&registry, "mod", func() state { return state{} }, module.Lifecycle[state]{
		Install: func(c *module.Context, s *state) error {
			emit, err := module.BindEmitter[int64](c, "ping")
			if err != nil {
				return err
			}
			s.emit = emit
			emitters = append(emitters, emit)
			return nil
		},
		OnMessage: func(c *module.Context, s *state, message module.Message) error {
			value, ok := module.Value[int64](message)
			if !ok {
				return nil
			}
			received++
			if value > 0 {
				return s.emit.EmitTo(c.Name(), value-1)
			}
			return nil
		},
		Update: func(_ *module.Context, _ *state, tick module.Tick) error {
			if tick.Delta != float32(app.fixedTime.Timestep.Seconds()) {
				t.Fatal("module update used frame delta")
			}
			updates++
			return nil
		}, Destroy: func(*module.Context, *state) error { destroyed++; return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := app.LoadModules(plan, registry, module.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if app.Context != nil {
		t.Fatal("loading modules opened a native window")
	}
	if _, err := app.LoadModules(plan, registry, module.Options{}); err == nil {
		t.Fatal("second runtime loaded without disposal")
	}
	if _, err := app.Modules().MountEntry(); err != nil {
		t.Fatal(err)
	}
	for _, emit := range emitters {
		if err := emit.EmitTo("a", 2); err != nil {
			t.Fatal(err)
		}
	}
	if err := app.Frame(); err != nil {
		t.Fatal(err)
	}
	if updates != 0 || received != 0 {
		t.Fatal("module ran without a fixed tick")
	}
	now = now.Add(app.fixedTime.Timestep / 2)
	if err := app.Frame(); err != nil {
		t.Fatal(err)
	}
	if updates != 0 {
		t.Fatal("render frame was treated as a module tick")
	}
	now = now.Add(app.fixedTime.Timestep / 2)
	if err := app.Frame(); err != nil {
		t.Fatal(err)
	}
	if updates != 2 || received != 2 {
		t.Fatalf("expected two instances at one fixed tick, got %d", updates)
	}
	now = now.Add(app.fixedTime.Timestep * 2)
	if err := app.Frame(); err != nil {
		t.Fatal(err)
	}
	if updates != 6 || received != 6 {
		t.Fatal("fixed catch-up did not update modules once per tick")
	}
	if app.World.EntityCount() == 0 || !entry.Alive() {
		t.Fatal("entry did not mount")
	}
	app.Destroy()
	if entry.Alive() || destroyed != 2 || app.World.EntityCount() != 0 {
		t.Fatal("App destruction leaked module entities/behaviors")
	}
}

func TestAppModuleColorContentInputAndStableFrames(t *testing.T) {
	restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(320, 240))
	defer restore()
	app := NewApp()
	defer app.Destroy()
	now := time.Unix(0, 0)
	app.now = func() time.Time { return now }
	plan := appModulePlan(t, `{"components":{"sprite":{"color":"#2f7df6","size":[160,48]},"input":{"pointer":true}}}`)
	registry := module.Registry{}
	clicks := 0
	if err := module.RegisterScene(&registry, "main.json", func() module.Signal { return module.Signal{} }, module.Lifecycle[module.Signal]{OnMessage: func(c *module.Context, _ *module.Signal, message module.Message) error {
		if message.Topic == module.InputTopic {
			value, ok := module.Value[module.Input](message)
			if !ok || value.Event.Kind != scene.InputEventTap {
				return nil
			}
			return module.Send(c, "a", "clicked", module.Signal{})
		}
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if err := module.RegisterMod(&registry, "mod", func() module.Signal { return module.Signal{} }, module.Lifecycle[module.Signal]{OnMessage: func(_ *module.Context, _ *module.Signal, m module.Message) error {
		if m.Topic == "clicked" {
			clicks++
		}
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.LoadModules(plan, registry, module.Options{}); err != nil {
		t.Fatal(err)
	}
	var root ecs.Entity
	for entity := range ecs.Query[scene.Sprite2D](app.World).All {
		root = entity
	}
	if root == 0 {
		t.Fatal("color sprite was not installed")
	}
	if err := app.Modules().HandleInput(scene.InputEvent{Entity: root, Kind: scene.InputEventTap}); err != nil {
		t.Fatal(err)
	}
	if err := app.Frame(); err != nil {
		t.Fatal(err)
	}
	if clicks != 0 {
		t.Fatal("input event bypassed fixed-tick delivery")
	}
	now = now.Add(app.fixedTime.Timestep)
	if err := app.Frame(); err != nil {
		t.Fatal(err)
	}
	if clicks != 1 {
		t.Fatal("pointer input did not reach its module event")
	}
	for range 64 {
		now = now.Add(app.fixedTime.Timestep)
		if err := app.Frame(); err != nil {
			t.Fatal(err)
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		now = now.Add(app.fixedTime.Timestep)
		if err := app.Frame(); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("stable module frame allocated %.2f times", allocations)
	}
}

func TestAppModulePendingEventsPreventIdle(t *testing.T) {
	app := NewApp()
	defer app.Destroy()
	app.SetRenderMode(RenderOnDemand)
	app.SetIdleEnabled(true)
	plan := appModulePlan(t, `{}`)
	registry := module.Registry{}
	var emit module.Emitter[module.Signal]
	if err := module.RegisterMod(&registry, "mod", func() module.Signal { return module.Signal{} }, module.Lifecycle[module.Signal]{Install: func(c *module.Context, _ *module.Signal) error {
		var err error
		emit, err = module.BindEmitter[module.Signal](c, "ping")
		return err
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.LoadModules(plan, registry, module.Options{}); err != nil {
		t.Fatal(err)
	}
	if !app.idleEligible() {
		t.Fatal("service-only modules prevented idle")
	}
	if err := emit.Emit(module.Signal{}); err != nil {
		t.Fatal(err)
	}
	if app.idleEligible() {
		t.Fatal("queued module event could sleep before its logic tick")
	}
	if err := app.Modules().Dispatch(); err != nil {
		t.Fatal(err)
	}
	if !app.idleEligible() {
		t.Fatal("empty channel failed to return to idle")
	}
}

func TestAppModulePendingRequestsPreventIdleUntilTimeout(t *testing.T) {
	app := NewApp()
	defer app.Destroy()
	app.SetRenderMode(RenderOnDemand)
	app.SetIdleEnabled(true)
	plan, err := project.CompileProject(fstest.MapFS{
		"resources/project.json":     &fstest.MapFile{Data: []byte(`{"entry":"main.json","scenes":["main.json"]}`)},
		"resources/scenes/main.json": &fstest.MapFile{Data: []byte(`{"modules":[{"id":"a","mod":"service"}]}`)},
		"service/mod.json":           &fstest.MapFile{Data: []byte(`{}`)},
	}, ".")
	if err != nil {
		t.Fatal(err)
	}
	registry := module.Registry{}
	var failure error
	var requester module.Requester[string, int64]
	if err := module.RegisterMod(&registry, "service", func() module.Signal { return module.Signal{} }, module.Lifecycle[module.Signal]{Install: func(c *module.Context, _ *module.Signal) error {
		var err error
		requester, err = module.BindRequester[string, int64](c, "ask")
		return err
	}, OnMessage: func(_ *module.Context, _ *module.Signal, message module.Message) error {
		if result, ok := module.ResultValue[int64](message); ok {
			failure = result.Err
		}
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.LoadModules(plan, registry, module.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := requester.SendTo("a", "work", module.RequestOptions{TimeoutTicks: 3}); err != nil {
		t.Fatal(err)
	}
	if err := app.Modules().Dispatch(); err != nil {
		t.Fatal(err)
	}
	if app.Modules().Queued() != 0 || app.Modules().Pending() != 1 || app.idleEligible() {
		t.Fatal("pending request slept before timeout")
	}
	if err := app.Modules().Dispatch(); err != nil {
		t.Fatal(err)
	}
	if err := app.Modules().Dispatch(); err != nil {
		t.Fatal(err)
	}
	if failure != module.ErrTimeout || !app.idleEligible() {
		t.Fatal("timeout failed to allow idle again")
	}
}

func TestAppModuleFailedMountCanRetryWithNewCapabilities(t *testing.T) {
	restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(320, 240))
	defer restore()
	app := NewApp()
	defer app.Destroy()
	invalid := appModulePlan(t, `{}`)
	badRegistry := module.Registry{}
	if err := module.RegisterMod(&badRegistry, "mod", func() module.Signal { return module.Signal{} }, module.Lifecycle[module.Signal]{Install: func(*module.Context, *module.Signal) error { return module.ErrInvalid }}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.LoadModules(invalid, badRegistry, module.Options{}); err == nil {
		t.Fatal("failed installation was accepted")
	}
	if app.Modules() != nil || app.World.EntityCount() != 0 {
		t.Fatal("failed entry mount polluted the app")
	}
	valid := appModulePlan(t, `{"components":{"sprite":{"color":"#ff0000","size":[10,10]},"input":{"pointer":true}}}`)
	if _, err := app.LoadModules(valid, module.Registry{}, module.Options{}); err != nil {
		t.Fatal(err)
	}
	if !app.hasSystemSet(systemSetRender2D) || !app.hasSystemSet(systemSetInput2D) {
		t.Fatal("retry failed to install new capabilities")
	}
	if err := app.Frame(); err != nil {
		t.Fatal(err)
	}
}
