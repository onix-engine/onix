package html

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestHTMLAnimationLeavesUnrelatedVisualsUntouched(t *testing.T) {
	world, owner, _ := htmlAnimationFixture(t, `<style>
 div {width:10px;height:10px;background:red}
 #moving {animation:move 1s linear infinite alternate}
 @keyframes move {to{transform:translateX(100px)}}
 </style><div id="moving"></div><div id="static"></div>`)
	moving := htmlAnimationVisual(t, world, owner, "moving")
	static := htmlAnimationVisual(t, world, owner, "static")
	// Static ECS visuals can also be controlled by the embedding application.
	world.GetMut[scene.UITransform](static).Rotation = .375
	world.GetMut[scene.Sprite2D](static).Alpha = .37
	state := world.GetRef[htmlRenderState](owner)
	advanceHTMLAnimations(world, state, .25)
	if got := world.GetRef[scene.UITransform](moving).Translation.X.Value; got != 25 {
		t.Fatalf("animated translation = %v, want 25", got)
	}
	if allocs := testing.AllocsPerRun(100, func() {
		advanceHTMLAnimations(world, state, .016)
	}); allocs != 0 {
		t.Fatalf("warm sparse animation allocated %g times", allocs)
	}
	if world.GetRef[scene.UITransform](static).Rotation != .375 || world.GetRef[scene.Sprite2D](static).Alpha != .37 {
		t.Fatal("animation overwrote an unrelated static visual")
	}
}

func TestHTMLNestedAnimationContinuesAfterAncestorTransitionEnds(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
 #parent {width:40px;height:40px;transition:transform .25s linear}
 .moving {width:10px;height:10px;background:red;animation:move 1s linear infinite alternate}
 @keyframes move {to{transform:translateY(100px)}}
 </style><div id="parent"><div class="moving" id="nested"></div></div><div class="moving" id="peer"></div>`)
	parent := incrementalHTMLNode(t, world, owner, "parent")
	nested := htmlAnimationVisual(t, world, owner, "nested")
	peer := htmlAnimationVisual(t, world, owner, "peer")
	initialNested := *world.GetRef[scene.UITransform](nested)
	initialPeer := *world.GetRef[scene.UITransform](peer)
	parent.SetAttribute("style", "transform:translateX(100px)")
	incrementalSync(t, world)
	for frame, x := range []float32{50, 100, 100} {
		htmlAnimationStep(t, world, clock, 125*time.Millisecond)
		got := world.GetRef[scene.UITransform](nested)
		y := float32(frame+1) * 12.5
		if got.Translation.X.Value != initialNested.Translation.X.Value+x || got.Translation.Y.Value != initialNested.Translation.Y.Value+y {
			t.Fatalf("frame %d: nested translation = %v, want offset (%v, %v)", frame, got.Translation, x, y)
		}
		got = world.GetRef[scene.UITransform](peer)
		if got.Translation.X.Value != initialPeer.Translation.X.Value || got.Translation.Y.Value != initialPeer.Translation.Y.Value+y {
			t.Fatalf("frame %d: disjoint animation translation = %v", frame, got.Translation)
		}
	}
}

func TestHTMLAnimationFollowsReparentedAndNewDescendants(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
 #moving, #static {width:40px;height:40px}
 #moving {animation:move 1s linear infinite alternate}
 .child {width:10px;height:10px;background:red}
 @keyframes move {to{transform:translateX(100px)}}
 </style><div id="moving"><div class="child" id="child"></div></div><div id="static"></div>`)
	moving := incrementalHTMLNode(t, world, owner, "moving")
	static := incrementalHTMLNode(t, world, owner, "static")
	child := incrementalHTMLNode(t, world, owner, "child")
	htmlAnimationStep(t, world, clock, 250*time.Millisecond)
	if !child.Remove() || !static.Append(child) {
		t.Fatal("could not reparent child")
	}
	added := child.Clone()
	added.SetAttribute("id", "added")
	if !moving.Append(added) {
		t.Fatal("could not append new animated descendant")
	}
	incrementalSync(t, world)
	staticVisual := htmlAnimationVisual(t, world, owner, "child")
	addedVisual := htmlAnimationVisual(t, world, owner, "added")
	staticX := world.GetRef[scene.UITransform](staticVisual).Translation.X.Value
	addedX := world.GetRef[scene.UITransform](addedVisual).Translation.X.Value
	htmlAnimationStep(t, world, clock, 250*time.Millisecond)
	if got := world.GetRef[scene.UITransform](staticVisual).Translation.X.Value; got != staticX {
		t.Fatalf("reparented child still followed its old ancestor: %v -> %v", staticX, got)
	}
	if got := world.GetRef[scene.UITransform](addedVisual).Translation.X.Value; got != addedX+25 {
		t.Fatalf("new child did not follow animated ancestor: %v -> %v", addedX, got)
	}
	if !added.Remove() || !child.Remove() || !moving.Append(child) {
		t.Fatal("could not remove and reparent descendants")
	}
	incrementalSync(t, world)
	returned := htmlAnimationVisual(t, world, owner, "child")
	returnedX := world.GetRef[scene.UITransform](returned).Translation.X.Value
	htmlAnimationStep(t, world, clock, 250*time.Millisecond)
	if got := world.GetRef[scene.UITransform](returned).Translation.X.Value; got != returnedX+25 {
		t.Fatalf("returned child did not follow animated ancestor: %v -> %v", returnedX, got)
	}
}

func BenchmarkHTMLSparseAnimationFrame(b *testing.B) {
	for _, count := range []int{0, 1000, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world := ecs.NewWorld()
			value := New(`<style>
 div {width:10px;height:1px;background:red}
 #moving {animation:move 1s linear infinite alternate}
 @keyframes move {to{transform:translateX(100px)}}
 </style><div id="moving"></div>` + strings.Repeat(`<div></div>`, count))
			value.Width, value.Height = 240, float32(count+20)
			bundle := ecs.NewBundle(1)
			bundle.Put(value)
			owner := world.Spawn(bundle)
			incrementalSync(b, world)
			state := world.GetRef[htmlRenderState](owner)
			if len(state.RenderNodes) < count+1 {
				b.Fatalf("benchmark culled static siblings: %d render nodes", len(state.RenderNodes))
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
