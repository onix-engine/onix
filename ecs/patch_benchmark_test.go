package ecs

import "testing"

type patchBenchRemovedA struct{ Value int }
type patchBenchRemovedB struct{ Value int }
type patchBenchInsertedA struct{ Value int }
type patchBenchInsertedB struct{ Value int }

// Each iteration replaces two components in both directions while retaining
// four others. The payload and final values match the sequential operation.
func BenchmarkMixedPatch(b *testing.B) {
	for _, mode := range []string{"Sequential", "Patch", "Commands"} {
		b.Run(mode, func(b *testing.B) {
			world := NewWorld()
			spawn := NewSpawner6[patchBenchRemovedA, patchBenchRemovedB, testPosition, testVelocity, testName, testSize](world)
			entity := spawn.Spawn(patchBenchRemovedA{}, patchBenchRemovedB{}, testPosition{}, testVelocity{}, testName{}, testSize{})
			forward := NewPatch2[patchBenchInsertedA, patchBenchInsertedB](world, Type[patchBenchRemovedA](), Type[patchBenchRemovedB]())
			backward := NewPatch2[patchBenchRemovedA, patchBenchRemovedB](world, Type[patchBenchInsertedA](), Type[patchBenchInsertedB]())
			run := func() {
				switch mode {
				case "Sequential":
					Remove[patchBenchRemovedA](world, entity)
					Remove[patchBenchRemovedB](world, entity)
					if err := Insert(world, entity, patchBenchInsertedA{Value: 1}); err != nil {
						b.Fatal(err)
					}
					if err := Insert(world, entity, patchBenchInsertedB{Value: 2}); err != nil {
						b.Fatal(err)
					}
					Remove[patchBenchInsertedA](world, entity)
					Remove[patchBenchInsertedB](world, entity)
					if err := Insert(world, entity, patchBenchRemovedA{Value: 3}); err != nil {
						b.Fatal(err)
					}
					if err := Insert(world, entity, patchBenchRemovedB{Value: 4}); err != nil {
						b.Fatal(err)
					}
				case "Patch":
					if err := forward.Apply(entity, patchBenchInsertedA{Value: 1}, patchBenchInsertedB{Value: 2}); err != nil {
						b.Fatal(err)
					}
					if err := backward.Apply(entity, patchBenchRemovedA{Value: 3}, patchBenchRemovedB{Value: 4}); err != nil {
						b.Fatal(err)
					}
				case "Commands":
					forward.Queue(world.Commands(), entity, patchBenchInsertedA{Value: 1}, patchBenchInsertedB{Value: 2})
					backward.Queue(world.Commands(), entity, patchBenchRemovedA{Value: 3}, patchBenchRemovedB{Value: 4})
					if err := world.FlushCommands(); err != nil {
						b.Fatal(err)
					}
				}
			}
			run()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				run()
			}
			if GetRef[patchBenchRemovedA](world, entity).Value != 3 || GetRef[patchBenchRemovedB](world, entity).Value != 4 {
				b.Fatal("incorrect final payload")
			}
		})
	}
}

func BenchmarkCommandInsertRecycled(b *testing.B) {
	world := NewWorld()
	spawn := NewSpawner1[testPosition](world)
	entity := spawn.Spawn(testPosition{})
	run := func() {
		CommandInsert(world.Commands(), entity, testPosition{X: 1})
		if err := world.FlushCommands(); err != nil {
			b.Fatal(err)
		}
	}
	run()
	b.ReportAllocs()
	for b.Loop() {
		run()
	}
	if GetRef[testPosition](world, entity).X != 1 {
		b.Fatal("insert command did not apply")
	}
}
