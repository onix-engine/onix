package html

import (
	"testing"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/scene"
)

func TestHTMLStaticAnimationWorkspaceLifecycle(t *testing.T) {
	world, owner, clock := htmlAnimationFixture(t, `<div id="item" style="width:20px;height:20px;background:red"></div>`)
	node := incrementalHTMLNode(t, world, owner, "item")
	visual := htmlAnimationVisual(t, world, owner, "item")
	state := ecs.GetRef[htmlRenderState](world, owner)
	initial := state.Animations[node.node]
	if initial == nil || initial.htmlNodeMotion != nil || Animating(world) {
		t.Fatal("static node owns a motion workspace or schedules frames")
	}
	node.SetAttribute("style", "width:20px;height:20px;background:red;transform:translateX(100px);transition:transform 1s linear")
	incrementalSync(t, world)
	if state.Animations[node.node] != initial || initial.htmlNodeMotion == nil || !Animating(world) {
		t.Fatal("adding transition failed to reuse the static baseline")
	}
	htmlAnimationStep(t, world, clock, 500*time.Millisecond)
	transform, _ := ecs.Get[scene.UITransform](world, visual)
	if transform.Translation.X.Value != 50 {
		t.Fatalf("introduced transition midpoint = %v", transform.Translation.X.Value)
	}
	node.SetAttribute("style", "width:20px;height:20px;background:red;transform:translateX(100px)")
	incrementalSync(t, world)
	if initial.htmlNodeMotion != nil || Animating(world) {
		t.Fatal("removing transition retained its workspace")
	}
	transform, _ = ecs.Get[scene.UITransform](world, visual)
	if transform.Translation.X.Value != 100 {
		t.Fatalf("cancelled transition target = %v", transform.Translation.X.Value)
	}
	node.SetAttribute("style", "width:20px;height:20px;background:red;transition:transform 1s linear")
	incrementalSync(t, world)
	htmlAnimationStep(t, world, clock, 500*time.Millisecond)
	transform, _ = ecs.Get[scene.UITransform](world, visual)
	if transform.Translation.X.Value != 50 {
		t.Fatalf("reintroduced transition midpoint = %v", transform.Translation.X.Value)
	}
}

func TestHTMLStaticAnimationWarmAllocations(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="item" style="transform:translateX(20px)"></div>`)
	node := incrementalHTMLNode(t, world, owner, "item")
	state := ecs.GetRef[htmlRenderState](world, owner)
	computed := state.Styles[node.node]
	if allocs := testing.AllocsPerRun(100, func() {
		// Force preparation without changing the actual style or motion needs.
		state.Animations[node.node].Computed = nil
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
	state := ecs.GetRef[htmlRenderState](world, owner)
	animation := state.Animations[child.node]
	foreground := animation.Foreground
	if animation.htmlNodeMotion != nil || foreground != (style.AnimationColor{R: .75, B: .25, A: 1}) {
		t.Fatalf("static child did not inherit current color: %+v", foreground)
	}
	child.SetAttribute("style", "color:green;transition:color 1s linear")
	incrementalSync(t, world)
	if animation.htmlNodeMotion == nil || animation.From.Colors[style.AnimationForeground] != foreground {
		t.Fatal("introduced color transition lost the previously displayed inherited color")
	}
}
