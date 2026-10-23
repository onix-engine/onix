package module

import (
	"errors"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
)

func managerPlan(t testing.TB) (*project.ProjectPlan, Registry, *managerProbe) {
	t.Helper()
	plan, _ := compileFixture(t, map[string]string{
		"resources/project.json":       `{"entry":"main.json","scenes":["main.json","game.json","broken.json","loop.json"]}`,
		"resources/scenes/main.json":   `{"modules":[{"id":"button","mod":"button"}]}`,
		"resources/scenes/game.json":   `{"modules":[{"id":"button","mod":"button"}]}`,
		"resources/scenes/broken.json": `{"modules":[]}`,
		"resources/scenes/loop.json":   `{"modules":[]}`,
		"button/mod.json":              `{"components":{"sprite":{"color":"#ffffff","size":[10,10]},"input":{"pointer":true}},"slots":{"text":{"type":"string"}}}`,
	})
	p := &managerProbe{}
	r := Registry{}
	must(t, RegisterMod(&r, "button", func() Signal { return Signal{} }, Lifecycle[Signal]{
		Install: func(c *Context, _ *Signal) error { p.buttons = append(p.buttons, c); p.installs++; return nil },
		Update: func(c *Context, _ *Signal, _ Tick) error {
			if c.scene.ID() == "main.json" {
				p.mainUpdates++
			} else {
				p.gameUpdates++
			}
			return nil
		},
	}))
	must(t, RegisterScene(&r, "main.json", func() Signal { return Signal{} }, Lifecycle[Signal]{OnMessage: func(c *Context, _ *Signal, m Message) error {
		switch m.Topic {
		case InputTopic:
			return Send(c, SceneTarget, "clicked", Signal{})
		case "clicked":
			return c.Switch(SceneSwitchRequest{Scene: "game.json", Startup: project.SlotValue{Type: project.StringValue, String: "hello"}})
		case "scene.switch.result":
			p.result, _ = Value[SceneSwitchResult](m)
			p.results++
		}
		return nil
	}}))
	must(t, RegisterScene(&r, "game.json", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error {
		if err := handleValue(c, "ack", func(_ *Context, value string) error { p.ack = value; return nil }); err != nil {
			return err
		}
		return handleValue(c, "scene.startup", func(c *Context, value SceneStartup) error {
			p.startup = value
			p.loadingState = c.scene.state
			p.duringActive = c.runtime.Manager().Active()
			return nil
		})
	}}))
	must(t, RegisterScene(&r, "broken.json", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(*Context, *Signal) error { return ErrInvalid }}))
	must(t, RegisterScene(&r, "loop.json", func() Signal { return Signal{} }, Lifecycle[Signal]{Install: func(c *Context, _ *Signal) error {
		emit, err := BindEmitter[Signal](c, "loop")
		if err != nil {
			return err
		}
		if err = handleValue(c, "loop", func(*Context, Signal) error { return emit.Emit(Signal{}) }); err != nil {
			return err
		}
		return emit.Emit(Signal{})
	}}))
	return plan, r, p
}

type managerProbe struct {
	buttons                                     []*Context
	installs, mainUpdates, gameUpdates, results int
	message, ack                                string
	result                                      SceneSwitchResult
	startup                                     SceneStartup
	loadingState                                SceneState
	duringActive                                *Scene
}

func TestSceneManagerDeclarativeClickBridgePreparesBeforeActivation(t *testing.T) {
	plan, registry, p := managerPlan(t)
	r := newFixture(t, plan, registry, Options{})
	main, err := r.Manager().StartEntry()
	must(t, err)
	if main.State() != SceneActive {
		t.Fatal("entry was not active")
	}
	must(t, r.HandleInput(scene.InputEvent{Entity: p.buttons[0].root, Kind: scene.InputEventTap}))
	must(t, r.Dispatch())
	if r.Manager().Active() == main {
		t.Fatal("scene did not apply the queued switch at the boundary")
	}
	must(t, r.Dispatch())
	game := r.Manager().Active()
	if game == main || game.ID() != "game.json" || main.State() != SceneDisposed || game.State() != SceneActive {
		t.Fatal("click did not switch scenes")
	}
	if p.startup.Value.String != "hello" || p.startup.From != "main.json" || p.loadingState != SceneLoading || p.duringActive != main {
		t.Fatal("startup was not isolated before activation")
	}
	if r.Pending() != 0 || r.tick != 2 {
		t.Fatal("startup leaked requests or advanced the active clock")
	}
	if _, ok := ecs.Get[scene.Visibility](r.world, p.buttons[0].root); ok {
		t.Fatal("old scene entity leaked")
	}
}

func TestSceneManagerFailurePreservesSourceAndReportsResult(t *testing.T) {
	for _, target := range []project.SceneID{"broken.json", "loop.json", "missing.json"} {
		t.Run(string(target), func(t *testing.T) {
			plan, registry, p := managerPlan(t)
			r := newFixture(t, plan, registry, Options{StartupBatches: 3})
			main, err := r.Manager().StartEntry()
			must(t, err)
			count := r.world.EntityCount()
			must(t, r.Manager().Switch(SceneSwitchRequest{Scene: target}))
			must(t, r.Dispatch())
			if r.Manager().Active() != main || main.State() != SceneActive || r.world.EntityCount() != count {
				t.Fatal("failed destination disturbed source")
			}
			must(t, r.Dispatch())
			if p.results != 1 || p.result.Err == nil || p.result.From != "main.json" || p.result.To != target {
				t.Fatal("switch failure was not delivered")
			}
			if target == "loop.json" && !errors.Is(p.result.Err, ErrStartupLimit) {
				t.Fatalf("startup failure %v", p.result.Err)
			}
			must(t, r.Update(1.0/60))
			if p.mainUpdates != 1 {
				t.Fatal("original scene stopped updating after failure")
			}
		})
	}
}

func TestSceneManagerRetainsPausesAndResumesSameGeneration(t *testing.T) {
	plan, registry, p := managerPlan(t)
	r := newFixture(t, plan, registry, Options{})
	main, err := r.Manager().StartEntry()
	must(t, err)
	writer, err := main.SlotWriter("button", "text")
	must(t, err)
	must(t, writer.Write(project.SlotValue{Type: project.StringValue, String: "retained"}))
	must(t, r.Dispatch())
	button := p.buttons[0]
	clicked, err := BindEmitter[Signal](button, "clicked")
	must(t, err)
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "game.json", Retain: true}))
	must(t, r.Dispatch())
	game := r.Manager().Active()
	if main.State() != SceneSuspended || !main.Alive() {
		t.Fatal("retained scene was disposed")
	}
	visibility, _ := ecs.Get[scene.Visibility](r.world, button.root)
	if visibility.Visible {
		t.Fatal("suspended content remained visible")
	}
	if !errors.Is(clicked.Emit(Signal{}), ErrUnavailable) || !errors.Is(writer.Write(project.SlotValue{Type: project.StringValue}), ErrUnavailable) {
		t.Fatal("suspended module accepted events")
	}
	queued := r.Queued()
	must(t, r.HandleInput(scene.InputEvent{Entity: button.root, Kind: scene.InputEventTap}))
	if r.Queued() != queued {
		t.Fatal("suspended input generated events")
	}
	must(t, r.Update(1.0/60))
	if p.mainUpdates != 0 || p.gameUpdates != 1 {
		t.Fatal("suspended scene updated")
	}
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "main.json"}))
	must(t, r.Dispatch())
	if r.Manager().Active() != main || main.State() != SceneActive || game.State() != SceneDisposed || p.installs != 2 {
		t.Fatal("resume recreated behavior")
	}
	visibility, _ = ecs.Get[scene.Visibility](r.world, button.root)
	if !visibility.Visible {
		t.Fatal("resume did not restore visibility")
	}
	text, _ := button.Slot("text")
	value, _ := ReadSlot[string](text)
	if value != "retained" {
		t.Fatal("resume reset slot state")
	}
	must(t, r.Update(1.0/60))
	if p.mainUpdates != 1 {
		t.Fatal("resumed update did not run")
	}
}

func TestPreparationPreservesParkedPayloadsAndRequestDeadlines(t *testing.T) {
	f := setupRequests(t, Options{StartupBatches: 2}, false)
	// A destination loops through the same int store used by a parked reply.
	// Preparation must preserve that value, the slot buffer and request deadline.
	_, err := f.clients[0].SendTo("server", "parked", RequestOptions{TimeoutTicks: 20})
	must(t, err)
	must(t, f.runtime.Dispatch())
	must(t, f.requests[0].Reply(f.server, 42))
	writer, err := f.scene.SlotWriter("a", "text")
	must(t, err)
	must(t, writer.Write(project.SlotValue{Type: project.StringValue, String: "held"}))
	index := f.runtime.pendingFirst
	before := f.runtime.pending[index]
	extra, _ := compileFixture(t, map[string]string{"resources/scenes/main.json": `{"modules":[]}`})
	extra.Scenes[0].ID = "loop.json"
	f.runtime.plan.Scenes = append(f.runtime.plan.Scenes, extra.Scenes[0])
	f.runtime.registry.scenes["loop.json"] = func(c *Context) error {
		emit, err := BindEmitter[int64](c, "loop")
		if err != nil {
			return err
		}
		if err = handleValue(c, "loop", func(*Context, int64) error { return emit.Emit(99) }); err != nil {
			return err
		}
		return emit.Emit(99)
	}
	_, err = f.runtime.Manager().prepare("loop.json", SceneStartup{})
	if !errors.Is(err, ErrStartupLimit) || f.runtime.pending[index].id != before.id || f.runtime.pending[index].deadline != before.deadline || f.runtime.tick != 1 || f.runtime.Queued() != 2 {
		t.Fatal("failed preparation altered active pending requests")
	}
	must(t, f.runtime.Dispatch())
	if len(f.results) != 1 || f.results[0].Value != 42 {
		t.Fatal("preparation overwrote parked reply payload")
	}
	text, _ := f.contexts[0].Slot("text")
	value, _ := ReadSlot[string](text)
	if value != "held" {
		t.Fatal("preparation cleared parked slot payload")
	}
}

func TestRetainedSceneResumeFailureKeepsCurrentActiveScene(t *testing.T) {
	plan, registry, _ := managerPlan(t)
	starts, resumes := 0, 0
	registry.scenes["main.json"] = func(c *Context) error {
		c.resume = func(SceneStartup) error { resumes++; return ErrInvalid }
		return handleValue(c, "scene.startup", func(*Context, SceneStartup) error { starts++; return nil })
	}
	r := newFixture(t, plan, registry, Options{})
	main, err := r.Manager().StartEntry()
	must(t, err)
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "game.json", Retain: true}))
	must(t, r.Dispatch())
	game := r.Manager().Active()
	must(t, r.Manager().Switch(SceneSwitchRequest{Scene: "main.json"}))
	must(t, r.Dispatch())
	if r.Manager().Active() != game || game.State() != SceneActive || main.State() != SceneSuspended || starts != 1 || resumes != 1 {
		t.Fatal("failed retained-scene resumption committed a switch or repeated startup")
	}
}
