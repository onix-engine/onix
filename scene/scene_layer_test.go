package scene

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/smallsort"
	"github.com/go-gl/mathgl/mgl32"
)

func TestSceneLayersOrderWorldAndUIAndIsolateCameras(t *testing.T) {
	restore := graphics.SetCurrentContextForTesting(graphics.Canvas2DContext(320, 180))
	t.Cleanup(restore)
	w := ecs.NewWorld()
	graphics.BindFrame(w, graphics.CurrentContext())
	RegisterSpriteRenderer2D(w)
	roots := ecs.NewSpawner2[SceneLayer, Visibility](w)
	back := roots.Spawn(SceneLayer{ID: 1, Order: 1}, Visibility{Visible: true})
	front := roots.Spawn(SceneLayer{ID: 2, Order: 2, Overlay: true}, Visibility{Visible: true})
	fallback := SpawnCamera2D(w)
	ecs.GetMut[Camera](w, fallback).Fallback = true
	frontCamera := SpawnCamera2D(w)
	if err := ecs.Insert(w, frontCamera, Parent{Entity: front}); err != nil {
		t.Fatal(err)
	}
	// Local UI order must not put an old scene above the foreground's world.
	sprite := NewSprite2D()
	sprite.Width, sprite.Height = 20, 20
	sprite.Shape = RoundedRectShape2D(graphics.White, 0)
	ui := ecs.NewSpawner5[Sprite2D, UIGlobalTransform, Visibility, Parent, UIStackIndex](w)
	uiEntity := ui.Spawn(sprite, UIGlobalTransform{Matrix: mgl32.Ident4()}, Visibility{Visible: true}, Parent{Entity: back}, 1000)
	world := ecs.NewSpawner4[Sprite2D, GlobalTransform, Visibility, Parent](w)
	worldEntity := world.Spawn(sprite, GlobalTransform{Matrix: mgl32.Ident4()}, Visibility{Visible: true}, Parent{Entity: front})
	if err := ExtractCamerasSystem("").Run(w); err != nil {
		t.Fatal(err)
	}
	if err := ExtractRender2DSystem("").Run(w); err != nil {
		t.Fatal(err)
	}
	state := renderWorld(w)
	if len(state.Cameras) != 2 || state.Cameras[0].Scene.ID != 1 || state.Cameras[1].Scene.ID != 2 || state.Cameras[1].Camera.Clear {
		t.Fatalf("overlay/fallback cameras: %+v", state.Cameras)
	}
	if len(state.Items2D) != 2 || state.Items2D[0].entity != uiEntity || state.Items2D[1].entity != worldEntity {
		t.Fatal("foreground world was sorted below background UI")
	}
	for _, camera := range state.Cameras {
		items := visibleRenderItems2D(w, state.Items2D, camera, camera.Entity)
		if len(items) != 1 || items[0].scene.ID != camera.Scene.ID {
			t.Fatal("camera collected another scene's content")
		}
	}
}

func TestRenderBatchesDoNotCombineSeparateScenes(t *testing.T) {
	w := ecs.NewWorld()
	items := []renderItem2D{
		{scene: SceneLayer{ID: 2, Order: 2}, order: 1},
		{scene: SceneLayer{ID: 1, Order: 1}, order: 10},
	}
	smallsort.SortFunc(items, compareRenderPhaseItem2D[renderItem2D])
	batches := 0
	drawRenderPhase2D(graphics.Canvas2DContext(320, 180), w, items, DrawViews2D{}, false, func(*renderItem2D) bool { return true }, func(batch renderBatch2D, _ renderViewport2D, begin bool) {
		if !begin || batch.end-batch.start != 1 {
			t.Fatal("batch crossed a scene boundary")
		}
		batches++
	})
	if batches != 2 {
		t.Fatal("missing scene batch")
	}
}
