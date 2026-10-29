package ecs

import "testing"

type traversalComponent[T any] struct{ Value int }

func queueTraversalComponent[T any](commands *Commands, entity Entity, count int) {
	for i := range count {
		commands.Insert(entity, traversalComponent[T]{i})
	}
}

func BenchmarkTraversalCommandTrim(b *testing.B) {
	world := NewWorld()
	commands := world.Commands()
	queueTraversalComponent[int](commands, 0, 128)
	queueTraversalComponent[string](commands, 0, 128)
	queueTraversalComponent[bool](commands, 0, 128)
	queueTraversalComponent[float32](commands, 0, 128)
	queueTraversalComponent[float64](commands, 0, 128)
	queueTraversalComponent[uint8](commands, 0, 128)
	queueTraversalComponent[uint16](commands, 0, 128)
	queueTraversalComponent[uint32](commands, 0, 128)
	commands.trimStorage()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		commands.trimStorage()
	}
}

func BenchmarkTraversalMigration(b *testing.B) {
	world := NewWorld()
	bundle := NewBundle(8)
	bundle.Put(traversalComponent[int]{1})
	bundle.Put(traversalComponent[string]{2})
	bundle.Put(traversalComponent[bool]{3})
	bundle.Put(traversalComponent[float32]{4})
	bundle.Put(traversalComponent[float64]{5})
	bundle.Put(traversalComponent[uint8]{6})
	bundle.Put(traversalComponent[uint16]{7})
	bundle.Put(traversalComponent[uint32]{8})
	entity := world.Spawn(bundle)
	run := func() {
		if err := world.Insert(entity, testPosition{X: 9}); err != nil {
			b.Fatal(err)
		}
		world.Remove[testPosition](entity)
	}
	run()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		run()
	}
}

func TestTraversalPartialCommandRollback(t *testing.T) {
	world := NewWorld()
	commands := world.Commands()
	entity := world.Spawn(NewBundle(0))
	commands.Insert(entity, testPosition{X: 7})
	bundle := NewBundle(1)
	bundle.Put(testPosition{X: 15})
	retained := commands.Spawn(bundle)
	checkpoint := commands.checkpoint()
	commands.Insert(entity, testPosition{X: 99})
	commands.Insert(entity, testVelocity{X: 99})
	discarded := commands.Spawn(bundle)
	commands.discardFrom(checkpoint)
	positions := commands.insertCommandStreamFor[testPosition]()
	velocities := commands.insertCommandStreamFor[testVelocity]()
	if len(positions.values) != 1 || positions.values[0].X != 7 || len(velocities.values) != 0 {
		t.Fatalf("rollback lost retained values: positions=%v velocities=%v", positions.values, velocities.values)
	}
	commands.trimStorage()
	if len(commands.spawnEntries) != 1 {
		t.Fatal("incorrect retained spawn entries")
	}
	if world.cancelReserved(discarded) {
		t.Fatal("discarded reservation survived")
	}
	commands.Insert(entity, testVelocity{X: 11})
	if err := world.FlushCommands(); err != nil {
		t.Fatal(err)
	}
	spawned, alive := world.Get[testPosition](retained)
	if !alive || spawned.X != 15 {
		t.Fatal("retained spawn lost")
	}
	position, _ := world.Get[testPosition](entity)
	velocity, _ := world.Get[testVelocity](entity)
	if position.X != 7 || velocity.X != 11 {
		t.Fatalf("wrong retained commands: %v %v", position, velocity)
	}
	// A later trim must not reuse the previous retained high-water marks.
	commands.Insert(entity, testPosition{X: 42})
	commands.discardFrom(0)
	if len(positions.values) != 0 || len(velocities.values) != 0 {
		t.Fatal("stale retained lengths")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		commands.Insert(entity, testPosition{X: 7})
		checkpoint := commands.checkpoint()
		commands.Insert(entity, testPosition{X: 99})
		commands.discardFrom(checkpoint)
		commands.discardFrom(0)
	}); allocs != 0 {
		t.Fatalf("rollback allocated %v times", allocs)
	}
}

func TestTraversalMigrationMovesMiddleRowsAndPreservesTicks(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner2[testPosition, testVelocity]()
	entities := []Entity{
		spawner.Spawn(testPosition{X: 1}, testVelocity{X: 11}),
		spawner.Spawn(testPosition{X: 2}, testVelocity{X: 12}),
		spawner.Spawn(testPosition{X: 3}, testVelocity{X: 13}),
	}
	ticks := make(map[Entity][2]Tick)
	for _, entity := range entities {
		_, meta, _ := world.entityMeta(entity)
		col := world.archetypes[meta.archetypeID].componentColumn[testPosition](world.registerComponent[testPosition]())
		ticks[entity] = [2]Tick{col.added[meta.row], col.changed[meta.row]}
	}
	for _, entity := range entities[:2] {
		if err := world.Insert(entity, testLocalTag{}); err != nil {
			t.Fatal(err)
		}
	}
	world.Remove[testLocalTag](entities[0]) // Swap the other entity in the target archetype.
	world.Remove[testVelocity](entities[1]) // Remove a non-tag column too.
	for i, entity := range entities {
		pos, ok := world.Get[testPosition](entity)
		if !ok || pos.X != float32(i+1) {
			t.Fatalf("entity %d lost its row: %v", entity, pos)
		}
		_, meta, _ := world.entityMeta(entity)
		col := world.archetypes[meta.archetypeID].componentColumn[testPosition](world.registerComponent[testPosition]())
		if got := [2]Tick{col.added[meta.row], col.changed[meta.row]}; got != ticks[entity] {
			t.Fatalf("ticks changed: %v != %v", got, ticks[entity])
		}
		velocity, exists := world.Get[testVelocity](entity)
		if exists != (i != 1) || exists && velocity.X != float32(i+11) {
			t.Fatalf("wrong velocity: %v %v", velocity, exists)
		}
	}
}
