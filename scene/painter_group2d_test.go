package scene

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/go-gl/mathgl/mgl32"
)

func TestPainterGroupsKeepRenderingAndPickingInTheSameOrder(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 100, false)
	roots := world.NewSpawner1[PainterGroup2D]()
	front, back := roots.Spawn(PainterGroup2D(2)), roots.Spawn(PainterGroup2D(1))
	for i, parent := range []ecs.Entity{front, back} {
		entity := entities[i]
		if err := world.Insert(entity, Parent{Entity: parent}); err != nil {
			t.Fatal(err)
		}
		if err := world.Insert(entity, InputTarget2D{Width: 10, Height: 5}); err != nil {
			t.Fatal(err)
		}
		world.GetMut[UIGlobalTransform](entity).Matrix = mgl32.Ident4()
		world.SetIfChanged(entity, UIStackIndex(i*2000-1000))
	}
	incrementalFrame(t, world, canvas)
	state := RenderWorldFor(world)
	cache := world.ScopeValue[inputPickCache2D, inputPickCache2D]()
	cache.refresh(world, 0, 0)
	cameras := []inputCamera2D{inputPickCamera(1, 0, 0)}
	check := func(want ecs.Entity) {
		t.Helper()
		if state.Items2D[len(state.Items2D)-1].entity != want {
			t.Fatal("rendering ignored inherited painter group")
		}
		target, _, _, hit := pickInputTarget2D(cameras, 1, cache.targets, 1, 1)
		if !hit || target.entity != want {
			t.Fatal("picking disagrees with grouped painter order")
		}
	}
	check(entities[0])
	world.SetIfChanged(front, PainterGroup2D(0))
	incrementalFrame(t, world, canvas)
	if !cache.refresh(world, 0, 0) {
		t.Fatal("group change did not invalidate picking")
	}
	check(entities[1])
}
