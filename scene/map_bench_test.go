package scene

import (
	"fmt"
	"testing"

	"github.com/onix-engine/onix/ecs"
)

func BenchmarkRenderHierarchyMaps(b *testing.B) {
	for _, sparse := range []bool{false, true} {
		b.Run(fmt.Sprintf("sparse=%t", sparse), func(b *testing.B) {
			w, entities := hierarchyFixture(4096)
			if err := w.Insert(entities[0], PainterGroup2D(7)); err != nil {
				b.Fatal(err)
			}
			selected := entities
			if sparse {
				selected = entities[len(entities)-8:]
			}
			cache := frameRenderHierarchy2DCachePtr(w)
			for _, entity := range selected {
				cache.resolve(w, entity)
			}
			b.ReportAllocs()
			for b.Loop() {
				for _, entity := range selected {
					if cache.resolve(w, entity).group != 7 {
						b.Fatal("inherited value lost")
					}
				}
			}
		})
	}
}

var hierarchyMapSink *renderHierarchy2DCache

func BenchmarkRenderHierarchyPopulation(b *testing.B) {
	w := ecs.NewWorld()
	spawn := w.NewSpawner1[PainterGroup2D]()
	entities := make([]ecs.Entity, 1024)
	for i := range entities {
		entities[i] = spawn.Spawn(PainterGroup2D(7))
	}
	frameRenderHierarchy2DCachePtr(w)
	b.ReportAllocs()
	for b.Loop() {
		cache := frameRenderHierarchy2DCachePtr(w)
		*cache = renderHierarchy2DCache{}
		cache = frameRenderHierarchy2DCachePtr(w)
		for _, entity := range entities {
			cache.resolve(w, entity)
		}
		hierarchyMapSink = cache
	}
}

func BenchmarkRenderHierarchySparsePopulation(b *testing.B) {
	w := ecs.NewWorld()
	spawn := w.NewSpawner1[PainterGroup2D]()
	entities := make([]ecs.Entity, 65536)
	for i := range entities {
		entities[i] = spawn.Spawn(PainterGroup2D(7))
	}
	frameRenderHierarchy2DCachePtr(w)
	b.ReportAllocs()
	for b.Loop() {
		cache := frameRenderHierarchy2DCachePtr(w)
		*cache = renderHierarchy2DCache{}
		cache = frameRenderHierarchy2DCachePtr(w)
		for _, entity := range entities[len(entities)-8:] {
			cache.resolve(w, entity)
		}
		hierarchyMapSink = cache
	}
}
