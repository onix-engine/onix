package editor

import (
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

const (
	tilemapSwatch = 36
	tilemapGap    = 4
	tilemapPad    = 10
	tilemapHeader = 28
	tilemapExitW  = 48
	tilemapExitH  = 20
)

var (
	tilemapPanelFill    = graphics.ColorFromRGBA(20, 24, 32, 235)
	tilemapPanelBorder  = graphics.ColorFromRGB(112, 119, 132)
	tilemapEraserFill   = graphics.ColorFromRGB(46, 51, 62)
	tilemapEraserBorder = graphics.ColorFromRGB(176, 183, 195)
	tilemapArmedColor   = graphics.ColorFromRGB(120, 190, 255)
	tilemapHoverColor   = graphics.ColorFromRGB(255, 209, 102)
	tilemapExitColor    = graphics.ColorFromRGB(176, 183, 195)
)

// workspaceTilemap drives the docked palette and in-canvas painting for the
// selected tilemap. Painting is committed as one undoable edit per stroke.
type workspaceTilemap struct {
	mod                                              project.ModID
	element                                          string
	entity                                           ecs.Entity
	tilemap                                          *scene.Tilemap2D
	texture                                          *graphics.Texture
	cols, rows, width, height, tileWidth, tileHeight int
	matrix                                           mgl32.Mat4
	anchor                                           scene.Anchor
	ui, painting                                     bool
	armed                                            int
	stroke                                           map[int]int
	palette                                          rect
	exit                                             rect
	exitText                                         scene.Text2D
	paletteCols, total                               int
}

func (t *workspaceTilemap) inactive() {
	t.entity, t.tilemap, t.painting, t.stroke = 0, nil, false, nil
}

// tilemapResolveStep returns the first failing step (0 = success).
// 1: base guards, 2: selected mod, 3: tilemap spec, 4: entity, 5: texture/component.
func (a *ModuleEditor) tilemapResolveStep() int {
	t := &a.tilemap
	v := &a.viewport2D
	if v.world == nil || v.err != nil || a.Document == nil || a.hasDrafts() || a.modalVisible() || a.workspaceMenuOpen() || a.dimension == previewDimension3D || a.Selected == "" {
		t.inactive()
		return 1
	}
	mod := a.selectedMod()
	if mod == "" {
		t.inactive()
		return 2
	}
	// Drafts are rejected above, so resolve the committed spec directly.
	spec := a.Document.Mods[mod].Spec
	var found *project.TilemapSpec
	if a.selectedElement == "" {
		found = spec.Components.Tilemap
	} else {
		for i := range spec.Elements {
			if spec.Elements[i].ID == a.selectedElement {
				found = spec.Elements[i].Components.Tilemap
				break
			}
		}
	}
	if found == nil || found.Tileset == "" {
		t.inactive()
		return 3
	}
	var entity ecs.Entity
	for i := range v.rows {
		row := &v.rows[i]
		if row.mod != mod {
			continue
		}
		if a.editingModule.Scene != "" {
			entity = row.root
			if a.selectedElement != "" {
				entity = row.targets[a.selectedElement]
			}
			break
		}
		if row.id == a.Selected {
			entity = row.root
			if a.selectedElement != "" {
				entity = row.targets[a.selectedElement]
			}
			break
		}
	}
	if entity == 0 {
		t.inactive()
		return 4
	}
	tc := v.world.GetRef[scene.Tilemap2D](entity)
	if tc == nil {
		t.inactive()
		return 5
	}
	if tc.Texture == nil || !tc.Texture.Valid() || tc.Columns <= 0 || tc.Width <= 0 || tc.Height <= 0 || tc.TileWidth <= 0 || tc.TileHeight <= 0 {
		t.inactive()
		return 5
	}
	matrix, ui := viewportContentMatrix(v.world, entity)
	anchor, _ := v.world.Get[scene.Anchor](entity)
	same := t.mod == mod && t.element == a.selectedElement
	t.mod, t.element, t.entity = mod, a.selectedElement, entity
	t.tilemap, t.texture, t.matrix, t.anchor, t.ui = tc, tc.Texture, matrix, anchor, ui
	t.cols, t.rows = tc.Columns, tc.Texture.Height/tc.TileHeight
	t.width, t.height, t.tileWidth, t.tileHeight = tc.Width, tc.Height, tc.TileWidth, tc.TileHeight
	if !same {
		t.armed, t.painting, t.stroke = -1, false, nil
	}
	t.layoutPalette()
	return 0
}

func (a *ModuleEditor) resolveTilemap() bool { return a.tilemapResolveStep() == 0 }

func (t *workspaceTilemap) layoutPalette() {
	t.total = t.cols*t.rows + 1
	t.paletteCols = min(max(t.cols, 1), 6)
	rows := (t.total + t.paletteCols - 1) / t.paletteCols
	width := float32(tilemapPad*2 + t.paletteCols*tilemapSwatch + (t.paletteCols-1)*tilemapGap)
	t.palette = rect{
		X: 10, Y: 10,
		W: max(width, float32(tilemapExitW+tilemapPad*2)),
		H: float32(tilemapHeader + tilemapPad*2 + rows*tilemapSwatch + (rows-1)*tilemapGap),
	}
	t.exit = rect{
		X: t.palette.X + t.palette.W - tilemapPad - tilemapExitW,
		Y: t.palette.Y + tilemapPad,
		W: tilemapExitW, H: tilemapExitH,
	}
}

func (t *workspaceTilemap) swatchRect(k int) rect {
	pc, pr := k%t.paletteCols, k/t.paletteCols
	return rect{
		X: t.palette.X + float32(tilemapPad+pc*(tilemapSwatch+tilemapGap)),
		Y: t.palette.Y + float32(tilemapHeader+tilemapPad+pr*(tilemapSwatch+tilemapGap)),
		W: tilemapSwatch, H: tilemapSwatch,
	}
}

func (t *workspaceTilemap) paletteHit(x, y float32) int {
	if x < t.palette.X || y < t.palette.Y || x >= t.palette.X+t.palette.W || y >= t.palette.Y+t.palette.H {
		return -1
	}
	ix, iy := int(x-(t.palette.X+tilemapPad)), int(y-(t.palette.Y+tilemapHeader+tilemapPad))
	if ix < 0 || iy < 0 {
		return -1
	}
	period := tilemapSwatch + tilemapGap
	pc, pr := ix/period, iy/period
	if ix%period >= tilemapSwatch || iy%period >= tilemapSwatch || pc >= t.paletteCols {
		return -1
	}
	if k := pr*t.paletteCols + pc; k >= 0 && k < t.total {
		return k
	}
	return -1
}

func (t *workspaceTilemap) cellCorner(cell int) (float32, float32, float32, float32) {
	base := t.tilemapBase()
	col, row := float32(cell%t.width*t.tileWidth), float32(cell/t.width*t.tileHeight)
	corner := base.Mul4x1(mgl32.Vec4{col, row, 0, 1})
	right := base.Mul4x1(mgl32.Vec4{col + float32(t.tileWidth), row, 0, 1})
	down := base.Mul4x1(mgl32.Vec4{col, row + float32(t.tileHeight), 0, 1})
	w := mgl32.Vec2{right[0] - corner[0], right[1] - corner[1]}.Len()
	h := mgl32.Vec2{down[0] - corner[0], down[1] - corner[1]}.Len()
	return corner[0], corner[1], w, h
}

func tilemapBaseFrom(matrix mgl32.Mat4, anchor scene.Anchor, width, height float32, ui bool) mgl32.Mat4 {
	x, y := anchor.TopLeft(width, height, ui)
	base := matrix.Mul4(mgl32.Translate3D(x, y, 0))
	if !ui {
		base = base.Mul4(mgl32.Scale3D(1, -1, 1))
	}
	return base
}

func (t *workspaceTilemap) tilemapBase() mgl32.Mat4 {
	return tilemapBaseFrom(t.matrix, t.anchor, float32(t.width*t.tileWidth), float32(t.height*t.tileHeight), t.ui)
}

// tilemapCellAt hit-tests a tilemap cell from window coordinates, mirroring the
// projection viewportContentBounds and drawTilemap use so painting matches pixels.
func (a *ModuleEditor) tilemapCellAt(entity ecs.Entity, windowX, windowY float32) (int, bool) {
	v := &a.viewport2D
	if entity == 0 || !viewportContentVisible(v.world, entity) {
		return 0, false
	}
	matrix, ui := viewportContentMatrix(v.world, entity)
	anchor, _ := v.world.Get[scene.Anchor](entity)
	tilemap := v.world.GetRef[scene.Tilemap2D](entity)
	if tilemap == nil || !tilemap.Valid() {
		return 0, false
	}
	bounds := a.canvasBounds()
	x, y := (windowX-bounds.X-v.frame.X)/max(.01, a.canvasScale), (windowY-bounds.Y-v.frame.Y)/max(.01, a.canvasScale)
	if !ui {
		camera, projection, transform, found := scene.PrimaryCamera2DView(v.world)
		if !found {
			return 0, false
		}
		camera.UpdateViewport(v.size[0], v.size[1])
		point, valid := camera.ViewportToWorld2D(transform, projection, mgl32.Vec2{x, y})
		if !valid {
			return 0, false
		}
		x, y = point[0], point[1]
	}
	base := tilemapBaseFrom(matrix, anchor, tilemap.PixelWidth(), tilemap.PixelHeight(), ui)
	a0, b0, c0, d0 := base[0], base[1], base[4], base[5]
	determinant := a0*d0 - b0*c0
	if abs(determinant) < 1e-8 {
		return 0, false
	}
	dx, dy := x-base[12], y-base[13]
	gridX, gridY := (d0*dx-c0*dy)/determinant, (-b0*dx+a0*dy)/determinant
	col := int(math.Floor(float64(gridX) / float64(tilemap.TileWidth)))
	row := int(math.Floor(float64(gridY) / float64(tilemap.TileHeight)))
	if col < 0 || row < 0 || col >= tilemap.Width || row >= tilemap.Height {
		return 0, false
	}
	return row*tilemap.Width + col, true
}

func (t *workspaceTilemap) paint(cell, value int) bool {
	if t.tilemap == nil || cell < 0 || cell >= len(t.tilemap.Tiles) || t.tilemap.Tiles[cell] == value {
		return false
	}
	if t.stroke == nil {
		t.stroke = make(map[int]int)
	}
	t.stroke[cell] = value
	t.tilemap.Tiles[cell] = value
	t.tilemap.Revision++
	return true
}

// tilemapPress is the canvas press entry point: it arms a palette tile or starts
// a paint stroke, and reports whether the press was consumed.
func (a *ModuleEditor) tilemapPress(x, y float32) bool {
	if !a.resolveTilemap() {
		return false
	}
	return a.tilemapExitPress(x, y) || a.tilemapArmPalette(x, y) || a.tilemapPressPaint(x, y)
}

// tilemapExitPress leaves tilemap editing: it closes the module editing space
// and clears the selection, otherwise a root-level tilemap resolves again.
func (a *ModuleEditor) tilemapExitPress(windowX, windowY float32) bool {
	t := &a.tilemap
	bounds := a.canvasBounds()
	x, y := windowX-bounds.X, windowY-bounds.Y
	if x < t.exit.X || y < t.exit.Y || x >= t.exit.X+t.exit.W || y >= t.exit.Y+t.exit.H {
		return false
	}
	t.armed, t.painting = -1, false
	if a.editingModule.Scene != "" {
		a.exitModuleEdit()
	}
	a.selectInstance("")
	return true
}

func (a *ModuleEditor) tilemapArmPalette(windowX, windowY float32) bool {
	t := &a.tilemap
	bounds := a.canvasBounds()
	k := t.paletteHit(windowX-bounds.X, windowY-bounds.Y)
	if k < 0 {
		return false
	}
	if t.armed == k {
		t.armed = -1
	} else {
		t.armed = k
	}
	a.changedCanvas()
	return true
}

// tilemapPressPaint begins a paint stroke on the cell under the press.
func (a *ModuleEditor) tilemapPressPaint(windowX, windowY float32) bool {
	t := &a.tilemap
	if t.armed < 0 {
		return false
	}
	cell, ok := a.tilemapCellAt(t.entity, windowX, windowY)
	if !ok {
		return false
	}
	t.painting = true
	if t.paint(cell, t.armed) {
		a.changedCanvas()
	}
	return true
}

func (a *ModuleEditor) updateTilemapPointer(pointer input.Pointer) {
	t := &a.tilemap
	if !t.painting {
		return
	}
	if pointer.Down || pointer.Released {
		if cell, ok := a.tilemapCellAt(t.entity, pointer.X, pointer.Y); ok && t.paint(cell, t.armed) {
			a.changedCanvas()
		}
	}
	if !pointer.Released {
		return
	}
	t.painting = false
	a.commitTilemapStroke()
}

func tilemapTiles(spec *project.ModSpec, element string) []int {
	if element == "" {
		if spec.Components.Tilemap != nil {
			return spec.Components.Tilemap.Tiles
		}
		return nil
	}
	for i := range spec.Elements {
		if spec.Elements[i].ID == element {
			if spec.Elements[i].Components.Tilemap != nil {
				return spec.Elements[i].Components.Tilemap.Tiles
			}
			return nil
		}
	}
	return nil
}

func (a *ModuleEditor) commitTilemapStroke() {
	t := &a.tilemap
	if len(t.stroke) == 0 {
		return
	}
	mod, element, changes := t.mod, t.element, t.stroke
	t.stroke = nil
	err := a.Document.EditMod(mod, "Paint tilemap", func(spec *project.ModSpec) error {
		tiles := tilemapTiles(spec, element)
		for cell, value := range changes {
			if cell >= 0 && cell < len(tiles) {
				tiles[cell] = value
			}
		}
		return nil
	})
	if err != nil && t.tilemap != nil {
		committedSpec := a.Document.Mods[mod].Spec
		committed := tilemapTiles(&committedSpec, element)
		for cell := range changes {
			if cell >= 0 && cell < len(committed) {
				t.tilemap.Tiles[cell] = committed[cell]
			}
		}
		t.tilemap.Revision++
	}
	a.report(err)
}

func (a *ModuleEditor) drawTilemapPalette(c *graphics.Canvas2D) {
	if !a.resolveTilemap() {
		return
	}
	t := &a.tilemap
	a.drawTilemapHover(c, t)
	c.DrawShapeMatrices(mgl32.Translate3D(t.palette.X, t.palette.Y, 0).Mul4(mgl32.Scale3D(t.palette.W, t.palette.H, 1)), mgl32.Ident3(), t.palette.W, t.palette.H, render.ShapeRoundedRect, tilemapPanelFill, tilemapPanelBorder, 8, 1, 1)
	c.DrawShapeMatrices(mgl32.Translate3D(t.exit.X, t.exit.Y, 0).Mul4(mgl32.Scale3D(t.exit.W, t.exit.H, 1)), mgl32.Ident3(), t.exit.W, t.exit.H, render.ShapeRoundedRect, tilemapEraserFill, tilemapPanelBorder, 4, 1, 1)
	if t.exitText.Value == "" {
		t.exitText = scene.NewText2D("Exit")
		t.exitText.Font, t.exitText.FontSize, t.exitText.Color = a.font, 12, tilemapExitColor
	}
	scene.DrawScreenText2D(c, &t.exitText, mgl32.Translate3D(t.exit.X+t.exit.W/2, t.exit.Y+t.exit.H/2, 0), scene.AnchorCenter)
	bottomLeft := render.CurrentRendererInfo().RenderTargetBottomLeft
	for k := range t.total {
		bounds := t.swatchRect(k)
		model := mgl32.Translate3D(bounds.X, bounds.Y, 0).Mul4(mgl32.Scale3D(bounds.W, bounds.H, 1))
		if k == 0 {
			c.DrawShapeMatrices(model, mgl32.Ident3(), bounds.W, bounds.H, render.ShapeRoundedRect, tilemapEraserFill, tilemapEraserBorder, 4, 1, 1)
		} else {
			a.drawTilemapSwatch(c, bottomLeft, model, k)
		}
		if t.armed == k {
			c.DrawShapeMatrices(mgl32.Translate3D(bounds.X-3, bounds.Y-3, 0).Mul4(mgl32.Scale3D(bounds.W+6, bounds.H+6, 1)), mgl32.Ident3(), bounds.W+6, bounds.H+6, render.ShapeRoundedRect, graphics.ColorRGBA{}, tilemapArmedColor, 4, 2, 1)
		}
	}
}

func (a *ModuleEditor) drawTilemapSwatch(c *graphics.Canvas2D, bottomLeft bool, model mgl32.Mat4, tile int) {
	t := &a.tilemap
	column, row := (tile-1)%t.cols, (tile-1)/t.cols
	fraction := mgl32.Vec2{1 / float32(t.cols), 1 / float32(t.rows)}
	u, v := t.texture.U+float32(column)*fraction[0]*t.texture.MaxU, t.texture.V+float32(row)*fraction[1]*t.texture.MaxV
	width, height := fraction[0]*t.texture.MaxU, fraction[1]*t.texture.MaxV
	if bottomLeft {
		v += height
		height = -height
	}
	c.BindTexture(t.texture)
	c.DrawTintedMatrices(model, mgl32.Translate2D(u, v).Mul3(mgl32.Scale2D(width, height)), graphics.White, 1)
}

func (a *ModuleEditor) drawTilemapHover(c *graphics.Canvas2D, t *workspaceTilemap) {
	if t.ui || (t.armed < 0 && !t.painting) || t.tilemap == nil || t.entity == 0 {
		return
	}
	window := a.worldPointer()
	cell, ok := a.tilemapCellAt(t.entity, window[0], window[1])
	if !ok {
		return
	}
	x, y, w, h := t.cellCorner(cell)
	c.SetViewMatrix(a.viewport2DView())
	c.DrawShapeMatrices(mgl32.Translate3D(x, y, 0).Mul4(mgl32.Scale3D(w, h, 1)), mgl32.Ident3(), w, h, render.ShapeRoundedRect, graphics.ColorRGBA{}, tilemapHoverColor, 0, 2/max(.01, a.canvasScale), 1)
	c.ResetViewMatrix()
}

func (a *ModuleEditor) worldPointer() [2]float32 {
	if in, ok := a.world.GetResource[*input.State](); ok && in != nil {
		pointer := in.Pointer()
		return [2]float32{pointer.X, pointer.Y}
	}
	return [2]float32{}
}
