package ecs

import (
	"testing"
)

func TestComponentRemovalCleanupPreservesMigration(t *testing.T) {
	world := NewWorld()
	calls := 0
	world.OnComponentRemove[testPosition](func(_ *World, _ Entity, _ *testPosition) { calls++ })
	spawner := world.NewSpawner1[testPosition]()
	entity := spawner.Spawn(testPosition{})
	if err := world.Insert(entity, testVelocity{}); err != nil {
		t.Fatal(err)
	}
	world.Remove[testVelocity](entity)
	if calls != 0 {
		t.Fatalf("migration ran cleanup %d times", calls)
	}
	world.Remove[testPosition](entity)
	if calls != 1 {
		t.Fatalf("removal ran cleanup %d times", calls)
	}
	if err := world.Insert(entity, testPosition{}); err != nil {
		t.Fatal(err)
	}
	world.Despawn(entity)
	if calls != 2 {
		t.Fatalf("despawn ran cleanup %d times", calls)
	}
}

func TestTickRangeNeverSkipsMatchingRows(t *testing.T) {
	ticks := []Tick{0, 1, 2, 63, 64, 128, Tick(maxChangeAge), Tick(^uint32(0) - 1), Tick(^uint32(0))}
	for _, a := range ticks {
		for _, b := range ticks {
			bounds := tickRange{}
			bounds.include(a)
			bounds.include(b)
			for _, last := range ticks {
				for _, current := range ticks {
					filter := rowFilter{lastRun: last, thisRun: current}
					if (tickInRange(a, last, current) || tickInRange(b, last, current)) && !bounds.matches(filter) {
						t.Fatalf("range %+v skipped matching ticks %d,%d in %d..%d", bounds, a, b, last, current)
					}
				}
			}
		}
	}
}

func TestChangedQuerySparseBlocksAfterSwapAndMigration(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner1[testPosition]()
	entities := make([]Entity, 256)
	for index := range entities {
		entities[index] = spawner.Spawn(testPosition{})
	}
	last := world.changeTick
	for _, index := range []int{3, 130, 255} {
		value := world.GetMut[testPosition](entities[index])
		value.X = 1
	}
	context := &SystemContext{World: world, lastRun: last, thisRun: world.changeTick}
	view := world.Query[testPosition](context.Changed[testPosition]())
	world.Despawn(entities[0])
	if err := world.Insert(entities[130], testVelocity{}); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, access := range view.All {
		value := access.Get()

		if value.X != 1 {
			t.Fatal("unchanged row matched")
		}
		count++
	}
	if count != 3 {
		t.Fatalf("changed rows=%d, want 3", count)
	}
	context.lastRun, context.thisRun = context.thisRun, world.nextChangeTick()
	count = 0
	for range world.Query[testPosition](context.Changed[testPosition]()).All {
		count++
	}
	if count != 0 {
		t.Fatalf("migration changed semantic ticks: %d", count)
	}
}

func BenchmarkChangedQuerySparse(b *testing.B) {
	world := NewWorld()
	spawner := world.NewSpawner1[testPosition]()
	spawner.Reserve(10000)
	var entity Entity
	for range 10000 {
		entity = spawner.Spawn(testPosition{})
	}
	context := &SystemContext{World: world, lastRun: world.changeTick}
	world.GetMut[testPosition](entity)
	context.thisRun = world.changeTick
	visit := func(Entity, testPosition) bool { return true }
	for entity, access := range world.Query[testPosition](context.Changed[testPosition]()).All {
		if !visit(entity, access.Get()) {
			break
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for entity, access := range world.Query[testPosition](context.Changed[testPosition]()).All {
			if !visit(entity, access.Get()) {
				break
			}
		}
	}
}
