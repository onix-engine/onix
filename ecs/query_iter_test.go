package ecs

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestQueryIterationMarksOnlyExplicitWrites(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner2[testPosition, testVelocity]()
	entity := spawner.Spawn(testPosition{X: 1}, testVelocity{})
	positionVersion, velocityVersion := world.ComponentVersion[testPosition](), world.ComponentVersion[testVelocity]()
	for _, row := range world.Query2[testPosition, testVelocity]().All {
		_ = row.A.Get()
		_ = row.B.Ref()
	}
	if world.ComponentVersion[testPosition]() != positionVersion || world.ComponentVersion[testVelocity]() != velocityVersion {
		t.Fatal("reading query components marked them changed")
	}
	for _, row := range world.Query2[testPosition, testVelocity]().All {
		row.B.Mut().X = 2
		if CommitComponent(row.A, row.A.Get()) {
			t.Fatal("equal commit marked a change")
		}
	}
	if world.ComponentVersion[testPosition]() != positionVersion || world.ComponentVersion[testVelocity]() == velocityVersion {
		t.Fatal("write marked the wrong component columns")
	}
	if world.GetRef[testPosition](entity).X != 1 {
		t.Fatal("read component was modified")
	}
}

func TestQueryIterationReleasesStructuralGuard(t *testing.T) {
	for _, exit := range []string{"break", "return", "panic"} {
		t.Run(exit, func(t *testing.T) {
			world := NewWorld()
			spawner := world.NewSpawner1[testPosition]()
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
				for range world.Query[testPosition]().All {
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
			if err := world.Insert(entity, testVelocity{}); err != nil {
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
	spawner := world.NewSpawner2[testPosition, testVelocity]()
	first := spawner.Spawn(testPosition{X: 1}, testVelocity{})
	second := spawner.Spawn(testPosition{X: 2}, testVelocity{})
	stale := spawner.Spawn(testPosition{}, testVelocity{})
	world.Despawn(stale)
	replacement := spawner.Spawn(testPosition{X: 3}, testVelocity{})
	velocities := world.NewSpawner1[testVelocity]()
	missing := velocities.Spawn(testVelocity{})
	context := &SystemContext{World: world, thisRun: world.changeTick}
	entities := []Entity{second, stale, 0, missing, first}
	view := world.Query2[testPosition, testVelocity](context.Added[testPosition](), Without[testAccessC]()).In(entities)
	for range 2 {
		var got []Entity
		for entity := range view.All {
			got = append(got, entity)
		}
		if !slices.Equal(got, []Entity{second, first}) {
			t.Fatalf("visited %v", got)
		}
	}
	if err := world.Insert(first, testAccessC{}); err != nil {
		t.Fatal(err)
	}
	if err := world.Insert(second, testVisibility{}); err != nil {
		t.Fatal(err)
	}
	for entity := range view.All {
		if entity != second {
			t.Fatalf("filtered/migrated entity %d, want %d", entity, second)
		}
	}
	for range world.Query[testPosition]().In(nil).All {
		t.Fatal("nil list visited entities")
	}
	// Saving a restricted view must not alter the unrestricted view or visit replacements.
	count := 0
	for range world.Query[testPosition]().All {
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

func TestMustMutRejectsStaleEmptyAndMissingComponents(t *testing.T) {
	world := NewWorld()
	spawner := world.NewSpawner1[testPosition]()
	entity := spawner.Spawn(testPosition{})
	slot := world.Slot(entity)
	slot.MustMut[testPosition]().X = 5
	world.Despawn(entity)
	missing := world.Slot(world.Spawn(NewBundle(0)))
	for _, check := range []struct {
		entity Entity
		access func()
	}{
		{entity, func() { world.MustMut[testPosition](entity) }},
		{entity, func() { slot.MustMut[testPosition]() }},
		{0, func() { EntitySlot{}.MustMut[testPosition]() }},
		{missing.Entity(), func() { missing.MustMut[testPosition]() }},
	} {
		func() {
			defer func() {
				message := fmt.Sprint(recover())
				if !strings.Contains(message, "testPosition") || !strings.Contains(message, fmt.Sprint(check.entity)) {
					t.Fatalf("missing entity/component diagnostic: %s", message)
				}
			}()
			check.access()
			t.Fatal("required access succeeded")
		}()
	}
}
