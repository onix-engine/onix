package ecs_test

import (
	"slices"
	"testing"

	"github.com/diiyw/onix/ecs"
)

type boundaryPosition struct{ X int }
type borrowedMessage struct{ Values []*int }
type messageArenaA struct{}
type messageArenaB struct{}

func TestComponentRevisionSeparatesWritesFromStructuralChanges(t *testing.T) {
	world := ecs.NewWorld()
	tick := world.ChangeTick()
	if version, structure := world.ComponentRevision[boundaryPosition](); version != 0 || structure != 0 || world.ChangeTick() != tick {
		t.Fatal("reading an unregistered component revision mutated the world")
	}
	spawn := world.NewSpawner1[boundaryPosition]()
	entity := spawn.Spawn(boundaryPosition{})
	version, structure := world.ComponentRevision[boundaryPosition]()
	world.MustMut[boundaryPosition](entity).X = 1
	written, unchanged := world.ComponentRevision[boundaryPosition]()
	if written <= version || unchanged != structure {
		t.Fatal("component write must change its content version only")
	}
	if err := world.Insert(entity, 7); err != nil {
		t.Fatal(err)
	}
	moved, shifted := world.ComponentRevision[boundaryPosition]()
	if moved <= written || shifted <= structure {
		t.Fatal("row migration must invalidate content and structural caches")
	}
}

func TestEntityIndexAndStructureVersionSupportExternalCaches(t *testing.T) {
	world := ecs.NewWorld()
	spawn := world.NewSpawner1[boundaryPosition]()
	version := world.StructureVersion()
	entity := spawn.Spawn(boundaryPosition{})
	index, ok := entity.Index()
	if !ok || int(index) >= world.EntitySlots() || world.StructureVersion() <= version {
		t.Fatal("spawn did not expose a metadata slot and structural change")
	}
	version = world.StructureVersion()
	world.MustMut[boundaryPosition](entity).X = 1
	if world.StructureVersion() != version {
		t.Fatal("component write invalidated structural caches")
	}
	if err := world.Insert(entity, 7); err != nil {
		t.Fatal(err)
	}
	if world.StructureVersion() <= version {
		t.Fatal("component insertion did not invalidate structural caches")
	}
	slots, version := world.EntitySlots(), world.StructureVersion()
	world.Despawn(entity)
	if world.StructureVersion() <= version || world.EntitySlots() != slots {
		t.Fatal("despawn must invalidate caches and retain the reusable slot")
	}
	reused := spawn.Spawn(boundaryPosition{})
	reusedIndex, _ := reused.Index()
	if reused == entity || reusedIndex != index || world.IsAlive(entity) || !world.IsAlive(reused) {
		t.Fatal("slot reuse did not preserve generation validation")
	}
	if _, ok := ecs.Entity(0).Index(); ok {
		t.Fatal("zero entity has a valid index")
	}
}

func TestMessageScratchRetainsBorrowedDataUntilMessageExpires(t *testing.T) {
	world := ecs.NewWorld()
	first := world.MessageScratch[messageArenaA, *int]()
	other := world.MessageScratch[messageArenaB, *int]()
	if first != world.MessageScratch[messageArenaA, *int]() || first == other ||
		first == ecs.NewWorld().MessageScratch[messageArenaA, *int]() {
		t.Fatal("message arenas must be stable and isolated by tag and world")
	}
	oldValue, newValue := 10, 20
	first.Values = append(first.Values, &oldValue)
	borrowed := first.Values[:]
	world.WriteMessage(borrowedMessage{Values: borrowed})
	world.AdvanceMessages()
	first.Values = append(first.Values, &newValue)
	world.WriteMessage(borrowedMessage{Values: first.Values[:]})
	var reader ecs.MessageReader[borrowedMessage]
	var got []int
	for message := range reader.All(world) {
		got = append(got, *message.Values[0])
	}
	if !slices.Equal(got, []int{10, 20}) {
		t.Fatalf("borrowed message values = %v, want [10 20]", got)
	}
	world.AdvanceMessages()
	if borrowed[0] != nil {
		t.Fatal("expired message arena retained a reference")
	}
	var lateReader ecs.MessageReader[borrowedMessage]
	count := 0
	for message := range lateReader.All(world) {
		count++
		if value := *message.Values[0]; value != newValue {
			t.Fatalf("retained message value = %d, want %d", value, newValue)
		}
	}
	if count != 1 {
		t.Fatalf("retained messages = %d, want 1", count)
	}
}

func TestMessageScratchDoesNotAllocateAfterWarmup(t *testing.T) {
	world := ecs.NewWorld()
	value := 42
	var reader ecs.MessageReader[borrowedMessage]
	frame := func() {
		arena := world.MessageScratch[messageArenaA, *int]()
		arena.Values = append(arena.Values, &value)
		world.WriteMessage(borrowedMessage{Values: arena.Values[:]})
		for message := range reader.All(world) {
			if len(message.Values) != 1 || message.Values[0] != &value {
				t.Fatal("message data was overwritten before consumption")
			}
		}
		world.AdvanceMessages()
	}
	frame()
	frame()
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("message arena allocations = %v, want 0", allocations)
	}
}

func TestChangesSinceSnapshotsComponentAndResourceWindows(t *testing.T) {
	world := ecs.NewWorld()
	spawn := world.NewSpawner1[boundaryPosition]()
	existing := spawn.Spawn(boundaryPosition{})
	baseline := world.ChangeTick()
	world.MustMut[boundaryPosition](existing).X = 1
	added := spawn.Spawn(boundaryPosition{X: 2})
	world.InsertResource(7)
	tick, version := world.ChangeTick(), world.MutationVersion()
	window := world.ChangesSince(baseline)
	if window.LastRun() != baseline || window.ThisRun() != tick ||
		world.ChangeTick() != tick || world.MutationVersion() != version {
		t.Fatal("snapshot advanced the world or captured the wrong tick window")
	}
	spawn.Spawn(boundaryPosition{X: 3})
	var changed []ecs.Entity
	for entity := range world.Query[boundaryPosition](window.Changed[boundaryPosition]()).All {
		changed = append(changed, entity)
	}
	if !slices.Equal(changed, []ecs.Entity{existing, added}) {
		t.Fatalf("snapshot changed entities = %v, want [%v %v]", changed, existing, added)
	}
	var created []ecs.Entity
	for entity := range world.Query[boundaryPosition](window.Added[boundaryPosition]()).All {
		created = append(created, entity)
	}
	if !slices.Equal(created, []ecs.Entity{added}) || !window.ResourceChanged[int]() {
		t.Fatal("snapshot lost component or resource changes")
	}
	world.InsertResource(8)
	if window.ResourceChanged[int]() {
		t.Fatal("snapshot included a resource write after its end tick")
	}
}

func TestResourceChangeTickIsReadOnly(t *testing.T) {
	world := ecs.NewWorld()
	if world.ResourceChangeTick[int]() != 0 {
		t.Fatal("missing resource has a change tick")
	}
	world.InsertResource(1)
	inserted, version := world.ChangeTick(), world.MutationVersion()
	if world.ResourceChangeTick[int]() != inserted {
		t.Fatal("inserted resource has the wrong change tick")
	}
	world.GetResource[int]()
	if world.ResourceChangeTick[int]() != inserted || world.ChangeTick() != inserted || world.MutationVersion() != version {
		t.Fatal("read-only resource access marked a write")
	}
	world.InsertResource(2)
	replaced := world.ResourceChangeTick[int]()
	if replaced <= inserted {
		t.Fatal("replacement did not advance the resource change tick")
	}
	world.GetResourceMut[int]()
	if world.ResourceChangeTick[int]() <= replaced {
		t.Fatal("mutable access did not advance the resource change tick")
	}
	world.RemoveResource[int]()
	if world.ResourceChangeTick[int]() != 0 {
		t.Fatal("removed resource has a change tick")
	}
}

func TestResourceConditionMethodExpressionsPreserveChangeWindows(t *testing.T) {
	var added ecs.Condition = (*ecs.SystemContext).ResourceAdded[int]
	var changed ecs.Condition = (*ecs.SystemContext).ResourceChanged[int]
	world := ecs.NewWorld()
	world.InsertResource(1)
	initial := world.ChangesSince(0)
	if !added(&initial) || !changed(&initial) {
		t.Fatal("resource insertion was not visible to method expression conditions")
	}
	world.InsertResource(2)
	written := world.ChangesSince(initial.ThisRun())
	if !added(&initial) || changed(&initial) || added(&written) || !changed(&written) {
		t.Fatal("resource conditions escaped their Added/Changed windows")
	}
	missing := ecs.NewWorld().ChangesSince(0)
	for _, context := range []*ecs.SystemContext{nil, {}, &missing} {
		if added(context) || changed(context) {
			t.Fatal("missing context or resource passed a resource condition")
		}
	}
}

func TestChangeFilterMethodsRejectNilAndForeignContexts(t *testing.T) {
	world := ecs.NewWorld()
	foreign := ecs.NewWorld().ChangesSince(0)
	for _, filter := range []struct {
		name  string
		build func(*ecs.SystemContext) ecs.QueryOption
	}{
		{"added", (*ecs.SystemContext).Added[boundaryPosition]},
		{"changed", (*ecs.SystemContext).Changed[boundaryPosition]},
	} {
		for _, check := range []struct {
			name    string
			context *ecs.SystemContext
			message string
		}{
			{"nil", nil, "ecs: system context is nil"},
			{"empty", &ecs.SystemContext{}, "ecs: change filter context belongs to a different world"},
			{"foreign", &foreign, "ecs: change filter context belongs to a different world"},
		} {
			t.Run(filter.name+"/"+check.name, func(t *testing.T) {
				// Options retain the context and validate it when building the query.
				option := filter.build(check.context)
				defer func() {
					if got := recover(); got != check.message {
						t.Fatalf("filter panic = %v, want %q", got, check.message)
					}
				}()
				world.Query[boundaryPosition](option)
			})
		}
	}
}
