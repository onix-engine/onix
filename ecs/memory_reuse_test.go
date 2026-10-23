package ecs

import (
	"fmt"
	"runtime"
	"slices"
	"testing"
)

func TestTemporarySlicePoolClearsReferencesAndBoundsCapacity(t *testing.T) {
	var pool temporarySlicePool[*int]
	storage := pool.get(16)
	value := 42
	for index := range *storage {
		(*storage)[index] = &value
	}
	retained := *storage
	other := pool.get(16)
	if &(*other)[0] == &retained[0] {
		t.Fatal("active borrowers shared storage")
	}
	pool.put(other)
	pool.put(storage)
	for _, pointer := range retained {
		if pointer != nil {
			t.Fatal("returned storage retained a reference")
		}
	}
	oversized := pool.get(maxTemporarySliceCapacity + 1)
	pool.put(oversized)
	if *oversized != nil {
		t.Fatal("pool retained an oversized backing array")
	}
	// Pool contents may disappear at any GC; correctness cannot rely on reuse.
	runtime.GC()
	runtime.GC()
	storage = pool.get(16)
	defer pool.put(storage)
	for _, pointer := range *storage {
		if pointer != nil {
			t.Fatal("borrowed storage contained stale references")
		}
	}
}

func TestLargeSpawnScratchSupportsReentryAndPanic(t *testing.T) {
	for _, prepared := range []bool{false, true} {
		t.Run(fmt.Sprint(prepared), func(t *testing.T) {
			world := NewWorld()
			inner := memoryBundle(16, true)
			outer := memoryBundle(16, true)
			outer.entries = outer.entries[:15]
			fail := true
			var nested Entity
			PutEntity(&outer, func(entity Entity) memoryComponent[[15]byte] {
				nested = world.Spawn(inner)
				if fail {
					panic("builder failed")
				}
				return memoryComponent[[15]byte]{Owner: entity, Value: 99}
			})
			cached := world.PrepareBundle(outer)
			spawn := func() Entity {
				if prepared {
					return world.SpawnPrepared(cached)
				}
				return world.Spawn(outer)
			}
			func() {
				defer func() {
					if value := recover(); value != "builder failed" {
						t.Fatalf("panic = %v", value)
					}
				}()
				spawn()
			}()
			if world.EntityCount() != 1 || len(world.free) != 1 {
				t.Fatal("builder panic leaked the outer reservation")
			}
			world.Despawn(nested)
			fail = false
			entity := spawn()
			first := GetRef[memoryComponent[[0]byte]](world, entity)
			last := GetRef[memoryComponent[[15]byte]](world, entity)
			nestedLast := GetRef[memoryComponent[[15]byte]](world, nested)
			if first.Owner != entity || first.Value != 42 || last.Owner != entity || last.Value != 99 ||
				nestedLast.Owner != nested || nestedLast.Value != 42 || world.EntityCount() != 2 {
				t.Fatal("reentrant spawn overwrote borrowed entries or materialized values")
			}
		})
	}
}

func TestOverflowQueryScratchKeepsNestedSnapshotsAndReleasesGuard(t *testing.T) {
	world := NewWorld()
	spawner := NewSpawner1[testPosition](world)
	spawner.Spawn(testPosition{X: 1})
	since := world.ChangeTick()
	spawner.Spawn(testPosition{X: 2})
	outerWindow := world.ChangesSince(0)
	innerWindow := world.ChangesSince(since)
	outer := Query[testPosition](world, memoryQueryOptions(&outerWindow, 9)...)
	inner := Query[testPosition](world, memoryQueryOptions(&innerWindow, 32)...)
	for range 2 {
		count := 0
		for range outer.All {
			count++
			nestedCount := 0
			for _, value := range inner.All {
				nestedCount++
				if value.Ref().X != 2 {
					t.Fatal("inner query lost its filter snapshot")
				}
			}
			if nestedCount != 1 {
				t.Fatalf("inner count = %d, want 1", nestedCount)
			}
		}
		if count != 2 {
			t.Fatalf("outer count = %d, want 2", count)
		}
	}
	for _, exit := range []string{"break", "return", "panic"} {
		func() {
			defer func() {
				if value := recover(); (value != nil) != (exit == "panic") {
					t.Fatalf("exit=%s panic=%v", exit, value)
				}
			}()
			for range outer.All {
				if exit == "panic" {
					panic("stop")
				}
				if exit == "return" {
					return
				}
				break
			}
		}()
		if world.activeQueryIterators != 0 {
			t.Fatalf("%s retained an active query guard", exit)
		}
	}
}

func TestCommandSpawnPanicClearsAbandonedEntries(t *testing.T) {
	world := NewWorld()
	valid := NewBundle(1)
	Put(&valid, testPosition{X: 7})
	entity := CommandSpawn(world.Commands(), valid)
	invalid := NewBundle(2)
	Put(&invalid, testName{Value: "retained on panic"})
	Put(&invalid, testName{Value: "duplicate"})
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("duplicate bundle did not panic")
			}
		}()
		CommandSpawn(world.Commands(), invalid)
	}()
	entries := world.commands.spawnEntries
	for _, entry := range entries[len(entries):cap(entries)] {
		if entry.entry != nil {
			t.Fatal("failed command retained an abandoned bundle reference")
		}
	}
	if err := world.FlushCommands(); err != nil || GetRef[testPosition](world, entity).X != 7 {
		t.Fatalf("failed command disturbed the preceding command: %v", err)
	}
}

func TestScheduleScratchReleasesReferencesAndRecoversAfterErrors(t *testing.T) {
	scratch := borrowScheduleScratch()
	scratch.indices["old"] = 0
	scratch.nodes = append(scratch.nodes, scheduleNode{name: "old"})
	scratch.ready = append(scratch.ready, "old")
	nodes, ready := scratch.nodes, scratch.ready
	scratch.release()
	if nodes[0] != (scheduleNode{}) || ready[0] != "" {
		t.Fatal("returned graph retained system names")
	}
	schedule := NewSchedule("rebuild")
	run := func(*SystemContext) error { return nil }
	if err := schedule.Add(SystemConfig{Name: "a", After: []string{"b"}, System: run}); err != nil {
		t.Fatal(err)
	}
	if _, err := schedule.Order(); err == nil {
		t.Fatal("missing dependency accepted")
	}
	if err := schedule.Add(SystemConfig{Name: "b", After: []string{"a"}, System: run}); err != nil {
		t.Fatal(err)
	}
	if _, err := schedule.Order(); err == nil {
		t.Fatal("dependency cycle accepted")
	}
	schedule.Remove("b")
	if err := schedule.Add(SystemConfig{Name: "b", System: run}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := schedule.Order()
	if err != nil || !slices.Equal(snapshot, []string{"b", "a"}) {
		t.Fatalf("order after errors = %v, %v", snapshot, err)
	}
	schedule.Remove("a")
	order, err := schedule.Order()
	if err != nil || !slices.Equal(order, []string{"b"}) || !slices.Equal(snapshot, []string{"b", "a"}) {
		t.Fatalf("recompile changed an owned order: old=%v new=%v err=%v", snapshot, order, err)
	}
	for _, old := range schedule.ordered[len(schedule.ordered):cap(schedule.ordered)] {
		if old.context != nil || old.config.System != nil {
			t.Fatal("schedule retained a removed system")
		}
	}
}

func TestPooledScratchSupportsConcurrentWorlds(t *testing.T) {
	for index := range 8 {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			t.Parallel()
			world := NewWorld()
			bundle := memoryBundle(16, true)
			schedule := NewSchedule("concurrent")
			run := func(*SystemContext) error { return nil }
			if err := schedule.Add(SystemConfig{Name: "a", After: []string{"b"}, System: run}); err != nil {
				t.Fatal(err)
			}
			if err := schedule.Add(SystemConfig{Name: "b", System: run}); err != nil {
				t.Fatal(err)
			}
			window := world.ChangesSince(0)
			options := make([]QueryOption, 9)
			for index := range options {
				options[index] = Changed[memoryComponent[[0]byte]](&window)
			}
			for range 100 {
				entity := world.Spawn(bundle)
				if value := GetRef[memoryComponent[[15]byte]](world, entity); value.Owner != entity || value.Value != 42 {
					t.Fatal("pooled storage mixed independent worlds")
				}
				window = world.ChangesSince(0)
				for range Query[memoryComponent[[0]byte]](world, options...).All {
				}
				schedule.dirty = true
				order, err := schedule.Order()
				if err != nil || !slices.Equal(order, []string{"b", "a"}) {
					t.Fatalf("concurrent compilation: order=%v err=%v", order, err)
				}
				world.Despawn(entity)
			}
		})
	}
}
