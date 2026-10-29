package html

import (
	"testing"
)

func TestHTMLLocalFlexOrderPropagatesSizeChanges(t *testing.T) {
	source := func(order string) string {
		return `<div id="flex" style="display:flex;flex-wrap:wrap;width:100px"><div style="width:60px;height:100px">A</div><div style="width:60px;height:10px">B</div><div id="target" style="width:40px;height:100px;order:` + order + `">C</div></div><div><input value="Following"/></div>`
	}
	world, owner := incrementalHTMLFixture(t, source("0"))
	target := incrementalHTMLNode(t, world, owner, "target")
	for _, order := range []string{"-1", "0", "-1", "0"} {
		target.SetAttribute("style", "width:40px;height:100px;order:"+order)
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		if state.SyncStats.LayoutWork.LocalFlexReflows != 0 {
			t.Fatal("changed outer height was treated as isolated")
		}
		assertHTMLPartialMatchesFresh(t, world, owner, source(order))
	}
}

func TestHTMLLocalFlexOrderConservativeContexts(t *testing.T) {
	for _, parent := range []string{"position:relative;left:10px", "display:flex"} {
		t.Run(parent, func(t *testing.T) {
			source := func(order string) string {
				return `<div style="` + parent + `"><div style="display:flex;height:100px"><div id="target" style="order:` + order + `"><input value="Value"/></div><div>Sibling</div></div><div>Parent sibling</div></div>`
			}
			world, owner := incrementalHTMLFixture(t, source("0"))
			target := incrementalHTMLNode(t, world, owner, "target")
			target.SetAttribute("style", "order:1")
			incrementalSync(t, world)
			state, _ := world.Get[htmlRenderState](owner)
			if state.SyncStats.LayoutWork.LocalFlexReflows != 0 {
				t.Fatal("dependent formatting context isolated")
			}
			assertHTMLPartialMatchesFresh(t, world, owner, source("1"))
		})
	}
}

func TestHTMLLocalFlexOrderWithExternalDirtyNode(t *testing.T) {
	source := func(order, height string) string {
		return `<div style="display:flex;height:100px"><div id="target" style="order:` + order + `"><input value="Value"/></div><div>Sibling</div></div><div id="other" style="height:` + height + `">Other</div><div>Following</div>`
	}
	world, owner := incrementalHTMLFixture(t, source("0", "20px"))
	incrementalHTMLNode(t, world, owner, "target").SetAttribute("style", "order:1")
	incrementalHTMLNode(t, world, owner, "other").SetAttribute("style", "height:60px")
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	if state.SyncStats.LayoutWork.LocalFlexReflows != 0 {
		t.Fatal("external dirty node skipped")
	}
	assertHTMLPartialMatchesFresh(t, world, owner, source("1", "60px"))
}

func TestHTMLLocalFlexOrderUpdatesAncestorClipExtents(t *testing.T) {
	source := func(order string) string {
		return `<div id="outer" style="overflow:auto;height:40px;width:200px"><div id="flex" style="display:flex;height:40px;width:200px"><div id="target" style="position:relative;width:100px;order:` + order + `"><div style="position:absolute;left:120px;top:100px;width:150px">Absolute</div></div><div style="width:100px">Sibling</div></div></div><div>Following</div>`
	}
	world, owner := incrementalHTMLFixture(t, source("0"))
	target := incrementalHTMLNode(t, world, owner, "target")
	for _, order := range []string{"1", "0", "1", "0"} {
		target.SetAttribute("style", "position:relative;width:100px;order:"+order)
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		if state.SyncStats.LayoutWork.LocalFlexReflows != 1 {
			t.Fatal("stable outer flow was not isolated")
		}
		assertHTMLContextMatchesFresh(t, world, owner, source(order))
	}
}
