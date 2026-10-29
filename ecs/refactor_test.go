package ecs

import (
	"testing"
)

func TestSetIfChangedPreservesTicksAndRejectsMissingComponents(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner1[testVisibility]()
	entity := spawner.Spawn(testVisibility{Visible: true})
	column, row := world.componentRow[testVisibility](entity)
	added, changed := column.added[row], column.changed[row]
	version, tick := world.ComponentVersion[testVisibility](), world.changeTick
	if world.SetIfChanged(entity, testVisibility{Visible: true}) || world.changeTick != tick || world.ComponentVersion[testVisibility]() != version {
		t.Fatal("equal writes changed ticks or versions")
	}
	if world.SetIfChanged(entity, benchmarkPosition{}) || world.Has[benchmarkPosition](entity) {
		t.Fatal("missing components were inserted")
	}
	if !world.SetIfChanged(entity, testVisibility{}) || column.added[row] != added || column.changed[row] == changed {
		t.Fatal("changed write did not preserve added tick and update changed tick")
	}
	world.Despawn(entity)
	replacement := spawner.Spawn(testVisibility{})
	if world.SetIfChanged(entity, testVisibility{Visible: true}) {
		t.Fatal("stale entity was updated")
	}
	if value, _ := world.Get[testVisibility](replacement); value.Visible {
		t.Fatal("stale write affected replacement")
	}
	if allocs := testing.AllocsPerRun(100, func() {
		world.SetIfChanged(replacement, testVisibility{Visible: true})
		world.SetIfChanged(replacement, testVisibility{})
	}); allocs != 0 {
		t.Fatalf("writes allocated %v times", allocs)
	}
}

func TestCommitComponentMarksOnlyChangedValues(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner2[testVisibility, benchmarkPosition]()
	entity := spawner.Spawn(testVisibility{}, benchmarkPosition{X: 1})
	version := world.ComponentVersion[benchmarkPosition]()
	for _, access := range world.Query2[testVisibility, benchmarkPosition]().All {
		visibility := access.A
		position := access.B

		if CommitComponent(position, benchmarkPosition{X: 1}) || !CommitComponent(visibility, testVisibility{Visible: true}) {
			t.Fatal("incorrect commit result")
		}
	}
	if world.ComponentVersion[benchmarkPosition]() != version {
		t.Fatal("equal component was marked changed")
	}
	if value, _ := world.Get[testVisibility](entity); !value.Visible {
		t.Fatal("changed value was not written")
	}
}

func BenchmarkRefactorComponentReads(b *testing.B) {
	world := NewWorld()
	spawner := world.NewSpawner1[benchmarkPosition]()
	entity := spawner.Spawn(benchmarkPosition{X: 1})
	b.Run("Value", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			value, _ := world.Get[benchmarkPosition](entity)
			benchmarkTotal = value.X
		}
	})
	b.Run("Reference", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			value := world.GetRef[benchmarkPosition](entity)
			benchmarkTotal = value.X
		}
	})
}
