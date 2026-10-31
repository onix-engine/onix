package html

import (
	"testing"
	"time"

	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/scene"
)

func TestHTMLStaticAnimationWorkspaceLifecycle(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<div id="item" style="width:20px;height:20px;background:red"></div>`)
	node := incrementalHTMLNode(t, world, owner, "item")
	visual := htmlAnimationVisual(t, world, owner, "item")
	state := world.GetRef[htmlRenderState](owner)
	if len(state.Animations) != 0 || state.AnimationBaselines[node.node].Computed != state.Styles[node.node] || Animating(world) {
		t.Fatal("static node owns an animation or lost its style baseline")
	}
	node.SetAttribute("style", "width:20px;height:20px;background:red;transform:translateX(100px);transition:transform 1s linear")
	incrementalSync(t, world)
	if state.Animations[node.node] == nil || !Animating(world) {
		t.Fatal("adding transition failed to initialize from the static baseline")
	}
	htmlAnimationStep(t, world, clock, 500*time.Millisecond)
	transform, _ := world.Get[scene.UITransform](visual)
	if transform.Translation.X.Value != 50 {
		t.Fatalf("introduced transition midpoint = %v", transform.Translation.X.Value)
	}
	node.SetAttribute("style", "width:20px;height:20px;background:red;transform:translateX(100px)")
	incrementalSync(t, world)
	if state.Animations[node.node] != nil || Animating(world) {
		t.Fatal("removing transition retained its animation")
	}
	transform, _ = world.Get[scene.UITransform](visual)
	if transform.Translation.X.Value != 100 {
		t.Fatalf("cancelled transition target = %v", transform.Translation.X.Value)
	}
	node.SetAttribute("style", "width:20px;height:20px;background:red;transition:transform 1s linear")
	incrementalSync(t, world)
	htmlAnimationStep(t, world, clock, 500*time.Millisecond)
	transform, _ = world.Get[scene.UITransform](visual)
	if transform.Translation.X.Value != 50 {
		t.Fatalf("reintroduced transition midpoint = %v", transform.Translation.X.Value)
	}
	node.Remove()
	incrementalSync(t, world)
	if _, exists := state.AnimationBaselines[node.node]; exists || state.Animations[node.node] != nil || Animating(world) {
		t.Fatal("removed node retained its animation baseline or runtime")
	}
}

func TestHTMLStaticAnimationWarmAllocations(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="item" style="transform:translateX(20px)"></div>`)
	node := incrementalHTMLNode(t, world, owner, "item")
	state := world.GetRef[htmlRenderState](owner)
	computed := state.Styles[node.node]
	if allocs := testing.AllocsPerRun(100, func() {
		// New static nodes need a compact baseline, without a heap object each.
		delete(state.AnimationBaselines, node.node)
		prepareHTMLNodeAnimation(state, node.node, computed)
	}); allocs != 0 {
		t.Fatalf("warm static preparation allocated %g times", allocs)
	}
}

func TestHTMLTransitionIntroducedDuringInheritedColorAnimation(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<style>
 #parent {color:red;animation:ink 1s linear forwards}
 @keyframes ink {to {color:blue}}
 </style><div id="parent"><span id="child">Text</span></div>`)
	child := incrementalHTMLNode(t, world, owner, "child")
	htmlAnimationStep(t, world, clock, 250*time.Millisecond)
	state := world.GetRef[htmlRenderState](owner)
	foreground := state.AnimationBaselines[child.node].Foreground
	if state.Animations[child.node] != nil || foreground != (style.AnimationColor{R: .75, B: .25, A: 1}) {
		t.Fatalf("static child did not inherit current color: %+v", foreground)
	}
	child.SetAttribute("style", "color:green;transition:color 1s linear")
	incrementalSync(t, world)
	animation := state.Animations[child.node]
	if animation == nil || animation.From.Colors[style.AnimationForeground] != foreground {
		t.Fatal("introduced color transition lost the previously displayed inherited color")
	}
}
