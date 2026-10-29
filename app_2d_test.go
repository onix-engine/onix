package onix

import (
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/project"
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
