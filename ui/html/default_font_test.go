package html

import (
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/scene"
	onixtext "github.com/diiyw/onix/text"
)

func TestHTMLUsesSharedSystemFontWithoutExplicitFont(t *testing.T) {
	font := onixtext.DefaultFont()
	if font == nil {
		t.Skip("system font unavailable")
	}
	world := ecs.NewWorld()
	page := New(`<div>System font</div>`)
	page.Width, page.Height = 200, 80
	spawn := world.NewSpawner1[Surface]()
	spawn.Spawn(page)
	if err := Sync(world); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, access := range world.Query[scene.Text2D]().All {
		value := access.Ref()
		if value.Font != font {
			t.Fatal("HTML did not use the shared system font")
		}
		found = found || value.Value != ""
	}
	if !found {
		t.Fatal("HTML produced no text without an explicit font")
	}
}
