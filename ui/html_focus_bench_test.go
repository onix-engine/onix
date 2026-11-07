package ui

import (
	"fmt"
	"testing"

	"github.com/onix-engine/onix/ecs"
	htmlui "github.com/onix-engine/onix/ui/html"
)

func BenchmarkHTMLFocusLookup(b *testing.B) {
	for _, count := range []int{100, 10000} {
		for _, capture := range []bool{false, true} {
			b.Run(fmt.Sprintf("nodes=%d/capture=%v", count, capture), func(b *testing.B) {
				world := ecs.NewWorld()
				owner := world.Spawn(ecs.NewBundle(0))
				spawner := world.NewSpawner1[htmlui.Focusable]()
				var last ecs.Entity
				for range count {
					last = spawner.Spawn(htmlui.Focusable{Owner: owner, Tag: "button", Type: "button"})
				}
				state := &uiEventState{}
				if capture {
					state.Focused = last
				}
				world.InsertResource(state)
				if capture {
					captureHTMLFocusRebuildState(world)
				} else {
					autofocusHTMLElement(world)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if capture {
						if state, ok := captureHTMLFocusRebuildState(world); !ok || state.Ordinal != count-1 {
							b.Fatal("lost focused element ordinal")
						}
					} else {
						autofocusHTMLElement(world)
					}
				}
			})
		}
	}
}
