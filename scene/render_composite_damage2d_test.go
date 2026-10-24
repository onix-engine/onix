package scene

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
)

func TestComposite2DSparsePreparationMatchesFullRebuild(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 1000, true)
	incrementalFrame(t, world, canvas)
	state := renderWorld(world)
	var view *compositor2D
	for _, candidate := range state.compositors2D {
		view = candidate
	}
	random := rand.New(rand.NewPCG(12, 34))
	for step := range 160 {
		entity := entities[random.IntN(len(entities))]
		// Move and fade the opaque cover as well as the content it hides.
		if step%7 == 0 {
			entity = entities[len(entities)-1]
		}
		switch step % 5 {
		case 0:
			ecs.GetMut[Sprite2D](world, entity).Shape.Fill.R = random.Float32()
		case 1:
			matrix := &ecs.GetMut[UIGlobalTransform](world, entity).Matrix
			matrix[12], matrix[13] = float32(random.IntN(1800)-400), float32(random.IntN(1000)-200)
		case 2:
			ecs.GetMut[Sprite2D](world, entity).Alpha = float32(step%2)*.5 + .5
		case 3:
			clip := &ecs.GetMut[UIGlobalTransform](world, entity).clip
			*clip = clipRect2D{valid: true, minX: 5, minY: 5, maxX: float32(random.IntN(1200)), maxY: float32(random.IntN(700))}
		case 4:
			ecs.GetMut[Sprite2D](world, entity).Width = float32(random.IntN(150) + 1)
		}
		if step%3 == 0 {
			ecs.GetMut[Sprite2D](world, entities[random.IntN(len(entities)-1)]).Shape.Fill.G = random.Float32()
		}
		incrementalFrame(t, world, canvas)
		hashes := make([]uint64, len(view.tiles))
		items := make([][]int, len(view.tiles))
		candidates := make([][]int, len(view.tiles))
		for i := range view.tiles {
			hashes[i] = view.tiles[i].hash
			items[i] = slices.Clone(view.tiles[i].items)
			candidates[i] = slices.Clone(view.tiles[i].candidates)
		}
		coverage, occluded := slices.Clone(view.coverage), view.occluded
		view.prepared = false
		incrementalFrame(t, world, canvas)
		if view.occluded != occluded || !slices.Equal(coverage, view.coverage) {
			t.Fatalf("step %d changed occlusion after full rebuild: %d -> %d", step, occluded, view.occluded)
		}
		for i := range view.tiles {
			if view.tiles[i].hash != hashes[i] || !slices.Equal(items[i], view.tiles[i].items) || !slices.Equal(candidates[i], view.tiles[i].candidates) {
				t.Fatalf("step %d tile %d differs from full rebuild", step, i)
			}
		}
	}
}

func TestComposite2DDenseScrollRestoresSparseIndex(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 1000, false)
	incrementalFrame(t, world, canvas)
	state := renderWorld(world)
	var view *compositor2D
	for _, candidate := range state.compositors2D {
		view = candidate
	}
	for _, entity := range entities {
		ecs.GetMut[UIGlobalTransform](world, entity).Matrix[12] += 128
	}
	incrementalFrame(t, world, canvas)
	if view.spatial {
		t.Fatal("dense scrolling maintained an unused sparse index")
	}
	for range 2 {
		ecs.GetMut[Sprite2D](world, entities[0]).Shape.Fill.R = 1 - ecs.GetRef[Sprite2D](world, entities[0]).Shape.Fill.R
		incrementalFrame(t, world, canvas)
		if !view.spatial {
			t.Fatal("quiet content did not restore spatial indexing")
		}
	}
	if state.Composite2D.PreparedTiles != 2 || state.Composite2D.PreparedItems >= 500 {
		t.Fatalf("post-scroll edit traversed unrelated content: %+v", state.Composite2D)
	}
}

func TestComposite2DSparseWorkAndSkippedGenerations(t *testing.T) {
	world, canvas, _, entities := incrementalRenderFixture(t, 10000, false)
	incrementalFrame(t, world, canvas)
	ecs.GetMut[Sprite2D](world, entities[0]).Shape.Fill = graphics.Black
	incrementalFrame(t, world, canvas)
	state := renderWorld(world)
	if stats := state.Composite2D; stats.PreparedTiles != 1 || stats.PreparedItems >= 1000 {
		t.Fatalf("local edit traversed unrelated content: %+v", stats)
	}
	// A camera that skips generations must rebuild, rather than consuming just
	// the last frame's damage and leaving an earlier edit out of its bins.
	context := ecs.SystemContext{World: world}
	ecs.GetMut[UIGlobalTransform](world, entities[0]).Matrix[12] = 600
	if err := extractRender2D(&context); err != nil {
		t.Fatal(err)
	}
	ecs.GetMut[Sprite2D](world, entities[1]).Shape.Fill = graphics.Black
	incrementalFrame(t, world, canvas)
	if state.Composite2D.PreparedItems != 10000 {
		t.Fatal("skipped generation did not rebuild spatial bins")
	}
	for _, view := range state.compositors2D {
		if position := ecs.ScopeValue[retainedRender2D, retainedRender2D](world).nodes[entities[0]].first; !slices.Contains(view.tiles[4].candidates, position) {
			t.Fatal("earlier movement was lost after a skipped generation")
		}
	}
}
