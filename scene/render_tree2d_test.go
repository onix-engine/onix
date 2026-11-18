package scene

import (
	"slices"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
)

func TestRenderTree2DRetainsIdentityAndReparents(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 2, false)
	incrementalFrame(t, world, canvas)

	cache := world.ScopeValue[renderTree2D, renderTree2D]()
	child := cache.nodes[entities[1]]
	if child == nil || child.id == 0 {
		t.Fatal("render tree did not assign a stable node identity")
	}
	identity := child.id
	state := RenderWorldFor(world)
	if len(state.Cameras) == 0 || len(cache.visiblePositions(world, state.Cameras[0], state.Cameras[0].Entity)) != 2 {
		t.Fatal("render tree did not retain camera visibility metadata")
	}

	world.GetMut[Sprite2D](entities[1]).Shape.Fill = graphics.Black
	incrementalFrame(t, world, canvas)
	if got := cache.nodes[entities[1]]; got != child || got.id != identity {
		t.Fatal("content update replaced the retained render node")
	}

	parent := entities[0]
	parents := world.NewSpawner1[Parent]()
	for range 8 {
		parent = parents.Spawn(Parent{Entity: parent})
	}
	if err := world.Insert(entities[1], Parent{Entity: parent}); err != nil {
		t.Fatal(err)
	}
	incrementalFrame(t, world, canvas)
	if child.renderParent != entities[0] {
		t.Fatalf("render tree parent = %v, want %v", child.renderParent, entities[0])
	}

	if !world.SetIfChanged(entities[1], Parent{}) {
		t.Fatal("reparent write was not recorded")
	}
	incrementalFrame(t, world, canvas)
	if child.renderParent != 0 {
		t.Fatalf("render tree retained stale parent %v", child.renderParent)
	}

	if cache.nodes[entities[1]].id != identity {
		t.Fatal("reparenting changed render node identity")
	}
}

func TestRenderTree2DCameraRoutesMixedNodeInPaintOrder(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 2, false)
	if err := world.Insert(entities[0], GlobalTransform{Matrix: mgl32.Ident4()}); err != nil {
		t.Fatal(err)
	}
	if err := world.Insert(entities[0], UITargetCamera{Entity: 2}); err != nil {
		t.Fatal(err)
	}
	incrementalFrame(t, world, canvas)
	tree := world.ScopeValue[renderTree2D, renderTree2D]()
	camera := RenderWorldFor(world).Cameras[0]
	if len(tree.items) != 3 || !tree.nodes[entities[0]].mixedCamera {
		t.Fatal("fixture did not produce world and UI parts for the same node")
	}
	for _, entity := range []ecs.Entity{camera.Entity, 2} {
		camera.Entity = entity
		want := slices.Clone(visibleRenderItems2D(world, tree.items, camera, 1))
		if actual := tree.visiblePositions(world, camera, 1); !slices.Equal(actual, want) {
			t.Fatalf("camera %d selected positions %v, want %v", entity, actual, want)
		}
	}
}
