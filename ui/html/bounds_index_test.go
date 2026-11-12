package html

import (
	"fmt"
	"strings"
	"testing"

	"github.com/onix-engine/onix/ecs"
)

func TestHTMLSameGeometryTopologyUsesStructureTracking(t *testing.T) {
	const first = `<div id="first" style="position:absolute;width:20px;height:20px;background:red"><div id="leaf" style="width:10px;height:10px;background:green"></div></div>`
	const second = `<div id="second" style="position:absolute;width:20px;height:20px;background:blue"></div>`
	const prefix = `<div id="left" style="position:absolute;width:80px;height:80px;overflow:auto">`
	const middle = `</div><div id="right" style="position:absolute;width:80px;height:80px;overflow:auto"><div id="slot">`
	const suffix = `</div></div>`
	world, owner := incrementalHTMLFixture(t, prefix+first+second+middle+suffix)
	defer Dispose(world)
	left := incrementalHTMLNode(t, world, owner, "left")
	firstNode := incrementalHTMLNode(t, world, owner, "first")
	leaf := incrementalHTMLNode(t, world, owner, "leaf")
	slot := incrementalHTMLNode(t, world, owner, "slot")
	state := world.GetRef[htmlRenderState](owner)
	firstBox, leafBox := state.boxForNode(firstNode.node), state.boxForNode(leaf.node)
	firstID, leafID := firstBox.StableID, leafBox.StableID
	firstHash, leafHash := firstBox.OwnGeometryHash(), leafBox.OwnGeometryHash()
	for _, step := range []struct {
		destination Node
		source      string
	}{
		{left, prefix + second + first + middle + suffix},
		{slot, prefix + second + middle + first + suffix},
		{left, prefix + second + first + middle + suffix},
	} {
		firstNode.Remove()
		step.destination.Append(firstNode)
		incrementalSync(t, world)
		state = world.GetRef[htmlRenderState](owner)
		if firstBox.StableID != firstID || leafBox.StableID != leafID || firstBox.OwnGeometryHash() != firstHash || leafBox.OwnGeometryHash() != leafHash {
			t.Fatal("same-geometry structural edit changed identity or geometry hash")
		}
		if state.SyncStats.PartialReconciles != 1 {
			t.Fatal("same-geometry structural edit fell back to a full render")
		}
		for _, rendered := range state.BoxRenderings {
			if rendered.Depth != htmlBoxDepth(rendered.Box)-1 || rendered.Parent != rendered.Box.Parent {
				t.Fatal("structural edit left stale ancestry")
			}
		}
		assertHTMLContextMatchesFresh(t, world, owner, step.source)
	}
}

func TestHTMLNodeBoundsIndexMembership(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="wrapper"><div id="child" style="height:20px"></div></div><div id="destination" style="height:40px"></div>`)
	defer Dispose(world)
	wrapper := incrementalHTMLNode(t, world, owner, "wrapper")
	child := incrementalHTMLNode(t, world, owner, "child")
	for _, display := range []string{"contents", "none", "block"} {
		wrapper.SetAttribute("style", "display:"+display)
		incrementalSync(t, world)
		if _, ok := wrapper.Bounds(); ok != (display == "block") {
			t.Fatalf("wrapper bounds disagree with display:%s", display)
		}
		if bounds, ok := child.Bounds(); ok != (display != "none") || ok && bounds.Height != 20 {
			t.Fatalf("child bounds disagree with display:%s: %+v, %v", display, bounds, ok)
		}
	}
	before, _ := child.Bounds()
	child.SetAttribute("style", "height:30px")
	if bounds, _ := child.Bounds(); bounds != before {
		t.Fatal("bounds changed before synchronization")
	}
	child.Remove()
	incrementalHTMLNode(t, world, owner, "destination").Append(child)
	incrementalSync(t, world)
	if bounds, ok := child.Bounds(); !ok || bounds.Height != 30 || bounds.Y != 0 {
		t.Fatalf("reparented bounds are stale: %+v, %v", bounds, ok)
	}
	world.GetMut[Surface](owner).Value = `<div id="replacement" style="height:15px"></div>`
	incrementalSync(t, world)
	if _, ok := child.Bounds(); ok {
		t.Fatal("replaced document still exposes old bounds")
	}
	replacement := incrementalHTMLNode(t, world, owner, "replacement")
	if bounds, ok := replacement.Bounds(); !ok || bounds.Height != 15 {
		t.Fatalf("replacement bounds missing: %+v, %v", bounds, ok)
	}
	if _, ok := (Node{}).Bounds(); ok {
		t.Fatal("invalid node exposes bounds")
	}
	if _, ok := (Node{world: ecs.NewWorld(), owner: owner, node: replacement.node}).Bounds(); ok {
		t.Fatal("missing owner exposes bounds")
	}
}

func BenchmarkHTMLNodeBounds(b *testing.B) {
	for _, count := range []int{128, 1024, 8192} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<style>div{height:1px}</style>`+strings.Repeat(`<div></div>`, count-1)+`<div id="target"></div>`)
			defer Dispose(world)
			node := incrementalHTMLNode(b, world, owner, "target")
			b.ReportAllocs()
			for b.Loop() {
				if bounds, ok := node.Bounds(); !ok || bounds.Height != 1 {
					b.Fatalf("missing bounds: %+v, %v", bounds, ok)
				}
			}
		})
	}
}
