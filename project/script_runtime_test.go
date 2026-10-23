package project

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	sceneapi "github.com/diiyw/onix/scene"
)

type runtimeScriptA struct{ Count int }
type runtimeScriptB struct{ Count int }

func runtimeTestScene(world *ecs.World, names ...string) *SceneInstance {
	scene := &SceneInstance{World: world, nodes: map[ecs.Entity]sceneSpawnNode{}, Entities: map[string]ecs.Entity{}, EntityIDs: map[ecs.Entity]string{}}
	for _, id := range names {
		entity := world.Spawn(ecs.NewBundle(0))
		scene.Entities[id], scene.EntityIDs[entity] = entity, id
		scene.nodes[entity] = sceneSpawnNode{Spec: SceneNodeSpec{ID: id, Scripts: []ScriptComponentSpec{{Name: "B"}, {Name: "A"}}}}
	}
	return scene
}

func TestGenericScriptsIsolateScenesAndExecuteInRegistryOrder(t *testing.T) {
	world := ecs.NewWorld()
	var calls []string
	registry := NewScriptRegistry(
		Script[runtimeScriptA]("A", Hooks[runtimeScriptA]{Update: func(value *runtimeScriptA, ctx *ScriptContext) error {
			value.Count++
			calls = append(calls, "A:"+ctx.Scene.ID(ctx.Entity))
			return nil
		}}),
		Script[runtimeScriptB]("B", Hooks[runtimeScriptB]{Update: func(value *runtimeScriptB, ctx *ScriptContext) error {
			value.Count++
			calls = append(calls, "B:"+ctx.Scene.ID(ctx.Entity))
			return nil
		}}),
	)
	first, second := runtimeTestScene(world, "one", "two"), runtimeTestScene(world, "other")
	for _, scene := range []*SceneInstance{first, second} {
		if err := scene.SetScriptRegistry(registry); err != nil {
			t.Fatal(err)
		}
	}
	context := &ecs.SystemContext{World: world, Commands: world.Commands()}
	if err := first.UpdateScripts(context); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, []string{"A:one", "A:two", "B:one", "B:two"}) {
		t.Fatalf("execution order = %v", calls)
	}
	if ecs.GetRef[runtimeScriptA](world, second.MustEntity("other")).Count != 0 {
		t.Fatal("another scene was updated")
	}
	calls = calls[:0]
	if removed := first.RemoveNodes("one"); removed != 1 {
		t.Fatalf("removed %d", removed)
	}
	if err := first.UpdateScripts(context); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, []string{"A:two", "B:two"}) {
		t.Fatalf("order after removal = %v", calls)
	}
	// Rebinding replaces existing components and rebuilds membership once.
	if err := first.SetScriptRegistry(registry); err != nil {
		t.Fatal(err)
	}
	if err := first.UpdateScripts(context); err != nil {
		t.Fatal(err)
	}
	if ecs.GetRef[runtimeScriptA](world, first.MustEntity("two")).Count != 1 {
		t.Fatal("rebind retained stale component state")
	}
}

func TestGenericScriptUpdateReusesContextsAndPreservesNestedCalls(t *testing.T) {
	world := ecs.NewWorld()
	scene := runtimeTestScene(world, "one", "two")
	context := &ecs.SystemContext{World: world, Commands: world.Commands()}
	nested := false
	registry := NewScriptRegistry(
		Script[runtimeScriptA]("A", Hooks[runtimeScriptA]{Update: func(value *runtimeScriptA, ctx *ScriptContext) error {
			entity := ctx.Entity
			value.Count++
			if !nested && entity == scene.MustEntity("one") {
				nested = true
				if err := scene.UpdateScripts(context); err != nil {
					return err
				}
				nested = false
				if ctx.Entity != entity {
					return errors.New("nested call overwrote outer context")
				}
			}
			return nil
		}}),
		Script[runtimeScriptB]("B", Hooks[runtimeScriptB]{}),
	)
	if err := scene.SetScriptRegistry(registry); err != nil {
		t.Fatal(err)
	}
	run := func() {
		if err := scene.UpdateScripts(context); err != nil {
			panic(err)
		}
	}
	run()
	if allocs := testing.AllocsPerRun(100, run); allocs != 0 {
		t.Fatalf("update allocations = %v, want 0", allocs)
	}
	world.Spawn(ecs.NewBundle(0))
}

func TestGenericScriptErrorsCleanUpAndStopTraversal(t *testing.T) {
	world := ecs.NewWorld()
	scene := runtimeTestScene(world, "one", "two")
	want := errors.New("hook failed")
	registry := NewScriptRegistry(
		Script[runtimeScriptA]("A", Hooks[runtimeScriptA]{Install: func(_ *runtimeScriptA, _ *ScriptContext) error { return want }}),
		Script[runtimeScriptB]("B", Hooks[runtimeScriptB]{}),
	)
	if err := scene.SetScriptRegistry(registry); !errors.Is(err, want) {
		t.Fatalf("install error = %v", err)
	}
	if ecs.Has[runtimeScriptA](world, scene.MustEntity("one")) || ecs.Has[runtimeScriptB](world, scene.MustEntity("one")) {
		t.Fatal("failed bind retained components")
	}
	registry = NewScriptRegistry(
		Script[runtimeScriptA]("A", Hooks[runtimeScriptA]{Update: func(_ *runtimeScriptA, _ *ScriptContext) error { return want }}),
		Script[runtimeScriptB]("B", Hooks[runtimeScriptB]{Update: func(value *runtimeScriptB, _ *ScriptContext) error { value.Count++; return nil }}),
	)
	if err := scene.SetScriptRegistry(registry); err != nil {
		t.Fatal(err)
	}
	err := scene.UpdateScripts(&ecs.SystemContext{World: world, Commands: world.Commands()})
	if !errors.Is(err, want) || !strings.Contains(err.Error(), `script "A"`) {
		t.Fatalf("update error = %v", err)
	}
	if ecs.GetRef[runtimeScriptB](world, scene.MustEntity("one")).Count != 0 {
		t.Fatal("update continued after error")
	}
	// An early return must release the query's structural guard.
	world.Spawn(ecs.NewBundle(0))
}

func TestGenericScriptFailedInstallRemovesInputRoutes(t *testing.T) {
	world := ecs.NewWorld()
	scene := runtimeTestScene(world, "one")
	target := world.Spawn(ecs.NewBundle(0))
	want := errors.New("install failed after listening")
	registry := NewScriptRegistry(
		Script[runtimeScriptB]("B", Hooks[runtimeScriptB]{}),
		Script[runtimeScriptA]("A", Hooks[runtimeScriptA]{
			Install: func(_ *runtimeScriptA, ctx *ScriptContext) error {
				if err := ctx.ListenInput(target); err != nil {
					return err
				}
				return want
			},
			Input: func(*runtimeScriptA, *ScriptContext, *sceneapi.InputEventContext) error { return nil },
		}),
	)
	if err := scene.SetScriptRegistry(registry); !errors.Is(err, want) {
		t.Fatalf("install error = %v", err)
	}
	if len(scene.inputBindings) != 0 || len(scene.globalInputBindings.keyboard) != 0 {
		t.Fatal("failed install retained input routes")
	}
}

func BenchmarkGenericScriptUpdate10000(b *testing.B) {
	world := ecs.NewWorld()
	scene := &SceneInstance{World: world, nodes: map[ecs.Entity]sceneSpawnNode{}}
	for range 10_000 {
		entity := world.Spawn(ecs.NewBundle(0))
		scene.nodes[entity] = sceneSpawnNode{Spec: SceneNodeSpec{Scripts: []ScriptComponentSpec{{Name: "A"}}}}
	}
	registry := NewScriptRegistry(Script[runtimeScriptA]("A", Hooks[runtimeScriptA]{
		Update: func(value *runtimeScriptA, _ *ScriptContext) error { value.Count++; return nil },
	}))
	if err := scene.SetScriptRegistry(registry); err != nil {
		b.Fatal(err)
	}
	context := &ecs.SystemContext{World: world, Commands: world.Commands()}
	if err := scene.UpdateScripts(context); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := scene.UpdateScripts(context); err != nil {
			b.Fatal(err)
		}
	}
}
