package ecs

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestQueryIterationMarksOnlyExplicitWrites(t *testing.T) {
	world := NewWorld()
	spawner := NewSpawner2[testPosition, testVelocity](world)
	entity := spawner.Spawn(testPosition{X: 1}, testVelocity{})
	positionVersion, velocityVersion := ComponentVersion[testPosition](world), ComponentVersion[testVelocity](world)
	for _, row := range Query2[testPosition, testVelocity](world).All {
		_ = row.A.Get()
		_ = row.B.Ref()
	}
	if ComponentVersion[testPosition](world) != positionVersion || ComponentVersion[testVelocity](world) != velocityVersion {
		t.Fatal("reading query components marked them changed")
	}
	for _, row := range Query2[testPosition, testVelocity](world).All {
		row.B.Mut().X = 2
		if CommitComponent(row.A, row.A.Get()) {
			t.Fatal("equal commit marked a change")
		}
	}
	if ComponentVersion[testPosition](world) != positionVersion || ComponentVersion[testVelocity](world) == velocityVersion {
		t.Fatal("write marked the wrong component columns")
	}
	if GetRef[testPosition](world, entity).X != 1 {
		t.Fatal("read component was modified")
	}
}

func TestQueryIterationReleasesStructuralGuard(t *testing.T) {
	for _, exit := range []string{"break", "return", "panic"} {
		t.Run(exit, func(t *testing.T) {
			world := NewWorld()
			spawner := NewSpawner1[testPosition](world)
			entity := spawner.Spawn(testPosition{})
			func() {
				defer func() {
					recovered := recover()
					if exit == "panic" && recovered != "stop" {
						t.Fatalf("panic = %v", recovered)
					}
					if exit != "panic" && recovered != nil {
						t.Fatalf("unexpected panic: %v", recovered)
					}
				}()
				for range Query[testPosition](world).All {
					switch exit {
					case "break":
						break
					case "return":
						return
					case "panic":
						panic("stop")
					}
					break
				}
			}()
			if world.activeQueryIterators != 0 {
				t.Fatal("iterator guard was not released")
			}
			if err := Insert(world, entity, testVelocity{}); err != nil {
				t.Fatal(err)
			}
		})
	}
	var zero QueryView1[testPosition]
	zero.All(nil)
	for range zero.All {
		t.Fatal("zero view visited an entity")
	}
}

func TestQueryInPreservesOrderFiltersAndGenerations(t *testing.T) {
	world := NewWorld()
	spawner := NewSpawner2[testPosition, testVelocity](world)
	first := spawner.Spawn(testPosition{X: 1}, testVelocity{})
	second := spawner.Spawn(testPosition{X: 2}, testVelocity{})
	stale := spawner.Spawn(testPosition{}, testVelocity{})
	world.Despawn(stale)
	replacement := spawner.Spawn(testPosition{X: 3}, testVelocity{})
	velocities := NewSpawner1[testVelocity](world)
	missing := velocities.Spawn(testVelocity{})
	context := &SystemContext{World: world, thisRun: world.changeTick}
	entities := []Entity{second, stale, 0, missing, first}
	view := Query2[testPosition, testVelocity](world, Added[testPosition](context), Without[testAccessC]()).In(entities)
	for range 2 {
		var got []Entity
		for entity := range view.All {
			got = append(got, entity)
		}
		if !slices.Equal(got, []Entity{second, first}) {
			t.Fatalf("visited %v", got)
		}
	}
	if err := Insert(world, first, testAccessC{}); err != nil {
		t.Fatal(err)
	}
	if err := Insert(world, second, testVisibility{}); err != nil {
		t.Fatal(err)
	}
	for entity := range view.All {
		if entity != second {
			t.Fatalf("filtered/migrated entity %d, want %d", entity, second)
		}
	}
	for range Query[testPosition](world).In(nil).All {
		t.Fatal("nil list visited entities")
	}
	// Saving a restricted view must not alter the unrestricted view or visit replacements.
	count := 0
	for range Query[testPosition](world).All {
		count++
	}
	if count != 3 || replacement == stale {
		t.Fatal("generation or view isolation failed")
	}
	run := func() {
		for _, access := range view.All {
			_ = access.A.Get()
		}
	}
	run()
	if allocs := testing.AllocsPerRun(100, run); allocs != 0 {
		t.Fatalf("limited query allocations = %v", allocs)
	}
}

func TestMustMutRejectsStaleAndForeignSlots(t *testing.T) {
	world := NewWorld()
	spawner := NewSpawner1[testPosition](world)
	entity := spawner.Spawn(testPosition{})
	slot := world.Slot(entity)
	MustMutSlot[testPosition](world, slot).X = 5
	world.Despawn(entity)
	for _, access := range []func(){
		func() { MustMut[testPosition](world, entity) },
		func() { MustMutSlot[testPosition](world, slot) },
		func() { MustMutSlot[testPosition](NewWorld(), slot) },
	} {
		func() {
			defer func() {
				message := fmt.Sprint(recover())
				if !strings.Contains(message, "testPosition") || !strings.Contains(message, fmt.Sprint(entity)) {
					t.Fatalf("missing entity/component diagnostic: %s", message)
				}
			}()
			access()
			t.Fatal("required access succeeded")
		}()
	}
}
