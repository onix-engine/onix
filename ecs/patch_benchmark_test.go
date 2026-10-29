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
			spawn := world.NewSpawner6[patchBenchRemovedA, patchBenchRemovedB, testPosition, testVelocity, testName, testSize]()
			entity := spawn.Spawn(patchBenchRemovedA{}, patchBenchRemovedB{}, testPosition{}, testVelocity{}, testName{}, testSize{})
			forward := world.NewPatch2[patchBenchInsertedA, patchBenchInsertedB](Type[patchBenchRemovedA](), Type[patchBenchRemovedB]())
			backward := world.NewPatch2[patchBenchRemovedA, patchBenchRemovedB](Type[patchBenchInsertedA](), Type[patchBenchInsertedB]())
			run := func() {
				switch mode {
				case "Sequential":
					world.Remove[patchBenchRemovedA](entity)
					world.Remove[patchBenchRemovedB](entity)
					if err := world.Insert(entity, patchBenchInsertedA{Value: 1}); err != nil {
						b.Fatal(err)
					}
					if err := world.Insert(entity, patchBenchInsertedB{Value: 2}); err != nil {
						b.Fatal(err)
					}
					world.Remove[patchBenchInsertedA](entity)
					world.Remove[patchBenchInsertedB](entity)
					if err := world.Insert(entity, patchBenchRemovedA{Value: 3}); err != nil {
						b.Fatal(err)
					}
					if err := world.Insert(entity, patchBenchRemovedB{Value: 4}); err != nil {
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
			if world.GetRef[patchBenchRemovedA](entity).Value != 3 || world.GetRef[patchBenchRemovedB](entity).Value != 4 {
				b.Fatal("incorrect final payload")
			}
		})
	}
}

func BenchmarkCommandInsertRecycled(b *testing.B) {
	world := NewWorld()
	spawn := world.NewSpawner1[testPosition]()
	entity := spawn.Spawn(testPosition{})
	run := func() {
		world.Commands().Insert(entity, testPosition{X: 1})
		if err := world.FlushCommands(); err != nil {
			b.Fatal(err)
		}
	}
	run()
	b.ReportAllocs()
	for b.Loop() {
		run()
	}
	if world.GetRef[testPosition](entity).X != 1 {
		b.Fatal("insert command did not apply")
	}
}
