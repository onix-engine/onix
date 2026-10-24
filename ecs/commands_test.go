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
