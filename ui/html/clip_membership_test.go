package html

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestHTMLClipMembershipMatchesFresh(t *testing.T) {
	for _, mode := range []string{"auto", "hidden", "scroll", "clip"} {
		for _, nested := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", mode, nested), func(t *testing.T) {
				prefix := `<div style="height:200px">`
				suffix := `</div><div><input id="other" value="Unchanged"/></div>`
				if nested {
					prefix = `<div id="outer" style="height:200px;overflow:auto">`
				}
				source := func(size string) string {
					return prefix + `<div id="target" style="position:relative;overflow:` + mode + `;` + size + `"><div id="inner" style="overflow:auto;height:30px"><input value="Value"/></div><div style="position:absolute;top:100px;width:80px">Absolute</div><span> </span></div>` + suffix
				}
				world, owner := incrementalHTMLFixture(t, source("width:120px;height:40px"))
				target := incrementalHTMLNode(t, world, owner, "target")
				other := incrementalControl(t, world, owner, "other")
				before, _ := world.Get[Control](other)
				for _, size := range []string{"width:120px;height:0px", "width:120px;height:40px", "width:0px;height:40px", "width:120px;height:40px", "width:0px;height:0px", "width:120px;height:40px"} {
					target.SetAttribute("style", "position:relative;overflow:"+mode+";"+size)
					incrementalSync(t, world)
					state, _ := world.Get[htmlRenderState](owner)
					if state.SyncStats.PartialReconciles != 1 {
						t.Fatalf("membership fell back: %+v", state.SyncStats)
					}
					after, _ := world.Get[Control](other)
					if after.RenderRevision != before.RenderRevision {
						t.Fatal("unrelated control rebuilt")
					}
					assertHTMLContextMatchesFresh(t, world, owner, source(size))
					for _, entity := range state.Children {
						if parent, ok := world.Get[scene.Parent](entity); ok && !world.IsAlive(parent.Entity) {
							t.Fatal("visual parent is retired")
						}
					}
					for _, route := range state.OverflowRoutes {
						if !world.IsAlive(route.entity) {
							t.Fatal("route refers to retired clip")
						}
					}
					for entity := range state.OverflowEntities {
						if !world.IsAlive(entity) {
							t.Fatal("retired clip retained in index")
						}
					}
				}
			})
		}
	}
}

func TestHTMLNestedClipMembershipChangesTogether(t *testing.T) {
	const css = `<style>.zero{height:0}.open{height:40px}</style>`
	source := func(class string) string {
		return css + `<div style="height:120px"><div id="outer" class="` + class + `" style="overflow:auto"><div id="inner" class="` + class + `" style="overflow:hidden"><div><input value="Value"/></div><div>Text</div></div></div></div>`
	}
	world, owner := incrementalHTMLFixture(t, source("open"))
	outer := incrementalHTMLNode(t, world, owner, "outer")
	inner := incrementalHTMLNode(t, world, owner, "inner")
	for _, class := range []string{"zero", "open", "zero", "open"} {
		outer.SetAttribute("class", class)
		inner.SetAttribute("class", class)
		incrementalSync(t, world)
		state, _ := world.Get[htmlRenderState](owner)
		if state.SyncStats.PartialReconciles != 1 {
			t.Fatal("nested membership fell back")
		}
		assertHTMLContextMatchesFresh(t, world, owner, source(class))
		if class == "zero" && (len(state.OverflowRoutes) != 0 || len(state.OverflowBindings) != 0) {
			t.Fatal("last clipping contexts left stale routes")
		}
	}
}

func TestHTMLClipMembershipFocusAndEditingContexts(t *testing.T) {
	for _, attributes := range []string{`tabindex="0"`, `contenteditable="true"`} {
		t.Run(attributes, func(t *testing.T) {
			source := func(height string) string {
				return `<div id="outer" style="overflow:auto;height:160px"><div id="target" ` + attributes + ` style="overflow:auto;height:` + height + `"><div>Focusable text</div><input value="Value"/></div></div>`
			}
			world, owner := incrementalHTMLFixture(t, source("40px"))
			target := incrementalHTMLNode(t, world, owner, "target")
			for _, height := range []string{"0px", "40px", "0px", "40px"} {
				target.SetAttribute("style", "overflow:auto;height:"+height)
				incrementalSync(t, world)
				state, _ := world.Get[htmlRenderState](owner)
				if state.SyncStats.PartialReconciles != 1 {
					t.Fatal("focus clip transition fell back")
				}
				assertHTMLContextMatchesFresh(t, world, owner, source(height))
			}
		})
	}
}

func BenchmarkHTMLClipMembership(b *testing.B) {
	for _, count := range []int{10, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<style>#target{height:40px;overflow:auto}#target.zero{height:0}</style><div style="height:120px"><div id="target"><div><input value="Value"/></div></div></div>`+strings.Repeat(`<div style="overflow:auto;height:40px"><input value="Unchanged"/></div>`, count))
			target := incrementalHTMLNode(b, world, owner, "target")
			for _, class := range []string{"zero", "", "zero", ""} {
				target.SetAttribute("class", class)
				incrementalSync(b, world)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if i%2 == 0 {
					target.SetAttribute("class", "zero")
				} else {
					target.SetAttribute("class", "")
				}
				incrementalSync(b, world)
			}
		})
	}
}

func TestHTMLClipMembershipUsesPaddingBox(t *testing.T) {
	source := func(css string) string {
		return `<div id="clip" style="overflow:auto;` + css + `"><div>Text</div></div>`
	}
	world, owner := incrementalHTMLFixture(t, source("height:40px;padding:5px"))
	node := incrementalHTMLNode(t, world, owner, "clip")
	state, _ := world.Get[htmlRenderState](owner)
	initial := state.OverflowBindings[node.node].entity
	for index, css := range []string{"height:0px;padding:5px", "height:0px;padding:0px", "height:0px;padding:5px"} {
		node.SetAttribute("style", "overflow:auto;"+css)
		incrementalSync(t, world)
		state, _ = world.Get[htmlRenderState](owner)
		if state.SyncStats.PartialReconciles != 1 {
			t.Fatal("padding transition fell back")
		}
		if index == 0 && (!world.IsAlive(initial) || state.OverflowBindings[node.node].entity != initial) {
			t.Fatal("positive padding box lost retained clip")
		}
		assertHTMLContextMatchesFresh(t, world, owner, source(css))
	}
}

func TestHTMLClipMembershipWithExternalBindingsMatchesFull(t *testing.T) {
	const source = `<div id="parent" style="height:120px"><div id="clip" style="overflow:auto;height:40px"><input value="Value"/></div></div>`
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
	target := incrementalHTMLNode(t, world, owner, "clip")
	refTarget := incrementalHTMLNode(t, reference, refOwner, "clip")
	for _, height := range []string{"0px", "40px", "0px", "40px"} {
		target.SetAttribute("style", "overflow:auto;height:"+height)
		refTarget.SetAttribute("style", "overflow:auto;height:"+height)
		refState := reference.GetMut[htmlRenderState](refOwner)
		refState.PartialValid = false
		incrementalSync(t, world)
		incrementalSync(t, reference)
		state, _ := world.Get[htmlRenderState](owner)
		want, _ := reference.Get[htmlRenderState](refOwner)
		if state.SyncStats.PartialReconciles != 1 || !equalHTMLPrimitiveSnapshots(htmlPrimitiveSnapshot(world, state), htmlPrimitiveSnapshot(reference, want)) {
			t.Fatal("bound clip transition differs from full render")
		}
	}
}

func TestHTMLClipMembershipSkipsUnrelatedControls(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div style="height:120px"><div id="target" style="overflow:auto;height:40px"><div><input value="Value"/></div></div></div>`+strings.Repeat(`<div style="overflow:auto;height:40px"><input value="Unchanged"/></div>`, 1000))
	incrementalHTMLNode(t, world, owner, "target").SetAttribute("style", "overflow:auto;height:0px")
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 || state.SyncStats.UpdatedControls != 1 || state.SyncStats.SnapshottedControls != 1 || state.SyncStats.UpdatedClips != 0 {
		t.Fatalf("unrelated controls or clips rebuilt: %+v", state.SyncStats)
	}
}

func TestHTMLClipMembershipKeepsNestedScrollState(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div style="height:160px"><div id="target" style="overflow:auto;height:40px"><div id="inner" style="overflow:auto;height:30px"><div style="height:100px">Text</div></div></div></div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	inner := incrementalHTMLNode(t, world, owner, "inner")
	state, _ := world.Get[htmlRenderState](owner)
	entity := state.OverflowBindings[inner.node].entity
	scroll := world.GetMut[scene.UIScroll](entity)
	scroll.Position[1] = -15
	for _, height := range []string{"0px", "40px", "0px", "40px"} {
		target.SetAttribute("style", "overflow:auto;height:"+height)
		incrementalSync(t, world)
		state, _ = world.Get[htmlRenderState](owner)
		scroll, _ := world.Get[scene.UIScroll](entity)
		if state.OverflowBindings[inner.node].entity != entity || scroll.Position[1] != -15 {
			t.Fatal("surviving nested clip lost scroll state")
		}
		parent, _ := world.Get[scene.Parent](entity)
		expected := owner
		if height == "40px" {
			expected = state.OverflowBindings[target.node].entity
		}
		if parent.Entity != expected {
			t.Fatal("nested clip was not reparented")
		}
	}
}
