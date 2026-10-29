package onix

import (
	"reflect"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
)

func TestSetScriptRegistryInstallsInputDispatchBeforeFixedUpdates(t *testing.T) {
	app := NewApp()
	registry := project.NewScriptRegistry(project.ScriptBinding{
		Name: "Input",
		Bind: func(*project.ScriptContext, project.ScriptComponentSpec) error {
			return nil
		},
		Input: func(*project.ScriptContext, *scene.InputEventContext) error {
			return nil
		},
	})
	if err := app.SetScriptRegistry(registry); err != nil {
		t.Fatal(err)
	}
	if err := app.SetScriptRegistry(registry); err != nil {
		t.Fatal(err)
	}
	globalOrder, err := app.Schedule(StageInputGlobal).Order()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(globalOrder, []string{"onix_script_global_input"}) {
		t.Fatalf("global input script systems = %v", globalOrder)
	}
	pointerOrder, err := app.Schedule(StageBeforeFixedUpdateLoop).Order()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pointerOrder, []string{"onix_script_pointer_input"}) {
		t.Fatalf("pointer script systems = %v", pointerOrder)
	}
	updateOrder, err := app.Schedule(StageUpdate).Order()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updateOrder, []string{"onix_script_components"}) {
		t.Fatalf("update script systems = %v", updateOrder)
	}
}

func TestSetScriptRegistryInstallsOnlySubscribedInputStages(t *testing.T) {
	app := NewApp()
	registry := project.NewScriptRegistry(project.ScriptBinding{
		Name:      "Pointer",
		Bind:      func(*project.ScriptContext, project.ScriptComponentSpec) error { return nil },
		Input:     func(*project.ScriptContext, *scene.InputEventContext) error { return nil },
		InputMask: scene.InputMaskPointer,
	})
	if err := app.SetScriptRegistry(registry); err != nil {
		t.Fatal(err)
	}
	globalOrder, err := app.Schedule(StageInputGlobal).Order()
	if err != nil {
		t.Fatal(err)
	}
	if len(globalOrder) != 0 {
		t.Fatalf("pointer-only registry installed global input stage: %v", globalOrder)
	}
	pointerOrder, err := app.Schedule(StageBeforeFixedUpdateLoop).Order()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pointerOrder, []string{"onix_script_pointer_input"}) {
		t.Fatalf("pointer input script systems = %v", pointerOrder)
	}
}

func TestRouteMouseDownKeepsWindowDragOutOfInputState(t *testing.T) {
	state := input.NewState()
	if !routeMouseDown(state, input.MouseLeft, 120, 16, func(x, y float64) bool { return true }) {
		t.Fatal("expected window drag to start")
	}
	if pointer := state.Pointer(); pointer.Down || pointer.Pressed {
		t.Fatalf("window drag leaked into pointer state: %+v", pointer)
	}

	if routeMouseDown(state, input.MouseLeft, 40, 80, func(x, y float64) bool { return false }) {
		t.Fatal("unexpected window drag")
	}
	if pointer := state.Pointer(); !pointer.Down || !pointer.Pressed {
		t.Fatalf("client click did not reach pointer state: %+v", pointer)
	}
}

func TestProcessInputEventsDerivesStateAndWritesMessages(t *testing.T) {
	world := ecs.NewWorld()
	state := input.NewState()
	world.InsertResource(state)
	state.Enqueue(input.Event{Kind: input.EventKeyDown, Key: input.KeyA})
	state.Enqueue(input.Event{Kind: input.EventTextInput, Text: "hello"})

	if err := processInputEvents(&ecs.SystemContext{World: world}); err != nil {
		t.Fatalf("process input events: %v", err)
	}
	edits := state.TextEdits()
	if !state.KeyPressed(input.KeyA) || len(edits) != 1 || edits[0].Text != "hello" {
		t.Fatalf("input state was not derived: key=%v edits=%+v", state.KeyPressed(input.KeyA), edits)
	}
	events := (&ecs.MessageReader[input.Event]{}).Read(world)
	if len(events) != 2 || events[0].Kind != input.EventKeyDown || events[1].Kind != input.EventTextInput {
		t.Fatalf("unexpected input messages: %#v", events)
	}
	unified := (&ecs.MessageReader[scene.InputEvent]{}).Read(world)
	if len(unified) != 2 || unified[0].Kind != scene.InputEventKeyDown || unified[0].Key != input.KeyA ||
		unified[1].Kind != scene.InputEventTextInput || unified[1].Text != "hello" {
		t.Fatalf("unexpected unified input messages: %#v", unified)
	}
}

func TestProcessInputEventsPublishesFileDrops(t *testing.T) {
	world := ecs.NewWorld()
	state := input.NewState()
	world.InsertResource(state)
	state.Enqueue(input.Event{Kind: input.EventFilesDropped, X: 8, Y: 12, File: "/tmp/example.txt"})

	if err := processInputEvents(&ecs.SystemContext{World: world}); err != nil {
		t.Fatalf("process input events: %v", err)
	}
	events := (&ecs.MessageReader[input.Event]{}).Read(world)
	if len(events) != 1 || events[0].Kind != input.EventFilesDropped || events[0].File != "/tmp/example.txt" || events[0].X != 8 || events[0].Y != 12 {
		t.Fatalf("unexpected file drop message: %#v", events)
	}
}

func TestInputHandlerConsumesLaterPhases(t *testing.T) {
	app := NewApp()
	state := input.NewState()
	app.World.InsertResource(state)
	phases := []input.Phase{}
	app.AddInputHandler(input.PhaseGlobal, func(dispatch *input.Dispatch) {
		phases = append(phases, dispatch.Phase)
		dispatch.Consume()
	})
	app.AddInputHandler(input.PhaseGUI, func(dispatch *input.Dispatch) {
		phases = append(phases, dispatch.Phase)
	})
	state.Enqueue(input.Event{Kind: input.EventKeyDown, Key: input.KeyEscape})
	app.ensureInputSystem()
	if err := app.schedule(StageInputPreprocess).Run(app.World); err != nil {
		t.Fatalf("process input handlers: %v", err)
	}
	if len(phases) != 1 || phases[0] != input.PhaseGlobal {
		t.Fatalf("unexpected input phases: %#v", phases)
	}
	event := (&ecs.MessageReader[input.Event]{}).Read(app.World)[0]
	if !state.EventConsumed(event.Seq) {
		t.Fatal("handled input event was not recorded")
	}
}

func TestAppFrameRunsStartupOnceAndOrderedStages(t *testing.T) {
	restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(320, 240))
	defer restore()

	app := NewApp()

	graphics.BindFrame(app.World, graphics.CurrentContext())
	app.SetTargetFPS(DefaultTargetTPS)
	now := time.Unix(0, 0)
	app.now = func() time.Time { return now }
	log := []string{}
	app.World.InsertResource(&log)

	appendLog := func(name string) ecs.System {
		return func(context *ecs.SystemContext) error {
			world := context.World
			out, _ := world.GetResource[*[]string]()
			*out = append(*out, name)
			return nil
		}
	}

	if err := app.AddSystem(StageStartup, ecs.SystemConfig{Name: "boot", System: appendLog("startup")}); err != nil {
		t.Fatalf("add startup: %v", err)
	}
	if err := app.AddSystem(StagePreUpdate, ecs.SystemConfig{Name: "pre", System: appendLog("pre")}); err != nil {
		t.Fatalf("add pre: %v", err)
	}
	if err := app.AddSystem(StageUpdate, ecs.SystemConfig{Name: "update", System: appendLog("update")}); err != nil {
		t.Fatalf("add update: %v", err)
	}
	if err := app.AddSystem(StagePostUpdate, ecs.SystemConfig{Name: "post", System: appendLog("post")}); err != nil {
		t.Fatalf("add post: %v", err)
	}
	if err := app.AddSystem(StageRender, ecs.SystemConfig{Name: "render", System: appendLog("render")}); err != nil {
		t.Fatalf("add render: %v", err)
	}

	if err := app.Frame(); err != nil {
		t.Fatalf("frame 1: %v", err)
	}
	now = now.Add(time.Second / DefaultTargetTPS)
	if err := app.Frame(); err != nil {
		t.Fatalf("frame 2: %v", err)
	}

	got, _ := app.World.GetResource[*[]string]()
	want := []string{
		"startup",
		"pre", "update", "post", "render",
		"pre", "update", "post", "render",
	}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("unexpected app stage order: got %v want %v", *got, want)
	}
}

func TestAppFrameDoesNotAllocateAfterWarmup(t *testing.T) {
	app := NewApp()
	app.SetTargetFPS(DefaultTargetTPS)
	if err := app.InstallSystems[UITextInput](); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(0, 0)
	app.now = func() time.Time { return now }
	if err := app.Frame(); err != nil {
		t.Fatal(err)
	}

	if allocations := testing.AllocsPerRun(100, func() {
		now = now.Add(time.Second / DefaultTargetTPS)
		if err := app.Frame(); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("app frame allocated %.0f objects, want 0", allocations)
	}
}

func TestUIHTMLInstallsTextInputRuntime(t *testing.T) {
	app := NewApp()
	if err := app.InstallSystems[UIHTML](); err != nil {
		t.Fatal(err)
	}
	if !app.hasSystemSet(systemSetUIHTML) || !app.hasSystemSet(systemSetUITextInput) {
		t.Fatal("UI HTML did not install its text-input dependency")
	}
}

func TestUIMarkdownInstallsHTMLRuntime(t *testing.T) {
	app := NewApp()
	if err := app.InstallSystems[UIMarkdown[testMarkdownRuntime]](); err != nil {
		t.Fatal(err)
	}
	if !app.hasSystemSet(systemSetUIMarkdown) || !app.hasSystemSet(systemSetUIHTML) || !app.hasSystemSet(systemSetUITextInput) {
		t.Fatal("UI Markdown did not install its HTML runtime dependency")
	}
}

type testMarkdownRuntime struct{}

func (testMarkdownRuntime) InstallMarkdown(app *App) error {
	return app.InstallSystems[UIHTML]()
}

func TestWebViewInstallDefersBackendUntilWindowContext(t *testing.T) {
	app := NewApp()
	if err := app.InstallSystems[WebView](); err != nil {
		t.Fatal(err)
	}
	if !app.hasSystemSet(systemSetWebView) || app.webViewBeforeResize == nil || app.webViewAfterResize == nil {
		t.Fatal("WebView system did not register its optional lifecycle hooks")
	}
	if _, ok := app.World.GetResource[*scene.WebViewRuntime](); ok {
		t.Fatal("WebView backend was initialized before the window context")
	}
	if err := app.InstallSystems[WebView](); err != nil {
		t.Fatal(err)
	}
}

func TestAppFrameRunsFixedLoopBetweenPreUpdateAndUpdate(t *testing.T) {
	restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(320, 240))
	defer restore()

	app := NewApp()

	graphics.BindFrame(app.World, graphics.CurrentContext())
	app.SetTargetFPS(DefaultTargetTPS)
	now := time.Unix(0, 0)
	app.now = func() time.Time { return now }
	log := []string{}
	app.World.InsertResource(&log)

	appendLog := func(name string) ecs.System {
		return func(context *ecs.SystemContext) error {
			world := context.World
			out, _ := world.GetResource[*[]string]()
			*out = append(*out, name)
			return nil
		}
	}

	systems := []struct {
		stage Stage
		name  string
	}{
		{StagePreUpdate, "pre"},
		{StageBeforeFixedUpdateLoop, "before_fixed"},
		{StageFixedFirst, "fixed_first"},
		{StageFixedPreUpdate, "fixed_pre"},
		{StageFixedUpdate, "fixed_update"},
		{StageFixedPostUpdate, "fixed_post"},
		{StageFixedLast, "fixed_last"},
		{StageAfterFixedUpdateLoop, "after_fixed"},
		{StageUpdate, "update"},
		{StagePostUpdate, "post"},
		{StageRender, "render"},
	}
	for _, system := range systems {
		if err := app.AddSystem(system.stage, ecs.SystemConfig{Name: system.name, System: appendLog(system.name)}); err != nil {
			t.Fatalf("add %s: %v", system.name, err)
		}
	}

	if err := app.Frame(); err != nil {
		t.Fatalf("frame 1: %v", err)
	}
	now = now.Add(2 * app.fixedTime.Timestep)
	if err := app.Frame(); err != nil {
		t.Fatalf("frame 2: %v", err)
	}

	got, _ := app.World.GetResource[*[]string]()
	want := []string{
		"pre", "before_fixed", "after_fixed", "update", "post", "render",
		"pre", "before_fixed",
		"fixed_first", "fixed_pre", "fixed_update", "fixed_post", "fixed_last",
		"fixed_first", "fixed_pre", "fixed_update", "fixed_post", "fixed_last",
		"after_fixed", "update", "post", "render",
	}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("unexpected fixed loop order: got %v want %v", *got, want)
	}
}

func TestAppFixedUpdateUsesFixedTimeAndUpdateUsesFrameTime(t *testing.T) {
	restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(320, 240))
	defer restore()

	app := NewApp()

	graphics.BindFrame(app.World, graphics.CurrentContext())
	app.SetTargetFPS(DefaultTargetTPS)
	now := time.Unix(0, 0)
	app.now = func() time.Time { return now }
	fixedDeltas := []time.Duration{}
	updateDeltas := []time.Duration{}

	if err := app.AddSystem(StageFixedUpdate, ecs.SystemConfig{
		Name: "fixed",
		System: func(context *ecs.SystemContext) error {
			world := context.World
			clock, _ := world.GetResource[*scene.Time]()
			fixedDeltas = append(fixedDeltas, clock.Delta)
			return nil
		},
	}); err != nil {
		t.Fatalf("add fixed: %v", err)
	}
	if err := app.AddSystem(StageUpdate, ecs.SystemConfig{
		Name: "update",
		System: func(context *ecs.SystemContext) error {
			world := context.World
			clock, _ := world.GetResource[*scene.Time]()
			updateDeltas = append(updateDeltas, clock.Delta)
			return nil
		},
	}); err != nil {
		t.Fatalf("add update: %v", err)
	}

	if err := app.Frame(); err != nil {
		t.Fatalf("frame 1: %v", err)
	}
	now = now.Add(2 * app.fixedTime.Timestep)
	if err := app.Frame(); err != nil {
		t.Fatalf("frame 2: %v", err)
	}

	wantFixed := []time.Duration{app.fixedTime.Timestep, app.fixedTime.Timestep}
	if !reflect.DeepEqual(fixedDeltas, wantFixed) {
		t.Fatalf("unexpected fixed deltas: got %v want %v", fixedDeltas, wantFixed)
	}
	wantUpdate := []time.Duration{0, 2 * app.fixedTime.Timestep}
	if !reflect.DeepEqual(updateDeltas, wantUpdate) {
		t.Fatalf("unexpected update deltas: got %v want %v", updateDeltas, wantUpdate)
	}
}

func TestAppFrameRunsFixedUpdateIndependentOfRender(t *testing.T) {
	restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(320, 240))
	defer restore()

	app := NewApp()

	graphics.BindFrame(app.World, graphics.CurrentContext())
	app.SetTargetFPS(30)
	app.SetTargetTPS(60)
	now := time.Unix(0, 0)
	app.now = func() time.Time { return now }
	log := []string{}
	app.World.InsertResource(&log)

	appendLog := func(name string) ecs.System {
		return func(context *ecs.SystemContext) error {
			world := context.World
			out, _ := world.GetResource[*[]string]()
			*out = append(*out, name)
			return nil
		}
	}

	if err := app.AddSystem(StageFixedUpdate, ecs.SystemConfig{Name: "fixed_update", System: appendLog("fixed_update")}); err != nil {
		t.Fatalf("add fixed update: %v", err)
	}
	if err := app.AddSystem(StageRender, ecs.SystemConfig{Name: "render", System: appendLog("render")}); err != nil {
		t.Fatalf("add render: %v", err)
	}

	if err := app.Frame(); err != nil {
		t.Fatalf("frame 1: %v", err)
	}
	now = now.Add(app.fixedTime.Timestep)
	if err := app.Frame(); err != nil {
		t.Fatalf("frame before render interval: %v", err)
	}

	got, _ := app.World.GetResource[*[]string]()
	want := []string{"render", "fixed_update"}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("unexpected fixed/render order: got %v want %v", *got, want)
	}
}

func TestAppFrameRunsRenderIndependentOfFixedUpdate(t *testing.T) {
	restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(320, 240))
	defer restore()

	app := NewApp()

	graphics.BindFrame(app.World, graphics.CurrentContext())
	app.SetTargetFPS(120)
	app.SetTargetTPS(20)
	now := time.Unix(0, 0)
	app.now = func() time.Time { return now }
	log := []string{}
	app.World.InsertResource(&log)

	appendLog := func(name string) ecs.System {
		return func(context *ecs.SystemContext) error {
			world := context.World
			out, _ := world.GetResource[*[]string]()
			*out = append(*out, name)
			return nil
		}
	}

	if err := app.AddSystem(StageFixedUpdate, ecs.SystemConfig{Name: "fixed_update", System: appendLog("fixed_update")}); err != nil {
		t.Fatalf("add fixed update: %v", err)
	}
	if err := app.AddSystem(StageRender, ecs.SystemConfig{Name: "render", System: appendLog("render")}); err != nil {
		t.Fatalf("add render: %v", err)
	}

	if err := app.Frame(); err != nil {
		t.Fatalf("frame 1: %v", err)
	}
	now = now.Add(app.renderPacer.interval)
	if err := app.Frame(); err != nil {
		t.Fatalf("frame before fixed interval: %v", err)
	}

	got, _ := app.World.GetResource[*[]string]()
	want := []string{"render", "render"}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("unexpected fixed/render order: got %v want %v", *got, want)
	}
}

func TestAppLoopRateUsesRenderRate(t *testing.T) {
	app := NewApp()
	app.SetTargetFPS(30)
	app.SetTargetTPS(60)
	if got := app.loopRate(); got != 30 {
		t.Fatalf("expected loop rate 30, got %d", got)
	}

	app.SetTargetFPS(120)
	app.SetTargetTPS(20)
	if got := app.loopRate(); got != 120 {
		t.Fatalf("expected loop rate 120, got %d", got)
	}
}
