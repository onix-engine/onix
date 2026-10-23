package editor

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	engineui "github.com/diiyw/onix/ui"
	htmlui "github.com/diiyw/onix/ui/html"
	"github.com/go-gl/mathgl/mgl32"
)

type workspace2DMod struct {
	workspaceModResources
	revision uint64
	used     uint64
}

type workspace2DRow struct {
	id                        string
	mod                       project.ModID
	revision, contentRevision uint64
	placement, root           ecs.Entity
	parts, slots              []ecs.Entity
	targets                   map[string]ecs.Entity
	selectedParts             []bool
	animated                  bool
	transform                 project.ModTransform
}

type workspaceViewport2D struct {
	world               *ecs.World
	mods                map[project.ModID]workspace2DMod
	retired             []*project.Resources
	rows                []workspace2DRow
	sceneRoot           ecs.Entity
	target, sceneTarget editorRenderTarget
	structure           uint64
	generation          uint64
	frame               rect
	offset              mgl32.Vec2
	panning             bool
	local               project.ModID
	focusPending        bool
	size                [2]int
	animated            bool
	cssAnimated         bool
	worldSpace          bool
	labels              [3]scene.Text2D
	err                 error
	imageName           string
	clearColor          graphics.ColorRGBA
	clearColorSource    string
	dragEntity          ecs.Entity
	dragTransform       project.ModTransform
}

func (v *workspaceViewport2D) dispose() {
	if v.world != nil {
		htmlui.Dispose(v.world)
	}
	for _, mod := range v.mods {
		_ = mod.resources.Dispose()
	}
	for _, resources := range v.retired {
		_ = resources.Dispose()
	}
	_ = v.target.disposeTarget()
	_ = v.sceneTarget.disposeTarget()
	*v = workspaceViewport2D{}
}

func (v *workspaceViewport2D) bottomLeft() bool {
	return render.CurrentRendererInfo().RenderTargetBottomLeft
}

func (a *ModuleEditor) viewport2DSize() (int, int) {
	width, height := a.Document.Spec.Viewport.Width, a.Document.Spec.Viewport.Height
	if width <= 0 {
		width = a.Document.Spec.Window.Width
	}
	if height <= 0 {
		height = a.Document.Spec.Window.Height
	}
	if width <= 0 {
		width = 750
	}
	if height <= 0 {
		height = 1624
	}
	if a.Local != "" {
		width, height = 640, 480
	}
	return width, height
}

func viewportWorldTransform(t project.ModTransform) scene.Transform {
	rotation := mgl32.QuatRotate(t.Rotation[2], mgl32.Vec3{0, 0, 1}).Mul(mgl32.QuatRotate(t.Rotation[1], mgl32.Vec3{0, 1, 0})).Mul(mgl32.QuatRotate(t.Rotation[0], mgl32.Vec3{1, 0, 0}))
	return scene.Transform{Translation: mgl32.Vec3(t.Position), Rotation: rotation, Scale: mgl32.Vec3(t.Scale)}
}

func viewportUITransform(t project.ModTransform) scene.UITransform {
	return scene.UITransform{Translation: scene.Px2(t.Position[0], t.Position[1]), Rotation: t.Rotation[2], Scale: mgl32.Vec2{t.Scale[0], t.Scale[1]}}
}

// Placement nodes have both spaces so mixed UI/world private content inherits
// authored ancestors exactly. Only content nodes participate in drawing.
func (v *workspaceViewport2D) spawnPlacement(t project.ModTransform, parent ecs.Entity, order int) ecs.Entity {
	b := ecs.NewBundle(7)
	ecs.Put(&b, viewportWorldTransform(t))
	ecs.Put(&b, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	ecs.Put(&b, viewportUITransform(t))
	ecs.Put(&b, scene.UIGlobalTransform{Matrix: mgl32.Ident4()})
	if parent != 0 {
		ecs.Put(&b, scene.Parent{Entity: parent})
	}
	ecs.Put(&b, scene.SiblingOrder(order))
	ecs.Put(&b, scene.Visibility{Visible: true})
	return v.world.Spawn(b)
}

func (v *workspaceViewport2D) setPlacement(entity ecs.Entity, t project.ModTransform) {
	if value := viewportWorldTransform(t); *ecs.GetRef[scene.Transform](v.world, entity) != value {
		*ecs.GetMut[scene.Transform](v.world, entity) = value
	}
	if value := viewportUITransform(t); *ecs.GetRef[scene.UITransform](v.world, entity) != value {
		*ecs.GetMut[scene.UITransform](v.world, entity) = value
	}
}

func (v *workspaceViewport2D) setContentTransform(entity ecs.Entity, t project.ModTransform) {
	if value := ecs.GetMut[scene.UITransform](v.world, entity); value != nil {
		*value = viewportUITransform(t)
	}
	if value := ecs.GetMut[scene.Transform](v.world, entity); value != nil {
		*value = viewportWorldTransform(t)
	}
}

func (a *ModuleEditor) initViewport2D() {
	v := &a.viewport2D
	v.world, v.mods = ecs.NewWorld(), make(map[project.ModID]workspace2DMod)
	v.imageName = workspaceCanvasImage + "-" + strconv.FormatUint(a.canvasImageVersion, 10)
	v.clearColor = graphics.ColorFromRGB(25, 32, 42)
	registerEditorUIRenderers2D(v.world)
	scene.RegisterCameraRenderer2D(v.world)
	scene.RegisterSpriteTrailRenderer2D(v.world)
	scene.RegisterParticleRenderer2D(v.world)
	scene.RegisterTilemapRenderer2D(v.world)
	scene.RegisterSpineRenderer2D(v.world)
	scene.RegisterMaterial2D[project.ModMaterial2D](v.world)
	engineui.RegisterRuntimeFeature[engineui.FullRuntime](v.world)
	ecs.InsertResource(v.world, editorUITheme(a.font))
	ecs.InsertResource(v.world, scene.NewTime())
	ecs.InsertResource(v.world, scene.NewRenderRequest(a.requestRender))
	cameraEntity := scene.SpawnCamera2D(v.world)
	camera := ecs.GetMut[scene.Camera](v.world, cameraEntity)
	camera.Clear, camera.Fallback = false, true
	v.sceneRoot = v.spawnPlacement(project.ModTransform{Scale: [3]float32{1, 1, 1}}, 0, 0)
	for i, label := range [...]string{"X", "Y", "0,0"} {
		v.labels[i] = scene.NewText2D(label)
		v.labels[i].Font, v.labels[i].FontSize = a.font, 12
	}
	v.labels[0].Color, v.labels[1].Color = editorTheme.axisX, editorTheme.axisY
	v.labels[2].Color = editorTheme.axisOrigin
}

func (v *workspaceViewport2D) removeRow(row *workspace2DRow) {
	for i := len(row.parts) - 1; i >= 0; i-- {
		v.world.Despawn(row.parts[i])
	}
	for _, entity := range row.slots {
		v.world.Despawn(entity)
	}
	if row.root != 0 {
		v.world.Despawn(row.root)
	}
	if row.placement != 0 {
		v.world.Despawn(row.placement)
	}
	*row = workspace2DRow{}
}

func (v *workspaceViewport2D) spawnContent(resources *project.Resources, t project.ModTransform, content project.ModComponents, parent ecs.Entity, order int, ui bool, visible, enabled bool) (ecs.Entity, error) {
	if content.Model != nil {
		content.Shader = nil
	}
	// Editor previews visual state; project physics and 3D content run in Play.
	content.Model, content.Camera = nil, nil
	content.RigidBody2D, content.CircleCollider2D, content.BoxCollider2D, content.SegmentCollider2D = nil, nil, nil, nil
	content.RigidBody3D, content.TriangleMeshCollider3D, content.ConvexHullCollider3D, content.CompoundCollider3D = nil, nil, nil, nil
	content.AmbientLight, content.DirectionalLight, content.PointLight, content.SpotLight, content.AreaLight = nil, nil, nil, nil, nil
	b, err := resources.ModContent(content)
	if err != nil {
		return 0, err
	}
	if ui {
		ecs.Put(&b, viewportUITransform(t))
		ecs.Put(&b, scene.UIGlobalTransform{Matrix: mgl32.Ident4()})
		if content.Anchor == nil {
			ecs.Put(&b, scene.AnchorTopLeft)
		}
	} else {
		ecs.Put(&b, viewportWorldTransform(t))
		ecs.Put(&b, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	}
	if content.Anchor != nil {
		ecs.Put(&b, *content.Anchor)
	}
	ecs.Put(&b, scene.Parent{Entity: parent})
	ecs.Put(&b, scene.SiblingOrder(order))
	ecs.Put(&b, scene.Visibility{Visible: visible})
	ecs.Put(&b, scene.Activity{Active: enabled})
	entity := v.world.Spawn(b)
	if text := ecs.GetMut[scene.Text2D](v.world, entity); text != nil && text.Font == nil {
		theme, _ := ecs.GetResource[engineui.Theme](v.world)
		text.Font = theme.Font
	}
	if surface := ecs.GetMut[htmlui.Surface](v.world, entity); surface != nil && surface.Font == nil {
		theme, _ := ecs.GetResource[engineui.Theme](v.world)
		surface.Font = theme.Font
	}
	return entity, nil
}

func bindViewportTracks[T any](owner ecs.Entity, row *workspace2DRow, tracks []scene.AnimationTrack[T]) ([]ecs.Entity, error) {
	result := make([]ecs.Entity, len(tracks))
	for i, track := range tracks {
		result[i] = owner
		if track.Binding.Path == "" {
			continue
		}
		var found bool
		result[i], found = row.targets[track.Binding.Path]
		if !found {
			return nil, fmt.Errorf("animation target %q is not private content", track.Binding.Path)
		}
	}
	return result, nil
}

func (v *workspaceViewport2D) bindAnimator(entity ecs.Entity, row *workspace2DRow) error {
	animator := ecs.GetMut[scene.Animator](v.world, entity)
	if animator == nil || animator.Clip == nil {
		return nil
	}
	floats, err := bindViewportTracks(entity, row, animator.Clip.FloatTracks)
	if err != nil {
		return err
	}
	textures, err := bindViewportTracks(entity, row, animator.Clip.TextureTracks)
	if err != nil {
		return err
	}
	bools, err := bindViewportTracks(entity, row, animator.Clip.BoolTracks)
	if err != nil {
		return err
	}
	animator.BindTargets(entity, floats, textures, bools)
	return nil
}

func (a *ModuleEditor) syncViewport2D() (resultErr error) {
	v := &a.viewport2D
	defer func() { v.err = resultErr }()
	if v.world == nil {
		a.initViewport2D()
	}
	if v.local != a.Local {
		v.offset, v.panning = mgl32.Vec2{}, false
		if a.Local != "" {
			a.zoom = 1
			v.focusPending = true
		}
	}
	if source := a.Document.Spec.ClearColor; source != v.clearColorSource {
		color := graphics.ColorFromRGB(25, 32, 42)
		if source != "" {
			var err error
			color, err = project.ParseColor(source, 1)
			if err != nil {
				return err
			}
		}
		v.clearColor, v.clearColorSource = color, source
	}
	width, height := a.viewport2DSize()
	a.layoutViewport2D()
	if v.size != [2]int{width, height} {
		v.size = [2]int{width, height}
		ecs.InsertResource(v.world, scene.ViewportSize{Width: float32(width), Height: float32(height), ScaleFactor: 1})
	}
	if v.structure != a.Preview.structure || len(v.rows) != len(a.Preview.Items) {
		for i := range v.rows {
			v.removeRow(&v.rows[i])
		}
		v.rows = slices.Grow(v.rows[:0], len(a.Preview.Items))[:len(a.Preview.Items)]
		clear(v.rows)
		v.structure = a.Preview.structure
	}
	sceneTransform := project.ModTransform{Scale: [3]float32{1, 1, 1}}
	if a.Local == "" {
		sceneTransform = a.previewDocument().Scene().Plan.Transform
	}
	v.setPlacement(v.sceneRoot, sceneTransform)
	v.worldSpace, v.animated = false, false
	v.generation++
	drawOrder := 0
	for i, item := range a.Preview.Items {
		row, mod := &v.rows[i], a.previewDocument().Mods[item.Mod]
		v.worldSpace = v.worldSpace || mod.Spec.Space == project.CoordinateSpaceWorld
		parent := v.sceneRoot
		if item.Parent >= 0 {
			parent = v.rows[item.Parent].placement
		}
		placement := project.ModTransform{Scale: [3]float32{1, 1, 1}}
		if a.Local == "" {
			placement = a.previewDocument().Scene().Plan.Instances[i].Placement
		}
		if a.canvasDrag.ID != "" && !a.canvasDrag.Definition && i == a.canvasDrag.Index {
			placement.Position[0] += a.canvasDrag.Delta[0]
			placement.Position[1] += a.canvasDrag.Delta[1]
		}
		if row.placement == 0 {
			row.placement = v.spawnPlacement(placement, parent, i+1)
		} else {
			v.setPlacement(row.placement, placement)
			if value := ecs.GetRef[scene.Parent](v.world, row.placement); value.Entity != parent {
				ecs.GetMut[scene.Parent](v.world, row.placement).Entity = parent
			}
		}
		if row.id != item.ID || row.mod != item.Mod || row.contentRevision != item.ContentRevision {
			for j := len(row.parts) - 1; j >= 0; j-- {
				v.world.Despawn(row.parts[j])
			}
			for _, entity := range row.slots {
				v.world.Despawn(entity)
			}
			row.slots = nil
			if row.root != 0 {
				v.world.Despawn(row.root)
			}
			row.root, row.parts = 0, nil
			store, ok := v.mods[item.Mod]
			if !ok || store.revision != item.ContentRevision {
				resources, err := openWorkspaceModResources(a.Document, mod.Plan, editorResourceFeature{})
				if err != nil {
					return fmt.Errorf("2D preview %s: %w", item.Mod, err)
				}
				if ok {
					v.retired = append(v.retired, store.resources)
				}
				store = workspace2DMod{workspaceModResources: resources, revision: item.ContentRevision}
				v.mods[item.Mod] = store
			}
			row.animated = viewport2DAnimated(mod.Spec.Components)
			ui := mod.Spec.Space != project.CoordinateSpaceWorld
			var err error
			row.root, err = v.spawnContent(store.resources, mod.Plan.Transform, store.plan.Spec.Components, row.placement, 0, ui, item.Visible, item.Enabled)
			if err != nil {
				return fmt.Errorf("2D preview %s: %w", item.Mod, err)
			}
			row.parts = make([]ecs.Entity, len(mod.Plan.Elements))
			row.selectedParts = make([]bool, len(mod.Plan.Elements))
			row.targets = make(map[string]ecs.Entity, len(mod.Plan.Elements)+len(mod.Plan.Slots))
			row.slots = make([]ecs.Entity, len(mod.Plan.Slots))
			for j, slot := range mod.Plan.Slots {
				row.slots[j], err = v.spawnContent(store.resources, slot.Placement, project.ModComponents{}, row.root, j+1, ui, item.Visible, item.Enabled)
				if err != nil {
					return err
				}
				row.targets[slot.Name] = row.slots[j]
			}
			for j, element := range mod.Plan.Elements {
				row.animated = row.animated || viewport2DAnimated(element.Spec.Components)
				partUI := ui
				if element.Spec.Space != "" {
					partUI = element.Spec.Space == project.CoordinateSpaceUI
				}
				if partUI != ui {
					if partUI && !ecs.Has[scene.UITransform](v.world, row.root) {
						_ = ecs.Insert(v.world, row.root, viewportUITransform(mod.Plan.Transform))
						_ = ecs.Insert(v.world, row.root, scene.UIGlobalTransform{Matrix: mgl32.Ident4()})
					}
					if !partUI && !ecs.Has[scene.Transform](v.world, row.root) {
						_ = ecs.Insert(v.world, row.root, viewportWorldTransform(mod.Plan.Transform))
						_ = ecs.Insert(v.world, row.root, scene.GlobalTransform{Matrix: mgl32.Ident4()})
					}
				}
				partParent := row.root
				if element.Parent >= 0 {
					partParent = row.parts[element.Parent]
				}
				visible := item.Visible && (element.Spec.Visible == nil || *element.Spec.Visible)
				row.parts[j], err = v.spawnContent(store.resources, element.Transform, store.plan.Elements[j].Spec.Components, partParent, j+len(row.slots)+1, partUI, visible, item.Enabled)
				if err != nil {
					return fmt.Errorf("2D preview %s/%s: %w", item.Mod, element.Spec.ID, err)
				}
				row.targets[element.Spec.ID] = row.parts[j]
			}
			if err := v.bindAnimator(row.root, row); err != nil {
				return err
			}
			for _, entity := range row.parts {
				if err := v.bindAnimator(entity, row); err != nil {
					return err
				}
			}
			row.id, row.mod, row.contentRevision = item.ID, item.Mod, item.ContentRevision
			row.transform = mod.Plan.Transform
		}
		if row.transform != mod.Plan.Transform {
			v.setContentTransform(row.root, mod.Plan.Transform)
			row.transform = mod.Plan.Transform
		}
		changed := row.revision != item.Revision
		if changed {
			ecs.GetMut[scene.Visibility](v.world, row.placement).Visible = item.Visible
			ecs.GetMut[scene.Visibility](v.world, row.root).Visible = item.Visible
			ecs.GetMut[scene.Activity](v.world, row.root).Active = item.Enabled
		}
		v.animated = v.animated || row.animated
		drawOrder++
		setViewport2DOrder(v.world, row.root, drawOrder)
		for _, entity := range row.slots {
			if changed {
				ecs.GetMut[scene.Activity](v.world, entity).Active = item.Enabled
				ecs.GetMut[scene.Visibility](v.world, entity).Visible = item.Visible
			}
			drawOrder++
			setViewport2DOrder(v.world, entity, drawOrder)
		}
		for j, entity := range row.parts {
			if changed {
				element := mod.Plan.Elements[j]
				ecs.GetMut[scene.Visibility](v.world, entity).Visible = item.Visible && (element.Spec.Visible == nil || *element.Spec.Visible)
				ecs.GetMut[scene.Activity](v.world, entity).Active = item.Enabled
			}
			drawOrder++
			setViewport2DOrder(v.world, entity, drawOrder)
		}
		row.revision = item.Revision
		store := v.mods[item.Mod]
		store.used = v.generation
		v.mods[item.Mod] = store
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
	clock, _ := ecs.GetResource[*scene.Time](v.world)
	clock.Delta = 0
	if drag := &a.canvasDrag; drag.ID != "" && drag.Entity != 0 {
		transform := drag.Transform
		transform.Position[0] += drag.Delta[0]
		transform.Position[1] += drag.Delta[1]
		v.dragEntity, v.dragTransform = drag.Entity, transform
	}
	a.syncAnimationPreview(0)
	if err := v.syncVisuals(false); err != nil {
		return err
	}
	if v.focusPending {
		a.focusViewport2D()
	}
	v.local, v.focusPending = a.Local, false
	return nil
}

func (a *ModuleEditor) focusViewport2D() {
	v := &a.viewport2D
	if a.Local == "" {
		v.offset = mgl32.Vec2{}
		a.layoutViewport2D()
		return
	}
	view := a.viewport2DView()
	var content rect
	found := false
	for _, row := range v.rows {
		for i := 0; i <= len(row.parts); i++ {
			entity := row.root
			if i > 0 {
				entity = row.parts[i-1]
			}
			if bounds, ok := a.viewport2DContentBounds(entity, view, false); ok {
				viewportUnionBounds(&content, &found, bounds)
			}
		}
	}
	if !found {
		return
	}
	canvas := a.canvasBounds()
	center := mgl32.Vec2{content.X + content.W/2 - v.frame.X, content.Y + content.H/2 - v.frame.Y}.Mul(1 / a.canvasScale)
	// Fit oversized content without enlarging modules that already fit.
	a.zoom *= min(1, max(1, canvas.W-24)/max(1, content.W), max(1, canvas.H-40)/max(1, content.H))
	a.layoutViewport2D()
	v.offset = v.offset.Add(mgl32.Vec2{canvas.W/2 - v.frame.X, canvas.H/2 - v.frame.Y}.Sub(center.Mul(a.canvasScale)))
	a.layoutViewport2D()
}

func viewport2DAnimated(content project.ModComponents) bool {
	return content.Animation != nil || content.Emitter != nil || content.Spine != nil || content.Trail != nil
}

func setViewport2DOrder(world *ecs.World, entity ecs.Entity, order int) {
	if ecs.Has[scene.UITransform](world, entity) {
		if value, ok := ecs.Get[scene.UIStackIndex](world, entity); !ok || int(value) != order {
			_ = ecs.Insert(world, entity, scene.UIStackIndex(order))
		}
	} else if value, ok := ecs.Get[scene.RenderOrder2D](world, entity); !ok || int(value) != order {
		_ = ecs.Insert(world, entity, scene.RenderOrder2D(order))
	}
}

func (v *workspaceViewport2D) syncVisuals(animate bool) error {
	if err := scene.UpdateWorldAnimatorSystem("").Run(v.world); err != nil {
		return err
	}
	if v.dragEntity != 0 {
		v.setContentTransform(v.dragEntity, v.dragTransform)
	}
	if animate {
		if err := scene.UpdateParticleEmitter2DSystem("").Run(v.world); err != nil {
			return err
		}
		clock, _ := ecs.GetResource[*scene.Time](v.world)
		if delta := clock.DeltaSeconds(); delta > 0 {
			for entity, access := range ecs.Query[scene.Spine2D](v.world).All {
				if player := access.Ref().Player; player != nil && scene.IsActive(v.world, entity) && viewportContentVisible(v.world, entity) {
					access.Mut().Player.Update(delta)
				}
			}
		}
	}
	for _, system := range [...]ecs.SystemConfig{scene.SyncGlobalTransformSystem(""), engineui.SyncSystem(""), engineui.SyncLayoutSystem(""), engineui.SyncAfterLayoutSystem(""), scene.UIScrollSystem("")} {
		if err := system.Run(v.world); err != nil {
			return err
		}
	}
	if err := scene.UpdateUIAnimatorSystem("").Run(v.world); err != nil {
		return err
	}
	if v.dragEntity != 0 {
		v.setContentTransform(v.dragEntity, v.dragTransform)
	}
	if err := scene.SyncUIGlobalTransformSystem("").Run(v.world); err != nil {
		return err
	}
	if animate {
		if err := scene.UpdateSpriteTrail2DSystem("").Run(v.world); err != nil {
			return err
		}
	}
	v.cssAnimated = htmlui.Animating(v.world)
	return nil
}

func (a *ModuleEditor) updateViewport2D() error {
	v := &a.viewport2D
	if v.world == nil || v.err != nil || !v.animated && !v.cssAnimated || a.dimension == previewDimension3D || a.Document == nil || a.modalVisible() {
		return nil
	}
	clock, _ := ecs.GetResource[*scene.Time](v.world)
	clock.Advance(time.Now())
	delta := min(clock.Delta, 100*time.Millisecond)
	clock.Elapsed -= clock.Delta - delta
	clock.Delta = delta
	a.syncAnimationPreview(clock.DeltaSeconds())
	if err := v.syncVisuals(true); err != nil {
		return err
	}
	a.updateAnimationPlaybackUI()
	ecs.AdvanceMessages(v.world)
	if a.requestRender != nil {
		a.requestRender()
	}
	return nil
}

func viewportRect(c *graphics.Canvas2D, r rect, color graphics.ColorRGBA) {
	c.DrawShapeMatrices(mgl32.Translate3D(r.X, r.Y, 0).Mul4(mgl32.Scale3D(r.W, r.H, 1)), mgl32.Ident3(), r.W, r.H, render.ShapeRoundedRect, color, graphics.ColorRGBA{}, 0, 0, 1)
}

func (a *ModuleEditor) renderViewport2D(c *graphics.Canvas2D) (resultErr error) {
	v := &a.viewport2D
	if v.world == nil || v.err != nil {
		return nil
	}
	defer func() {
		// Playback is paused while a modal is open, so there is nothing to animate.
		if resultErr == nil && !a.modalVisible() && (v.animated || v.cssAnimated) && a.requestWake != nil {
			a.requestWake(time.Now().Add(time.Second / defaultTargetFPS))
		}
	}()
	if a.Local == "" {
		width, height := v.size[0], v.size[1]
		if ok, err := v.sceneTarget.ensureTarget(width, height, 1); !ok || err != nil {
			return err
		}
		// Scene views retain the runtime viewport and authored camera background.
		if err := c.DrawTo(v.sceneTarget.renderTarget, render.RenderTargetClear{Color: render.Color(v.clearColor), ClearColor: true, ClearDepth: true}, func() error {
			if err := scene.ExtractCamerasSystem("").Run(v.world); err != nil {
				return err
			}
			state, _ := ecs.GetResource[*scene.RenderWorld](v.world)
			if len(state.Cameras) > 0 && state.Cameras[0].Camera.Clear {
				viewportRect(c, rect{W: float32(width), H: float32(height)}, state.Cameras[0].Camera.ClearColor)
			}
			if err := scene.ExtractRender2DSystem("").Run(v.world); err != nil {
				return err
			}
			return scene.RenderCamerasSystem("").Run(v.world)
		}); err != nil {
			return err
		}
	}
	bounds := a.canvasBounds()
	if ok, err := v.target.ensureTarget(int(bounds.W), int(bounds.H), 1); !ok || err != nil {
		return err
	}
	return c.DrawTo(v.target.renderTarget, render.RenderTargetClear{Color: render.Color(editorTheme.viewportBG), ClearColor: true, ClearDepth: true}, func() error {
		a.drawViewport2DGuides(c, bounds)
		c.ResetViewMatrix()
		c.ResetProjectionMatrix()
		if a.Local != "" {
			// Module coordinates extend on every side of the origin. Apply the
			// editor view directly so only the visible canvas clips content.
			uiView := mgl32.Translate3D(v.frame.X, v.frame.Y, 0).Mul4(mgl32.Scale3D(a.canvasScale, a.canvasScale, 1))
			scene.Draw2D(c, v.world, scene.DrawViews2D{World: a.viewport2DView(), UI: uiView})
			c.ResetViewMatrix()
			c.ResetProjectionMatrix()
		} else {
			drawPreview3DColorTexture(c, v.sceneTarget.colorTexture, v.frame, 0)
			c.DrawShapeMatrices(mgl32.Translate3D(v.frame.X, v.frame.Y, 0).Mul4(mgl32.Scale3D(v.frame.W, v.frame.H, 1)), mgl32.Ident3(), v.frame.W, v.frame.H, render.ShapeRoundedRect, graphics.ColorRGBA{}, graphics.ColorFromRGB(112, 119, 132), 0, 1, 1)
		}
		a.drawViewport2DSelection(c)
		a.drawTilemapPalette(c)
		return nil
	})
}

func (a *ModuleEditor) viewport2DView() mgl32.Mat4 {
	v := &a.viewport2D
	view := mgl32.Translate3D(v.frame.X, v.frame.Y, 0).Mul4(mgl32.Scale3D(a.canvasScale, a.canvasScale, 1))
	if v.worldSpace {
		camera, projection, transform, ok := scene.PrimaryCamera2DView(v.world)
		if ok {
			camera.UpdateViewport(v.size[0], v.size[1])
			clipToScreen := mgl32.Translate3D(float32(v.size[0])/2, float32(v.size[1])/2, 0).Mul4(mgl32.Scale3D(float32(v.size[0])/2, -float32(v.size[1])/2, 1))
			view = view.Mul4(clipToScreen).Mul4(camera.ClipFromWorld(transform, projection))
		}
	}
	return view
}

func (a *ModuleEditor) drawViewport2DGuides(c *graphics.Canvas2D, bounds rect) {
	view := a.viewport2DView()
	if abs(view.Det()) < 1e-8 {
		return
	}
	inverse := view.Inv()
	low, high := mgl32.Vec3{math.MaxFloat32, math.MaxFloat32, 0}, mgl32.Vec3{-math.MaxFloat32, -math.MaxFloat32, 0}
	for _, point := range [...]mgl32.Vec3{{0, 0, 0}, {bounds.W, 0, 0}, {0, bounds.H, 0}, {bounds.W, bounds.H, 0}} {
		local := inverse.Mul4x1(point.Vec4(1)).Vec3()
		low[0], low[1] = min(low[0], local[0]), min(low[1], local[1])
		high[0], high[1] = max(high[0], local[0]), max(high[1], local[1])
	}
	scale := max(.01, mgl32.Vec2{view[0], view[1]}.Len())
	step := viewportGridStep(scale)
	c.SetViewMatrix(view)
	// Draw both grid levels in one pass per direction, aligned to the origin.
	for axis := range 2 {
		first, last := int(math.Ceil(float64(low[axis]/step))), int(math.Floor(float64(high[axis]/step)))
		for index := first; index <= last; index++ {
			if index == 0 {
				continue
			}
			color := editorTheme.gridMinor
			if index%4 == 0 {
				color = editorTheme.gridMajor
			}
			line := rect{X: float32(index) * step, Y: low[1], W: 1 / scale, H: high[1] - low[1]}
			if axis == 1 {
				line = rect{X: low[0], Y: float32(index) * step, W: high[0] - low[0], H: 1 / scale}
			}
			viewportRect(c, line, color)
		}
	}
	viewportRect(c, rect{X: low[0], Y: -1 / scale, W: high[0] - low[0], H: 2 / scale}, editorTheme.axisX)
	viewportRect(c, rect{X: -1 / scale, Y: low[1], W: 2 / scale, H: high[1] - low[1]}, editorTheme.axisY)
	c.ResetViewMatrix()
	origin := view.Mul4x1(mgl32.Vec4{0, 0, 0, 1})
	for i, direction := range [...]mgl32.Vec2{{view[0], view[1]}, {view[4], view[5]}} {
		if i == 1 && direction[1] > 0 {
			direction = direction.Mul(-1)
		}
		if position, ok := viewportAxisLabelPosition(origin.Vec2(), direction, bounds); ok {
			if i == 0 {
				position[1] = clamp(position[1]-20, 8, bounds.H-24)
			} else {
				position[0] = clamp(position[0]+8, 8, bounds.W-24)
			}
			scene.DrawScreenText2D(c, &a.viewport2D.labels[i], mgl32.Translate3D(position[0], position[1], 0), scene.AnchorTopLeft)
		}
	}
	if origin[0] >= 0 && origin[1] >= 0 && origin[0] < bounds.W && origin[1] < bounds.H {
		viewportRect(c, rect{X: origin[0] - 3, Y: origin[1] - 3, W: 6, H: 6}, editorTheme.axisOrigin)
		scene.DrawScreenText2D(c, &a.viewport2D.labels[2], mgl32.Translate3D(clamp(origin[0]+8, 8, bounds.W-44), clamp(origin[1]+8, 14, bounds.H-18), 0), scene.AnchorTopLeft)
	}
}

func viewportAxisLabelPosition(origin, direction mgl32.Vec2, bounds rect) (mgl32.Vec2, bool) {
	entry, exit := float32(-math.MaxFloat32), float32(math.MaxFloat32)
	for axis, extent := range [...]float32{bounds.W - 20, bounds.H - 20} {
		if abs(direction[axis]) < 1e-6 {
			if origin[axis] < 4 || origin[axis] > extent {
				return mgl32.Vec2{}, false
			}
			continue
		}
		first, last := (4-origin[axis])/direction[axis], (extent-origin[axis])/direction[axis]
		entry, exit = max(entry, min(first, last)), min(exit, max(first, last))
	}
	return origin.Add(direction.Mul(exit)), entry <= exit
}

func (a *ModuleEditor) pickViewport2D(x, y float32) string {
	id, _ := a.pickViewport2DTarget(x, y)
	return id
}

func (a *ModuleEditor) pickViewport2DTarget(x, y float32) (string, string) {
	v := &a.viewport2D
	bounds := a.canvasBounds()
	if x < bounds.X || y < bounds.Y || x >= bounds.X+bounds.W || y >= bounds.Y+bounds.H {
		return "", ""
	}
	x, y = (x-bounds.X-v.frame.X)/max(.01, a.canvasScale), (y-bounds.Y-v.frame.Y)/max(.01, a.canvasScale)
	if a.Local == "" && (x < 0 || y < 0 || x > float32(v.size[0]) || y > float32(v.size[1])) {
		return "", ""
	}
	worldPick, worldElement := "", ""
	for i := len(v.rows) - 1; i >= 0; i-- {
		row := &v.rows[i]
		for j := len(row.parts); j >= 0; j-- {
			entity := row.root
			if j > 0 {
				entity = row.parts[j-1]
			}
			if v.hitContent(entity, x, y) {
				id, element := row.id, ""
				if a.Local != "" {
					id = a.editingModule.ID
					if j > 0 {
						element = a.previewDocument().Mods[row.mod].Plan.Elements[j-1].Spec.ID
					}
				}
				if ecs.Has[scene.UITransform](v.world, entity) {
					return id, element
				}
				if worldPick == "" {
					worldPick, worldElement = id, element
				}
			}
		}
	}
	return worldPick, worldElement
}

func (v *workspaceViewport2D) hitContent(entity ecs.Entity, x, y float32) bool {
	if !viewportContentVisible(v.world, entity) {
		return false
	}
	matrix, ui := viewportContentMatrix(v.world, entity)
	if ui {
		if clip, ok := ecs.GetRef[scene.UIGlobalTransform](v.world, entity).ClipRect(); ok && !viewportContains(rect{X: clip.X, Y: clip.Y, W: clip.Width, H: clip.Height}, x, y) {
			return false
		}
	}
	determinant := matrix[0]*matrix[5] - matrix[1]*matrix[4]
	if abs(determinant) < 1e-8 {
		return false
	}
	bounds, ok := viewportContentBounds(v.world, entity, mgl32.Ident4(), ui)
	if !ok {
		return false
	}
	if !ui {
		camera, projection, transform, found := scene.PrimaryCamera2DView(v.world)
		if !found {
			return false
		}
		camera.UpdateViewport(v.size[0], v.size[1])
		if point, valid := camera.ViewportToWorld2D(transform, projection, mgl32.Vec2{x, y}); valid {
			x, y = point[0], point[1]
		} else {
			return false
		}
	}
	x, y = x-matrix[12], y-matrix[13]
	return viewportContains(bounds, (matrix[5]*x-matrix[4]*y)/determinant, (matrix[0]*y-matrix[1]*x)/determinant)
}
