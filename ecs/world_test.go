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
	Put(&aBundle, testPosition{X: 10, Y: 20})
	Put(&aBundle, testVelocity{X: 2, Y: 3})
	a := world.Spawn(aBundle)
	bBundle := NewBundle(1)
	Put(&bBundle, testPosition{X: -5, Y: 8})
	b := world.Spawn(bBundle)

	var moved []Entity
	for entity, access := range Query2[testPosition, testVelocity](world).All {
		pos := access.A.Mut()
		vel := access.B.Mut()

		pos.X += vel.X
		pos.Y += vel.Y
		moved = append(moved, entity)
	}

	if len(moved) != 1 || moved[0] != a {
		t.Fatalf("expected only entity %d to match query, got %v", a, moved)
	}
	pos, ok := Get[testPosition](world, a)
	if !ok || pos.X != 12 || pos.Y != 23 {
		t.Fatalf("unexpected updated position: %#v ok=%v", pos, ok)
	}
	if Has[testVelocity](world, b) {
		t.Fatalf("expected entity %d to have no velocity", b)
	}

	world.Despawn(a)
	if world.IsAlive(a) {
		t.Fatalf("expected entity %d to be despawned", a)
	}
	if _, ok := Get[testPosition](world, a); ok {
		t.Fatalf("expected despawned entity %d to have no position", a)
	}
}

func TestPreparedBundleCanBeReusedAndIsWorldBound(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(2)
	Put(&bundle, testVelocity{X: 3})
	Put(&bundle, testPosition{X: 7})
	prepared := world.PrepareBundle(bundle)
	first := world.SpawnPrepared(prepared)
	second := world.SpawnPrepared(prepared)
	if world.EntityCount() != 2 {
		t.Fatalf("prepared spawns changed entity count to %d", world.EntityCount())
	}
	for _, entity := range []Entity{first, second} {
		if !Has[testPosition](world, entity) || !Has[testVelocity](world, entity) {
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
		"resource change tick": func() { ResourceChangeTick[int](world) },
		"component revision":   func() { ComponentRevision[testPosition](world) },
		"message scratch":      func() { MessageScratch[testLocalTag, int](world) },
		"despawn":              func() { world.Despawn(0) },
		"is alive":             func() { world.IsAlive(0) },
		"entity count":         func() { world.EntityCount() },
		"entity slot":          func() { world.Slot(0) },
		"entity by slot":       func() { world.EntityBySlot(EntitySlot{}) },
		"insert":               func() { _ = Insert(world, Entity(0), testPosition{}) },
		"remove":               func() { Remove[testPosition](world, 0) },
		"has":                  func() { Has[testPosition](world, 0) },
		"has slot":             func() { HasSlot[testPosition](world, EntitySlot{}) },
		"get":                  func() { Get[testPosition](world, 0) },
		"get slot":             func() { GetSlot[testPosition](world, EntitySlot{}) },
		"get ref":              func() { GetRef[testPosition](world, 0) },
		"get ref slot":         func() { GetRefSlot[testPosition](world, EntitySlot{}) },
		"get mut":              func() { GetMut[testPosition](world, 0) },
		"get mut slot":         func() { GetMutSlot[testPosition](world, EntitySlot{}) },
		"query":                func() { Query[testPosition](world) },
		"commands":             func() { world.Commands() },
		"flush commands":       func() { _ = world.FlushCommands() },
		"insert resource":      func() { InsertResource(world, 1) },
		"get resource":         func() { GetResource[int](world) },
		"advance messages": func() {
			AdvanceMessages(world)
		},
		"frame scratch": func() { FrameScratch[testLocalTag, int](world) },
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
	InsertResource(world, "onix")
	if name, ok := GetResource[string](world); !ok || name != "onix" {
		t.Fatalf("unexpected resource: %q ok=%v", name, ok)
	}

	WriteMessage(world, 1)
	WriteMessage(world, 2)
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
	WriteMessage(world, 3)
	if got := first.ReadInto(world, buffer); len(got) != 1 || got[0] != 3 {
		t.Fatalf("unexpected ReadInto messages: %v", got)
	}

	AdvanceMessages(world)
	WriteMessage(world, 4)
	if got := first.Read(world); len(got) != 1 || got[0] != 4 {
		t.Fatalf("unexpected next-frame messages: %v", got)
	}
}

func TestMessageReaderAllStopsWithoutDiscardingUnreadMessages(t *testing.T) {
	world := NewWorld()
	WriteMessage(world, 1)
	WriteMessage(world, 2)
	WriteMessage(world, 3)

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
	WriteMessage(world, 1)
	WriteMessage(world, 2)
	AdvanceMessages(world)
	WriteMessage(world, 3)

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
	spawner := NewSpawner2[testPosition, testVelocity](world)
	entity := spawner.Spawn(testPosition{X: 4}, testVelocity{Y: 7})

	position, positionOK := Get[testPosition](world, entity)
	velocity, velocityOK := Get[testVelocity](world, entity)
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
	NewSpawner2[testPosition, testPosition](NewWorld())
}

func TestReadReferencesDoNotMarkComponentsChanged(t *testing.T) {
	world := NewWorld()
	spawner := NewSpawner1[testPosition](world)
	entity := spawner.Spawn(testPosition{X: 9})
	_, meta, _ := world.entityMeta(entity)
	column := componentColumn[testPosition](world.archetypes[meta.archetypeID], registerComponent[testPosition](world))
	before := column.changed[int(meta.row)]

	for _, access := range Query[testPosition](world).All {
		position := access.Ref()

		if position.X != 9 {
			t.Fatalf("unexpected read reference: %+v", position)
		}
	}
	if ok := GetRef[testPosition](world, entity); ok == nil {
		t.Fatal("expected entity read reference")
	}
	if after := column.changed[int(meta.row)]; after != before {
		t.Fatalf("read reference changed tick: before=%d after=%d", before, after)
	}
}

func TestEntityGenerationRejectsStaleEntity(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	Put(&bundle, testPosition{})
	stale := world.Spawn(bundle)
	if !world.Despawn(stale) {
		t.Fatal("expected despawn to succeed")
	}

	replacementBundle := NewBundle(1)
	Put(&replacementBundle, testPosition{X: 7})
	replacement := world.Spawn(replacementBundle)
	if stale == replacement {
		t.Fatal("expected reused entity index to have a new generation")
	}
	if world.IsAlive(stale) {
		t.Fatal("stale entity must remain invalid")
	}
	if position, ok := Get[testPosition](world, replacement); !ok || position.X != 7 {
		t.Fatalf("unexpected replacement component: %#v ok=%v", position, ok)
	}
}

func TestEntitySlotRejectsStaleAndForeignWorlds(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	Put(&bundle, testPosition{X: 4})
	entity := world.Spawn(bundle)
	slot := world.Slot(entity)

	if !slot.Valid() {
		t.Fatal("alive entity slot must be valid")
	}
	if (EntitySlot{}).Valid() {
		t.Fatal("zero entity slot must be invalid")
	}
	if got, ok := world.EntityBySlot(slot); !ok || got != entity {
		t.Fatalf("slot resolved to %d/%v, want %d/true", got, ok, entity)
	}
	if slot.Entity() != entity || !HasSlot[testPosition](world, slot) {
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
	if HasSlot[testPosition](foreign, slot) {
		t.Fatal("foreign world found a slot component")
	}
	if _, ok := GetSlot[testPosition](foreign, slot); ok {
		t.Fatal("foreign world read a slot component")
	}
	if ok := GetRefSlot[testPosition](foreign, slot); ok != nil {
		t.Fatal("foreign world read a slot component reference")
	}
	if ok := GetMutSlot[testPosition](foreign, slot); ok != nil {
		t.Fatal("foreign world read a mutable slot component reference")
	}

	if !world.Despawn(entity) {
		t.Fatal("expected entity despawn")
	}
	if got, ok := world.EntityBySlot(slot); ok || got != 0 {
		t.Fatalf("stale slot resolved after despawn to %d/%v", got, ok)
	}

	replacement := world.Spawn(bundle)
	if got, ok := world.EntityBySlot(slot); ok || got != 0 {
		t.Fatalf("stale slot resolved after index reuse to %d/%v (replacement %d)", got, ok, replacement)
	}
	if got, ok := world.EntityBySlot(world.Slot(replacement)); !ok || got != replacement {
		t.Fatalf("replacement slot resolved to %d/%v, want %d/true", got, ok, replacement)
	}
}

func TestEntitySlotLookupDoesNotAllocate(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	Put(&bundle, testPosition{})
	entity := world.Spawn(bundle)
	slot := world.Slot(entity)
	if allocations := testing.AllocsPerRun(100, func() {
		if resolved, ok := world.EntityBySlot(slot); !ok || resolved != entity {
			t.Fatalf("slot resolved to %d/%v, want %d/true", resolved, ok, entity)
		}
	}); allocations != 0 {
		t.Fatalf("slot lookup allocated %.0f objects, want 0", allocations)
	}
	if position, ok := GetSlot[testPosition](world, slot); !ok || position != (testPosition{}) {
		t.Fatalf("slot component read = %+v/%v", position, ok)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		position := GetMutSlot[testPosition](world, slot)
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
	Put(&bundle, testPosition{X: 1})
	entity := world.Spawn(bundle)

	if err := Insert(world, entity, testVelocity{X: 2}); err != nil {
		t.Fatalf("insert velocity: %v", err)
	}
	if !Has[testVelocity](world, entity) {
		t.Fatal("expected inserted velocity")
	}
	removed, ok := Remove[testPosition](world, entity)
	if !ok || removed.X != 1 {
		t.Fatalf("unexpected removed position: %#v ok=%v", removed, ok)
	}
	if Has[testPosition](world, entity) {
		t.Fatal("expected position to be removed")
	}
}

func TestStructuralMutationDuringQueryPanics(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	Put(&bundle, testPosition{})
	entity := world.Spawn(bundle)

	defer func() {
		if recover() == nil {
			t.Fatal("expected structural mutation during query to panic")
		}
	}()
	for _, access := range Query[testPosition](world).All {
		access.Mut()

		_ = Insert(world, entity, testVelocity{})
	}
}

func TestCommandsApplyAfterQuery(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	Put(&bundle, testPosition{})
	entity := world.Spawn(bundle)

	for _, access := range Query[testPosition](world).All {
		access.Mut()

		CommandInsert(world.Commands(), entity, testVelocity{X: 3})
	}
	if Has[testVelocity](world, entity) {
		t.Fatal("deferred component should not exist before flush")
	}
	if err := world.FlushCommands(); err != nil {
		t.Fatalf("flush commands: %v", err)
	}
	if velocity, ok := Get[testVelocity](world, entity); !ok || velocity.X != 3 {
		t.Fatalf("unexpected deferred component: %#v ok=%v", velocity, ok)
	}
}

func TestCommandsRemoveAndReservedQueueAvoidStableFrameAllocations(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	Put(&bundle, testPosition{})
	entity := world.Spawn(bundle)
	commands := world.Commands()
	commands.Reserve(1)

	CommandInsert(commands, entity, testVelocity{X: 3})
	if err := world.FlushCommands(); err != nil {
		t.Fatalf("flush insert command: %v", err)
	}
	if !Has[testVelocity](world, entity) {
		t.Fatal("insert command did not add velocity")
	}

	CommandRemove[testVelocity](commands, entity)
	if err := world.FlushCommands(); err != nil {
		t.Fatalf("flush remove command: %v", err)
	}
	if Has[testVelocity](world, entity) {
		t.Fatal("remove command did not remove velocity")
	}

	CommandInsert(commands, entity, testVelocity{X: 4})
	if err := world.FlushCommands(); err != nil {
		t.Fatalf("warm command flush: %v", err)
	}
	allocations := testing.AllocsPerRun(100, func() {
		CommandInsert(commands, entity, testVelocity{X: 5})
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
	Put(&firstBundle, testPosition{X: 1})
	Put(&firstBundle, testVelocity{})
	first := world.Spawn(firstBundle)
	secondBundle := NewBundle(1)
	Put(&secondBundle, testPosition{X: 2})
	world.Spawn(secondBundle)

	var matched []Entity
	for entity, access := range Query[testPosition](world, With[testVelocity]()).All {
		access.Mut()

		matched = append(matched, entity)
	}
	if len(matched) != 1 || matched[0] != first {
		t.Fatalf("unexpected With filter result: %v", matched)
	}

	matched = matched[:0]
	for entity, access := range Query[testPosition](world, Without[testVelocity]()).All {
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
	Put(&bundle, testPosition{})
	entity := world.Spawn(bundle)
	var runs []int
	schedule := NewSchedule("locals")
	if err := schedule.Add(SystemConfig{
		Name: "stateful",
		System: func(context *SystemContext) error {
			count := Local[testLocalTag, int](context)
			*count++
			runs = append(runs, *count)
			if *count == 1 {
				CommandInsert(context.Commands, entity, testVelocity{X: 4})
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
	if velocity, ok := Get[testVelocity](world, entity); !ok || velocity.X != 4 {
		t.Fatalf("unexpected scheduled command result: %#v ok=%v", velocity, ok)
	}
}

func TestSystemConfigRunReusesContextWithoutAllocating(t *testing.T) {
	world := NewWorld()
	runs := 0
	system := SystemConfig{
		Name: "direct_context",
		System: func(context *SystemContext) error {
			count := Local[testLocalTag, int](context)
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
	Put(&bundle, testPosition{X: 5})
	entity := CommandSpawn(world.Commands(), bundle)
	CommandInsert(world.Commands(), entity, testVelocity{X: 6})
	if world.IsAlive(entity) {
		t.Fatal("reserved entity must not be alive before flush")
	}
	if err := world.FlushCommands(); err != nil {
		t.Fatalf("flush commands: %v", err)
	}
	if !world.IsAlive(entity) {
		t.Fatal("spawned entity should be alive after flush")
	}
	if velocity, ok := Get[testVelocity](world, entity); !ok || velocity.X != 6 {
		t.Fatalf("unexpected component inserted after spawn: %#v ok=%v", velocity, ok)
	}
}

func TestPutEntityMaterializesBeforeArchetypeCommit(t *testing.T) {
	world := NewWorld()
	var nested Entity
	bundle := NewBundle(1)
	PutEntity(&bundle, func(Entity) testPosition {
		inner := NewBundle(1)
		Put(&inner, testPosition{X: 2})
		nested = world.Spawn(inner)
		return testPosition{X: 1}
	})
	outer := world.Spawn(bundle)

	outerPosition, outerOK := Get[testPosition](world, outer)
	nestedPosition, nestedOK := Get[testPosition](world, nested)
	if !outerOK || !nestedOK || outerPosition.X != 1 || nestedPosition.X != 2 {
		t.Fatalf("reentrant spawn mismatched rows: outer=%+v/%v nested=%+v/%v", outerPosition, outerOK, nestedPosition, nestedOK)
	}
}

func TestPutEntityPanicRollsBackReservation(t *testing.T) {
	world := NewWorld()
	bundle := NewBundle(1)
	PutEntity(&bundle, func(Entity) testPosition {
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
	for range Query[testPosition](world).All {
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
				Put(&bundle, testPosition{X: 1})
				reserved = CommandSpawn(context.Commands, bundle)
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
	PutEntity(&failed, func(Entity) testPosition {
		panic("build failed")
	})
	failedEntity := CommandSpawn(world.Commands(), failed)
	valid := NewBundle(1)
	Put(&valid, testPosition{X: 2})
	validEntity := CommandSpawn(world.Commands(), valid)

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
			*Local[testLocalTag, int](context)++
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
	InsertResource(world, &[]string{})
	runLog, _ := GetResource[*[]string](world)

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
	Put(&bundle, testPosition{X: 1})
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
			for range Query[testPosition](world, Added[testPosition](context)).All {
				snapshot.added++
			}
			for range Query[testPosition](world, Changed[testPosition](context)).All {
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
	position := GetMut[testPosition](world, entity)
	if position == nil {
		t.Fatal("expected mutable position")
	}
	position.X = 2
	if err := schedule.Run(world); err != nil {
		t.Fatalf("mutated run: %v", err)
	}
	if err := Insert(world, entity, testPosition{X: 3}); err != nil {
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
	Put(&bundle, testPosition{})
	world.Spawn(bundle)

	var changed []int
	schedule := NewSchedule("mutable_query")
	if err := schedule.Add(SystemConfig{
		Name: "observe",
		System: func(context *SystemContext) error {
			count := 0
			for range Query[testPosition](world, Changed[testPosition](context)).All {
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

	for _, access := range Query[testPosition](world).All {
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
	spawner := NewSpawner2[testPosition, testVelocity](world)
	entity := spawner.Spawn(testPosition{X: 1}, testVelocity{X: 2})
	_, meta, _ := world.entityMeta(entity)
	archetype := world.archetypes[meta.archetypeID]
	positionID := world.componentTypes[reflect.TypeFor[testPosition]()]
	velocityID := world.componentTypes[reflect.TypeFor[testVelocity]()]
	positionColumn := componentColumn[testPosition](archetype, positionID)
	velocityColumn := componentColumn[testVelocity](archetype, velocityID)
	positionTick, velocityTick := positionColumn.changed[int(meta.row)], velocityColumn.changed[int(meta.row)]

	for _, access := range Query2[testPosition, testVelocity](world).All {
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
	Put(&bundle, testPosition{})
	Put(&bundle, testVelocity{})
	Put(&bundle, testAccessC{})
	Put(&bundle, testAccessD{})
	Put(&bundle, testAccessE{})
	Put(&bundle, testAccessF{})
	Put(&bundle, testAccessG{})
	world.Spawn(bundle)
	matched := 0
	for _, access := range Query7[testPosition, testVelocity, testAccessC, testAccessD, testAccessE, testAccessF, testAccessG](world).All {
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
	Put(&bundle, testPosition{})
	entity := world.Spawn(bundle)

	var snapshots [][2]int
	schedule := NewSchedule("archetype_ticks")
	if err := schedule.Add(SystemConfig{
		Name: "observe",
		System: func(context *SystemContext) error {
			var added, changed int
			for range Query[testPosition](world, Added[testPosition](context)).All {
				added++
			}
			for range Query[testPosition](world, Changed[testPosition](context)).All {
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
	if err := Insert(world, entity, testVelocity{}); err != nil {
		t.Fatalf("insert velocity: %v", err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("insert move run: %v", err)
	}
	if _, ok := Remove[testVelocity](world, entity); !ok {
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
	spawner := NewSpawner1[testPosition](world)
	first := spawner.Spawn(testPosition{})
	second := spawner.Spawn(testPosition{})
	_, firstMeta, _ := world.entityMeta(first)
	positionArchetype := world.archetypes[firstMeta.archetypeID]

	if err := Insert(world, first, testVelocity{}); err != nil {
		t.Fatal(err)
	}
	velocityID := world.componentTypes[reflect.TypeFor[testVelocity]()]
	velocityArchetypeID, ok := positionArchetype.addEdges[velocityID]
	if !ok {
		t.Fatal("add transition was not cached")
	}
	if err := Insert(world, second, testVelocity{}); err != nil {
		t.Fatal(err)
	}
	_, secondMeta, _ := world.entityMeta(second)
	if secondMeta.archetypeID != velocityArchetypeID {
		t.Fatalf("cached add transition selected archetype %d, want %d", secondMeta.archetypeID, velocityArchetypeID)
	}
	if _, ok := Remove[testVelocity](world, first); !ok {
		t.Fatal("remove velocity")
	}
	velocityArchetype := world.archetypes[velocityArchetypeID]
	if velocityArchetype.removeEdges[velocityID] != positionArchetype.id {
		t.Fatal("reverse remove transition was not cached")
	}
}

func TestResourceChangeDetection(t *testing.T) {
	world := NewWorld()
	InsertResource(world, 1)

	var snapshots [][2]bool
	schedule := NewSchedule("resource_changes")
	if err := schedule.Add(SystemConfig{
		Name: "observe",
		System: func(context *SystemContext) error {
			snapshots = append(snapshots, [2]bool{
				ResourceAdded[int](context),
				ResourceChanged[int](context),
			})
			return nil
		},
	}); err != nil {
		t.Fatalf("add observer: %v", err)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("initial run: %v", err)
	}
	if value, ok := GetResource[int](world); !ok || value != 1 {
		t.Fatalf("unexpected resource: %d ok=%v", value, ok)
	}
	if err := schedule.Run(world); err != nil {
		t.Fatalf("read-only run: %v", err)
	}
	InsertResource(world, 2)
	if err := schedule.Run(world); err != nil {
		t.Fatalf("replacement run: %v", err)
	}
	value, ok := GetResourceMut[int](world)
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
	Put(&bundle, testPosition{})
	world.Spawn(bundle)

	enabled := false
	matched := 0
	schedule := NewSchedule("skipped_changes")
	if err := schedule.Add(SystemConfig{
		Name:  "observe",
		RunIf: []Condition{func(*SystemContext) bool { return enabled }},
		System: func(context *SystemContext) error {
			for range Query[testPosition](world, Added[testPosition](context)).All {
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
