package ecs

import "testing"

func TestMigrationReserveOnlyGrowsUsedTargets(t *testing.T) {
	for _, patchMode := range []bool{false, true} {
		world := NewWorld()
		source := NewSpawner2[testPosition, testVelocity](world)
		source.Reserve(4) // Preallocated empty sources must also be prepared.
		unused := NewSpawner1[testName](world)
		entityCapacity := cap(world.entities)
		archetypes := len(world.archetypes)
		if patchMode {
			patch := NewPatch1[testSize](world, Type[testVelocity]())
			patch.Reserve(1024)
		} else {
			removal := PrepareRemoval(world, Type[testVelocity]())
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
	source := NewSpawner1[testPosition](world)
	source.Reserve(4)
	patch := NewPatch1[testPosition](world)
	patch.Reserve(1024)
	if cap(source.plan.archetype.entities) != 4 || cap(world.entities) != 4 || len(world.archetypes) != 2 {
		t.Fatal("replacement-only patch reserved migration storage")
	}
}

func TestCommandDiscardPreservesRemovalPlansAndSpawnRanges(t *testing.T) {
	world := NewWorld()
	spawner := NewSpawner2[testPosition, testVelocity](world)
	first := spawner.Spawn(testPosition{}, testVelocity{})
	second := spawner.Spawn(testPosition{}, testVelocity{})
	commands := world.Commands()
	removeVelocity := PrepareRemoval(world, Type[testVelocity]())
	removePosition := PrepareRemoval(world, Type[testPosition]())
	bundle := NewBundle(1)
	Put(&bundle, testName{Value: "kept"})
	CommandRemovePrepared(commands, first, removeVelocity)
	spawned := CommandSpawn(commands, bundle)
	checkpoint := commands.checkpoint()
	CommandRemovePrepared(commands, second, removePosition)
	discarded := CommandSpawn(commands, NewBundle(0))
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
	empty := CommandSpawn(commands, NewBundle(0))
	another := CommandSpawn(commands, bundle)
	if err := world.FlushCommands(); err != nil {
		t.Fatal(err)
	}
	if Has[testVelocity](world, first) || !Has[testPosition](world, first) || !Has[testPosition](world, second) || !Has[testVelocity](world, second) {
		t.Fatal("wrong prepared removal was applied")
	}
	if !world.IsAlive(empty) || Has[testName](world, empty) || GetRef[testName](world, spawned).Value != "kept" || GetRef[testName](world, another).Value != "kept" {
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
		spawner := NewSpawner2[testPosition, testVelocity](world)
		entity := spawner.Spawn(testPosition{}, testVelocity{})
		commands := world.Commands()
		if panics {
			OnComponentRemove[testVelocity](world, func(*World, Entity, *testVelocity) { panic("cleanup") })
		} else {
			CommandInsert(commands, Entity(0), testPosition{})
		}
		CommandRemovePrepared(commands, entity, PrepareRemoval(world, Type[testVelocity]()))
		bundle := NewBundle(1)
		Put(&bundle, testName{})
		reserved := CommandSpawn(commands, bundle)
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
			spawner := NewSpawner2[testPosition, testVelocity](world)
			spawner.Reserve(count + 1)
			entities := make([]Entity, count)
			for i := range entities {
				entities[i] = spawner.Spawn(testPosition{}, testVelocity{})
			}
			last := world.ChangeTick()
			for _, index := range []int{0, 63, 64, 4095, 4096, 8191, 8192} {
				if index < count {
					GetMut[testPosition](world, entities[index]).X++
					GetMut[testVelocity](world, entities[index]).X++
				}
			}
			window := world.ChangesSince(last)
			if count > 1 {
				world.Despawn(entities[1]) // Swap a boundary row across groups.
			}
			if count > 64 {
				if err := Insert(world, entities[64], testName{}); err != nil {
					t.Fatal(err)
				}
			}
			if count > 4096 {
				Remove[testVelocity](world, entities[4096])
			}
			// Future writes and additions are excluded from the saved window.
			if count > 0 {
				GetMut[testPosition](world, entities[0]).X++
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
		options := []QueryOption{Changed[testPosition](window), Changed[testVelocity](window)}
		kind := filterChanged
		if added {
			options = []QueryOption{Added[testPosition](window), Added[testVelocity](window)}
			kind = filterAdded
		}
		expected := make(map[Entity]bool)
		for _, archetype := range world.archetypes {
			position, _ := archetype.column(registerComponent[testPosition](world)).(*typedColumn[testPosition])
			velocity, _ := archetype.column(registerComponent[testVelocity](world)).(*typedColumn[testVelocity])
			if position == nil || velocity == nil {
				continue
			}
			for row, entity := range archetype.entities {
				if tickInRange(position.ticks(kind)[row], window.LastRun(), window.ThisRun()) && tickInRange(velocity.ticks(kind)[row], window.LastRun(), window.ThisRun()) {
					expected[entity] = true
				}
			}
		}
		for entity := range Query2[testPosition, testVelocity](world, options...).All {
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
	spawner := NewSpawner2[testPosition, testVelocity](world)
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
	spawner := NewSpawner2[testPosition, testVelocity](world)
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
