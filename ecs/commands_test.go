package ecs

import "testing"

func TestCommandSpawnReusesAndClearsStorage(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner1[testPosition]()
	spawner.Reserve(1)
	bundle := NewBundle(1)
	bundle.Put(testPosition{X: 1})
	prepared := world.PrepareBundle(bundle)
	for _, usePrepared := range []bool{false, true} {
		run := func() {
			var entity Entity
			if usePrepared {
				entity = world.Commands().SpawnPrepared(prepared)
			} else {
				entity = world.Commands().Spawn(bundle)
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

func TestQueuedPreparedSpawnsSharePlanAcrossRollback(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(2)
	bundle.Put(17)
	bundle.PutEntity(func(entity Entity) Entity { return entity })
	prepared := world.PrepareBundle(bundle)
	commands := world.Commands()
	first := commands.SpawnPrepared(prepared)
	checkpoint := commands.checkpoint()
	discarded := commands.SpawnPrepared(prepared)
	commands.discardFrom(checkpoint)
	if len(commands.spawns.entries) != 0 || len(commands.spawns.plans) != 1 || commands.spawns.plans[0] != prepared.plan {
		t.Fatal("rollback must retain only the first plan reference")
	}
	plans := commands.spawns.plans
	for _, plan := range plans[len(plans):cap(plans)] {
		if plan != nil {
			t.Fatal("rollback retained the discarded plan")
		}
	}
	reused := commands.SpawnPrepared(prepared)
	if reused == discarded || reused.PickingID() != discarded.PickingID() {
		t.Fatal("discarded reservation did not advance its generation and recycle its slot")
	}
	if _, alive := world.EntityByPickingID(reused.PickingID()); alive {
		t.Fatal("reserved slot was visible before flush")
	}
	if err := world.FlushCommands(); err != nil {
		t.Fatal(err)
	}
	for _, entity := range []Entity{first, reused} {
		if got, ok := world.EntityByPickingID(entity.PickingID()); !ok || got != entity {
			t.Fatalf("picking ID for %v resolved to %v, %t", entity, got, ok)
		}
		owner, _ := world.Get[Entity](entity)
		value, _ := world.Get[int](entity)
		if owner != entity || value != 17 {
			t.Fatalf("prepared spawn %v has owner %v and value %d", entity, owner, value)
		}
	}
	assertCommandStorageRecycled(t, commands)
}

func assertCommandStorageRecycled(t *testing.T, commands *Commands) {
	t.Helper()
	if len(commands.activeStreams) != 0 {
		t.Fatal("flush retained active command streams")
	}
	spawns := commands.spawns
	if len(commands.queue) != 0 || cap(commands.queue) == 0 || len(spawns.entries) != 0 || len(spawns.plans) != 0 {
		t.Fatalf("storage not recycled: queue=%d/%d entries=%d plans=%d", len(commands.queue), cap(commands.queue), len(spawns.entries), len(spawns.plans))
	}
	if cap(spawns.entries) == 0 && cap(spawns.plans) == 0 {
		t.Fatal("flush dropped all reusable spawn storage")
	}
	for _, entry := range spawns.entries[:cap(spawns.entries)] {
		if entry.entry != nil {
			t.Fatal("spawn storage retained a bundle reference")
		}
	}
	for _, plan := range spawns.plans[:cap(spawns.plans)] {
		if plan != nil {
			t.Fatal("spawn storage retained a prepared plan")
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
	patch := world.NewPatch1[*int]()
	commands.Insert(entity, testName{Value: "kept"})
	checkpoint := commands.checkpoint()
	patch.Queue(commands, entity, &value)
	commands.Insert(entity, &value)
	commands.discardFrom(checkpoint)
	if len(commands.activeStreams) != 1 {
		t.Fatal("rollback retained empty streams or lost the preceding stream")
	}
	stream := commands.insertCommandStreamFor[*int]()
	for _, pointer := range stream.values[:cap(stream.values)] {
		if pointer != nil {
			t.Fatal("rollback retained a pointer payload")
		}
	}
	// Removal commands need a dispatch stream but own no queued values.
	commands.Remove[testVelocity](entity)
	if len(commands.activeStreams) != 1 {
		t.Fatal("value-free removal activated a value stream")
	}
	for range 3 {
		patch.Queue(commands, entity, &value)
		commands.Insert(entity, &value)
	}
	if len(commands.activeStreams) != 3 {
		t.Fatal("reactivation duplicated or omitted a stream")
	}
	if err := world.FlushCommands(); err != nil {
		t.Fatal(err)
	}
	if len(commands.activeStreams) != 0 || world.GetRef[testName](entity).Value != "kept" || *world.GetRef[*int](entity) != &value {
		t.Fatal("flush lost retained values or failed to reset the active streams")
	}
	for _, pointer := range stream.values[:cap(stream.values)] {
		if pointer != nil {
			t.Fatal("flush retained a reactivated payload")
		}
	}
	run := func() {
		commands.Insert(entity, &value)
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
	spawn := world.NewSpawner1[testPosition]()
	entity := spawn.Spawn(testPosition{})
	assertRemovalPanic(t, func() {
		for range world.Query[testPosition]().All {
			_ = world.FlushCommands()
		}
	})
	world.OnComponentRemove[testPosition](func(w *World, _ Entity, _ *testPosition) {
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
		patch := world.NewPatch1[*int]()
		if panics {
			bundle := NewBundle(1)
			bundle.PutEntity(func(Entity) testPosition { panic("build failed") })
			commands.Spawn(bundle)
		} else {
			commands.Insert(Entity(0), testPosition{})
		}
		patch.Queue(commands, entity, &value)
		commands.Insert(entity, &value)
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
		if len(commands.activeStreams) != 0 || world.Has[*int](entity) {
			t.Fatal("failed flush retained streams or applied pending writes")
		}
		stream := commands.insertCommandStreamFor[*int]()
		for _, pointer := range stream.values[:cap(stream.values)] {
			if pointer != nil {
				t.Fatal("failed flush retained an insert payload")
			}
		}
		patchStream := commands.commandStreamFor(func() *valueCommandStream[patchCommand1[*int]] {
			return &valueCommandStream[patchCommand1[*int]]{}
		})
		for _, command := range patchStream.values[:cap(patchStream.values)] {
			if command.a != nil || command.patch.plan != nil {
				t.Fatal("failed flush retained a patch payload")
			}
		}
		patch.Queue(commands, entity, &value)
		commands.Insert(entity, &value)
		if err := world.FlushCommands(); err != nil || *world.GetRef[*int](entity) != &value {
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
			bundle.PutEntity(func(Entity) testPosition { panic("build failed") })
			commands.SpawnPrepared(world.PrepareBundle(bundle))
		} else {
			commands.Insert(Entity(0), testPosition{})
		}
		bundle := NewBundle(1)
		bundle.Put(testPosition{})
		reserved := commands.Spawn(bundle)
		commands.SpawnPrepared(world.PrepareBundle(bundle))
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
		if world.IsAlive(reserved) || world.EntityCount() != 0 || len(world.free) != world.EntitySlots() {
			t.Fatal("failed flush retained a reserved entity")
		}
		entity := commands.Spawn(bundle)
		if err := world.FlushCommands(); err != nil || !world.IsAlive(entity) {
			t.Fatalf("flush after failure: alive=%v err=%v", world.IsAlive(entity), err)
		}
	}
}

func BenchmarkCommandSpawnPreparedRecycled(b *testing.B) {
	world := NewWorld()
	spawner := world.NewSpawner1[benchmarkPosition]()
	spawner.Reserve(1)
	bundle := NewBundle(1)
	bundle.Put(benchmarkPosition{})
	prepared := world.PrepareBundle(bundle)
	run := func() {
		entity := world.Commands().SpawnPrepared(prepared)
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
