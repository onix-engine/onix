package project

import (
	"os"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
)

func TestScenePreloaderActivatesPreparedScene(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{}`)
	writeTestFile(t, dir, "resources/scenes.json", `{"entry":"loading","scenes":[{"id":"loading","path":"scenes/loading.scene.json"},{"id":"menu","path":"scenes/menu.scene.json"}]}`)
	writeTestFile(t, dir, "resources/scenes/loading.scene.json", `{"nodes":[{"id":"loading","coordinateSpace":"ui"}]}`)
	writeTestFile(t, dir, "resources/scenes/menu.scene.json", `{"nodes":[{"id":"menu","coordinateSpace":"ui"},{"id":"caption","coordinateSpace":"ui","components":{"text":{"value":"Ready"}}}]}`)
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()

	world := ecs.NewWorld()
	active, err := bundle.InstantiateEntryScene(world)
	if err != nil {
		t.Fatal(err)
	}
	preloader, err := bundle.BeginPreloadScene(world, "menu")
	if err != nil {
		t.Fatal(err)
	}
	for !preloader.Ready() {
		if _, err := preloader.Advance(0); err != nil {
			t.Fatal(err)
		}
	}
	preparedCaption, err := preloader.scene.Entity("caption")
	if err != nil {
		t.Fatal(err)
	}
	preparedVisibility, ok := ecs.Get[scene.Visibility](world, preparedCaption)
	if !ok || preparedVisibility.Visible {
		t.Fatalf("prepared caption visibility = %#v, want hidden", preparedVisibility)
	}
	if scene.IsActive(world, preparedCaption) {
		t.Fatal("preloaded scene was allowed to simulate")
	}
	if err := preloader.Activate(active); err != nil {
		t.Fatal(err)
	}
	defer active.Dispose()
	if active.SceneID() != "menu" {
		t.Fatalf("active scene = %q, want menu", active.SceneID())
	}
	if _, err := active.Entity("loading"); err == nil {
		t.Fatal("loading scene entity remained after activation")
	}
	caption, err := active.Entity("caption")
	if err != nil {
		t.Fatal(err)
	}
	visibility, ok := ecs.Get[scene.Visibility](world, caption)
	if !ok || !visibility.Visible {
		t.Fatalf("prepared caption visibility = %#v, want visible", visibility)
	}
	if !scene.IsActive(world, caption) {
		t.Fatal("activated scene remained paused")
	}
}
