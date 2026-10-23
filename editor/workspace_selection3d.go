package editor

import (
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	"github.com/go-gl/mathgl/mgl32"
)

func (v *workspaceViewport3D) measurePart(part *workspace3DPart) {
	mesh := ecs.GetRef[render3d.Mesh](v.world, part.entity)
	if mesh == nil || mesh.Mesh == nil {
		return
	}
	part.low, part.high = mgl32.Vec3{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}, mgl32.Vec3{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	for _, vertex := range mesh.Mesh.Vertices {
		for axis, value := range vertex.Position {
			part.low[axis], part.high[axis] = min(part.low[axis], value), max(part.high[axis], value)
		}
	}
}

func (a *ModuleEditor) viewport3DSelectionBounds(index int) (rect, bool) {
	bounds, found, _ := a.viewport3DSelectionGeometry(index)
	return bounds, found
}

func (a *ModuleEditor) viewport3DSelectionGeometry(index int) (rect, bool, bool) {
	v, canvas := &a.viewport3D, a.canvasBounds()
	projection, _ := ecs.Get[scene.Projection](v.world, v.view.camera)
	camera := preview3DCameraTransformAround(v.view.target, v.view.yaw, v.view.pitch, v.view.distance/a.zoom).LocalMatrix()
	clipFromWorld := projection.Matrix(canvas.W, canvas.H).Mul4(camera.Inv())
	var bounds rect
	found, hasMesh, hasHelper := false, false, false
	var helpers preview3DHelperGeometry
	end, target := index+1, a.Preview.Items[index].Content.Col(3)
	targetVisible := a.Preview.Items[index].Visible
	if drag := &a.canvasDrag; drag.ID != "" && drag.ThreeD && index == drag.Index && drag.Element == "" {
		target = target.Add(drag.WorldDelta.Vec4(0))
	}
	if a.selectedElement == "" {
		for end < len(a.Preview.Items) && a.Preview.Items[end].Parent >= int32(index) {
			end++
		}
	}
	for i := index; i < end; i++ {
		row := &v.rows[i]
		mod := a.previewDocument().Mods[row.mod]
		for j := range row.parts {
			part := &row.parts[j]
			part.selected = a.selectedElement == "" || j > 0 && mod.Plan.Elements[j-1].Spec.ID == a.selectedElement
			if j > 0 && mod.Plan.Elements[j-1].Parent >= 0 {
				part.selected = part.selected || row.parts[mod.Plan.Elements[j-1].Parent+1].selected
			}
			if j > 0 && mod.Plan.Elements[j-1].Spec.ID == a.selectedElement {
				targetVisible = targetVisible && part.visible
				target = a.Preview.Items[i].Content.Mul4(part.matrix).Col(3)
				if drag := &a.canvasDrag; drag.ID != "" && drag.ThreeD {
					target = target.Add(drag.WorldDelta.Vec4(0))
				}
			}
			if !part.selected || part.entity == 0 {
				continue
			}
			mesh := ecs.GetRef[render3d.Mesh](v.world, part.entity)
			visible := ecs.GetRef[scene.Visibility](v.world, part.entity)
			if visible == nil || !visible.Visible {
				continue
			}
			if part.helper {
				hasHelper = true
				a.viewport3DHelperGeometry(part, &projection, camera, canvas, &helpers)
				for _, line := range helpers.lines[:helpers.count] {
					if start, end, ok := preview3DProjectHelperLine(clipFromWorld, canvas, line); ok {
						viewportUnionBounds(&bounds, &found, rect{X: start.X(), Y: start.Y()})
						viewportUnionBounds(&bounds, &found, rect{X: end.X(), Y: end.Y()})
					}
				}
			}
			if mesh == nil || mesh.Mesh == nil {
				continue
			}
			hasMesh = true
			matrix := clipFromWorld.Mul4(ecs.GetRef[scene.GlobalTransform](v.world, part.entity).Matrix)
			for corner := range 8 {
				point := part.low.Vec4(1)
				for axis := range 3 {
					if corner&(1<<axis) != 0 {
						point[axis] = part.high[axis]
					}
				}
				point = matrix.Mul4x1(point)
				if point[3] <= 1e-6 || point[2] < 0 || point[2] > point[3] {
					continue
				}
				viewportUnionBounds(&bounds, &found, rect{X: canvas.X + (point[0]/point[3]+1)*canvas.W*.5, Y: canvas.Y + (1-point[1]/point[3])*canvas.H*.5})
			}
		}
	}
	if !found && targetVisible {
		point := clipFromWorld.Mul4x1(target)
		if point[3] > 1e-6 && point[2] >= 0 && point[2] <= point[3] {
			bounds = rect{X: canvas.X + (point[0]/point[3]+1)*canvas.W*.5 - 6, Y: canvas.Y + (1-point[1]/point[3])*canvas.H*.5 - 6, W: 12, H: 12}
			found = true
		}
	}
	// Helpers use their orange wires; meshes and empty transforms retain frames.
	return bounds, found, hasMesh || !hasHelper
}

func (a *ModuleEditor) drawViewport3DSelection(c *graphics.Canvas2D) {
	if a.selectedResource != "" || a.selectedSlot != "" || a.slotCreating {
		return
	}
	bounds := a.canvasBounds()
	c.PushClipRect(bounds.X, bounds.Y, bounds.W, bounds.H)
	for i, row := range a.viewport3D.rows {
		if a.isSelected(row.id) || a.Local != "" && a.Selected == a.editingModule.ID {
			if bounds, ok, frame := a.viewport3DSelectionGeometry(i); ok && frame {
				drawViewportSelection(c, bounds)
			}
		}
	}
	c.PopClipRect()
}

func (a *ModuleEditor) pressViewport3D(x, y float32) {
	if a.Document == nil || a.viewport3D.world == nil || a.hasDrafts() || a.modalVisible() || a.workspaceMenuOpen() {
		return
	}
	id, element := a.pickViewport3DTarget(x, y)
	if id == "" && a.Selected != "" && a.selectedResource == "" && a.selectedSlot == "" {
		for i, row := range a.viewport3D.rows {
			if row.id != a.Selected && a.Local == "" {
				continue
			}
			if bounds, ok, frame := a.viewport3DSelectionGeometry(i); ok && frame && viewportSelectionBorderContains(bounds, x, y) {
				id, element = a.Selected, a.selectedElement
				break
			}
		}
	}
	if id != "" {
		a.beginCanvasDrag(id, element, x, y)
	} else {
		a.selectInstance("")
	}
}
