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
	if version, structure := ecs.ComponentRevision[boundaryPosition](world); version != 0 || structure != 0 || world.ChangeTick() != tick {
		t.Fatal("reading an unregistered component revision mutated the world")
	}
	spawn := ecs.NewSpawner1[boundaryPosition](world)
	entity := spawn.Spawn(boundaryPosition{})
	version, structure := ecs.ComponentRevision[boundaryPosition](world)
	ecs.MustMut[boundaryPosition](world, entity).X = 1
	written, unchanged := ecs.ComponentRevision[boundaryPosition](world)
	if written <= version || unchanged != structure {
		t.Fatal("component write must change its content version only")
	}
	if err := ecs.Insert(world, entity, 7); err != nil {
		t.Fatal(err)
	}
	moved, shifted := ecs.ComponentRevision[boundaryPosition](world)
	if moved <= written || shifted <= structure {
		t.Fatal("row migration must invalidate content and structural caches")
	}
}

func TestEntityIndexAndStructureVersionSupportExternalCaches(t *testing.T) {
	world := ecs.NewWorld()
	spawn := ecs.NewSpawner1[boundaryPosition](world)
	version := world.StructureVersion()
	entity := spawn.Spawn(boundaryPosition{})
	index, ok := entity.Index()
	if !ok || int(index) >= world.EntitySlots() || world.StructureVersion() <= version {
		t.Fatal("spawn did not expose a metadata slot and structural change")
	}
	version = world.StructureVersion()
	ecs.MustMut[boundaryPosition](world, entity).X = 1
	if world.StructureVersion() != version {
		t.Fatal("component write invalidated structural caches")
	}
	if err := ecs.Insert(world, entity, 7); err != nil {
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
	first := ecs.MessageScratch[messageArenaA, *int](world)
	other := ecs.MessageScratch[messageArenaB, *int](world)
	if first != ecs.MessageScratch[messageArenaA, *int](world) || first == other ||
		first == ecs.MessageScratch[messageArenaA, *int](ecs.NewWorld()) {
		t.Fatal("message arenas must be stable and isolated by tag and world")
	}
	oldValue, newValue := 10, 20
	first.Values = append(first.Values, &oldValue)
	borrowed := first.Values[:]
	ecs.WriteMessage(world, borrowedMessage{Values: borrowed})
	ecs.AdvanceMessages(world)
	first.Values = append(first.Values, &newValue)
	ecs.WriteMessage(world, borrowedMessage{Values: first.Values[:]})
	var reader ecs.MessageReader[borrowedMessage]
	var got []int
	for message := range reader.All(world) {
		got = append(got, *message.Values[0])
	}
	if !slices.Equal(got, []int{10, 20}) {
		t.Fatalf("borrowed message values = %v, want [10 20]", got)
	}
	ecs.AdvanceMessages(world)
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
		arena := ecs.MessageScratch[messageArenaA, *int](world)
		arena.Values = append(arena.Values, &value)
		ecs.WriteMessage(world, borrowedMessage{Values: arena.Values[:]})
		for message := range reader.All(world) {
			if len(message.Values) != 1 || message.Values[0] != &value {
				t.Fatal("message data was overwritten before consumption")
			}
		}
		ecs.AdvanceMessages(world)
	}
	frame()
	frame()
	if allocations := testing.AllocsPerRun(100, frame); allocations != 0 {
		t.Fatalf("message arena allocations = %v, want 0", allocations)
	}
}

func TestChangesSinceSnapshotsComponentAndResourceWindows(t *testing.T) {
	world := ecs.NewWorld()
	spawn := ecs.NewSpawner1[boundaryPosition](world)
	existing := spawn.Spawn(boundaryPosition{})
	baseline := world.ChangeTick()
	ecs.MustMut[boundaryPosition](world, existing).X = 1
	added := spawn.Spawn(boundaryPosition{X: 2})
	ecs.InsertResource(world, 7)
	tick, version := world.ChangeTick(), world.MutationVersion()
	window := world.ChangesSince(baseline)
	if window.LastRun() != baseline || window.ThisRun() != tick ||
		world.ChangeTick() != tick || world.MutationVersion() != version {
		t.Fatal("snapshot advanced the world or captured the wrong tick window")
	}
	spawn.Spawn(boundaryPosition{X: 3})
	var changed []ecs.Entity
	for entity := range ecs.Query[boundaryPosition](world, ecs.Changed[boundaryPosition](&window)).All {
		changed = append(changed, entity)
	}
	if !slices.Equal(changed, []ecs.Entity{existing, added}) {
		t.Fatalf("snapshot changed entities = %v, want [%v %v]", changed, existing, added)
	}
	var created []ecs.Entity
	for entity := range ecs.Query[boundaryPosition](world, ecs.Added[boundaryPosition](&window)).All {
		created = append(created, entity)
	}
	if !slices.Equal(created, []ecs.Entity{added}) || !ecs.ResourceChanged[int](&window) {
		t.Fatal("snapshot lost component or resource changes")
	}
	ecs.InsertResource(world, 8)
	if ecs.ResourceChanged[int](&window) {
		t.Fatal("snapshot included a resource write after its end tick")
	}
}

func TestResourceChangeTickIsReadOnly(t *testing.T) {
	world := ecs.NewWorld()
	if ecs.ResourceChangeTick[int](world) != 0 {
		t.Fatal("missing resource has a change tick")
	}
	ecs.InsertResource(world, 1)
	inserted, version := world.ChangeTick(), world.MutationVersion()
	if ecs.ResourceChangeTick[int](world) != inserted {
		t.Fatal("inserted resource has the wrong change tick")
	}
	ecs.GetResource[int](world)
	if ecs.ResourceChangeTick[int](world) != inserted || world.ChangeTick() != inserted || world.MutationVersion() != version {
		t.Fatal("read-only resource access marked a write")
	}
	ecs.InsertResource(world, 2)
	replaced := ecs.ResourceChangeTick[int](world)
	if replaced <= inserted {
		t.Fatal("replacement did not advance the resource change tick")
	}
	ecs.GetResourceMut[int](world)
	if ecs.ResourceChangeTick[int](world) <= replaced {
		t.Fatal("mutable access did not advance the resource change tick")
	}
	ecs.RemoveResource[int](world)
	if ecs.ResourceChangeTick[int](world) != 0 {
		t.Fatal("removed resource has a change tick")
	}
}
