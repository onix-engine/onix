package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestHTMLNodeSelectionRefreshesSelectorAndLabel(t *testing.T) {
	world := ecs.NewWorld()
	surface := htmlui.New(`<select id="mode"><option>A</option><option>B</option></select>`)
	surface.Width, surface.Height = 200, 100
	spawner := world.NewSpawner1[htmlui.Surface]()
	owner := spawner.Spawn(surface)
	if err := SyncHTML(world); err != nil {
		t.Fatal(err)
	}
	root, err := htmlui.Root(world, owner)
	if err != nil {
		t.Fatal(err)
	}
	node, _ := root.FindByID("mode")
	control, ok := htmlui.ControlByID(world, owner, "mode")
	if !ok {
		t.Fatal("select missing")
	}
	for _, index := range []int{1, 0, 1, -1, 0} {
		if !node.SetSelectedIndex(index) {
			t.Fatal("selection failed")
		}
		if err := SyncHTML(world); err != nil {
			t.Fatal(err)
		}
		if err := SyncWith[Widgets](world); err != nil {
			t.Fatal(err)
		}
		value, _ := world.Get[htmlui.Control](control)
		selector, _ := world.Get[Selector](control)
		if value.Selected != index || selector.Selected != index {
			t.Fatal("selection was overwritten by UI bridge", value.Selected, selector.Selected)
		}
		want := ""
		if index == 0 {
			want = "A"
		}
		if index == 1 {
			want = "B"
		}
		for _, access := range world.Query2[htmlui.ControlText, scene.Text2D]().All {
			if access.A.Get().Control == control && access.B.Get().Value != want {
				t.Fatal("control label retained previous selection", access.B.Get().Value)
			}
		}
	}
}
