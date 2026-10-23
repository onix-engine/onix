package ui

import (
	"slices"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestUILayoutHierarchyCacheInvalidation(t *testing.T) {
	for _, test := range []struct {
		name    string
		rebuild bool
		mutate  func(*ecs.World, ecs.Entity, ecs.Entity, ecs.Entity)
	}{
		{"unchanged", false, func(*ecs.World, ecs.Entity, ecs.Entity, ecs.Entity) {}},
		{"size", false, func(world *ecs.World, _, child, _ ecs.Entity) { _ = ecs.Insert(world, child, scene.Size2D{Width: 25}) }},
		{"layout_values", false, func(world *ecs.World, parent, _, _ ecs.Entity) {
			layout := ecs.GetMut[Layout](world, parent)
			layout.SpacingX++
		}},
		{"transform_values", false, func(world *ecs.World, _, child, _ ecs.Entity) {
			transform := ecs.GetMut[scene.UITransform](world, child)
			transform.Translation.X.Value++
		}},
		{"reparent", true, func(world *ecs.World, _, child, other ecs.Entity) {
			parent := ecs.GetMut[scene.Parent](world, child)
			parent.Entity = other
		}},
		{"remove_parent", true, func(world *ecs.World, _, child, _ ecs.Entity) { ecs.Remove[scene.Parent](world, child) }},
		{"order", true, func(world *ecs.World, _, child, _ ecs.Entity) {
			order := ecs.GetMut[scene.SiblingOrder](world, child)
			*order = -10
		}},
		{"remove_order", true, func(world *ecs.World, _, child, _ ecs.Entity) { ecs.Remove[scene.SiblingOrder](world, child) }},
		{"remove_layout", true, func(world *ecs.World, parent, _, _ ecs.Entity) { ecs.Remove[Layout](world, parent) }},
		{"remove_transform", true, func(world *ecs.World, parent, _, _ ecs.Entity) { ecs.Remove[scene.UITransform](world, parent) }},
		{"despawn_child", true, func(world *ecs.World, _, child, _ ecs.Entity) { world.Despawn(child) }},
		{"despawn_parent", true, func(world *ecs.World, parent, _, _ ecs.Entity) { world.Despawn(parent) }},
		{"spawn_child", true, func(world *ecs.World, parent, _, _ ecs.Entity) {
			spawner := ecs.NewSpawner2[scene.Parent, scene.UITransform](world)
			spawner.Spawn(scene.Parent{Entity: parent}, scene.NewUITransform())
		}},
		{"reuse_slot", true, func(world *ecs.World, parent, child, _ ecs.Entity) {
			world.Despawn(child)
			spawner := ecs.NewSpawner2[scene.Parent, scene.UITransform](world)
			spawner.Spawn(scene.Parent{Entity: parent}, scene.NewUITransform())
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := ecs.NewWorld()
			parents := ecs.NewSpawner2[Layout, scene.UITransform](world)
			parent := parents.Spawn(NewLayout(), scene.NewUITransform())
			other := parents.Spawn(NewLayout(), scene.NewUITransform())
			children := ecs.NewSpawner4[scene.Parent, scene.UITransform, scene.SiblingOrder, scene.Size2D](world)
			child := children.Spawn(scene.Parent{Entity: parent}, scene.NewUITransform(), 1, scene.Size2D{})
			children.Spawn(scene.Parent{Entity: parent}, scene.NewUITransform(), 2, scene.Size2D{})
			var cache uiLayoutScratch
			if !refreshUILayoutHierarchy(world, &cache) || refreshUILayoutHierarchy(world, &cache) {
				t.Fatal("cache did not warm up")
			}
			test.mutate(world, parent, child, other)
			if rebuilt := refreshUILayoutHierarchy(world, &cache); rebuilt != test.rebuild {
				t.Fatalf("rebuilt=%v, want %v", rebuilt, test.rebuild)
			}
			var fresh uiLayoutScratch
			refreshUILayoutHierarchy(world, &fresh)
			if !slices.Equal(cache.children, fresh.children) || !slices.Equal(cache.nodes, fresh.nodes) {
				t.Fatalf("cached topology differs from a fresh traversal: children=%v nodes=%v", cache.children, cache.nodes)
			}
			if refreshUILayoutHierarchy(world, &cache) {
				t.Fatal("unchanged topology rebuilt again")
			}
		})
	}
}

func TestUILayoutHierarchyCacheTracksAncestorsWithoutLayout(t *testing.T) {
	world := ecs.NewWorld()
	roots := ecs.NewSpawner1[scene.UITransform](world)
	root := roots.Spawn(scene.NewUITransform())
	ancestors := ecs.NewSpawner1[scene.Parent](world)
	ancestor := ancestors.Spawn(scene.Parent{Entity: root})
	layouts := ecs.NewSpawner3[scene.Parent, Layout, scene.UITransform](world)
	leaf := layouts.Spawn(scene.Parent{Entity: ancestor}, NewLayout(), scene.NewUITransform())
	var cache uiLayoutScratch
	refreshUILayoutHierarchy(world, &cache)
	if cache.depths[leaf] != 2 {
		t.Fatalf("initial depth=%d, want 2", cache.depths[leaf])
	}
	ecs.Remove[scene.Parent](world, ancestor)
	if !refreshUILayoutHierarchy(world, &cache) || cache.depths[leaf] != 1 {
		t.Fatalf("ancestor change not reflected: depth=%d", cache.depths[leaf])
	}
}

func TestUILayoutHierarchyCacheDoesNotAllocate(t *testing.T) {
	world, _ := benchmarkUILayoutWorld(128)
	var cache uiLayoutScratch
	refreshUILayoutHierarchy(world, &cache)
	if allocations := testing.AllocsPerRun(100, func() {
		if refreshUILayoutHierarchy(world, &cache) {
			t.Fatal("stable hierarchy rebuilt")
		}
	}); allocations != 0 {
		t.Fatalf("allocations=%v, want 0", allocations)
	}
}

func benchmarkUILayoutWorld(count int) (*ecs.World, ecs.Entity) {
	world := ecs.NewWorld()
	parents := ecs.NewSpawner2[Layout, scene.UITransform](world)
	parent := parents.Spawn(NewLayout(), scene.NewUITransform())
	children := ecs.NewSpawner3[scene.Parent, scene.UITransform, scene.SiblingOrder](world)
	children.Reserve(count)
	var child ecs.Entity
	for index := range count {
		child = children.Spawn(scene.Parent{Entity: parent}, scene.NewUITransform(), scene.SiblingOrder(index))
	}
	return world, child
}

func BenchmarkUILayoutHierarchy(b *testing.B) {
	for _, rebuild := range []bool{false, true} {
		name := "Stable"
		if rebuild {
			name = "Rebuild"
		}
		b.Run(name, func(b *testing.B) {
			world, child := benchmarkUILayoutWorld(1000)
			var cache uiLayoutScratch
			refreshUILayoutHierarchy(world, &cache)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if rebuild {
					order := ecs.GetMut[scene.SiblingOrder](world, child)
					*order = -*order
				}
				refreshUILayoutHierarchy(world, &cache)
			}
		})
	}
}
