package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestUILayoutComputesOnlyChangedContainer(t *testing.T) {
	world := ecs.NewWorld()
	parents := world.NewSpawner3[Layout, scene.UITransform, scene.Size2D]()
	children := world.NewSpawner4[scene.Parent, scene.UITransform, scene.Size2D, scene.Anchor]()
	one := parents.Spawn(Layout{Type: LayoutHorizontal, AutoAlignment: true}, scene.NewUITransform(), scene.Size2D{Width: 100, Height: 20})
	two := parents.Spawn(Layout{Type: LayoutHorizontal, AutoAlignment: true}, scene.NewUITransform(), scene.Size2D{Width: 100, Height: 20})
	first := children.Spawn(scene.Parent{Entity: one}, scene.NewUITransform(), scene.Size2D{Width: 10, Height: 10}, scene.AnchorTopLeft)
	second := children.Spawn(scene.Parent{Entity: one}, scene.NewUITransform(), scene.Size2D{Width: 10, Height: 10}, scene.AnchorTopLeft)
	children.Spawn(scene.Parent{Entity: two}, scene.NewUITransform(), scene.Size2D{Width: 10, Height: 10}, scene.AnchorTopLeft)
	sync := SyncLayoutSystem("")
	for range 3 {
		if err := sync.Run(world); err != nil {
			t.Fatal(err)
		}
	}
	scratch := world.FrameValue[uiLayoutScratchTag, uiLayoutScratch]()
	if scratch.recomputed != 0 {
		t.Fatal("static layouts were recomputed")
	}
	size := world.GetMut[scene.Size2D](first)
	size.Width = 25
	if err := sync.Run(world); err != nil {
		t.Fatal(err)
	}
	pose, _ := world.Get[scene.UITransform](second)
	if pose.Translation.X.Value != -25 || scratch.recomputed != 1 {
		t.Fatalf("position=%v recomputed=%d", pose.Translation.X, scratch.recomputed)
	}
}
