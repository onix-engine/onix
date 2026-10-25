package ecs

import "testing"

func TestCommandSpawnReusesAndClearsStorage(t *testing.T) {
	world := NewWorld()
	spawner := NewSpawner1[testPosition](world)
	spawner.Reserve(1)
	bundle := NewBundle(1)
	Put(&bundle, testPosition{X: 1})
	prepared := world.PrepareBundle(bundle)
	for _, usePrepared := range []bool{false, true} {
		run := func() {
			var entity Entity
			if usePrepared {
				entity = CommandSpawnPrepared(world.Commands(), prepared)
			} else {
				entity = CommandSpawn(world.Commands(), bundle)
			}
			if err := world.FlushCommands(); err != nil {
				t.Fatal(err)
			}
			world.Despawn(entity)
		}
		if allocations := testing.AllocsPerRun(100, run); allocations != 0 {
			t.Fatalf("prepared=%v allocations=%v, want 0", usePrepared, allocations)
		}
		assertCommandStorageRecycled(t, world.Commands())
	}
}

func assertCommandStorageRecycled(t *testing.T, commands *Commands) {
	t.Helper()
	if len(commands.activeStreams) != 0 {
		t.Fatal("flush retained active command streams")
	}
	if len(commands.queue) != 0 || cap(commands.queue) == 0 || len(commands.spawnEntries) != 0 || cap(commands.spawnEntries) == 0 {
		t.Fatalf("storage not recycled: queue=%d/%d spawn=%d/%d", len(commands.queue), cap(commands.queue), len(commands.spawnEntries), cap(commands.spawnEntries))
	}
	for _, entry := range commands.spawnEntries[:cap(commands.spawnEntries)] {
		if entry.entry != nil {
			t.Fatal("spawn storage retained a bundle reference")
		}
	}
	for _, queued := range commands.queue[:cap(commands.queue)] {
		if queued != (queuedCommand{}) {
			t.Fatal("queue storage retained a command")
		}
	}
	if len(commands.removals) != 0 {
		t.Fatal("prepared removal payloads were not recycled")
	}
	for _, plan := range commands.removals[:cap(commands.removals)] {
		if plan != nil {
			t.Fatal("prepared removal storage retained a plan")
		}
	}
}

func TestCommandStreamsReactivateAfterPartialRollback(t *testing.T) {
	world := NewWorld()
	entity := world.Spawn(NewBundle(0))
	commands := world.Commands()
	value := 7
	patch := NewPatch1[*int](world)
	CommandInsert(commands, entity, testName{Value: "kept"})
	checkpoint := commands.checkpoint()
	patch.Queue(commands, entity, &value)
	CommandInsert(commands, entity, &value)
	commands.discardFrom(checkpoint)
	if len(commands.activeStreams) != 1 {
		t.Fatal("rollback retained empty streams or lost the preceding stream")
	}
	stream := insertCommandStreamFor[*int](commands)
	for _, pointer := range stream.values[:cap(stream.values)] {
		if pointer != nil {
			t.Fatal("rollback retained a pointer payload")
		}
	}
	// Removal commands need a dispatch stream but own no queued values.
	CommandRemove[testVelocity](commands, entity)
	if len(commands.activeStreams) != 1 {
		t.Fatal("value-free removal activated a value stream")
	}
	for range 3 {
		patch.Queue(commands, entity, &value)
		CommandInsert(commands, entity, &value)
	}
	if len(commands.activeStreams) != 3 {
		t.Fatal("reactivation duplicated or omitted a stream")
	}
	if err := world.FlushCommands(); err != nil {
		t.Fatal(err)
	}
	if len(commands.activeStreams) != 0 || GetRef[testName](world, entity).Value != "kept" || *GetRef[*int](world, entity) != &value {
		t.Fatal("flush lost retained values or failed to reset the active streams")
	}
	for _, pointer := range stream.values[:cap(stream.values)] {
		if pointer != nil {
			t.Fatal("flush retained a reactivated payload")
		}
	}
	run := func() {
		CommandInsert(commands, entity, &value)
		checkpoint := commands.checkpoint()
		patch.Queue(commands, entity, &value)
		commands.discardFrom(checkpoint)
		patch.Queue(commands, entity, &value)
		if err := world.FlushCommands(); err != nil {
			panic(err)
		}
	}
	if allocations := testing.AllocsPerRun(100, run); allocations != 0 {
		t.Fatalf("reactivation allocated %v times", allocations)
	}
}

func TestEmptyCommandFlushKeepsStructuralGuards(t *testing.T) {
	world := NewWorld()
	spawn := NewSpawner1[testPosition](world)
	entity := spawn.Spawn(testPosition{})
	assertRemovalPanic(t, func() {
		for range Query[testPosition](world).All {
			_ = world.FlushCommands()
		}
	})
	OnComponentRemove[testPosition](world, func(w *World, _ Entity, _ *testPosition) {
		assertRemovalPanic(t, func() { _ = w.FlushCommands() })
	})
	world.Despawn(entity)
	if err := world.FlushCommands(); err != nil {
		t.Fatal(err)
	}
}

func TestCommandValueStreamsRecoverAfterFailedFlush(t *testing.T) {
	for _, panics := range []bool{false, true} {
		world := NewWorld()
		entity := world.Spawn(NewBundle(0))
		commands := world.Commands()
		value := 42
		patch := NewPatch1[*int](world)
		if panics {
			bundle := NewBundle(1)
			PutEntity(&bundle, func(Entity) testPosition { panic("build failed") })
			CommandSpawn(commands, bundle)
		} else {
			CommandInsert(commands, Entity(0), testPosition{})
		}
		patch.Queue(commands, entity, &value)
		CommandInsert(commands, entity, &value)
		func() {
			defer func() {
				if got := recover(); (got != nil) != panics {
					t.Fatalf("panic = %v, expected panic = %v", got, panics)
				}
			}()
			if err := world.FlushCommands(); err == nil {
				t.Fatal("expected failed flush")
			}
		}()
		if len(commands.activeStreams) != 0 || Has[*int](world, entity) {
			t.Fatal("failed flush retained streams or applied pending writes")
		}
		stream := insertCommandStreamFor[*int](commands)
		for _, pointer := range stream.values[:cap(stream.values)] {
			if pointer != nil {
				t.Fatal("failed flush retained an insert payload")
			}
		}
		patchStream := commandStreamFor(commands, func() *valueCommandStream[patchCommand1[*int]] {
			return &valueCommandStream[patchCommand1[*int]]{}
		})
		for _, command := range patchStream.values[:cap(patchStream.values)] {
			if command.a != nil || command.patch.plan != nil {
				t.Fatal("failed flush retained a patch payload")
			}
		}
		patch.Queue(commands, entity, &value)
		CommandInsert(commands, entity, &value)
		if err := world.FlushCommands(); err != nil || *GetRef[*int](world, entity) != &value {
			t.Fatalf("reactivation after failed flush: %v", err)
		}
	}
}

func TestCommandStorageRecycledAfterFailure(t *testing.T) {
	for _, panics := range []bool{false, true} {
		world := NewWorld()
		commands := world.Commands()
		if panics {
			bundle := NewBundle(1)
			PutEntity(&bundle, func(Entity) testPosition { panic("build failed") })
			CommandSpawn(commands, bundle)
		} else {
			CommandInsert(commands, Entity(0), testPosition{})
		}
		bundle := NewBundle(1)
		Put(&bundle, testPosition{})
		reserved := CommandSpawn(commands, bundle)
		func() {
			defer func() {
				if value := recover(); (value != nil) != panics {
					t.Fatalf("panic=%v, want panic=%v", value, panics)
				}
			}()
			if err := world.FlushCommands(); err == nil {
				t.Fatal("expected command error")
			}
		}()
		assertCommandStorageRecycled(t, commands)
		if world.IsAlive(reserved) || world.EntityCount() != 0 {
			t.Fatal("failed flush retained a reserved entity")
		}
		entity := CommandSpawn(commands, bundle)
		if err := world.FlushCommands(); err != nil || !world.IsAlive(entity) {
			t.Fatalf("flush after failure: alive=%v err=%v", world.IsAlive(entity), err)
		}
	}
}

func BenchmarkCommandSpawnPreparedRecycled(b *testing.B) {
	world := NewWorld()
	spawner := NewSpawner1[benchmarkPosition](world)
	spawner.Reserve(1)
	bundle := NewBundle(1)
	Put(&bundle, benchmarkPosition{})
	prepared := world.PrepareBundle(bundle)
	run := func() {
		entity := CommandSpawnPrepared(world.Commands(), prepared)
		if err := world.FlushCommands(); err != nil {
			b.Fatal(err)
		}
		world.Despawn(entity)
	}
	run()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		run()
	}
}
