package onix

import (
	"slices"
	"testing"
	"testing/fstest"

	"github.com/go-gl/mathgl/mgl32"
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/project"
	"github.com/onix-engine/onix/render"
	"github.com/onix-engine/onix/scene"
)

func TestLoadProjectRejectsMissingSceneSystemsBeforeWindowCreation(t *testing.T) {
	source := fstest.MapFS{
		"resources/project.json":           {Data: []byte(`{"window":{"width":64,"height":64}}`)},
		"resources/resources.json":         {Data: []byte(`{}`)},
		"resources/scenes.json":            {Data: []byte(`{"entry":"main","scenes":[{"id":"main","path":"scenes/main.scene.json"}]}`)},
		"resources/scenes/main.scene.json": {Data: []byte(`{"nodes":[{"id":"sprite","components":{"sprite":{"texture":{"color":"#ffffff"}}}}]}`)},
	}
	bundle, err := project.Load(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	app := NewApp()
	if _, err := app.LoadScene(bundle); err == nil {
		t.Fatal("scene loaded without its required render system")
	}
	if app.Context != nil {
		t.Fatal("missing system validation created a window context")
	}
}

func TestLoadSceneRejectsNilApp(t *testing.T) {
	if _, err := (*App)(nil).LoadScene(nil); err == nil {
		t.Fatal("nil app was accepted")
	}
}

func TestRender2DDoesNotInstallOptionalRenderPipelines(t *testing.T) {
	app := NewApp()
	if err := app.InstallSystems[Render2D](); err != nil {
		t.Fatal(err)
	}
	order, err := app.Schedule(StageRender).Order()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range order {
		switch name {
		case "extract_render_3d", "prepare_render_resources_3d", "sort_render_phases_3d":
			t.Fatalf("2D installation unexpectedly included %q", name)
		}
	}
}

func TestRenderText2DUpgradesToFullRendering(t *testing.T) {
	app := NewApp()
	defer app.Destroy()
	canvas := graphics.NewCanvas2D(render.ActiveDevice(), 320, 160)
	t.Cleanup(graphics.SetCurrentContextForTesting(canvas))
	graphics.BindFrame(app.World, canvas)
	texts := app.World.NewSpawner3[scene.Text2D, scene.UIGlobalTransform, scene.Visibility]()
	sprites := app.World.NewSpawner3[scene.Sprite2D, scene.UIGlobalTransform, scene.Visibility]()
	transform := scene.UIGlobalTransform{Matrix: mgl32.Ident4()}
	visible := scene.Visibility{Visible: true}
	text := texts.Spawn(scene.NewText2D("hello world"), transform, visible)
	sprite := sprites.Spawn(scene.Sprite2D{Shape: scene.RoundedRectShape2D(graphics.White, 1), Width: 8, Height: 8, Alpha: 1}, transform, visible)

	check := func(full bool) {
		t.Helper()
		if err := app.InstallSystems[RenderText2D](); err != nil {
			t.Fatal(err)
		}
		if got := app.hasProjectFeature(project.FeatureRender2D); got != full {
			t.Fatalf("full renderer feature=%t, want %t", got, full)
		}
		var cameras int
		for _, camera := range app.World.Query[scene.Camera](ecs.With[scene.Camera2D]()).All {
			if camera.Ref().Fallback {
				cameras++
			}
		}
		order, err := app.Schedule(StageRender).Order()
		extract, draw := slices.Index(order, "extract_render_2d"), slices.Index(order, "render_cameras")
		if err != nil || cameras != 1 || extract < 0 || draw <= extract {
			t.Fatalf("fallback cameras=%d, render order=%v, err=%v", cameras, order, err)
		}
		if err := scene.ExtractRender2DSystem("").Run(app.World); err != nil {
			t.Fatal(err)
		}
		var hasText, hasSprite bool
		items := scene.RenderWorldFor(app.World).Items2D
		for i := range items {
			switch items[i].Entity() {
			case text:
				hasText = true
			case sprite:
				hasSprite = true
			}
		}
		if !hasText || hasSprite != full {
			t.Fatalf("full=%t: extracted text=%t sprite=%t", full, hasText, hasSprite)
		}
	}
	check(false)
	check(false)
	if err := app.InstallSystems[Render2D](); err != nil {
		t.Fatal(err)
	}
	check(true)
}
