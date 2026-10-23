package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/dom"
)

func TestHTMLKeyedSlotPreservesStaticChildrenAndReleasesReplacedNodes(t *testing.T) {
	world := ecs.NewWorld()
	spawner := ecs.NewSpawner1[Surface](world)
	page := spawner.Spawn(New(""))
	parent := dom.NewElement("ul", nil)
	before, after := dom.NewElement("header", nil), dom.NewElement("footer", nil)
	before.Parent, after.Parent = parent, parent
	parent.Children = []*dom.Node{before, after}
	list := &htmlComponentKeyedList{}
	reconcileHTMLComponentKeyedList(world, page, parent, list, []VNode{
		{Key: 1, Tag: "li", Children: []VNode{TextVNode(10, "span", "nested")}},
		TextVNode(2, "li", "two"),
	}, "scope")
	first, second := parent.Children[0], parent.Children[1]
	next := []VNode{TextVNode(2, "li", "two"), TextVNode(1, "p", "replaced"), TextVNode(1, "li", "duplicate"), {Key: 3}}
	reconcileHTMLComponentKeyedList(world, page, parent, list, next, "scope")
	if len(parent.Children) != 4 || parent.Children[0] != second || parent.Children[1].Data != "p" || parent.Children[2] != before || parent.Children[3] != after {
		t.Fatal("key matching or static child ordering changed")
	}
	if first.Parent != nil || list.Children[first] != nil {
		t.Fatal("replaced node or its nested state was retained")
	}
	if value, _ := parent.Children[1].Attr("class"); !htmlClassContains(value, "scope") {
		t.Fatal("replacement lost component scope")
	}
	reconcileHTMLComponentKeyedList(world, page, parent, list, next, "scope")
	if allocs := testing.AllocsPerRun(100, func() {
		reconcileHTMLComponentKeyedList(world, page, parent, list, next, "scope")
	}); allocs != 0 {
		t.Fatalf("stable reconciliation allocated %v times", allocs)
	}
}

func BenchmarkRefactorHTMLKeyedList(b *testing.B) {
	for _, nested := range []bool{false, true} {
		name := "Slot"
		if nested {
			name = "Children"
		}
		b.Run(name, func(b *testing.B) {
			world := ecs.NewWorld()
			owner := ecs.NewSpawner1[Surface](world)
			page := owner.Spawn(New(""))
			parent := dom.NewElement("ul", nil)
			keyed := &htmlComponentKeyedList{}
			children := &htmlComponentVNodeChildren{}
			states := make(map[*dom.Node]*htmlComponentVNodeChildren)
			var next [2][32]VNode
			for index := range next[0] {
				next[0][index] = TextVNode(uint64(index), "li", "item")
				next[1][31-index] = next[0][index]
			}
			sync := func(index int) {
				if nested {
					reconcileHTMLComponentVNodeChildren(world, page, parent, children, next[index][:], states, "")
				} else {
					reconcileHTMLComponentKeyedList(world, page, parent, keyed, next[index][:], "")
				}
			}
			sync(0)
			sync(1)
			b.ReportAllocs()
			b.ResetTimer()
			for index := range b.N {
				sync(index & 1)
			}
		})
	}
}

func TestHTMLKeyedBatchRemovalPreservesStaticChildren(t *testing.T) {
	world := ecs.NewWorld()
	owners := ecs.NewSpawner1[Surface](world)
	owner := owners.Spawn(New(""))
	parent := dom.NewElement("ul", nil)
	before, after := dom.NewElement("header", nil), dom.NewElement("footer", nil)
	parent.AppendChild(before)
	parent.AppendChild(after)
	list := &htmlComponentKeyedList{}
	next := make([]VNode, 128)
	for i := range next {
		next[i] = VNode{Key: uint64(i), Tag: "li", Children: []VNode{TextVNode(0, "span", "nested")}}
	}
	reconcileHTMLComponentKeyedList(world, owner, parent, list, next, "")
	old := append([]htmlComponentKeyedItem(nil), list.Items...)
	reconcileHTMLComponentKeyedList(world, owner, parent, list, next[len(next)-1:], "")
	if len(parent.Children) != 3 || parent.Children[0] != old[len(old)-1].Node || parent.Children[1] != before || parent.Children[2] != after {
		t.Fatal("batch removal changed retained or static child order")
	}
	for _, item := range old[:len(old)-1] {
		if item.Node.Parent != nil || list.Children[item.Node] != nil || list.Children[item.Node.Children[0]] != nil {
			t.Fatal("batch removal retained parent or descendant reconciliation state")
		}
	}
}
