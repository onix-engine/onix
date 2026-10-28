package scene

import (
	"cmp"
	"reflect"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/smallsort"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

type CameraPipeline uint8

type sceneCameraScratch struct{}

const (
	CameraPipeline2D CameraPipeline = iota
	CameraPipeline3D
	cameraPipelineCount
)

type ExtractedCamera struct {
	Scene        SceneLayer
	Entity       ecs.Entity
	Camera       Camera
	Projection   Projection
	Transform    GlobalTransform
	Pipeline     CameraPipeline
	RenderLayers RenderLayers
	Target       *render.RenderTarget
	ViewMode     render.RenderViewMode
	DepthClear   float32
	OutputSize   mgl32.Vec2
}

// CameraRenderOptions contains the output settings shared by camera pipelines.
type CameraRenderOptions struct {
	Target     *render.RenderTarget
	ViewMode   render.RenderViewMode
	DepthClear float32
}

// CameraRenderer submits the prepared render data for one extracted camera.
type CameraRenderer func(*graphics.Canvas2D, *ecs.World, ExtractedCamera) error

type cameraSettingsReader struct {
	typ      reflect.Type
	read     func(*ecs.World, ecs.Entity) (CameraRenderOptions, bool)
	revision func(*ecs.World) uint64
}

type RenderWorld struct {
	Cameras               []ExtractedCamera
	Items2D               []renderItem2D
	materials2D           []material2DReader
	registeredMaterials2D map[reflect.Type]struct{}
	collectors2D          [renderCollector2DCount]renderCollector2D
	drawers2D             [renderItemKind2DCount]renderItemDrawer2D
	batchDrawers2D        [renderItemKind2DCount]renderBatchDrawer2D
	cameraRenderers       [cameraPipelineCount]CameraRenderer
	cameraSettings        [cameraPipelineCount]cameraSettingsReader
	camera2DLayer         registeredCameraRenderer
	activeCamera2D        ExtractedCamera
	activeCamera2DSet     bool
	whiteTexture2D        *graphics.Texture
	registry2D            uint64
	compositors2D         map[compositeViewKey2D]*compositor2D
	compositeFrame2D      uint64
	compositeAllowed2D    bool
	Preparation2D         RenderPreparation2D
	Composite2D           CompositeStatistics2D
}

type compositeViewKey2D struct {
	Camera ecs.Entity
	Scene  uint64
}

// Dispose releases retained GPU output while the window's device is alive.
func (state *RenderWorld) Dispose() error {
	for key, view := range state.compositors2D {
		if err := view.dispose(); err != nil {
			return err
		}
		delete(state.compositors2D, key)
	}
	return nil
}

type visibleRenderItems2DScratch struct{}

func NewRenderWorld() *RenderWorld {
	return &RenderWorld{registeredMaterials2D: make(map[reflect.Type]struct{})}
}

type material2DReader struct {
	read      func(*ecs.World, ecs.Entity) (graphics.Material2DData, bool)
	structure func(*ecs.World) uint64
	poll      func(*ecs.World, *retainedRender2D)
}

// RegisterMaterial2D enables extraction of MeshMaterial2D[T] components. It
// mirrors Bevy's Material2dPlugin<T> registration boundary.
func RegisterMaterial2D[T graphics.Material2D](world *ecs.World) {
	state := renderWorld(world)
	typ := reflect.TypeFor[T]()
	if _, ok := state.registeredMaterials2D[typ]; ok {
		return
	}
	state.registeredMaterials2D[typ] = struct{}{}
	state.registry2D++
	state.materials2D = append(state.materials2D, material2DReader{read: func(world *ecs.World, entity ecs.Entity) (graphics.Material2DData, bool) {
		component, ok := ecs.Get[MeshMaterial2D[T]](world, entity)
		if !ok || component.Material == nil {
			return graphics.Material2DData{}, false
		}
		return (*component.Material).Material2D(), true
	}, structure: ecs.ComponentStructureVersion[MeshMaterial2D[T]], poll: func(world *ecs.World, cache *retainedRender2D) {
		// Material pointers may be edited without replacing their ECS component.
		// Inspect only entities using this type, preserving the static scene cache.
		for entity := range ecs.Query[MeshMaterial2D[T]](world).All {
			if node := cache.nodes[entity]; node != nil && !node.queued {
				material := material2DForEntity(world, entity)
				for position := range node.positions {
					if cache.items[position].material2D != material {
						cache.mark(entity)
						break
					}
				}
			}
		}
	}})
}

func material2DForEntity(world *ecs.World, entity ecs.Entity) graphics.Material2DData {
	var result graphics.Material2DData
	readers := renderWorld(world).materials2D
	for index := len(readers) - 1; index >= 0; index-- {
		if material, ok := readers[index].read(world, entity); ok {
			result = material
			break
		}
	}
	if shader, ok := ecs.Get[Shader[graphics.Material2DShader]](world, entity); ok {
		result.Shader, result.Custom = shader.Program, shader.Custom
	}
	return result
}

func renderWorld(world *ecs.World) *RenderWorld {
	state, ok := ecs.GetResource[*RenderWorld](world)
	if ok && state != nil {
		return state
	}
	state = NewRenderWorld()
	ecs.InsertResource(world, state)
	return state
}

func particleFallbackTexture2D(world *ecs.World) *graphics.Texture {
	state := renderWorld(world)
	if state.whiteTexture2D == nil || !state.whiteTexture2D.ValidFor(graphics.DeviceFor(world)) {
		state.whiteTexture2D = graphics.CreateTextureByColor(graphics.White)
	}
	return state.whiteTexture2D
}

func ExtractCamerasSystem(name string) ecs.SystemConfig {
	if name == "" {
		name = "extract_cameras"
	}
	return ecs.SystemConfig{Name: name, System: ExtractCameras}
}

// ExtractCameras refreshes snapshots of active registered cameras.
// Pipelines can call it directly without constructing a SystemConfig.
func ExtractCameras(context *ecs.SystemContext) error {
	world := context.World
	state := renderWorld(world)
	state.Cameras = state.Cameras[:0]
	c := graphics.CanvasFor(world)
	if c == nil {
		return nil
	}
	canvasWidth, canvasHeight := c.Size()
	var fallback ExtractedCamera
	has2D := false
	sceneCameras := ecs.FrameScratchMap[sceneCameraScratch, uint64, bool](world)
	for entity, access := range ecs.Query[Camera](world).All {
		if !sceneCameraVisible(world, entity) {
			continue
		}
		camera := access.Get()

		if !camera.Active {
			continue
		}
		projection, projectionOK := ecs.Get[Projection](world, entity)
		transform, transformOK := ecs.Get[GlobalTransform](world, entity)
		if !projectionOK || !transformOK {
			continue
		}
		width, height := int(canvasWidth), int(canvasHeight)
		pipeline, options, found := cameraSettingsForEntity(state, world, entity)
		if !found {
			continue
		}
		if options.Target != nil {
			width, height = options.Target.Size()
		}
		camera.UpdateViewport(width, height)
		candidate := ExtractedCamera{
			Scene:        LayerForEntity(world, entity),
			Entity:       entity,
			Camera:       camera,
			Projection:   projection,
			Transform:    transform,
			Pipeline:     pipeline,
			RenderLayers: LayersForEntity(world, entity),
			Target:       options.Target,
			ViewMode:     options.ViewMode,
			DepthClear:   options.DepthClear,
			OutputSize:   mgl32.Vec2{canvasWidth, canvasHeight},
		}
		if camera.Fallback {
			fallback = candidate
			continue
		}
		if candidate.Scene.Overlay && candidate.Target == nil {
			candidate.Camera.Clear = false
		}
		has2D = has2D || pipeline == CameraPipeline2D
		if pipeline == CameraPipeline2D {
			sceneCameras[candidate.Scene.ID] = true
		}
		state.Cameras = append(state.Cameras, candidate)
	}
	if fallback.Entity != 0 {
		for _, access := range ecs.Query[SceneLayer](world).All {
			layer := access.Get()
			if sceneCameras[layer.ID] {
				continue
			}
			candidate := fallback
			candidate.Scene = layer
			if layer.Overlay {
				candidate.Camera.Clear = false
			}
			state.Cameras = append(state.Cameras, candidate)
			sceneCameras[layer.ID] = true
		}
		if !has2D && len(state.Cameras) == 0 {
			state.Cameras = append(state.Cameras, fallback)
		}
	}
	smallsort.InsertionSortFunc(state.Cameras, compareExtractedCamera)
	return nil
}

func cameraSettingsForEntity(state *RenderWorld, world *ecs.World, entity ecs.Entity) (CameraPipeline, CameraRenderOptions, bool) {
	if state != nil {
		for pipeline := len(state.cameraSettings) - 1; pipeline >= 0; pipeline-- {
			if read := state.cameraSettings[pipeline].read; read != nil {
				if options, ok := read(world, entity); ok {
					return CameraPipeline(pipeline), options, true
				}
			}
		}
	}
	return CameraPipeline2D, CameraRenderOptions{ViewMode: render.RenderViewLit}, ecs.Has[Camera2D](world, entity)
}

func compareExtractedCamera(a, b ExtractedCamera) int {
	if order := cmp.Compare(a.Scene.Order, b.Scene.Order); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Camera.Order, b.Camera.Order); order != 0 {
		return order
	}
	return cmp.Compare(a.Entity, b.Entity)
}

func activeCamera2DComponents(world *ecs.World) (Camera, Projection, GlobalTransform, bool) {
	if state, ok := ecs.GetResource[*RenderWorld](world); ok && state != nil && state.activeCamera2DSet {
		return state.activeCamera2D.Camera, state.activeCamera2D.Projection, state.activeCamera2D.Transform, true
	}
	return primaryCamera2DComponents(world)
}

// ClipFromWorld includes viewport placement for draws into the full output surface.
func (camera ExtractedCamera) ClipFromWorld() mgl32.Mat4 {
	clipFromWorld := camera.Camera.ClipFromWorld(camera.Transform, camera.Projection)
	var width, height float32
	if camera.Target != nil {
		targetWidth, targetHeight := camera.Target.Size()
		width, height = float32(targetWidth), float32(targetHeight)
	} else {
		width, height = camera.OutputSize[0], camera.OutputSize[1]
		if width <= 0 || height <= 0 {
			return clipFromWorld
		}
	}
	viewport := camera.Camera.ResolvedViewport()
	if viewport.X == 0 && viewport.Y == 0 && viewport.Width == width && viewport.Height == height {
		return clipFromWorld
	}
	return viewport.PlacementMatrix(width, height).Mul4(clipFromWorld)
}

// PlacementMatrix maps local clip coordinates into this viewport's output region.
func (viewport CameraViewport) PlacementMatrix(width, height float32) mgl32.Mat4 {
	sx, sy := viewport.Width/width, viewport.Height/height
	ox := (viewport.X+viewport.Width*0.5)*2/width - 1
	oy := 1 - (viewport.Y+viewport.Height*0.5)*2/height
	return mgl32.Mat4{
		sx, 0, 0, 0,
		0, sy, 0, 0,
		0, 0, 1, 0,
		ox, oy, 0, 1,
	}
}

func visibleRenderItems2D(world *ecs.World, items []renderItem2D, camera ExtractedCamera, defaultUICamera ecs.Entity) []renderItem2D {
	visible := ecs.FrameScratch[visibleRenderItems2DScratch, renderItem2D](world)
	visible.Values = visible.Values[:0]
	for _, item := range items {
		if renderItemForCamera2D(&item, camera, defaultUICamera) {
			visible.Values = append(visible.Values, item)
		}
	}
	visible.ReleaseUnused()
	return visible.Values
}
