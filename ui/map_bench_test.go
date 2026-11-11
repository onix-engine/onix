package ui

import (
	"fmt"
	"testing"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/scene"
)

func BenchmarkTreeMaps(b *testing.B) {
	for _, count := range []int{128, 1024} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			world := ecs.NewWorld()
			trees := world.NewSpawner1[Tree[int]]()
			tree := trees.Spawn(Tree[int]{Width: 200, RowHeight: 20})
			spawn := world.NewSpawner1[TreeNode[int]]()
			for i := range count {
				spawn.Spawn(TreeNode[int]{Tree: tree, ID: i, Parent: (i - 1) / 4, HasParent: i != 0, Expanded: true, Order: i})
			}
			system := SyncTreeSystem[int]("")
			if err := system.Run(world); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				if err := system.Run(world); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkUILayoutHierarchyMaps(b *testing.B) {
	world := ecs.NewWorld()
	spawn := world.NewSpawner3[Layout, scene.UITransform, scene.Parent]()
	var parent ecs.Entity
	for i := range 1024 {
		if i%16 == 0 {
			parent = 0
		}
		parent = spawn.Spawn(Layout{}, scene.NewUITransform(), scene.Parent{Entity: parent})
	}
	var scratch uiLayoutScratch
	refreshUILayoutHierarchy(world, &scratch)
	b.ReportAllocs()
	for b.Loop() {
		scratch.initialized = false
		refreshUILayoutHierarchy(world, &scratch)
	}
}
