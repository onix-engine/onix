package ui

import (
	"slices"
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/scene"
)

func TestUILayoutHierarchyCacheInvalidation(t *testing.T) {
	for _, test := range []struct {
		mutate  func(*ecs.World, ecs.Entity, ecs.Entity, ecs.Entity)
		name    string
		rebuild bool
	}{
		{name: "unchanged", rebuild: false, mutate: func(*ecs.World, ecs.Entity, ecs.Entity, ecs.Entity) {}},
		{name: "size", rebuild: false, mutate: func(world *ecs.World, _, child, _ ecs.Entity) { _ = world.Insert(child, scene.Size2D{Width: 25}) }},
		{name: "layout_values", rebuild: false, mutate: func(world *ecs.World, parent, _, _ ecs.Entity) {
			layout := world.GetMut[Layout](parent)
			layout.SpacingX++
		}},
		{name: "transform_values", rebuild: false, mutate: func(world *ecs.World, _, child, _ ecs.Entity) {
			transform := world.GetMut[scene.UITransform](child)
			transform.Translation.X.Value++
		}},
		{name: "unrelated_migration", rebuild: false, mutate: func(world *ecs.World, _, child, _ ecs.Entity) { _ = world.Insert(child, "unrelated") }},
		{name: "reparent", rebuild: true, mutate: func(world *ecs.World, _, child, other ecs.Entity) {
			parent := world.GetMut[scene.Parent](child)
			parent.Entity = other
		}},
		{name: "remove_parent", rebuild: true, mutate: func(world *ecs.World, _, child, _ ecs.Entity) { world.Remove[scene.Parent](child) }},
		{name: "order", rebuild: true, mutate: func(world *ecs.World, _, child, _ ecs.Entity) {
			order := world.GetMut[scene.SiblingOrder](child)
			*order = -10
		}},
		{name: "remove_order", rebuild: true, mutate: func(world *ecs.World, _, child, _ ecs.Entity) { world.Remove[scene.SiblingOrder](child) }},
		{name: "remove_layout", rebuild: true, mutate: func(world *ecs.World, parent, _, _ ecs.Entity) { world.Remove[Layout](parent) }},
		{name: "remove_transform", rebuild: true, mutate: func(world *ecs.World, parent, _, _ ecs.Entity) { world.Remove[scene.UITransform](parent) }},
		{name: "despawn_child", rebuild: true, mutate: func(world *ecs.World, _, child, _ ecs.Entity) { world.Despawn(child) }},
		{name: "despawn_parent", rebuild: true, mutate: func(world *ecs.World, parent, _, _ ecs.Entity) { world.Despawn(parent) }},
		{name: "spawn_child", rebuild: true, mutate: func(world *ecs.World, parent, _, _ ecs.Entity) {
			spawner := world.NewSpawner2[scene.Parent, scene.UITransform]()
			spawner.Spawn(scene.Parent{Entity: parent}, scene.NewUITransform())
		}},
		{name: "reuse_slot", rebuild: true, mutate: func(world *ecs.World, parent, child, _ ecs.Entity) {
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
			if !slices.Equal(cache.nodes, fresh.nodes) {
				t.Fatalf("cached topology differs from a fresh traversal: nodes=%v", cache.nodes)
			}
			for _, node := range cache.nodes {
				cache.collectChildren(world, node.entity, &uiLayoutRuntimeState{})
				fresh.collectChildren(world, node.entity, &uiLayoutRuntimeState{})
				if !slices.Equal(cache.layoutChildren, fresh.layoutChildren) {
					t.Fatalf("cached child order differs from a fresh traversal at %d", node.entity)
				}
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
	if len(cache.nodes) != 1 || cache.nodes[0] != (uiLayoutNode{entity: leaf, depth: 2}) {
		t.Fatalf("initial layout order=%v, want leaf at depth 2", cache.nodes)
	}
	world.Remove[scene.Parent](ancestor)
	if !refreshUILayoutHierarchy(world, &cache) || cache.nodes[0].depth != 1 {
		t.Fatalf("ancestor change not reflected: nodes=%v", cache.nodes)
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

func TestUILayoutSharedHierarchyPreservesScopeAndSiblingOrder(t *testing.T) {
	world := ecs.NewWorld()
	roots := world.NewSpawner1[scene.UITransform]()
	root := roots.Spawn(scene.NewUITransform())
	parents := world.NewSpawner5[Layout, scene.UITransform, scene.Size2D, scene.Anchor, scene.Parent]()
	parent := parents.Spawn(Layout{Type: LayoutHorizontal, AutoAlignment: true}, scene.NewUITransform(), scene.Size2D{Width: 100, Height: 20}, scene.AnchorTopLeft, scene.Parent{Entity: root})
	children := world.NewSpawner5[scene.Parent, scene.UITransform, scene.Size2D, scene.Anchor, scene.SiblingOrder]()
	one := children.Spawn(scene.Parent{Entity: parent}, scene.NewUITransform(), scene.Size2D{Width: 10, Height: 10}, scene.AnchorTopLeft, 0)
	two := children.Spawn(scene.Parent{Entity: parent}, scene.NewUITransform(), scene.Size2D{Width: 10, Height: 10}, scene.AnchorTopLeft, 0)
	first := children.Spawn(scene.Parent{Entity: parent}, scene.NewUITransform(), scene.Size2D{Width: 10, Height: 10}, scene.AnchorTopLeft, -1)
	var scope ecs.QueryScope
	scope.Reset([]ecs.Entity{parent, one, two})
	world.SetQueryScope(&scope)
	update := func() {
		t.Helper()
		if err := UpdateLayout(world); err != nil {
			t.Fatal(err)
		}
	}
	position := func(entity ecs.Entity, want float32) {
		t.Helper()
		if transform := world.GetRef[scene.UITransform](entity); transform.Translation.X.Value != want {
			t.Fatalf("position(%d)=%v, want %v", entity, transform.Translation.X, want)
		}
	}
	update()
	position(one, 0)
	position(two, 10)
	world.GetMut[scene.Size2D](one).Width = 20
	update()
	position(two, 20)
	scope.Reset([]ecs.Entity{parent, two, first, one})
	update()
	position(first, 0)
	position(one, 10)
	position(two, 30)
	// A disabled layout may skip several topology versions before it returns.
	world.GetMut[Layout](parent).Type = LayoutNone
	*world.GetMut[scene.SiblingOrder](two) = -2
	update()
	world.GetMut[Layout](parent).Type = LayoutHorizontal
	update()
	position(two, 0)
	position(first, 10)
	position(one, 20)
	// Reparenting remains visible even when the old and new ancestors are
	// outside the scope selected by the layout system.
	world.GetMut[scene.Parent](one).Entity = root
	update()
	position(two, 0)
	position(first, 10)
	for range 3 {
		update()
	}
	if allocations := testing.AllocsPerRun(100, update); allocations != 0 {
		t.Fatalf("stable scoped layout allocated %g times", allocations)
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
