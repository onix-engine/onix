package editor

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
)

func TestTilemapResolveStep(t *testing.T) {
	a := workspaceEditorFixture(t)

	// Create a minimal tileset PNG for the tilemap.
	tilesetPath := filepath.Join(a.Document.ProjectDir, "button", "assets", "tileset.png")
	if err := os.MkdirAll(filepath.Dir(tilesetPath), 0755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tilesetPath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	if err := a.Document.EditMod("button", "Add tilemap element", func(mod *project.ModSpec) error {
		mod.Elements = append(mod.Elements, project.ModElementSpec{
			ID: "map",
			Components: project.ModComponents{
				Tilemap: &project.TilemapSpec{
					ID:         "map",
					Width:      4,
					Height:     4,
					TileWidth:  16,
					TileHeight: 16,
					Tileset:    "assets/tileset.png",
					Tiles:      make([]int, 16),
				},
			},
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	context := &ecs.SystemContext{World: ecs.NewWorld()}
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}

	// Scene mode: select the module instance and its tilemap element.
	a.selectElement("start", "map")
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if step := a.tilemapResolveStep(); step != 0 {
		t.Fatalf("scene mode resolve failed at step %d", step)
	}

	// Module-edit mode.
	a.enterModuleEdit(a.Document.Active, "start")
	a.selectElement("start", "map")
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if step := a.tilemapResolveStep(); step != 0 {
		t.Fatalf("module-edit mode resolve failed at step %d", step)
	}
	if a.tilemap.tilemap.Alpha != 1 {
		t.Fatalf("tilemap alpha should default to 1, got %v", a.tilemap.tilemap.Alpha)
	}

	// Arm the first real tile (index 1) by clicking the palette.
	bounds := a.canvasBounds()
	paletteX := bounds.X + 10 + tilemapPad + tilemapSwatch + tilemapGap + tilemapSwatch/2
	paletteY := bounds.Y + 10 + tilemapHeader + tilemapPad + tilemapSwatch/2
	if !a.tilemapPress(paletteX, paletteY) {
		t.Fatal("palette click did not arm a tile")
	}
	if a.tilemap.armed != 1 {
		t.Fatalf("expected tile 1 armed, got %d", a.tilemap.armed)
	}

	// Paint the first cell by clicking the center of the tilemap.
	v := &a.viewport2D
	cellX := bounds.X + v.frame.X + a.canvasScale*float32(16)/2
	cellY := bounds.Y + v.frame.Y + a.canvasScale*float32(16)/2
	if !a.tilemapPress(cellX, cellY) {
		t.Fatal("canvas click did not start painting")
	}
	if a.tilemap.tilemap.Tiles[0] != 1 {
		t.Fatalf("expected tile 0 painted to 1, got %d", a.tilemap.tilemap.Tiles[0])
	}
	a.updateTilemapPointer(input.Pointer{X: cellX, Y: cellY, Released: true})

	// A click outside the tilemap must not drop the selection that drives edit mode.
	a.pressViewport2D(bounds.X+bounds.W-4, bounds.Y+bounds.H-4)
	if a.Selected != "start" || a.selectedElement != "map" || !a.resolveTilemap() {
		t.Fatalf("blank click exited tilemap editing: selected %q element %q", a.Selected, a.selectedElement)
	}

	// The palette Exit button closes the editing space and hides the palette.
	ex := a.tilemap.exit
	if !a.tilemapPress(bounds.X+ex.X+ex.W/2, bounds.Y+ex.Y+ex.H/2) {
		t.Fatal("palette exit button did not consume the press")
	}
	if a.editingModule.Scene != "" || a.Selected != "" || a.resolveTilemap() {
		t.Fatalf("palette exit button stayed in the editing space: module %+v selected %q", a.editingModule, a.Selected)
	}

	// A root-level tilemap resolves straight from the instance, so Exit must clear the selection.
	if err := a.Document.EditMod("button", "Add root tilemap", func(mod *project.ModSpec) error {
		mod.Components.Tilemap = &project.TilemapSpec{
			ID: "root", Width: 4, Height: 4,
			TileWidth: 16, TileHeight: 16,
			Tileset: "assets/tileset.png",
			Tiles:   make([]int, 16),
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.selectInstance("start")
	if err := a.sync(context); err != nil {
		t.Fatal(err)
	}
	if !a.resolveTilemap() {
		t.Fatal("root-level tilemap did not open the palette")
	}
	ex = a.tilemap.exit
	if !a.tilemapPress(bounds.X+ex.X+ex.W/2, bounds.Y+ex.Y+ex.H/2) {
		t.Fatal("exit did not close the root-level palette")
	}
	if a.Selected != "" || a.resolveTilemap() {
		t.Fatalf("exit left the root-level palette open: selected %q", a.Selected)
	}
}
