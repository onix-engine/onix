package html

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestHTMLAnimationComposesAcrossGeneratedContainers(t *testing.T) {
	for _, container := range []struct{ name, tag, extra string }{
		{"plain", "div", ""},
		{"clip", "div", "overflow:hidden"},
		{"scroll", "div", "overflow:auto"},
		{"control", "button", "border:0;padding:0"},
	} {
		for _, scale := range []float32{0, 2} {
			t.Run(fmt.Sprintf("%s/scale%g", container.name, scale), func(t *testing.T) {
				world, owner, clock := htmlAnimationFixture(t, fmt.Sprintf(`<style>
 #root {position:absolute;left:40px;top:30px;width:100px;height:100px;transform-origin:0 0;animation:turn 1s linear both}
 #container {position:absolute;left:20px;top:10px;width:60px;height:60px;transform:scale(%g,1);transform-origin:0 0;%s}
 #leaf {position:absolute;left:10px;top:5px;width:10px;height:10px;background:red;transform:rotate(30deg);transform-origin:0 0}
 @keyframes turn {to{transform:rotate(90deg)}}
 </style><div id="root"><%s id="container"><div id="leaf"></div></%s></div>`, scale, container.extra, container.tag, container.tag))
				visual := htmlAnimationVisual(t, world, owner, "leaf")
				for frame := 1; frame <= 2; frame++ {
					htmlAnimationStep(t, world, clock, 250*time.Millisecond)
					if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
						t.Fatal(err)
					}
					want := mgl32.Translate3D(40, 30, 0).
						Mul4(mgl32.HomogRotate3DZ(float32(frame) * math.Pi / 8)).
						Mul4(mgl32.Translate3D(20, 10, 0)).
						Mul4(mgl32.Scale3D(scale, 1, 1)).
						Mul4(mgl32.Translate3D(10, 5, 0)).
						Mul4(mgl32.HomogRotate3DZ(math.Pi / 6))
					got := world.GetRef[scene.UIGlobalTransform](visual).Matrix
					for _, point := range []mgl32.Vec4{{0, 0, 0, 1}, {10, 10, 0, 1}} {
						if got.Mul4x1(point).Sub(want.Mul4x1(point)).Len() > .001 {
							t.Fatalf("frame %d, point %v: got %v, want %v", frame, point, got, want)
						}
					}
				}
				state := world.GetRef[htmlRenderState](owner)
				if allocs := testing.AllocsPerRun(100, func() {
					advanceHTMLAnimations(world, state, .001)
				}); allocs != 0 {
					t.Fatalf("warm container composition allocated %g times", allocs)
				}
			})
		}
	}
}

func TestHTMLAnimationCompositionRefreshesSameFrameStyles(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
 #root {color:red;animation:ink 1s linear both;transform:rotate(90deg);transform-origin:0 0}
 #clip {width:100px;height:80px;overflow:hidden;transform:scale(2);transform-origin:0 0}
 #leaf {display:inline-block;background:currentColor;border:1px solid currentColor;text-decoration:line-through;animation:tint 1s linear both}
 @keyframes ink {to{color:blue}}
 @keyframes tint {from{color:currentColor}to{color:white}}
 </style><div id="root"><div id="clip"><span id="leaf">text</span></div></div>`)
	leaf := incrementalHTMLNode(t, world, owner, "leaf")
	htmlAnimationStep(t, world, clock, 500*time.Millisecond)
	checkColor := func(want graphics.ColorRGBA, minimum int) {
		t.Helper()
		state := world.GetRef[htmlRenderState](owner)
		count := 0
		for node, rendered := range state.RenderNodes {
			if node != leaf.node && node.Parent != leaf.node {
				continue
			}
			for _, entity := range rendered.Entities {
				if sprite := world.GetRef[scene.Sprite2D](entity); sprite != nil {
					if sprite.Shape.Fill != want {
						t.Fatalf("inherited sprite color = %v, want %v", sprite.Shape.Fill, want)
					}
					count++
				}
				if text := world.GetRef[scene.Text2D](entity); text != nil {
					if text.Color != want {
						t.Fatalf("inherited text color = %v, want %v", text.Color, want)
					}
					count++
				}
			}
		}
		if count < minimum {
			t.Fatalf("expected at least %d painted entities, got %d", minimum, count)
		}
	}
	checkColor(graphics.ColorRGBA{R: .75, G: .5, B: .75, A: 1}, 4)
	// No frame advancement: immediate restyles must not reuse animation results.
	leaf.SetAttribute("style", "animation:none;color:#00ff00;transform:translateX(7px)")
	incrementalSync(t, world)
	checkColor(graphics.ColorRGBA{G: 1, A: 1}, 2)
	leaf.SetAttribute("style", "animation:none;color:transparent;transform:translateX(11px)")
	incrementalSync(t, world)
	checkColor(graphics.ColorRGBA{}, 1)
	leaf.SetAttribute("style", "")
	incrementalSync(t, world)
	checkColor(graphics.ColorRGBA{R: .5, B: .5, A: 1}, 4)
}

func TestHTMLAnimationCompositionAtDistantScrollOrigin(t *testing.T) {
	world := ecs.NewWorld()
	clock := scene.NewTime()
	world.InsertResource(clock)
	value := New(`<style>
 #root {position:absolute;left:40px;top:1000030px;width:80px;height:80px;transform-origin:0 0;animation:turn 1s linear both}
 #middle {position:absolute;left:20px;top:10px;width:40px;height:40px;transform:scale(2,1);transform-origin:0 0}
 #leaf {position:absolute;left:10px;top:5px;width:10px;height:10px;background:red;transform:scale(2,1) rotate(30deg);transform-origin:0 0}
 @keyframes turn {to{transform:rotate(90deg)}}
 </style><div id="root"><div id="middle"><div id="leaf"></div></div></div>`)
	value.Width, value.Height = 240, 1000200
	bundle := ecs.NewBundle(1)
	bundle.Put(value)
	owner := world.Spawn(bundle)
	incrementalSync(t, world)
	visual := htmlAnimationVisual(t, world, owner, "leaf")
	area := htmlBindingArea(world, 0, 0)
	SetNodeScroll(world, owner, "root", area, 0, 1000000)
	SetNodeOffset(world, owner, "root", 3, 7)
	for frame := 1; frame <= 2; frame++ {
		htmlAnimationStep(t, world, clock, 250*time.Millisecond)
		if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
			t.Fatal(err)
		}
		want := mgl32.Translate3D(43, 37, 0).
			Mul4(mgl32.HomogRotate3DZ(float32(frame) * math.Pi / 8)).
			Mul4(mgl32.Translate3D(20, 10, 0)).
			Mul4(mgl32.Scale3D(2, 1, 1)).
			Mul4(mgl32.Translate3D(10, 5, 0)).
			Mul4(mgl32.Scale3D(2, 1, 1)).
			Mul4(mgl32.HomogRotate3DZ(math.Pi / 6))
		got := world.GetRef[scene.UIGlobalTransform](visual).Matrix
		for _, point := range []mgl32.Vec4{{0, 0, 0, 1}, {10, 10, 0, 1}} {
			if got.Mul4x1(point).Sub(want.Mul4x1(point)).Len() > .001 {
				t.Fatalf("frame %d, distant scroll origin: got %v, want %v", frame, got, want)
			}
		}
	}
}

func BenchmarkHTMLAnimationAncestorComposition(b *testing.B) {
	for _, depth := range []int{1, 32, 128} {
		b.Run(fmt.Sprint(depth), func(b *testing.B) {
			world := ecs.NewWorld()
			value := New(`<style>
 .ancestor {transform:rotate(.1deg);transform-origin:25% 75%;color:currentColor}
 #moving {animation:move 1s linear infinite alternate}
 .leaf {height:1px;width:10px;background:currentColor}
 @keyframes move {to{transform:translateX(100px);color:blue}}
 </style><div id="moving">` + strings.Repeat(`<div class="ancestor">`, depth) + strings.Repeat(`<div class="leaf"></div>`, 100) + strings.Repeat(`</div>`, depth) + `</div>`)
			value.Width, value.Height = 240, 400
			bundle := ecs.NewBundle(1)
			bundle.Put(value)
			owner := world.Spawn(bundle)
			incrementalSync(b, world)
			state := world.GetRef[htmlRenderState](owner)
			if len(state.RenderNodes) < 100 {
				b.Fatal("benchmark descendants were culled")
			}
			advanceHTMLAnimations(world, state, .016)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				advanceHTMLAnimations(world, state, .016)
			}
		})
	}
}
