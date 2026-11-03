package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestHTMLDirtyComponentsWaitForScopeAndActivity(t *testing.T) {
	world := ecs.NewWorld()
	renders := make(map[ecs.Entity]int)
	definition := &htmlDefinitionRuntime{render: func(_ *ecs.World, entity ecs.Entity, _ *htmlComponentInstance) {
		renders[entity]++
	}}
	spawner := world.NewSpawner1[htmlComponentInstance]()
	first := spawner.Spawn(htmlComponentInstance{Definition: definition})
	second := spawner.Spawn(htmlComponentInstance{Definition: definition})
	removed := spawner.Spawn(htmlComponentInstance{Definition: definition})
	for _, entity := range []ecs.Entity{first, second, removed} {
		markHTMLComponentDirty(world, entity)
		markHTMLComponentDirty(world, entity)
	}
	world.Despawn(removed)
	replacement := spawner.Spawn(htmlComponentInstance{Definition: definition})
	var scope ecs.QueryScope
	scope.Reset([]ecs.Entity{first})
	world.SetQueryScope(&scope)
	system := SyncComponentsSystem("")
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if renders[first] != 1 || renders[second] != 0 || renders[replacement] != 0 || NeedsFrame(world) {
		t.Fatalf("scoped renders=%v pending=%v", renders, NeedsFrame(world))
	}
	scope.Reset([]ecs.Entity{first, second, replacement})
	if err := world.Insert(second, scene.Activity{Active: false}); err != nil {
		t.Fatal(err)
	}
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if renders[second] != 0 || NeedsFrame(world) {
		t.Fatal("inactive component ran or kept requesting frames")
	}
	world.GetMut[scene.Activity](second).Active = true
	if !NeedsFrame(world) {
		t.Fatal("reactivated component lost its dirty state")
	}
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}
	if renders[first] != 1 || renders[second] != 1 || renders[replacement] != 0 || NeedsFrame(world) {
		t.Fatalf("resumed renders=%v pending=%v", renders, NeedsFrame(world))
	}
}

func TestHTMLDirtyComponentSyncDoesNotAllocate(t *testing.T) {
	world := ecs.NewWorld()
	spawner := world.NewSpawner1[htmlComponentInstance]()
	entity := spawner.Spawn(htmlComponentInstance{Definition: &htmlDefinitionRuntime{}})
	system := SyncComponentsSystem("")
	step := func() {
		markHTMLComponentDirty(world, entity)
		if err := system.Run(world); err != nil {
			panic(err)
		}
	}
	step()
	step()
	if allocations := testing.AllocsPerRun(100, step); allocations != 0 {
		t.Fatalf("dirty sync allocated %g objects", allocations)
	}
}
