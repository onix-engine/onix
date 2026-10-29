package ecs

import (
	"errors"
	"reflect"
	"testing"
)

type testPosition struct {
	X float32
	Y float32
}

type testVelocity struct {
	X float32
	Y float32
}

type testLocalTag struct{}

type testAccessC struct{ Value int }
type testAccessD struct{ Value int }
type testAccessE struct{ Value int }
type testAccessF struct{ Value int }
type testAccessG struct{ Value int }

func TestWorldSpawnQueryAndDespawn(t *testing.T) {
	world := NewWorld()
	aBundle := NewBundle(2)
	aBundle.Put(testPosition{X: 10, Y: 20})
	aBundle.Put(testVelocity{X: 2, Y: 3})
	a := world.Spawn(aBundle)
	bBundle := NewBundle(1)
	bBundle.Put(testPosition{X: -5, Y: 8})
	b := world.Spawn(bBundle)

	var moved []Entity
	for entity, access := range world.Query2[testPosition, testVelocity]().All {
		pos := access.A.Mut()
		vel := access.B.Mut()

		pos.X += vel.X
		pos.Y += vel.Y
		moved = append(moved, entity)
	}

	if len(moved) != 1 || moved[0] != a {
		t.Fatalf("expected only entity %d to match query, got %v", a, moved)
	}
	pos, ok := world.Get[testPosition](a)
	if !ok || pos.X != 12 || pos.Y != 23 {
		t.Fatalf("unexpected updated position: %#v ok=%v", pos, ok)
	}
	if world.Has[testVelocity](b) {
		t.Fatalf("expected entity %d to have no velocity", b)
	}

	world.Despawn(a)
	if world.IsAlive(a) {
		t.Fatalf("expected entity %d to be despawned", a)
	}
	if _, ok := world.Get[testPosition](a); ok {
		t.Fatalf("expected despawned entity %d to have no position", a)
	}
}

func TestPreparedBundleCanBeReusedAndIsWorldBound(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(2)
	bundle.Put(testVelocity{X: 3})
	bundle.Put(testPosition{X: 7})
	prepared := world.PrepareBundle(bundle)
	first := world.SpawnPrepared(prepared)
	second := world.SpawnPrepared(prepared)
	if world.EntityCount() != 2 {
		t.Fatalf("prepared spawns changed entity count to %d", world.EntityCount())
	}
	for _, entity := range []Entity{first, second} {
		if !world.Has[testPosition](entity) || !world.Has[testVelocity](entity) {
			t.Fatalf("prepared spawn %d is missing components", entity)
		}
	}
	other := NewWorld()
	defer func() {
		if recover() == nil {
			t.Fatal("expected a prepared bundle from another world to panic")
		}
	}()
	other.SpawnPrepared(prepared)
}

func TestWorldStateAPIsPanicForNilWorld(t *testing.T) {
	var world *World
	checks := map[string]func(){
		"structure version":    func() { world.StructureVersion() },
		"change tick":          func() { world.ChangeTick() },
		"entity slots":         func() { world.EntitySlots() },
		"changes since":        func() { world.ChangesSince(0) },
		"resource change tick": func() { world.ResourceChangeTick[int]() },
		"component revision":   func() { world.ComponentRevision[testPosition]() },
		"message scratch":      func() { world.MessageScratch[testLocalTag, int]() },
		"despawn":              func() { world.Despawn(0) },
		"is alive":             func() { world.IsAlive(0) },
		"entity count":         func() { world.EntityCount() },
		"entity slot":          func() { world.Slot(0) },
		"entity by slot":       func() { world.EntityBySlot(EntitySlot{}) },
		"insert":               func() { _ = world.Insert(Entity(0), testPosition{}) },
		"remove":               func() { world.Remove[testPosition](0) },
		"has":                  func() { world.Has[testPosition](0) },
		"get":                  func() { world.Get[testPosition](0) },
		"get ref":              func() { world.GetRef[testPosition](0) },
		"get mut":              func() { world.GetMut[testPosition](0) },
		"query":                func() { world.Query[testPosition]() },
		"commands":             func() { world.Commands() },
		"flush commands":       func() { _ = world.FlushCommands() },
		"insert resource":      func() { world.InsertResource(1) },
		"get resource":         func() { world.GetResource[int]() },
		"advance messages": func() {
			world.AdvanceMessages()
		},
		"frame scratch": func() { world.FrameScratch[testLocalTag, int]() },
	}
	for name, check := range checks {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected panic")
				}
			}()
			check()
		})
	}
}

func TestResourcesAndMessages(t *testing.T) {
	world := NewWorld()
	world.InsertResource("onix")
	if name, ok := world.GetResource[string](); !ok || name != "onix" {
		t.Fatalf("unexpected resource: %q ok=%v", name, ok)
	}

	world.WriteMessage(1)
	world.WriteMessage(2)
	var first MessageReader[int]
	var second MessageReader[int]
	if got := first.Read(world); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("unexpected first reader messages: %v", got)
	}
	if got := second.Read(world); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("unexpected second reader messages: %v", got)
	}
	if got := first.Read(world); len(got) != 0 {
		t.Fatalf("expected first reader to have no unread messages: %v", got)
	}
	buffer := make([]int, 0, 2)
	world.WriteMessage(3)
	if got := first.ReadInto(world, buffer); len(got) != 1 || got[0] != 3 {
		t.Fatalf("unexpected ReadInto messages: %v", got)
	}

	world.AdvanceMessages()
	world.WriteMessage(4)
	if got := first.Read(world); len(got) != 1 || got[0] != 4 {
		t.Fatalf("unexpected next-frame messages: %v", got)
	}
}

func TestMessageReaderAllStopsWithoutDiscardingUnreadMessages(t *testing.T) {
	world := NewWorld()
	world.WriteMessage(1)
	world.WriteMessage(2)
	world.WriteMessage(3)

	var reader MessageReader[int]
	var first []int
	for message := range reader.All(world) {
		first = append(first, *message)
		if *message == 2 {
			break
		}
	}
	if len(first) != 2 || first[0] != 1 || first[1] != 2 {
		t.Fatalf("unexpected first message batch: %v", first)
	}

	var remaining []int
	for message := range reader.All(world) {
		remaining = append(remaining, *message)
	}
	if len(remaining) != 1 || remaining[0] != 3 {
		t.Fatalf("unexpected remaining messages: %v", remaining)
	}
}

func TestMessageReaderAllTraversesBothFrameBuffers(t *testing.T) {
	world := NewWorld()
	world.WriteMessage(1)
	world.WriteMessage(2)
	world.AdvanceMessages()
	world.WriteMessage(3)

	var reader MessageReader[int]
	var messages []int
	for message := range reader.All(world) {
		messages = append(messages, *message)
	}
	if len(messages) != 3 || messages[0] != 1 || messages[1] != 2 || messages[2] != 3 {
		t.Fatalf("unexpected messages across frame buffers: %v", messages)
	}
}

func TestSpawner2CreatesTypedComponents(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner2[testPosition, testVelocity]()
	entity := spawner.Spawn(testPosition{X: 4}, testVelocity{Y: 7})

	position, positionOK := world.Get[testPosition](entity)
	velocity, velocityOK := world.Get[testVelocity](entity)
	if !positionOK || !velocityOK || position.X != 4 || velocity.Y != 7 {
		t.Fatalf("unexpected spawned components: position=%+v velocity=%+v", position, velocity)
	}
}

func TestSpawnerRejectsDuplicateComponentTypes(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected duplicate spawner components to panic")
		}
	}()
	NewWorld().NewSpawner2[testPosition, testPosition]()
}

func TestReadReferencesDoNotMarkComponentsChanged(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner1[testPosition]()
	entity := spawner.Spawn(testPosition{X: 9})
	_, meta, _ := world.entityMeta(entity)
	column := world.archetypes[meta.archetypeID].componentColumn[testPosition](world.registerComponent[testPosition]())
	before := column.changed[int(meta.row)]

	for _, access := range world.Query[testPosition]().All {
		position := access.Ref()

		if position.X != 9 {
			t.Fatalf("unexpected read reference: %+v", position)
		}
	}
	if ok := world.GetRef[testPosition](entity); ok == nil {
		t.Fatal("expected entity read reference")
	}
	if after := column.changed[int(meta.row)]; after != before {
		t.Fatalf("read reference changed tick: before=%d after=%d", before, after)
	}
}

func TestEntityGenerationRejectsStaleEntity(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	bundle.Put(testPosition{})
	stale := world.Spawn(bundle)
	if !world.Despawn(stale) {
		t.Fatal("expected despawn to succeed")
	}

	replacementBundle := NewBundle(1)
	replacementBundle.Put(testPosition{X: 7})
	replacement := world.Spawn(replacementBundle)
	if stale == replacement {
		t.Fatal("expected reused entity index to have a new generation")
	}
	if world.IsAlive(stale) {
		t.Fatal("stale entity must remain invalid")
	}
	if position, ok := world.Get[testPosition](replacement); !ok || position.X != 7 {
		t.Fatalf("unexpected replacement component: %#v ok=%v", position, ok)
	}
}

func TestEntitySlotBindsWorldAndRejectsEmptyAndStaleEntities(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	bundle.Put(testPosition{X: 4})
	entity := world.Spawn(bundle)
	slot := world.Slot(entity)
	assertMissing := func(slot EntitySlot) {
		t.Helper()
		if slot.Has[testPosition]() {
			t.Fatal("empty or stale slot has a component")
		}
		if value, ok := slot.Get[testPosition](); ok || value != (testPosition{}) {
			t.Fatalf("empty or stale slot read = %+v/%v", value, ok)
		}
		if slot.GetRef[testPosition]() != nil || slot.GetMut[testPosition]() != nil {
			t.Fatal("empty or stale slot returned a component pointer")
		}
	}
	assertMissing(EntitySlot{})

	if !slot.Valid() {
		t.Fatal("alive entity slot must be valid")
	}
	if (EntitySlot{}).Valid() {
		t.Fatal("zero entity slot must be invalid")
	}
	if got, ok := world.EntityBySlot(slot); !ok || got != entity {
		t.Fatalf("slot resolved to %d/%v, want %d/true", got, ok, entity)
	}
	if slot.Entity() != entity || !slot.Has[testPosition]() {
		t.Fatalf("slot does not expose entity %d with its component", entity)
	}
	if got, ok := NewWorld().EntityBySlot(slot); ok || got != 0 {
		t.Fatalf("foreign world resolved slot to %d/%v", got, ok)
	}
	foreign := NewWorld()
	foreignEntity := foreign.Spawn(bundle)
	foreignSlot := foreign.Slot(foreignEntity)
	if got, ok := world.EntityBySlot(foreignSlot); ok || got != 0 {
		t.Fatalf("world accepted a foreign slot as %d/%v", got, ok)
	}
	if foreignEntity != entity {
		t.Fatal("test requires identical handles in different worlds")
	}
	foreignSlot.MustMut[testPosition]().X = 9
	tick, version := world.ChangeTick(), world.ComponentVersion[testPosition]()
	if value, ok := slot.Get[testPosition](); !ok || value.X != 4 {
		t.Fatalf("slot read from wrong world: %+v/%v", value, ok)
	}
	if value := slot.GetRef[testPosition](); value == nil || value.X != 4 {
		t.Fatalf("slot reference came from wrong world: %+v", value)
	}
	if world.ChangeTick() != tick || world.ComponentVersion[testPosition]() != version {
		t.Fatal("slot reads marked the component changed")
	}
	slot.GetMut[testPosition]().X = 5
	if value := foreignSlot.GetRef[testPosition](); value.X != 9 {
		t.Fatal("slot mutation affected a different world")
	}
	if world.ChangeTick() <= tick || world.ComponentVersion[testPosition]() <= version {
		t.Fatal("slot mutation did not mark the component changed")
	}

	if !world.Despawn(entity) {
		t.Fatal("expected entity despawn")
	}
	if got, ok := world.EntityBySlot(slot); ok || got != 0 {
		t.Fatalf("stale slot resolved after despawn to %d/%v", got, ok)
	}
	assertMissing(slot)

	replacement := world.Spawn(bundle)
	if got, ok := world.EntityBySlot(slot); ok || got != 0 {
		t.Fatalf("stale slot resolved after index reuse to %d/%v (replacement %d)", got, ok, replacement)
	}
	assertMissing(slot)
	if got, ok := world.EntityBySlot(world.Slot(replacement)); !ok || got != replacement {
		t.Fatalf("replacement slot resolved to %d/%v, want %d/true", got, ok, replacement)
	}
}

func TestEntitySlotLookupDoesNotAllocate(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	bundle.Put(testPosition{})
	entity := world.Spawn(bundle)
	slot := world.Slot(entity)
	if allocations := testing.AllocsPerRun(100, func() {
		if resolved, ok := world.EntityBySlot(slot); !ok || resolved != entity {
			t.Fatalf("slot resolved to %d/%v, want %d/true", resolved, ok, entity)
		}
	}); allocations != 0 {
		t.Fatalf("slot lookup allocated %.0f objects, want 0", allocations)
	}
	if position, ok := slot.Get[testPosition](); !ok || position != (testPosition{}) {
		t.Fatalf("slot component read = %+v/%v", position, ok)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		position := slot.GetMut[testPosition]()
		if position == nil {
			t.Fatal("slot mutable component read failed")
		}
	}); allocations != 0 {
		t.Fatalf("slot component lookup allocated %.0f objects, want 0", allocations)
	}
}

func TestInsertAndRemoveMoveEntityBetweenArchetypes(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	bundle.Put(testPosition{X: 1})
	entity := world.Spawn(bundle)

	if err := world.Insert(entity, testVelocity{X: 2}); err != nil {
		t.Fatalf("insert velocity: %v", err)
	}
	if !world.Has[testVelocity](entity) {
		t.Fatal("expected inserted velocity")
	}
	removed, ok := world.Remove[testPosition](entity)
	if !ok || removed.X != 1 {
		t.Fatalf("unexpected removed position: %#v ok=%v", removed, ok)
	}
	if world.Has[testPosition](entity) {
		t.Fatal("expected position to be removed")
	}
}

func TestStructuralMutationDuringQueryPanics(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	bundle.Put(testPosition{})
	entity := world.Spawn(bundle)

	defer func() {
		if recover() == nil {
			t.Fatal("expected structural mutation during query to panic")
		}
	}()
	for _, access := range world.Query[testPosition]().All {
		access.Mut()

		_ = world.Insert(entity, testVelocity{})
	}
}

func TestCommandsApplyAfterQuery(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	bundle.Put(testPosition{})
	entity := world.Spawn(bundle)

	for _, access := range world.Query[testPosition]().All {
		access.Mut()

		world.Commands().Insert(entity, testVelocity{X: 3})
	}
	if world.Has[testVelocity](entity) {
		t.Fatal("deferred component should not exist before flush")
	}
	if err := world.FlushCommands(); err != nil {
		t.Fatalf("flush commands: %v", err)
	}
	if velocity, ok := world.Get[testVelocity](entity); !ok || velocity.X != 3 {
		t.Fatalf("unexpected deferred component: %#v ok=%v", velocity, ok)
	}
}

func TestCommandsRemoveAndReservedQueueAvoidStableFrameAllocations(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	bundle.Put(testPosition{})
	entity := world.Spawn(bundle)
	commands := world.Commands()
	commands.Reserve(1)

	commands.Insert(entity, testVelocity{X: 3})
	if err := world.FlushCommands(); err != nil {
		t.Fatalf("flush insert command: %v", err)
	}
	if !world.Has[testVelocity](entity) {
		t.Fatal("insert command did not add velocity")
	}

	commands.Remove[testVelocity](entity)
	if err := world.FlushCommands(); err != nil {
		t.Fatalf("flush remove command: %v", err)
	}
	if world.Has[testVelocity](entity) {
		t.Fatal("remove command did not remove velocity")
	}

	commands.Insert(entity, testVelocity{X: 4})
	if err := world.FlushCommands(); err != nil {
		t.Fatalf("warm command flush: %v", err)
	}
	allocations := testing.AllocsPerRun(100, func() {
		commands.Insert(entity, testVelocity{X: 5})
		if err := world.FlushCommands(); err != nil {
			t.Fatalf("stable command flush: %v", err)
		}
	})
	if allocations != 0 {
		t.Fatalf("reserved command queue allocated %.0f objects, want 0", allocations)
	}
}

func TestQueryWithAndWithoutFilters(t *testing.T) {
	world := NewWorld()
	firstBundle := NewBundle(2)
	firstBundle.Put(testPosition{X: 1})
	firstBundle.Put(testVelocity{})
	first := world.Spawn(firstBundle)
	secondBundle := NewBundle(1)
	secondBundle.Put(testPosition{X: 2})
	world.Spawn(secondBundle)

	var matched []Entity
	for entity, access := range world.Query[testPosition](With[testVelocity]()).All {
		access.Mut()

		matched = append(matched, entity)
	}
	if len(matched) != 1 || matched[0] != first {
		t.Fatalf("unexpected With filter result: %v", matched)
	}

	matched = matched[:0]
	for entity, access := range world.Query[testPosition](Without[testVelocity]()).All {
		access.Mut()

		matched = append(matched, entity)
	}
	if len(matched) != 1 || matched[0] == first {
		t.Fatalf("unexpected Without filter result: %v", matched)
	}
}

func TestSchedulePersistsSystemLocalsAndFlushesCommands(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	bundle.Put(testPosition{})
	entity := world.Spawn(bundle)
	var runs []int
	schedule := NewSchedule("locals")
	if err := schedule.Add(SystemConfig{
		Name: "stateful",
		System: func(context *SystemContext) error {
			count := context.Local[testLocalTag, int]()
			*count++
			runs = append(runs, *count)
			if *count == 1 {
				context.Commands.Insert(entity, testVelocity{X: 4})
			}
			return nil
		},
	}); err != nil {
		t.Fatalf("add system: %v", err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("run 1: %v", err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("run 2: %v", err)
	}
	if len(runs) != 2 || runs[0] != 1 || runs[1] != 2 {
		t.Fatalf("unexpected local state: %v", runs)
	}
	if velocity, ok := world.Get[testVelocity](entity); !ok || velocity.X != 4 {
		t.Fatalf("unexpected scheduled command result: %#v ok=%v", velocity, ok)
	}
}

func TestSystemConfigRunReusesContextWithoutAllocating(t *testing.T) {
	world := NewWorld()
	runs := 0
	system := SystemConfig{
		Name: "direct_context",
		System: func(context *SystemContext) error {
			count := context.Local[testLocalTag, int]()
			*count++
			runs = *count
			return nil
		},
	}
	if err := system.Run(world); err != nil {
		t.Fatal(err)
	}

	initialRuns := runs
	measuredRuns := 0
	if allocations := testing.AllocsPerRun(100, func() {
		measuredRuns++
		if err := system.Run(world); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("direct system run allocated %.0f objects, want 0", allocations)
	}
	if runs != initialRuns+measuredRuns {
		t.Fatalf("direct system local count = %d, want %d", runs, initialRuns+measuredRuns)
	}
}

func TestCommandSpawnReservesEntityForFollowingCommands(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	bundle.Put(testPosition{X: 5})
	entity := world.Commands().Spawn(bundle)
	world.Commands().Insert(entity, testVelocity{X: 6})
	if world.IsAlive(entity) {
		t.Fatal("reserved entity must not be alive before flush")
	}
	if err := world.FlushCommands(); err != nil {
		t.Fatalf("flush commands: %v", err)
	}
	if !world.IsAlive(entity) {
		t.Fatal("spawned entity should be alive after flush")
	}
	if velocity, ok := world.Get[testVelocity](entity); !ok || velocity.X != 6 {
		t.Fatalf("unexpected component inserted after spawn: %#v ok=%v", velocity, ok)
	}
}

func TestPutEntityMaterializesBeforeArchetypeCommit(t *testing.T) {
	world := NewWorld()
	var nested Entity
	bundle := NewBundle(1)
	bundle.PutEntity(func(Entity) testPosition {
		inner := NewBundle(1)
		inner.Put(testPosition{X: 2})
		nested = world.Spawn(inner)
		return testPosition{X: 1}
	})
	outer := world.Spawn(bundle)

	outerPosition, outerOK := world.Get[testPosition](outer)
	nestedPosition, nestedOK := world.Get[testPosition](nested)
	if !outerOK || !nestedOK || outerPosition.X != 1 || nestedPosition.X != 2 {
		t.Fatalf("reentrant spawn mismatched rows: outer=%+v/%v nested=%+v/%v", outerPosition, outerOK, nestedPosition, nestedOK)
	}
}

func TestPutEntityPanicRollsBackReservation(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	bundle.PutEntity(func(Entity) testPosition {
		panic("build failed")
	})

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected entity component builder panic")
			}
		}()
		world.Spawn(bundle)
	}()

	if world.EntityCount() != 0 || len(world.entities) != 1 || world.entities[0].state != entityFree || len(world.free) != 1 {
		t.Fatalf("failed spawn leaked reservation: count=%d entities=%+v free=%v", world.EntityCount(), world.entities, world.free)
	}
	matched := 0
	for range world.Query[testPosition]().All {
		matched++
	}
	if matched != 0 {
		t.Fatalf("failed spawn left %d query rows", matched)
	}
}

func TestSystemErrorDiscardsCommandsAndReservation(t *testing.T) {
	world := NewWorld()
	schedule := NewSchedule("rollback")
	var reserved Entity
	shouldFail := true
	if err := schedule.Add(SystemConfig{
		Name: "spawn_then_fail",
		System: func(context *SystemContext) error {
			if shouldFail {
				bundle := NewBundle(1)
				bundle.Put(testPosition{X: 1})
				reserved = context.Commands.Spawn(bundle)
				return errors.New("failed")
			}
			return nil
		},
	}); err != nil {
		t.Fatalf("add system: %v", err)
	}

	if err := schedule.Run(world); err == nil {
		t.Fatal("expected system failure")
	}
	context := schedule.systems["spawn_then_fail"].context
	if len(world.commands.queue) != 0 || world.EntityCount() != 0 || context.lastRun != 0 {
		t.Fatalf("failed system leaked state: commands=%d entities=%d lastRun=%d", len(world.commands.queue), world.EntityCount(), context.lastRun)
	}
	if world.IsAlive(reserved) {
		t.Fatal("failed system left reserved entity alive")
	}

	shouldFail = false
	if err := schedule.Run(world); err != nil {
		t.Fatalf("retry system: %v", err)
	}
	if world.EntityCount() != 0 || world.IsAlive(reserved) {
		t.Fatal("retry applied commands from failed system")
	}
}

func TestCommandPanicDiscardsRemainingReservations(t *testing.T) {
	world := NewWorld()
	failed := NewBundle(1)
	failed.PutEntity(func(Entity) testPosition {
		panic("build failed")
	})
	failedEntity := world.Commands().Spawn(failed)
	valid := NewBundle(1)
	valid.Put(testPosition{X: 2})
	validEntity := world.Commands().Spawn(valid)

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected flush panic")
			}
		}()
		_ = world.FlushCommands()
	}()
	if len(world.commands.queue) != 0 || world.EntityCount() != 0 || world.IsAlive(failedEntity) || world.IsAlive(validEntity) {
		t.Fatalf("panic leaked command state: queue=%d count=%d failed=%v valid=%v", len(world.commands.queue), world.EntityCount(), world.IsAlive(failedEntity), world.IsAlive(validEntity))
	}
}

func TestScheduleRejectsDifferentWorld(t *testing.T) {
	schedule := NewSchedule("bound_world")
	if err := schedule.Add(SystemConfig{
		Name: "local",
		System: func(context *SystemContext) error {
			*context.Local[testLocalTag, int]()++
			return nil
		},
	}); err != nil {
		t.Fatalf("add system: %v", err)
	}
	if err := schedule.Run(NewWorld()); err != nil {
		t.Fatalf("run first world: %v", err)
	}
	if err := schedule.Run(NewWorld()); err == nil {
		t.Fatal("expected schedule to reject a different world")
	}
}

func TestScheduleOrderReturnsCompileError(t *testing.T) {
	schedule := NewSchedule("invalid")
	if err := schedule.Add(SystemConfig{Name: "dependent", After: []string{"missing"}, System: func(*SystemContext) error { return nil }}); err != nil {
		t.Fatalf("add system: %v", err)
	}
	if _, err := schedule.Order(); err == nil {
		t.Fatal("expected missing dependency error")
	}
}

func TestScheduleOrderingAndConditions(t *testing.T) {
	world := NewWorld()
	world.InsertResource(&[]string{})
	runLog, _ := world.GetResource[*[]string]()

	schedule := NewSchedule("update")
	err := schedule.Add(SystemConfig{
		Name: "physics",
		System: func(context *SystemContext) error {
			*runLog = append(*runLog, "physics")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("add physics: %v", err)
	}
	err = schedule.Add(SystemConfig{
		Name:  "movement",
		After: []string{"physics"},
		System: func(context *SystemContext) error {
			*runLog = append(*runLog, "movement")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("add movement: %v", err)
	}
	err = schedule.Add(SystemConfig{
		Name:  "render",
		After: []string{"movement"},
		RunIf: []Condition{
			func(context *SystemContext) bool {
				return context.World.EntityCount() == 0
			},
		},
		System: func(context *SystemContext) error {
			*runLog = append(*runLog, "render")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("add render: %v", err)
	}

	if err := schedule.Run(world); err != nil {
		t.Fatalf("run schedule: %v", err)
	}
	got := *runLog
	if len(got) != 3 || got[0] != "physics" || got[1] != "movement" || got[2] != "render" {
		t.Fatalf("unexpected schedule order: %v", got)
	}
}

func TestAddedAndChangedFiltersTrackSystemRuns(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	bundle.Put(testPosition{X: 1})
	entity := world.Spawn(bundle)

	type changeSnapshot struct {
		added   int
		changed int
	}
	var snapshots []changeSnapshot
	schedule := NewSchedule("change_filters")
	if err := schedule.Add(SystemConfig{
		Name: "observe",
		System: func(context *SystemContext) error {
			var snapshot changeSnapshot
			for range world.Query[testPosition](context.Added[testPosition]()).All {
				snapshot.added++
			}
			for range world.Query[testPosition](context.Changed[testPosition]()).All {
				snapshot.changed++
			}
			snapshots = append(snapshots, snapshot)
			return nil
		},
	}); err != nil {
		t.Fatalf("add observer: %v", err)
	}

	if err := schedule.Run(world); err != nil {
		t.Fatalf("initial run: %v", err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("unchanged run: %v", err)
	}
	position := world.GetMut[testPosition](entity)
	if position == nil {
		t.Fatal("expected mutable position")
	}
	position.X = 2
	if err := schedule.Run(world); err != nil {
		t.Fatalf("mutated run: %v", err)
	}
	if err := world.Insert(entity, testPosition{X: 3}); err != nil {
		t.Fatalf("replace position: %v", err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("replaced run: %v", err)
	}

	want := []changeSnapshot{{added: 1, changed: 1}, {}, {changed: 1}, {changed: 1}}
	if len(snapshots) != len(want) {
		t.Fatalf("unexpected snapshots: %v", snapshots)
	}
	for i := range want {
		if snapshots[i] != want[i] {
			t.Fatalf("snapshot %d: got %+v want %+v", i, snapshots[i], want[i])
		}
	}
}

func TestMutableQueryMarksChanged(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	bundle.Put(testPosition{})
	world.Spawn(bundle)

	var changed []int
	schedule := NewSchedule("mutable_query")
	if err := schedule.Add(SystemConfig{
		Name: "observe",
		System: func(context *SystemContext) error {
			count := 0
			for range world.Query[testPosition](context.Changed[testPosition]()).All {
				count++
			}
			changed = append(changed, count)
			return nil
		},
	}); err != nil {
		t.Fatalf("add observer: %v", err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("initial run: %v", err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("unchanged run: %v", err)
	}

	for _, access := range world.Query[testPosition]().All {
		access.Mut()
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("mutable query run: %v", err)
	}
	if len(changed) != 3 || changed[0] != 1 || changed[1] != 0 || changed[2] != 1 {
		t.Fatalf("unexpected changed counts: %v", changed)
	}
}

func TestQueryAccessMarksOnlyRequestedMutableComponents(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner2[testPosition, testVelocity]()
	entity := spawner.Spawn(testPosition{X: 1}, testVelocity{X: 2})
	_, meta, _ := world.entityMeta(entity)
	archetype := world.archetypes[meta.archetypeID]
	positionID := world.componentTypes[reflect.TypeFor[testPosition]()]
	velocityID := world.componentTypes[reflect.TypeFor[testVelocity]()]
	positionColumn := archetype.componentColumn[testPosition](positionID)
	velocityColumn := archetype.componentColumn[testVelocity](velocityID)
	positionTick, velocityTick := positionColumn.changed[int(meta.row)], velocityColumn.changed[int(meta.row)]

	for _, access := range world.Query2[testPosition, testVelocity]().All {
		position := access.A
		velocity := access.B

		position.Mut().X += velocity.Ref().X
	}
	if positionColumn.changed[int(meta.row)] == positionTick {
		t.Fatal("mutable access did not mark position changed")
	}
	if velocityColumn.changed[int(meta.row)] != velocityTick {
		t.Fatal("read access marked velocity changed")
	}
}

func TestQueryAccessSupportsSevenComponents(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(7)
	bundle.Put(testPosition{})
	bundle.Put(testVelocity{})
	bundle.Put(testAccessC{})
	bundle.Put(testAccessD{})
	bundle.Put(testAccessE{})
	bundle.Put(testAccessF{})
	bundle.Put(testAccessG{})
	world.Spawn(bundle)
	matched := 0
	for _, access := range world.Query7[testPosition, testVelocity, testAccessC, testAccessD, testAccessE, testAccessF, testAccessG]().All {
		a := access.A
		b := access.B
		c := access.C
		d := access.D
		e := access.E
		f := access.F
		g := access.G

		a.Mut().X = 1
		_ = b.Get()
		_ = c.Get()
		_ = d.Get()
		_ = e.Get()
		_ = f.Get()
		_ = g.Get()
		matched++
	}
	if matched != 1 {
		t.Fatalf("seven-component query matched %d entities", matched)
	}
}

func TestArchetypeMovesPreserveComponentTicks(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	bundle.Put(testPosition{})
	entity := world.Spawn(bundle)

	var snapshots [][2]int
	schedule := NewSchedule("archetype_ticks")
	if err := schedule.Add(SystemConfig{
		Name: "observe",
		System: func(context *SystemContext) error {
			var added, changed int
			for range world.Query[testPosition](context.Added[testPosition]()).All {
				added++
			}
			for range world.Query[testPosition](context.Changed[testPosition]()).All {
				changed++
			}
			snapshots = append(snapshots, [2]int{added, changed})
			return nil
		},
	}); err != nil {
		t.Fatalf("add observer: %v", err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("initial run: %v", err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("settle run: %v", err)
	}
	if err := world.Insert(entity, testVelocity{}); err != nil {
		t.Fatalf("insert velocity: %v", err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("insert move run: %v", err)
	}
	if _, ok := world.Remove[testVelocity](entity); !ok {
		t.Fatal("expected velocity removal")
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("remove move run: %v", err)
	}

	want := [][2]int{{1, 1}, {0, 0}, {0, 0}, {0, 0}}
	if len(snapshots) != len(want) {
		t.Fatalf("unexpected snapshots: %v", snapshots)
	}
	for i := range want {
		if snapshots[i] != want[i] {
			t.Fatalf("snapshot %d: got %v want %v", i, snapshots[i], want[i])
		}
	}
}

func TestArchetypeTransitionsAreCachedInBothDirections(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner1[testPosition]()
	first := spawner.Spawn(testPosition{})
	second := spawner.Spawn(testPosition{})
	_, firstMeta, _ := world.entityMeta(first)
	positionArchetype := world.archetypes[firstMeta.archetypeID]

	if err := world.Insert(first, testVelocity{}); err != nil {
		t.Fatal(err)
	}
	velocityID := world.componentTypes[reflect.TypeFor[testVelocity]()]
	velocityArchetypeID, ok := positionArchetype.addEdges[velocityID]
	if !ok {
		t.Fatal("add transition was not cached")
	}
	if err := world.Insert(second, testVelocity{}); err != nil {
		t.Fatal(err)
	}
	_, secondMeta, _ := world.entityMeta(second)
	if secondMeta.archetypeID != velocityArchetypeID {
		t.Fatalf("cached add transition selected archetype %d, want %d", secondMeta.archetypeID, velocityArchetypeID)
	}
	if _, ok := world.Remove[testVelocity](first); !ok {
		t.Fatal("remove velocity")
	}
	velocityArchetype := world.archetypes[velocityArchetypeID]
	if velocityArchetype.removeEdges[velocityID] != positionArchetype.id {
		t.Fatal("reverse remove transition was not cached")
	}
}

func TestResourceChangeDetection(t *testing.T) {
	world := NewWorld()
	world.InsertResource(1)

	var snapshots [][2]bool
	schedule := NewSchedule("resource_changes")
	if err := schedule.Add(SystemConfig{
		Name: "observe",
		System: func(context *SystemContext) error {
			snapshots = append(snapshots, [2]bool{
				context.ResourceAdded[int](),
				context.ResourceChanged[int](),
			})
			return nil
		},
	}); err != nil {
		t.Fatalf("add observer: %v", err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("initial run: %v", err)
	}
	if value, ok := world.GetResource[int](); !ok || value != 1 {
		t.Fatalf("unexpected resource: %d ok=%v", value, ok)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("read-only run: %v", err)
	}
	world.InsertResource(2)
	if err := schedule.Run(world); err != nil {
		t.Fatalf("replacement run: %v", err)
	}
	value, ok := world.GetResourceMut[int]()
	if !ok {
		t.Fatal("expected mutable resource")
	}
	*value = 3
	if err := schedule.Run(world); err != nil {
		t.Fatalf("mutable run: %v", err)
	}

	want := [][2]bool{{true, true}, {false, false}, {false, true}, {false, true}}
	if len(snapshots) != len(want) {
		t.Fatalf("unexpected snapshots: %v", snapshots)
	}
	for i := range want {
		if snapshots[i] != want[i] {
			t.Fatalf("snapshot %d: got %v want %v", i, snapshots[i], want[i])
		}
	}
}

func TestSkippedSystemRetainsLastRun(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	bundle.Put(testPosition{})
	world.Spawn(bundle)

	enabled := false
	matched := 0
	schedule := NewSchedule("skipped_changes")
	if err := schedule.Add(SystemConfig{
		Name:  "observe",
		RunIf: []Condition{func(*SystemContext) bool { return enabled }},
		System: func(context *SystemContext) error {
			for range world.Query[testPosition](context.Added[testPosition]()).All {
				matched++
			}
			return nil
		},
	}); err != nil {
		t.Fatalf("add observer: %v", err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("skipped run: %v", err)
	}
	enabled = true
	if err := schedule.Run(world); err != nil {
		t.Fatalf("enabled run: %v", err)
	}
	if matched != 1 {
		t.Fatalf("expected skipped system to retain added change, got %d", matched)
	}
}

func TestSystemErrorClearsActiveChangeTick(t *testing.T) {
	world := NewWorld()
	schedule := NewSchedule("error")
	if err := schedule.Add(SystemConfig{
		Name:   "fail",
		System: func(*SystemContext) error { return errors.New("failed") },
	}); err != nil {
		t.Fatalf("add failing system: %v", err)
	}
	if err := schedule.Run(world); err == nil {
		t.Fatal("expected system error")
	}
	if world.activeChangeTick != 0 {
		t.Fatalf("active change tick leaked after error: %d", world.activeChangeTick)
	}
}

func TestTickInRangeHandlesWraparound(t *testing.T) {
	lastRun := Tick(^uint32(0) - 1)
	thisRun := Tick(1)
	if tickInRange(lastRun, lastRun, thisRun) {
		t.Fatal("last run tick must not be reported as new")
	}
	if !tickInRange(Tick(^uint32(0)), lastRun, thisRun) {
		t.Fatal("pre-wrap change should be reported as new")
	}
	if !tickInRange(thisRun, lastRun, thisRun) {
		t.Fatal("post-wrap change should be reported as new")
	}
}
