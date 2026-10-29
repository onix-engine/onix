package ecs

import (
	"fmt"
	"testing"
)

type memoryComponent[Tag any] struct {
	Owner Entity
	Value int
}

func putMemoryComponent[Tag any](bundle *Bundle, dynamic bool) {
	if dynamic {
		bundle.PutEntity(func(entity Entity) memoryComponent[Tag] {
			return memoryComponent[Tag]{Owner: entity, Value: 42}
		})
	} else {
		bundle.Put(memoryComponent[Tag]{Value: 42})
	}
}

func memoryBundle(count int, dynamic bool) Bundle {
	bundle := NewBundle(count)
	putMemoryComponent[[0]byte](&bundle, dynamic)
	putMemoryComponent[[1]byte](&bundle, dynamic)
	if count == 2 {
		return bundle
	}
	putMemoryComponent[[2]byte](&bundle, dynamic)
	putMemoryComponent[[3]byte](&bundle, dynamic)
	putMemoryComponent[[4]byte](&bundle, dynamic)
	putMemoryComponent[[5]byte](&bundle, dynamic)
	putMemoryComponent[[6]byte](&bundle, dynamic)
	putMemoryComponent[[7]byte](&bundle, dynamic)
	if count == 8 {
		return bundle
	}
	putMemoryComponent[[8]byte](&bundle, dynamic)
	putMemoryComponent[[9]byte](&bundle, dynamic)
	putMemoryComponent[[10]byte](&bundle, dynamic)
	putMemoryComponent[[11]byte](&bundle, dynamic)
	putMemoryComponent[[12]byte](&bundle, dynamic)
	putMemoryComponent[[13]byte](&bundle, dynamic)
	putMemoryComponent[[14]byte](&bundle, dynamic)
	putMemoryComponent[[15]byte](&bundle, dynamic)
	return bundle
}

// Reuse descriptions and entity slots so the measurement isolates per-spawn
// temporary storage rather than permanent table growth or bundle ownership.
func BenchmarkSpawnScratch(b *testing.B) {
	for _, count := range []int{2, 8, 16} {
		for _, dynamic := range []bool{false, true} {
			for _, prepared := range []bool{false, true} {
				b.Run(fmt.Sprintf("components=%d/dynamic=%t/prepared=%t", count, dynamic, prepared), func(b *testing.B) {
					world := NewWorld()
					bundle := memoryBundle(count, dynamic)
					cached := world.PrepareBundle(bundle)
					run := func() {
						var entity Entity
						if prepared {
							entity = world.SpawnPrepared(cached)
						} else {
							entity = world.Spawn(bundle)
						}
						world.Despawn(entity)
					}
					run()
					b.ReportAllocs()
					for b.Loop() {
						run()
					}
				})
			}
		}
	}
}

func memoryQueryOptions(context *SystemContext, count int) []QueryOption {
	options := make([]QueryOption, count)
	for index := range options {
		options[index] = context.Changed[testPosition]()
	}
	return options
}

func BenchmarkQueryBindingScratch(b *testing.B) {
	for _, count := range []int{1, 8, 9, 16, 32} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world := NewWorld()
			spawner := world.NewSpawner1[testPosition]()
			spawner.Spawn(testPosition{X: 1})
			context := world.ChangesSince(0)
			view := world.Query[testPosition](memoryQueryOptions(&context, count)...)
			consume := func() {
				for _, access := range view.All {
					benchmarkTotal += access.Ref().X
				}
			}
			consume()
			b.ReportAllocs()
			for b.Loop() {
				consume()
			}
		})
	}
}

func BenchmarkScheduleDependencyScratch(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			schedule := NewSchedule("chain")
			previous := ""
			for index := range count {
				config := SystemConfig{Name: fmt.Sprintf("system_%06d", index), System: func(*SystemContext) error { return nil }}
				if previous != "" {
					config.After = []string{previous}
				}
				if err := schedule.Add(config); err != nil {
					b.Fatal(err)
				}
				previous = config.Name
			}
			if err := schedule.compile(); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				schedule.dirty = true
				if err := schedule.compile(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
