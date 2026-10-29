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
		{"size", false, func(world *ecs.World, _, child, _ ecs.Entity) { _ = world.Insert(child, scene.Size2D{Width: 25}) }},
		{"layout_values", false, func(world *ecs.World, parent, _, _ ecs.Entity) {
			layout := world.GetMut[Layout](parent)
			layout.SpacingX++
		}},
		{"transform_values", false, func(world *ecs.World, _, child, _ ecs.Entity) {
			transform := world.GetMut[scene.UITransform](child)
			transform.Translation.X.Value++
		}},
		{"reparent", true, func(world *ecs.World, _, child, other ecs.Entity) {
			parent := world.GetMut[scene.Parent](child)
			parent.Entity = other
		}},
		{"remove_parent", true, func(world *ecs.World, _, child, _ ecs.Entity) { world.Remove[scene.Parent](child) }},
		{"order", true, func(world *ecs.World, _, child, _ ecs.Entity) {
			order := world.GetMut[scene.SiblingOrder](child)
			*order = -10
		}},
		{"remove_order", true, func(world *ecs.World, _, child, _ ecs.Entity) { world.Remove[scene.SiblingOrder](child) }},
		{"remove_layout", true, func(world *ecs.World, parent, _, _ ecs.Entity) { world.Remove[Layout](parent) }},
		{"remove_transform", true, func(world *ecs.World, parent, _, _ ecs.Entity) { world.Remove[scene.UITransform](parent) }},
		{"despawn_child", true, func(world *ecs.World, _, child, _ ecs.Entity) { world.Despawn(child) }},
		{"despawn_parent", true, func(world *ecs.World, parent, _, _ ecs.Entity) { world.Despawn(parent) }},
		{"spawn_child", true, func(world *ecs.World, parent, _, _ ecs.Entity) {
			spawner := world.NewSpawner2[scene.Parent, scene.UITransform]()
			spawner.Spawn(scene.Parent{Entity: parent}, scene.NewUITransform())
		}},
		{"reuse_slot", true, func(world *ecs.World, parent, child, _ ecs.Entity) {
			world.Despawn(child)
			spawner := world.NewSpawner2[scene.Parent, scene.UITransform]()
			spawner.Spawn(scene.Parent{Entity: parent}, scene.NewUITransform())
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := ecs.NewWorld()
			parents := world.NewSpawner2[Layout, scene.UITransform]()
			parent := parents.Spawn(NewLayout(), scene.NewUITransform())
			other := parents.Spawn(NewLayout(), scene.NewUITransform())
			children := world.NewSpawner4[scene.Parent, scene.UITransform, scene.SiblingOrder, scene.Size2D]()
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
	roots := world.NewSpawner1[scene.UITransform]()
	root := roots.Spawn(scene.NewUITransform())
	ancestors := world.NewSpawner1[scene.Parent]()
	ancestor := ancestors.Spawn(scene.Parent{Entity: root})
	layouts := world.NewSpawner3[scene.Parent, Layout, scene.UITransform]()
	leaf := layouts.Spawn(scene.Parent{Entity: ancestor}, NewLayout(), scene.NewUITransform())
	var cache uiLayoutScratch
	refreshUILayoutHierarchy(world, &cache)
	if cache.depths[leaf] != 2 {
		t.Fatalf("initial depth=%d, want 2", cache.depths[leaf])
	}
	world.Remove[scene.Parent](ancestor)
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
	parents := world.NewSpawner2[Layout, scene.UITransform]()
	parent := parents.Spawn(NewLayout(), scene.NewUITransform())
	children := world.NewSpawner3[scene.Parent, scene.UITransform, scene.SiblingOrder]()
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
					order := world.GetMut[scene.SiblingOrder](child)
					*order = -*order
				}
				refreshUILayoutHierarchy(world, &cache)
			}
		})
	}
}
