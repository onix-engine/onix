package scene

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
)

func TestRenderHierarchySparseDenseAndGeneration(t *testing.T) {
	w := ecs.NewWorld()
	spawn := w.NewSpawner1[PainterGroup2D]()
	entities := make([]ecs.Entity, 4096)
	for i := range entities {
		entities[i] = spawn.Spawn(PainterGroup2D(i + 1))
	}
	cache := frameRenderHierarchy2DCachePtr(w)
	last := entities[len(entities)-1]
	cache.resolve(w, last)
	if cache.values.Len() != 0 || len(cache.sparse) != 1 {
		t.Fatal("one high entity reserved a dense world index")
	}
	for _, entity := range entities {
		cache.resolve(w, entity)
	}
	if cache.sparse != nil || cache.values.Len() != len(entities) {
		t.Fatal("dense population did not adopt slot indexing")
	}
	for i, entity := range entities {
		if cache.resolve(w, entity).group != uint64(i+1) {
			t.Fatal("representation change lost an entry")
		}
	}
	w.Despawn(last)
	replacement := spawn.Spawn(PainterGroup2D(99))
	if cache.resolve(w, replacement).group != 99 || cache.values.Get(last) != nil {
		t.Fatal("recycled slot inherited stale data")
	}
	cache.resetValues()
	cache.resolve(w, replacement)
	if cache.values.Len() != 0 || len(cache.sparse) != 1 {
		t.Fatal("sparse reset retained dense storage")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		cache.resetValues()
		cache.resolve(w, replacement)
		cache.MaintainCache()
	}); allocations != 0 {
		t.Fatalf("stable sparse storage allocated %g", allocations)
	}
}
