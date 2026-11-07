package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/dom"
)

func BenchmarkHTMLComponentDiff(b *testing.B) {
	world := ecs.NewWorld()
	spawn := world.NewSpawner1[htmlRenderState]()
	owner := spawn.Spawn(htmlRenderState{})
	parent := dom.NewElement("ul", nil)
	old := make([]htmlComponentKeyedItem, 1024)
	spare := make([]htmlComponentKeyedItem, 0, len(old))
	next := make([]VNode, len(old))
	for index := range old {
		node := dom.NewElement("li", nil)
		node.AppendChild(dom.NewText("item"))
		parent.AppendChild(node)
		old[index] = htmlComponentKeyedItem{Node: node, Key: uint64(index)}
		next[len(old)-1-index] = TextVNode(uint64(index), "li", "item")
	}
	for _, warm := range []bool{false, true} {
		name := "populate"
		if warm {
			name = "warm"
		}
		b.Run(name, func(b *testing.B) {
			list := htmlComponentVNodeChildren{Items: old, Scratch: spare}
			if warm {
				reconcileHTMLComponentVNodeList(world, owner, parent, &list, next, nil, "")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if !warm {
					list = htmlComponentVNodeChildren{Items: old, Scratch: spare}
				}
				reconcileHTMLComponentVNodeList(world, owner, parent, &list, next, nil, "")
			}
		})
	}
}
