package html

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestHTMLStructureClipInsertionRemovalMatchesFresh(t *testing.T) {
	for _, mode := range []string{"auto", "scroll", "hidden", "clip"} {
		for _, outer := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", mode, outer), func(t *testing.T) {
				prefix := `<div id="parent" style="height:120px">`
				suffix := `</div><div id="other" style="overflow:auto;height:40px"><input id="field" value="Unchanged"/></div>`
				if outer {
					prefix = `<div id="outer" style="overflow:auto;height:160px">` + prefix
					suffix = `</div>` + suffix
				}
				childSource := `<div id="child" style="height:50px;overflow:` + mode + `"><div id="nested" style="overflow:hidden;height:30px"><div style="height:90px">Nested</div></div><div style="height:130px">Text</div></div>`
				world, owner := incrementalHTMLFixture(t, prefix+childSource+suffix)
				parent := incrementalHTMLNode(t, world, owner, "parent")
				child := incrementalHTMLNode(t, world, owner, "child")
				field := incrementalControl(t, world, owner, "field")
				before, _ := world.Get[Control](field)
				for _, present := range []bool{false, true, false, true} {
					if present {
						parent.Append(child)
					} else {
						child.Remove()
					}
					incrementalSync(t, world)
					state, _ := world.Get[htmlRenderState](owner)
					if state.SyncStats.PartialReconciles != 1 {
						t.Fatalf("clip structure fell back: %+v", state.SyncStats)
					}
					after, _ := world.Get[Control](field)
					if after.RenderRevision != before.RenderRevision {
						t.Fatal("unrelated clipped control rebuilt")
					}
					content := ""
					if present {
						content = childSource
					}
					assertHTMLContextMatchesFresh(t, world, owner, prefix+content+suffix)
					for node, binding := range state.OverflowBindings {
						if !world.IsAlive(binding.entity) || !htmlNodeWithin(node, state.Root) {
							t.Fatal("retired clip remains indexed")
						}
					}
					for node, binding := range state.OverflowRoutes {
						if !htmlNodeWithin(node, state.Root) || !world.IsAlive(binding.entity) {
							t.Fatal("retired route remains indexed")
						}
					}
				}
			})
		}
	}
}

func TestHTMLStructureCreatesFirstClipAndRemovesLast(t *testing.T) {
	const prefix = `<div id="parent" style="height:100px">`
	const childSource = `<div id="child" style="overflow:auto;height:40px"><div style="height:130px">Text</div></div>`
	const suffix = `</div><div>After</div>`
	world, owner := incrementalHTMLFixture(t, prefix+childSource+suffix)
	parent := incrementalHTMLNode(t, world, owner, "parent")
	child := incrementalHTMLNode(t, world, owner, "child")
	for _, present := range []bool{false, true, false, true} {
		if present {
			parent.Append(child)
		} else {
			child.Remove()
		}
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		if state.SyncStats.PartialReconciles != 1 {
			t.Fatal("first/last clip fell back")
		}
		source := prefix + suffix
		if present {
			source = prefix + childSource + suffix
		}
		assertHTMLContextMatchesFresh(t, world, owner, source)
		if !present && (len(state.OverflowBindings) != 0 || len(state.OverflowEntities) != 0 || len(state.OverflowRoutes) != 0) {
			t.Fatal("last clip leaked routing state")
		}
	}
}

func TestHTMLStructureClipExtentAndPosition(t *testing.T) {
	const prefix = `<div id="scroller" style="overflow:auto;height:40px"><div id="parent" style="height:80px">`
	const childSource = `<div id="child" style="height:200px">Content</div>`
	const suffix = `</div></div><div>After</div>`
	world, owner := incrementalHTMLFixture(t, prefix+childSource+suffix)
	parent := incrementalHTMLNode(t, world, owner, "parent")
	child := incrementalHTMLNode(t, world, owner, "child")
	scroller := incrementalHTMLNode(t, world, owner, "scroller")
	initial, _ := world.Get[htmlRenderState](owner)
	clip := initial.OverflowBindings[scroller.node].entity
	scroll := world.GetMut[scene.UIScroll](clip)
	scroll.Position[1] = -150
	for _, present := range []bool{false, true, false, true} {
		if present {
			parent.Append(child)
		} else {
			child.Remove()
		}
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		scroll, _ := world.Get[scene.UIScroll](clip)
		if state.SyncStats.PartialReconciles != 1 || state.OverflowBindings[scroller.node].entity != clip || scroll.Position[1] != -40 {
			t.Fatalf("stable scroller state lost: stats=%+v clip=%v/%v position=%v", state.SyncStats, state.OverflowBindings[scroller.node].entity, clip, scroll.Position)
		}
		source := prefix + suffix
		if present {
			source = prefix + childSource + suffix
		}
		assertHTMLContextMatchesFresh(t, world, owner, source)
		expected := float32(80)
		if present {
			expected = 200
		}
		if scroll.ContentSize[1] != expected {
			t.Fatalf("content height=%v, want %v", scroll.ContentSize[1], expected)
		}
	}
}

func TestHTMLStructureMovesClipWithAbsoluteDescendant(t *testing.T) {
	const prefix = `<div id="parent"><div id="remove" style="height:30px">Before</div></div>`
	const suffix = `<div id="scroller" style="overflow:auto;height:60px"><div style="position:absolute;top:100px">Absolute</div><input value="Value"/></div>`
	world, owner := incrementalHTMLFixture(t, prefix+suffix)
	incrementalHTMLNode(t, world, owner, "remove").Remove()
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 {
		t.Fatal("moved clip fell back")
	}
	assertHTMLContextMatchesFresh(t, world, owner, `<div id="parent"></div>`+suffix)
}

func TestHTMLStructureClipSizeMembershipIsLocal(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroller" style="overflow:auto"><div id="child">Text</div></div>`)
	incrementalHTMLNode(t, world, owner, "child").Remove()
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 {
		t.Fatal("existing clip size membership fell back")
	}
	assertHTMLContextMatchesFresh(t, world, owner, `<div id="scroller" style="overflow:auto"></div>`)
}

func BenchmarkHTMLStructuralClipping(b *testing.B) {
	for _, count := range []int{10, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<div id="scroller" style="height:40px;overflow:auto"><div id="child" style="height:80px">Target</div></div>`+strings.Repeat(`<div style="height:40px;overflow:auto"><input value="Unchanged"/></div>`, count))
			parent := incrementalHTMLNode(b, world, owner, "scroller")
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

func TestHTMLFirstClipAfterUnclippedLayoutChanges(t *testing.T) {
	const clipSource = `<div id="clip" style="overflow:auto;height:40px"><div style="height:90px">Clipped</div></div>`
	world, owner := incrementalHTMLFixture(t, `<div id="parent" style="height:120px">`+clipSource+`</div><div id="resized" style="height:20px">After</div>`)
	clip := incrementalHTMLNode(t, world, owner, "clip")
	clip.Remove()
	incrementalSync(t, world)
	incrementalHTMLNode(t, world, owner, "resized").SetAttribute("style", "height:90px")
	incrementalSync(t, world)
	incrementalHTMLNode(t, world, owner, "parent").Append(clip)
	incrementalSync(t, world)
	assertHTMLContextMatchesFresh(t, world, owner, `<div id="parent" style="height:120px">`+clipSource+`</div><div id="resized" style="height:90px">After</div>`)
}

func TestHTMLStructuralClipFocusAndControls(t *testing.T) {
	for _, attributes := range []string{`tabindex="0"`, `contenteditable="true"`} {
		t.Run(attributes, func(t *testing.T) {
			prefix := `<div id="outer" style="overflow:auto;height:120px"><div id="parent" ` + attributes + `>`
			const childSource = `<div id="clip" style="overflow:auto;height:50px"><div>Text</div><input value="Value"/></div>`
			const suffix = `</div></div><div>After</div>`
			world, owner := incrementalHTMLFixture(t, prefix+childSource+suffix)
			child := incrementalHTMLNode(t, world, owner, "clip")
			parent := incrementalHTMLNode(t, world, owner, "parent")
			for _, present := range []bool{false, true} {
				if present {
					parent.Append(child)
				} else {
					child.Remove()
				}
				incrementalSync(t, world)
				state, _ := world.Get[htmlRenderState](owner)
				if state.SyncStats.PartialReconciles != 1 {
					t.Fatalf("clipped context fell back: %+v", state.SyncStats)
				}
				source := prefix + suffix
				if present {
					source = prefix + childSource + suffix
				}
				assertHTMLContextMatchesFresh(t, world, owner, source)
			}
		})
	}
}

func TestHTMLStructuralClippingSkipsUnrelatedControls(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroller" style="height:40px;overflow:auto"><div id="child" style="height:80px">Target</div></div>`+strings.Repeat(`<div style="height:40px;overflow:auto"><input value="Unchanged"/></div>`, 1000))
	incrementalHTMLNode(t, world, owner, "child").Remove()
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 || state.SyncStats.UpdatedClips != 1 || state.SyncStats.UpdatedControls != 0 || state.SyncStats.SnapshottedControls != 0 || state.SyncStats.ReconciledBoxes > 3 {
		t.Fatalf("unrelated clipped controls updated: %+v", state.SyncStats)
	}
}

func TestHTMLStructuralClipWithExternalBindingMatchesFull(t *testing.T) {
	const source = `<div id="parent" style="height:120px"><div id="clip" style="overflow:auto;height:40px"><input value="Value"/></div></div><div>After</div>`
	world, owner := incrementalHTMLFixture(t, source)
	reference, refOwner := incrementalHTMLFixture(t, source)
	for _, fixture := range []struct {
		world *ecs.World
		owner ecs.Entity
	}{{world, owner}, {reference, refOwner}} {
		area := htmlBindingArea(fixture.world, -3, -7)
		SetNodeOffset(fixture.world, fixture.owner, "parent", 5, 11)
		SetNodeScroll(fixture.world, fixture.owner, "parent", area, 13, 17)
	}
	child := incrementalHTMLNode(t, world, owner, "clip")
	refChild := incrementalHTMLNode(t, reference, refOwner, "clip")
	parent := incrementalHTMLNode(t, world, owner, "parent")
	refParent := incrementalHTMLNode(t, reference, refOwner, "parent")
	for _, present := range []bool{false, true, false, true} {
		if present {
			parent.Append(child)
			refParent.Append(refChild)
		} else {
			child.Remove()
			refChild.Remove()
		}
		refState := reference.GetMut[htmlRenderState](refOwner)
		refState.PartialValid = false
		incrementalSync(t, world)
		incrementalSync(t, reference)
		state, _ := world.Get[htmlRenderState](owner)
		want, _ := reference.Get[htmlRenderState](refOwner)
		if state.SyncStats.PartialReconciles != 1 || !equalHTMLPrimitiveSnapshots(htmlPrimitiveSnapshot(world, state), htmlPrimitiveSnapshot(reference, want)) {
			t.Fatal("external binding and structural clip differ from full rendering")
		}
	}
}
