package ecs

import (
	"math/rand/v2"
	"slices"
	"testing"
)

func TestQueryTablesTrackOccupancyAndOrder(t *testing.T) {
	world := NewWorld()
	world.registerComponent[int]()
	extra := auditRegisterColumns(world, 8)
	tables := make([]*archetype, 256)
	query := world.Query[int]() // Keep this plan before creating matching tables.
	for shape := range tables {
		ids := []componentID{0}
		for bit, id := range extra {
			if shape&(1<<bit) != 0 {
				ids = append(ids, id)
			}
		}
		tables[shape] = world.archetypeForOwned(ids)
	}
	check := func() {
		t.Helper()
		var want, got []Entity
		for _, table := range world.archetypes {
			want = append(want, table.entities...)
		}
		for entity := range query.All {
			got = append(got, entity)
			// Nested iteration must not consume the outer cursor's remaining bits.
			count := 0
			for range world.Query[int](Unscoped()).All {
				count++
			}
			if count != len(want) {
				t.Fatalf("nested query visited %d entities, want %d", count, len(want))
			}
		}
		if !slices.Equal(got, want) {
			t.Fatalf("query order=%v, table order=%v", got, want)
		}
		got = got[:0]
		for entity := range world.Entities() {
			got = append(got, entity)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("entity order=%v, table order=%v", got, want)
		}
		count := 0
		for range query.All {
			count++
			break
		}
		if (count == 0) != (len(want) == 0) {
			t.Fatal("early exit lost table occupancy")
		}
	}
	check()
	var entities []Entity
	for _, shape := range []int{0, 1, 62, 63, 64, 126, 127, 128, 190, 191, 192, 254, 255} {
		entities = append(entities, spawnQueryDistributionEntity(world, tables[shape]))
	}
	check()
	for _, entity := range entities {
		world.Despawn(entity)
		check()
	}
	for _, table := range tables {
		spawnQueryDistributionEntity(world, table)
	}
	check()
	// Filters still intersect tick blocks after matching/nonempty table bits.
	last := world.ChangeTick()
	changed := tables[192].entities[0]
	*world.GetMut[int](changed) = 7
	window := world.ChangesSince(last)
	var changedEntities []Entity
	for entity := range world.Query[int](window.Changed[int]()).All {
		changedEntities = append(changedEntities, entity)
	}
	if !slices.Equal(changedEntities, []Entity{changed}) {
		t.Fatalf("change filter visited %v, want %v", changedEntities, changed)
	}
	if n := testing.AllocsPerRun(100, func() {
		for range query.All {
		}
	}); n != 0 {
		t.Fatalf("warm table traversal allocated %g times", n)
	}
}

func TestScopedQueriesMatchEntityOracleAcrossMutations(t *testing.T) {
	world := NewWorld()
	spawn := world.NewSpawner1[int]()
	var entities []Entity
	for i := range 150 {
		entities = append(entities, spawn.Spawn(i))
	}
	var a, b QueryScope
	a.Reset(append(slices.Clone(entities[:70]), entities[0], 0))
	b.Reset(slices.Clone(entities[70:]))
	patch := world.NewPatch1[bool]()
	remove := world.PrepareRemoval(Type[bool]())
	random := rand.New(rand.NewPCG(1, 2))
	check := func(scope *QueryScope) {
		t.Helper()
		world.SetQueryScope(scope)
		members := make(map[Entity]bool)
		for _, entity := range scope.entities {
			if world.IsAlive(entity) {
				members[entity] = true
			}
		}
		var got, want []Entity
		for _, table := range world.archetypes {
			for _, entity := range table.entities {
				if members[entity] && world.Has[int](entity) && !world.Has[bool](entity) {
					want = append(want, entity)
				}
			}
		}
		for entity := range world.Query[int](Without[bool]()).All {
			got = append(got, entity)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("scoped query=%v, entity oracle=%v", got, want)
		}
		for _, entity := range entities {
			if world.InQueryScope(entity) != members[entity] {
				t.Fatalf("wrong scope membership for %v", entity)
			}
		}
	}
	for step := range 240 {
		entity := entities[random.IntN(len(entities))]
		switch step % 6 {
		case 0:
			if err := patch.Apply(entity, true); err != nil {
				t.Fatal(err)
			}
		case 1:
			remove.Apply(entity)
		case 2:
			if err := world.Insert(entity, float64(step)); err != nil {
				t.Fatal(err)
			}
		case 3:
			world.Despawn(entity)
			for i := range entities {
				if entities[i] == entity {
					entities[i] = spawn.Spawn(step)
					break
				}
			}
		case 4:
			world.Remove[float64](entity)
		case 5:
			world.Commands().Insert(entity, true)
			if err := world.FlushCommands(); err != nil {
				t.Fatal(err)
			}
		}
		if step%11 == 0 {
			a.Reset(slices.Clone(entities[:70]))
		}
		check(&a)
		check(&b)
	}
}

func TestScopeHandlesNewTablesAndPendingSpawns(t *testing.T) {
	world := NewWorld()
	spawn := world.NewSpawner1[int]()
	member := spawn.Spawn(1)
	var scope QueryScope
	scope.Reset([]Entity{member})
	world.SetQueryScope(&scope)
	scope.bind(world)
	others := world.NewSpawner1[string]()
	other := others.Spawn("outside")
	if world.InQueryScope(other) {
		t.Fatal("new table leaked into existing scope")
	}
	for range world.Query[string]().All {
		t.Fatal("query visited new out-of-scope table")
	}
	// A reserved command handle was valid before becoming a live entity.
	bundle := NewBundle(1)
	bundle.Put(3)
	pending := world.Commands().Spawn(bundle)
	scope.Reset([]Entity{pending})
	scope.bind(world)
	if err := world.FlushCommands(); err != nil {
		t.Fatal(err)
	}
	if !world.InQueryScope(pending) {
		t.Fatal("reserved handle did not become a scope member after spawn")
	}
	// Reusing a scope on a different, smaller world must discard old row bits.
	second := NewWorld()
	second.SetQueryScope(&scope)
	for range second.Query[int]().All {
		t.Fatal("scope leaked rows across worlds")
	}
}
