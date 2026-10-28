package render3d

import (
	"fmt"
	"math"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

// SSRSettings controls single-bounce screen-space reflections. Distances are
// in meters. Rough reflections fade into IBL; no temporal history is retained.
type SSRSettings struct {
	Enabled      bool
	MaxDistance  float32
	Thickness    float32
	Strength     float32
	MaxRoughness float32
	EdgeFade     float32
	Steps        int
}

func NewSSRSettings() SSRSettings {
	return SSRSettings{Enabled: true, MaxDistance: 50, Thickness: .1, Strength: 1, MaxRoughness: .6, EdgeFade: .1, Steps: 64}
}

func (s SSRSettings) Validate() error {
	if !s.Enabled {
		return nil
	}
	for _, v := range [...]float32{s.MaxDistance, s.Thickness, s.Strength, s.MaxRoughness, s.EdgeFade} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return fmt.Errorf("SSR settings must be finite")
		}
	}
	if s.MaxDistance <= 0 || s.Thickness <= 0 || s.Thickness >= s.MaxDistance || s.Strength < 0 || s.Strength > 1 || s.MaxRoughness <= .045 || s.MaxRoughness > 1 || s.EdgeFade <= 0 || s.EdgeFade > .5 {
		return fmt.Errorf("SSR requires distance > thickness > 0, strength in [0,1], roughness in (0.045,1] and edge fade in (0,0.5]")
	}
	if s.Steps != 32 && s.Steps != 64 && s.Steps != 128 {
		return fmt.Errorf("SSR steps must be 32, 64 or 128")
	}
	return nil
}

// NoScreenReflection disables receiving SSR, while keeping the mesh visible
// to other reflectors. Transparent and custom materials never receive SSR.
type NoScreenReflection struct{}

type screenReflection3D struct {
	depth, radiance, occlusion, resolve renderView3D
	projection, inverse                 mgl32.Mat4
	settings                            SSRSettings
	saveOcclusion                       bool
}

func setupViewSSR3D(view *renderView3D, settings SSRSettings) error {
	view.reflectionActive = false
	if view.frame.Mode != render.RenderViewLit {
		return nil
	}
	if err := settings.Validate(); err != nil {
		return err
	}
	if !settings.Enabled || settings.Strength == 0 {
		if s := view.reflection; s != nil {
			if err := s.depth.frame.Target.Dispose(); err != nil {
				return err
			}
			if err := s.radiance.frame.Target.Dispose(); err != nil {
				return err
			}
			view.reflection = nil
		}
		return nil
	}
	matrix := view.frame.Camera.screenProjection
	if determinant := matrix.Det(); math.IsNaN(float64(determinant)) || math.IsInf(float64(determinant), 0) || determinant == 0 {
		return fmt.Errorf("ecs: SSR requires an invertible camera projection")
	}
	width, height := max(1, int(view.frame.Viewport.Width)), max(1, int(view.frame.Viewport.Height))
	colorWidth := width
	if view.occlusionActive {
		// Keep RGBE's exponent separate from AO. An optional second tile saves
		// exact per-pixel SSAO without an additional fragment sampler.
		colorWidth *= 2
	}
	if view.reflection == nil {
		depth, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: width, Height: height, Depth: true})
		if err != nil {
			return err
		}
		color, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: colorWidth, Height: height, Depth: true})
		if err != nil {
			_ = depth.Dispose()
			return err
		}
		s := &screenReflection3D{}
		for _, pass := range [...]*renderView3D{&s.depth, &s.radiance, &s.occlusion, &s.resolve} {
			pass.dynamicGeometry = make(map[meshGeometryKey3D]uint32)
			pass.deformationIndices = make(map[meshGeometryKey3D]uint32)
		}
		s.depth.frame.Target = depth
		s.radiance.frame.Target = color
		view.reflection = s
	}
	s := view.reflection
	s.projection, s.inverse = matrix, matrix.Inv()
	forward := cameraForwardFromInverse3D(s.inverse)
	view.lighting.CameraForward = [4]float32{forward.X(), forward.Y(), forward.Z(), 0}
	s.settings, s.saveOcclusion = settings, view.occlusionActive
	s.depth.frame.Target.Resize(width, height)
	s.radiance.frame.Target.Resize(colorWidth, height)
	view.reflectionActive = true
	return nil
}

func renderViewSSR3D(canvas *graphics.Canvas2D, state *World, view *renderView3D) error {
	s := view.reflection
	for _, pass := range [...]*renderView3D{&s.depth, &s.radiance, &s.occlusion, &s.resolve} {
		pass.frame.Camera, pass.frame.Mode = view.frame.Camera, render.RenderViewLit
		if pass != &s.resolve {
			pass.frame.Camera.viewProjection = s.projection
		}
		pass.ground, pass.environment = view.ground, view.environment
		pass.ambient, pass.lightDir, pass.lightColor, pass.lights = view.ambient, view.lightDir, view.lightColor, view.lights
		pass.lighting = view.lighting
		pass.opaque.items = view.opaque.items
	}
	width, height := s.depth.frame.Target.Size()
	viewport := render.Rect{Width: int32(width), Height: int32(height)}
	s.depth.frame.Viewport = viewport
	s.depth.frame.Clear = render.RenderTargetClear{ClearColor: true, ClearDepth: true}
	s.depth.lighting.Params[1] = render.MeshPassReflectionDepth
	if err := render3DView(canvas, state, &s.depth); err != nil {
		return err
	}
	s.radiance.frame.Viewport = viewport
	s.radiance.frame.Clear = render.RenderTargetClear{ClearColor: true, ClearDepth: true}
	s.radiance.lighting.Params[1] = render.MeshPassReflectionCapture
	if err := render3DView(canvas, state, &s.radiance); err != nil {
		return err
	}
	if s.saveOcclusion {
		s.occlusion.frame.Target = s.radiance.frame.Target
		s.occlusion.frame.Viewport = render.Rect{X: int32(width), Width: int32(width), Height: int32(height)}
		s.occlusion.frame.Clear = render.RenderTargetClear{ClearDepth: true}
		s.occlusion.lighting.Params[1] = render.MeshPassReflectionOcclusion
		if err := render3DView(canvas, state, &s.occlusion); err != nil {
			return err
		}
	}
	color, err := s.radiance.frame.Target.ColorTextureFor(canvas.Device())
	if err != nil {
		return err
	}
	depth, err := s.depth.frame.Target.ColorTextureFor(canvas.Device())
	if err != nil {
		return err
	}
	s.resolve.frame.Target, s.resolve.frame.Viewport, s.resolve.frame.Clear = view.frame.Target, view.frame.Viewport, view.frame.Clear
	lighting := &s.resolve.lighting
	lighting.Params[1] = render.MeshPassReflectionResolve
	lighting.ShadowTexture, lighting.OcclusionTexture = color, depth
	lighting.OcclusionInverseProjection = [16]float32(s.inverse)
	// The resolve pass consumes captured lighting, so the first shadow record
	// and its two texture bindings can carry SSR data within GLES3's limits.
	lighting.Shadows[0] = render.ShadowMap3D{
		Matrix: [16]float32(s.projection),
		Rect:   [4]float32{float32(s.settings.Steps), s.settings.EdgeFade, 0, 0},
		Params: [4]float32{s.settings.MaxDistance, s.settings.Thickness, s.settings.Strength, s.settings.MaxRoughness},
	}
	if s.saveOcclusion {
		lighting.Shadows[0].Rect[2] = 1
	}
	return render3DView(canvas, state, &s.resolve)
}

func cameraForwardFromInverse3D(inverse mgl32.Mat4) mgl32.Vec3 {
	near, middle := inverse.Mul4x1(mgl32.Vec4{0, 0, 1, 1}), inverse.Mul4x1(mgl32.Vec4{0, 0, .5, 1})
	return middle.Vec3().Mul(1 / middle.W()).Sub(near.Vec3().Mul(1 / near.W())).Normalize()
}
