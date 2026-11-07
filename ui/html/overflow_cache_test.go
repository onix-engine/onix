package html

import (
	"testing"

	"github.com/onix-engine/onix/scene"
)

func TestHTMLOverflowRecordsShareUpdatesAndRelease(t *testing.T) {
	source := func(css string) string {
		return `<div id="outer" style="overflow:auto;width:180px;height:90px"><div id="scroller" style="` + css + `"><a id="link" href="#link">Link</a><div style="width:150px;height:120px">Content</div></div></div>`
	}
	world, owner := incrementalHTMLFixture(t, source("overflow:scroll;width:80px;height:40px"))
	scroller := incrementalHTMLNode(t, world, owner, "scroller")
	link := incrementalHTMLNode(t, world, owner, "link")
	state := world.GetRef[htmlRenderState](owner)
	binding := state.OverflowBindings[scroller.node]
	if binding == nil || len(binding.parts) != 4 {
		t.Fatal("missing scroll record or scrollbar parts")
	}
	parts := &binding.parts[0]
	check := func() {
		t.Helper()
		state = world.GetRef[htmlRenderState](owner)
		if state.BoxRenderings[scroller.node].Overflow != binding || state.OverflowBindings[scroller.node] != binding || state.OverflowEntities[binding.entity] != binding || state.OverflowRoutes[link.node] != binding {
			t.Fatal("overflow indices do not share the owned record")
		}
		if &binding.parts[0] != parts {
			t.Fatal("scrollbar update replaced its warm storage")
		}
		for _, target := range state.FocusTargets {
			if target.Node == link.node {
				parent := world.GetRef[scene.Parent](target.Entity)
				if parent == nil || parent.Entity != binding.entity {
					t.Fatal("inline link focus target escaped overflow clipping")
				}
				return
			}
		}
		t.Fatal("missing link focus target")
	}
	check()
	for _, css := range []string{"overflow:scroll;width:90px;height:45px;margin-left:12px", "overflow:scroll;width:70px;height:35px;margin-left:4px"} {
		scroller.SetAttribute("style", css)
		incrementalSync(t, world)
		check()
		if binding.rect != state.BoxRenderings[scroller.node].Box.PaddingBox() {
			t.Fatal("shared overflow record retained an old rectangle")
		}
		assertHTMLContextMatchesFresh(t, world, owner, source(css))
	}
	clip := binding.entity
	scroller.SetAttribute("style", "overflow:visible;width:80px;height:40px")
	incrementalSync(t, world)
	state = world.GetRef[htmlRenderState](owner)
	outer := incrementalHTMLNode(t, world, owner, "outer")
	if state.BoxRenderings[scroller.node].Overflow != nil || state.OverflowBindings[scroller.node] != nil || state.OverflowEntities[clip] != nil || world.IsAlive(clip) || state.OverflowRoutes[link.node] != state.OverflowBindings[outer.node] {
		t.Fatal("removed overflow record or route survived reconciliation")
	}
	assertHTMLContextMatchesFresh(t, world, owner, source("overflow:visible;width:80px;height:40px"))
}
