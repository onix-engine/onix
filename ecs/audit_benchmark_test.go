package ecs

import (
	"fmt"
	"testing"
)

// Isolate empty flush cost after a world has used many command value types.
// Stream type identity is irrelevant to resetStorage; each entry owns storage.
func BenchmarkAuditEmptyCommandFlush(b *testing.B) {
	for _, count := range []int{0, 64, 1024} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world := NewWorld()
			for range count {
				world.commands.streamList = append(world.commands.streamList, &typedInsertCommandStream[int]{})
			}
			b.ReportAllocs()
			for b.Loop() {
				if err := world.FlushCommands(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// One active value stream among many retained streams isolates cleanup work
// from the size of the current command batch.
func BenchmarkAuditSingleActiveCommand(b *testing.B) {
	for _, count := range []int{0, 64, 1024} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world := NewWorld()
			spawn := world.NewSpawner1[testPosition]()
			entity := spawn.Spawn(testPosition{})
			commands := world.Commands()
			for range count {
				commands.streamList = append(commands.streamList, &typedInsertCommandStream[int]{})
			}
			run := func() {
				commands.Insert(entity, testPosition{X: 1})
				if err := world.FlushCommands(); err != nil {
					b.Fatal(err)
				}
			}
			run()
			b.ReportAllocs()
			for b.Loop() {
				run()
			}
		})
	}
}

// Synthetic IDs isolate column binding from reflection and type registration.
func auditRegisterColumns(world *World, count int) []componentID {
	ids := make([]componentID, count)
	for i := range ids {
		id := componentID(len(world.componentInfos))
		ids[i] = id
		world.componentInfos = append(world.componentInfos, componentInfo{
			newColumn: func() column {
				return &typedColumn[int]{columnState: columnState{world: world, id: id}}
			},
		})
	}
	return ids
}

// Destination archetypes are prewarmed; transition bindings are deliberately
// rebuilt to measure the cold path, not steady-state Apply performance.
func BenchmarkAuditTransitionBinding(b *testing.B) {
	for _, count := range []int{8, 64, 256} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world := NewWorld()
			ids := auditRegisterColumns(world, count)
			source := world.archetypeFor(ids)
			removed := maskFor(ids[len(ids)-1:])
			b.Run("Removal", func(b *testing.B) {
				plan := &removalPlan{world: world, mask: removed, transitions: make(map[uint32]*removalTransition)}
				plan.transition(source)
				b.ReportAllocs()
				for b.Loop() {
					clear(plan.transitions)
					plan.transition(source)
				}
			})
			b.Run("Patch", func(b *testing.B) {
				plan := newPatchPlan(world, ids[:1], nil)
				plan.removed = removed
				plan.transition(source)
				b.ReportAllocs()
				for b.Loop() {
					clear(plan.transitions)
					plan.transition(source)
				}
			})
		})
	}
}

// Keep one live entity while increasing the number of historical matching
// archetypes. Empty tables remain cached in queryPlan.archetypes.
func BenchmarkAuditQueryHistoricalArchetypes(b *testing.B) {
	for _, count := range []int{1, 256, 4096} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world := NewWorld()
			spawner := world.NewSpawner1[testPosition]()
			spawner.Spawn(testPosition{X: 1})
			extra := auditRegisterColumns(world, 12)
			for shape := 1; shape < count; shape++ {
				ids := []componentID{0}
				for bit, id := range extra {
					if shape&(1<<bit) != 0 {
						ids = append(ids, id)
					}
				}
				world.archetypeForOwned(ids)
			}
			view := world.Query[testPosition]()
			view.All(func(Entity, ComponentAccess[testPosition]) bool { return true })
			b.ReportAllocs()
			for b.Loop() {
				var total float32
				for _, access := range view.All {
					total += access.Ref().X
				}
				benchmarkTotal = total
			}
		})
	}
}
