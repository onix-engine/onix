package ui

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
)

func TestImageGridLaysOutCardsAndReportsRemoval(t *testing.T) {
	world := newUIWorld(t)
	items := []ImageGridItem{
		{Label: "1 Image", Removable: true},
		{Label: "2 Image", Removable: true},
		{Label: "3 Image", Removable: true},
		{Label: "4 Image", Removable: false},
	}
	deleteIcon := graphics.CreateTextureByColor(graphics.White)
	deleteIconStyle := TextureIcon(deleteIcon, 14, 14)
	grid := SpawnImageGrid(world, ImageGridOptions{X: 10, Y: 20, Width: 112, Columns: 3, Gap: 8, Items: items, DeleteIcon: deleteIconStyle, Radius: 4})

	component, ok := world.Get[ImageGrid](grid)
	if !ok || component.Width != 112 || component.Columns != 3 || component.DeleteIcon != deleteIconStyle || len(component.Items) != len(items) {
		t.Fatalf("image grid = %#v", component)
	}
	if got, want := ImageGridHeight(component.Width, len(items), component.Columns, component.Gap), float32(72); got != want {
		t.Fatalf("image grid height = %v, want %v", got, want)
	}

	items[0].Label = "changed"
	if component.Items[0].Label != "1 Image" {
		t.Fatalf("image grid must own a copy of its items: %#v", component.Items)
	}

	cells := 0
	var remove ecs.Entity
	for _, access := range world.Query[imageGridCell]().All {
		cell := access.Mut()

		if cell.Grid == grid {
			cells++
		}
	}
	for entity, access := range world.Query[imageGridRemove]().All {
		control := access.Mut()

		if control.Grid == grid && control.Index == 1 {
			remove = entity
			break
		}
	}
	if cells != 4 || remove == 0 {
		t.Fatalf("grid cards = %d, remove control = %v", cells, remove)
	}
	iconFound := false
	for _, access := range world.Query2[scene.Parent, scene.UIImage2D]().All {
		parent := access.A.Mut()
		image := access.B.Mut()

		iconFound = iconFound || parent.Entity == remove && image.Texture == deleteIcon
		if !(!iconFound) {
			break
		}
	}
	if !iconFound {
		t.Fatal("image grid delete icon was not rendered")
	}

	activate(world, remove)
	events := (&ecs.MessageReader[ImageGridEvent]{}).Read(world)
	if len(events) != 1 || events[0] != (ImageGridEvent{Grid: grid, Index: 1}) {
		t.Fatalf("image grid events = %#v", events)
	}
}
