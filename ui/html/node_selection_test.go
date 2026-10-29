package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
)

func TestHTMLNodeLiveSelectionBeforeAndAfterUserInput(t *testing.T) {
	world := ecs.NewWorld()
	surface := New(`<select id="mode"><option selected>A</option><option>B</option></select>`)
	surface.Width, surface.Height = 200, 100
	spawner := world.NewSpawner1[Surface]()
	owner := spawner.Spawn(surface)
	root, err := Root(world, owner)
	if err != nil {
		t.Fatal(err)
	}
	node, ok := root.FindByID("mode")
	if !ok || !node.SetSelectedIndex(1) {
		t.Fatal("selection before rendering failed")
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	control, ok := ControlByID(world, owner, "mode")
	if !ok {
		t.Fatal("select was not rendered")
	}
	value, _ := world.Get[Control](control)
	if value.Selected != 1 || !value.Options[0].DefaultSelected || value.Options[1].DefaultSelected {
		t.Fatal("live selection overwrote authored defaults", value)
	}
	if !SetControlSelected(world, control, 0) || !node.SetSelectedIndex(1) {
		t.Fatal("programmatic selection after user input failed")
	}
	value, _ = world.Get[Control](control)
	if value.Selected != 1 || value.Value != "B" {
		t.Fatal("rendered control retained stale selection", value)
	}
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if !node.SetSelectedIndex(0) || !node.SetSelectedIndex(1) {
			panic("invalid select")
		}
	}); allocations != 0 {
		t.Fatalf("warm selection allocated %g", allocations)
	}
	if !node.SetSelectedIndex(-1) {
		t.Fatal("selection could not be cleared")
	}
	value, _ = world.Get[Control](control)
	if value.Selected != -1 || value.Value != "" {
		t.Fatal("cleared selection retained value", value)
	}
}
