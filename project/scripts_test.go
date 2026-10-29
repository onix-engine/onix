package project

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	sceneapi "github.com/diiyw/onix/scene"
)

func TestSceneScriptInputErrorRetainsLaterMessages(t *testing.T) {
	for _, route := range []string{"direct", "bubble", "global"} {
		t.Run(route, func(t *testing.T) {
			world := ecs.NewWorld()
			owner := world.Spawn(ecs.NewBundle(0))
			child := world.Spawn(ecs.NewBundle(0))
			failure := errors.New("input failed")
			calls, delivered := 0, uint64(0)
			binding := scriptInputBinding{owner: owner, name: "Input", handle: func(_ *ScriptContext, event *sceneapi.InputEventContext) error {
				calls++
				if calls == 1 {
					return failure
				}
				delivered = event.Event.Seq
				return nil
			}}
			scene := &SceneInstance{World: world, inputBindings: map[ecs.Entity][]scriptInputBinding{owner: {binding}}}
			event := sceneapi.InputEvent{Entity: owner, Kind: sceneapi.InputEventTap}
			switch route {
			case "bubble":
				event.Entity, event.Path = child, []ecs.Entity{child, owner}
			case "global":
				event.Entity, event.Kind = 0, sceneapi.InputEventKeyDown
				scene.globalInputBindings.add(sceneapi.InputMaskKeyboard, binding)
			}
			event.Seq = 1
			world.WriteMessage(event)
			world.AdvanceMessages()
			event.Seq = 2
			world.WriteMessage(event)
			context := &ecs.SystemContext{World: world, Commands: world.Commands()}
			var reader ecs.MessageReader[sceneapi.InputEvent]
			if err := scene.DispatchInputScripts(context, &reader); !errors.Is(err, failure) || calls != 1 {
				t.Fatalf("failed dispatch: calls=%d error=%v", calls, err)
			}
			if err := scene.DispatchInputScripts(context, &reader); err != nil || calls != 2 || delivered != 2 {
				t.Fatalf("resumed dispatch: calls=%d message=%d error=%v", calls, delivered, err)
			}
			if err := scene.DispatchInputScripts(context, &reader); err != nil || calls != 2 {
				t.Fatalf("drained dispatch: calls=%d error=%v", calls, err)
			}
		})
	}
}

type testSpriteScript struct {
	Alpha   float32
	Updated bool
}

type testInputScript struct {
	PointerEvents int
	GlobalEvents  int
	Current       ecs.Entity
	Key           input.KeyCode
	Text          string
	File          string
}

func (script *testInputScript) OnInput(_ *ScriptContext, event *sceneapi.InputEventContext) error {
	switch event.Event.Kind {
	case sceneapi.InputEventTap:
		script.PointerEvents++
		script.Current = event.Current
		event.Consume()
	case sceneapi.InputEventKeyDown:
		script.GlobalEvents++
		script.Current = event.Current
		script.Key = event.Event.Key
		event.Consume()
	case sceneapi.InputEventTextInput:
		script.GlobalEvents++
		script.Current = event.Current
		script.Text = event.Event.Text
	case sceneapi.InputEventFilesDropped:
		script.GlobalEvents++
		script.Current = event.Current
		script.File = event.Event.File
	}
	return nil
}

func (script *testSpriteScript) Install(context *ScriptContext) error {
	sprite := context.World.GetMut[sceneapi.Sprite2D](context.Entity)
	if sprite != nil {
		sprite.Alpha = script.Alpha
	}
	return nil
}

func (script *testSpriteScript) Update(_ *ScriptContext) error {
	script.Updated = true
	return nil
}

func (script *testSpriteScript) Destroy(_ *ScriptContext) error {
	script.Updated = false
	return nil
}

func TestSceneScriptsBindUpdateAndDestroy(t *testing.T) {
	registry := NewScriptRegistry(Script[testSpriteScript]("TestSprite", Hooks[testSpriteScript]{
		Install: (*testSpriteScript).Install,
		Update:  (*testSpriteScript).Update,
		Destroy: (*testSpriteScript).Destroy,
	}))
	raw, _ := json.Marshal(float32(0.25))
	scene := &SceneInstance{
		World:          ecs.NewWorld(),
		nodes:          map[ecs.Entity]sceneSpawnNode{},
		Entities:       map[string]ecs.Entity{},
		EntityIDs:      map[ecs.Entity]string{},
		scriptBindings: map[ecs.Entity][]ScriptBinding{},
	}
	bundle := ecs.NewBundle(1)
	bundle.Put(sceneapi.Sprite2D{Alpha: 1})
	entity := scene.World.Spawn(bundle)
	scene.Entities["sprite"] = entity
	scene.EntityIDs[entity] = "sprite"
	scene.nodes[entity] = sceneSpawnNode{Spec: SceneNodeSpec{ID: "sprite", Scripts: []ScriptComponentSpec{{Name: "TestSprite", Values: map[string]json.RawMessage{"Alpha": raw}}}}}
	if err := scene.SetScriptRegistry(registry); err != nil {
		t.Fatalf("bind scripts: %v", err)
	}
	sprite, ok := scene.World.Get[sceneapi.Sprite2D](entity)
	if !ok || sprite.Alpha != 0.25 {
		t.Fatalf("script install did not update sprite: %+v, %v", sprite, ok)
	}
	if err := scene.UpdateScripts(&ecs.SystemContext{World: scene.World, Commands: scene.World.Commands()}); err != nil {
		t.Fatalf("update scripts: %v", err)
	}
	value, ok := scene.World.Get[testSpriteScript](entity)
	if !ok || !value.Updated {
		t.Fatalf("script update did not run: %+v, %v", value, ok)
	}
	if removed := scene.RemoveNodes("sprite"); removed != 1 || scene.World.IsAlive(entity) {
		t.Fatalf("script entity was not removed: removed=%d alive=%v", removed, scene.World.IsAlive(entity))
	}
}

func TestSceneScriptsRouteInputEventsToBoundTargets(t *testing.T) {
	world := ecs.NewWorld()
	inputState := input.NewState()
	world.InsertResource(inputState)
	scene := &SceneInstance{
		World:          world,
		nodes:          map[ecs.Entity]sceneSpawnNode{},
		Entities:       map[string]ecs.Entity{},
		EntityIDs:      map[ecs.Entity]string{},
		scriptBindings: map[ecs.Entity][]ScriptBinding{},
		inputBindings:  map[ecs.Entity][]scriptInputBinding{},
	}
	owner := world.Spawn(ecs.NewBundle(0))
	target := world.Spawn(ecs.NewBundle(0))
	other := world.Spawn(ecs.NewBundle(0))
	scene.Entities["owner"], scene.EntityIDs[owner] = owner, "owner"
	scene.Entities["target"], scene.EntityIDs[target] = target, "target"
	scene.Entities["other"], scene.EntityIDs[other] = other, "other"
	scene.Roots = []ecs.Entity{owner, target, other}
	scene.nodes[owner] = sceneSpawnNode{Spec: SceneNodeSpec{ID: "owner", Scripts: []ScriptComponentSpec{{Name: "Input"}}}}
	scene.nodes[target] = sceneSpawnNode{Spec: SceneNodeSpec{ID: "target"}}
	scene.nodes[other] = sceneSpawnNode{Spec: SceneNodeSpec{ID: "other"}}

	registry := NewScriptRegistry(Script[testInputScript]("Input", Hooks[testInputScript]{
		Install: func(_ *testInputScript, context *ScriptContext) error { return context.ListenInput(target) },
		Input:   (*testInputScript).OnInput,
	}))
	if err := scene.SetScriptRegistry(registry); err != nil {
		t.Fatal(err)
	}
	if len(scene.inputBindings[owner]) != 1 || len(scene.inputBindings[target]) != 1 ||
		len(scene.globalInputBindings.keyboard) != 1 || len(scene.globalInputBindings.text) != 1 || len(scene.globalInputBindings.files) != 1 {
		t.Fatalf("input bindings = owner:%d target:%d", len(scene.inputBindings[owner]), len(scene.inputBindings[target]))
	}

	var reader ecs.MessageReader[sceneapi.InputEvent]
	context := &ecs.SystemContext{World: world, Commands: world.Commands()}
	world.WriteMessage(sceneapi.InputEvent{Entity: other, Kind: sceneapi.InputEventTap})
	world.WriteMessage(sceneapi.InputEvent{Entity: target, Kind: sceneapi.InputEventTap})
	if err := scene.DispatchInputScripts(context, &reader); err != nil {
		t.Fatal(err)
	}
	script, _ := world.Get[testInputScript](owner)
	if script.PointerEvents != 1 || script.Current != target {
		t.Fatalf("input script = %+v", script)
	}
	var observed ecs.MessageReader[sceneapi.InputEvent]
	events := observed.Read(world)
	if len(events) != 2 || !events[1].Handled {
		t.Fatalf("routed input events = %+v", events)
	}

	world.WriteMessage(sceneapi.InputEvent{Entity: other, Path: []ecs.Entity{other, owner}, Kind: sceneapi.InputEventTap})
	if err := scene.DispatchInputScripts(context, &reader); err != nil {
		t.Fatal(err)
	}
	script, _ = world.Get[testInputScript](owner)
	if script.PointerEvents != 2 || script.Current != owner {
		t.Fatalf("bubbled input script = %+v", script)
	}
	if err := world.Insert(other, sceneapi.InputTarget2D{}); err != nil {
		t.Fatal(err)
	}
	world.WriteMessage(sceneapi.InputEvent{Entity: other, Path: []ecs.Entity{other, owner}, Kind: sceneapi.InputEventTap})
	if err := scene.DispatchInputScripts(context, &reader); err != nil {
		t.Fatal(err)
	}
	script, _ = world.Get[testInputScript](owner)
	if script.PointerEvents != 2 {
		t.Fatalf("stop-filtered pointer reached parent: %+v", script)
	}
	if err := world.Insert(target, sceneapi.InputTarget2D{InputFilter: sceneapi.InputFilterIgnore}); err != nil {
		t.Fatal(err)
	}
	world.WriteMessage(sceneapi.InputEvent{Entity: target, Path: []ecs.Entity{target, owner}, Kind: sceneapi.InputEventTap})
	if err := scene.DispatchInputScripts(context, &reader); err != nil {
		t.Fatal(err)
	}
	script, _ = world.Get[testInputScript](owner)
	if script.PointerEvents != 3 || script.Current != owner {
		t.Fatalf("ignore-filtered target did not bubble to parent: %+v", script)
	}

	world.WriteMessage(sceneapi.InputEvent{Seq: 1, Kind: sceneapi.InputEventKeyDown, Key: input.KeySpace})
	world.WriteMessage(sceneapi.InputEvent{Seq: 2, Kind: sceneapi.InputEventTextInput, Text: "Onix"})
	world.WriteMessage(sceneapi.InputEvent{Seq: 3, Kind: sceneapi.InputEventFilesDropped, File: "/tmp/onix.txt"})
	if err := scene.DispatchInputScripts(context, &reader); err != nil {
		t.Fatal(err)
	}
	script, _ = world.Get[testInputScript](owner)
	if script.GlobalEvents != 3 || script.Current != owner || script.Key != input.KeySpace || script.Text != "Onix" || script.File != "/tmp/onix.txt" {
		t.Fatalf("global input script = %+v", script)
	}
	if !inputState.EventConsumed(1) {
		t.Fatal("consumed global input was not propagated to input state")
	}

	if removed := scene.RemoveNodes("target"); removed != 1 {
		t.Fatalf("removed input target count = %d", removed)
	}
	if len(scene.inputBindings[target]) != 0 || len(scene.inputBindings[owner]) != 1 {
		t.Fatalf("bindings remained after target removal: %+v", scene.inputBindings)
	}
	if removed := scene.RemoveNodes("owner"); removed != 1 || len(scene.inputBindings) != 0 ||
		len(scene.globalInputBindings.keyboard) != 0 || len(scene.globalInputBindings.text) != 0 || len(scene.globalInputBindings.files) != 0 {
		t.Fatalf("bindings remained after owner removal: %+v", scene.inputBindings)
	}
}

func TestSceneScriptInputMasksFilterCategories(t *testing.T) {
	world := ecs.NewWorld()
	owner := world.Spawn(ecs.NewBundle(0))
	calls := [4]int{}
	bindings := []ScriptBinding{
		{Name: "Pointer", Bind: func(*ScriptContext, ScriptComponentSpec) error { return nil }, InputMask: sceneapi.InputMaskPointer},
		{Name: "Keyboard", Bind: func(*ScriptContext, ScriptComponentSpec) error { return nil }, InputMask: sceneapi.InputMaskKeyboard},
		{Name: "Text", Bind: func(*ScriptContext, ScriptComponentSpec) error { return nil }, InputMask: sceneapi.InputMaskText},
		{Name: "Files", Bind: func(*ScriptContext, ScriptComponentSpec) error { return nil }, InputMask: sceneapi.InputMaskFiles},
	}
	for index := range bindings {
		category := index
		bindings[index].Input = func(*ScriptContext, *sceneapi.InputEventContext) error {
			calls[category]++
			return nil
		}
	}
	scene := &SceneInstance{
		World: world,
		nodes: map[ecs.Entity]sceneSpawnNode{owner: {Spec: SceneNodeSpec{ID: "owner", Scripts: []ScriptComponentSpec{
			{Name: "Pointer"}, {Name: "Keyboard"}, {Name: "Text"}, {Name: "Files"},
		}}}},
		Entities:       map[string]ecs.Entity{"owner": owner},
		EntityIDs:      map[ecs.Entity]string{owner: "owner"},
		scriptBindings: map[ecs.Entity][]ScriptBinding{},
		inputBindings:  map[ecs.Entity][]scriptInputBinding{},
	}
	if err := scene.SetScriptRegistry(NewScriptRegistry(bindings...)); err != nil {
		t.Fatal(err)
	}
	if len(scene.inputBindings[owner]) != 1 || len(scene.globalInputBindings.keyboard) != 1 ||
		len(scene.globalInputBindings.text) != 1 || len(scene.globalInputBindings.files) != 1 {
		t.Fatalf("categorized input bindings = pointer:%d keyboard:%d text:%d files:%d",
			len(scene.inputBindings[owner]), len(scene.globalInputBindings.keyboard),
			len(scene.globalInputBindings.text), len(scene.globalInputBindings.files))
	}
	var reader ecs.MessageReader[sceneapi.InputEvent]
	events := [...]sceneapi.InputEvent{
		{Entity: owner, Kind: sceneapi.InputEventTap},
		{Kind: sceneapi.InputEventKeyDown},
		{Kind: sceneapi.InputEventTextInput},
		{Kind: sceneapi.InputEventFilesDropped},
	}
	for _, event := range events {
		world.WriteMessage(event)
	}
	if err := scene.DispatchInputScripts(&ecs.SystemContext{World: world, Commands: world.Commands()}, &reader); err != nil {
		t.Fatal(err)
	}
	if calls != [4]int{1, 1, 1, 1} {
		t.Fatalf("categorized input calls = %v", calls)
	}
}

func TestSceneScriptInputDispatchDoesNotAllocateAfterWarmup(t *testing.T) {
	world := ecs.NewWorld()
	entity := world.Spawn(ecs.NewBundle(0))
	binding := ScriptBinding{
		Name: "Input",
		Input: func(*ScriptContext, *sceneapi.InputEventContext) error {
			return nil
		},
	}
	scene := &SceneInstance{
		World: world,
		inputBindings: map[ecs.Entity][]scriptInputBinding{
			entity: {{owner: entity, name: binding.Name, handle: binding.Input}},
		},
		globalInputBindings: scriptGlobalInputBindings{
			keyboard: []scriptInputBinding{{owner: entity, name: binding.Name, handle: binding.Input}},
		},
	}
	context := &ecs.SystemContext{World: world, Commands: world.Commands()}
	var reader ecs.MessageReader[sceneapi.InputEvent]
	run := func() {
		world.WriteMessage(sceneapi.InputEvent{Entity: entity, Kind: sceneapi.InputEventTap})
		world.WriteMessage(sceneapi.InputEvent{Kind: sceneapi.InputEventKeyDown, Key: input.KeySpace})
		if err := scene.DispatchInputScripts(context, &reader); err != nil {
			panic(err)
		}
		world.AdvanceMessages()
	}
	run()
	run()
	if allocations := testing.AllocsPerRun(100, run); allocations != 0 {
		t.Fatalf("pointer dispatch allocations = %v, want 0", allocations)
	}
}

func BenchmarkSceneScriptGlobalInputMask10000(b *testing.B) {
	const bindingCount = 10_000
	benchmark := func(b *testing.B, mask sceneapi.InputMask) {
		world := ecs.NewWorld()
		scene := &SceneInstance{World: world}
		handle := func(*ScriptContext, *sceneapi.InputEventContext) error { return nil }
		for range bindingCount {
			owner := world.Spawn(ecs.NewBundle(0))
			scene.globalInputBindings.add(mask, scriptInputBinding{owner: owner, name: "Input", handle: handle})
		}
		context := &ecs.SystemContext{World: world, Commands: world.Commands()}
		var reader ecs.MessageReader[sceneapi.InputEvent]
		run := func() {
			world.WriteMessage(sceneapi.InputEvent{Kind: sceneapi.InputEventKeyDown})
			if err := scene.DispatchInputScripts(context, &reader); err != nil {
				b.Fatal(err)
			}
			world.AdvanceMessages()
		}
		run()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			run()
		}
	}
	b.Run("PointerFiltered", func(b *testing.B) { benchmark(b, sceneapi.InputMaskPointer) })
	b.Run("KeyboardSubscribers", func(b *testing.B) { benchmark(b, sceneapi.InputMaskKeyboard) })
}
