package editor

import (
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func (a *ModuleEditor) beginViewport3DDrag(id, element string, x, y float32) {
	instance := a.Document.Instance(id)
	if instance == nil {
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
		drag := workspacePlacementDrag{ID: id, Mod: item.Mod, Element: element, Index: index, End: index + 1, StartX: x, StartY: y, ThreeD: true, Definition: a.Local != "" || element != "", Version: a.canvasDrag.Version + 1}
		parent := mgl32.Ident4()
		drag.PlanePoint = item.Content.Col(3).Vec3()
		if element != "" {
			for i, part := range mod.Plan.Elements {
				if part.Spec.ID != element {
					continue
				}
				drag.Transform, drag.Position = part.Transform, part.Transform.Position
				parent = item.Content
				if part.Parent >= 0 {
					parent = parent.Mul4(a.viewport3D.rows[index].parts[part.Parent+1].matrix)
				}
				drag.PlanePoint = item.Content.Mul4(a.viewport3D.rows[index].parts[i+1].matrix).Col(3).Vec3()
				break
			}
		} else if a.Local != "" {
			drag.Transform, drag.Position = mod.Plan.Transform, mod.Plan.Transform.Position
		} else {
			copy(drag.Position[:], instance.Placement.Position)
			parent = previewTransformMatrix(a.Document.Scene().Plan.Transform, project.CoordinateSpaceWorld)
			if item.Parent >= 0 {
				parent = a.Preview.Items[item.Parent].Placement
			}
		}
		if abs(parent.Det()) < 1e-8 {
			return
		}
		drag.Inverse3D = parent.Inv()
		v := &a.viewport3D
		camera := preview3DCameraTransformAround(v.view.target, v.view.yaw, v.view.pitch, v.view.distance/a.zoom).LocalMatrix()
		drag.PlaneNormal = camera.Col(2).Vec3().Normalize()
		point, ok := a.viewport3DDragPoint(&drag, x, y)
		if !ok {
			return
		}
		drag.StartWorld = point
		for drag.End < len(a.Preview.Items) && a.Preview.Items[drag.End].Parent >= int32(index) {
			drag.End++
		}
		a.canvasDrag = drag
		return
	}
}

func (a *ModuleEditor) viewport3DDragPoint(drag *workspacePlacementDrag, x, y float32) (mgl32.Vec3, bool) {
	v, bounds := &a.viewport3D, a.canvasBounds()
	projection, _ := ecs.Get[scene.Projection](v.world, v.view.camera)
	camera := preview3DCameraTransformAround(v.view.target, v.view.yaw, v.view.pitch, v.view.distance/a.zoom).LocalMatrix()
	ray, ok := preview3DScreenRay(bounds, x, y, projection, camera)
	if !ok {
		return mgl32.Vec3{}, false
	}
	denominator := ray.direction.Dot(drag.PlaneNormal)
	if abs(denominator) < 1e-6 {
		return mgl32.Vec3{}, false
	}
	distance := drag.PlanePoint.Sub(ray.origin).Dot(drag.PlaneNormal) / denominator
	return ray.origin.Add(ray.direction.Mul(distance)), distance >= 0
}

func (a *ModuleEditor) updateViewport3DDrag(pointer input.Pointer) {
	drag := &a.canvasDrag
	if pointer.Down || pointer.Released {
		if abs(pointer.X-drag.StartX) > 5 || abs(pointer.Y-drag.StartY) > 5 {
			drag.Moved = true
		}
		if point, ok := a.viewport3DDragPoint(drag, pointer.X, pointer.Y); ok && drag.Moved {
			worldDelta := point.Sub(drag.StartWorld)
			delta := drag.Inverse3D.Mul4x1(worldDelta.Vec4(0)).Vec3()
			if a.settings.SnapToGrid {
				step := a.viewport3DSnapStep()
				for axis := range delta {
					delta[axis] = float32(math.Round(float64((drag.Position[axis]+delta[axis])/step)))*step - drag.Position[axis]
				}
				worldDelta = drag.Inverse3D.Inv().Mul4x1(delta.Vec4(0)).Vec3()
			}
			if worldDelta != drag.WorldDelta {
				drag.WorldDelta, drag.Delta, drag.DeltaZ = worldDelta, [2]float32{delta[0], delta[1]}, delta[2]
				drag.Version++
				a.changedCanvas()
			}
		}
	}
	if pointer.Released {
		a.finishPlacementDrag()
	}
}
