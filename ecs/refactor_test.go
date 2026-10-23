package ecs

import (
	"testing"
)

func TestSetIfChangedPreservesTicksAndRejectsMissingComponents(t *testing.T) {
	world := NewWorld()
	spawner := NewSpawner1[testVisibility](world)
	entity := spawner.Spawn(testVisibility{Visible: true})
	column, row := componentRow[testVisibility](world, entity)
	added, changed := column.added[row], column.changed[row]
	version, tick := ComponentVersion[testVisibility](world), world.changeTick
	if SetIfChanged(world, entity, testVisibility{Visible: true}) || world.changeTick != tick || ComponentVersion[testVisibility](world) != version {
		t.Fatal("equal writes changed ticks or versions")
	}
	if SetIfChanged(world, entity, benchmarkPosition{}) || Has[benchmarkPosition](world, entity) {
		t.Fatal("missing components were inserted")
	}
	if !SetIfChanged(world, entity, testVisibility{}) || column.added[row] != added || column.changed[row] == changed {
		t.Fatal("changed write did not preserve added tick and update changed tick")
	}
	world.Despawn(entity)
	replacement := spawner.Spawn(testVisibility{})
	if SetIfChanged(world, entity, testVisibility{Visible: true}) {
		t.Fatal("stale entity was updated")
	}
	if value, _ := Get[testVisibility](world, replacement); value.Visible {
		t.Fatal("stale write affected replacement")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		SetIfChanged(world, replacement, testVisibility{Visible: true})
		SetIfChanged(world, replacement, testVisibility{})
	}); allocs != 0 {
		t.Fatalf("writes allocated %v times", allocs)
	}
}

func TestCommitComponentMarksOnlyChangedValues(t *testing.T) {
	world := NewWorld()
	spawner := NewSpawner2[testVisibility, benchmarkPosition](world)
	entity := spawner.Spawn(testVisibility{}, benchmarkPosition{X: 1})
	version := ComponentVersion[benchmarkPosition](world)
	for _, access := range Query2[testVisibility, benchmarkPosition](world).All {
		visibility := access.A
		position := access.B

		if CommitComponent(position, benchmarkPosition{X: 1}) || !CommitComponent(visibility, testVisibility{Visible: true}) {
			t.Fatal("incorrect commit result")
		}
	}
	if ComponentVersion[benchmarkPosition](world) != version {
		t.Fatal("equal component was marked changed")
	}
	if value, _ := Get[testVisibility](world, entity); !value.Visible {
		t.Fatal("changed value was not written")
	}
}

func BenchmarkRefactorComponentReads(b *testing.B) {
	world := NewWorld()
	spawner := NewSpawner1[benchmarkPosition](world)
	entity := spawner.Spawn(benchmarkPosition{X: 1})
	b.Run("Value", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			value, _ := Get[benchmarkPosition](world, entity)
			benchmarkTotal = value.X
		}
	})
	b.Run("Reference", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			value := GetRef[benchmarkPosition](world, entity)
			benchmarkTotal = value.X
		}
	})
}
