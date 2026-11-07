package html

import (
	"fmt"
	"strings"
	"testing"

	"github.com/onix-engine/onix/ecs"
)

func BenchmarkHTMLBindingSubtreeUpdate(b *testing.B) {
	for _, count := range []int{100, 10000} {
		for _, scrolling := range []bool{false, true} {
			b.Run(fmt.Sprintf("siblings=%d/scroll=%v", count, scrolling), func(b *testing.B) {
				world := ecs.NewWorld()
				value := New(`<style>div{width:10px;height:1px;background:red}</style><div id="target"></div>` + strings.Repeat(`<div></div>`, count))
				value.Width, value.Height = 240, float32(count+20)
				bundle := ecs.NewBundle(1)
				bundle.Put(value)
				owner := world.Spawn(bundle)
				incrementalSync(b, world)
				area := htmlBindingArea(world, -3, -7)
				SetNodeOffset(world, owner, "target", 3, 7)
				SetNodeScroll(world, owner, "target", area, 0, 0)
				if state := world.GetRef[htmlRenderState](owner); len(state.RenderNodes) < count+1 {
					b.Fatal("benchmark culled static siblings")
				}
				x := float32(3)
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					x = 7 - x
					if scrolling {
						SetNodeScroll(world, owner, "target", area, x, 0)
					} else {
						SetNodeOffset(world, owner, "target", x, 7)
					}
				}
			})
		}
	}
}
