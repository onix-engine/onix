package ecs

import (
	"fmt"
	"slices"
	"testing"
)

func BenchmarkScopedQuery(b *testing.B) {
	for _, matches := range []int{1000, 10000} {
		for _, changed := range []bool{false, true} {
			b.Run(fmt.Sprintf("Matches=%d/Changed=%t", matches, changed), func(b *testing.B) {
				w := NewWorld()
				matching, other := w.NewSpawner1[scopeNumber](), w.NewSpawner1[bool]()
				entities := make([]Entity, 10000)
				for i := range entities {
					if i < matches {
						entities[i] = matching.Spawn(scopeNumber(i))
					} else {
						entities[i] = other.Spawn(false)
					}
				}
				var scope QueryScope
				scope.Reset(entities)
				w.SetQueryScope(&scope)
				q := w.Query[scopeNumber]()
				if changed {
					last := w.ChangeTick()
					*w.GetMut[scopeNumber](entities[0]) = 1
					changes := w.ChangesSince(last)
					q = w.Query[scopeNumber](changes.Changed[scopeNumber]())
				}
				for range q.All {
				}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					var sum float32
					for _, access := range q.All {
						sum += float32(access.Get())
					}
					benchmarkTotal = sum
				}
			})
		}
	}
}

func BenchmarkSparseScope(b *testing.B) {
	w := NewWorld()
	spawn := w.NewSpawner1[scopeNumber]()
	entities := make([]Entity, 100000)
	for i := range entities {
		entities[i] = spawn.Spawn(scopeNumber(i))
	}
	var scope QueryScope
	// Widely separated members must not scan the intervening empty blocks.
	scope.Reset([]Entity{entities[len(entities)-1], entities[0]})
	w.SetQueryScope(&scope)
	query := w.Query[scopeNumber]()
	for range query.All {
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var sum float32
		for _, access := range query.All {
			sum += float32(access.Get())
		}
		benchmarkTotal = sum
	}
}

func TestQueryScopeIndexTracksRowsAndGenerations(t *testing.T) {
	w := NewWorld()
	spawn := w.NewSpawner1[scopeNumber]()
	entities := make([]Entity, 140)
	for i := range entities {
		entities[i] = spawn.Spawn(scopeNumber(i))
	}
	members := []Entity{entities[139], entities[0], entities[63], entities[64], entities[65], entities[139]}
	var scope QueryScope
	scope.Reset(members)
	w.SetQueryScope(&scope)
	check := func(want ...int) {
		t.Helper()
		var got []int
		for _, access := range w.Query[scopeNumber]().All {
			got = append(got, int(access.Get()))
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("scope values = %v, want %v", got, want)
		}
	}
	check(0, 63, 64, 65, 139)
	if err := w.Insert(entities[64], true); err != nil {
		t.Fatal(err)
	}
	w.Despawn(entities[1]) // swaps a scoped row into an unscoped row's position
	check(0, 63, 64, 65, 139)
	w.Despawn(entities[63])
	replacement := spawn.Spawn(999)
	if replacement == entities[63] {
		t.Fatal("entity generation was not recycled")
	}
	check(0, 64, 65, 139)
	var got []int
	for _, access := range w.Query[scopeNumber]().In([]Entity{entities[65], entities[0], entities[65]}).All {
		got = append(got, int(access.Get()))
	}
	if !slices.Equal(got, []int{65, 0, 65}) {
		t.Fatalf("explicit order = %v", got)
	}
}

func TestQueryScopeIndexIntersectsChangedBlocks(t *testing.T) {
	w := NewWorld()
	spawn := w.NewSpawner1[scopeNumber]()
	var members []Entity
	for i := range 8200 {
		e := spawn.Spawn(scopeNumber(i))
		if i%3 == 1 {
			members = append(members, e)
		}
	}
	var scope QueryScope
	scope.Reset(members)
	w.SetQueryScope(&scope)
	last := w.ChangeTick()
	for _, i := range []int{0, 21, 22, 1365, 2732} {
		*w.GetMut[scopeNumber](members[i]) += 10000
	}
	changes := w.ChangesSince(last)
	query := w.Query[scopeNumber](changes.Changed[scopeNumber]())
	count := 0
	for _, access := range query.All {
		if access.Get() < 10000 {
			t.Fatal("unchanged member visited")
		}
		count++
	}
	if count != 5 {
		t.Fatalf("changed count = %d", count)
	}
	if n := testing.AllocsPerRun(100, func() {
		for range query.All {
		}
	}); n != 0 {
		t.Fatalf("warm scoped query allocated %g", n)
	}
}

func TestSparseScopeSkipsEmptyBlocksAndRebuildsMembership(t *testing.T) {
	w := NewWorld()
	spawn := w.NewSpawner1[scopeNumber]()
	entities := make([]Entity, 10000)
	for i := range entities {
		entities[i] = spawn.Spawn(scopeNumber(i))
	}
	var scope QueryScope
	scope.Reset([]Entity{entities[9999], entities[64]})
	w.SetQueryScope(&scope)
	check := func(query QueryView1[scopeNumber], want ...scopeNumber) {
		t.Helper()
		var got []scopeNumber
		for _, access := range query.All {
			got = append(got, access.Get())
		}
		if !slices.Equal(got, want) {
			t.Fatalf("sparse scope = %v, want %v", got, want)
		}
	}
	check(w.Query[scopeNumber](), 64, 9999)
	last := w.ChangeTick()
	*w.GetMut[scopeNumber](entities[9999]) = 10000
	*w.GetMut[scopeNumber](entities[5000]) = 10000 // changed but outside the scope
	changes := w.ChangesSince(last)
	check(w.Query[scopeNumber](changes.Changed[scopeNumber]()), 10000)
	scope.Reset([]Entity{entities[5000]})
	check(w.Query[scopeNumber](), 10000)
	scope.Reset(nil)
	check(w.Query[scopeNumber]())
}
