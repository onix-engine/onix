package ecs

import "testing"

type scopeNumber int
type scopeCacheTag struct{}

func TestQueryScopeUsesCurrentScopeForCachedQueries(t *testing.T) {
	w := NewWorld()
	spawn := NewSpawner1[scopeNumber](w)
	a, b := spawn.Spawn(1), spawn.Spawn(2)
	query := Query[scopeNumber](w)
	var first, second QueryScope
	first.Reset([]Entity{a})
	second.Reset([]Entity{b})
	for _, scope := range []*QueryScope{&first, &second} {
		w.SetQueryScope(scope)
		count := 0
		for e := range query.All {
			if e != scope.entities[0] {
				t.Fatal("cached query escaped current scope")
			}
			count++
		}
		if count != 1 {
			t.Fatalf("scoped count = %d", count)
		}
	}
	second.Reset(nil)
	for range query.All {
		t.Fatal("empty scope visited an entity")
	}
	// An explicit list is a deliberate loading/ownership operation.
	for e := range query.In([]Entity{a}).All {
		if e != a {
			t.Fatal("explicit query list was changed")
		}
	}
	w.SetQueryScope(nil)
	count := 0
	for range query.All {
		count++
	}
	if count != 2 {
		t.Fatal("unrestricted query did not restore all entities")
	}
}

func TestQueryScopesReuseIndependentCachesWithoutAllocating(t *testing.T) {
	w := NewWorld()
	spawn := NewSpawner1[scopeNumber](w)
	entity := spawn.Spawn(3)
	var a, b QueryScope
	a.Reset([]Entity{entity})
	b.Reset(nil)
	w.SetQueryScope(&a)
	value := ScopeValue[scopeCacheTag, int](w)
	*value = 42
	w.SetQueryScope(&b)
	if *ScopeValue[scopeCacheTag, int](w) != 0 {
		t.Fatal("scope caches were shared")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		w.SetQueryScope(&a)
		if *ScopeValue[scopeCacheTag, int](w) != 42 {
			panic("lost scope cache")
		}
		for _, n := range Query[scopeNumber](w).All {
			if n.Get() != 3 {
				panic("wrong value")
			}
		}
		w.SetQueryScope(&b)
		for range Query[scopeNumber](w).All {
			panic("empty scope")
		}
	}); allocations != 0 {
		t.Fatalf("warm scopes allocated %g times", allocations)
	}
}
