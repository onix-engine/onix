package ecs

import (
	"fmt"
	"testing"
)

type mapBenchmarkScratch struct{}

func BenchmarkFrameScratchMap(b *testing.B) {
	w := NewWorld()
	frame := func() {
		values := w.FrameScratchMap[mapBenchmarkScratch, int, int]()
		for i := range 128 {
			values[i] = i
		}
		w.Maintain()
	}
	frame()
	b.ReportAllocs()
	for b.Loop() {
		frame()
	}
}

func BenchmarkArchetypeEdges(b *testing.B) {
	w := NewWorld()
	spawn := w.NewSpawner1[int]()
	entity := spawn.Spawn(1)
	cycle := func() {
		if err := w.Insert(entity, true); err != nil {
			b.Fatal(err)
		}
		if _, ok := w.Remove[bool](entity); !ok {
			b.Fatal("missing component")
		}
	}
	cycle()
	b.ReportAllocs()
	for b.Loop() {
		cycle()
	}
}

var edgeBenchmarkSink *archetype

func BenchmarkArchetypeEdgeDistribution(b *testing.B) {
	for _, count := range []int{1, 4, 16, 64} {
		for _, cold := range []bool{false, true} {
			b.Run(fmt.Sprintf("%d/cold=%t", count, cold), func(b *testing.B) {
				world := NewWorld()
				ids := auditRegisterColumns(world, count)
				source := world.archetypeFor(nil)
				for _, id := range ids {
					world.archetypeWithAdded(source, id)
				}
				b.ReportAllocs()
				var i int
				for b.Loop() {
					if cold {
						source.edges = nil
						for _, id := range ids {
							edgeBenchmarkSink = world.archetypeWithAdded(source, id)
						}
					} else {
						edgeBenchmarkSink = world.archetypeWithAdded(source, ids[(i*17)&(count-1)])
						i++
					}
				}
			})
		}
	}
}
