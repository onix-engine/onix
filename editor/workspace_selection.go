package editor

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/go-gl/mathgl/mgl32"
)

func viewportContentMatrix(world *ecs.World, entity ecs.Entity) (mgl32.Mat4, bool) {
	if global := ecs.GetRef[scene.UIGlobalTransform](world, entity); global != nil {
		return global.Matrix, true
	}
	if global := ecs.GetRef[scene.GlobalTransform](world, entity); global != nil {
		return global.Matrix, false
	}
	return mgl32.Ident4(), false
}

func viewportContentVisible(world *ecs.World, entity ecs.Entity) bool {
	for entity != 0 {
		if visible := ecs.GetRef[scene.Visibility](world, entity); visible != nil && !visible.Visible {
			return false
		}
		parent := ecs.GetRef[scene.Parent](world, entity)
		if parent == nil {
			break
		}
		entity = parent.Entity
	}
	return true
}

// The same live geometry is used for picking and selection overlays. Entities
// without a drawn box report no bounds so callers fall back to their origin.
func viewportContentBounds(world *ecs.World, entity ecs.Entity, matrix mgl32.Mat4, ui bool) (rect, bool) {
	anchor, _ := ecs.Get[scene.Anchor](world, entity)
	var lowX, lowY, highX, highY float32
	ok := false
	if sprite := ecs.GetRef[scene.Sprite2D](world, entity); sprite != nil {
		lowX, lowY, highX, highY, ok = scene.SpriteBounds2D(sprite, anchor, matrix, ui)
	}
	if text := ecs.GetRef[scene.Text2D](world, entity); text != nil {
		lowX, lowY, highX, highY, ok = scene.TextBounds2D(text, anchor, matrix, ui)
	}
	if skeleton := ecs.GetRef[scene.Spine2D](world, entity); skeleton != nil && skeleton.Player != nil {
		if minX, minY, maxX, maxY, valid := skeleton.Player.Bounds(); valid {
			model := scene.SpineModelMatrix2D(matrix, ui).Mul4(mgl32.Translate3D(minX, minY, 0))
			lowX, lowY, highX, highY, ok = scene.BoundsInSpace2D(model, scene.AnchorTopLeft, maxX-minX, maxY-minY, true)
		}
	}
	if !ok {
		if emitter := ecs.GetRef[scene.ParticleEmitter2D](world, entity); emitter != nil {
			lowX, lowY, highX, highY, ok = scene.ParticleEmitterBounds2D(emitter, matrix, ui)
		} else if size := ecs.GetRef[scene.Size2D](world, entity); size != nil && (size.Width > 0 || size.Height > 0) {
			lowX, lowY, highX, highY, ok = scene.BoundsInSpace2D(matrix, anchor, size.Width, size.Height, ui)
		} else if surface := ecs.GetRef[htmlui.Surface](world, entity); surface != nil {
			lowX, lowY, highX, highY, ok = scene.BoundsInSpace2D(matrix, anchor, surface.Width, surface.Height, ui)
		}
	}
	return rect{X: lowX, Y: lowY, W: highX - lowX, H: highY - lowY}, ok
}

func viewportUnionBounds(result *rect, found *bool, value rect) {
	if !*found {
		*result, *found = value, true
		return
	}
	right, bottom := max(result.X+result.W, value.X+value.W), max(result.Y+result.H, value.Y+value.H)
	result.X, result.Y = min(result.X, value.X), min(result.Y, value.Y)
	result.W, result.H = right-result.X, bottom-result.Y
}

func viewportContains(bounds rect, x, y float32) bool {
	return x >= bounds.X && y >= bounds.Y && x <= bounds.X+bounds.W && y <= bounds.Y+bounds.H
}

func viewportSelectionBorderContains(bounds rect, x, y float32) bool {
	outer := rect{X: bounds.X - 6, Y: bounds.Y - 6, W: bounds.W + 12, H: bounds.H + 12}
	inner := rect{X: bounds.X + 6, Y: bounds.Y + 6, W: bounds.W - 12, H: bounds.H - 12}
	return viewportContains(outer, x, y) && !viewportContains(inner, x, y)
}

func (a *ModuleEditor) viewport2DContentBounds(entity ecs.Entity, worldView mgl32.Mat4, fallback bool) (rect, bool) {
	v := &a.viewport2D
	if entity == 0 || !viewportContentVisible(v.world, entity) {
		return rect{}, false
	}
	matrix, ui := viewportContentMatrix(v.world, entity)
	native, ok := viewportContentBounds(v.world, entity, matrix, ui)
	view := worldView
	if ui {
		view = mgl32.Translate3D(v.frame.X, v.frame.Y, 0).Mul4(mgl32.Scale3D(a.canvasScale, a.canvasScale, 1))
	}
	if !ok {
		if !fallback {
			return rect{}, false
		}
		point := view.Mul4x1(matrix.Col(3))
		return rect{X: point[0] - 6, Y: point[1] - 6, W: 12, H: 12}, true
	}
	if ui {
		if clip, clipped := ecs.GetRef[scene.UIGlobalTransform](v.world, entity).ClipRect(); clipped {
			right, bottom := min(native.X+native.W, clip.X+clip.Width), min(native.Y+native.H, clip.Y+clip.Height)
			native.X, native.Y = max(native.X, clip.X), max(native.Y, clip.Y)
			native.W, native.H = right-native.X, bottom-native.Y
			if native.W <= 0 || native.H <= 0 {
				return rect{}, false
			}
		}
	}
	lowX, lowY, highX, highY, ok := scene.BoundsInSpace2D(view.Mul4(mgl32.Translate3D(native.X, native.Y, 0)), scene.AnchorTopLeft, native.W, native.H, true)
	return rect{X: lowX, Y: lowY, W: highX - lowX, H: highY - lowY}, ok
}

func (a *ModuleEditor) viewport2DSelectionBounds(index int, view mgl32.Mat4) (rect, bool) {
	v := &a.viewport2D
	row := &v.rows[index]
	var bounds rect
	found := false
	target := row.root
	end := index + 1
	if a.selectedElement == "" {
		for end < len(a.Preview.Items) && a.Preview.Items[end].Parent >= int32(index) {
			end++
		}
	}
	for i := index; i < end; i++ {
		row := &v.rows[i]
		mod := a.previewDocument().Mods[row.mod]
		if a.selectedElement == "" {
			if value, ok := a.viewport2DContentBounds(row.root, view, false); ok {
				viewportUnionBounds(&bounds, &found, value)
			}
		}
		for j, entity := range row.parts {
			element := &mod.Plan.Elements[j]
			selected := a.selectedElement == "" || element.Spec.ID == a.selectedElement
			if element.Spec.ID == a.selectedElement {
				target = entity
			}
			if element.Parent >= 0 {
				selected = selected || row.selectedParts[element.Parent]
			}
			row.selectedParts[j] = selected
			if selected {
				if value, ok := a.viewport2DContentBounds(entity, view, false); ok {
					viewportUnionBounds(&bounds, &found, value)
				}
			}
		}
	}
	if !found {
		return a.viewport2DContentBounds(target, view, true)
	}
	return bounds, true
}

func drawViewportSelection(c *graphics.Canvas2D, bounds rect) {
	// Keep outlines readable at every zoom, including empty transform nodes.
	bounds.X, bounds.Y, bounds.W, bounds.H = bounds.X-2, bounds.Y-2, max(8, bounds.W+4), max(8, bounds.H+4)
	c.DrawShapeMatrices(mgl32.Translate3D(bounds.X, bounds.Y, 0).Mul4(mgl32.Scale3D(bounds.W, bounds.H, 1)), mgl32.Ident3(), bounds.W, bounds.H, render.ShapeRoundedRect, graphics.ColorRGBA{}, editorTheme.accent, 0, 2, 1)
}

func (a *ModuleEditor) drawViewport2DSelection(c *graphics.Canvas2D) {
	if a.selectedResource != "" || a.selectedSlot != "" || a.slotCreating {
		return
	}
	view := a.viewport2DView()
	for i, row := range a.viewport2D.rows {
		selected := a.isSelected(row.id)
		if a.Local != "" {
			selected = a.Selected == a.editingModule.ID
		}
		if selected {
			if bounds, ok := a.viewport2DSelectionBounds(i, view); ok {
				drawViewportSelection(c, bounds)
			}
		}
	}
}

func (a *ModuleEditor) pressViewport2D(x, y float32) {
	if a.Document == nil || a.viewport2D.world == nil || a.hasDrafts() || a.modalVisible() || a.workspaceMenuOpen() {
		return
	}
	id, element := a.pickViewport2DTarget(x, y)
	// Geometry wins over an existing selection; its border remains a drag target.
	if id == "" && a.Selected != "" && a.selectedResource == "" && a.selectedSlot == "" {
		view, canvas := a.viewport2DView(), a.canvasBounds()
		for i, row := range a.viewport2D.rows {
			if row.id != a.Selected && a.Local == "" {
				continue
			}
			if bounds, ok := a.viewport2DSelectionBounds(i, view); ok && viewportSelectionBorderContains(bounds, x-canvas.X, y-canvas.Y) {
				id, element = a.Selected, a.selectedElement
				break
			}
		}
	}
	if id != "" {
		a.beginCanvasDrag(id, element, x, y)
	} else if !a.resolveTilemap() {
		// An empty-area click keeps the tilemap editor open so blank cells stay paintable.
		a.selectInstance("")
	}
}
