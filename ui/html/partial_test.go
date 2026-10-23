package html

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func assertHTMLPartialMatchesFresh(t *testing.T, world *ecs.World, owner ecs.Entity, source string) {
	t.Helper()
	fresh, freshOwner := incrementalHTMLFixture(t, source)
	got, _ := ecs.Get[htmlRenderState](world, owner)
	want, _ := ecs.Get[htmlRenderState](fresh, freshOwner)
	compareHTMLLayout(t, got.Boxes, want.Boxes)
	if !equalHTMLPrimitiveSnapshots(htmlPrimitiveSnapshot(world, got), htmlPrimitiveSnapshot(fresh, want)) {
		t.Fatalf("partial primitives differ from fresh render for %s\ngot: %#v\nwant: %#v", source, htmlPrimitiveSnapshot(world, got), htmlPrimitiveSnapshot(fresh, want))
	}
	seen := make(map[ecs.Entity]bool)
	for _, entity := range got.Children {
		if !world.IsAlive(entity) || seen[entity] {
			t.Fatal("owner list contains a dead or duplicate entity")
		}
		seen[entity] = true
	}
	for _, group := range got.EntityGroups {
		for _, binding := range group.Bindings {
			if !seen[binding.Entity] {
				t.Fatal("generated entity missing from owner list")
			}
		}
	}
	for node, rendered := range got.RenderNodes {
		for _, entity := range rendered.Entities {
			if !seen[entity] || htmlEntityNode(world, entity) != node {
				t.Fatal("stale node-to-entity index")
			}
		}
	}
}

func TestHTMLPartialTextTouchesOnlyAffectedBoxes(t *testing.T) {
	for _, count := range []int{10, 1000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, `<div id="target">Before</div>`+strings.Repeat(`<div>Unchanged</div>`, count))
			target := incrementalHTMLNode(t, world, owner, "target")
			before, _ := ecs.Get[htmlRenderState](world, owner)
			children := slices.Clone(before.Children)
			target.SetText("After")
			incrementalSync(t, world)
			after, _ := ecs.Get[htmlRenderState](world, owner)
			if after.SyncStats.PartialReconciles != 1 || after.SyncStats.ReconciledBoxes != 2 || after.SyncStats.Reused != 1 || !slices.Equal(children, after.Children) {
				t.Fatalf("unrelated primitives reconciled: %+v", after.SyncStats)
			}
		})
	}
}

func TestHTMLPartialFragmentGrowthShrinkAndPaint(t *testing.T) {
	const suffix = `<div id="sibling">Unchanged</div>`
	world, owner := incrementalHTMLFixture(t, `<div id="target" style="width:40px">A</div>`+suffix)
	target := incrementalHTMLNode(t, world, owner, "target")
	for _, step := range []struct{ css, text string }{
		{"width:40px", "Longer wrapping text"},
		{"width:40px;background:red;text-decoration:line-through", "Longer wrapping text"},
		{"width:40px;border-top:2px solid blue", "A"},
		{"width:40px", " "},
		{"width:40px;background:blue", "Restored"},
		{"width:40px", "A"},
	} {
		target.SetAttribute("style", step.css)
		target.SetText(step.text)
		incrementalSync(t, world)
		state, _ := ecs.Get[htmlRenderState](world, owner)
		if state.SyncStats.PartialReconciles != 1 {
			t.Fatalf("ordinary fragment mutation fell back: %+v", state.SyncStats)
		}
		assertHTMLPartialMatchesFresh(t, world, owner, `<div id="target" style="`+step.css+`">`+step.text+`</div>`+suffix)
	}
}

func TestHTMLPartialTranslatedAndFlexPrimitives(t *testing.T) {
	for _, container := range []string{"", "position:relative;left:3px;top:5px", "display:flex;flex-wrap:wrap", "display:flex;flex-direction:column"} {
		t.Run(container, func(t *testing.T) {
			source := func(width int) string {
				return fmt.Sprintf(`<div style="%s"><div id="target" style="width:%dpx">Wrapping text here</div><div style="width:120px"><div>Nested following text</div></div></div><div>Last</div>`, container, width)
			}
			world, owner := incrementalHTMLFixture(t, source(120))
			target := incrementalHTMLNode(t, world, owner, "target")
			for _, width := range []int{30, 100, 40, 120} {
				target.SetAttribute("style", fmt.Sprintf("width:%dpx", width))
				incrementalSync(t, world)
				state, _ := ecs.Get[htmlRenderState](world, owner)
				if state.SyncStats.PartialReconciles != 1 {
					t.Fatal("ordinary geometry update did not reconcile locally")
				}
				assertHTMLPartialMatchesFresh(t, world, owner, source(width))
			}
		})
	}
}

func TestHTMLPartialPreservesUnrelatedControlState(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target">Before</div><div><input id="field" value="initial"/></div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	field := incrementalHTMLNode(t, world, owner, "field")
	state, _ := ecs.Get[htmlRenderState](world, owner)
	var control ecs.Entity
	for key, group := range state.EntityGroups {
		if key.Node == field.node && key.Role == htmlRoleControl {
			control = group.Bindings[0].Entity
			break
		}
	}
	if control == 0 {
		t.Fatal("missing control")
	}
	SetControlValue(world, control, "typed")
	live := ecs.GetMut[Control](world, control)
	live.SelectionAnchor, live.SelectionFocus = 1, 3
	revision := live.RenderRevision
	target.SetText("After")
	incrementalSync(t, world)
	state, _ = ecs.Get[htmlRenderState](world, owner)
	live = ecs.GetMut[Control](world, control)
	if state.SyncStats.PartialReconciles != 1 || live.Value != "typed" || live.SelectionAnchor != 1 || live.SelectionFocus != 3 || live.RenderRevision != revision {
		t.Fatal("unrelated control was rewritten")
	}
}

func TestHTMLPartialUpdatesStackingDependencies(t *testing.T) {
	for _, context := range []string{"overflow:auto;height:40px", "overflow:hidden;height:0px", "position:relative;z-index:1"} {
		t.Run(context, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, `<div id="target" style="`+context+`"><div>Child</div></div>`)
			target := incrementalHTMLNode(t, world, owner, "target")
			next := context + ";width:120px;z-index:2"
			target.SetAttribute("style", next)
			incrementalSync(t, world)
			state, _ := ecs.Get[htmlRenderState](world, owner)
			if state.SyncStats.PartialReconciles != 1 {
				t.Fatal("stacking dependency fell back")
			}
			assertHTMLPartialMatchesFresh(t, world, owner, `<div id="target" style="`+next+`"><div>Child</div></div>`)
		})
	}
}

func TestHTMLPartialEmptyTextRemovesAllFragments(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target">Before</div><div>Unchanged</div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	before, _ := ecs.Get[htmlRenderState](world, owner)
	removed := slices.Clone(before.RenderNodes[target.node.Children[0]].Entities)
	target.SetText("")
	incrementalSync(t, world)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	if state.SyncStats.PartialReconciles != 1 || state.RenderNodes[target.node.Children[0]] != nil {
		t.Fatal("empty text left a stale rendering index")
	}
	for _, entity := range removed {
		if world.IsAlive(entity) || slices.Contains(state.Children, entity) {
			t.Fatal("empty text retained an old fragment")
		}
	}
}

func TestHTMLPartialEventBindingAfterSkippedNode(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target">Before</div><div id="other" style="background:red">Other</div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	other := incrementalHTMLNode(t, world, owner, "other")
	target.SetText("After")
	incrementalSync(t, world)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	surface := state.EntityGroups[htmlRenderIdentity{Node: other.node, Role: htmlRoleSurface}].Bindings[0].Entity
	clicks := 0
	other.OnClick(func(*PointerEvent) { clicks++ })
	incrementalSync(t, world)
	state, _ = ecs.Get[htmlRenderState](world, owner)
	if state.EventTargets[other.node] != surface {
		t.Fatal("event-only binding failed to reuse skipped surface")
	}
	scene.DispatchInputEvent(world, scene.InputEvent{Entity: surface, Kind: scene.InputEventTap})
	if clicks != 1 {
		t.Fatal("retained event handler did not fire")
	}
	other.OnClick(nil)
	incrementalSync(t, world)
	target.SetText("Again")
	incrementalSync(t, world)
	state, _ = ecs.Get[htmlRenderState](world, owner)
	if state.SyncStats.PartialReconciles != 0 {
		t.Fatal("event-only membership changes did not refresh box index")
	}
	target.SetText("After")
	incrementalSync(t, world)
	state, _ = ecs.Get[htmlRenderState](world, owner)
	if state.SyncStats.PartialReconciles != 1 {
		t.Fatal("partial updates did not resume")
	}
	assertHTMLPartialMatchesFresh(t, world, owner, `<div id="target">After</div><div id="other" style="background:red">Other</div>`)
}

func TestHTMLPartialAfterTopologyRefresh(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target">Before</div><div id="other">Other</div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	other := incrementalHTMLNode(t, world, owner, "other")
	for range 3 {
		other.SetAttribute("style", "display:none")
		incrementalSync(t, world)
		target.SetText("After")
		incrementalSync(t, world)
		assertHTMLPartialMatchesFresh(t, world, owner, `<div id="target">After</div><div id="other" style="display:none">Other</div>`)
		other.RemoveAttribute("style")
		incrementalSync(t, world)
		target.SetText("Before")
		incrementalSync(t, world)
		state, _ := ecs.Get[htmlRenderState](world, owner)
		if state.SyncStats.PartialReconciles != 1 {
			t.Fatal("partial updates did not resume after topology refresh")
		}
		assertHTMLPartialMatchesFresh(t, world, owner, `<div id="target">Before</div><div id="other">Other</div>`)
	}
}

func TestHTMLPartialAbsolutePositionAfterHeightChange(t *testing.T) {
	for _, position := range []string{"bottom:0;right:0", "top:10%;left:5%"} {
		t.Run(position, func(t *testing.T) {
			source := func(width int) string {
				return fmt.Sprintf(`<div style="position:relative"><div id="target" style="width:%dpx">Wrapping text here</div><div style="position:absolute;%s"><div>Positioned</div></div></div><div>After</div>`, width, position)
			}
			world, owner := incrementalHTMLFixture(t, source(120))
			target := incrementalHTMLNode(t, world, owner, "target")
			for _, width := range []int{30, 100, 40, 120} {
				target.SetAttribute("style", fmt.Sprintf("width:%dpx", width))
				incrementalSync(t, world)
				assertHTMLPartialMatchesFresh(t, world, owner, source(width))
			}
		})
	}
}
