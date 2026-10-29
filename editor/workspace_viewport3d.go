package editor

import (
	"fmt"
	"math"
	"slices"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	"github.com/go-gl/mathgl/mgl32"
)

type workspace3DMod struct {
	workspaceModResources
	revision uint64
	used     uint64
	cameras  map[*project.Camera3DSpec]workspace3DCamera
}

type workspace3DPart struct {
	entity     ecs.Entity
	matrix     mgl32.Mat4
	visible    bool
	selected   bool
	low, high  mgl32.Vec3
	cameraSpec *project.Camera3DSpec
	cameraID   string
	helper     bool
}

type workspace3DRow struct {
	id       string
	mod      project.ModID
	revision uint64
	parts    []workspace3DPart
	selected bool
}

// Authored meshes, lights and camera transforms enter this world. No module
// behavior, simulation, event routes or project cameras run in the viewport.
type workspaceViewport3D struct {
	world       *ecs.World
	view        editorViewport3D
	mods        map[project.ModID]workspace3DMod
	retired     []*project.Resources
	rows        []workspace3DRow
	structure   uint64
	generation  uint64
	revision    uint64
	dragVersion uint64
	local       project.ModID
	err         error
	navigating  bool
}

func (v *workspaceViewport3D) dispose() {
	for _, mod := range v.mods {
		_ = mod.resources.Dispose()
	}
	for _, resources := range v.retired {
		_ = resources.Dispose()
	}
	for _, texture := range [...]*graphics.Texture{v.view.axisTexture, v.view.studioTexture} {
		if texture != nil {
			_ = texture.Dispose()
		}
	}
	if v.world != nil {
		if state, ok := v.world.GetResource[*render3d.World](); ok {
			_ = state.Dispose()
		}
	}
	_ = v.view.disposeTarget()
	*v = workspaceViewport3D{}
}

func (a *ModuleEditor) initViewport3D() error {
	v := &a.viewport3D
	v.world, v.mods = ecs.NewWorld(), make(map[project.ModID]workspace3DMod)
	render3d.Register(v.world)
	v.view.distance, v.view.target = 14, mgl32.Vec3{4, 4, 0}
	v.view.labels = make(map[string]*scene.Text2D)
	bundle := ecs.NewBundle(7)
	camera := scene.NewCamera()
	camera.ClearColor = editorTheme.viewport3DBG
	bundle.Put(camera)
	bundle.Put(render3d.Camera{})
	bundle.Put(scene.PerspectiveProjection3D())
	bundle.Put(render3d.Environment{})
	bundle.Put(render3d.ViewLighting{})
	bundle.Put(render3d.GroundPlane{})
	bundle.Put(scene.GlobalTransform{Matrix: preview3DCameraTransformAround(v.view.target, v.view.yaw, v.view.pitch, v.view.distance).LocalMatrix()})
	v.view.camera = v.world.Spawn(bundle)
	a.setupPreview3DGuides()
	return nil
}

func (v *workspaceViewport3D) removeRow(row *workspace3DRow) {
	for _, part := range row.parts {
		if part.entity != 0 {
			v.world.Despawn(part.entity)
		}
	}
	*row = workspace3DRow{}
}

func (a *ModuleEditor) syncViewport3D() (resultErr error) {
	v := &a.viewport3D
	defer func() { v.err = resultErr }()
	initialized := v.world == nil
	if initialized {
		if err := a.initViewport3D(); err != nil {
			return err
		}
	}
	graphics.BindFrame(v.world, graphics.CanvasFor(a.world))
	if !initialized && v.revision == a.Preview.Revision && v.structure == a.Preview.structure && v.dragVersion == a.canvasDrag.Version {
		return v.err
	}
	if v.structure != a.Preview.structure || len(v.rows) != len(a.Preview.Items) {
		for i := range v.rows {
			v.removeRow(&v.rows[i])
		}
		v.rows = slices.Grow(v.rows[:0], len(a.Preview.Items))[:len(a.Preview.Items)]
		clear(v.rows)
		v.structure = a.Preview.structure
	}
	v.generation++
	for index, item := range a.Preview.Items {
		row, mod := &v.rows[index], a.previewDocument().Mods[item.Mod]
		if row.id != item.ID || row.mod != item.Mod || row.revision != item.ContentRevision {
			v.removeRow(row)
			if mod.Spec.Space == project.CoordinateSpaceWorld {
				row.parts = make([]workspace3DPart, len(mod.Plan.Elements)+1)
				root, err := a.spawnViewport3DContent(item.Mod, item.ContentRevision, -1, mod.Spec.Components)
				if err != nil {
					return fmt.Errorf("3D preview %s: %w", item.Mod, err)
				}
				row.parts[0] = workspace3DPart{entity: root, matrix: mgl32.Ident4(), visible: true, cameraSpec: mod.Spec.Components.Camera, cameraID: item.ID, helper: viewport3DHasHelpers(mod.Spec.Components)}
				v.measurePart(&row.parts[0])
				for i, element := range mod.Plan.Elements {
					part := &row.parts[i+1]
					part.matrix = previewTransformMatrix(element.Transform, project.CoordinateSpaceWorld)
					part.visible = element.Spec.Visible == nil || *element.Spec.Visible
					if element.Parent >= 0 {
						parent := &row.parts[element.Parent+1]
						part.matrix = parent.matrix.Mul4(part.matrix)
						part.visible = part.visible && parent.visible
					}
					if element.Spec.Space != project.CoordinateSpaceUI {
						part.helper = viewport3DHasHelpers(element.Spec.Components)
						part.cameraSpec = element.Spec.Components.Camera
						if part.cameraSpec != nil {
							part.cameraID = item.ID + "/" + element.Spec.ID
						}
						part.entity, err = a.spawnViewport3DContent(item.Mod, item.ContentRevision, i, element.Spec.Components)
						if err != nil {
							return fmt.Errorf("3D preview %s/%s: %w", item.Mod, element.Spec.ID, err)
						}
						v.measurePart(part)
					}
				}
			}
			row.id, row.mod, row.revision = item.ID, item.Mod, item.ContentRevision
		}
		for i := range row.parts {
			part := &row.parts[i]
			part.selected = a.canvasDrag.ID != "" && a.canvasDrag.ThreeD && index == a.canvasDrag.Index && (a.canvasDrag.Element == "" || i > 0 && mod.Plan.Elements[i-1].Spec.ID == a.canvasDrag.Element)
			if i > 0 && mod.Plan.Elements[i-1].Parent >= 0 {
				part.selected = part.selected || row.parts[mod.Plan.Elements[i-1].Parent+1].selected
			}
			if part.entity == 0 {
				continue
			}
			matrix, visible := item.Content.Mul4(part.matrix), item.Visible && part.visible
			drag := &a.canvasDrag
			if part.selected || drag.ID != "" && drag.ThreeD && !drag.Definition && index > drag.Index && index < drag.End {
				matrix = mgl32.Translate3D(drag.WorldDelta[0], drag.WorldDelta[1], drag.WorldDelta[2]).Mul4(matrix)
			}
			if value, _ := v.world.Get[scene.GlobalTransform](part.entity); value.Matrix != matrix {
				v.world.GetMut[scene.GlobalTransform](part.entity).Matrix = matrix
			}
			if value, _ := v.world.Get[scene.Visibility](part.entity); value.Visible != visible {
				v.world.GetMut[scene.Visibility](part.entity).Visible = visible
			}
		}
		if store, ok := v.mods[item.Mod]; ok {
			store.used = v.generation
			v.mods[item.Mod] = store
		}
	}
	for id, store := range v.mods {
		if store.used != v.generation {
			_ = store.resources.Dispose()
			delete(v.mods, id)
		}
	}
	for _, resources := range v.retired {
		_ = resources.Dispose()
	}
	clear(v.retired)
	v.retired = v.retired[:0]
	v.revision = a.Preview.Revision
	v.dragVersion = a.canvasDrag.Version
	if initialized || v.local != a.Local {
		a.focusViewport3D()
	}
	v.local = a.Local
	return nil
}

func (a *ModuleEditor) viewport3DResources(id project.ModID, revision uint64) (*project.Resources, error) {
	v := &a.viewport3D
	store, ok := v.mods[id]
	if ok && store.revision == revision {
		return store.resources, nil
	}
	resources, err := openWorkspaceModResources(a.Document, a.previewDocument().Mods[id].Plan, project.ImageFeature[editorImageDecoder]{})
	if err != nil {
		return nil, err
	}
	if ok {
		v.retired = append(v.retired, store.resources)
	}
	v.mods[id] = workspace3DMod{workspaceModResources: resources, revision: revision, used: v.generation}
	return resources.resources, nil
}

func (a *ModuleEditor) spawnViewport3DContent(id project.ModID, revision uint64, element int, content project.ModComponents) (ecs.Entity, error) {
	if content.Model == nil && content.AmbientLight == nil && content.DirectionalLight == nil && content.PointLight == nil && content.SpotLight == nil && content.AreaLight == nil {
		if content.Camera == nil {
			return 0, nil
		}
		bundle := ecs.NewBundle(2)
		bundle.Put(scene.GlobalTransform{})
		bundle.Put(scene.Visibility{Visible: true})
		return a.viewport3D.world.Spawn(bundle), nil
	}
	resources, err := a.viewport3DResources(id, revision)
	if err != nil {
		return 0, err
	}
	if element >= 0 {
		content = a.viewport3D.mods[id].plan.Elements[element].Spec.Components
	} else {
		content = a.viewport3D.mods[id].plan.Spec.Components
	}
	bundle, err := resources.ModContent(project.ModComponents{Model: content.Model, Shader: content.Shader, AmbientLight: content.AmbientLight, DirectionalLight: content.DirectionalLight, PointLight: content.PointLight, SpotLight: content.SpotLight, AreaLight: content.AreaLight})
	if err != nil {
		return 0, err
	}
	bundle.Put(scene.GlobalTransform{})
	bundle.Put(scene.Visibility{Visible: true})
	return a.viewport3D.world.Spawn(bundle), nil
}

func (a *ModuleEditor) focusViewport3D() {
	v := &a.viewport3D
	if v.world == nil {
		return
	}
	low, high := mgl32.Vec3{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}, mgl32.Vec3{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	found := false
	bounds := a.canvasBounds()
	projection := v.world.GetRef[scene.Projection](v.view.camera)
	camera := preview3DCameraTransformAround(v.view.target, v.view.yaw, v.view.pitch, v.view.distance/max(.05, a.zoom)).LocalMatrix()
	var helpers preview3DHelperGeometry
	for _, row := range v.rows {
		if a.Selected != "" && row.id != a.Selected && a.Local == "" {
			continue
		}
		mod := a.previewDocument().Mods[row.mod]
		for i := range row.parts {
			part := &row.parts[i]
			part.selected = a.selectedElement == "" || i > 0 && mod.Plan.Elements[i-1].Spec.ID == a.selectedElement
			if i > 0 && mod.Plan.Elements[i-1].Parent >= 0 {
				part.selected = part.selected || row.parts[mod.Plan.Elements[i-1].Parent+1].selected
			}
			if !part.selected {
				continue
			}
			mesh, ok := v.world.Get[render3d.Mesh](part.entity)
			visible, _ := v.world.Get[scene.Visibility](part.entity)
			if !visible.Visible {
				continue
			}
			global, _ := v.world.Get[scene.GlobalTransform](part.entity)
			if ok && mesh.Mesh != nil {
				center := global.Matrix.Mul4x1(mesh.Mesh.Bounds.Center.Vec4(1)).Vec3()
				radius := mesh.Mesh.Bounds.Radius * max(global.Matrix.Col(0).Vec3().Len(), global.Matrix.Col(1).Vec3().Len(), global.Matrix.Col(2).Vec3().Len())
				for axis := range 3 {
					low[axis], high[axis] = min(low[axis], center[axis]-radius), max(high[axis], center[axis]+radius)
				}
				found = true
			}
			if part.helper {
				a.viewport3DHelperGeometry(part, projection, camera, bounds, &helpers)
				origin := global.Matrix.Col(3).Vec3()
				for axis := range 3 {
					low[axis], high[axis] = min(low[axis], origin[axis]-.5), max(high[axis], origin[axis]+.5)
				}
				for _, line := range helpers.lines[:helpers.bodyCount] {
					for _, point := range line {
						for axis := range 3 {
							low[axis], high[axis] = min(low[axis], point[axis]), max(high[axis], point[axis])
						}
					}
				}
				found = found || helpers.count > 0
			}
		}
	}
	v.view.target, v.view.distance = mgl32.Vec3{4, 4, 0}, 14
	if found {
		v.view.target = low.Add(high).Mul(.5)
		halfFov := projection.Perspective.Fov * .5
		if aspect := bounds.W / bounds.H; aspect < 1 {
			halfFov = float32(math.Atan(math.Tan(float64(halfFov)) * float64(aspect)))
		}
		v.view.distance = max(.1, high.Sub(low).Len()*.5/float32(math.Sin(float64(halfFov)))*1.2)
	}
	a.syncViewport3DProjection()
}

func (a *ModuleEditor) updateViewport3DInput(in *input.State) {
	v := &a.viewport3D
	if v.world == nil || a.Document == nil || a.modalVisible() || a.workspaceMenuOpen() {
		return
	}
	if a.canvasDrag.ID != "" {
		a.updatePlacementDrag(in.Pointer())
		return
	}
	if a.handleViewport3DControls(in) || a.handlePreview3DOrientation(in) {
		return
	}
	pointer, bounds := in.Pointer(), a.canvasBounds()
	inside := pointer.X >= bounds.X && pointer.X < bounds.X+bounds.W && pointer.Y >= bounds.Y && pointer.Y < bounds.Y+bounds.H
	navigation := in.MouseHeld(input.MouseRight) || in.MouseHeld(input.MouseMiddle)
	if !navigation {
		v.navigating = false
	} else if inside && (in.MousePressed(input.MouseRight) || in.MousePressed(input.MouseMiddle)) {
		v.navigating = true
	}
	if inside && pointer.WheelY != 0 {
		a.zoom = clamp(a.zoom*float32(math.Pow(1.15, float64(pointer.WheelY))), .05, 8)
		a.syncViewport3DProjection()
		a.changed()
		in.ConsumePointerWheel()
	}
	if v.navigating && (pointer.DeltaX != 0 || pointer.DeltaY != 0) {
		if in.MouseHeld(input.MouseMiddle) && (in.KeyHeld(input.KeyLeftShift) || in.KeyHeld(input.KeyRightShift)) {
			camera := preview3DCameraTransformAround(v.view.target, v.view.yaw, v.view.pitch, v.view.distance/a.zoom).LocalMatrix()
			projection, _ := v.world.Get[scene.Projection](v.view.camera)
			units := preview3DWorldPerPixel(&projection, v.view.distance/a.zoom, bounds)
			v.view.target = v.view.target.Sub(camera.Col(0).Vec3().Mul(pointer.DeltaX * units)).Add(camera.Col(1).Vec3().Mul(pointer.DeltaY * units))
		} else {
			v.view.yaw -= pointer.DeltaX * .01
			v.view.pitch -= pointer.DeltaY * .01
		}
		a.changedCanvas()
	}
	if inside && in.KeyPressed(input.KeyF) {
		a.zoom = 1
		a.focusViewport3D()
		a.changed()
	}
	if inside && in.KeyPressed(input.KeyO) {
		v.view.orthographic = !v.view.orthographic
		a.syncViewport3DProjection()
		a.changedCanvas()
	}
	if inside && in.MousePressed(input.MouseLeft) && !a.hasDrafts() {
		a.pressViewport3D(pointer.X, pointer.Y)
		a.updatePlacementDrag(pointer)
	}
}

func (a *ModuleEditor) pickViewport3D(x, y float32) string {
	id, _ := a.pickViewport3DTarget(x, y)
	return id
}

func (a *ModuleEditor) pickViewport3DTarget(x, y float32) (string, string) {
	v := &a.viewport3D
	projection, _ := v.world.Get[scene.Projection](v.view.camera)
	camera := preview3DCameraTransformAround(v.view.target, v.view.yaw, v.view.pitch, v.view.distance/a.zoom).LocalMatrix()
	ray, ok := preview3DViewportRay(a.canvasBounds(), x, y, projection, camera)
	if !ok {
		return "", ""
	}
	best, helperDepth := float32(math.MaxFloat32), float32(-1)
	selectedRow, selectedPart, helperRow, helperPart := -1, 0, -1, 0
	bounds := a.canvasBounds()
	clip := projection.Matrix(bounds.W, bounds.H).Mul4(camera.Inv())
	var helpers preview3DHelperGeometry
	for rowIndex := range v.rows {
		row := &v.rows[rowIndex]
		for i := range row.parts {
			part := &row.parts[i]
			mesh, meshOK := v.world.Get[render3d.Mesh](part.entity)
			visibility, _ := v.world.Get[scene.Visibility](part.entity)
			if !visibility.Visible {
				continue
			}
			global, _ := v.world.Get[scene.GlobalTransform](part.entity)
			if meshOK {
				if distance, hit := preview3DMeshRayDistance(ray, global.Matrix, mesh.Mesh); hit && distance < best {
					selectedRow, selectedPart, best = rowIndex, i, distance
				}
			}
			if part.helper {
				a.viewport3DHelperGeometry(part, &projection, camera, bounds, &helpers)
				if depth, hit := helpers.hit(clip, bounds, mgl32.Vec2{x, y}); hit && depth > helperDepth {
					helperRow, helperPart, helperDepth = rowIndex, i, depth
				}
			}
		}
	}
	// Helpers are visible overlays, so their strokes take precedence over meshes.
	if helperRow >= 0 {
		selectedRow, selectedPart = helperRow, helperPart
	}
	if selectedRow < 0 {
		return "", ""
	}
	row := &v.rows[selectedRow]
	if a.Local != "" {
		if selectedPart > 0 {
			return a.editingModule.ID, a.previewDocument().Mods[row.mod].Plan.Elements[selectedPart-1].Spec.ID
		}
		return a.editingModule.ID, ""
	}
	return row.id, ""
}

func (a *ModuleEditor) render(context *ecs.SystemContext) error {
	c := graphics.CanvasFor(context.World)
	if c == nil {
		return nil
	}
	c.ClearColor(editorTheme.windowBG)
	if a.Document != nil && a.dimension != previewDimension3D {
		return a.renderViewport2D(c)
	}
	v := &a.viewport3D
	if a.Document == nil || a.dimension != previewDimension3D || v.world == nil || v.err != nil {
		return nil
	}
	graphics.BindFrame(v.world, c)
	bounds := a.canvasBounds()
	a.syncViewport3DProjection()
	projection := v.world.GetRef[scene.Projection](v.view.camera)
	ok, err := v.view.ensureTarget(c.Device(), int(bounds.W), int(bounds.H), preview3DRenderScale(a.settings.Viewport3D.Supersample))
	if err != nil || !ok {
		return err
	}
	width, height := v.view.renderTarget.Size()
	camera := v.world.GetMut[scene.Camera](v.view.camera)
	camera.Viewport = scene.CameraViewport{Width: float32(width), Height: float32(height)}
	v.world.GetMut[render3d.Camera](v.view.camera).Target = v.view.renderTarget
	v.world.GetMut[scene.GlobalTransform](v.view.camera).Matrix = preview3DCameraTransformAround(v.view.target, v.view.yaw, v.view.pitch, v.view.distance/max(.05, a.zoom)).LocalMatrix()
	if err := a.syncViewport3DEnvironment(); err != nil {
		return err
	}
	for _, system := range [...]ecs.SystemConfig{scene.ExtractCamerasSystem(""), render3d.ExtractSystem(""), render3d.PrepareSystem(""), render3d.SortSystem(""), render3d.DrawSystem("")} {
		if err := system.Run(v.world); err != nil {
			return err
		}
	}
	cameraMatrix := v.world.GetRef[scene.GlobalTransform](v.view.camera).Matrix
	if err := a.drawPreview3DDepthGrid(c, projection, cameraMatrix); err != nil {
		return err
	}
	if err := a.drawPreview3DAxesDepth(c, projection, cameraMatrix); err != nil {
		return err
	}
	c.ResetViewMatrix()
	c.ResetProjectionMatrix()
	drawPreview3DColorTexture(c, v.view.colorTexture, bounds, editorPanelCornerRadius-1)
	c.PushClipRect(bounds.X, bounds.Y, bounds.W, bounds.H)
	a.drawViewport3DHelpers(c, projection, cameraMatrix)
	a.drawViewport3DSelection(c)
	a.drawPreview3DAxes(c, projection, cameraMatrix)
	a.drawPreview3DOrientation(c, cameraMatrix)
	a.drawViewport3DControls(c)
	c.PopClipRect()
	return nil
}
