package html

import (
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

func BenchmarkHTMLStaticAnimationPreparation(b *testing.B) {
	node := dom.NewElement("div", nil)
	styles, err := style.Compute(node, nil)
	if err != nil {
		b.Fatal(err)
	}
	state := htmlRenderState{htmlStyleState: htmlStyleState{Styles: styles}}
	prepareHTMLNodeAnimation(&state, node, styles[node])
	b.ReportAllocs()
	for b.Loop() {
		delete(state.Animations, node)
		prepareHTMLNodeAnimation(&state, node, styles[node])
	}
}

func BenchmarkHTMLFullRenderStylesheetReuse(b *testing.B) {
	world, owner := incrementalHTMLFixture(b, `<div style="width:20px;height:20px;background:red"></div>`)
	value := ecs.GetMut[Surface](world, owner)
	value.CSS = strings.Repeat(`.unused {color:#123456}`, 800)
	incrementalSync(b, world)
	update := func() {
		surface := ecs.GetMut[Surface](world, owner)
		if surface.Width == 320 {
			surface.Width = 321
		} else {
			surface.Width = 320
		}
		if err := Sync(world); err != nil {
			b.Fatal(err)
		}
	}
	update()
	update()
	b.ReportAllocs()
	for b.Loop() {
		update()
	}
}

func BenchmarkHTMLHoverStyleReuse(b *testing.B) {
	world, owner := incrementalHTMLFixture(b, `<style>#target {width:20px;height:20px;background:red} #target:hover {background:blue;color:green}</style><button id="target">Hover</button>`)
	target := incrementalHTMLNode(b, world, owner, "target")
	state := ecs.GetRef[htmlRenderState](world, owner)
	visual := state.RenderNodes[target.node].Entities[0]
	update := func() {
		for _, hovered := range []bool{true, false} {
			if !SetVisualHover(world, visual, hovered) {
				b.Fatal("hover failed")
			}
			if err := Sync(world); err != nil {
				b.Fatal(err)
			}
		}
	}
	update()
	b.ReportAllocs()
	for b.Loop() {
		update()
	}
}
