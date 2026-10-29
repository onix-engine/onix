package html

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func incrementalControl(t testing.TB, world *ecs.World, owner ecs.Entity, id string) ecs.Entity {
	t.Helper()
	node := incrementalHTMLNode(t, world, owner, id)
	state, _ := world.Get[htmlRenderState](owner)
	for key, group := range state.EntityGroups {
		if key.Node == node.node && key.Role == htmlRoleControl {
			return group.Bindings[0].Entity
		}
	}
	t.Fatalf("missing control %s", id)
	return 0
}

func TestHTMLControlLocalGeometryMatchesFresh(t *testing.T) {
	cases := []string{
		`<input id="target" style="%s" value="Value"/>`,
		`<textarea id="target" style="%s">Some text</textarea>`,
		`<input id="target" type="checkbox" checked style="%s"/>`,
		`<input id="target" type="radio" checked style="%s"/>`,
		`<select id="target" style="%s"><option>A</option><option selected>B</option></select>`,
		`<button id="target" style="%s">Button</button>`,
		`<button id="target" style="display:block;%s"><div>Nested button text</div></button>`,
		`<div id="target" tabindex="0" style="%s"><div>Focusable child</div></div>`,
		`<a id="target" href="#target" style="%s">Wrapping link text</a>`,
		`<div id="target" contenteditable="true" style="%s"><div>Editable child</div></div>`,
	}
	for i, template := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			source := func(css string) string {
				return `<div id="scroller" style="overflow:auto;height:60px">` + fmt.Sprintf(template, css) + `</div><div>Unchanged</div>`
			}
			world, owner := incrementalHTMLFixture(t, source("width:120px"))
			target := incrementalHTMLNode(t, world, owner, "target")
			for _, css := range []string{"width:70px", "width:150px;background:red", "width:100px"} {
				// Keep the block button's display fixed; changing topology uses the full path.
				if strings.Contains(template, "display:block;") {
					css = "display:block;" + css
				}
				target.SetAttribute("style", css)
				incrementalSync(t, world)
				state, _ := world.Get[htmlRenderState](owner)
				if state.SyncStats.PartialReconciles != 1 {
					t.Fatalf("stable control/focus context fell back: %+v", state.SyncStats)
				}
				assertHTMLContextMatchesFresh(t, world, owner, source(css))
			}
		})
	}
}

func TestHTMLControlLocalPreservesLiveStateAndSkipsSiblings(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>.wide{width:180px}</style><div><input id="target" value="initial"/></div><div><input id="other" value="other"/></div>`+strings.Repeat(`<div><input value="Unchanged"/></div>`, 1000))
	target := incrementalHTMLNode(t, world, owner, "target")
	entity := incrementalControl(t, world, owner, "target")
	other := incrementalControl(t, world, owner, "other")
	SetControlValue(world, entity, "typed")
	SetControlFocus(world, entity, true)
	incrementalSync(t, world) // Establish focus before testing an ordinary DOM update.
	live := world.GetMut[Control](entity)
	live.SelectionAnchor, live.SelectionFocus = 1, 3
	input := world.GetMut[scene.InputState2D](entity)
	input.Pressed, input.Hovered = true, true
	unchanged, _ := world.Get[Control](other)
	target.SetAttribute("class", "wide")
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	live = world.GetMut[Control](entity)
	input = world.GetMut[scene.InputState2D](entity)
	after, _ := world.Get[Control](other)
	if state.SyncStats.PartialReconciles != 1 || state.SyncStats.UpdatedControls != 1 || state.SyncStats.SnapshottedControls != 1 {
		t.Fatalf("unrelated controls visited: %+v", state.SyncStats)
	}
	if live.Value != "typed" || !live.DirtyValue || live.SelectionAnchor != 1 || live.SelectionFocus != 3 || !input.Pressed || !input.Hovered || state.Input.Focused != target.node || after.RenderRevision != unchanged.RenderRevision {
		t.Fatal("live control/focus state changed")
	}
}

func TestHTMLFocusLocalBoundsShrinkAndInlineTargetCleanup(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="outer" tabindex="0"><div id="target" style="width:180px">Child</div></div><div><a id="link" href="#">Long link text</a><a id="other" href="#">Other</a></div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	outer := incrementalHTMLNode(t, world, owner, "outer")
	link := incrementalHTMLNode(t, world, owner, "link")
	for _, width := range []int{30, 200, 50} {
		target.SetAttribute("style", fmt.Sprintf("width:%dpx", width))
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		if state.SyncStats.PartialReconciles != 1 {
			t.Fatal("focus subtree fell back")
		}
		binding := state.FocusTargets[state.FocusByNode[outer.node]]
		if binding.Rect != state.BoxesByNode[outer.node].BorderBox() {
			t.Fatal("focus bounds retained previous geometry")
		}
		assertHTMLPartialMatchesFresh(t, world, owner, fmt.Sprintf(`<div id="outer" tabindex="0"><div id="target" style="width:%dpx">Child</div></div><div><a id="link" href="#">Long link text</a><a id="other" href="#">Other</a></div>`, width))
	}
	link.SetText(" ")
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	if _, exists := state.FocusByNode[link.node]; exists {
		t.Fatal("empty inline focus target remains indexed")
	}
	for node, index := range state.FocusByNode {
		if state.FocusTargets[index].Node != node {
			t.Fatal("focus index corrupt after removal")
		}
	}
	link.SetText("Restored")
	incrementalSync(t, world)
	assertHTMLPartialMatchesFresh(t, world, owner, `<div id="outer" tabindex="0"><div id="target" style="width:50px">Child</div></div><div><a id="link" href="#">Restored</a><a id="other" href="#">Other</a></div>`)
}

func BenchmarkHTMLControlLocalResize(b *testing.B) {
	for _, count := range []int{10, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<style>.wide{width:180px}</style><div><input id="target" value="Value"/></div>`+strings.Repeat(`<div><input value="Unchanged"/></div>`, count))
			target := incrementalHTMLNode(b, world, owner, "target")
			for _, class := range []string{"wide", ""} {
				target.SetAttribute("class", class)
				incrementalSync(b, world)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if i%2 == 0 {
					target.SetAttribute("class", "wide")
				} else {
					target.SetAttribute("class", "")
				}
				if err := Sync(world); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestHTMLFocusTargetCanMoveBetweenEmittingBoxes(t *testing.T) {
	source := func(height int) string {
		return fmt.Sprintf(`<div id="scroller" style="height:80px;overflow:auto"><div id="target" tabindex="0" style="height:%dpx"><div>Child</div></div></div><div>After</div>`, height)
	}
	world, owner := incrementalHTMLFixture(t, source(0))
	target := incrementalHTMLNode(t, world, owner, "target")
	for _, height := range []int{30, 0, 40, 0, 20} {
		target.SetAttribute("style", fmt.Sprintf("height:%dpx", height))
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		if state.SyncStats.PartialReconciles != 1 {
			t.Fatalf("focus ownership transfer fell back: %+v", state.SyncStats)
		}
		assertHTMLContextMatchesFresh(t, world, owner, source(height))
		for _, record := range state.BoxRenderings {
			for _, key := range record.Groups {
				if group := state.EntityGroups[key]; group != nil && group.Box != record {
					t.Fatal("group retained by former emitting box")
				}
			}
		}
	}
}

func TestHTMLEditingHostLocalSelectionAndDescendantText(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="host" contenteditable="true"><div id="target">Before</div></div><div>After</div>`)
	host := incrementalHTMLNode(t, world, owner, "host")
	target := incrementalHTMLNode(t, world, owner, "target")
	state, _ := world.Get[htmlRenderState](owner)
	entity := state.FocusTargets[state.FocusByNode[host.node]].Entity
	editable := world.GetMut[Editable](entity)
	editable.Anchor, editable.Focus = 1, 3
	host.SetAttribute("style", "width:120px")
	incrementalSync(t, world)
	editable = world.GetMut[Editable](entity)
	if editable.Anchor != 1 || editable.Focus != 3 {
		t.Fatal("geometry update changed editable selection")
	}
	target.SetText("Changed")
	incrementalSync(t, world)
	state, _ = world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 {
		t.Fatal("descendant text did not update editing context locally")
	}
	assertHTMLPartialMatchesFresh(t, world, owner, `<div id="host" contenteditable="true" style="width:120px"><div id="target">Changed</div></div><div>After</div>`)
}

func TestHTMLWarmControlResizeDoesNotAllocate(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>.wide{width:180px}</style><div><input id="target" value="Value"/></div>`+strings.Repeat(`<div><input value="Unchanged"/></div>`, 10))
	target := incrementalHTMLNode(t, world, owner, "target")
	update := func() {
		target.SetAttribute("class", "wide")
		incrementalSync(t, world)
		target.SetAttribute("class", "")
		incrementalSync(t, world)
	}
	for range 10 {
		update()
	}
	// One interned instance per newly distinct style: both toggle directions
	// restyle the input, unchanged nodes stay allocation-free.
	if n := testing.AllocsPerRun(100, update); n > 2 {
		t.Fatalf("warm input resize allocated %v times", n)
	}
}

func TestHTMLSelectLocalResizePreservesSelectionAndOpenState(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div><select id="target"><option>A</option><option>B</option></select></div><div>After</div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	entity := incrementalControl(t, world, owner, "target")
	SetControlSelected(world, entity, 1)
	incrementalSync(t, world)
	control := world.GetMut[Control](entity)
	control.Open = true
	control.SelectionAnchor, control.SelectionFocus = 1, 1
	target.SetAttribute("style", "width:180px")
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	control = world.GetMut[Control](entity)
	if state.SyncStats.PartialReconciles != 1 || !control.Open || control.Selected != 1 || control.SelectionAnchor != 1 || control.SelectionFocus != 1 || !control.Options[1].Selected {
		t.Fatal("local select resize reset live state")
	}
}

func TestHTMLTextareaLocalTextPreservesDirtyValue(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div><textarea id="target">Initial</textarea></div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	entity := incrementalControl(t, world, owner, "target")
	SetControlValue(world, entity, "Typed")
	incrementalSync(t, world)
	target.SetText("New default")
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	control, _ := world.Get[Control](entity)
	if state.SyncStats.PartialReconciles != 1 || control.Value != "Typed" || control.DefaultValue != "New default" || !control.DirtyValue {
		t.Fatalf("local textarea default update lost dirty value: %+v", control)
	}
}
