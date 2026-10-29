package editor

import (
	"fmt"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestWorkspaceOutlinerLongListScrolls(t *testing.T) {
	a := workspaceEditorFixture(t)
	for i := range 100 {
		a.Document.Scene().Spec.Modules = append(a.Document.Scene().Spec.Modules, project.ModInstanceSpec{ID: fmt.Sprintf("row-%d", i), Mod: "empty"})
	}
	world := ecs.NewWorld()
	syncWorkspaceTest(t, a, world)
	control, ok := htmlui.ControlByID(world, a.shell, "scene:main.json")
	if !ok {
		t.Fatal("missing scene heading")
	}
	content, ok := world.Get[htmlui.ScrollContent](control)
	if !ok {
		t.Fatal("scene list has no scroll container")
	}
	scroll, ok := world.Get[scene.UIScroll](content.Area)
	if !ok || !scroll.Vertical || scroll.ContentSize[1] <= scroll.ViewportSize[1] {
		t.Fatalf("long list is not scrollable: %+v", scroll)
	}
	if scroll.Horizontal {
		t.Fatal("scene list has an unnecessary horizontal scrollbar")
	}
	if scroll.ViewportSize[1] > a.layout().Files.H-34 {
		t.Fatalf("list viewport extends beyond panel: %+v", scroll)
	}
	if err := scene.SyncUIGlobalTransformSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	in := input.NewState()
	world.InsertResource(in)
	layout := a.layout()
	in.MoveMouse(float64(layout.Files.X+40), float64(layout.Files.Y+60))
	in.ScrollMouse(0, -1)
	if err := scene.UIScrollSystem("").Run(world); err != nil {
		t.Fatal(err)
	}
	scroll, _ = world.Get[scene.UIScroll](content.Area)
	if scroll.Position[1] >= 0 {
		t.Fatalf("wheel did not scroll the list: %+v", scroll)
	}
	position := scroll.Position
	a.selectInstance("row-20")
	syncWorkspaceTest(t, a, world)
	control, _ = htmlui.ControlByID(world, a.shell, "scene:main.json")
	content, _ = world.Get[htmlui.ScrollContent](control)
	scroll, _ = world.Get[scene.UIScroll](content.Area)
	if scroll.Position != position {
		t.Fatalf("selection reset list scroll: got %v, want %v", scroll.Position, position)
	}
	a.Document.Scene().Spec.Modules = a.Document.Scene().Spec.Modules[:3]
	a.changed()
	syncWorkspaceTest(t, a, world)
	if shortScroll := a.outlinerScroll(world); shortScroll != nil && (shortScroll.Vertical || shortScroll.Position[1] != 0) {
		t.Fatalf("short list retained overflow or invalid scroll position: %+v", shortScroll)
	}
}
