package html

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
)

func multiFlexOrderSource(order string, count int) string {
	return `<div style="height:240px"><div style="display:flex;height:80px"><div id="a" style="order:` + order + `"><input value="A"/></div><div><input value="B"/></div></div><div style="display:flex;height:80px"><div id="b" style="order:` + order + `"><input value="C"/></div><div><input value="D"/></div></div></div>` + strings.Repeat(`<div><input value="Unchanged"/></div>`, count)
}

func TestHTMLMultipleLocalFlexReflows(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, multiFlexOrderSource("0", 1000))
	a := incrementalHTMLNode(t, world, owner, "a")
	b := incrementalHTMLNode(t, world, owner, "b")
	for _, order := range []string{"1", "0", "1", "0"} {
		a.SetAttribute("style", "order:"+order)
		b.SetAttribute("style", "order:"+order)
		incrementalSync(t, world)
		state, _ := ecs.Get[htmlRenderState](world, owner)
		// Reused measurements may retain native controls instead of emitting
		// them again. Geometry/state must still match the fresh tree below.
		if state.SyncStats.LayoutWork.LocalFlexReflows != 2 || state.SyncStats.LayoutWork.Visited > 20 || state.SyncStats.UpdatedControls > 4 {
			t.Fatalf("nonlocal multi reflow: %+v", state.SyncStats)
		}
		assertHTMLOwnerSpans(t, state)
		assertHTMLPartialMatchesFresh(t, world, owner, multiFlexOrderSource(order, 1000))
	}
	cycle := func() {
		a.SetAttribute("style", "order:1")
		b.SetAttribute("style", "order:1")
		incrementalSync(t, world)
		a.SetAttribute("style", "order:0")
		b.SetAttribute("style", "order:0")
		incrementalSync(t, world)
	}
	// One interned instance per newly distinct order style; both cycle
	// directions restyle the two items.
	if allocations := testing.AllocsPerRun(100, cycle); allocations > 2 {
		t.Fatalf("warm multi order allocated %g times", allocations)
	}
}

func TestHTMLNestedFlexReflowCoalescesRoots(t *testing.T) {
	source := func(order string) string {
		return `<div style="display:flex;height:100px"><div id="outer" style="display:flex;order:` + order + `"><div id="inner" style="order:` + order + `"><input value="Value"/></div><div>Inner sibling</div></div><div>Outer sibling</div></div>`
	}
	world, owner := incrementalHTMLFixture(t, source("0"))
	outer := incrementalHTMLNode(t, world, owner, "outer")
	inner := incrementalHTMLNode(t, world, owner, "inner")
	for _, order := range []string{"1", "0", "1", "0"} {
		inner.SetAttribute("style", "order:"+order)
		outer.SetAttribute("style", "display:flex;order:"+order)
		incrementalSync(t, world)
		state, _ := ecs.Get[htmlRenderState](world, owner)
		if state.SyncStats.LayoutWork.LocalFlexReflows != 1 || state.SyncStats.LayoutWork.OrderContainers != 2 {
			t.Fatalf("nested roots not coalesced: %+v", state.SyncStats)
		}
		assertHTMLPartialMatchesFresh(t, world, owner, source(order))
		assertHTMLOwnerSpans(t, state)
	}
}

func TestHTMLMultipleFlexReflowGeometryFallback(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			source := func(order string) string {
				stable := `<div style="display:flex;height:80px"><div id="stable" style="order:` + order + `">Stable</div><div>Sibling</div></div>`
				changing := `<div style="display:flex;flex-wrap:wrap;width:100px"><div style="width:60px;height:100px">A</div><div style="width:60px;height:10px">B</div><div id="changing" style="width:40px;height:100px;order:` + order + `">C</div></div>`
				if reverse {
					return changing + stable + `<div><input value="Following"/></div>`
				}
				return stable + changing + `<div><input value="Following"/></div>`
			}
			world, owner := incrementalHTMLFixture(t, source("0"))
			stable := incrementalHTMLNode(t, world, owner, "stable")
			changing := incrementalHTMLNode(t, world, owner, "changing")
			for _, order := range []string{"-1", "1", "-1", "1"} {
				stable.SetAttribute("style", "order:"+order)
				changing.SetAttribute("style", "width:40px;height:100px;order:"+order)
				incrementalSync(t, world)
				state, _ := ecs.Get[htmlRenderState](world, owner)
				if state.SyncStats.LayoutWork.LocalFlexReflows != 0 {
					t.Fatal("partial success cleared ancestor invalidation")
				}
				assertHTMLPartialMatchesFresh(t, world, owner, source(order))
				assertHTMLOwnerSpans(t, state)
			}
		})
	}
}

func BenchmarkHTMLMultiOrder(b *testing.B) {
	for _, count := range []int{10, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, multiFlexOrderSource("0", count))
			a := incrementalHTMLNode(b, world, owner, "a")
			targetB := incrementalHTMLNode(b, world, owner, "b")
			for _, order := range []string{"1", "0", "1", "0"} {
				a.SetAttribute("style", "order:"+order)
				targetB.SetAttribute("style", "order:"+order)
				incrementalSync(b, world)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				order := "order:0"
				if i%2 == 0 {
					order = "order:1"
				}
				a.SetAttribute("style", order)
				targetB.SetAttribute("style", order)
				incrementalSync(b, world)
			}
		})
	}
}
