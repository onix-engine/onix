package html

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/scene"
)

func TestHTMLStackChangesMatchFresh(t *testing.T) {
	for _, parent := range []string{"position:relative", "display:flex", "position:relative;z-index:3"} {
		t.Run(parent, func(t *testing.T) {
			source := func(z, inner string) string {
				return `<div style="` + parent + `"><div id="target" style="position:relative;z-index:` + z + `;background:red"><div id="clip" style="overflow:auto;height:40px"><div id="inner" style="position:relative;z-index:` + inner + `"><input id="field" value="Value"/><div style="height:100px">Text</div></div></div></div><div style="position:relative;z-index:2;background:blue">Sibling</div></div>`
			}
			world, owner := incrementalHTMLFixture(t, source("auto", "-1"))
			target := incrementalHTMLNode(t, world, owner, "target")
			inner := incrementalHTMLNode(t, world, owner, "inner")
			clip := incrementalHTMLNode(t, world, owner, "clip")
			control := incrementalControl(t, world, owner, "field")
			state, _ := world.Get[htmlRenderState](owner)
			clipEntity := state.OverflowBindings[clip.node].entity
			scroll := world.GetMut[scene.UIScroll](clipEntity)
			scroll.Position[1] = -10
			before, _ := world.Get[Control](control)
			for _, step := range []struct{ z, inner string }{{"4", "-1"}, {"-2", "-1"}, {"auto", "-1"}, {"2", "999999"}, {"-999999", "-999999"}, {"auto", "auto"}} {
				target.SetAttribute("style", "position:relative;z-index:"+step.z+";background:red")
				inner.SetAttribute("style", "position:relative;z-index:"+step.inner)
				incrementalSync(t, world)
				state, _ = world.Get[htmlRenderState](owner)
				if state.SyncStats.PartialReconciles != 1 || state.SyncStats.Spawned != 0 || state.SyncStats.Despawned != 0 || state.SyncStats.UpdatedControls != 0 {
					t.Fatalf("nonlocal stack update: %+v", state.SyncStats)
				}
				after, _ := world.Get[Control](control)
				if after.RenderRevision != before.RenderRevision {
					t.Fatal("descendant control rebuilt")
				}
				scroll, _ := world.Get[scene.UIScroll](clipEntity)
				if state.OverflowBindings[clip.node].entity != clipEntity || scroll.Position[1] != -10 {
					t.Fatal("clip state reset")
				}
				assertHTMLContextMatchesFresh(t, world, owner, source(step.z, step.inner))
			}
		})
	}
}

func TestHTMLStackChangeWithStructureMatchesFresh(t *testing.T) {
	source := func(z, child string) string {
		return `<div id="target" style="position:relative;z-index:` + z + `">` + child + `<div style="position:relative;z-index:4"><input value="Value"/></div></div><div>Sibling</div>`
	}
	world, owner := incrementalHTMLFixture(t, source("1", `<div id="removed">Remove</div>`))
	incrementalHTMLNode(t, world, owner, "removed").Remove()
	incrementalHTMLNode(t, world, owner, "target").SetAttribute("style", "position:relative;z-index:-1")
	incrementalSync(t, world)
	state, _ := world.Get[htmlRenderState](owner)
	if state.SyncStats.PartialReconciles != 1 {
		t.Fatal("combined stack and structure change fell back")
	}
	assertHTMLPartialMatchesFresh(t, world, owner, source("-1", ""))
}

func BenchmarkHTMLStackChange(b *testing.B) {
	for _, count := range []int{10, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<style>#target{position:relative;z-index:1}#target.changed{z-index:-1}</style><div id="target"><div><input value="Value"/></div></div>`+strings.Repeat(`<div><input value="Unchanged"/></div>`, count))
			target := incrementalHTMLNode(b, world, owner, "target")
			for _, class := range []string{"changed", "", "changed", ""} {
				target.SetAttribute("class", class)
				incrementalSync(b, world)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if i%2 == 0 {
					target.SetAttribute("class", "changed")
				} else {
					target.SetAttribute("class", "")
				}
				incrementalSync(b, world)
			}
		})
	}
}

func TestHTMLStaticAndFlexItemStackChangesMatchFresh(t *testing.T) {
	for _, display := range []string{"block", "flex"} {
		t.Run(display, func(t *testing.T) {
			source := func(z string) string {
				return `<div style="display:` + display + `"><div id="target" style="z-index:` + z + `;background:red"><div style="position:relative;z-index:3"><input value="Value"/></div></div><div style="position:relative;z-index:2">Sibling</div></div>`
			}
			world, owner := incrementalHTMLFixture(t, source("auto"))
			target := incrementalHTMLNode(t, world, owner, "target")
			for _, z := range []string{"-1", "4", "auto"} {
				target.SetAttribute("style", "z-index:"+z+";background:red")
				incrementalSync(t, world)
				state, _ := world.Get[htmlRenderState](owner)
				if state.SyncStats.PartialReconciles != 1 || state.SyncStats.UpdatedControls != 0 {
					t.Fatalf("unexpected reconciliation: %+v", state.SyncStats)
				}
				if display == "block" && state.SyncStats.OrderUpdated != 0 {
					t.Fatal("static block changed descendant stacking")
				}
				assertHTMLPartialMatchesFresh(t, world, owner, source(z))
			}
		})
	}
}
