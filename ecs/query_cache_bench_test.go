package ecs

import (
	"fmt"
	"testing"
)

func BenchmarkScopeStructuralRefresh(b *testing.B) {
	for _, count := range []int{1000, 10000} {
		for _, mode := range []string{"stable", "outside-table", "outside-historical", "append-same-table", "move-member"} {
			b.Run(fmt.Sprintf("%d/%s", count, mode), func(b *testing.B) {
				world := NewWorld()
				spawn := world.NewSpawner1[int]()
				spawn.Reserve(count + 1)
				members := make([]Entity, count)
				for i := range members {
					members[i] = spawn.Spawn(i)
				}
				others := world.NewSpawner1[float64]()
				outside := others.Spawn(0)
				moving := outside
				if mode == "move-member" {
					moving = members[0]
				}
				if err := world.Insert(moving, true); err != nil {
					b.Fatal(err)
				}
				world.Remove[bool](moving)
				if mode == "outside-historical" {
					extra := auditRegisterColumns(world, 12)
					for shape := 1; shape < 4096; shape++ {
						ids := []componentID{0}
						for bit, id := range extra {
							if shape&(1<<bit) != 0 {
								ids = append(ids, id)
							}
						}
						world.archetypeForOwned(ids)
					}
				}
				var scope QueryScope
				scope.Reset(members)
				scope.bind(world)
				present := false
				b.ReportAllocs()
				for b.Loop() {
					switch mode {
					case "outside-table", "outside-historical", "move-member":
						if present {
							world.Remove[bool](moving)
						} else if err := world.Insert(moving, true); err != nil {
							b.Fatal(err)
						}
						present = !present
					case "append-same-table":
						if outside != 0 {
							world.Despawn(outside)
							outside = 0
						} else {
							outside = spawn.Spawn(0)
						}
					}
					scope.bind(world)
				}
			})
		}
	}
}

// All columns use int storage so synthetic component IDs can model table
// distributions independently of reflection/type-registration overhead.
func spawnQueryDistributionEntity(world *World, table *archetype) Entity {
	entity := world.reserveEntity()
	row, tick := table.appendEntity(entity), world.mutationTick()
	for _, storage := range table.columns {
		storage.(*typedColumn[int]).append(1, tick)
	}
	index, _ := entity.Index()
	world.commitSpawn(index, table, row)
	return entity
}

func BenchmarkQueryTableDistribution(b *testing.B) {
	for _, mode := range []string{"dense-live", "dense-empty", "sparse-matches", "sparse-live"} {
		b.Run(mode, func(b *testing.B) {
			world := NewWorld()
			world.registerComponent[int]()
			extra := auditRegisterColumns(world, 12)
			for shape := range 4096 {
				ids := make([]componentID, 0, 13)
				matches := mode != "sparse-matches" || shape%1024 == 0
				if matches {
					ids = append(ids, 0)
				}
				for bit, id := range extra {
					if shape&(1<<bit) != 0 {
						ids = append(ids, id)
					}
				}
				table := world.archetypeForOwned(ids)
				if mode == "dense-live" || mode == "sparse-matches" && matches || mode == "sparse-live" && shape%1024 == 0 {
					spawnQueryDistributionEntity(world, table)
				}
			}
			query := world.Query[int]()
			for range query.All {
			}
			b.ReportAllocs()
			for b.Loop() {
				total := 0
				for _, access := range query.All {
					total += access.Get()
				}
				benchmarkTotal = float32(total)
			}
		})
	}
}
