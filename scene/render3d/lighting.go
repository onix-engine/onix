package render3d

import (
	"fmt"
	"math"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func (s ShadowSettings) Validate() error {
	if !s.Enabled {
		return nil
	}
	if s.Resolution < 64 || s.Resolution > 8192 || s.Resolution&(s.Resolution-1) != 0 {
		return fmt.Errorf("shadow resolution must be a power of two between 64 and 8192")
	}
	for _, v := range [...]float32{s.Radius, s.Depth, s.Bias, s.NormalBias, s.Distance, s.SplitLambda, s.CascadeBlend} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || v < 0 {
			return fmt.Errorf("shadow settings must be finite and non-negative")
		}
	}
	if max(1, s.Cascades) == 1 && s.Radius <= 0 || s.Depth <= 0 {
		return fmt.Errorf("shadow depth and single-map radius must be positive")
	}
	if s.Cascades < 0 || s.Cascades > 4 || s.Cascades > 1 && s.Distance <= 0 || s.SplitLambda > 1 || s.CascadeBlend > .5 {
		return fmt.Errorf("shadows require 1–4 cascades, positive cascade distance, split lambda in [0,1] and blend in [0,0.5]")
	}
	return nil
}

func setupViewLighting3D(world *ecs.World, state *World, view *renderView3D, entity ecs.Entity) error {
	view.scene = scene.LayerForEntity(world, entity).ID
	view.lighting = render.MeshLighting{}
	view.environment = Environment{}
	view.ground = GroundPlane{}
	view.shadowActive = false
	view.occlusionActive = false
	view.reflectionActive = false
	view.fxaaActive = false
	var ssao SSAOSettings
	var ssr SSRSettings
	camera, cameraOK := ecs.Get[Camera](world, entity)
	budget, err := effectiveQuality3D(world, camera)
	if err != nil {
		return err
	}
	view.budget = budget
	view.ambient, view.lightDir, view.lightColor = state.ambient, state.lightDir, state.lightColor
	shadowLights := selectViewLights3D(view, state.lights, scene.LayersForEntity(world, entity), budget.lights)
	for _, light := range view.lights {
		if light.Kind == render.Light3DAmbient {
			view.ambient = light.Color
			break
		}
	}
	for _, light := range view.lights {
		if light.Kind == render.Light3DDirectional {
			view.lightDir, view.lightColor = light.Direction, light.Color
			break
		}
	}
	disableShadows, shadowResolution := false, 0
	if config, ok := ecs.Get[ViewLighting](world, entity); ok {
		if config.Override {
			a, sun := config.Ambient, config.Sun
			view.ambient = [4]float32{a.Color.R * a.Brightness, a.Color.G * a.Brightness, a.Color.B * a.Brightness, 1}
			direction := config.Direction
			if direction.LenSqr() < 0.000001 {
				direction = mgl32.Vec3{0, -1, 0}
			}
			direction = direction.Normalize()
			strength := sun.Illuminance
			view.lightDir = [4]float32{direction.X(), direction.Y(), direction.Z(), 0}
			view.lightColor = [4]float32{sun.Color.R * strength, sun.Color.G * strength, sun.Color.B * strength, 1}
			view.overrideLights = [2]render.Light3D{
				{Kind: render.Light3DAmbient, Color: view.ambient},
				{Kind: render.Light3DDirectional, PositionDirection: view.lightDir, Direction: view.lightDir, Color: view.lightColor},
			}
			view.lights = view.overrideLights[:]
			clear(shadowLights[:])
			shadowLights[1].directional = sun.Shadows
		}
		if config.DisableShadows {
			disableShadows = true
		}
		if config.ShadowResolution > 0 {
			shadowResolution = config.ShadowResolution
		}
	}
	if view.frame.Mode == render.RenderViewLit {
		if e, ok := ecs.Get[Environment](world, entity); ok {
			view.environment = e
		}
		view.ground, _ = ecs.Get[GroundPlane](world, entity)
	}
	e := view.environment
	view.lighting.Params[0] = e.Exposure
	if cameraOK {
		if err := camera.Fog.Validate(); err != nil {
			return err
		}
		if err := camera.ToneMapping.Validate(); err != nil {
			return err
		}
		if camera.ToneMapping == graphics.ToneMappingACES {
			view.lighting.ToneMapping = 1
		}
		camera.Fog.writeLighting(&view.lighting)
		ssao = camera.SSAO
		ssr = camera.SSR
		if err := camera.Exposure.Validate(); err != nil {
			return err
		}
		view.lighting.Params[0] = camera.Exposure.Multiplier()
		if e.Exposure > 0 {
			view.lighting.Params[0] *= e.Exposure
		}
		if err := ssao.Validate(); err != nil {
			return err
		}
		if err := ssr.Validate(); err != nil {
			return err
		}
		ssao, ssr = budget.apply(ssao, ssr)
	}
	if e.Atlas != nil {
		view.lighting.Environment = [4]float32{e.Intensity, e.Rotation, 1 / float32(e.Atlas.Width), 8 / float32(e.Atlas.Height)}
		view.lighting.EnvironmentEncoding = e.Encoding
	} else if e.Panorama != nil {
		view.lighting.Environment[1] = e.Rotation
	}
	// A temporary picking/unlit pass must not destroy the lit view's cache.
	if view.frame.Mode != render.RenderViewLit {
		return nil
	}
	if err := setupShadowAtlas3D(view, shadowLights, disableShadows, shadowResolution); err != nil {
		return err
	}
	if view.frame.Camera.skyScale > 0 {
		forward := view.frame.Camera.skyDirection
		view.lighting.CameraForward = [4]float32{forward.X(), forward.Y(), forward.Z(), 0}
	}
	if err := setupViewFXAA3D(view, camera.FXAA); err != nil {
		return err
	}
	if err := setupViewSSAO3D(view, ssao); err != nil {
		return err
	}
	return setupViewSSR3D(view, ssr)
}

// Stabilize the projection in light space so sub-texel camera movement does
// not cause shadow edges to shimmer. Reverse Z maps the near plane to one.
func directionalShadowMatrix(center, direction mgl32.Vec3, s ShadowSettings) mgl32.Mat4 {
	up := mgl32.Vec3{0, 1, 0}
	if float32(math.Abs(float64(direction.Y()))) > 0.99 {
		up = mgl32.Vec3{0, 0, 1}
	}
	rotation := mgl32.LookAtV(mgl32.Vec3{}, direction, up)
	lightCenter := rotation.Mul4x1(center.Vec4(1)).Vec3()
	texel := 2 * s.Radius / float32(s.Resolution)
	lightCenter[0] = float32(math.Floor(float64(lightCenter[0]/texel)+0.5)) * texel
	lightCenter[1] = float32(math.Floor(float64(lightCenter[1]/texel)+0.5)) * texel
	view := mgl32.Translate3D(-lightCenter.X(), -lightCenter.Y(), -lightCenter.Z()-s.Depth*0.5).Mul4(rotation)
	projection := mgl32.Mat4{1 / s.Radius, 0, 0, 0, 0, 1 / s.Radius, 0, 0, 0, 0, 1 / s.Depth, 0, 0, 0, 1, 1}
	return projection.Mul4(view)
}

func appendSky3D(state *World, view *renderView3D, device *render.Device) error {
	if state.skyMesh == nil {
		state.skyMesh = graphics.CubeMesh(2)
	}
	handle := state.skyMesh.RenderHandleFor(device)
	if err := state.skyMesh.UploadError(); err != nil {
		return err
	}
	position := view.frame.Camera.position
	scale := max(view.frame.Camera.skyScale, 1)
	view.instances = append(view.instances, render.Instance3D{
		Model:     [16]float32(mgl32.Translate3D(position.X(), position.Y(), position.Z()).Mul4(mgl32.Scale3D(scale, scale, scale))),
		BaseColor: [4]float32{1, 1, 1, 1}, Material: [4]float32{0.5, 1.5, 0.5, 2048 + 512}, UVTransform: [4]float32{1, 1, 0, 0},
	})
	if sky := view.environment.Sky; sky != nil {
		instance := &view.instances[len(view.instances)-1]
		instance.Material[3] += 16384
		instance.Custom[0] = [4]float32{sky.Zenith.X(), sky.Zenith.Y(), sky.Zenith.Z(), 0}
		instance.Custom[1] = [4]float32{sky.Horizon.X(), sky.Horizon.Y(), sky.Horizon.Z(), 0}
		instance.Custom[2] = [4]float32{sky.Ground.X(), sky.Ground.Y(), sky.Ground.Z(), 0}
	}
	if view.frame.Camera.skyScale > 0 {
		direction := view.frame.Camera.skyDirection
		view.instances[len(view.instances)-1].Custom[3] = [4]float32{direction.X(), direction.Y(), direction.Z(), 1}
	}
	texture := state.white.RenderHandleFor(device)
	if panorama := view.environment.Panorama; panorama != nil && view.environment.Sky == nil {
		texture = panorama.RenderHandleFor(device)
		if err := panorama.UploadError(); err != nil {
			return err
		}
		view.instances[len(view.instances)-1].Material[3] += 8388608
	}
	view.draws = append(view.draws, render.MeshDraw3D{Mesh: handle, Texture: texture, IndexCount: uint32(len(state.skyMesh.Indices)), FirstInstance: uint32(len(view.instances) - 1), InstanceCount: 1})
	return nil
}

func appendGround3D(state *World, view *renderView3D, device *render.Device) error {
	if state.groundMesh == nil {
		state.groundMesh = graphics.PlaneMesh(1)
	}
	handle := state.groundMesh.RenderHandleFor(device)
	if err := state.groundMesh.UploadError(); err != nil {
		return err
	}
	g := view.ground
	color := g.Color.Linear()
	view.instances = append(view.instances, render.Instance3D{
		Model:     [16]float32(mgl32.Translate3D(g.Center.X(), g.Center.Y(), g.Center.Z()).Mul4(mgl32.Scale3D(g.Size, 1, g.Size))),
		BaseColor: [4]float32{color.R, color.G, color.B, 1}, Material: [4]float32{0.85, 1.5, 0.5, 512}, UVTransform: [4]float32{1, 1, 0, 0},
		Custom: [4][4]float32{{1, 1, 1, 0}, {1, 1, 1, 0}},
	})
	view.draws = append(view.draws, render.MeshDraw3D{Mesh: handle, Texture: state.white.RenderHandleFor(device), IndexCount: uint32(len(state.groundMesh.Indices)), FirstInstance: uint32(len(view.instances) - 1), InstanceCount: 1})
	return nil
}
