package ui

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
	"testing"
)

func TestVisualSettersPreserveFieldsAndSkipEqualWrites(t *testing.T) {
	world := ecs.NewWorld()
	spawner := ecs.NewSpawner3[scene.Text2D, scene.UITransform, scene.Size2D](world)
	transform := scene.NewUITransform()
	transform.Scale, transform.Rotation = mgl32.Vec2{2, 3}, 0.5
	entity := spawner.Spawn(scene.Text2D{Value: "old", FontSize: 18, Spans: []scene.TextSpan2D{{}}}, transform, scene.Size2D{Width: 10, Height: 20})
	if !SetText(world, entity, "new") || !SetPosition(world, entity, scene.Px2(30, 40)) || !SetSize(world, entity, 50, 60) {
		t.Fatal("changed fields did not submit")
	}
	text := ecs.GetRef[scene.Text2D](world, entity)
	next := ecs.GetRef[scene.UITransform](world, entity)
	if text.FontSize != 18 || len(text.Spans) != 1 || next.Scale != transform.Scale || next.Rotation != transform.Rotation {
		t.Fatal("setters overwrote unrelated fields")
	}
	versions := [3]uint64{ecs.ComponentVersion[scene.Text2D](world), ecs.ComponentVersion[scene.UITransform](world), ecs.ComponentVersion[scene.Size2D](world)}
	run := func() {
		if SetText(world, entity, "new") || SetPosition(world, entity, scene.Px2(30, 40)) || SetSize(world, entity, 50, 60) {
			panic("equal update submitted")
		}
	}
	if allocs := testing.AllocsPerRun(100, run); allocs != 0 {
		t.Fatalf("stable visual allocations = %v", allocs)
	}
	if versions != [3]uint64{ecs.ComponentVersion[scene.Text2D](world), ecs.ComponentVersion[scene.UITransform](world), ecs.ComponentVersion[scene.Size2D](world)} {
		t.Fatal("equal setter marked Changed")
	}
	world.Despawn(entity)
	if SetText(world, entity, "missing") || SetPosition(world, entity, scene.Px2(1, 2)) || SetSize(world, entity, 1, 2) {
		t.Fatal("stale entity was modified")
	}
}
