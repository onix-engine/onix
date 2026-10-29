package editor

import (
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

type workspacePlacementDrag struct {
	ID             string
	Element        string
	Mod            project.ModID
	Definition     bool
	WorldSpace     bool
	Entity         ecs.Entity
	Transform      project.ModTransform
	ThreeD         bool
	Inverse3D      mgl32.Mat4
	PlanePoint     mgl32.Vec3
	PlaneNormal    mgl32.Vec3
	StartWorld     mgl32.Vec3
	WorldDelta     mgl32.Vec3
	DeltaZ         float32
	Index, End     int
	StartX, StartY float32
	Position       [3]float32
	Inverse        [4]float32
	Delta          [2]float32
	DisplayDelta   [2]float32
	Moved          bool
	Version        uint64
}

func (a *ModuleEditor) beginPlacementDrag(id string, x, y float32) {
	a.beginCanvasDrag(id, "", x, y)
}

func (a *ModuleEditor) beginCanvasDrag(id, element string, x, y float32) {
	if a.Document == nil || a.hasDrafts() || a.modalVisible() || a.workspaceMenuOpen() {
		return
	}
	instance := a.Document.Instance(id)
	if instance == nil {
		return
	}
	if element != "" {
		a.selectElement(id, element)
		a.expandModule(workspaceModuleKey{Scene: a.Document.Active, ID: id})
		delete(a.sectionsCollapsed, a.treeRowControlID(workspaceTreeRow{Scene: a.Document.Active, ID: id, Group: "elements"}))
		a.revealElement(id, instance.Mod, element)
	} else {
		a.selectInstance(id)
	}
	delete(a.collapsedScenes, a.Document.Active)
	if a.dimension == previewDimension3D {
		a.beginViewport3DDrag(id, element, x, y)
		return
	}
	for index, item := range a.Preview.Items {
		if item.ID != id && !(a.Local != "" && instance.Mod == a.Local) {
			continue
		}
		for parent := item.Parent; parent >= 0; parent = a.Preview.Items[parent].Parent {
			a.expandModule(workspaceModuleKey{Scene: a.Document.Active, ID: a.Preview.Items[parent].ID})
		}
		mod := a.Document.Mods[item.Mod]
		drag := workspacePlacementDrag{ID: id, Element: element, Mod: item.Mod, Index: index, End: index + 1, StartX: x, StartY: y, Definition: a.Local != "" || element != "", WorldSpace: mod.Spec.Space == project.CoordinateSpaceWorld, Version: a.canvasDrag.Version + 1}
		parent := mgl32.Ident4()
		if element != "" {
			row := &a.viewport2D.rows[index]
			drag.Entity = row.targets[element]
			if drag.Entity == 0 {
				return
			}
			for _, part := range mod.Plan.Elements {
				if part.Spec.ID == element {
					drag.Transform = part.Transform
					break
				}
			}
			drag.WorldSpace = !a.viewport2D.world.Has[scene.UITransform](drag.Entity)
			ancestor := a.viewport2D.world.GetRef[scene.Parent](drag.Entity)
			if drag.WorldSpace {
				global := a.viewport2D.world.GetRef[scene.GlobalTransform](ancestor.Entity)
				if global == nil {
					return
				}
				parent = global.Matrix
			} else {
				global := a.viewport2D.world.GetRef[scene.UIGlobalTransform](ancestor.Entity)
				if global == nil {
					return
				}
				parent = global.Matrix
			}
			drag.Position = drag.Transform.Position
		} else if a.Local != "" {
			drag.Entity = a.viewport2D.rows[index].root
			drag.Transform = mod.Plan.Transform
			drag.Position = drag.Transform.Position
		} else if item.Parent >= 0 {
			parent = a.Preview.Items[item.Parent].Placement
		} else {
			parent = previewTransformMatrix(a.Document.Scene().Plan.Transform, mod.Spec.Space)
		}
		determinant := parent[0]*parent[5] - parent[1]*parent[4]
		if math.Abs(float64(determinant)) < 1e-7 {
			return
		}
		drag.Inverse = [4]float32{parent[5] / determinant, -parent[4] / determinant, -parent[1] / determinant, parent[0] / determinant}
		if !drag.Definition {
			copy(drag.Position[:], instance.Placement.Position)
		}
		for drag.End < len(a.Preview.Items) && a.Preview.Items[drag.End].Parent >= int32(index) {
			drag.End++
		}
		a.canvasDrag = drag
		return
	}
}

func (a *ModuleEditor) updatePlacementDrag(pointer input.Pointer) {
	drag := &a.canvasDrag
	if drag.ID == "" {
		return
	}
	if drag.ThreeD {
		a.updateViewport3DDrag(pointer)
		return
	}
	if pointer.Down || pointer.Released {
		x, y := pointer.X-drag.StartX, pointer.Y-drag.StartY
		if abs(x) > 5 || abs(y) > 5 {
			drag.Moved = true
		}
		if drag.Moved {
			x, y = x/max(.01, a.canvasScale), y/max(.01, a.canvasScale)
			if drag.WorldSpace {
				if v := &a.viewport2D; v.world != nil {
					camera, projection, transform, ok := scene.PrimaryCamera2DView(v.world)
					if ok {
						camera.UpdateViewport(v.size[0], v.size[1])
						start, startOK := camera.ViewportToWorld2D(transform, projection, mgl32.Vec2{})
						end, endOK := camera.ViewportToWorld2D(transform, projection, mgl32.Vec2{x, y})
						if startOK && endOK {
							x, y = end[0]-start[0], end[1]-start[1]
						}
					}
				}
			}
			delta := [2]float32{drag.Inverse[0]*x + drag.Inverse[1]*y, drag.Inverse[2]*x + drag.Inverse[3]*y}
			if a.settings.SnapToGrid {
				scale := a.canvasScale
				if drag.WorldSpace && a.viewport2D.world != nil {
					view := a.viewport2DView()
					scale = mgl32.Vec2{view[0], view[1]}.Len()
				}
				step := viewportGridStep(max(.01, scale))
				for i := range delta {
					delta[i] = float32(math.Round(float64((drag.Position[i]+delta[i])/step)))*step - drag.Position[i]
				}
				determinant := drag.Inverse[0]*drag.Inverse[3] - drag.Inverse[1]*drag.Inverse[2]
				x = (drag.Inverse[3]*delta[0] - drag.Inverse[1]*delta[1]) / determinant
				y = (-drag.Inverse[2]*delta[0] + drag.Inverse[0]*delta[1]) / determinant
			}
			if delta != drag.Delta {
				drag.Delta, drag.DisplayDelta = delta, [2]float32{x, y}
				drag.Version++
				a.changedCanvas()
			}
		}
	}
	if !pointer.Released {
		return
	}
	a.finishPlacementDrag()
}

func (a *ModuleEditor) finishPlacementDrag() {
	drag := &a.canvasDrag
	if drag.Moved && (drag.Delta != [2]float32{} || drag.DeltaZ != 0) {
		position := []float32{drag.Position[0] + drag.Delta[0], drag.Position[1] + drag.Delta[1], drag.Position[2] + drag.DeltaZ}
		if drag.Definition {
			a.report(a.Document.EditMod(drag.Mod, "Move module content in preview", func(value *project.ModSpec) error {
				if drag.Element == "" {
					value.Transform.Position = position
				} else {
					for i := range value.Elements {
						if value.Elements[i].ID == drag.Element {
							value.Elements[i].Transform.Position = position
							break
						}
					}
				}
				return nil
			}))
		} else {
			a.report(a.Document.EditInstance(drag.ID, "Move instance in preview", func(value *project.ModInstanceSpec) error { value.Placement.Position = position; return nil }))
		}
	}
	a.cancelPlacementDrag()
	a.changed()
}

func (a *ModuleEditor) cancelPlacementDrag() {
	drag := &a.canvasDrag
	if drag.ID == "" {
		return
	}
	if !drag.ThreeD && drag.Entity != 0 && a.viewport2D.world != nil {
		a.viewport2D.setContentTransform(drag.Entity, drag.Transform)
		a.viewport2D.dragEntity = 0
	}
	drag.ID = ""
	drag.Version++
}
