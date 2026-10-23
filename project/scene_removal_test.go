package project

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestSceneComponentClearersPreserveProfileAndMigrateOnce(t *testing.T) {
	for _, test := range []struct {
		name             string
		clear            sceneComponentClearer
		removesAnimation bool
	}{
		{"full", clearAuthoredSceneComponents, true},
		{"2d", clear2DSceneComponents, false},
		{"2d_animation", clear2DAnimationSceneComponents, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			world := ecs.NewWorld()
			nodes := ecs.NewSpawner5[scene.Size2D, scene.Sprite2D, scene.Text2D, scene.Animator, scene.Name](world)
			nodes.Reserve(1)
			cleanup := 0
			ecs.OnComponentRemove[scene.Sprite2D](world, func(*ecs.World, ecs.Entity, *scene.Sprite2D) { cleanup++ })
			run := func() {
				entity := nodes.Spawn(scene.Size2D{}, scene.Sprite2D{}, scene.Text2D{}, scene.Animator{}, scene.Name{Value: "retained"})
				before := ecs.ComponentStructureVersion[scene.Name](world)
				test.clear(world, entity)
				if ecs.ComponentStructureVersion[scene.Name](world)-before != 2 || ecs.GetRef[scene.Name](world, entity).Value != "retained" ||
					ecs.Has[scene.Sprite2D](world, entity) || ecs.Has[scene.Text2D](world, entity) || ecs.Has[scene.Size2D](world, entity) ||
					ecs.Has[scene.Animator](world, entity) == test.removesAnimation {
					t.Fatal("scene clear did not preserve its profile or migrate once")
				}
				world.Despawn(entity)
			}
			if allocs := testing.AllocsPerRun(100, run); allocs != 0 {
				t.Fatalf("scene component cleanup allocated %v times", allocs)
			}
			if cleanup != 101 {
				t.Fatalf("sprite cleanup ran %d times, want 101", cleanup)
			}
		})
	}
}
