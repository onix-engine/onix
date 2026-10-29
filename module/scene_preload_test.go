package module

import (
	"errors"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/physics"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
)

type scopeProbe struct{}

func TestPreparedSceneCanBeFirstForeground(t *testing.T) {
	for _, entry := range []bool{false, true} {
		t.Run(map[bool]string{false: "activate", true: "start-entry"}[entry], func(t *testing.T) {
			plan, registry, p := managerPlan(t)
			enters := 0
			install := registry.scenes["main.json"]
			registry.scenes["main.json"] = func(c *Context) error {
				if err := install(c); err != nil {
					return err
				}
				c.enter = func(v SceneStartup) error {
					enters++
					if v.From != "" || v.Value.String != "initial" {
						return ErrInvalid
					}
					return nil
				}
				return nil
			}
			r := newFixture(t, plan, registry, Options{})
			ready, err := r.Manager().Preload(ScenePreloadRequest{Scene: "main.json", Startup: project.SlotValue{Type: project.StringValue, String: "initial"}})
			must(t, err)
			if r.NeedsTick() || r.Manager().Active() != nil {
				t.Fatal("initial preload started simulation")
			}
			if entry {
				_, err = r.Manager().StartEntry()
				must(t, err)
			} else {
				must(t, r.Manager().Activate(ready, SceneActivationOptions{}))
				must(t, r.Dispatch())
			}
			if r.Manager().Active() != ready || enters != 1 || p.installs != 1 {
				t.Fatal("initial activation did not reuse the prepared instance")
			}
		})
	}
}

func TestEntryEnterFailureDisposesNewInstanceBeforeRetry(t *testing.T) {
	plan, registry, _ := managerPlan(t)
	enters, destroys := 0, 0
	delete(registry.scenes, "main.json")
	must(t, registry.RegisterScene("main.json", func() Signal { return Signal{} }, Lifecycle[Signal]{
		Enter: func(*Context, *Signal, SceneStartup) error {
			enters++
			if enters == 1 {
				return ErrInvalid
			}
			return nil
		},
		Destroy: func(*Context, *Signal) error { destroys++; return nil },
	}))
	r := newFixture(t, plan, registry, Options{})
	_, err := r.Manager().StartEntry()
	if !errors.Is(err, ErrInvalid) || len(r.scenes) != 0 || r.world.EntityCount() != 0 || destroys != 1 {
		t.Fatal("failed initial activation leaked its instance", err)
	}
	_, err = r.Manager().StartEntry()
	must(t, err)
	if len(r.scenes) != 1 || enters != 2 {
		t.Fatal("entry activation could not be retried")
	}
}

func TestScenePreloadIsHeadlessAndActivationReusesPreparedInstance(t *testing.T) {
	plan, registry, p := managerPlan(t)
	starts, enters, resumes := 0, 0, 0
	install := registry.scenes["game.json"]
	registry.scenes["game.json"] = func(c *Context) error {
		if err := install(c); err != nil {
			return err
		}
		c.enter = func(value SceneStartup) error {
			enters++
			if value.From != "main.json" || value.Value.String != "prepared" {
				return ErrInvalid
			}
			return nil
		}
		c.resume = func(SceneStartup) error { resumes++; return nil }
		return handleValue(c, "scene.startup", func(*Context, SceneStartup) error { starts++; return nil })
	}
	r := newFixture(t, plan, registry, Options{})
	main, err := r.Manager().StartEntry()
	must(t, err)
	tick := r.tick
	ready, err := r.Manager().Preload(ScenePreloadRequest{Scene: "game.json", Startup: project.SlotValue{Type: project.StringValue, String: "prepared"}})
	must(t, err)
	if ready.State() != SceneReady || r.Manager().Active() != main || starts != 1 || enters != 0 || r.tick != tick {
		t.Fatal("preloading entered the game loop or changed foreground state")
	}
	button := p.buttons[1]
	for e := range r.world.Query[scene.Sprite2D]().All {
		if e == button.root {
			t.Fatal("ready scene entered simulation queries")
		}
	}
	previous := r.world.SetQueryScope(r.RenderScope())
	for e := range r.world.Query[scene.Sprite2D]().All {
		if e == button.root {
			t.Fatal("ready scene entered render queries")
		}
	}
	r.world.SetQueryScope(previous)
	emit, err := button.BindEmitter[Signal]("clicked")
	must(t, err)
	if !errors.Is(emit.Emit(Signal{}), ErrUnavailable) {
		t.Fatal("ready scene accepted a runtime event")
	}
	generation := ready.Generation()
	must(t, r.Manager().Activate(ready, SceneActivationOptions{Retain: true}))
	must(t, r.Dispatch())
	if r.Manager().Active() != ready || ready.Generation() != generation || p.installs != 2 || enters != 1 || starts != 1 {
		t.Fatal("activation rebuilt the scene or repeated startup")
	}
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "main.json", Retain: true}))
	must(t, r.Dispatch())
	must(t, r.Manager().Activate(ready, SceneActivationOptions{}))
	must(t, r.Dispatch())
	if resumes != 1 || enters != 1 || starts != 1 {
		t.Fatal("resumption repeated initial lifetime hooks")
	}
}

func TestSceneScopesExcludeHiddenProceduralDescendantsAndPausePhysics(t *testing.T) {
	plan, registry, p := managerPlan(t)
	r := newFixture(t, plan, registry, Options{})
	main, err := r.Manager().StartEntry()
	must(t, err)
	w := r.world
	b := ecs.NewBundle(5)
	b.Put(scene.Parent{Entity: p.buttons[0].root})
	b.Put(scopeProbe{})
	b.Put(scene.TransformFromXYZ(0, 0, 0))
	body := physics.NewRigidBody2D(physics.BodyDynamic)
	body.Velocity = physics.V(10, 0)
	b.Put(body)
	child := w.Spawn(b)
	w.InsertResource(&scene.Time{Delta: time.Second / 60})
	r.SyncScopes()
	must(t, physics.StepSystem2D("").Run(w))
	pose, _ := w.Get[scene.Transform](child)
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "game.json", Retain: true}))
	must(t, r.Dispatch())
	for range w.Query[scopeProbe]().All {
		t.Fatal("hidden descendant remained in simulation")
	}
	previous := w.SetQueryScope(r.RenderScope())
	for range w.Query[scopeProbe]().All {
		t.Fatal("hidden descendant remained in rendering")
	}
	w.SetQueryScope(previous)
	must(t, physics.StepSystem2D("").Run(w))
	after, _ := w.Get[scene.Transform](child)
	if after != pose || main.State() != SceneSuspended {
		t.Fatal("hidden scene continued simulating")
	}
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "main.json"}))
	must(t, r.Dispatch())
	must(t, physics.StepSystem2D("").Run(w))
	after, _ = w.Get[scene.Transform](child)
	if after.Translation.X() <= pose.Translation.X() {
		t.Fatal("retained physics did not resume")
	}
	must(t, main.Dispose())
	if w.IsAlive(child) {
		t.Fatal("scene disposal leaked a procedural descendant")
	}
}

func TestSceneOverlayKeepsBackgroundRenderingAndOnlyUpdatesForeground(t *testing.T) {
	plan, registry, p := managerPlan(t)
	r := newFixture(t, plan, registry, Options{})
	main, err := r.Manager().StartEntry()
	must(t, err)
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "game.json", Overlay: true}))
	must(t, r.Dispatch())
	game := r.Manager().Active()
	if main.State() != SceneSuspended || !main.rendered || !scene.IsVisible(r.world, p.buttons[0].root) || scene.IsInteractive(r.world, p.buttons[0].root) {
		t.Fatal("overlay background visibility/input were wrong")
	}
	must(t, r.Update(1.0/60))
	if p.mainUpdates != 0 || p.gameUpdates != 1 || game.layer.Order <= main.layer.Order {
		t.Fatal("overlay updated its background or lost painter order")
	}
	count := 0
	for range r.world.Query[scene.Sprite2D]().All {
		count++
	}
	if count != 1 {
		t.Fatalf("simulation sprites = %d, want 1", count)
	}
	previous := r.world.SetQueryScope(r.RenderScope())
	count = 0
	for range r.world.Query[scene.Sprite2D]().All {
		count++
	}
	r.world.SetQueryScope(previous)
	if count != 2 {
		t.Fatalf("render sprites = %d, want 2", count)
	}
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "main.json"}))
	must(t, r.Dispatch())
	if game.Alive() || !main.rendered {
		t.Fatal("closing overlay did not restore its retained background")
	}
}

func TestPreparedSceneDisposedBeforeActivationIsNotReloaded(t *testing.T) {
	plan, registry, p := managerPlan(t)
	r := newFixture(t, plan, registry, Options{})
	main, err := r.Manager().StartEntry()
	must(t, err)
	ready, err := r.Manager().Preload(ScenePreloadRequest{Scene: "game.json"})
	must(t, err)
	must(t, r.Manager().Activate(ready, SceneActivationOptions{}))
	// Disposal shares the queued structural boundary with activation.
	r.busy = true
	must(t, ready.Dispose())
	r.busy = false
	must(t, r.Dispatch())
	must(t, r.Dispatch())
	if r.Manager().Active() != main || p.installs != 2 || !errors.Is(p.result.Err, ErrUnavailable) {
		t.Fatal("discarded prepared instance was reloaded")
	}
}

func TestRetainedSceneSwitchAndScopedTicksAllocateNothingWarm(t *testing.T) {
	plan, registry, _ := managerPlan(t)
	r := newFixture(t, plan, registry, Options{})
	_, err := r.Manager().StartEntry()
	must(t, err)
	step := func() {
		id := project.SceneID("game.json")
		if r.Manager().Active().ID() == id {
			id = "main.json"
		}
		must(t, r.Manager().Switch(SceneSwitchRequest{Scene: id, Retain: true}))
		must(t, r.Dispatch())
		must(t, r.Dispatch())
		must(t, r.Update(1.0/60))
		r.SyncScopes()
		for range r.world.Query[scene.Sprite2D]().All {
		}
	}
	for range 8 {
		step()
	}
	if allocations := testing.AllocsPerRun(100, step); allocations != 0 {
		t.Fatalf("warm scene switches/ticks allocated %g times", allocations)
	}
}
