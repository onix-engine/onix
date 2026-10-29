package html

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestHTMLReparentMatchesFresh(t *testing.T) {
	for _, contexts := range []struct{ name, left, right string }{
		{"plain", "", ""},
		{"enter-focus", "", `tabindex="0"`},
		{"enter-editing", "", `contenteditable="true"`},
		{"focus", `tabindex="0"`, `tabindex="0"`},
		{"editing", `contenteditable="true"`, `contenteditable="true"`},
	} {
		for _, clips := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/clips=%v", contexts.name, clips), func(t *testing.T) {
				moved := `<div id="moved" style="position:relative;z-index:2"><div id="inner" style="overflow:auto;height:30px"><input id="field" value="Value"/><div style="height:100px">Text</div></div><div style="position:absolute;left:10px;top:10px">Absolute</div><span> </span></div>`
				source := func(right bool) string {
					leftContent, rightContent := moved, ""
					if right {
						leftContent, rightContent = "", moved
					}
					overflow := "visible"
					if clips {
						overflow = "auto"
					}
					return `<div id="left" ` + contexts.left + ` style="position:relative;z-index:3;color:red;height:120px;overflow:` + overflow + `">` + leftContent + `</div><div style="height:200px"><div id="right" ` + contexts.right + ` style="position:relative;z-index:-1;color:blue;height:80px;overflow:` + overflow + `">` + rightContent + `</div></div><div><input id="other" value="Unchanged"/></div>`
				}
				world, owner := incrementalHTMLFixture(t, source(false))
				movedNode := incrementalHTMLNode(t, world, owner, "moved")
				left := incrementalHTMLNode(t, world, owner, "left")
				right := incrementalHTMLNode(t, world, owner, "right")
				other := incrementalControl(t, world, owner, "other")
				before, _ := world.Get[Control](other)
				field := incrementalControl(t, world, owner, "field")
				for _, destination := range []bool{true, false, true, false} {
					movedNode.Remove()
					if destination {
						right.Append(movedNode)
					} else {
						left.Append(movedNode)
					}
					incrementalSync(t, world)
					state, _ := world.Get[htmlRenderState](owner)
					if state.SyncStats.PartialReconciles != 1 {
						t.Fatalf("move fell back: %+v", state.SyncStats)
					}
					if incrementalControl(t, world, owner, "field") != field {
						t.Fatal("moved input replaced")
					}
					after, _ := world.Get[Control](other)
					if after.RenderRevision != before.RenderRevision {
						t.Fatal("unrelated control rebuilt")
					}
					for _, rendered := range state.BoxRenderings {
						if rendered.Depth != htmlBoxDepth(rendered.Box)-1 || rendered.Parent != rendered.Box.Parent {
							t.Fatal("stale box ancestry")
						}
					}
					assertHTMLContextMatchesFresh(t, world, owner, source(destination))
				}
			})
		}
	}
}

func TestHTMLReparentPreservesInputAndNestedScroll(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="left" style="height:120px;overflow:auto"><div id="moved" style="overflow:auto;height:40px"><div style="height:100px"><input id="field" value="Value"/></div></div></div><div id="right" style="height:120px"></div>`)
	moved := incrementalHTMLNode(t, world, owner, "moved")
	right := incrementalHTMLNode(t, world, owner, "right")
	field := incrementalControl(t, world, owner, "field")
	SetControlValue(world, field, "Typed")
	control := world.GetMut[Control](field)
	control.SelectionAnchor, control.SelectionFocus = 1, 3
	state, _ := world.Get[htmlRenderState](owner)
	clip := state.OverflowBindings[moved.node].entity
	scroll := world.GetMut[scene.UIScroll](clip)
	scroll.Position[1] = -15
	moved.Remove()
	right.Append(moved)
	incrementalSync(t, world)
	state, _ = world.Get[htmlRenderState](owner)
	scrollValue, _ := world.Get[scene.UIScroll](clip)
	controlValue, _ := world.Get[Control](field)
	if state.SyncStats.PartialReconciles != 1 || state.OverflowBindings[moved.node].entity != clip || scrollValue.Position[1] != -15 || controlValue.Value != "Typed" || controlValue.SelectionAnchor != 1 || controlValue.SelectionFocus != 3 {
		t.Fatal("move reset input or scroll state")
	}
	parent, _ := world.Get[scene.Parent](clip)
	if parent.Entity != owner {
		t.Fatal("clip retained previous parent")
	}
}

func BenchmarkHTMLReparent(b *testing.B) {
	for _, count := range []int{10, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<div id="left" style="height:80px;overflow:auto"><div id="moved"><input value="Value"/></div></div><div id="right" style="height:80px;overflow:auto"></div>`+strings.Repeat(`<div style="height:40px;overflow:auto"><input value="Unchanged"/></div>`, count))
			moved := incrementalHTMLNode(b, world, owner, "moved")
			left := incrementalHTMLNode(b, world, owner, "left")
			right := incrementalHTMLNode(b, world, owner, "right")
			for range 2 {
				moved.Remove()
				right.Append(moved)
				incrementalSync(b, world)
				moved.Remove()
				left.Append(moved)
				incrementalSync(b, world)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				moved.Remove()
				if i%2 == 0 {
					right.Append(moved)
				} else {
					left.Append(moved)
				}
				incrementalSync(b, world)
			}
		})
	}
}

func TestHTMLReparentExternalBindingsMatchFull(t *testing.T) {
	const source = `<div id="left" style="height:120px;overflow:auto"><div id="moved"><input value="Value"/><div>Text</div></div></div><div id="right" style="height:120px"></div>`
	world, owner := incrementalHTMLFixture(t, source)
	reference, refOwner := incrementalHTMLFixture(t, source)
	for _, fixture := range []struct {
		world *ecs.World
		owner ecs.Entity
	}{{world, owner}, {reference, refOwner}} {
		area := htmlBindingArea(fixture.world, -3, -7)
		SetNodeOffset(fixture.world, fixture.owner, "left", 5, 11)
		SetNodeScroll(fixture.world, fixture.owner, "right", area, 13, 17)
	}
	moved := incrementalHTMLNode(t, world, owner, "moved")
	refMoved := incrementalHTMLNode(t, reference, refOwner, "moved")
	for _, id := range []string{"right", "left", "right", "left"} {
		moved.Remove()
		incrementalHTMLNode(t, world, owner, id).Append(moved)
		refMoved.Remove()
		incrementalHTMLNode(t, reference, refOwner, id).Append(refMoved)
		refState := reference.GetMut[htmlRenderState](refOwner)
		refState.PartialValid = false
		incrementalSync(t, world)
		incrementalSync(t, reference)
		state, _ := world.Get[htmlRenderState](owner)
		want, _ := reference.Get[htmlRenderState](refOwner)
		if state.SyncStats.PartialReconciles != 1 || !equalHTMLPrimitiveSnapshots(htmlPrimitiveSnapshot(world, state), htmlPrimitiveSnapshot(reference, want)) {
			t.Fatal("bound move differs from full render")
		}
	}
}

func TestHTMLReparentFromRetiredParentIsLocal(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="left" tabindex="0"><div id="moved">Text</div></div><div id="right"></div>`)
	moved := incrementalHTMLNode(t, world, owner, "moved")
	moved.Remove()
	incrementalHTMLNode(t, world, owner, "left").Remove()
	incrementalHTMLNode(t, world, owner, "right").Append(moved)
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 {
		t.Fatal("retired source context fell back")
	}
	assertHTMLPartialMatchesFresh(t, world, owner, `<div id="right"><div id="moved">Text</div></div>`)
}

func TestHTMLReparentSkipsUnrelatedControls(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="left" style="height:80px;overflow:auto"><div id="moved"><input value="Value"/></div></div><div id="right" style="height:80px;overflow:auto"></div>`+strings.Repeat(`<div style="height:40px;overflow:auto"><input value="Unchanged"/></div>`, 1000))
	moved := incrementalHTMLNode(t, world, owner, "moved")
	moved.Remove()
	incrementalHTMLNode(t, world, owner, "right").Append(moved)
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 || state.SyncStats.UpdatedControls != 1 || state.SyncStats.SnapshottedControls != 1 || state.SyncStats.UpdatedClips != 2 {
		t.Fatalf("move updated unrelated contexts: %+v", state.SyncStats)
	}
}

func TestHTMLReparentReroutesUnchangedGeometry(t *testing.T) {
	const moved = `<div id="moved"><div style="position:absolute;left:10px;top:10px"><input value="Value"/></div><span> </span></div>`
	source := func(right bool) string {
		a, b := moved, ""
		if right {
			a, b = "", moved
		}
		return `<div id="left" style="position:absolute;left:0;top:0;width:120px;height:80px;overflow:auto">` + a + `</div><div id="right" style="position:absolute;left:0;top:0;width:120px;height:80px">` + b + `</div>`
	}
	world, owner := incrementalHTMLFixture(t, source(false))
	node := incrementalHTMLNode(t, world, owner, "moved")
	for _, right := range []bool{true, false, true, false} {
		destination := "left"
		if right {
			destination = "right"
		}
		node.Remove()
		incrementalHTMLNode(t, world, owner, destination).Append(node)
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		if state.SyncStats.PartialReconciles != 1 {
			t.Fatal("same-geometry move fell back")
		}
		assertHTMLContextMatchesFresh(t, world, owner, source(right))
	}
}
