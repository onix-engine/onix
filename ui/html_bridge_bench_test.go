package ui

import (
	"strconv"
	"testing"

	"github.com/onix-engine/onix/ecs"
	htmlui "github.com/onix-engine/onix/ui/html"
)

func BenchmarkHTMLControlBridge(b *testing.B) {
	for _, count := range []int{10, 10000} {
		for _, dirty := range []bool{false, true} {
			mode := "clean"
			if dirty {
				mode = "one-dirty"
			}
			b.Run(strconv.Itoa(count)+"/"+mode, func(b *testing.B) {
				world := ecs.NewWorld()
				spawner := world.NewSpawner1[htmlui.Control]()
				var entity ecs.Entity
				for range count {
					entity = spawner.Spawn(htmlui.Control{Tag: "input", Value: "value", RenderRevision: 1})
				}
				for range 2 {
					enrichHTMLControls(world)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if dirty {
						world.GetMut[htmlui.Control](entity).RenderRevision++
					}
					enrichHTMLControls(world)
				}
			})
		}
	}
}
