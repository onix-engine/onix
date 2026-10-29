package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func BenchmarkTraversalAutoGrid(b *testing.B) {
	world := ecs.NewWorld()
	spawner := world.NewSpawner3[scene.UITransform, scene.Size2D, scene.Anchor]()
	parent := spawner.Spawn(scene.NewUITransform(), scene.Size2D{Width: 800, Height: 600}, scene.AnchorTopLeft)
	children := make([]ecs.Entity, 1000)
	for i := range children {
		children[i] = spawner.Spawn(scene.NewUITransform(), scene.Size2D{Width: 20, Height: 10}, scene.AnchorTopLeft)
	}
	layout := Layout{Type: LayoutGrid, ResizeMode: LayoutResizeContainer, AutoAlignment: true}
	applyUILayout(world, parent, layout, children)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		applyUILayout(world, parent, layout, children)
	}
}

func TestTraversalScaledResizeLayout(t *testing.T) {
	for _, vertical := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			for _, available := range []float32{0, 120} {
				world := ecs.NewWorld()
				spawner := world.NewSpawner3[scene.UITransform, scene.Size2D, scene.Anchor]()
				parent := spawner.Spawn(scene.NewUITransform(), scene.Size2D{Width: available, Height: available}, scene.AnchorTopLeft)
				transform := scene.NewUITransform()
				transform.Scale[0], transform.Scale[1] = 2, 2
				children := []ecs.Entity{spawner.Spawn(transform, scene.Size2D{Width: 10, Height: 10}, scene.AnchorTopLeft), spawner.Spawn(transform, scene.Size2D{Width: 20, Height: 20}, scene.AnchorTopLeft)}
				layout := Layout{Type: LayoutHorizontal, ResizeMode: LayoutResizeChildren, AffectedByScale: true, AutoAlignment: true}
				if vertical {
					layout.Type = LayoutVertical
				}
				if reverse {
					layout.HorizontalDirection = LayoutRightToLeft
					layout.VerticalDirection = LayoutBottomToTop
				}
				applyUILayout(world, parent, layout, children)
				for i, child := range children {
					size, _ := world.Get[scene.Size2D](child)
					tr, _ := world.Get[scene.UITransform](child)
					length, position := size.Width, tr.Translation.X.Value
					if vertical {
						length, position = size.Height, tr.Translation.Y.Value
					}
					wantLength := available * float32(i+1) / 6
					wantPosition := float32(i) * available / 3
					if reverse {
						wantPosition = available - wantPosition - wantLength*2
					}
					if length != wantLength || position != wantPosition {
						t.Fatalf("vertical=%v reverse=%v available=%v child=%d: size=%v pos=%v want %v/%v", vertical, reverse, available, i, length, position, wantLength, wantPosition)
					}
				}
			}
		}
	}
}

func TestTraversalAutoGridContainerAndCellSize(t *testing.T) {
	world := ecs.NewWorld()
	spawner := world.NewSpawner3[scene.UITransform, scene.Size2D, scene.Anchor]()
	parent := spawner.Spawn(scene.NewUITransform(), scene.Size2D{Width: 100, Height: 100}, scene.AnchorTopLeft)
	children := []ecs.Entity{
		spawner.Spawn(scene.NewUITransform(), scene.Size2D{Width: 30, Height: 10}, scene.AnchorTopLeft),
		spawner.Spawn(scene.NewUITransform(), scene.Size2D{Width: 40, Height: 20}, scene.AnchorTopLeft),
		spawner.Spawn(scene.NewUITransform(), scene.Size2D{Width: 10, Height: 5}, scene.AnchorTopLeft),
	}
	layout := Layout{Type: LayoutGrid, ResizeMode: LayoutResizeContainer}
	applyUILayout(world, parent, layout, children)
	size, _ := world.Get[scene.Size2D](parent)
	if size.Width != 80 || size.Height != 40 {
		t.Fatalf("container=%v", size)
	}
	for i, child := range children {
		tr, _ := world.Get[scene.UITransform](child)
		if tr.Translation.X.Value != float32(i%2)*40 || tr.Translation.Y.Value != float32(i/2)*20 {
			t.Fatalf("child %d position=%v", i, tr.Translation)
		}
	}
	if allocs := testing.AllocsPerRun(100, func() { applyUILayout(world, parent, layout, children) }); allocs != 0 {
		t.Fatalf("grid allocated %v times", allocs)
	}
}
