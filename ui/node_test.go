package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestNodeSpawnerDefaultsAndOptions(t *testing.T) {
	world := ecs.NewWorld()
	nodes := NewNodeSpawner[scene.Name](world)
	plain := nodes.Spawn(NodeOptions{}, scene.Name{Value: "plain"})
	if *world.GetRef[scene.UITransform](plain) != scene.NewUITransform() ||
		*world.GetRef[scene.Anchor](plain) != scene.AnchorTopLeft ||
		!world.GetRef[scene.Visibility](plain).Visible ||
		world.GetRef[scene.Parent](plain).Entity != 0 ||
		*world.GetRef[scene.UIStackIndex](plain) != 0 ||
		world.GetRef[scene.UIGlobalTransform](plain) == nil {
		t.Fatal("zero options did not produce a complete visible node")
	}
	child := nodes.Spawn(NodeOptions{Parent: plain, X: 12, Y: -30, StackIndex: 9, Hidden: true, Anchor: &scene.AnchorCenter}, scene.Name{Value: "child"})
	transform := world.GetRef[scene.UITransform](child)
	if transform.Translation != scene.Px2(12, -30) || transform.Scale != (mgl32.Vec2{1, 1}) ||
		world.GetRef[scene.Visibility](child).Visible ||
		*world.GetRef[scene.Anchor](child) != scene.AnchorCenter ||
		world.GetRef[scene.Parent](child).Entity != plain ||
		*world.GetRef[scene.UIStackIndex](child) != 9 {
		t.Fatal("node options were not applied")
	}
	if err := world.Insert(child, scene.Size2D{Width: 20, Height: 10}); err != nil {
		t.Fatal(err)
	}
	if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	if matrix := world.GetRef[scene.UIGlobalTransform](child).Matrix; matrix[12] != 12 || matrix[13] != -30 {
		t.Fatalf("node transform was not resolved: %v", matrix)
	}
}

func TestNodeSpawnersRejectDuplicateComponents(t *testing.T) {
	for _, run := range []func(){
		func() { NewNodeSpawner[scene.UITransform](ecs.NewWorld()) },
		func() { NewNodeSpawner[scene.Parent](ecs.NewWorld()) },
		func() { NewNodeSpawner2[scene.Size2D, scene.Size2D](ecs.NewWorld()) },
		func() { NewNodeSpawner2[scene.Name, scene.Visibility](ecs.NewWorld()) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("duplicate component was accepted")
				}
			}()
			run()
		}()
	}
}

func TestNodeSpawnersReserveCompleteBurstsWithoutAllocations(t *testing.T) {
	world := ecs.NewWorld()
	nodes := NewNodeSpawner[scene.Size2D](world)
	pairs := NewNodeSpawner2[scene.Size2D, scene.Name](world)
	nodes.Reserve(128)
	pairs.Reserve(128)
	run := func() {
		var entities [128]ecs.Entity
		for i := range entities {
			entities[i] = nodes.Spawn(NodeOptions{X: float32(i)}, scene.Size2D{Width: 10, Height: 20})
		}
		for _, entity := range entities {
			world.Despawn(entity)
		}
		for i := range entities {
			entities[i] = pairs.Spawn(NodeOptions{X: float32(i)}, scene.Size2D{Width: 10, Height: 20}, scene.Name{Value: "node"})
		}
		for _, entity := range entities {
			if world.GetRef[scene.Name](entity).Value != "node" || world.GetRef[scene.Size2D](entity).Width != 10 {
				panic("pair payload not stored")
			}
			world.Despawn(entity)
		}
	}
	if allocs := testing.AllocsPerRun(50, run); allocs != 0 {
		t.Fatalf("reserved node burst allocations = %v", allocs)
	}
}

func TestSpawnNodeCachesStayInTheirOwnWorld(t *testing.T) {
	first, second := ecs.NewWorld(), ecs.NewWorld()
	entity := SpawnNode(first, NodeOptions{}, scene.Name{Value: "first"})
	other := SpawnNode(second, NodeOptions{}, scene.Name{Value: "second"})
	if first.GetRef[scene.Name](entity).Value != "first" || second.GetRef[scene.Name](other).Value != "second" {
		t.Fatal("cached node factory crossed worlds")
	}
	if first.MutationVersion() == 0 || second.MutationVersion() == 0 {
		t.Fatal("typed spawning did not record mutation")
	}
	run := func() {
		worldEntity := SpawnNode(first, NodeOptions{Parent: entity}, scene.Name{Value: "child"})
		worldPair := SpawnNode2(second, NodeOptions{Parent: other}, scene.Size2D{}, scene.Name{Value: "child"})
		first.Despawn(worldEntity)
		second.Despawn(worldPair)
	}
	if allocs := testing.AllocsPerRun(100, run); allocs != 0 {
		t.Fatalf("cached convenience spawning allocations = %v", allocs)
	}
}
