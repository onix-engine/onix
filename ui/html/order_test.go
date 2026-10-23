package html

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestHTMLOrderChangesMatchFresh(t *testing.T) {
	for _, direction := range []string{"row", "row-reverse", "column", "column-reverse"} {
		t.Run(direction, func(t *testing.T) {
			source := func(order, extra string) string {
				return `<div style="height:240px"><div id="flex" style="display:flex;flex-direction:` + direction + `;flex-wrap:wrap;width:180px;height:160px"><div id="target" tabindex="0" style="width:80px;height:60px;overflow:auto;order:` + order + `;` + extra + `"><div style="height:100px"><input id="field" value="Value"/></div></div><div id="second" style="width:80px;height:60px;order:0"><input value="Second"/></div><div style="width:80px;height:60px;order:1">Third</div></div></div><div><input id="other" value="Unchanged"/></div>`
			}
			world, owner := incrementalHTMLFixture(t, source("0", ""))
			node := incrementalHTMLNode(t, world, owner, "target")
			field := incrementalControl(t, world, owner, "field")
			other := incrementalControl(t, world, owner, "other")
			before, _ := ecs.Get[Control](world, other)
			for _, step := range []struct{ order, extra string }{{"2", ""}, {"-1", ""}, {"0", ""}, {"2", "width:70px;color:blue"}, {"-2", "overflow:hidden"}, {"0", ""}} {
				node.SetAttribute("style", "width:80px;height:60px;overflow:auto;order:"+step.order+";"+step.extra)
				incrementalSync(t, world)
				state, _ := ecs.Get[htmlRenderState](world, owner)
				if state.SyncStats.PartialReconciles != 1 {
					t.Fatalf("order fell back: %+v", state.SyncStats)
				}
				if incrementalControl(t, world, owner, "field") != field {
					t.Fatal("input replaced")
				}
				after, _ := ecs.Get[Control](world, other)
				if after.RenderRevision != before.RenderRevision {
					t.Fatal("unrelated input rebuilt")
				}
				assertHTMLContextMatchesFresh(t, world, owner, source(step.order, step.extra))
				for _, entity := range state.Children {
					if focus, ok := ecs.Get[Focusable](world, entity); ok && focus.Order != htmlEntityNode(world, entity).Order {
						t.Fatal("CSS order changed DOM focus order")
					}
				}
			}
		})
	}
}

func TestHTMLOrderWithStructureAndTreeFallbacks(t *testing.T) {
	const prefix = `<div id="parent" style="display:flex"><div id="target" style="order:0">First</div><div id="remove">Remove</div><div>Last</div></div>`
	for _, extra := range []string{"", "position:absolute", "display:none"} {
		t.Run(extra, func(t *testing.T) {
			world, owner := incrementalHTMLFixture(t, prefix)
			incrementalHTMLNode(t, world, owner, "target").SetAttribute("style", "order:2;"+extra)
			incrementalHTMLNode(t, world, owner, "remove").Remove()
			incrementalSync(t, world)
			state, _ := ecs.Get[htmlRenderState](world, owner)
			want := 0
			if extra == "" {
				want = 1
			}
			if state.SyncStats.PartialReconciles != want {
				t.Fatalf("wrong tree preflight: %+v", state.SyncStats)
			}
			assertHTMLPartialMatchesFresh(t, world, owner, `<div id="parent" style="display:flex"><div id="target" style="order:2;`+extra+`">First</div><div>Last</div></div>`)
		})
	}
}

func TestHTMLOrderSkipsUnrelatedControls(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div style="height:100px;display:flex"><div id="target" style="order:0"><input value="Value"/></div><div><input value="Second"/></div></div>`+strings.Repeat(`<div><input value="Unchanged"/></div>`, 1000))
	incrementalHTMLNode(t, world, owner, "target").SetAttribute("style", "order:1")
	incrementalSync(t, world)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	if state.SyncStats.PartialReconciles != 1 || state.SyncStats.UpdatedControls > 2 || state.SyncStats.SnapshottedControls > 2 || state.SyncStats.LayoutWork.TopologyVisited != 0 || state.SyncStats.LayoutWork.OrderContainers != 1 || state.SyncStats.LayoutWork.LocalFlexReflows != 1 || state.SyncStats.LayoutWork.Visited > 10 {
		t.Fatalf("order updated unrelated inputs: %+v", state.SyncStats)
	}
	assertHTMLPartialMatchesFresh(t, world, owner, `<div style="height:100px;display:flex"><div id="target" style="order:1"><input value="Value"/></div><div><input value="Second"/></div></div>`+strings.Repeat(`<div><input value="Unchanged"/></div>`, 1000))
}

func BenchmarkHTMLOrder(b *testing.B) {
	for _, count := range []int{10, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<style>#target{order:0}#target.changed{order:1}</style><div style="height:100px;display:flex"><div id="target"><input value="Value"/></div><div><input value="Second"/></div></div>`+strings.Repeat(`<div><input value="Unchanged"/></div>`, count))
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

func TestHTMLOrderOnBlockAndTreeChanges(t *testing.T) {
	for _, extra := range []string{"", "position:relative", "display:flex"} {
		t.Run(extra, func(t *testing.T) {
			source := func(style string) string {
				return `<div><div id="target" style="` + style + `"><input value="Value"/></div><div>Sibling</div></div>`
			}
			world, owner := incrementalHTMLFixture(t, source("order:0"))
			incrementalHTMLNode(t, world, owner, "target").SetAttribute("style", "order:2;"+extra)
			incrementalSync(t, world)
			state, _ := ecs.Get[htmlRenderState](world, owner)
			want := 0
			if extra == "" {
				want = 1
			}
			if state.SyncStats.PartialReconciles != want {
				t.Fatalf("unexpected path: %+v", state.SyncStats)
			}
			assertHTMLPartialMatchesFresh(t, world, owner, source("order:2;"+extra))
		})
	}
}

func TestHTMLOrderKeepsInputAndScrollState(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div style="display:flex"><div id="target" style="height:40px;width:120px;overflow:auto;order:0"><div style="height:100px"><input id="field" value="Value"/></div></div><div style="width:120px">Sibling</div></div>`)
	node := incrementalHTMLNode(t, world, owner, "target")
	field := incrementalControl(t, world, owner, "field")
	SetControlValue(world, field, "Typed")
	control := ecs.GetMut[Control](world, field)
	control.SelectionAnchor, control.SelectionFocus = 1, 3
	state, _ := ecs.Get[htmlRenderState](world, owner)
	clip := state.OverflowBindings[node.node].entity
	scroll := ecs.GetMut[scene.UIScroll](world, clip)
	scroll.Position[1] = -10
	for _, order := range []string{"1", "-1", "0"} {
		node.SetAttribute("style", "height:40px;width:120px;overflow:auto;order:"+order)
		incrementalSync(t, world)
		state, _ = ecs.Get[htmlRenderState](world, owner)
		got, _ := ecs.Get[Control](world, field)
		position, _ := ecs.Get[scene.UIScroll](world, clip)
		if state.SyncStats.PartialReconciles != 1 || state.OverflowBindings[node.node].entity != clip || position.Position[1] != -10 || got.Value != "Typed" || got.SelectionAnchor != 1 || got.SelectionFocus != 3 {
			t.Fatal("order change reset live state")
		}
	}
}

func TestHTMLWarmOrderDoesNotAllocate(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>#target{order:0}#target.changed{order:1}</style><div style="height:100px;display:flex"><div id="target"><input value="Value"/></div><div><input value="Second"/></div></div>`)
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
	if allocations := testing.AllocsPerRun(100, cycle); allocations != 0 {
		t.Fatalf("warm order allocated %g times per round trip", allocations)
	}
}

func TestHTMLNestedLocalOrderMatchesFresh(t *testing.T) {
	source := func(order string) string {
		return `<div style="display:flex;height:200px"><div id="outer" style="display:flex;order:` + order + `"><div id="inner" style="order:` + order + `"><input value="Value"/></div><div>Inner sibling</div></div><div>Outer sibling</div></div>`
	}
	world, owner := incrementalHTMLFixture(t, source("0"))
	outer := incrementalHTMLNode(t, world, owner, "outer")
	inner := incrementalHTMLNode(t, world, owner, "inner")
	for _, order := range []string{"1", "0", "-1", "0"} {
		outer.SetAttribute("style", "display:flex;order:"+order)
		inner.SetAttribute("style", "order:"+order)
		incrementalSync(t, world)
		state, _ := ecs.Get[htmlRenderState](world, owner)
		if state.SyncStats.PartialReconciles != 1 || state.SyncStats.LayoutWork.TopologyVisited != 0 || state.SyncStats.LayoutWork.OrderContainers != 2 {
			t.Fatalf("nested order fell back: %+v", state.SyncStats)
		}
		assertHTMLPartialMatchesFresh(t, world, owner, source(order))
	}
	// A later paint-only sync must not retain the previous reorder worklist.
	inner.SetAttribute("style", "order:0;color:blue")
	incrementalSync(t, world)
	state, _ := ecs.Get[htmlRenderState](world, owner)
	if len(state.LayoutCache.Reordered) != 0 || len(state.LayoutOrderNodes) != 0 {
		t.Fatal("reorder scratch survived sync")
	}
}
