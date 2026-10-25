package ecs

import (
	"fmt"
	"testing"
)

func TestPatchWriteOrderAcrossSourceSubsets(t *testing.T) {
	for shape := range 32 {
		t.Run(fmt.Sprint(shape), func(t *testing.T) {
			world := NewWorld()
			registerComponent[testName](world)
			registerComponent[testSize](world)
			registerComponent[testPosition](world)
			registerComponent[testVelocity](world)
			bundle := NewBundle(5)
			if shape&1 != 0 {
				Put(&bundle, testName{Value: "old"})
			}
			if shape&2 != 0 {
				Put(&bundle, testSize{Width: 5})
			}
			if shape&4 != 0 {
				Put(&bundle, testPosition{X: 6})
			}
			if shape&8 != 0 {
				Put(&bundle, testVelocity{X: 7})
			}
			if shape&16 != 0 {
				Put(&bundle, testAnchor{8, 0})
			}
			entity := world.Spawn(bundle)
			other := world.Spawn(bundle)
			cleanups := 0
			OnComponentRemove[testSize](world, func(w *World, e Entity, value *testSize) {
				cleanups++
				if e != entity || value.Width != 5 || Has[testName](w, e) != (shape&1 != 0) {
					t.Fatal("cleanup observed the wrong source row")
				}
			})
			// Caller order differs from ID order; a written type also appears in
			// the removal set, and some sources have no retained components.
			patch := NewPatch3[testVelocity, testName, testPosition](world, Type[testSize](), Type[testPosition]())
			if err := patch.Apply(entity, testVelocity{X: 17}, testName{Value: "new"}, testPosition{X: 16}); err != nil {
				t.Fatal(err)
			}
			if GetRef[testVelocity](world, entity).X != 17 || GetRef[testName](world, entity).Value != "new" || GetRef[testPosition](world, entity).X != 16 || Has[testSize](world, entity) {
				t.Fatal("patch mixed write arguments or removed a written component")
			}
			if Has[testAnchor](world, entity) != (shape&16 != 0) || Has[testSize](world, other) != (shape&2 != 0) || cleanups != (shape>>1)&1 {
				t.Fatal("patch corrupted retained/swapped components or cleanup count")
			}
			if shape&16 != 0 && GetRef[testAnchor](world, entity)[0] != 8 {
				t.Fatal("retained component value changed")
			}
		})
	}
}

func TestPatchMigratesOncePreservingTicksAndSwappedEntity(t *testing.T) {
	w := NewWorld()
	spawn := NewSpawner3[testPosition, testVelocity, testName](w)
	first := spawn.Spawn(testPosition{X: 1}, testVelocity{X: 2}, testName{Value: "first"})
	second := spawn.Spawn(testPosition{X: 3}, testVelocity{X: 4}, testName{Value: "second"})
	positionColumn, positionRow := componentRow[testPosition](w, first)
	nameColumn, nameRow := componentRow[testName](w, first)
	positionAdded := positionColumn.added[positionRow]
	nameAdded, nameChanged := nameColumn.added[nameRow], nameColumn.changed[nameRow]
	patch := NewPatch2[testPosition, testSize](w, Type[testVelocity](), Type[testPosition](), Type[testVisibility]())
	patch.Reserve(2)
	cleaned := 0
	OnComponentRemove[testVelocity](w, func(world *World, entity Entity, velocity *testVelocity) {
		cleaned++
		if entity != first || velocity.X != 2 || !Has[testVelocity](world, entity) || Has[testSize](world, entity) || GetRef[testPosition](world, entity).X != 1 {
			t.Fatal("cleanup did not observe the complete original row")
		}
	})
	OnComponentRemove[testPosition](w, func(*World, Entity, *testPosition) { t.Fatal("replacement ran removal cleanup") })
	structure := w.StructureVersion()
	if err := patch.Apply(first, testPosition{X: 10}, testSize{}); err != nil {
		t.Fatal(err)
	}
	if w.StructureVersion() != structure+1 || cleaned != 1 || Has[testVelocity](w, first) || !Has[testSize](w, first) {
		t.Fatal("patch did not perform one migration")
	}
	positionColumn, positionRow = componentRow[testPosition](w, first)
	nameColumn, nameRow = componentRow[testName](w, first)
	sizeColumn, sizeRow := componentRow[testSize](w, first)
	if positionColumn.values[positionRow].X != 10 || positionColumn.added[positionRow] != positionAdded || positionColumn.changed[positionRow] != sizeColumn.added[sizeRow] {
		t.Fatal("replacement lost Added or did not share the insertion tick")
	}
	if nameColumn.values[nameRow].Value != "first" || nameColumn.added[nameRow] != nameAdded || nameColumn.changed[nameRow] != nameChanged {
		t.Fatal("retained component ticks changed")
	}
	if GetRef[testPosition](w, second).X != 3 || GetRef[testName](w, second).Value != "second" || GetRef[testVelocity](w, second).X != 4 {
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
	spawn := NewSpawner2[testPosition, testVelocity](w)
	entity := spawn.Spawn(testPosition{X: 1}, testVelocity{})
	patch := NewPatch1[testSize](w, Type[testVelocity]())
	OnComponentRemove[testVelocity](w, func(*World, Entity, *testVelocity) { panic("cleanup") })
	version := w.MutationVersion()
	func() {
		defer func() {
			if got := recover(); got != "cleanup" {
				t.Fatalf("panic = %v", got)
			}
		}()
		_ = patch.Apply(entity, testSize{})
	}()
	if w.MutationVersion() != version || Has[testSize](w, entity) || !Has[testVelocity](w, entity) {
		t.Fatal("cleanup panic partially applied patch")
	}
	OnComponentRemove[testVelocity](w, nil)
	if err := patch.Apply(entity, testSize{}); err != nil {
		t.Fatal(err)
	}
}

func TestPatchCommandsKeepValuesAndPlansAndClearReferences(t *testing.T) {
	w := NewWorld()
	spawn := NewSpawner2[testPosition, testVelocity](w)
	first := spawn.Spawn(testPosition{}, testVelocity{})
	second := spawn.Spawn(testPosition{}, testVelocity{})
	removeVelocity := NewPatch1[*int](w, Type[testVelocity]())
	keepVelocity := NewPatch1[*int](w)
	a, b := 1, 2
	for entity := range Query[testPosition](w).All {
		if entity == first {
			removeVelocity.Queue(w.Commands(), entity, &a)
		} else {
			keepVelocity.Queue(w.Commands(), entity, &b)
		}
	}
	if err := w.FlushCommands(); err != nil {
		t.Fatal(err)
	}
	if Has[testVelocity](w, first) || !Has[testVelocity](w, second) || *GetRef[*int](w, first) != &a || *GetRef[*int](w, second) != &b {
		t.Fatal("queued patch lost per-command values or plan")
	}
	stream := commandStreamFor(w.Commands(), func() *valueCommandStream[patchCommand1[*int]] { return &valueCommandStream[patchCommand1[*int]]{} })
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
	if err := w.FlushCommands(); err != nil || *GetRef[*int](w, second) != &a || *GetRef[*int](w, first) != &a {
		t.Fatal("command discard lost retained values or applied discarded values")
	}
}

func TestPatchRejectsStaleEntitiesForeignCommandsAndActiveQueries(t *testing.T) {
	w := NewWorld()
	spawn := NewSpawner1[testPosition](w)
	entity := spawn.Spawn(testPosition{})
	patch := NewPatch1[testSize](w)
	w.Despawn(entity)
	replacement := spawn.Spawn(testPosition{})
	version := w.MutationVersion()
	if err := patch.Apply(entity, testSize{}); err == nil || Has[testSize](w, replacement) || w.MutationVersion() != version {
		t.Fatal("stale patch changed replacement or world")
	}
	for _, run := range []func(){
		func() { patch.Queue(NewWorld().Commands(), replacement, testSize{}) },
		func() {
			for entity := range Query[testPosition](w).All {
				_ = patch.Apply(entity, testSize{})
			}
		},
		func() { NewPatch2[testSize, testSize](w) },
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
		spawn := NewSpawner2[testPosition, testVelocity](w)
		entity := spawn.Spawn(testPosition{}, testVelocity{})
		forward := NewPatch2[testPosition, testSize](w, Type[testVelocity]())
		backward := NewPatch2[testPosition, testVelocity](w, Type[testSize]())
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
			if err := Insert2(w, entity, testPosition{}, testVelocity{}); err != nil {
				panic(err)
			}
		}); allocations != 0 {
			t.Fatalf("Insert2 allocations=%v", allocations)
		}
	}
}
