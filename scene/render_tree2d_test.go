package scene

import (
	"testing"

	"github.com/onix-engine/onix/graphics"
)

func TestRenderTree2DRetainsIdentityAndReparents(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 2, false)
	incrementalFrame(t, world, canvas)

	cache := world.ScopeValue[retainedRender2D, retainedRender2D]()
	child := cache.nodes[entities[1]]
	if child == nil || child.id == 0 {
		t.Fatal("render tree did not assign a stable node identity")
	}
	identity := child.id
	state := RenderWorldFor(world)
	if len(state.Cameras) == 0 || !child.renderForCamera(state.Cameras[0], state.Cameras[0].Entity) {
		t.Fatal("render tree did not retain camera visibility metadata")
	}
	if child.renderBounds.Empty() {
		t.Fatal("render tree did not retain paint bounds")
	}

	world.GetMut[Sprite2D](entities[1]).Shape.Fill = graphics.Black
	incrementalFrame(t, world, canvas)
	if got := cache.nodes[entities[1]]; got != child || got.id != identity {
		t.Fatal("content update replaced the retained render node")
	}

	if err := world.Insert(entities[1], Parent{Entity: entities[0]}); err != nil {
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

	state = RenderWorldFor(world)
	for _, item := range state.Items2D {
		if item.entity == entities[1] && item.renderNode != identity {
			t.Fatalf("render item identity = %d, want %d", item.renderNode, identity)
		}
	}
}
