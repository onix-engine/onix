package ui

import (
	"fmt"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func quadraticButtonGroupFixture(count int) (*ecs.World, ecs.Entity, []ecs.Entity, []ecs.Entity) {
	world := ecs.NewWorld()
	owners := ecs.NewSpawner2[ButtonGroup, scene.Size2D](world)
	owner := owners.Spawn(ButtonGroup{Height: 20, StackIndex: 3}, scene.Size2D{})
	buttons := ecs.NewSpawner5[Control, scene.InputTarget2D, buttonGroupItem, scene.UITransform, scene.UIStackIndex](world)
	dividers := ecs.NewSpawner4[buttonGroupDivider, scene.UITransform, scene.Size2D, scene.UIStackIndex](world)
	var items, boundaries []ecs.Entity
	for i := range count {
		items = append(items, buttons.Spawn(Control{}, scene.InputTarget2D{Width: float32(i%3 + 1)}, buttonGroupItem{Group: owner, Index: i}, scene.NewUITransform(), scene.UIStackIndex(3)))
		if i+1 < count {
			boundaries = append(boundaries, dividers.Spawn(buttonGroupDivider{Group: owner, Index: i, Width: 1}, scene.NewUITransform(), scene.Size2D{}, scene.UIStackIndex(5)))
		}
	}
	ecs.GetMut[ButtonGroup](world, owner).Buttons = items
	syncButtonGroups(world, Skin{})
	return world, owner, items, boundaries
}

func TestButtonGroupDividerPrefixesRefreshWithControls(t *testing.T) {
	world, owner, items, dividers := quadraticButtonGroupFixture(4)
	check := func(want []float32, width float32) {
		t.Helper()
		syncButtonGroups(world, Skin{})
		group, _ := ecs.Get[ButtonGroup](world, owner)
		if group.Width != width {
			t.Fatalf("group width=%v, want %v", group.Width, width)
		}
		for i, entity := range dividers {
			transform, _ := ecs.Get[scene.UITransform](world, entity)
			if transform.Translation.X.Value != want[i]-.5 {
				t.Fatalf("divider %d=%v, want %v", i, transform.Translation.X.Value, want[i]-.5)
			}
		}
	}
	check([]float32{1, 3, 6}, 7)
	SetControlBounds(world, items[0], 10, 20)
	check([]float32{10, 12, 15}, 16)
	ecs.Remove[Control](world, items[1])
	check([]float32{10, 14, 13}, 14)
	world.Despawn(items[2])
	check([]float32{10, 11, 11}, 11)
	if n := testing.AllocsPerRun(100, func() { syncButtonGroups(world, Skin{}) }); n != 0 {
		t.Fatalf("warm divider sync allocated %v objects", n)
	}
}

func BenchmarkButtonGroupDividerTraversal(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world, _, _, _ := quadraticButtonGroupFixture(count)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				syncButtonGroups(world, Skin{})
			}
		})
	}
}
