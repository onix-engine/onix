package ecs

import "testing"

func TestMigrationReserveOnlyGrowsUsedTargets(t *testing.T) {
	for _, patchMode := range []bool{false, true} {
		world := NewWorld()
		source := world.NewSpawner2[testPosition, testVelocity]()
		source.Reserve(4) // Preallocated empty sources must also be prepared.
		unused := world.NewSpawner1[testName]()
		entityCapacity := cap(world.entities)
		archetypes := len(world.archetypes)
		if patchMode {
			patch := world.NewPatch1[testSize](Type[testVelocity]())
			patch.Reserve(1024)
		} else {
			removal := world.PrepareRemoval(Type[testVelocity]())
			removal.Reserve(1024)
		}
		if cap(world.entities) != entityCapacity || cap(source.plan.archetype.entities) != 4 || cap(unused.plan.archetype.entities) != 0 {
			t.Fatal("migration reservation grew entity slots, its source or an unused table")
		}
		if len(world.archetypes) != archetypes+1 || cap(world.archetypes[archetypes].entities) < 1024 {
			t.Fatal("migration reservation did not prepare exactly its used target")
		}
	}
	world := NewWorld()
	source := world.NewSpawner1[testPosition]()
	source.Reserve(4)
	patch := world.NewPatch1[testPosition]()
	patch.Reserve(1024)
	if cap(source.plan.archetype.entities) != 4 || cap(world.entities) != 4 || len(world.archetypes) != 2 {
		t.Fatal("replacement-only patch reserved migration storage")
	}
}

func TestCommandDiscardPreservesRemovalPlansAndSpawnRanges(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner2[testPosition, testVelocity]()
	first := spawner.Spawn(testPosition{}, testVelocity{})
	second := spawner.Spawn(testPosition{}, testVelocity{})
	commands := world.Commands()
	removeVelocity := world.PrepareRemoval(Type[testVelocity]())
	removePosition := world.PrepareRemoval(Type[testPosition]())
	bundle := NewBundle(1)
	bundle.Put(testName{Value: "kept"})
	commands.RemovePrepared(first, removeVelocity)
	spawned := commands.Spawn(bundle)
	checkpoint := commands.checkpoint()
	commands.RemovePrepared(second, removePosition)
	discarded := commands.Spawn(NewBundle(0))
	commands.discardFrom(checkpoint)
	if len(commands.removals) != 1 || commands.removals[0] != removeVelocity.plan {
		t.Fatal("discard lost the preceding removal plan")
	}
	for _, plan := range commands.removals[1:cap(commands.removals)] {
		if plan != nil {
			t.Fatal("discard retained an abandoned removal plan")
		}
	}
	if world.cancelReserved(discarded) {
		t.Fatal("discard retained an empty-bundle spawn reservation")
	}
	// A zero-component spawn and another nonempty spawn must keep distinct ranges.
	empty := commands.Spawn(NewBundle(0))
	another := commands.Spawn(bundle)
	if err := world.FlushCommands(); err != nil {
		t.Fatal(err)
	}
	if world.Has[testVelocity](first) || !world.Has[testPosition](first) || !world.Has[testPosition](second) || !world.Has[testVelocity](second) {
		t.Fatal("wrong prepared removal was applied")
	}
	if !world.IsAlive(empty) || world.Has[testName](empty) || world.GetRef[testName](spawned).Value != "kept" || world.GetRef[testName](another).Value != "kept" {
		t.Fatal("spawn component ranges were corrupted")
	}
	for _, plan := range commands.removals[:cap(commands.removals)] {
		if plan != nil {
			t.Fatal("flush retained a removal plan")
		}
	}
}

func TestRemovalCommandReferencesClearAfterFailure(t *testing.T) {
	for _, panics := range []bool{false, true} {
		world := NewWorld()
		spawner := world.NewSpawner2[testPosition, testVelocity]()
		entity := spawner.Spawn(testPosition{}, testVelocity{})
		commands := world.Commands()
		if panics {
			world.OnComponentRemove[testVelocity](func(*World, Entity, *testVelocity) { panic("cleanup") })
		} else {
			commands.Insert(Entity(0), testPosition{})
		}
		commands.RemovePrepared(entity, world.PrepareRemoval(Type[testVelocity]()))
		bundle := NewBundle(1)
		bundle.Put(testName{})
		reserved := commands.Spawn(bundle)
		func() {
			defer func() {
				if value := recover(); (value != nil) != panics {
					t.Fatalf("panic=%v, want panic=%v", value, panics)
				}
			}()
			if err := world.FlushCommands(); err == nil {
				t.Fatal("expected a command failure")
			}
		}()
		assertCommandStorageRecycled(t, commands)
		if world.cancelReserved(reserved) {
			t.Fatal("failed flush retained a pending spawn")
		}
	}
}

func TestChangeSummaryBoundariesMatchRowTicks(t *testing.T) {
	for _, count := range []int{0, 1, 63, 64, 65, 4095, 4096, 4097, 8193} {
		for _, wrap := range []bool{false, true} {
			world := NewWorld()
			if wrap {
				world.changeTick = Tick(^uint32(0) - 80)
			}
			spawner := world.NewSpawner2[testPosition, testVelocity]()
			spawner.Reserve(count + 1)
			entities := make([]Entity, count)
			for i := range entities {
				entities[i] = spawner.Spawn(testPosition{}, testVelocity{})
			}
			last := world.ChangeTick()
			for _, index := range []int{0, 63, 64, 4095, 4096, 8191, 8192} {
				if index < count {
					world.GetMut[testPosition](entities[index]).X++
					world.GetMut[testVelocity](entities[index]).X++
				}
			}
			window := world.ChangesSince(last)
			if count > 1 {
				world.Despawn(entities[1]) // Swap a boundary row across groups.
			}
			if count > 64 {
				if err := world.Insert(entities[64], testName{}); err != nil {
					t.Fatal(err)
				}
			}
			if count > 4096 {
				world.Remove[testVelocity](entities[4096])
			}
			// Future writes and additions are excluded from the saved window.
			if count > 0 {
				world.GetMut[testPosition](entities[0]).X++
			}
			spawner.Spawn(testPosition{}, testVelocity{})
			assertChangedRowsMatchTicks(t, world, &window)
			latest := world.ChangesSince(window.ThisRun())
			assertChangedRowsMatchTicks(t, world, &latest)
		}
	}
}

func assertChangedRowsMatchTicks(t *testing.T, world *World, window *SystemContext) {
	t.Helper()
	for _, added := range []bool{false, true} {
		options := []QueryOption{window.Changed[testPosition](), window.Changed[testVelocity]()}
		kind := filterChanged
		if added {
			options = []QueryOption{window.Added[testPosition](), window.Added[testVelocity]()}
			kind = filterAdded
		}
		expected := make(map[Entity]bool)
		for _, archetype := range world.archetypes {
			position, _ := archetype.column(world.registerComponent[testPosition]()).(*typedColumn[testPosition])
			velocity, _ := archetype.column(world.registerComponent[testVelocity]()).(*typedColumn[testVelocity])
			if position == nil || velocity == nil {
				continue
			}
			for row, entity := range archetype.entities {
				if tickInRange(position.ticks(kind)[row], window.LastRun(), window.ThisRun()) && tickInRange(velocity.ticks(kind)[row], window.LastRun(), window.ThisRun()) {
					expected[entity] = true
				}
			}
		}
		for entity := range world.Query2[testPosition, testVelocity](options...).All {
			if !expected[entity] {
				t.Fatalf("unexpected or duplicate entity %v, added=%v window=%v..%v", entity, added, window.LastRun(), window.ThisRun())
			}
			delete(expected, entity)
		}
		if len(expected) != 0 {
			t.Fatalf("summary skipped %d matching entities, added=%v window=%v..%v", len(expected), added, window.LastRun(), window.ThisRun())
		}
	}
}

func TestChangeSummariesSurviveEmptyAndRegrowth(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner2[testPosition, testVelocity]()
	for range 2 {
		last := world.ChangeTick()
		for range 8193 {
			spawner.Spawn(testPosition{}, testVelocity{})
		}
		window := world.ChangesSince(last)
		assertChangedRowsMatchTicks(t, world, &window)
		for len(spawner.plan.archetype.entities) != 0 {
			world.Despawn(spawner.plan.archetype.entities[0])
		}
		assertChangedRowsMatchTicks(t, world, &window)
	}
}

func TestChangeSummariesGrowAfterCachedQueries(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner2[testPosition, testVelocity]()
	for _, count := range []int{1, 65, 129, 1025, 4097, 8193} {
		last := world.ChangeTick()
		for world.EntityCount() < count {
			spawner.Spawn(testPosition{}, testVelocity{})
		}
		window := world.ChangesSince(last)
		assertChangedRowsMatchTicks(t, world, &window)
		idle := world.ChangesSince(window.ThisRun())
		assertChangedRowsMatchTicks(t, world, &idle)
	}
}
