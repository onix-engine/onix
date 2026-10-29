package ecs

import (
	"fmt"
	"testing"
)

func TestPatchWriteOrderAcrossSourceSubsets(t *testing.T) {
	for shape := range 32 {
		t.Run(fmt.Sprint(shape), func(t *testing.T) {
			world := NewWorld()
			world.registerComponent[testName]()
			world.registerComponent[testSize]()
			world.registerComponent[testPosition]()
			world.registerComponent[testVelocity]()
			bundle := NewBundle(5)
			if shape&1 != 0 {
				bundle.Put(testName{Value: "old"})
			}
			if shape&2 != 0 {
				bundle.Put(testSize{Width: 5})
			}
			if shape&4 != 0 {
				bundle.Put(testPosition{X: 6})
			}
			if shape&8 != 0 {
				bundle.Put(testVelocity{X: 7})
			}
			if shape&16 != 0 {
				bundle.Put(testAnchor{8, 0})
			}
			entity := world.Spawn(bundle)
			other := world.Spawn(bundle)
			cleanups := 0
			world.OnComponentRemove[testSize](func(w *World, e Entity, value *testSize) {
				cleanups++
				if e != entity || value.Width != 5 || w.Has[testName](e) != (shape&1 != 0) {
					t.Fatal("cleanup observed the wrong source row")
				}
			})
			// Caller order differs from ID order; a written type also appears in
			// the removal set, and some sources have no retained components.
			patch := world.NewPatch3[testVelocity, testName, testPosition](Type[testSize](), Type[testPosition]())
			if err := patch.Apply(entity, testVelocity{X: 17}, testName{Value: "new"}, testPosition{X: 16}); err != nil {
				t.Fatal(err)
			}
			if world.GetRef[testVelocity](entity).X != 17 || world.GetRef[testName](entity).Value != "new" || world.GetRef[testPosition](entity).X != 16 || world.Has[testSize](entity) {
				t.Fatal("patch mixed write arguments or removed a written component")
			}
			if world.Has[testAnchor](entity) != (shape&16 != 0) || world.Has[testSize](other) != (shape&2 != 0) || cleanups != (shape>>1)&1 {
				t.Fatal("patch corrupted retained/swapped components or cleanup count")
			}
			if shape&16 != 0 && world.GetRef[testAnchor](entity)[0] != 8 {
				t.Fatal("retained component value changed")
			}
		})
	}
}

func TestPatchMigratesOncePreservingTicksAndSwappedEntity(t *testing.T) {
	w := NewWorld()
	spawn := w.NewSpawner3[testPosition, testVelocity, testName]()
	first := spawn.Spawn(testPosition{X: 1}, testVelocity{X: 2}, testName{Value: "first"})
	second := spawn.Spawn(testPosition{X: 3}, testVelocity{X: 4}, testName{Value: "second"})
	positionColumn, positionRow := w.componentRow[testPosition](first)
	nameColumn, nameRow := w.componentRow[testName](first)
	positionAdded := positionColumn.added[positionRow]
	nameAdded, nameChanged := nameColumn.added[nameRow], nameColumn.changed[nameRow]
	patch := w.NewPatch2[testPosition, testSize](Type[testVelocity](), Type[testPosition](), Type[testVisibility]())
	patch.Reserve(2)
	cleaned := 0
	w.OnComponentRemove[testVelocity](func(world *World, entity Entity, velocity *testVelocity) {
		cleaned++
		if entity != first || velocity.X != 2 || !world.Has[testVelocity](entity) || world.Has[testSize](entity) || world.GetRef[testPosition](entity).X != 1 {
			t.Fatal("cleanup did not observe the complete original row")
		}
	})
	w.OnComponentRemove[testPosition](func(*World, Entity, *testPosition) { t.Fatal("replacement ran removal cleanup") })
	structure := w.StructureVersion()
	if err := patch.Apply(first, testPosition{X: 10}, testSize{}); err != nil {
		t.Fatal(err)
	}
	if w.StructureVersion() != structure+1 || cleaned != 1 || w.Has[testVelocity](first) || !w.Has[testSize](first) {
		t.Fatal("patch did not perform one migration")
	}
	positionColumn, positionRow = w.componentRow[testPosition](first)
	nameColumn, nameRow = w.componentRow[testName](first)
	sizeColumn, sizeRow := w.componentRow[testSize](first)
	if positionColumn.values[positionRow].X != 10 || positionColumn.added[positionRow] != positionAdded || positionColumn.changed[positionRow] != sizeColumn.added[sizeRow] {
		t.Fatal("replacement lost Added or did not share the insertion tick")
	}
	if nameColumn.values[nameRow].Value != "first" || nameColumn.added[nameRow] != nameAdded || nameColumn.changed[nameRow] != nameChanged {
		t.Fatal("retained component ticks changed")
	}
	if w.GetRef[testPosition](second).X != 3 || w.GetRef[testName](second).Value != "second" || w.GetRef[testVelocity](second).X != 4 {
		t.Fatal("swapped entity row was corrupted")
	}
	structure = w.StructureVersion()
	added := sizeColumn.added[sizeRow]
	if err := patch.Apply(first, testPosition{X: 11}, testSize{}); err != nil {
		t.Fatal(err)
	}
	if w.StructureVersion() != structure || sizeColumn.added[sizeRow] != added || cleaned != 1 {
		t.Fatal("replacement-only patch migrated or reset Added")
	}
}

func TestPatchCleanupPanicLeavesStorageAndReleasesGuard(t *testing.T) {
	w := NewWorld()
	spawn := w.NewSpawner2[testPosition, testVelocity]()
	entity := spawn.Spawn(testPosition{X: 1}, testVelocity{})
	patch := w.NewPatch1[testSize](Type[testVelocity]())
	w.OnComponentRemove[testVelocity](func(*World, Entity, *testVelocity) { panic("cleanup") })
	version := w.MutationVersion()
	func() {
		defer func() {
			if got := recover(); got != "cleanup" {
				t.Fatalf("panic = %v", got)
			}
		}()
		_ = patch.Apply(entity, testSize{})
	}()
	if w.MutationVersion() != version || w.Has[testSize](entity) || !w.Has[testVelocity](entity) {
		t.Fatal("cleanup panic partially applied patch")
	}
	w.OnComponentRemove[testVelocity](nil)
	if err := patch.Apply(entity, testSize{}); err != nil {
		t.Fatal(err)
	}
}

func TestPatchCommandsKeepValuesAndPlansAndClearReferences(t *testing.T) {
	w := NewWorld()
	spawn := w.NewSpawner2[testPosition, testVelocity]()
	first := spawn.Spawn(testPosition{}, testVelocity{})
	second := spawn.Spawn(testPosition{}, testVelocity{})
	removeVelocity := w.NewPatch1[*int](Type[testVelocity]())
	keepVelocity := w.NewPatch1[*int]()
	a, b := 1, 2
	for entity := range w.Query[testPosition]().All {
		if entity == first {
			removeVelocity.Queue(w.Commands(), entity, &a)
		} else {
			keepVelocity.Queue(w.Commands(), entity, &b)
		}
	}
	if err := w.FlushCommands(); err != nil {
		t.Fatal(err)
	}
	if w.Has[testVelocity](first) || !w.Has[testVelocity](second) || *w.GetRef[*int](first) != &a || *w.GetRef[*int](second) != &b {
		t.Fatal("queued patch lost per-command values or plan")
	}
	stream := w.Commands().commandStreamFor(func() *valueCommandStream[patchCommand1[*int]] { return &valueCommandStream[patchCommand1[*int]]{} })
	for _, value := range stream.values[:cap(stream.values)] {
		if value.a != nil || value.patch.plan != nil {
			t.Fatal("flushed command storage retained references")
		}
	}
	// Discarded commands must release payloads without losing earlier commands.
	keepVelocity.Queue(w.Commands(), second, &a)
	checkpoint := w.Commands().checkpoint()
	removeVelocity.Queue(w.Commands(), first, &b)
	w.Commands().discardFrom(checkpoint)
	if err := w.FlushCommands(); err != nil || *w.GetRef[*int](second) != &a || *w.GetRef[*int](first) != &a {
		t.Fatal("command discard lost retained values or applied discarded values")
	}
}

func TestPatchRejectsStaleEntitiesForeignCommandsAndActiveQueries(t *testing.T) {
	w := NewWorld()
	spawn := w.NewSpawner1[testPosition]()
	entity := spawn.Spawn(testPosition{})
	patch := w.NewPatch1[testSize]()
	w.Despawn(entity)
	replacement := spawn.Spawn(testPosition{})
	version := w.MutationVersion()
	if err := patch.Apply(entity, testSize{}); err == nil || w.Has[testSize](replacement) || w.MutationVersion() != version {
		t.Fatal("stale patch changed replacement or world")
	}
	for _, run := range []func(){
		func() { patch.Queue(NewWorld().Commands(), replacement, testSize{}) },
		func() {
			for entity := range w.Query[testPosition]().All {
				_ = patch.Apply(entity, testSize{})
			}
		},
		func() { w.NewPatch2[testSize, testSize]() },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("invalid patch operation did not panic")
				}
			}()
			run()
		}()
	}
	if err := patch.Apply(replacement, testSize{}); err != nil {
		t.Fatal("query guard was not released:", err)
	}
}

func TestPatchAndQueuedPatchDoNotAllocateAfterWarmup(t *testing.T) {
	for _, queued := range []bool{false, true} {
		w := NewWorld()
		spawn := w.NewSpawner2[testPosition, testVelocity]()
		entity := spawn.Spawn(testPosition{}, testVelocity{})
		forward := w.NewPatch2[testPosition, testSize](Type[testVelocity]())
		backward := w.NewPatch2[testPosition, testVelocity](Type[testSize]())
		forward.Reserve(1)
		backward.Reserve(1)
		run := func() {
			if queued {
				forward.Queue(w.Commands(), entity, testPosition{}, testSize{})
				backward.Queue(w.Commands(), entity, testPosition{}, testVelocity{})
				if err := w.FlushCommands(); err != nil {
					panic(err)
				}
			} else {
				if err := forward.Apply(entity, testPosition{}, testSize{}); err != nil {
					panic(err)
				}
				if err := backward.Apply(entity, testPosition{}, testVelocity{}); err != nil {
					panic(err)
				}
			}
		}
		if allocations := testing.AllocsPerRun(100, run); allocations != 0 {
			t.Fatalf("queued=%v allocations=%v", queued, allocations)
		}
		if allocations := testing.AllocsPerRun(100, func() {
			if err := w.Insert2(entity, testPosition{}, testVelocity{}); err != nil {
				panic(err)
			}
		}); allocations != 0 {
			t.Fatalf("Insert2 allocations=%v", allocations)
		}
	}
}
