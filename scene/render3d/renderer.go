package render3d

import (
	"cmp"
	"fmt"
	"math"
	"reflect"
	"slices"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/reuse"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

// Mesh attaches asset-owned geometry to a scene entity.
type Mesh struct {
	Mesh *graphics.Mesh
}

// Deformation stores reusable deformation state. CPU renderers reuse
// vertices here, while GPU-capable renderers upload the joint and morph data
// without constructing dynamic vertex geometry.
type Deformation struct {
	MorphWeights  []float32
	JointEntities []ecs.Entity
	InverseBind   []mgl32.Mat4
	JointMatrices []mgl32.Mat4
	// NodeMatrices maps baked rigid geometry from its initial model space to
	// its animated model space. Skinned submeshes use JointMatrices instead.
	NodeEntities    []ecs.Entity
	NodeInverseBind []mgl32.Mat4
	NodeMatrices    []mgl32.Mat4
	vertices        []render.Vertex3D
	gpuJoints       [][16]float32
}

// UpdateDeformationsSystem resolves optional joint and rigid node
// entities into model-space matrices before ExtractSystem runs.
func UpdateDeformationsSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "update_mesh_deformations_3d"
	}
	return ecs.SystemConfig{Name: name, System: updateMeshDeformations3D}
}

func updateMeshDeformations3D(context *ecs.SystemContext) error {
	for entity, access := range ecs.Query[Deformation](context.World).All {
		if !scene.IsActive(context.World, entity) {
			continue
		}
		deformation := access.Ref()

		if len(deformation.JointEntities) == 0 && len(deformation.NodeEntities) == 0 {
			continue
		}
		if len(deformation.JointEntities) != 0 && len(deformation.JointEntities) != len(deformation.InverseBind) {
			return fmt.Errorf("ecs: mesh deformation joint entity and inverse bind counts differ")
		}
		if len(deformation.NodeEntities) != 0 && len(deformation.NodeEntities) != len(deformation.NodeInverseBind) {
			return fmt.Errorf("ecs: mesh deformation node entity and inverse bind counts differ")
		}
		meshFromWorld := mgl32.Ident4()
		if global, ok := ecs.Get[scene.GlobalTransform](context.World, entity); ok {
			determinant := global.Matrix.Det()
			if math.IsNaN(float64(determinant)) || math.IsInf(float64(determinant), 0) || math.Abs(float64(determinant)) < 1e-12 {
				return fmt.Errorf("ecs: mesh deformation owner transform is singular")
			}
			meshFromWorld = global.Matrix.Inv()
		}
		deformation = access.Mut()
		if len(deformation.JointEntities) != 0 {
			deformation.JointMatrices = reuse.Resize(deformation.JointMatrices, len(deformation.JointEntities))
			deformation.gpuJoints = reuse.Resize(deformation.gpuJoints, len(deformation.JointEntities))
		}
		for index, jointEntity := range deformation.JointEntities {
			global, ok := ecs.Get[scene.GlobalTransform](context.World, jointEntity)
			if !ok {
				deformation.JointMatrices[index] = mgl32.Ident4()
				deformation.gpuJoints[index] = [16]float32(mgl32.Ident4())
				continue
			}
			deformation.JointMatrices[index] = meshFromWorld.Mul4(global.Matrix).Mul4(deformation.InverseBind[index])
			deformation.gpuJoints[index] = [16]float32(deformation.JointMatrices[index])
		}
		if len(deformation.NodeEntities) != 0 {
			deformation.NodeMatrices = reuse.Resize(deformation.NodeMatrices, len(deformation.NodeEntities))
			for index, nodeEntity := range deformation.NodeEntities {
				deformation.NodeMatrices[index] = mgl32.Ident4()
				if global, ok := ecs.Get[scene.GlobalTransform](context.World, nodeEntity); ok {
					deformation.NodeMatrices[index] = meshFromWorld.Mul4(global.Matrix).Mul4(deformation.NodeInverseBind[index])
				}
			}
		}
	}
	return nil
}

// Material binds typed material data to a mesh and its optional submeshes.
type Material[T graphics.Material3D] struct {
	// Materials uses the indices in Mesh.Submeshes. Material supplies the
	// material for unpartitioned geometry or a slot without an entry.
	Material  *T
	Materials []T
}

type extractedView3D struct {
	viewProjection   mgl32.Mat4
	screenProjection mgl32.Mat4
	skyScale         float32
	skyDirection     mgl32.Vec3
	position         mgl32.Vec3
	viewport         scene.CameraViewport
}

type extractedMesh3D struct {
	noReflection                   bool
	noOcclusion                    bool
	noShadow                       bool
	surfaceTextures                [render.MeshSurfaceTextureSlots]render.TextureHandle
	surfaceTextureFlags            uint32
	entity                         ecs.Entity
	mesh                           *graphics.Mesh
	meshHandle                     render.MeshHandle
	shaderHandle                   render.MeshShaderHandle
	textureHandle                  render.TextureHandle
	normalTextureHandle            render.TextureHandle
	metallicRoughnessTextureHandle render.TextureHandle
	meshKey                        uint32
	material                       graphics.Material3DData
	transform                      mgl32.Mat4
	distance                       float32
	vertices                       []render.Vertex3D
	deformation                    *Deformation
	firstIndex                     uint32
	indexCount                     uint32
}

type extractedMeshCandidate3D struct {
	scene        uint64
	entity       ecs.Entity
	mesh         *graphics.Mesh
	material     graphics.Material3DData
	transform    mgl32.Mat4
	bounds       graphics.BoundingSphere
	layers       scene.RenderLayers
	deformation  *Deformation
	cpuVertices  []render.Vertex3D
	cpuBounds    graphics.BoundingSphere
	cpuReady     bool
	customShader bool
}

type material3DReader func(*ecs.World, ecs.Entity, int) (graphics.Material3DData, bool)

type meshGeometryKey3D struct {
	mesh        *graphics.Mesh
	deformation *Deformation
}

type renderPhase3D struct {
	items []extractedMesh3D
}

type renderView3D struct {
	frustum            frustum3D
	scene              uint64
	fxaa               *antialiasing3D
	fxaaActive         bool
	budget             qualityBudget3D
	selectedLights     [render.MaxMeshLights]render.Light3D
	reflection         *screenReflection3D
	reflectionActive   bool
	occlusion          *renderView3D
	occlusionActive    bool
	shadow             *shadowAtlas3D
	shadowActive       bool
	ground             GroundPlane
	ambient            [4]float32
	lightDir           [4]float32
	lightColor         [4]float32
	lights             []render.Light3D
	overrideLights     [2]render.Light3D
	environment        Environment
	lighting           render.MeshLighting
	frame              render.RenderView[extractedView3D]
	extracted          []extractedMesh3D
	previousExtracted  int
	opaque             renderPhase3D
	transparent        renderPhase3D
	instances          []render.Instance3D
	draws              []render.MeshDraw3D
	vertices           []render.Vertex3D
	indices            []uint32
	deformations       []render.MeshDeformationData
	meshKeys           map[*graphics.Mesh]uint32
	dynamicGeometry    map[meshGeometryKey3D]uint32
	deformationIndices map[meshGeometryKey3D]uint32
}

func (view *renderView3D) resetExtracted() {
	view.previousExtracted = max(view.previousExtracted, len(view.extracted))
	view.extracted = view.extracted[:0]
	view.opaque.items, view.transparent.items = nil, nil
	if view.occlusion != nil {
		view.occlusion.opaque.items = nil
	}
	if s := view.reflection; s != nil {
		for _, pass := range [...]*renderView3D{&s.depth, &s.radiance, &s.occlusion, &s.resolve} {
			pass.opaque.items = nil
		}
	}
}

func (view *renderView3D) releaseUnusedExtracted() {
	if view.previousExtracted > len(view.extracted) {
		clear(view.extracted[len(view.extracted):view.previousExtracted])
	}
	view.previousExtracted = len(view.extracted)
	if view.shadow != nil {
		for _, shadow := range view.shadow.views[:view.shadow.count] {
			if shadow != nil {
				shadow.releaseUnusedExtracted()
			}
		}
	}
}

// World stores per-camera frame snapshots. Simulation components are
// read only during Extract; prepare, queue, sort, and draw use these snapshots.
type World struct {
	fxaaMesh   *graphics.Mesh
	skyMesh    *graphics.Mesh
	groundMesh *graphics.Mesh
	views      map[ecs.Entity]*renderView3D
	viewOrder  []ecs.Entity
	ambient    [4]float32
	lightDir   [4]float32
	lightColor [4]float32
	lights     []extractedLight3D
	white      *graphics.Texture
	materials  []material3DReader
	registered map[reflect.Type]struct{}
}

type activeRenderViews3DScratch struct{}
type extractedMeshes3DScratch struct{}
type preparedMeshHandles3DScratch struct{}
type preparedShaderHandles3DScratch struct{}
type preparedTextureHandles3DScratch struct{}

type renderResource3D[H any] interface {
	comparable
	RenderHandleFor(*render.Device) H
	UploadError() error
}

func prepareRenderResource3D[A renderResource3D[H], H any](cache map[A]H, resource A, device *render.Device) (H, error) {
	if handle, ok := cache[resource]; ok {
		return handle, nil
	}
	handle := resource.RenderHandleFor(device)
	if err := resource.UploadError(); err != nil {
		return handle, err
	}
	cache[resource] = handle
	return handle, nil
}

// NewWorld creates renderer-owned storage with the built-in standard material.
// Register installs this resource automatically when it is absent.
func NewWorld() *World {
	state := &World{
		white:      graphics.CreateTextureByColor(graphics.White),
		views:      make(map[ecs.Entity]*renderView3D),
		lights:     make([]extractedLight3D, 0, render.MaxMeshLights),
		registered: make(map[reflect.Type]struct{}),
	}
	registerMaterial3DReader[graphics.StandardMaterial](state)
	return state
}

// Dispose releases render-owned view caches. Scene meshes and camera targets
// remain owned by their respective assets and callers.
func (state *World) Dispose() error {
	if state == nil {
		return nil
	}
	var first error
	for _, view := range state.views {
		if err := disposeViewTargets3D(view); err != nil && first == nil {
			first = err
		}
	}
	for _, mesh := range [...]*graphics.Mesh{state.skyMesh, state.groundMesh, state.fxaaMesh} {
		if mesh != nil {
			if err := mesh.Dispose(); err != nil && first == nil {
				first = err
			}
		}
	}
	clear(state.views)
	state.viewOrder = state.viewOrder[:0]
	state.skyMesh, state.groundMesh = nil, nil
	state.fxaaMesh = nil
	return first
}

// RegisterMaterial enables extraction of Material[T] components. Like
// Bevy's MaterialPlugin<T>, each custom material type is registered once.
func RegisterMaterial[T graphics.Material3D](world *ecs.World) {
	registerMaterial3DReader[T](renderWorld3D(world))
}

func registerMaterial3DReader[T graphics.Material3D](state *World) {
	typ := reflect.TypeFor[T]()
	if _, ok := state.registered[typ]; ok {
		return
	}
	state.registered[typ] = struct{}{}
	state.materials = append(state.materials, func(world *ecs.World, entity ecs.Entity, slot int) (graphics.Material3DData, bool) {
		component, ok := ecs.Get[Material[T]](world, entity)
		if !ok {
			return graphics.Material3DData{}, false
		}
		if slot >= 0 && slot < len(component.Materials) {
			return component.Materials[slot].Material3D(), true
		}
		if component.Material == nil {
			return graphics.Material3DData{}, false
		}
		return (*component.Material).Material3D(), true
	})
}

func (state *World) materialFor(world *ecs.World, entity ecs.Entity, slot int) graphics.Material3DData {
	result := graphics.NewStandardMaterial(graphics.White).Material3D()
	for index := len(state.materials) - 1; index >= 0; index-- {
		if value, ok := state.materials[index](world, entity, slot); ok {
			result = value
			break
		}
	}
	if shader, ok := ecs.Get[scene.Shader[graphics.MeshShader]](world, entity); ok {
		result.Shader, result.Custom = shader.Program, shader.Custom
	}
	return result
}

func (state *World) viewFor(entity ecs.Entity) *renderView3D {
	view := state.views[entity]
	if view == nil {
		view = &renderView3D{
			meshKeys:           make(map[*graphics.Mesh]uint32),
			dynamicGeometry:    make(map[meshGeometryKey3D]uint32),
			deformationIndices: make(map[meshGeometryKey3D]uint32),
		}
		state.views[entity] = view
	}
	return view
}

func ExtractSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "extract_render_3d"
	}
	return ecs.SystemConfig{Name: name, System: extractRender3D}
}

// PrepareSystem prepares GPU resources and queues main-view
// meshes into opaque and transparent phases in the same traversal.
func PrepareSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "prepare_render_resources_3d"
	}
	return ecs.SystemConfig{Name: name, System: prepareRenderResources3D}
}

func SortSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "sort_render_phases_3d"
	}
	return ecs.SystemConfig{Name: name, System: sortRenderPhases3D}
}

func DrawSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "render_3d"
	}
	return ecs.SystemConfig{Name: name, System: render3D}
}

func renderWorld3D(world *ecs.World) *World {
	state, ok := ecs.GetResource[*World](world)
	if ok && state != nil {
		return state
	}
	state = NewWorld()
	ecs.InsertResource(world, state)
	return state
}

func extractRender3D(context *ecs.SystemContext) (err error) {
	world := context.World
	state := renderWorld3D(world)
	defer func() {
		if err != nil {
			for _, view := range state.views {
				view.releaseUnusedExtracted()
			}
		}
	}()
	state.viewOrder = state.viewOrder[:0]
	activeViews := ecs.FrameScratchMap[activeRenderViews3DScratch, ecs.Entity, bool](world)
	renderState, _ := ecs.GetResource[*scene.RenderWorld](world)
	if renderState == nil {
		return fmt.Errorf("render3d: camera renderer has not been registered")
	}
	if len(renderState.Cameras) == 0 {
		if err := scene.ExtractCameras(context); err != nil {
			return err
		}
	}

	ambient := NewAmbientLight()
	if value, ok := ecs.GetResource[*AmbientLight](world); ok && value != nil {
		ambient = value
	}
	state.ambient = [4]float32{ambient.Color.R * ambient.Brightness, ambient.Color.G * ambient.Brightness, ambient.Color.B * ambient.Brightness, 1}
	state.lightDir = [4]float32{0, -1, 0, 0}
	state.lightColor = [4]float32{}
	state.lights = state.lights[:0]
	state.lights = append(state.lights, extractedLight3D{Light3D: render.Light3D{Kind: render.Light3DAmbient, Color: state.ambient}})
	directionalSet := false
	for entity, access := range ecs.Query2[AmbientLight, scene.GlobalTransform](world).All {
		light := access.A.Get()

		if visibility, ok := ecs.Get[scene.Visibility](world, entity); ok && !visibility.Visible {
			continue
		}
		color := [4]float32{light.Color.R * light.Brightness, light.Color.G * light.Brightness, light.Color.B * light.Brightness, 1}
		layer := scene.LayerForEntity(world, entity)
		if layer.ID == 0 {
			state.ambient, state.lights[0].Color = color, color
		} else {
			state.lights = append(state.lights, extractedLight3D{entity: entity, scene: layer.ID, Light3D: render.Light3D{Kind: render.Light3DAmbient, Color: color}})
		}
	}
	for entity, access := range ecs.Query2[DirectionalLight, scene.GlobalTransform](world).All {
		light := access.A.Get()
		transform := access.B.Get()

		if light.Validate() != nil {
			continue
		}
		if visibility, ok := ecs.Get[scene.Visibility](world, entity); ok && !visibility.Visible {
			continue
		}
		direction := transform.Matrix.Mul4x1(mgl32.Vec4{0, 0, -1, 0}).Vec3().Normalize()
		strength := light.Illuminance
		directionalColor := [4]float32{light.Color.R * strength, light.Color.G * strength, light.Color.B * strength, 1}
		if !directionalSet && scene.LayerForEntity(world, entity).ID == 0 {
			state.lightDir = [4]float32{direction.X(), direction.Y(), direction.Z(), 0}
			state.lightColor = directionalColor
			directionalSet = true
		}
		state.lights = append(state.lights, extractedLight3D{entity: entity, scene: scene.LayerForEntity(world, entity).ID, layers: scene.LayersForEntity(world, entity), shadows: extractedShadowLight3D{directional: light.Shadows}, Light3D: render.Light3D{Kind: render.Light3DDirectional,
			PositionDirection: [4]float32{direction.X(), direction.Y(), direction.Z(), 0},
			Direction:         [4]float32{direction.X(), direction.Y(), direction.Z(), 0},
			Color:             directionalColor, Params: [4]float32{0, strength}}})
	}
	extractLocalLights[PointLight](world, state)
	extractLocalLights[SpotLight](world, state)
	extractLocalLights[AreaLight](world, state)

	meshScratch := ecs.FrameScratch[extractedMeshes3DScratch, extractedMeshCandidate3D](world)
	meshScratch.Values = meshScratch.Values[:0]
	var deformationErr error
	for entity, access := range ecs.Query2[Mesh, scene.GlobalTransform](world).All {
		mesh := access.A.Get()
		transform := access.B.Get()

		if mesh.Mesh == nil || len(mesh.Mesh.Vertices) == 0 || len(mesh.Mesh.Indices) == 0 {
			continue
		}
		if visibility, ok := ecs.Get[scene.Visibility](world, entity); ok && !visibility.Visible {
			continue
		}
		material := state.materialFor(world, entity, -1)
		customShader := material.Shader != nil
		for _, part := range mesh.Mesh.Submeshes {
			customShader = customShader || state.materialFor(world, entity, int(part.Material)).Shader != nil
		}
		deformation := ecs.GetMut[Deformation](world, entity)
		meshScratch.Values = append(meshScratch.Values, extractedMeshCandidate3D{
			entity: entity, scene: scene.LayerForEntity(world, entity).ID, mesh: mesh.Mesh, material: material, transform: transform.Matrix,
			bounds: mesh.Mesh.Bounds, layers: scene.LayersForEntity(world, entity), deformation: deformation,
			customShader: customShader,
		})
	}
	meshScratch.ReleaseUnused()
	for _, camera := range renderState.Cameras {
		if camera.Pipeline != scene.CameraPipeline3D {
			continue
		}
		view := state.viewFor(camera.Entity)
		activeViews[camera.Entity] = true
		view.resetExtracted()
		clear(view.meshKeys)
		viewport := camera.Camera.ResolvedViewport()
		view.frame = render.NewRenderView(extractedView3D{
			viewProjection:   camera.ClipFromWorld(),
			screenProjection: camera.Camera.ClipFromWorld(camera.Transform, camera.Projection),
			position:         camera.Transform.Matrix.Col(3).Vec3(),
			viewport:         viewport,
		})
		// Swapchain draws use the full surface plus a clip rect. Offscreen
		// draws set a GPU viewport, so their projection must remain local.
		if camera.Target != nil {
			view.frame.Camera.viewProjection = view.frame.Camera.screenProjection
		}
		if camera.Projection.Kind == scene.ProjectionOrthographic {
			p := camera.Projection.Orthographic
			p.Update(viewport.Width, viewport.Height)
			view.frame.Camera.skyScale = max(p.Area.MaxX-p.Area.MinX, p.Area.MaxY-p.Area.MinY) * 2
			view.frame.Camera.skyDirection = camera.Transform.Matrix.Col(2).Vec3().Mul(-1).Normalize()
		}
		view.frame.Target = camera.Target
		view.frame.Viewport = render.Rect{X: int32(viewport.X), Y: int32(viewport.Y), Width: int32(viewport.Width), Height: int32(viewport.Height)}
		view.frame.Order = camera.Camera.Order
		view.frame.Mode = camera.ViewMode
		view.frame.Clear = render.RenderTargetClear{
			Color: render.Color(camera.Camera.ClearColor), Depth: camera.DepthClear,
			ClearColor: camera.Camera.Clear, ClearDepth: true,
		}
		if err := setupViewLighting3D(world, state, view, camera.Entity); err != nil {
			return err
		}
		_, gpuDeformation := graphics.DeviceFor(world).Driver().(render.MeshGPUDeformationDriver)
		gpuDeformation = gpuDeformation && view.frame.Mode != render.RenderViewPicking
		for baseIndex := range meshScratch.Values {
			base := &meshScratch.Values[baseIndex]
			if camera.Scene.ID != 0 && base.scene != 0 && camera.Scene.ID != base.scene {
				continue
			}
			if !camera.RenderLayers.Intersects(base.layers) {
				continue
			}
			bounds := base.bounds
			var vertices []render.Vertex3D
			if deformation := base.deformation; deformation != nil && (len(base.mesh.Skin) != 0 || len(base.mesh.MorphTargets) != 0) {
				needsCPU := !gpuDeformation || base.customShader
				if needsCPU {
					if !base.cpuReady {
						deformation.vertices, deformationErr = base.mesh.DeformVertices(deformation.vertices, deformation.MorphWeights, deformation.JointMatrices)
						if deformationErr != nil {
							break
						}
						base.cpuVertices = deformation.vertices
						base.cpuBounds = meshVertexBounds3D(base.cpuVertices)
						base.cpuReady = true
					}
					vertices = base.cpuVertices
					bounds = base.cpuBounds
				}
			}
			gpuDeformed := base.deformation != nil && len(vertices) == 0 && (len(base.mesh.Skin) != 0 || len(base.mesh.MorphTargets) != 0)
			meshKey, ok := view.meshKeys[base.mesh]
			if !ok {
				meshKey = uint32(len(view.meshKeys) + 1)
				view.meshKeys[base.mesh] = meshKey
			}
			_, noShadow := ecs.Get[NoShadow](world, base.entity)
			_, noOcclusion := ecs.Get[NoAmbientOcclusion](world, base.entity)
			_, noReflection := ecs.Get[NoScreenReflection](world, base.entity)
			for partIndex := 0; partIndex < max(1, len(base.mesh.Submeshes)); partIndex++ {
				part := graphics.Submesh{IndexCount: uint32(len(base.mesh.Indices)), Node: -1}
				material, transform := base.material, base.transform
				if len(base.mesh.Submeshes) != 0 {
					part = base.mesh.Submeshes[partIndex]
					material = state.materialFor(world, base.entity, int(part.Material))
				}
				if part.Node >= 0 && base.deformation != nil && len(base.deformation.NodeMatrices) != 0 {
					if int(part.Node) >= len(base.deformation.NodeMatrices) {
						return fmt.Errorf("ecs: submesh node %d exceeds deformation nodes", part.Node)
					}
					transform = transform.Mul4(base.deformation.NodeMatrices[part.Node])
				}
				position := transform.Mul4x1(bounds.Center.Vec4(1)).Vec3()
				scale := max(transform.Col(0).Vec3().Len(), transform.Col(1).Vec3().Len(), transform.Col(2).Vec3().Len())
				visible := gpuDeformed || view.frustum.containsSphere(position, bounds.Radius*scale)
				delta := position.Sub(view.frame.Camera.position)
				item := extractedMesh3D{
					noReflection: noReflection,
					noOcclusion:  noOcclusion,
					noShadow:     noShadow,
					entity:       base.entity, mesh: base.mesh, meshKey: meshKey, material: material, transform: transform,
					distance: delta.Dot(delta), vertices: vertices, deformation: base.deformation,
					firstIndex: part.FirstIndex, indexCount: part.IndexCount,
				}
				if visible {
					view.extracted = append(view.extracted, item)
				}
				if view.shadowActive && !noShadow && material.AlphaMode != graphics.AlphaBlend && !material.Surface.Unlit && material.Shader == nil {
					for _, shadow := range view.shadow.views[:view.shadow.count] {
						if gpuDeformed || shadow.frustum.containsSphere(position, bounds.Radius*scale) {
							shadow.extracted = append(shadow.extracted, item)
						}
					}
				}
			}
		}
		if deformationErr != nil {
			return fmt.Errorf("ecs: deform 3D mesh: %w", deformationErr)
		}
		state.viewOrder = append(state.viewOrder, camera.Entity)
	}
	for entity, view := range state.views {
		view.releaseUnusedExtracted()
		if !activeViews[entity] {
			if err := disposeViewTargets3D(state.views[entity]); err != nil {
				return err
			}
			delete(state.views, entity)
		}
	}
	return nil
}

func prepareRenderResources3D(context *ecs.SystemContext) (err error) {
	state := renderWorld3D(context.World)
	// A failed preparation must not leave partially queued views available.
	defer func() {
		if err != nil {
			for _, entity := range state.viewOrder {
				view := state.views[entity]
				view.opaque.items, view.transparent.items = nil, nil
			}
		}
	}()
	if len(state.viewOrder) == 0 {
		return nil
	}
	meshHandles := ecs.FrameScratchMap[preparedMeshHandles3DScratch, *graphics.Mesh, render.MeshHandle](context.World)
	shaderHandles := ecs.FrameScratchMap[preparedShaderHandles3DScratch, *graphics.MeshShader, render.MeshShaderHandle](context.World)
	textureHandles := ecs.FrameScratchMap[preparedTextureHandles3DScratch, *graphics.Texture, render.TextureHandle](context.World)
	defer func() {
		clear(meshHandles)
		clear(shaderHandles)
		clear(textureHandles)
	}()
	whiteHandle, err := prepareRenderResource3D(textureHandles, state.white, graphics.DeviceFor(context.World))
	if err != nil {
		return err
	}
	for _, entity := range state.viewOrder {
		main := state.views[entity]
		viewCount := 1
		if main.shadowActive {
			viewCount += main.shadow.count
		}
		for viewIndex := 0; viewIndex < viewCount; viewIndex++ {
			view := main
			if viewIndex > 0 {
				view = main.shadow.views[viewIndex-1]
			}
			opaqueCount := 0
			for index := range view.extracted {
				item := &view.extracted[index]
				if len(item.vertices) == 0 {
					item.meshHandle, err = prepareRenderResource3D(meshHandles, item.mesh, graphics.DeviceFor(context.World))
					if err != nil {
						return fmt.Errorf("ecs: prepare 3D mesh: %w", err)
					}
				} else {
					item.meshHandle = render.MeshHandle{}
				}
				material := item.material
				item.surfaceTextureFlags = 0
				for slot, texture := range material.Surface.Textures() {
					item.surfaceTextures[slot] = render.TextureHandle{}
					if texture != nil {
						item.surfaceTextureFlags |= render.MeshSurfaceTextureFlag(slot)
						item.surfaceTextures[slot], err = prepareRenderResource3D(textureHandles, texture, graphics.DeviceFor(context.World))
						if err != nil {
							return fmt.Errorf("ecs: prepare surface texture: %w", err)
						}
					}
				}
				if material.Shader != nil {
					item.shaderHandle, err = prepareRenderResource3D(shaderHandles, material.Shader, graphics.DeviceFor(context.World))
					if err != nil {
						return fmt.Errorf("ecs: prepare 3D material shader: %w", err)
					}
				} else {
					item.shaderHandle = render.MeshShaderHandle{}
				}
				item.textureHandle = whiteHandle
				if material.BaseColorTexture != nil {
					item.textureHandle, err = prepareRenderResource3D(textureHandles, material.BaseColorTexture, graphics.DeviceFor(context.World))
					if err != nil {
						return fmt.Errorf("ecs: prepare 3D material texture: %w", err)
					}
				}
				item.normalTextureHandle = render.TextureHandle{}
				if material.NormalTexture != nil {
					item.normalTextureHandle, err = prepareRenderResource3D(textureHandles, material.NormalTexture, graphics.DeviceFor(context.World))
					if err != nil {
						return fmt.Errorf("ecs: prepare 3D normal texture: %w", err)
					}
				}
				item.metallicRoughnessTextureHandle = render.TextureHandle{}
				if material.MetallicRoughnessTexture != nil {
					item.metallicRoughnessTextureHandle, err = prepareRenderResource3D(textureHandles, material.MetallicRoughnessTexture, graphics.DeviceFor(context.World))
					if err != nil {
						return fmt.Errorf("ecs: prepare 3D metallic roughness texture: %w", err)
					}
				}
				// Partition prepared snapshots in place. The two phases borrow
				// disjoint ranges instead of retaining duplicate material records.
				if view == main && (view.frame.Mode == render.RenderViewPicking || item.material.AlphaMode != graphics.AlphaBlend) {
					if opaqueCount != index {
						view.extracted[opaqueCount], view.extracted[index] = *item, view.extracted[opaqueCount]
					}
					opaqueCount++
				}
			}
			if view == main {
				view.opaque.items = view.extracted[:opaqueCount:opaqueCount]
				view.transparent.items = view.extracted[opaqueCount:len(view.extracted):len(view.extracted)]
			}
		}
	}
	return nil
}

func sortRenderPhases3D(context *ecs.SystemContext) error {
	state := renderWorld3D(context.World)
	for _, entity := range state.viewOrder {
		view := state.views[entity]
		slices.SortFunc(view.opaque.items, compareOpaque3D)
		slices.SortFunc(view.transparent.items, func(a, b extractedMesh3D) int {
			if a.distance > b.distance {
				return -1
			}
			if a.distance < b.distance {
				return 1
			}
			return cmp.Compare(a.entity, b.entity)
		})
	}
	return nil
}

func compareOpaque3D(a, b extractedMesh3D) int {
	if aKey, bKey := a.material.Shader.BatchKey(), b.material.Shader.BatchKey(); aKey != bKey {
		if aKey < bKey {
			return -1
		}
		return 1
	}
	if aKey, bKey := a.material.BaseColorTexture.BatchKey(), b.material.BaseColorTexture.BatchKey(); aKey != bKey {
		if aKey < bKey {
			return -1
		}
		return 1
	}
	if aKey, bKey := a.material.NormalTexture.BatchKey(), b.material.NormalTexture.BatchKey(); aKey != bKey {
		if aKey < bKey {
			return -1
		}
		return 1
	}
	if aKey, bKey := a.material.MetallicRoughnessTexture.BatchKey(), b.material.MetallicRoughnessTexture.BatchKey(); aKey != bKey {
		if aKey < bKey {
			return -1
		}
		return 1
	}
	if a.meshKey != b.meshKey {
		if a.meshKey < b.meshKey {
			return -1
		}
		return 1
	}
	if a.firstIndex != b.firstIndex {
		return cmp.Compare(a.firstIndex, b.firstIndex)
	}
	return cmp.Compare(a.entity, b.entity)
}

func render3D(context *ecs.SystemContext) error {
	state := renderWorld3D(context.World)
	c := graphics.CanvasFor(context.World)
	if c == nil {
		return nil
	}
	for _, entity := range state.viewOrder {
		if err := renderCamera3D(c, state, state.views[entity]); err != nil {
			return err
		}
	}
	return nil
}

func render3DView(c *graphics.Canvas2D, state *World, view *renderView3D) error {
	if c == nil || state == nil || view == nil {
		return nil
	}
	if view.shadowActive {
		for _, shadow := range view.shadow.views[:view.shadow.count] {
			shadow.opaque.items = shadow.extracted
			if err := render3DView(c, state, shadow); err != nil {
				return err
			}
		}
		texture, err := view.shadow.target.ColorTextureFor(c.Device())
		if err != nil {
			return err
		}
		view.lighting.ShadowTexture = texture
	}
	if view.occlusionActive {
		if err := renderViewSSAO3D(c, state, view); err != nil {
			return err
		}
	}
	if view.reflectionActive {
		if err := renderViewSSR3D(c, state, view); err != nil {
			return err
		}
	}
	viewport := view.frame.Camera.viewport
	if view.frame.Mode == render.RenderViewPicking {
		if view.frame.Target == nil {
			return fmt.Errorf("ecs: 3D picking view requires a render target")
		}
		descriptor := view.frame.Target.Descriptor()
		if descriptor.ColorFormat != render.RenderTargetUint32 || !descriptor.Depth {
			return fmt.Errorf("ecs: 3D picking view requires an R32Uint target with depth")
		}
	}
	if view.frame.Target == nil {
		c.PushClipRect(viewport.X, viewport.Y, viewport.Width, viewport.Height)
		defer c.PopClipRect()
	}
	view.instances = view.instances[:0]
	view.draws = view.draws[:0]
	view.vertices = view.vertices[:0]
	view.indices = view.indices[:0]
	previousDeformations := len(view.deformations)
	view.deformations = view.deformations[:0]
	defer func() {
		if len(view.deformations) < previousDeformations {
			clear(view.deformations[len(view.deformations):previousDeformations])
		}
	}()
	clear(view.dynamicGeometry)
	clear(view.deformationIndices)
	picking := view.frame.Mode == render.RenderViewPicking
	if !picking && view.environment.Atlas != nil {
		view.lighting.EnvironmentTexture = view.environment.Atlas.RenderHandleFor(c.Device())
		if err := view.environment.Atlas.UploadError(); err != nil {
			return err
		}
	}
	pass := view.lighting.Params[1]
	if !picking && !view.reflectionActive && (pass == render.MeshPassLit || pass == render.MeshPassReflectionCapture || pass == render.MeshPassReflectionResolve) && view.environment.Background && (view.environment.Panorama != nil || view.environment.Atlas != nil || view.environment.Sky != nil) {
		if err := appendSky3D(state, view, c.Device()); err != nil {
			return err
		}
	}
	if !picking && !view.reflectionActive && view.ground.Size > 0 {
		if err := appendGround3D(state, view, c.Device()); err != nil {
			return err
		}
	}
	view.appendPhase(view.opaque.items, state.white, picking)
	view.appendPhase(view.transparent.items, state.white, picking)
	if len(view.draws) == 0 && view.frame.Target == nil {
		return nil
	}
	batch := render.MeshBatch3D{
		Lighting:         view.lighting,
		ViewProjection:   [16]float32(view.frame.Camera.viewProjection),
		CameraPosition:   [4]float32{view.frame.Camera.position.X(), view.frame.Camera.position.Y(), view.frame.Camera.position.Z(), 1},
		AmbientLight:     view.ambient,
		LightDirection:   view.lightDir,
		DirectionalLight: view.lightColor,
		Lights:           view.lights,
		Vertices:         view.vertices,
		Indices:          view.indices,
		Instances:        view.instances,
		Draws:            view.draws,
		Deformations:     view.deformations,
	}
	if view.frame.Camera.skyScale > 0 {
		batch.CameraPosition[3] = -1
	}
	if view.frame.Target != nil {
		if picking {
			return c.DrawMeshPickingTo(view.frame.Target, view.frame.Viewport, view.frame.Clear, batch)
		}
		clear := view.frame.Clear
		if view.reflectionActive {
			clear = render.RenderTargetClear{}
		}
		return c.DrawMeshesTo(view.frame.Target, view.frame.Viewport, clear, batch)
	}
	return c.DrawMeshes(batch)
}

func (view *renderView3D) appendPhase(items []extractedMesh3D, white *graphics.Texture, picking bool) {
	pass := view.lighting.Params[1]
	for _, item := range items {
		material := item.material
		if pass == render.MeshPassOcclusionDepth && (item.noOcclusion || material.Surface.Unlit || material.Shader != nil || material.AlphaMode == graphics.AlphaBlend) {
			continue
		}
		if view.reflectionActive && material.Shader == nil && material.AlphaMode != graphics.AlphaBlend || pass == render.MeshPassReflectionResolve && material.Shader != nil {
			continue
		}
		if pass >= render.MeshPassReflectionCapture && material.AlphaMode == graphics.AlphaBlend {
			continue
		}
		shader := item.shaderHandle
		if pass == render.MeshPassReflectionCapture || pass == render.MeshPassReflectionDepth || pass == render.MeshPassReflectionOcclusion {
			// Custom stages cannot encode radiance or depth. Their standard
			// geometry blocks rays, with an invalid color for IBL fallback.
			material.Shader = nil
			shader = render.MeshShaderHandle{}
		}
		uv := [4]float32{1, 1, 0, 0}
		if material.BaseColorTexture != nil {
			texture := material.BaseColorTexture
			uv = [4]float32{texture.MaxU, texture.MaxV, texture.U, texture.V}
		}
		materialMode := float32(material.AlphaMode)
		if material.NormalTexture != nil {
			materialMode += 4
		}
		if material.MetallicRoughnessTexture != nil {
			materialMode += 8
		}
		materialMode += float32(item.surfaceTextureFlags)
		if material.Surface.Unlit {
			materialMode += 256
		}
		if item.noOcclusion {
			materialMode += 32768
		}
		if item.noReflection {
			materialMode += render.MeshFlagNoReflection
		}
		if material.Surface.DoubleSided {
			materialMode += 512
		}
		if item.transform.Col(0).Vec3().Cross(item.transform.Col(1).Vec3()).Dot(item.transform.Col(2).Vec3()) < 0 {
			materialMode += 1024
		}
		if item.noShadow {
			materialMode += 4096
		}
		custom := material.Custom
		if material.Shader == nil {
			s := material.Surface
			sinRotation, cosRotation := float64(0), float64(1)
			if s.AnisotropyStrength > 0 {
				sinRotation, cosRotation = math.Sincos(float64(s.AnisotropyRotation))
			}
			custom[0] = [4]float32{s.NormalScale, s.OcclusionStrength, s.SpecularFactor, s.ClearcoatFactor}
			custom[1] = [4]float32{s.SpecularColor.R, s.SpecularColor.G, s.SpecularColor.B, s.ClearcoatRoughness}
			custom[2] = [4]float32{s.SheenColor.R, s.SheenColor.G, s.SheenColor.B, s.SheenRoughness}
			custom[3] = [4]float32{s.AnisotropyStrength, float32(cosRotation), float32(sinRotation), s.ClearcoatNormalScale}
		}
		instance := render.Instance3D{
			Model:            [16]float32(item.transform),
			BaseColor:        [4]float32{material.BaseColor.R, material.BaseColor.G, material.BaseColor.B, material.BaseColor.A},
			EmissiveMetallic: [4]float32{material.Emissive.R * material.EmissiveStrength, material.Emissive.G * material.EmissiveStrength, material.Emissive.B * material.EmissiveStrength, clamp01(material.Metallic)},
			Material:         [4]float32{clamp(material.PerceptualRoughness, 0.045, 1), material.IOR, material.AlphaCutoff, materialMode},
			UVTransform:      uv,
			Custom:           custom,
		}
		if item.material.Shader != nil && pass == render.MeshPassReflectionCapture {
			instance.Material[1] = -1
		}
		if picking {
			instance.Custom = [4][4]float32{}
			instance.SetPickingID(item.entity.PickingID())
		}
		view.instances = append(view.instances, instance)
		draw := render.MeshDraw3D{
			SurfaceTextures:          item.surfaceTextures,
			Mesh:                     item.meshHandle,
			Shader:                   shader,
			Texture:                  item.textureHandle,
			NormalTexture:            item.normalTextureHandle,
			MetallicRoughnessTexture: item.metallicRoughnessTextureHandle,
			FirstIndex:               item.firstIndex,
			IndexCount:               item.indexCount,
			BaseVertex:               0,
			FirstInstance:            uint32(len(view.instances) - 1),
			InstanceCount:            1,
			AlphaMode:                uint32(material.AlphaMode),
		}
		key := meshGeometryKey3D{mesh: item.mesh, deformation: item.deformation}
		if item.deformation != nil && len(item.vertices) == 0 && (len(item.mesh.Skin) != 0 || len(item.mesh.MorphTargets) != 0) {
			if index, ok := view.deformationIndices[key]; ok {
				draw.DeformationIndex = index
			} else {
				if cap(item.deformation.gpuJoints) < len(item.deformation.JointMatrices) {
					item.deformation.gpuJoints = make([][16]float32, len(item.deformation.JointMatrices))
				} else {
					item.deformation.gpuJoints = item.deformation.gpuJoints[:len(item.deformation.JointMatrices)]
				}
				for index, matrix := range item.deformation.JointMatrices {
					item.deformation.gpuJoints[index] = [16]float32(matrix)
				}
				view.deformations = append(view.deformations, render.MeshDeformationData{
					Mesh: item.meshHandle, JointMatrices: item.deformation.gpuJoints,
					MorphWeights: item.deformation.MorphWeights,
				})
				draw.DeformationIndex = uint32(len(view.deformations))
				view.deformationIndices[key] = draw.DeformationIndex
			}
		}
		if len(item.vertices) != 0 {
			draw.Mesh = render.MeshHandle{}
			firstIndex, ok := view.dynamicGeometry[key]
			if !ok {
				vertexBase := uint32(len(view.vertices))
				firstIndex = uint32(len(view.indices))
				view.vertices = append(view.vertices, item.vertices...)
				for _, index := range item.mesh.Indices {
					view.indices = append(view.indices, vertexBase+index)
				}
				view.dynamicGeometry[key] = firstIndex
			}
			draw.FirstIndex += firstIndex
		}
		if len(view.draws) != 0 {
			previous := &view.draws[len(view.draws)-1]
			if previous.Mesh == draw.Mesh && previous.Shader == draw.Shader && previous.Texture == draw.Texture && previous.NormalTexture == draw.NormalTexture &&
				previous.MetallicRoughnessTexture == draw.MetallicRoughnessTexture && previous.SurfaceTextures == draw.SurfaceTextures &&
				previous.FirstIndex == draw.FirstIndex && previous.IndexCount == draw.IndexCount &&
				previous.BaseVertex == draw.BaseVertex && (previous.AlphaMode == uint32(graphics.AlphaBlend)) == (draw.AlphaMode == uint32(graphics.AlphaBlend)) &&
				previous.BlendMode == draw.BlendMode && previous.DeformationIndex == draw.DeformationIndex && previous.FirstInstance+previous.InstanceCount == draw.FirstInstance {
				previous.InstanceCount++
				continue
			}
		}
		view.draws = append(view.draws, draw)
	}
}

func meshVertexBounds3D(vertices []render.Vertex3D) graphics.BoundingSphere {
	if len(vertices) == 0 {
		return graphics.BoundingSphere{}
	}
	center := mgl32.Vec3{}
	for _, vertex := range vertices {
		center = center.Add(mgl32.Vec3(vertex.Position))
	}
	center = center.Mul(1 / float32(len(vertices)))
	radiusSquared := float32(0)
	for _, vertex := range vertices {
		delta := mgl32.Vec3(vertex.Position).Sub(center)
		radiusSquared = max(radiusSquared, delta.Dot(delta))
	}
	return graphics.BoundingSphere{Center: center, Radius: float32(math.Sqrt(float64(radiusSquared)))}
}

type frustum3D struct {
	planes  [6]mgl32.Vec4
	lengths [6]float32
}

func newFrustum3D(clipFromWorld mgl32.Mat4) frustum3D {
	rows := [4]mgl32.Vec4{
		{clipFromWorld[0], clipFromWorld[4], clipFromWorld[8], clipFromWorld[12]},
		{clipFromWorld[1], clipFromWorld[5], clipFromWorld[9], clipFromWorld[13]},
		{clipFromWorld[2], clipFromWorld[6], clipFromWorld[10], clipFromWorld[14]},
		{clipFromWorld[3], clipFromWorld[7], clipFromWorld[11], clipFromWorld[15]},
	}
	frustum := frustum3D{planes: [6]mgl32.Vec4{
		rows[3].Add(rows[0]), rows[3].Sub(rows[0]),
		rows[3].Add(rows[1]), rows[3].Sub(rows[1]),
		rows[2], rows[3].Sub(rows[2]),
	}}
	for i, plane := range frustum.planes {
		frustum.lengths[i] = plane.Vec3().Len()
	}
	return frustum
}

func (frustum *frustum3D) containsSphere(center mgl32.Vec3, radius float32) bool {
	for i := range frustum.planes {
		plane := frustum.planes[i]
		if plane.Vec3().Dot(center)+plane.W() < -radius*frustum.lengths[i] {
			return false
		}
	}
	return true
}

func clamp01(value float32) float32 {
	return clamp(value, 0, 1)
}

func clamp(value, low, high float32) float32 {
	return min(max(value, low), high)
}
