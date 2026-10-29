package main

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/module"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

// Load the real project and generated registry without opening an editor/window.
func TestProjectInputAndLayout(t *testing.T) {
	source := os.DirFS(".")
	plan, err := project.CompileProject(source, ".")
	if err != nil {
		t.Fatal(err)
	}
	registry, err := OnixRegistry()
	if err != nil {
		t.Fatal(err)
	}
	world := ecs.NewWorld()
	runtime, err := module.New[OnixContentProfile](world, plan, registry, OnixModuleOptions(source))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err = runtime.Manager().StartEntry(); err != nil {
		t.Fatal(err)
	}
	if err = htmlui.Sync(world); err != nil {
		t.Fatal(err)
	}
	var page ecs.Entity
	for entity := range world.Query[htmlui.Surface]().All {
		page = entity
	}
	if page == 0 {
		t.Fatal("project has no HTML surface")
	}
	nodeText := func(id string) string {
		t.Helper()
		node, ok := htmlui.NodeByID(world, page, id)
		if !ok {
			t.Fatal("missing node", id)
		}
		markup, err := node.Serialize()
		if err != nil {
			t.Fatal(err)
		}
		return markup[strings.IndexByte(markup, '>')+1 : strings.LastIndexByte(markup, '<')]
	}
	assertTileText := func() {
		t.Helper()
		want := make(map[string]int)
		for i := range 16 {
			if value := nodeText("value-" + strconv.Itoa(i)); value != "" && value != "0" {
				want[value]++
			}
		}
		got := make(map[string]int)
		for _, access := range world.Query[scene.Text2D]().All {
			text := access.Get()
			if text.Value != "0" && text.FontSize >= 24 && text.FontSize <= 44 {
				if _, err := strconv.ParseUint(text.Value, 10, 32); err == nil {
					got[text.Value]++
				}
			}
		}
		for value, count := range want {
			if got[value] != count {
				t.Fatalf("tile %s: emitted %d text visuals, want %d", value, got[value], count)
			}
		}
	}
	assertTileText()
	if err := htmlui.Sync(world); err != nil {
		t.Fatal(err)
	}
	assertTileText()
	key := func(code input.KeyCode) {
		t.Helper()
		if err := runtime.HandleInput(scene.InputEvent{Kind: scene.InputEventKeyDown, Key: code}); err != nil {
			t.Fatal(err)
		}
		if err := runtime.Dispatch(); err != nil {
			t.Fatal(err)
		}
		if err := htmlui.Sync(world); err != nil {
			t.Fatal(err)
		}
		assertTileText()
	}
	key(input.KeyLeft)
	if nodeText("moves") == "0" {
		key(input.KeyRight)
	}
	if nodeText("moves") != "1" {
		t.Fatal("keyboard input did not reach the game through the scene")
	}
	key(input.KeyZ)
	if nodeText("moves") != "0" || nodeText("score") != "0" {
		t.Fatal("undo did not update the retained UI")
	}
	key(input.KeyR)
	undo, ok := htmlui.ControlByID(world, page, "undo")
	if !ok {
		t.Fatal("missing undo button")
	}
	control, _ := world.Get[htmlui.Control](undo)
	if !control.Disabled {
		t.Fatal("undo must be disabled after restart")
	}
	// Check logical geometry headlessly; visual acceptance remains manual.
	for _, id := range []string{"cell-0", "cell-15"} {
		node, ok := htmlui.NodeByID(world, page, id)
		if !ok {
			t.Fatal("missing cell", id)
		}
		bounds, ok := node.Bounds()
		if !ok || bounds.Width != 101 || bounds.Height != 101 {
			t.Fatal("unexpected cell geometry", id, bounds)
		}
	}
	if err := runtime.HandleHTMLEvent(htmlui.Event{Entity: page, ID: "right", Kind: htmlui.EventClick}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Dispatch(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.HandleHTMLEvent(htmlui.Event{Entity: page, ID: "left", Kind: htmlui.EventClick}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Dispatch(); err != nil {
		t.Fatal(err)
	}
	if nodeText("moves") == "0" {
		t.Fatal("HTML button events did not reach the game")
	}
}
