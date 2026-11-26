package scene

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/internal/reuse"
)

func renderHierarchyPopulationFixture() (*ecs.World, []ecs.Entity) {
	w := ecs.NewWorld()
	spawn := w.NewSpawner1[PainterGroup2D]()
	entities := make([]ecs.Entity, 4096)
	for i := range entities {
		entities[i] = spawn.Spawn(PainterGroup2D(i + 1))
	}
	return w, entities
}

func TestRenderHierarchyHighFirstRebuildNoAllocations(t *testing.T) {
	w, entities := renderHierarchyPopulationFixture()
	cache := frameRenderHierarchy2DCachePtr(w)
	frame := func() {
		// Several systems may invalidate the cache before a frame is complete.
		// A partial population must not replace the previous density estimate.
		cache.resetValues()
		cache.resolve(w, entities[len(entities)-1])
		cache.resetValues()
		for i := len(entities) - 1; i >= 0; i-- {
			if got := cache.resolve(w, entities[i]).group; got != uint64(i+1) {
				t.Fatalf("entity %d lost its group: %d", i, got)
			}
		}
		cache.MaintainCache()
	}
	frame()
	if n := testing.AllocsPerRun(100, frame); n != 0 {
		t.Fatalf("warm high-first rebuild allocated %g", n)
	}
	cache.resetValues()
	cache.resolve(w, entities[len(entities)-1])
	cache.MaintainCache()
	if cache.values.Len() != 0 || entityStoreBytes(&cache.values) != 0 || len(cache.sparse) != 1 {
		t.Fatal("sparse population retained dense storage")
	}
}

func BenchmarkRenderHierarchyHighFirstRebuild(b *testing.B) {
	w, entities := renderHierarchyPopulationFixture()
	cache := frameRenderHierarchy2DCachePtr(w)
	frame := func() {
		cache.resetValues()
		for i := len(entities) - 1; i >= 0; i-- {
			cache.resolve(w, entities[i])
		}
		cache.MaintainCache()
	}
	frame()
	frame()
	b.ReportAllocs()
	for b.Loop() {
		frame()
	}
}

func BenchmarkRenderHierarchyAlternatingPopulation(b *testing.B) {
	w, entities := renderHierarchyPopulationFixture()
	cache := frameRenderHierarchy2DCachePtr(w)
	frame := func(selected []ecs.Entity) {
		cache.resetValues()
		for _, entity := range selected {
			cache.resolve(w, entity)
		}
		cache.MaintainCache()
	}
	for range 2 {
		frame(entities)
		frame(entities[len(entities)-1:])
	}
	b.ReportAllocs()
	for b.Loop() {
		frame(entities)
		frame(entities[len(entities)-1:])
	}
}

func TestRenderHierarchyPromotesAfterPopulationStabilizes(t *testing.T) {
	w, entities := renderHierarchyPopulationFixture()
	cache := frameRenderHierarchy2DCachePtr(w)
	frame := func(selected []ecs.Entity) {
		cache.resetValues()
		for _, entity := range selected {
			cache.resolve(w, entity)
		}
		cache.MaintainCache()
	}
	for range 3 {
		frame(entities[len(entities)-1:])
		frame(entities)
		if cache.sparse == nil {
			t.Fatal("one dense frame rebuilt a sparse cache")
		}
	}
	for range reuse.RetentionFrames + 1 {
		frame(entities)
	}
	if cache.sparse != nil || cache.values.Len() != len(entities) {
		t.Fatal("stable dense population kept the sparse map")
	}
}
