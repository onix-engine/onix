package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

// Use the canvas's original seven-component Bundle as the creation baseline.
func BenchmarkSpawnNodeBundleRecycled(b *testing.B) {
	world := ecs.NewWorld()
	spawn := func() ecs.Entity {
		bundle := ecs.NewBundle(7)
		ecs.Put(&bundle, scene.UITransformFromXY(10, 20))
		ecs.Put(&bundle, scene.UIGlobalTransform{})
		ecs.Put(&bundle, scene.Parent{})
		ecs.Put(&bundle, scene.AnchorTopLeft)
		ecs.Put(&bundle, scene.Visibility{Visible: true})
		ecs.Put(&bundle, scene.UIStackIndex(3))
		ecs.Put(&bundle, scene.Size2D{Width: 40, Height: 50})
		return world.Spawn(bundle)
	}
	world.Despawn(spawn())
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		world.Despawn(spawn())
	}
}

// Recycling keeps capacity growth outside the measured cleanup path.
func BenchmarkUnbindControlRecycled(b *testing.B) {
	world := ecs.NewWorld()
	nodes := ecs.NewSpawner7[scene.Name, Control, scene.InputTarget2D, scene.InputState2D, Button, Part[BodyPart], Content[string]](world)
	nodes.Reserve(1)
	run := func() {
		entity := nodes.Spawn(scene.Name{Value: "authored"}, Control{}, scene.InputTarget2D{}, scene.InputState2D{}, Button{}, Part[BodyPart]{}, Content[string]{})
		UnbindControl(world, entity)
		world.Despawn(entity)
	}
	run()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		run()
	}
}

func BenchmarkSpawnNodeRecycled(b *testing.B) {
	world := ecs.NewWorld()
	nodes := NewNodeSpawner[scene.Size2D](world)
	nodes.Reserve(1)
	world.Despawn(nodes.Spawn(NodeOptions{}, scene.Size2D{}))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		world.Despawn(nodes.Spawn(NodeOptions{X: 10, Y: 20, StackIndex: 3}, scene.Size2D{Width: 40, Height: 50}))
	}
}

func BenchmarkSpawnNodeCachedRecycled(b *testing.B) {
	world := ecs.NewWorld()
	world.Despawn(SpawnNode(world, NodeOptions{}, scene.Size2D{}))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		world.Despawn(SpawnNode(world, NodeOptions{X: 10, Y: 20, StackIndex: 3}, scene.Size2D{Width: 40, Height: 50}))
	}
}
