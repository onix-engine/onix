package html

import (
	"strings"
	"testing"

	"github.com/onix-engine/onix/scene"
)

func TestHTMLBindingSettersSkipUnrelatedRenderNodes(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="target"><input id="field" value="Value"/></div>`+strings.Repeat(`<div style="width:10px;height:1px;background:red"></div>`, 1000))
	field := incrementalControl(t, world, owner, "field")
	area := htmlBindingArea(world, -3, -7)
	SetNodeOffset(world, owner, "target", 3, 11)
	SetNodeScroll(world, owner, "target", area, 0, 13)
	state := world.GetRef[htmlRenderState](owner)
	state.SyncStats.BindingEntities = 0
	before := *world.GetRef[scene.UITransform](field)
	SetNodeOffset(world, owner, "target", 5, 11)
	after := world.GetRef[scene.UITransform](field)
	if after.Translation.X.Value != before.Translation.X.Value+2 {
		t.Fatal("local offset did not move the control")
	}
	SetNodeScroll(world, owner, "target", area, 0, 17)
	if after = world.GetRef[scene.UITransform](field); after.Translation.Y.Value != before.Translation.Y.Value-4 {
		t.Fatal("local scroll origin did not rebase the control")
	}
	if state.SyncStats.BindingEntities == 0 || state.SyncStats.BindingEntities > 32 {
		t.Fatalf("local bindings visited unrelated entities: %d", state.SyncStats.BindingEntities)
	}
	x := float32(3)
	if allocs := testing.AllocsPerRun(100, func() {
		x = 8 - x
		SetNodeOffset(world, owner, "target", x, 11)
		SetNodeScroll(world, owner, "target", area, 0, x)
	}); allocs != 0 {
		t.Fatalf("warm binding setters allocated %g times", allocs)
	}
}

func TestHTMLBindingSetterRefreshesDeadContainerOutsideTarget(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<div id="other"></div><div id="outer"><input id="field"/></div>`)
	first, second := htmlBindingArea(world, 0, 0), htmlBindingArea(world, 0, 0)
	SetNodeScroll(world, owner, "outer", first, 0, 0)
	SetNodeScroll(world, owner, "field", second, 0, 0)
	field := incrementalControl(t, world, owner, "field")
	if world.GetRef[scene.Parent](field).Entity != first {
		t.Fatal("ancestor scroll precedence was not established")
	}
	world.Despawn(first)
	SetNodeOffset(world, owner, "other", 1, 1)
	if world.GetRef[scene.Parent](field).Entity != second {
		t.Fatal("dead container outside the updated subtree prevented fallback")
	}
}

func TestHTMLBindingSetterRefreshesRenamedAndMovedNodes(t *testing.T) {
	world, owner := incrementalHTMLFixture(t, `<style>.paint{width:10px;height:10px;background:red}</style><div id="left"><div class="paint" id="bound"></div></div><div id="right"><div class="paint" id="next"></div></div>`)
	oldNode := incrementalHTMLNode(t, world, owner, "bound")
	newNode := incrementalHTMLNode(t, world, owner, "next")
	oldVisual := htmlAnimationVisual(t, world, owner, "bound")
	newVisual := htmlAnimationVisual(t, world, owner, "next")
	oldX := world.GetRef[scene.UITransform](oldVisual).Translation.X.Value
	newX := world.GetRef[scene.UITransform](newVisual).Translation.X.Value
	SetNodeOffset(world, owner, "bound", 5, 0)
	oldNode.SetAttribute("id", "old")
	newNode.SetAttribute("id", "bound")
	SetNodeOffset(world, owner, "bound", 9, 0)
	if world.GetRef[scene.UITransform](oldVisual).Translation.X.Value != oldX || world.GetRef[scene.UITransform](newVisual).Translation.X.Value != newX+9 {
		t.Fatal("renaming the binding target left stale offsets")
	}
	incrementalSync(t, world)
	SetNodeOffset(world, owner, "left", 20, 0)
	right := incrementalHTMLNode(t, world, owner, "right")
	if !oldNode.Remove() || !right.Append(oldNode) {
		t.Fatal("could not move bound descendant")
	}
	SetNodeOffset(world, owner, "left", 30, 0)
	if got := world.GetRef[scene.UITransform](oldVisual).Translation.X.Value; got != oldX {
		t.Fatalf("moved descendant retained its old ancestor offset: %v", got)
	}
}
