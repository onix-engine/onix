package html

import (
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/html/dom"
	"github.com/onix-engine/onix/internal/reuse"
)

func TestHTMLComponentCompactDiffHandlesDuplicatesAndReplacements(t *testing.T) {
	world := ecs.NewWorld()
	spawn := world.NewSpawner1[htmlRenderState]()
	owner := spawn.Spawn(htmlRenderState{})
	parent := dom.NewElement("ul", nil)
	children := make(map[*dom.Node]*htmlComponentVNodeChildren)
	var list htmlComponentVNodeChildren
	commit := func(next []VNode) {
		reconcileHTMLComponentVNodeChildren(world, owner, parent, &list, next, children, "")
	}
	commit([]VNode{TextVNode(0, "li", "zero"), TextVNode(^uint64(0), "li", "max"), TextVNode(7, "li", "remove")})
	zero, max, removed := parent.Children[0], parent.Children[1], parent.Children[2]
	commit([]VNode{TextVNode(^uint64(0), "li", "max updated"), TextVNode(0, "span", "replace"), TextVNode(0, "li", "duplicate"), TextVNode(8, "", "invalid")})
	if len(parent.Children) != 2 || parent.Children[0] != max || parent.Children[1] == zero || parent.Children[1].Data != "span" {
		t.Fatal("compact index confused zero/max keys, duplicates or changed tags")
	}
	if zero.Parent != nil || removed.Parent != nil || htmlNodeText(parent.Children[1]) != "replace" {
		t.Fatal("removed/replaced nodes survived or a duplicate overwrote the first value")
	}
	if n := testing.AllocsPerRun(100, func() {
		commit([]VNode{TextVNode(^uint64(0), "li", "max updated"), TextVNode(0, "span", "replace")})
	}); n != 0 {
		t.Fatalf("compact keyed diff allocated %g on a warm commit", n)
	}
}

func TestHTMLCompactDiffAndAncestorsReclaimPeaks(t *testing.T) {
	var index htmlComponentDiffIndex
	index.reset()
	for key := range uint64(4096) {
		index.values[key] = (key + 1) << 2
	}
	oldIndex := index.values
	index.reset()
	for range reuse.RetentionFrames + 1 {
		index.values[0], index.values[^uint64(0)] = 1, 2
		index.reset()
	}
	oldIndex[17] = 1
	if _, ok := index.values[17]; ok {
		t.Fatal("component index retained its historical map")
	}
	var ancestors htmlAncestorCache[int, int]
	parent := func(node int) int { return node - 1 }
	inherit := func(node, _ int) int { return node }
	if ancestors.resolve(4096, parent, inherit) != 4096 {
		t.Fatal("ancestor fixture did not resolve")
	}
	oldAncestors := ancestors.values
	peak := cap(ancestors.path)
	for range reuse.RetentionFrames + 2 {
		ancestors.clear()
		if ancestors.resolve(2, parent, inherit) != 2 {
			t.Fatal("shrinking ancestor cache lost its result")
		}
	}
	oldAncestors[17] = 17
	if _, ok := ancestors.values[17]; ok || cap(ancestors.path) >= peak {
		t.Fatal("ancestor cache retained peak map or path storage")
	}
	if n := testing.AllocsPerRun(100, func() {
		ancestors.clear()
		ancestors.resolve(2, parent, inherit)
	}); n != 0 {
		t.Fatalf("reclaimed ancestor cache allocated %g", n)
	}
}
