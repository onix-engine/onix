package html

import "testing"

func TestHTMLRetainedInputContextOwnershipChanges(t *testing.T) {
	source := func(tabindex string) string {
		return `<section id="outer" tabindex="0"><div id="item" ` + tabindex + `>First</div><div id="sibling">Second</div></section>`
	}
	world, owner := incrementalHTMLFixture(t, source(""))
	outer := incrementalHTMLNode(t, world, owner, "outer")
	item := incrementalHTMLNode(t, world, owner, "item")
	sibling := incrementalHTMLNode(t, world, owner, "sibling")
	state := world.GetRef[htmlRenderState](owner)
	outerInput := state.BoxRenderings[outer.node].Input
	if outerInput == nil || state.BoxRenderings[item.node].Input != outerInput || state.BoxRenderings[sibling.node].Input != outerInput {
		t.Fatal("descendants failed to share their inherited focus context")
	}
	for range 2 {
		item.SetAttribute("tabindex", "0")
		incrementalSync(t, world)
		assertHTMLContextMatchesFresh(t, world, owner, source(`tabindex="0"`))
		if state.BoxRenderings[item.node].Input == outerInput || state.BoxRenderings[sibling.node].Input != outerInput {
			t.Fatal("new focus owner changed a sibling's inherited context")
		}
		item.RemoveAttribute("tabindex")
		incrementalSync(t, world)
		assertHTMLContextMatchesFresh(t, world, owner, source(""))
		if state.BoxRenderings[item.node].Input != outerInput || state.BoxRenderings[sibling.node].Input != outerInput {
			t.Fatal("removing the local focus owner lost the inherited context")
		}
	}
}

func TestHTMLRetainedEffectsAllocateOnlyWhenNeeded(t *testing.T) {
	source := func(effect string) string {
		return `<div id="item" style="width:30px;height:20px;background:red;` + effect + `"></div><div id="plain" style="width:30px;height:20px;background:blue"></div>`
	}
	world, owner := incrementalHTMLFixture(t, source(""))
	item := incrementalHTMLNode(t, world, owner, "item")
	plain := incrementalHTMLNode(t, world, owner, "plain")
	state := world.GetRef[htmlRenderState](owner)
	if state.BoxRenderings[item.node].Effects != nil || state.BoxRenderings[plain.node].Effects != nil {
		t.Fatal("ordinary paint retained optional effect geometry")
	}
	var storage *htmlBoxEffects
	for _, effect := range []string{"box-shadow:10px 2px 4px black", "", "filter:blur(2px)", "", "opacity:.5", ""} {
		item.SetAttribute("style", "width:30px;height:20px;background:red;"+effect)
		incrementalSync(t, world)
		assertHTMLContextMatchesFresh(t, world, owner, source(effect))
		surface := htmlCullRole(world, owner, item, htmlRoleSurface)
		// A single background applies opacity directly without compositing.
		composite := effect == "filter:blur(2px)"
		if surface == 0 || world.Has[htmlOpacitySurface](surface) != composite || world.Has[htmlElementSurface](surface) == composite {
			t.Fatal("reused surface retained the wrong ordinary/composite marker")
		}
		rendered := state.BoxRenderings[item.node]
		if storage == nil {
			storage = rendered.Effects
		}
		if storage == nil || rendered.Effects != storage {
			t.Fatal("effect changes failed to retain reusable geometry storage")
		}
		if state.BoxRenderings[plain.node].Effects != nil {
			t.Fatal("effect mutation allocated geometry for an unrelated box")
		}
	}
}
