package html

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/scene"
)

func TestHTMLStructuralRenderingMatchesFresh(t *testing.T) {
	for _, first := range []string{
		`<div id="first">First</div>`,
		`<div id="first"><input value="Value"/></div>`,
		`<div id="first" tabindex="0"><div>Focus</div></div>`,
		`<div id="first" contenteditable="true"><span>Editable</span></div>`,
		`<div id="first" style="position:relative;z-index:2"><div style="position:relative;z-index:-1">Stacked</div></div>`,
	} {
		t.Run(first, func(t *testing.T) {
			const prefix = `<div id="parent" style="height:100px">`
			const second = `<div id="second"><input value="Second"/></div>`
			const suffix = `</div><div id="other" tabindex="0"><div><input id="otherControl" value="Unchanged"/></div></div>`
			world, owner := incrementalHTMLFixture(t, prefix+first+second+suffix)
			parent := incrementalHTMLNode(t, world, owner, "parent")
			removed := incrementalHTMLNode(t, world, owner, "first")
			other := incrementalControl(t, world, owner, "otherControl")
			before, _ := world.Get[Control](other)
			for _, step := range []struct {
				apply  func()
				source string
			}{
				{func() { removed.Remove() }, prefix + second + suffix},
				{func() { parent.Append(removed) }, prefix + second + first + suffix},
				{func() { parent.Clear() }, prefix + suffix},
				{func() { parent.Append(removed) }, prefix + first + suffix},
			} {
				step.apply()
				incrementalSync(t, world)
				state, _ := world.Get[htmlRenderState](owner)
				if state.SyncStats.PartialReconciles != 1 {
					t.Fatalf("structure fell back: %+v", state.SyncStats)
				}
				after, _ := world.Get[Control](other)
				if after.RenderRevision != before.RenderRevision {
					t.Fatal("unrelated control rebuilt")
				}
				assertHTMLPartialMatchesFresh(t, world, owner, step.source)
				for _, entity := range state.Children {
					if focus, ok := world.Get[Focusable](entity); ok && focus.Order != htmlEntityNode(world, entity).Order {
						t.Fatal("stale focus order")
					}
				}
				for node := range state.BoxRenderings {
					if !htmlNodeWithin(node, state.Root) {
						t.Fatal("retired box retained")
					}
				}
			}
		})
	}
}

func TestHTMLStructureReordersUntouchedStackingContexts(t *testing.T) {
	const prefix = `<div id="parent" style="height:40px"><div id="remove">Remove</div></div>`
	const suffix = `<div style="position:relative;z-index:3"><div style="position:relative;z-index:2"><a href="#" id="link">Unchanged link</a></div></div>`
	world, owner := incrementalHTMLFixture(t, prefix+suffix)
	incrementalHTMLNode(t, world, owner, "remove").Remove()
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 || state.SyncStats.OrderUpdated == 0 {
		t.Fatalf("missing order-only update: %+v", state.SyncStats)
	}
	assertHTMLPartialMatchesFresh(t, world, owner, `<div id="parent" style="height:40px"></div>`+suffix)
}

func TestHTMLStructureFlowMovementMatchesFresh(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="parent"><div id="remove">Remove</div></div><div>Moved</div><div><input value="Value"/></div>`)
	incrementalHTMLNode(t, world, owner, "remove").Remove()
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 {
		t.Fatalf("flow structure fell back: %+v", state.SyncStats)
	}
	assertHTMLPartialMatchesFresh(t, world, owner, `<div id="parent"></div><div>Moved</div><div><input value="Value"/></div>`)
}

func TestHTMLStructureRetainsBoundControlState(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="parent" style="height:40px"><div id="remove">Remove</div></div><div><input id="other" value="Value"/></div>`)
	other := incrementalControl(t, world, owner, "other")
	SetControlValue(world, other, "Typed")
	incrementalSync(t, world)
	area := htmlBindingArea(world, -3, -7)
	SetNodeOffset(world, owner, "other", 5, 11)
	SetNodeScroll(world, owner, "other", area, 13, 17)
	transform, _ := world.Get[scene.UITransform](other)
	content, _ := world.Get[ScrollContent](other)
	control := world.GetMut[Control](other)
	control.SelectionAnchor, control.SelectionFocus = 1, 3
	before := *control
	incrementalHTMLNode(t, world, owner, "remove").Remove()
	incrementalSync(t, world)
	after, _ := world.Get[Control](other)
	next, _ := world.Get[scene.UITransform](other)
	scroll, _ := world.Get[ScrollContent](other)
	state, _ := world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 || after.Value != "Typed" || after.SelectionAnchor != 1 || after.SelectionFocus != 3 || after.RenderRevision != before.RenderRevision || next != transform || scroll != content {
		t.Fatal("unrelated bound control changed")
	}
}

func BenchmarkHTMLStructuralRendering(b *testing.B) {
	for _, count := range []int{10, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<div id="parent" style="height:40px"><div id="child">Target</div></div>`+strings.Repeat(`<div><input value="Unchanged"/></div>`, count))
			parent := incrementalHTMLNode(b, world, owner, "parent")
			child := incrementalHTMLNode(b, world, owner, "child")
			for range 2 {
				child.Remove()
				incrementalSync(b, world)
				parent.Append(child)
				incrementalSync(b, world)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if i%2 == 0 {
					child.Remove()
				} else {
					parent.Append(child)
				}
				incrementalSync(b, world)
			}
		})
	}
}

func TestHTMLStructureEventRetirementAndReattach(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="parent" style="height:40px"><div id="child" style="height:20px"></div></div><div id="other" style="height:20px"></div>`)
	parent := incrementalHTMLNode(t, world, owner, "parent")
	child := incrementalHTMLNode(t, world, owner, "child")
	other := incrementalHTMLNode(t, world, owner, "other")
	clicks := 0
	child.OnClick(func(*PointerEvent) { clicks++ })
	other.OnClick(func(*PointerEvent) { clicks++ })
	parent.SetAttribute("style", "height:40px;background:red")
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	retired, kept := state.EventTargets[child.node], state.EventTargets[other.node]
	child.Remove()
	incrementalSync(t, world)
	state, _ = world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 || world.IsAlive(retired) || state.EventTargets[child.node] != 0 || state.EventTargets[other.node] != kept {
		t.Fatal("event retirement or retention failed")
	}
	scene.DispatchInputEvent(world, scene.InputEvent{Entity: kept, Kind: scene.InputEventTap})
	parent.Append(child)
	incrementalSync(t, world)
	state, _ = world.Get[htmlRenderState](owner)
	scene.DispatchInputEvent(world, scene.InputEvent{Entity: state.EventTargets[child.node], Kind: scene.InputEventTap})
	if clicks != 2 {
		t.Fatal("event callbacks lost after structural update")
	}
}

func TestHTMLStructureClipSupportAndConservativeFallbacks(t *testing.T) {
	t.Run("clip", func(t *testing.T) {
		world, owner := incrementalHTMLFixture(t, `<div id="scroller" style="overflow:auto;height:40px"><div id="child">Text</div></div>`)
		incrementalHTMLNode(t, world, owner, "child").Remove()
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		if state.SyncStats.PartialReconciles != 1 {
			t.Fatal("stable clip structure fell back")
		}
		assertHTMLContextMatchesFresh(t, world, owner, `<div id="scroller" style="overflow:auto;height:40px"></div>`)
	})
	t.Run("reparent", func(t *testing.T) {
		world, owner := incrementalHTMLFixture(t, `<div><div id="child">Text</div></div><div id="destination"></div>`)
		child := incrementalHTMLNode(t, world, owner, "child")
		child.Remove()
		incrementalHTMLNode(t, world, owner, "destination").Append(child)
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		if state.SyncStats.PartialReconciles != 1 {
			t.Fatal("retained box reparent fell back")
		}
		assertHTMLPartialMatchesFresh(t, world, owner, `<div></div><div id="destination"><div id="child">Text</div></div>`)
	})
	t.Run("display", func(t *testing.T) {
		const css = `<style>#parent:empty{display:none}</style>`
		world, owner := incrementalHTMLFixture(t, css+`<div id="parent"><div id="child">Text</div></div><div>After</div>`)
		incrementalHTMLNode(t, world, owner, "child").Remove()
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		if state.SyncStats.PartialReconciles != 0 {
			t.Fatal("disappearing attached box unexpectedly local")
		}
		assertHTMLPartialMatchesFresh(t, world, owner, css+`<div id="parent"></div><div>After</div>`)
	})
}

func TestHTMLStructureInsideFocusContext(t *testing.T) {
	for _, attributes := range []string{`tabindex="0"`, `contenteditable="true"`} {
		t.Run(attributes, func(t *testing.T) {
			prefix := `<div id="parent" ` + attributes + `>`
			const childSource = `<div id="child">Text</div>`
			const suffix = `</div><div>After</div>`
			world, owner := incrementalHTMLFixture(t, prefix+childSource+suffix)
			child := incrementalHTMLNode(t, world, owner, "child")
			parent := incrementalHTMLNode(t, world, owner, "parent")
			child.Remove()
			incrementalSync(t, world)
			assertHTMLPartialMatchesFresh(t, world, owner, prefix+suffix)
			parent.Append(child)
			incrementalSync(t, world)
			state, _ := world.Get[htmlRenderState](owner)
			if state.SyncStats.PartialReconciles != 1 {
				t.Fatalf("context structure fell back: %+v", state.SyncStats)
			}
			assertHTMLPartialMatchesFresh(t, world, owner, prefix+childSource+suffix)
		})
	}
}

func TestHTMLStructuralUpdateSkipsUnrelatedControls(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="parent" style="height:40px"><div id="child">Target</div></div>`+strings.Repeat(`<div><input value="Unchanged"/></div>`, 1000))
	incrementalHTMLNode(t, world, owner, "child").Remove()
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 || state.SyncStats.UpdatedControls != 0 || state.SyncStats.SnapshottedControls != 0 || state.SyncStats.ReconciledBoxes > 3 || state.SyncStats.OrderUpdated == 0 {
		t.Fatalf("unrelated controls rebuilt: %+v", state.SyncStats)
	}
}
