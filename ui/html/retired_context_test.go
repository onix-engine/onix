package html

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/scene"
)

func TestHTMLRetiredSourceContextsMatchFresh(t *testing.T) {
	for _, attrs := range []string{"", `tabindex="0"`, `contenteditable="true"`} {
		for _, destinationAttrs := range []string{"", `tabindex="0"`, `contenteditable="true"`} {
			t.Run(attrs+"/"+destinationAttrs, func(t *testing.T) {
				const moved = `<div id="moved" style="position:relative;z-index:2"><div id="inner" style="height:30px;overflow:auto"><input id="field" value="Value"/><div style="height:100px">Text</div></div><div style="position:absolute;top:10px">Absolute</div><span> </span></div>`
				prefix := `<div id="destination" ` + destinationAttrs + ` style="height:150px;overflow:auto">`
				suffix := `</div></div><div><input id="other" value="Unchanged"/></div>`
				world, owner := incrementalHTMLFixture(t, `<div style="height:400px"><div id="retired" `+attrs+` style="height:100px;overflow:auto"><div id="source" `+attrs+` style="height:80px;overflow:hidden">`+moved+`</div><div>Retired sibling</div></div>`+prefix+suffix)
				retired := incrementalHTMLNode(t, world, owner, "retired")
				source := incrementalHTMLNode(t, world, owner, "source")
				node := incrementalHTMLNode(t, world, owner, "moved")
				dest := incrementalHTMLNode(t, world, owner, "destination")
				before, _ := world.Get[htmlRenderState](owner)
				oldClip := before.OverflowBindings[retired.node].entity
				field := incrementalControl(t, world, owner, "field")
				other := incrementalControl(t, world, owner, "other")
				unchanged, _ := world.Get[Control](other)
				node.Remove()
				retired.Remove()
				dest.Append(node)
				incrementalSync(t, world)
				state, _ := world.Get[htmlRenderState](owner)
				if state.SyncStats.PartialReconciles != 1 || world.IsAlive(oldClip) {
					t.Fatalf("retired context was reused: %+v", state.SyncStats)
				}
				if incrementalControl(t, world, owner, "field") != field {
					t.Fatal("surviving input replaced")
				}
				after, _ := world.Get[Control](other)
				if after.RenderRevision != unchanged.RenderRevision {
					t.Fatal("unrelated input rebuilt")
				}
				assertHTMLContextMatchesFresh(t, world, owner, `<div style="height:400px">`+prefix+moved+suffix)
				for _, old := range []Node{retired, source} {
					if state.BoxRenderings[old.node] != nil || state.EventTargets[old.node] != 0 {
						t.Fatal("retired node remains indexed")
					}
					if _, ok := state.FocusByNode[old.node]; ok {
						t.Fatal("retired focus owner remains indexed")
					}
				}
				for _, entity := range state.Children {
					if parent, ok := world.Get[scene.Parent](entity); ok && !world.IsAlive(parent.Entity) {
						t.Fatal("dead parent")
					}
					if focus, ok := world.Get[FocusOwner](entity); ok && !world.IsAlive(focus.Entity) {
						t.Fatal("dead focus owner")
					}
					if content, ok := world.Get[ScrollContent](entity); ok && !world.IsAlive(content.Area) {
						t.Fatal("dead scroll area")
					}
				}
				for _, route := range state.OverflowRoutes {
					if !world.IsAlive(route.entity) {
						t.Fatal("dead clip route")
					}
				}
				// A later insertion of the detached source must create fresh context state.
				dest.Append(retired)
				incrementalSync(t, world)
				state, _ = world.Get[htmlRenderState](owner)
				if state.SyncStats.PartialReconciles != 1 {
					t.Fatal("later source reattachment fell back")
				}
				if newClip := state.OverflowBindings[retired.node].entity; newClip == oldClip || !world.IsAlive(newClip) {
					t.Fatal("source clip was not recreated")
				}
			})
		}
	}
}

func TestHTMLRetiredSourceKeepsSurvivingScrollAndInput(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="source" tabindex="0" style="height:120px;overflow:auto"><div id="moved" style="height:40px;overflow:auto"><div style="height:100px"><input id="field" value="Value"/></div></div></div><div id="destination" style="height:120px"></div>`)
	source := incrementalHTMLNode(t, world, owner, "source")
	node := incrementalHTMLNode(t, world, owner, "moved")
	field := incrementalControl(t, world, owner, "field")
	SetControlValue(world, field, "Typed")
	control := world.GetMut[Control](field)
	control.SelectionAnchor, control.SelectionFocus = 1, 3
	state, _ := world.Get[htmlRenderState](owner)
	clip := state.OverflowBindings[node.node].entity
	scroll := world.GetMut[scene.UIScroll](clip)
	scroll.Position[1] = -15
	node.Remove()
	source.Remove()
	incrementalHTMLNode(t, world, owner, "destination").Append(node)
	incrementalSync(t, world)
	state, _ = world.Get[htmlRenderState](owner)
	gotScroll, _ := world.Get[scene.UIScroll](clip)
	gotControl, _ := world.Get[Control](field)
	if state.SyncStats.PartialReconciles != 1 || state.OverflowBindings[node.node].entity != clip || gotScroll.Position[1] != -15 || gotControl.Value != "Typed" || gotControl.SelectionAnchor != 1 || gotControl.SelectionFocus != 3 {
		t.Fatal("survivor state lost")
	}
}

func BenchmarkHTMLRetiredSource(b *testing.B) {
	for _, count := range []int{10, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<div id="container" style="height:240px"><div id="source" tabindex="0" style="height:80px;overflow:auto"><div id="moved"><input value="Value"/></div></div><div id="destination" style="height:80px;overflow:auto"></div></div>`+strings.Repeat(`<div style="height:40px;overflow:auto"><input value="Unchanged"/></div>`, count))
			container := incrementalHTMLNode(b, world, owner, "container")
			source := incrementalHTMLNode(b, world, owner, "source")
			dest := incrementalHTMLNode(b, world, owner, "destination")
			moved := incrementalHTMLNode(b, world, owner, "moved")
			cycle := func() {
				moved.Remove()
				source.Remove()
				dest.Append(moved)
				incrementalSync(b, world)
				moved.Remove()
				source.Append(moved)
				container.Append(source)
				incrementalSync(b, world)
			}
			for range 2 {
				cycle()
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				cycle()
			}
		})
	}
}

func TestHTMLRetiredSourceEventsAndUnrelatedControls(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div style="height:240px"><div id="source" tabindex="0" style="height:80px;overflow:auto"><div id="moved"><input value="Value"/></div></div><div id="destination" style="height:80px;overflow:auto"></div></div>`+strings.Repeat(`<div style="height:40px;overflow:auto"><input value="Unchanged"/></div>`, 1000))
	source := incrementalHTMLNode(t, world, owner, "source")
	moved := incrementalHTMLNode(t, world, owner, "moved")
	calls := 0
	source.OnClick(func(*PointerEvent) { calls += 100 })
	moved.OnClick(func(*PointerEvent) { calls++ })
	source.SetAttribute("style", "height:80px;overflow:auto;background:red")
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	oldTarget := state.EventTargets[source.node]
	moved.Remove()
	source.Remove()
	incrementalHTMLNode(t, world, owner, "destination").Append(moved)
	incrementalSync(t, world)
	state, _ = world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 || state.SyncStats.UpdatedControls != 1 || state.SyncStats.SnapshottedControls != 1 || state.SyncStats.UpdatedClips != 1 {
		t.Fatalf("retired source rebuilt unrelated contexts: %+v", state.SyncStats)
	}
	if world.IsAlive(oldTarget) || state.EventTargets[source.node] != 0 {
		t.Fatal("retired event target survived")
	}
	scene.DispatchInputEvent(world, scene.InputEvent{Entity: state.EventTargets[moved.node], Kind: scene.InputEventTap})
	if calls != 1 {
		t.Fatal("moved event handler lost or old source handler fired")
	}
}
