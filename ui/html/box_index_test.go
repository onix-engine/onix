package html

import (
	"fmt"
	"strings"
	"testing"

	"github.com/onix-engine/onix/html/dom"
	"github.com/onix-engine/onix/html/layout"
)

func assertHTMLBoxIndex(t *testing.T, state *htmlRenderState) {
	t.Helper()
	count := 0
	var visit func(*layout.Box)
	visit = func(box *layout.Box) {
		count++
		if rendered := state.BoxRenderings[box.Node]; rendered == nil || rendered.Box != box || state.boxForNode(box.Node) != box {
			t.Fatal("rendered box index disagrees with the committed layout tree")
		}
		for _, child := range box.Children {
			visit(child)
		}
	}
	visit(state.Boxes)
	if len(state.BoxRenderings) != count {
		t.Fatalf("retired boxes remain indexed: got %d, want %d", len(state.BoxRenderings), count)
	}
}

func TestHTMLBoxIndexFullAndLocalMembership(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>.changed{width:40px}</style><div id="target" style="height:20px;background:red"></div><div id="other" style="height:20px;background:blue"></div>`)
	defer Dispose(world)
	target := incrementalHTMLNode(t, world, owner, "target")
	other := incrementalHTMLNode(t, world, owner, "other")
	state := world.GetRef[htmlRenderState](owner)
	retained := state.BoxRenderings[other.node]
	generation := retained.Generation
	target.SetAttribute("class", "changed")
	incrementalSync(t, world)
	state = world.GetRef[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 || retained.Generation != generation || state.boxForNode(other.node) != retained.Box {
		t.Fatal("local emission invalidated an untouched sibling")
	}
	assertHTMLBoxIndex(t, state)
	world.GetMut[Surface](owner).Width++
	incrementalSync(t, world)
	state = world.GetRef[htmlRenderState](owner)
	assertHTMLBoxIndex(t, state)
	for _, rendered := range state.BoxRenderings {
		if rendered.Generation != state.RenderGeneration {
			t.Fatal("full emission did not renew every box")
		}
	}
	// During full emission, retain old records for reuse while hiding them from
	// current membership. A local pass must still expose untouched ancestors.
	state.RenderGeneration++
	state.Reconciling = true
	if state.boxForNode(other.node) != nil {
		t.Fatal("full emission exposed a box that has not been emitted")
	}
	state.LocalReconcile = true
	if state.boxForNode(other.node) != retained.Box {
		t.Fatal("local emission hid an untouched box")
	}
}

func TestHTMLBoxIndexTopologyAndDocumentReplacement(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="wrapper"><div id="child" style="height:20px;background:red"></div></div><div id="destination"></div>`)
	defer Dispose(world)
	wrapper := incrementalHTMLNode(t, world, owner, "wrapper")
	child := incrementalHTMLNode(t, world, owner, "child")
	destination := incrementalHTMLNode(t, world, owner, "destination")
	for _, value := range []string{"display:contents", "display:none", "display:block;visibility:hidden", "display:block"} {
		wrapper.SetAttribute("style", value)
		incrementalSync(t, world)
		state := world.GetRef[htmlRenderState](owner)
		assertHTMLBoxIndex(t, state)
		if value == "display:contents" || value == "display:none" {
			if state.boxForNode(wrapper.node) != nil {
				t.Fatal("a boxless element retained its box")
			}
		}
		if (state.boxForNode(child.node) == nil) != (value == "display:none") {
			t.Fatal("descendant box membership disagrees with display")
		}
	}
	child.Remove()
	destination.Append(child)
	wrapper.Remove()
	incrementalSync(t, world)
	state := world.GetRef[htmlRenderState](owner)
	assertHTMLBoxIndex(t, state)
	if state.boxForNode(wrapper.node) != nil || state.boxForNode(child.node).Parent != state.boxForNode(destination.node) {
		t.Fatal("reparenting retained the old box owner")
	}
	oldNodes := make([]*dom.Node, 0, len(state.BoxRenderings))
	for node := range state.BoxRenderings {
		oldNodes = append(oldNodes, node)
	}
	world.GetMut[Surface](owner).Value = `<div id="replacement" style="height:30px;background:blue"></div>`
	incrementalSync(t, world)
	state = world.GetRef[htmlRenderState](owner)
	assertHTMLBoxIndex(t, state)
	for _, node := range oldNodes {
		if state.boxForNode(node) != nil {
			t.Fatal("document replacement retained an old DOM node")
		}
	}
}

func BenchmarkHTMLBoxIndexFullSync(b *testing.B) {
	for _, count := range []int{128, 1024} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<style>div{height:1px;background:red}</style>`+strings.Repeat(`<div></div>`, count))
			defer Dispose(world)
			update := func() {
				surface := world.GetMut[Surface](owner)
				if surface.Width == 320 {
					surface.Width = 321
				} else {
					surface.Width = 320
				}
				incrementalSync(b, world)
			}
			update()
			update()
			b.ReportAllocs()
			for b.Loop() {
				update()
			}
		})
	}
}
