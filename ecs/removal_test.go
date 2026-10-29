package ecs

import (
	"slices"
	"testing"
)

func TestPreparedRemovalMovesOnceAndPreservesTicks(t *testing.T) {
	world := NewWorld()
	nodes := world.NewSpawner4[testPosition, testVelocity, testName, testSize]()
	first := nodes.Spawn(testPosition{X: 1}, testVelocity{X: 2}, testName{Value: "first"}, testSize{})
	second := nodes.Spawn(testPosition{X: 3}, testVelocity{X: 4}, testName{Value: "second"}, testSize{})
	world.GetMut[testPosition](first).X = 5
	positionColumn, positionRow := world.componentRow[testPosition](first)
	added, changed := positionColumn.added[positionRow], positionColumn.changed[positionRow]
	retainedCleanup, removedCleanup := 0, 0
	world.OnComponentRemove[testPosition](func(*World, Entity, *testPosition) { retainedCleanup++ })
	removal := world.PrepareRemoval(Type[testVelocity](), Type[testSize](), Type[testVelocity](), Type[testVisibility]())
	// Installing after preparation must affect the cached transition too.
	removal.Reserve(2)
	world.OnComponentRemove[testVelocity](func(w *World, entity Entity, value *testVelocity) {
		removedCleanup++
		if value.X != 2 || entity != first || !w.Has[testSize](entity) || w.GetRef[testName](entity).Value != "first" {
			t.Fatal("cleanup did not see the complete source entity")
		}
	})
	before, tick := world.structureVersion, world.changeTick
	if !removal.Apply(first) || world.structureVersion != before+1 || world.changeTick != tick {
		t.Fatal("batch did not perform exactly one tick-preserving migration")
	}
	if removedCleanup != 1 || retainedCleanup != 0 || world.Has[testVelocity](first) || world.Has[testSize](first) {
		t.Fatal("incorrect component cleanup")
	}
	positionColumn, positionRow = world.componentRow[testPosition](first)
	if positionColumn.values[positionRow].X != 5 || positionColumn.added[positionRow] != added || positionColumn.changed[positionRow] != changed {
		t.Fatal("retained value or ticks changed")
	}
	if world.GetRef[testPosition](second).X != 3 || world.GetRef[testName](second).Value != "second" {
		t.Fatal("swapped entity row was not updated")
	}
	context := &SystemContext{World: world, lastRun: changed - 1, thisRun: world.changeTick}
	var matches []Entity
	for entity := range world.Query[testPosition](context.Changed[testPosition]()).All {
		matches = append(matches, entity)
	}
	if !slices.Equal(matches, []Entity{first}) {
		t.Fatalf("changed query after migration: %v", matches)
	}
	before, version := world.structureVersion, world.MutationVersion()
	if removal.Apply(first) || world.structureVersion != before || world.MutationVersion() != version || removedCleanup != 1 {
		t.Fatal("absent removal mutated the world or repeated cleanup")
	}
	world.Despawn(first)
	if retainedCleanup != 1 {
		t.Fatal("retained cleanup did not run on later despawn")
	}
}

func TestPreparedRemovalEmptyTargetsStaleEntitiesAndNewShapes(t *testing.T) {
	world := NewWorld()
	removal := world.PrepareRemoval(Type[testPosition](), Type[testVelocity]())
	nodes := world.NewSpawner1[testPosition]()
	first := nodes.Spawn(testPosition{X: 1})
	if !removal.Apply(first) || !world.IsAlive(first) || world.Has[testPosition](first) {
		t.Fatal("removing the last component did not retain a live empty entity")
	}
	world.Despawn(first)
	replacement := nodes.Spawn(testPosition{X: 2})
	if removal.Apply(first) || removal.Apply(0) || world.GetRef[testPosition](replacement).X != 2 {
		t.Fatal("stale generation affected the replacement")
	}
	if err := world.Insert(replacement, testVelocity{X: 3}); err != nil {
		t.Fatal(err)
	}
	if !removal.Apply(replacement) || world.Has[testVelocity](replacement) {
		t.Fatal("a newly created source archetype was not resolved")
	}
	if world.PrepareRemoval().Apply(replacement) {
		t.Fatal("empty removal changed an entity")
	}
}

func TestPreparedRemovalReservedBurstDoesNotAllocate(t *testing.T) {
	world := NewWorld()
	nodes := world.NewSpawner3[testPosition, testVelocity, testName]()
	nodes.Reserve(128)
	removal := world.PrepareRemoval(Type[testVelocity](), Type[testName]())
	removal.Reserve(128)
	run := func() {
		var entities [128]Entity
		for i := range entities {
			entities[i] = nodes.Spawn(testPosition{X: float32(i)}, testVelocity{}, testName{})
		}
		for _, entity := range entities {
			if !removal.Apply(entity) {
				panic("components not removed")
			}
		}
		for _, entity := range entities {
			world.Despawn(entity)
		}
	}
	if allocs := testing.AllocsPerRun(50, run); allocs != 0 {
		t.Fatalf("prepared removal burst allocations = %v", allocs)
	}
}

func TestPreparedRemovalCommandsAndIterationGuard(t *testing.T) {
	world := NewWorld()
	nodes := world.NewSpawner2[testPosition, testVelocity]()
	first := nodes.Spawn(testPosition{}, testVelocity{})
	second := nodes.Spawn(testPosition{}, testVelocity{})
	removal := world.PrepareRemoval(Type[testVelocity]())
	removal.Reserve(2)
	before := world.structureVersion
	for entity := range world.Query[testPosition]().All {
		world.Commands().RemovePrepared(entity, removal)
	}
	if !world.Has[testVelocity](first) || !world.Has[testVelocity](second) {
		t.Fatal("queued removal executed during iteration")
	}
	if err := world.FlushCommands(); err != nil {
		t.Fatal(err)
	}
	if world.Has[testVelocity](first) || world.Has[testVelocity](second) || world.structureVersion != before+2 {
		t.Fatal("commands did not apply one migration per entity")
	}
	assertRemovalPanic(t, func() {
		for entity := range world.Query[testPosition]().All {
			removal.Apply(entity)
		}
	})
	assertRemovalPanic(t, func() { NewWorld().Commands().RemovePrepared(first, removal) })
	assertRemovalPanic(t, func() { PreparedRemoval{}.Apply(first) })
	assertRemovalPanic(t, func() { world.PrepareRemoval(ComponentType{}) })
}

func TestRemovalCleanupPanicKeepsStorageAndReleasesGuard(t *testing.T) {
	for _, mode := range []string{"batch", "single", "despawn"} {
		t.Run(mode, func(t *testing.T) {
			world := NewWorld()
			nodes := world.NewSpawner2[testPosition, testVelocity]()
			entity := nodes.Spawn(testPosition{X: 1}, testVelocity{X: 2})
			removal := world.PrepareRemoval(Type[testPosition](), Type[testVelocity]())
			world.OnComponentRemove[testVelocity](func(w *World, entity Entity, _ *testVelocity) {
				if w.GetRef[testPosition](entity).X != 1 {
					t.Fatal("cleanup saw a partially removed row")
				}
				w.Despawn(entity) // forbidden by the cleanup contract
			})
			before := world.structureVersion
			assertRemovalPanic(t, func() {
				switch mode {
				case "batch":
					removal.Apply(entity)
				case "single":
					world.Remove[testVelocity](entity)
				case "despawn":
					world.Despawn(entity)
				}
			})
			if world.activeComponentCleanups != 0 || world.structureVersion != before || world.GetRef[testVelocity](entity).X != 2 {
				t.Fatal("cleanup panic left inconsistent storage or a locked world")
			}
			count := 0
			for range world.Query2[testPosition, testVelocity]().All {
				count++
			}
			if count != 1 {
				t.Fatalf("rows after failed cleanup: %d", count)
			}
			world.OnComponentRemove[testVelocity](nil)
			world.Despawn(entity)
		})
	}
}

func TestPreparedRemovalCommandFailureDropsRemainingWork(t *testing.T) {
	world := NewWorld()
	nodes := world.NewSpawner2[testPosition, testVelocity]()
	first := nodes.Spawn(testPosition{}, testVelocity{})
	second := nodes.Spawn(testPosition{}, testVelocity{})
	removal := world.PrepareRemoval(Type[testVelocity]())
	world.Commands().RemovePrepared(first, removal)
	world.Commands().Insert(Entity(0), testName{})
	world.Commands().RemovePrepared(second, removal)
	if err := world.FlushCommands(); err == nil {
		t.Fatal("invalid insert did not fail")
	}
	if world.Has[testVelocity](first) || !world.Has[testVelocity](second) || len(world.commands.queue) != 0 {
		t.Fatal("failed flush replayed or retained pending removal")
	}
	if err := world.FlushCommands(); err != nil || !world.Has[testVelocity](second) {
		t.Fatal("discarded removal reappeared on a later flush")
	}
	world.OnComponentRemove[testVelocity](func(*World, Entity, *testVelocity) { panic("cleanup failed") })
	world.Commands().RemovePrepared(second, removal)
	reserved := world.Commands().Spawn(NewBundle(0))
	assertRemovalPanic(t, func() { world.FlushCommands() })
	_, meta, ok := world.entityMeta(second)
	if !ok || !world.archetypes[meta.archetypeID].has(world.registerComponent[testVelocity]()) || world.IsAlive(reserved) || len(world.commands.queue) != 0 {
		t.Fatal("cleanup panic left moved storage or pending spawn reservations")
	}
	if index, _ := reserved.Index(); world.entities[index].state != entityFree {
		t.Fatal("later spawn reservation was not released")
	}
}

func assertRemovalPanic(t *testing.T, run func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	run()
}

func BenchmarkPreparedRemovalRecycled(b *testing.B) {
	world := NewWorld()
	nodes := world.NewSpawner4[testPosition, testVelocity, testName, testSize]()
	nodes.Reserve(1)
	removal := world.PrepareRemoval(Type[testVelocity](), Type[testName](), Type[testSize]())
	removal.Reserve(1)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		entity := nodes.Spawn(testPosition{}, testVelocity{}, testName{}, testSize{})
		removal.Apply(entity)
		world.Despawn(entity)
	}
}

func BenchmarkSequentialRemovalRecycled(b *testing.B) {
	world := NewWorld()
	nodes := world.NewSpawner4[testPosition, testVelocity, testName, testSize]()
	nodes.Reserve(1)
	run := func() {
		entity := nodes.Spawn(testPosition{}, testVelocity{}, testName{}, testSize{})
		world.Remove[testVelocity](entity)
		world.Remove[testName](entity)
		world.Remove[testSize](entity)
		world.Despawn(entity)
	}
	run()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		run()
	}
}
