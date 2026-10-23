package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestWidgetStretchCommitsFinalDimensionsOnce(t *testing.T) {
	world := ecs.NewWorld()
	parents := ecs.NewSpawner1[scene.Size2D](world)
	parent := parents.Spawn(scene.Size2D{Width: 100, Height: 80})
	widget := NewWidget()
	widget.AlignLeft, widget.AlignRight, widget.AlignTop, widget.AlignBottom = true, true, true, true
	widget.Left, widget.Right, widget.Top, widget.Bottom = scene.Px(2), scene.Px(3), scene.Px(4), scene.Px(5)
	nodes := ecs.NewSpawner7[Widget, scene.Parent, scene.UITransform, scene.Size2D, scene.InputTarget2D, scene.InputBlocker2D, scene.Anchor](world)
	child := nodes.Spawn(widget, scene.Parent{Entity: parent}, scene.NewUITransform(),
		scene.Size2D{Width: 20, Height: 10},
		scene.InputTarget2D{Width: 20, Height: 10, Receiver: parent, PassThrough: true},
		scene.InputBlocker2D{Width: 20, Height: 10, Receiver: parent, PassThrough: true}, scene.AnchorTopLeft)
	sizeVersion := ecs.ComponentVersion[scene.Size2D](world)
	targetVersion := ecs.ComponentVersion[scene.InputTarget2D](world)
	blockerVersion := ecs.ComponentVersion[scene.InputBlocker2D](world)
	widgetVersion := ecs.ComponentVersion[Widget](world)
	syncUIWidgets(world)
	size := ecs.GetRef[scene.Size2D](world, child)
	target := ecs.GetRef[scene.InputTarget2D](world, child)
	blocker := ecs.GetRef[scene.InputBlocker2D](world, child)
	if size.Width != 95 || size.Height != 71 || target.Width != 95 || target.Height != 71 || blocker.Width != 95 || blocker.Height != 71 {
		t.Fatal("stretched visual and input dimensions diverged")
	}
	if target.Receiver != parent || !target.PassThrough || blocker.Receiver != parent || !blocker.PassThrough {
		t.Fatal("resizing changed input behavior")
	}
	if ecs.ComponentVersion[scene.Size2D](world) != sizeVersion+1 || ecs.ComponentVersion[scene.InputTarget2D](world) != targetVersion+1 || ecs.ComponentVersion[scene.InputBlocker2D](world) != blockerVersion+1 {
		t.Fatal("dual-axis stretch wrote intermediate dimensions")
	}
	if ecs.ComponentVersion[Widget](world) != widgetVersion {
		t.Fatal("layout marked its configuration as changed")
	}
	version := world.MutationVersion()
	if allocations := testing.AllocsPerRun(100, func() { syncUIWidgets(world) }); allocations != 0 || world.MutationVersion() != version {
		t.Fatalf("unchanged layout: allocations=%v, mutation=%d, want %d", allocations, world.MutationVersion(), version)
	}
}
