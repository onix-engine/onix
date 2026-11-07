package html

import (
	"strconv"
	"testing"

	"github.com/onix-engine/onix/ecs"
)

func BenchmarkHTMLComponentTraversal(b *testing.B) {
	for _, count := range []int{10, 10000} {
		for _, mode := range []string{"clean", "one-dirty", "idle"} {
			b.Run(strconv.Itoa(count)+"/"+mode, func(b *testing.B) {
				world := ecs.NewWorld()
				spawner := world.NewSpawner1[htmlComponentInstance]()
				definition := &htmlDefinitionRuntime{}
				var entity ecs.Entity
				for range count {
					entity = spawner.Spawn(htmlComponentInstance{Definition: definition})
				}
				system := SyncComponentsSystem("")
				for range 2 {
					markHTMLComponentDirty(world, entity)
					if err := system.Run(world); err != nil {
						b.Fatal(err)
					}
					NeedsFrame(world)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if mode == "idle" {
						NeedsFrame(world)
						continue
					}
					if mode == "one-dirty" {
						markHTMLComponentDirty(world, entity)
					}
					if err := system.Run(world); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
