package ecs

import "testing"

type traversalComponent[T any] struct{ Value int }

func queueTraversalComponent[T any](commands *Commands, entity Entity, count int) {
	for i := range count {
		CommandInsert(commands, entity, traversalComponent[T]{i})
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
	Put(&bundle, traversalComponent[int]{1})
	Put(&bundle, traversalComponent[string]{2})
	Put(&bundle, traversalComponent[bool]{3})
	Put(&bundle, traversalComponent[float32]{4})
	Put(&bundle, traversalComponent[float64]{5})
	Put(&bundle, traversalComponent[uint8]{6})
	Put(&bundle, traversalComponent[uint16]{7})
	Put(&bundle, traversalComponent[uint32]{8})
	entity := world.Spawn(bundle)
	run := func() {
		if err := Insert(world, entity, testPosition{X: 9}); err != nil {
			b.Fatal(err)
		}
		Remove[testPosition](world, entity)
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
	CommandInsert(commands, entity, testPosition{X: 7})
	bundle := NewBundle(1)
	Put(&bundle, testPosition{X: 15})
	retained := CommandSpawn(commands, bundle)
	checkpoint := commands.checkpoint()
	CommandInsert(commands, entity, testPosition{X: 99})
	CommandInsert(commands, entity, testVelocity{X: 99})
	discarded := CommandSpawn(commands, bundle)
	commands.discardFrom(checkpoint)
	positions := insertCommandStreamFor[testPosition](commands)
	velocities := insertCommandStreamFor[testVelocity](commands)
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
	CommandInsert(commands, entity, testVelocity{X: 11})
	if err := world.FlushCommands(); err != nil {
		t.Fatal(err)
	}
	spawned, alive := Get[testPosition](world, retained)
	if !alive || spawned.X != 15 {
		t.Fatal("retained spawn lost")
	}
	position, _ := Get[testPosition](world, entity)
	velocity, _ := Get[testVelocity](world, entity)
	if position.X != 7 || velocity.X != 11 {
		t.Fatalf("wrong retained commands: %v %v", position, velocity)
	}
	// A later trim must not reuse the previous retained high-water marks.
	CommandInsert(commands, entity, testPosition{X: 42})
	commands.discardFrom(0)
	if len(positions.values) != 0 || len(velocities.values) != 0 {
		t.Fatal("stale retained lengths")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		CommandInsert(commands, entity, testPosition{X: 7})
		checkpoint := commands.checkpoint()
		CommandInsert(commands, entity, testPosition{X: 99})
		commands.discardFrom(checkpoint)
		commands.discardFrom(0)
	}); allocs != 0 {
		t.Fatalf("rollback allocated %v times", allocs)
	}
}

func TestTraversalMigrationMovesMiddleRowsAndPreservesTicks(t *testing.T) {
	world := NewWorld()
	spawner := NewSpawner2[testPosition, testVelocity](world)
	entities := []Entity{
		spawner.Spawn(testPosition{X: 1}, testVelocity{X: 11}),
		spawner.Spawn(testPosition{X: 2}, testVelocity{X: 12}),
		spawner.Spawn(testPosition{X: 3}, testVelocity{X: 13}),
	}
	ticks := make(map[Entity][2]Tick)
	for _, entity := range entities {
		_, meta, _ := world.entityMeta(entity)
		col := componentColumn[testPosition](world.archetypes[meta.archetypeID], registerComponent[testPosition](world))
		ticks[entity] = [2]Tick{col.added[meta.row], col.changed[meta.row]}
	}
	for _, entity := range entities[:2] {
		if err := Insert(world, entity, testLocalTag{}); err != nil {
			t.Fatal(err)
		}
	}
	Remove[testLocalTag](world, entities[0]) // Swap the other entity in the target archetype.
	Remove[testVelocity](world, entities[1]) // Remove a non-tag column too.
	for i, entity := range entities {
		pos, ok := Get[testPosition](world, entity)
		if !ok || pos.X != float32(i+1) {
			t.Fatalf("entity %d lost its row: %v", entity, pos)
		}
		_, meta, _ := world.entityMeta(entity)
		col := componentColumn[testPosition](world.archetypes[meta.archetypeID], registerComponent[testPosition](world))
		if got := [2]Tick{col.added[meta.row], col.changed[meta.row]}; got != ticks[entity] {
			t.Fatalf("ticks changed: %v != %v", got, ticks[entity])
		}
		velocity, exists := Get[testVelocity](world, entity)
		if exists != (i != 1) || exists && velocity.X != float32(i+11) {
			t.Fatalf("wrong velocity: %v %v", velocity, exists)
		}
	}
}
