package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestHTMLNeedsFrameTracksPendingSurface(t *testing.T) {
	world := ecs.NewWorld()
	page := New(`<div id="label">before</div>`)
	page.Width, page.Height = 200, 100
	bundle := ecs.NewBundle(1)
	bundle.Put(page)
	owner := world.Spawn(bundle)
	if !NeedsFrame(world) {
		t.Fatal("new surface did not request synchronization")
	}
	incrementalSync(t, world)
	if NeedsFrame(world) {
		t.Fatal("settled surface kept scheduling frames")
	}
	for _, change := range []struct {
		name  string
		apply func()
	}{
		{"DOM", func() { incrementalHTMLNode(t, world, owner, "label").SetText("after") }},
		{"CSS", func() { world.GetMut[Surface](owner).CSS = "div { color:red }" }},
		{"viewport", func() { world.GetMut[Surface](owner).Width = 300 }},
	} {
		t.Run(change.name, func(t *testing.T) {
			change.apply()
			if !NeedsFrame(world) {
				t.Fatal("pending change allowed suspension")
			}
			incrementalSync(t, world)
			if NeedsFrame(world) {
				t.Fatal("synchronized change kept scheduling frames")
			}
		})
	}
	if allocations := testing.AllocsPerRun(100, func() { NeedsFrame(world) }); allocations != 0 {
		t.Fatalf("idle check allocated %g times", allocations)
	}
}

func TestHTMLNeedsFrameIgnoresSuspendedComponentsAndAnimations(t *testing.T) {
	world, owner, _ := htmlAnimationFixture(t, `<style>
 @keyframes spin {to {transform:rotate(360deg)}}
 #spin {width:20px;height:20px;background:red;animation:spin 1s infinite}
 </style><div id="spin"></div>`)
	definition, err := Define[struct{}, string](NewTemplate(`<span data-ref="label"></span>`), "", Behavior[struct{}, string]{
		Render: func(context RenderContext[struct{}, string], view *View) { view.Text("label", *context.State) },
	})
	if err != nil {
		t.Fatal(err)
	}
	component, err := definition.MountComponent(world, owner, struct{}{}, "before", 1)
	if err != nil {
		t.Fatal(err)
	}
	incrementalSync(t, world)
	if !NeedsFrame(world) || !Animating(world) {
		t.Fatal("active animation allowed suspension")
	}
	UpdateComponentState[struct{}, string](world, component, func(value *string) { *value = "after" })
	if err := world.Insert(owner, scene.Activity{Active: false}); err != nil {
		t.Fatal(err)
	}
	if err := world.Insert(owner, scene.Visibility{Visible: false}); err != nil {
		t.Fatal(err)
	}
	if NeedsFrame(world) || Animating(world) {
		t.Fatal("suspended page kept scheduling frames")
	}
	world.GetMut[scene.Activity](owner).Active = true
	if !NeedsFrame(world) {
		t.Fatal("reactivated page lost pending work")
	}
}

func TestHTMLNeedsFrameTracksScrollPaintWithoutConsumingIt(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="scroll" style="width:80px;height:80px;overflow:auto"><div style="width:2000px;height:2000px"></div></div>`)
	node := incrementalHTMLNode(t, world, owner, "scroll")
	state := world.GetRef[htmlRenderState](owner)
	area := state.OverflowBindings[node.node].entity
	for _, axis := range []int{0, 1} {
		scroll := world.GetMut[scene.UIScroll](area)
		scroll.Position[axis] = -64
		if NeedsFrame(world) {
			t.Fatal("scroll within the paint buffer unnecessarily requested another frame")
		}
		scroll.Position[axis] = -1000
		if !NeedsFrame(world) {
			t.Fatal("newly visible paint did not prevent idle")
		}
		if allocations := testing.AllocsPerRun(100, func() { NeedsFrame(world) }); allocations != 0 {
			t.Fatalf("pending paint check allocated %g times", allocations)
		}
		if len(state.PaintCullRoots) != 0 || state.BoxRenderings[node.node].CullChanged {
			t.Fatal("idle check modified reconciliation scratch")
		}
		incrementalSync(t, world)
		if NeedsFrame(world) {
			t.Fatal("synchronized scroll paint kept scheduling frames")
		}
		world.GetMut[scene.UIScroll](area).Position[axis] = 0
		incrementalSync(t, world)
	}
}
