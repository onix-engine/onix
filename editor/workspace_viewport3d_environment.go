package editor

import (
	"math"

	editorresources "github.com/diiyw/onix/editor/resources"
	"github.com/diiyw/onix/graphics"
	graphicspng "github.com/diiyw/onix/graphics/codec/png"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/scene"
	"github.com/diiyw/onix/scene/render3d"
	"github.com/go-gl/mathgl/mgl32"
)

type viewport3DSettings struct {
	SceneLighting     bool    `json:"sceneLighting,omitempty"`
	EnvironmentCamera string  `json:"environmentCamera,omitempty"`
	Background        uint8   `json:"background,omitempty"` // gradient, solid, environment, scene
	IntensityEV       int     `json:"intensityEV,omitempty"`
	ExposureEV        int     `json:"exposureEV,omitempty"`
	Rotation          uint8   `json:"rotation,omitempty"` // eighth turns
	Shadows           uint8   `json:"shadows,omitempty"`  // off, 1024, 2048
	Ground            bool    `json:"ground,omitempty"`
	HideGrid          bool    `json:"hideGrid,omitempty"`
	GridPlane         uint8   `json:"gridPlane,omitempty"` // auto, XZ, XY, YZ
	SnapStep          float32 `json:"snapStep,omitempty"`  // zero follows the visible grid
	Supersample       bool    `json:"supersample,omitempty"`
}

type workspace3DCamera struct {
	camera      render3d.Camera
	environment render3d.Environment
	clearColor  graphics.ColorRGBA
}

// Read camera appearance without spawning or running authored cameras.
func (a *ModuleEditor) viewport3DEnvironmentSource() (workspace3DCamera, string, error) {
	var selected *workspace3DPart
	var selectedMod project.ModID
	var selectedRevision uint64
	for index, row := range a.viewport3D.rows {
		for i := range row.parts {
			part := &row.parts[i]
			if part.cameraSpec == nil || !part.visible || !a.Preview.Items[index].Visible {
				continue
			}
			if selected == nil || part.cameraID == a.settings.Viewport3D.EnvironmentCamera ||
				selected.cameraID != a.settings.Viewport3D.EnvironmentCamera && (part.cameraSpec.Order > selected.cameraSpec.Order || part.cameraSpec.Order == selected.cameraSpec.Order && part.cameraID < selected.cameraID) {
				selected, selectedMod, selectedRevision = part, row.mod, row.revision
			}
		}
	}
	if selected == nil {
		return workspace3DCamera{}, "", nil
	}
	resources, err := a.viewport3DResources(selectedMod, selectedRevision)
	if err != nil {
		return workspace3DCamera{}, "", err
	}
	store := a.viewport3D.mods[selectedMod]
	value, ok := store.cameras[selected.cameraSpec]
	if !ok {
		bundle, err := resources.ModContent(project.ModComponents{Camera: selected.cameraSpec})
		if err != nil {
			return workspace3DCamera{}, "", err
		}
		value.camera, _ = bundle.Value[render3d.Camera]()
		value.environment, _ = bundle.Value[render3d.Environment]()
		camera, _ := bundle.Value[scene.Camera]()
		value.clearColor = camera.ClearColor
		if store.cameras == nil {
			store.cameras = make(map[*project.Camera3DSpec]workspace3DCamera)
		}
		store.cameras[selected.cameraSpec] = value
		a.viewport3D.mods[selectedMod] = store
	}
	return value, selected.cameraID, nil
}

func preview3DRenderScale(supersample bool) int {
	if supersample {
		return 2
	}
	return 1
}

func (a *ModuleEditor) syncViewport3DProjection() {
	v := &a.viewport3D.view
	if a.viewport3D.world == nil {
		return
	}
	distance := v.distance / max(.05, a.zoom)
	p := a.viewport3D.world.GetMut[scene.Projection](v.camera)
	*p = scene.PerspectiveProjection3D()
	p.Perspective.Near = clamp(distance*.001, .00001, 1)
	if v.orthographic {
		p.Kind = scene.ProjectionOrthographic
		p.Orthographic = scene.NewOrthographicProjection2D()
		p.Orthographic.ReverseZ = true
		p.Orthographic.Near = p.Perspective.Near
		p.Orthographic.Far = max(distance*100, 1000)
		p.Orthographic.ScalingMode = scene.FixedVerticalScaling(2 * distance * float32(math.Tan(math.Pi/8)))
	}
}

func (a *ModuleEditor) syncViewport3DEnvironment() error {
	if a.viewport3D.world == nil {
		return nil
	}
	v, settings, world := &a.viewport3D.view, a.settings.Viewport3D, a.viewport3D.world
	var environment render3d.Environment
	var sourceID string
	var sourceCamera workspace3DCamera
	if settings.SceneLighting {
		source, id, err := a.viewport3DEnvironmentSource()
		if err != nil {
			return err
		}
		environment, sourceID = source.environment, id
		sourceCamera = source
	}
	if camera := world.GetMut[scene.Camera](v.camera); camera != nil {
		camera.ClearColor = editorTheme.viewport3DBG
		if settings.Background == 3 && sourceID != "" {
			camera.ClearColor = sourceCamera.clearColor
		}
	}
	exposure := graphics.NewExposure()
	var ssao render3d.SSAOSettings
	var ssr render3d.SSRSettings
	var quality render3d.Quality
	var fxaa bool
	var fog render3d.FogSettings
	var toneMapping graphics.ToneMapping
	if sourceID != "" {
		source := sourceCamera.camera
		exposure = source.Exposure
		ssao = source.SSAO
		ssr = source.SSR
		quality, fxaa = source.Quality, source.FXAA
		fog, toneMapping = source.Fog, source.ToneMapping
	}
	if target := world.GetMut[render3d.Camera](v.camera); target != nil {
		target.Exposure = exposure
		target.SSAO = ssao
		target.SSR = ssr
		target.Quality, target.FXAA = quality, fxaa
		target.Fog, target.ToneMapping = fog, toneMapping
	}
	lighting := render3d.ViewLighting{DisableShadows: settings.Shadows == 0}
	if settings.Shadows > 0 {
		lighting.ShadowResolution = 1024
		if settings.Shadows == 2 {
			lighting.ShadowResolution = 2048
		}
	}
	if !settings.SceneLighting {
		if v.studioTexture == nil {
			texture, err := graphicspng.Decode(editorresources.StudioEnvironment, graphics.TextureOptions{Usage: graphics.ResourceUsageDefault})
			if err != nil {
				return err
			}
			v.studioTexture = texture
		}
		environment = render3d.Environment{Atlas: v.studioTexture, Intensity: 10000, Exposure: 1, Background: true}
		lighting.Override = true
		lighting.Ambient = render3d.AmbientLight{Color: graphics.White, Brightness: 250}
		lighting.Sun = render3d.DirectionalLight{Color: graphics.ColorRGBA{R: 1, G: 0.96, B: 0.9, A: 1}, Illuminance: 18000}
		lighting.Direction = mgl32.Vec3{0.5, -0.8, -0.3}
		if settings.Shadows != 0 {
			lighting.Sun.Shadows = render3d.NewShadowSettings()
			lighting.Sun.Shadows.Resolution = 1024
			if settings.Shadows == 2 {
				lighting.Sun.Shadows.Resolution = 2048
			}
			lighting.Sun.Shadows.Radius = max(v.distance/max(.05, a.zoom)*2, 1)
			lighting.Sun.Shadows.Depth = max(v.distance/max(.05, a.zoom)*8, 10)
		}
	}
	if environment.Exposure <= 0 {
		environment.Exposure = 1
	}
	environment.Intensity *= float32(math.Exp2(float64(max(-8, min(8, settings.IntensityEV)))))
	environment.Exposure *= float32(math.Exp2(float64(max(-8, min(8, settings.ExposureEV)))))
	environment.Rotation += float32(settings.Rotation%8) * math.Pi / 4
	switch settings.Background {
	case 0:
		v.sky = graphics.SkyRadiance{Zenith: mgl32.Vec3{0.045, 0.057, 0.075}, Horizon: mgl32.Vec3{0.14, 0.15, 0.17}, Ground: mgl32.Vec3{0.025, 0.028, 0.032}}
		scale := 1 / exposure.Multiplier()
		v.sky.Zenith = v.sky.Zenith.Mul(scale)
		v.sky.Horizon = v.sky.Horizon.Mul(scale)
		v.sky.Ground = v.sky.Ground.Mul(scale)
		environment.Sky, environment.Background = &v.sky, true
	case 1:
		environment.Background = false
	case 2:
		environment.Sky, environment.Background = nil, environment.Atlas != nil
	}
	if target := world.GetMut[render3d.Environment](v.camera); target != nil {
		*target = environment
	}
	if target := world.GetMut[render3d.ViewLighting](v.camera); target != nil {
		*target = lighting
	}
	if target := world.GetMut[render3d.GroundPlane](v.camera); target != nil {
		*target = render3d.GroundPlane{}
		if settings.Ground {
			target.Center = mgl32.Vec3{v.target.X(), -0.002, v.target.Z()}
			target.Size, target.Color = max(v.distance/max(.05, a.zoom)*24, 10), rgba(90, 92, 98, 255)
		}
	}
	return nil
}
