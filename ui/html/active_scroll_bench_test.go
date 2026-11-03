package html

import (
	"fmt"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func BenchmarkHTMLAnimationActivePopulation(b *testing.B) {
	for _, count := range []int{0, 1000, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, owner := incrementalHTMLFixture(b, `<style>
 #moving {width:10px;height:10px;background:red;animation:move 1s linear infinite alternate}
 .idle {transition:transform 1s linear}
 @keyframes move {to{transform:translateX(100px)}}
 </style><div id="moving"></div><div class="idle" id="idle"></div>`)
			state := world.GetRef[htmlRenderState](owner)
			idle := incrementalHTMLNode(b, world, owner, "idle")
			for range count {
				node := dom.NewElement("div", nil)
				state.Root.AppendChild(node)
				prepareHTMLNodeAnimation(state, node, state.Styles[idle.node])
			}
			advanceHTMLAnimations(world, state, .016)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				advanceHTMLAnimations(world, state, .016)
				if !htmlAnimating(state) {
					b.Fatal("lost active animation")
				}
			}
		})
	}
}

func htmlScrollPopulation(count int) (*ecs.World, ecs.Entity) {
	world := ecs.NewWorld()
	areas := world.NewSpawner1[scene.UIScroll]()
	contents := world.NewSpawner2[ScrollContent, scene.UITransform]()
	var first ecs.Entity
	for range 100 {
		area := areas.Spawn(scene.UIScroll{ViewportSize: mgl32.Vec2{100, 100}, ContentSize: mgl32.Vec2{100, 1000}})
		if first == 0 {
			first = area
		}
		for i := range count / 100 {
			contents.Spawn(ScrollContent{Area: area, Y: float32(i)}, scene.NewUITransform())
		}
	}
	SyncScrollContent(world)
	return world, first
}

func BenchmarkHTMLScrollContentPopulation(b *testing.B) {
	for _, moving := range []bool{false, true} {
		b.Run(fmt.Sprintf("moving=%v", moving), func(b *testing.B) {
			world, area := htmlScrollPopulation(10000)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if moving {
					scroll := world.GetMut[scene.UIScroll](area)
					scroll.Position[1] = -100 - scroll.Position[1]
				}
				SyncScrollContent(world)
			}
		})
	}
}

func BenchmarkHTMLScrollbarPopulation(b *testing.B) {
	world := ecs.NewWorld()
	areas := world.NewSpawner1[scene.UIScroll]()
	parts := world.NewSpawner4[ScrollbarPart, scene.UITransform, scene.Size2D, scene.Sprite2D]()
	for range 1000 {
		area := areas.Spawn(scene.UIScroll{
			ViewportSize: mgl32.Vec2{100, 100}, ContentSize: mgl32.Vec2{1000, 1000},
			Horizontal: true, Vertical: true, ScrollbarSize: 10,
		})
		for axis := range uint8(2) {
			for kind := range uint8(2) {
				parts.Spawn(ScrollbarPart{Area: area, Axis: axis, Kind: kind}, scene.NewUITransform(), scene.Size2D{}, scene.NewSprite2D())
			}
		}
	}
	SyncScrollbars(world)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		SyncScrollbars(world)
	}
}
