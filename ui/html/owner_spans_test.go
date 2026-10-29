package html

import (
	"slices"
	"strings"
	"testing"

	"github.com/diiyw/onix/html/layout"
)

func assertHTMLOwnerSpans(t *testing.T, state htmlRenderState) {
	t.Helper()
	if !state.OwnerSpansValid {
		t.Fatal("owner spans invalid")
	}
	cursor := 0
	var visit func(*layout.Box)
	visit = func(box *layout.Box) {
		rendered := state.BoxRenderings[box.Node]
		if rendered.OwnerStart != cursor {
			t.Fatalf("wrong span start at %s: got %d want %d", box.StableID, rendered.OwnerStart, cursor)
		}
		for _, entity := range rendered.Entities {
			if cursor >= len(state.Children) || state.Children[cursor] != entity {
				t.Fatal("owner sequence differs from box sequence")
			}
			cursor++
		}
		if rendered.Clip != 0 {
			if cursor >= len(state.Children) || state.Children[cursor] != rendered.Clip {
				t.Fatal("missing clip in owner span")
			}
			cursor++
		}
		for _, child := range box.Children {
			visit(child)
		}
		if rendered.OwnerEnd != cursor {
			t.Fatal("wrong span end")
		}
	}
	visit(state.Boxes)
	if cursor != len(state.Children) {
		t.Fatal("owner list contains extra entities")
	}
}

func htmlOwnerSubtreeBoxes(box *layout.Box) int {
	count := 1
	for _, child := range box.Children {
		count += htmlOwnerSubtreeBoxes(child)
	}
	return count
}

func TestHTMLOwnerReorderOnlyWritesAffectedSpan(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="before" style="height:30px;overflow:auto"><input value="Before"/></div><div id="flex" style="height:80px;display:flex;overflow:auto"><div id="target" style="height:40px;overflow:auto;order:0"><input value="Value"/></div><div style="height:40px;overflow:hidden"><input value="Second"/></div></div>`+strings.Repeat(`<div style="height:40px;overflow:auto"><input value="Unchanged"/></div>`, 1000))
	node := incrementalHTMLNode(t, world, owner, "target")
	flex := incrementalHTMLNode(t, world, owner, "flex")
	state, _ := world.Get[htmlRenderState](owner)
	assertHTMLOwnerSpans(t, state)
	rendered := state.BoxRenderings[flex.node]
	start, end := rendered.OwnerStart, rendered.OwnerEnd
	prefix, suffix := slices.Clone(state.Children[:start]), slices.Clone(state.Children[end:])
	expectedWork := htmlOwnerSubtreeBoxes(rendered.Box)
	for _, order := range []string{"1", "0", "1", "0"} {
		node.SetAttribute("style", "height:40px;overflow:auto;order:"+order)
		incrementalSync(t, world)
		state, _ = world.Get[htmlRenderState](owner)
		if state.SyncStats.PartialReconciles != 1 || state.SyncStats.OwnerListBoxes != expectedWork {
			t.Fatalf("nonlocal owner update: %+v", state.SyncStats)
		}
		if !slices.Equal(prefix, state.Children[:start]) || !slices.Equal(suffix, state.Children[end:]) {
			t.Fatal("outside owner span changed")
		}
		assertHTMLOwnerSpans(t, state)
	}
}

func TestHTMLOwnerNestedAndDisjointOrderSpans(t *testing.T) {
	source := func(order string) string {
		return `<div id="outer" style="display:flex;height:160px"><div id="target" style="display:flex;order:` + order + `"><div id="inner" style="order:` + order + `"><input value="Value"/></div><div>Inner sibling</div></div><div>Outer sibling</div></div><div id="separate" style="display:flex;height:80px"><div id="other" style="order:` + order + `">Other</div><div>Other sibling</div></div>`
	}
	world, owner := incrementalHTMLFixture(t, source("0"))
	target := incrementalHTMLNode(t, world, owner, "target")
	inner := incrementalHTMLNode(t, world, owner, "inner")
	other := incrementalHTMLNode(t, world, owner, "other")
	outer := incrementalHTMLNode(t, world, owner, "outer")
	separate := incrementalHTMLNode(t, world, owner, "separate")
	for _, order := range []string{"1", "0", "1", "0"} {
		target.SetAttribute("style", "display:flex;order:"+order)
		inner.SetAttribute("style", "order:"+order)
		other.SetAttribute("style", "order:"+order)
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		expectedWork := htmlOwnerSubtreeBoxes(state.BoxRenderings[outer.node].Box) + htmlOwnerSubtreeBoxes(state.BoxRenderings[separate.node].Box)
		if state.SyncStats.OwnerListBoxes != expectedWork {
			t.Fatalf("nested spans rewritten twice: %+v", state.SyncStats)
		}
		assertHTMLOwnerSpans(t, state)
		assertHTMLPartialMatchesFresh(t, world, owner, source(order))
	}
}

func TestHTMLOwnerSpansRecoverAfterMembershipChanges(t *testing.T) {
	source := func(order, css string) string {
		return `<div id="flex" style="display:flex;height:80px"><div id="target" style="height:40px;order:` + order + `;` + css + `"><div>Text</div></div><div>Sibling</div></div><div>Unchanged</div>`
	}
	world, owner := incrementalHTMLFixture(t, source("0", ""))
	node := incrementalHTMLNode(t, world, owner, "target")
	for _, step := range []struct{ order, css string }{{"1", "overflow:auto;background:red"}, {"0", "overflow:auto;background:red"}, {"1", ""}, {"0", ""}} {
		node.SetAttribute("style", "height:40px;order:"+step.order+";"+step.css)
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		assertHTMLOwnerSpans(t, state)
		assertHTMLContextMatchesFresh(t, world, owner, source(step.order, step.css))
	}
	// Tree changes rebuild offsets before a later local reorder.
	incrementalHTMLNode(t, world, owner, "flex").SetAttribute("style", "display:flex;height:80px;position:relative")
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	assertHTMLOwnerSpans(t, state)
	node.SetAttribute("style", "height:40px;order:1")
	incrementalSync(t, world)
	state, _ = world.Get[htmlRenderState](owner)
	assertHTMLOwnerSpans(t, state)
}

func TestHTMLOwnerSpansAfterStructuralShift(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="before">Before</div><div id="flex" style="display:flex;height:80px"><div id="target">Target</div><div>Sibling</div></div><div>After</div>`)
	incrementalHTMLNode(t, world, owner, "before").Remove()
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	assertHTMLOwnerSpans(t, state)
	target := incrementalHTMLNode(t, world, owner, "target")
	flex := incrementalHTMLNode(t, world, owner, "flex")
	target.SetAttribute("style", "order:1")
	incrementalSync(t, world)
	state, _ = world.Get[htmlRenderState](owner)
	if state.SyncStats.OwnerListBoxes != htmlOwnerSubtreeBoxes(state.BoxRenderings[flex.node].Box) {
		t.Fatal("structural shift prevented local span update")
	}
	assertHTMLOwnerSpans(t, state)
	assertHTMLPartialMatchesFresh(t, world, owner, `<div id="flex" style="display:flex;height:80px"><div id="target" style="order:1">Target</div><div>Sibling</div></div><div>After</div>`)
}

func TestHTMLClippedOwnerReorderReusesListStorage(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>#target.changed{order:1}</style><div style="display:flex;height:80px;overflow:auto"><div id="target" style="height:40px;overflow:auto"><input value="Value"/></div><div><input value="Second"/></div></div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	cycle := func() {
		target.SetAttribute("class", "changed")
		incrementalSync(t, world)
		target.SetAttribute("class", "")
		incrementalSync(t, world)
	}
	for range 4 {
		cycle()
	}
	before, _ := world.Get[htmlRenderState](owner)
	storage := &before.Children[0]
	capacity := cap(before.Children)
	for range 100 {
		cycle()
	}
	state, _ := world.Get[htmlRenderState](owner)
	if &state.Children[0] != storage || cap(state.Children) != capacity {
		t.Fatal("owner list storage replaced")
	}
	assertHTMLOwnerSpans(t, state)
}
