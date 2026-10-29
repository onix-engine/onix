package ecs

import (
	"fmt"
	"testing"
	"unsafe"
)

func BenchmarkCommandBatch(b *testing.B) {
	const count = 1000
	for _, mode := range []string{"Insert", "Spawn", "RemovePrepared"} {
		b.Run(mode, func(b *testing.B) {
			world := NewWorld()
			spawner := world.NewSpawner2[testPosition, testVelocity]()
			spawner.Reserve(count)
			entities := make([]Entity, count)
			removal := world.PrepareRemoval(Type[testVelocity]())
			removal.Reserve(count)
			bundle := NewBundle(2)
			bundle.Put(testPosition{})
			bundle.Put(testVelocity{})
			prepared := world.PrepareBundle(bundle)
			commands := world.Commands()
			commands.Reserve(count)
			if mode == "Insert" {
				for i := range entities {
					entities[i] = spawner.Spawn(testPosition{}, testVelocity{})
				}
			}
			run := func() {
				for i := range entities {
					switch mode {
					case "Insert":
						commands.Insert(entities[i], testPosition{X: float32(i)})
					case "Spawn":
						entities[i] = commands.SpawnPrepared(prepared)
					case "RemovePrepared":
						entities[i] = spawner.Spawn(testPosition{}, testVelocity{})
						commands.RemovePrepared(entities[i], removal)
					}
				}
				if err := world.FlushCommands(); err != nil {
					b.Fatal(err)
				}
				if mode != "Insert" {
					for _, entity := range entities {
						world.Despawn(entity)
					}
				}
			}
			run()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				run()
			}
			b.ReportMetric(float64(unsafe.Sizeof(queuedCommand{})), "B/command")
		})
	}
}

func BenchmarkPatchReserveShapes(b *testing.B) {
	for b.Loop() {
		world := NewWorld()
		spawner := world.NewSpawner2[testPosition, testVelocity]()
		spawner.Reserve(1)
		spawner.Spawn(testPosition{}, testVelocity{})
		// Registered but unused shapes must not multiply the capacity request.
		world.NewSpawner1[testName]()
		world.NewSpawner1[testVisibility]()
		world.NewSpawner1[testParent]()
		world.NewSpawner1[testAnchor]()
		world.NewSpawner1[testStack]()
		patch := world.NewPatch1[testSize](Type[testVelocity]())
		patch.Reserve(1024)
	}
	b.ReportAllocs()
}

func BenchmarkChangedQueryDistribution(b *testing.B) {
	for _, count := range []int{10_000, 100_000} {
		for _, pattern := range []string{"None", "One", "Scattered", "All"} {
			b.Run(fmt.Sprintf("%d/%s", count, pattern), func(b *testing.B) {
				world := NewWorld()
				spawner := world.NewSpawner1[testPosition]()
				spawner.Reserve(count)
				entities := make([]Entity, count)
				for i := range entities {
					entities[i] = spawner.Spawn(testPosition{X: 1})
				}
				last := world.ChangeTick()
				for i, entity := range entities {
					if pattern == "All" || pattern == "One" && i == count/2 || pattern == "Scattered" && i%64 == 0 {
						world.GetMut[testPosition](entity).X++
					}
				}
				window := world.ChangesSince(last)
				run := func() {
					var total float32
					for _, access := range world.Query[testPosition](window.Changed[testPosition]()).All {
						total += access.Ref().X
					}
					benchmarkTotal = total
				}
				run()
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					run()
				}
			})
		}
	}
}

// Include cache invalidation and rebuilding in the timing, as systems create a
// new change window every frame rather than repeatedly reading one snapshot.
func BenchmarkChangedQueryFrames(b *testing.B) {
	world := NewWorld()
	spawner := world.NewSpawner1[testPosition]()
	spawner.Reserve(100_000)
	var changed Entity
	for i := range 100_000 {
		entity := spawner.Spawn(testPosition{})
		if i == 50_000 {
			changed = entity
		}
	}
	window := world.ChangesSince(0)
	run := func() {
		last := world.ChangeTick()
		world.GetMut[testPosition](changed).X++
		window = world.ChangesSince(last)
		for _, access := range world.Query[testPosition](window.Changed[testPosition]()).All {
			benchmarkTotal = access.Ref().X
		}
	}
	run()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		run()
	}
}
