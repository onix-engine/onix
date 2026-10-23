package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/style"
)

func TestHTMLFullCascadeReusesStylesAndReleasesRemovedNodes(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target" style="color:red">First</div>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	state := ecs.GetRef[htmlRenderState](world, owner)
	initial := state.Styles[target.node]
	// A viewport change forces the full cascade, without changing this style.
	ecs.GetMut[Surface](world, owner).Width += 100
	incrementalSync(t, world)
	state = ecs.GetRef[htmlRenderState](world, owner)
	if state.Styles[target.node] != initial {
		t.Fatal("full viewport update replaced an unchanged style")
	}
	oldRoot := state.Root
	ecs.GetMut[Surface](world, owner).Value = `<div id="replacement" style="color:red">Second</div>`
	incrementalSync(t, world)
	replacement := incrementalHTMLNode(t, world, owner, "replacement")
	state = ecs.GetRef[htmlRenderState](world, owner)
	if state.Styles[replacement.node] != initial {
		t.Fatal("replacement DOM did not share its equal immutable style")
	}
	if state.Styles[oldRoot] != nil || state.Styles[target.node] != nil {
		t.Fatal("replacement DOM retained detached nodes in the style map")
	}
	replacement.SetAttribute("style", "color:blue")
	incrementalSync(t, world)
	state = ecs.GetRef[htmlRenderState](world, owner)
	if state.Styles[replacement.node].Color != style.ColorValue(style.Color{B: 255, A: 255}) || initial.Color != style.ColorValue(style.Color{R: 255, A: 255}) {
		t.Fatal("incremental restyle changed a shared cached instance or kept stale color")
	}
}

func TestHTMLStylesheetCacheInvalidation(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target">Text</div>`)
	surface := ecs.GetMut[Surface](world, owner)
	surface.CSS = `#target {width:2em;color:red}`
	incrementalSync(t, world)
	state := ecs.GetRef[htmlRenderState](world, owner)
	sheet := state.Stylesheet
	ecs.GetMut[Surface](world, owner).Width++
	incrementalSync(t, world)
	if state.Stylesheet != sheet {
		t.Fatal("viewport change recompiled an identical stylesheet")
	}
	ecs.GetMut[Surface](world, owner).FontSize = 24
	incrementalSync(t, world)
	target := incrementalHTMLNode(t, world, owner, "target")
	if state.Stylesheet == sheet || state.Styles[target.node].Width != style.LengthPx(48) {
		t.Fatal("root font size failed to invalidate stylesheet")
	}
	sheet = state.Stylesheet
	ecs.GetMut[Surface](world, owner).CSS = `#target {color:blue}`
	incrementalSync(t, world)
	if state.Stylesheet == sheet || state.Styles[target.node].Color != style.ColorValue(style.Color{B: 255, A: 255}) {
		t.Fatal("external CSS change failed to invalidate stylesheet")
	}
	sheet = state.Stylesheet
	ecs.GetMut[Surface](world, owner).Value = `<style>#target {color:green}</style><div id="target">Replacement</div>`
	incrementalSync(t, world)
	target = incrementalHTMLNode(t, world, owner, "target")
	if state.Stylesheet == sheet || state.Styles[target.node].Color != style.ColorValue(style.Color{G: 128, A: 255}) {
		t.Fatal("embedded CSS change failed to invalidate stylesheet")
	}
}

func TestHTMLHoverStylesWarmAllocations(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>#target {width:20px;height:20px;background:red} #target:hover {background:blue;color:green}</style><button id="target">Hover</button>`)
	target := incrementalHTMLNode(t, world, owner, "target")
	state := ecs.GetRef[htmlRenderState](world, owner)
	initial := state.Styles[target.node]
	visual := state.RenderNodes[target.node].Entities[0]
	if allocs := testing.AllocsPerRun(100, func() {
		for _, hovered := range []bool{true, false} {
			if !SetVisualHover(world, visual, hovered) {
				panic("hover failed")
			}
			if err := Sync(world); err != nil {
				panic(err)
			}
		}
	}); allocs != 0 {
		t.Fatalf("warm hover cycle allocated %g times", allocs)
	}
	if state.Styles[target.node] != initial {
		t.Fatal("leaving hover did not restore the cached immutable style")
	}
}
