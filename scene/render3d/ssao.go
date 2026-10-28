package render3d

import (
	"fmt"
	"math"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
)

// SSAOSettings controls screen-space ambient occlusion for a single camera.
// Radius and Bias are world distances. ResolutionScale is 0.25–1; Samples
// selects an 8, 16 or 32 tap spiral. The zero value disables the prepass.
type SSAOSettings struct {
	Enabled         bool
	Radius          float32
	Bias            float32
	Strength        float32
	ResolutionScale float32
	Samples         int
}

func NewSSAOSettings() SSAOSettings {
	return SSAOSettings{Enabled: true, Radius: .5, Bias: .025, Strength: 1, ResolutionScale: .5, Samples: 16}
}

func (s SSAOSettings) Validate() error {
	if !s.Enabled {
		return nil
	}
	for _, v := range [...]float32{s.Radius, s.Bias, s.Strength, s.ResolutionScale} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return fmt.Errorf("SSAO settings must be finite")
		}
	}
	if s.Radius <= 0 || s.Bias < 0 || s.Bias >= s.Radius || s.Strength < 0 || s.Strength > 4 || s.ResolutionScale < .25 || s.ResolutionScale > 1 {
		return fmt.Errorf("SSAO requires radius > bias >= 0, strength in [0,4] and resolution scale in [0.25,1]")
	}
	if s.Samples != 8 && s.Samples != 16 && s.Samples != 32 {
		return fmt.Errorf("SSAO samples must be 8, 16 or 32")
	}
	return nil
}

// NoAmbientOcclusion excludes first-person meshes and effects from the
// depth prepass and from receiving screen-space ambient occlusion.
type NoAmbientOcclusion struct{}

func disposeViewTargets3D(view *renderView3D) error {
	var targets [5]*render.RenderTarget
	if view.shadow != nil {
		targets[0] = view.shadow.target
	}
	if view.occlusion != nil {
		targets[1] = view.occlusion.frame.Target
	}
	if view.reflection != nil {
		targets[2] = view.reflection.depth.frame.Target
		targets[3] = view.reflection.radiance.frame.Target
	}
	if view.fxaa != nil {
		targets[4] = view.fxaa.target
	}
	var first error
	for _, target := range targets {
		if target != nil {
			if err := target.Dispose(); err != nil && first == nil {
				first = err
			}
		}
	}
	return first
}

func setupViewSSAO3D(view *renderView3D, settings SSAOSettings) error {
	view.occlusionActive = false
	// Picking and temporary unlit views preserve the lit camera's target.
	if view.frame.Mode != render.RenderViewLit {
		return nil
	}
	if err := settings.Validate(); err != nil {
		return err
	}
	if !settings.Enabled || settings.Strength == 0 {
		if view.occlusion != nil {
			if err := view.occlusion.frame.Target.Dispose(); err != nil {
				return err
			}
			view.occlusion = nil
		}
		return nil
	}
	matrix := view.frame.Camera.viewProjection
	if view.fxaaActive {
		matrix = view.frame.Camera.screenProjection
	}
	if determinant := matrix.Det(); math.IsNaN(float64(determinant)) || math.IsInf(float64(determinant), 0) || determinant == 0 {
		return fmt.Errorf("ecs: SSAO requires an invertible camera projection")
	}
	width := max(1, int(math.Ceil(float64(view.frame.Viewport.Width)*float64(settings.ResolutionScale))))
	height := max(1, int(math.Ceil(float64(view.frame.Viewport.Height)*float64(settings.ResolutionScale))))
	if view.occlusion == nil {
		target, err := render.NewRenderTarget(render.RenderTargetDescriptor{Width: width, Height: height, Depth: true})
		if err != nil {
			return err
		}
		view.occlusion = &renderView3D{dynamicGeometry: make(map[meshGeometryKey3D]uint32), deformationIndices: make(map[meshGeometryKey3D]uint32)}
		view.occlusion.frame.Target = target
	}
	pass := view.occlusion
	pass.frame.Target.Resize(width, height)
	pass.frame.Camera, pass.frame.Mode = view.frame.Camera, render.RenderViewLit
	pass.frame.Camera.viewProjection = matrix
	pass.frame.Viewport = render.Rect{Width: int32(width), Height: int32(height)}
	pass.frame.Clear = render.RenderTargetClear{ClearColor: true, ClearDepth: true}
	pass.ground = view.ground
	pass.lighting = render.MeshLighting{Params: [4]float32{1, 2}}
	view.lighting.OcclusionInverseProjection = [16]float32(matrix.Inv())
	view.lighting.OcclusionParams = [4]float32{settings.Radius, settings.Bias, settings.Strength, float32(settings.Samples)}
	view.occlusionActive = true
	return nil
}

func renderViewSSAO3D(canvas *graphics.Canvas2D, state *World, view *renderView3D) error {
	pass := view.occlusion
	pass.opaque.items = view.opaque.items
	if err := render3DView(canvas, state, pass); err != nil {
		return err
	}
	texture, err := pass.frame.Target.ColorTextureFor(canvas.Device())
	if err != nil {
		return err
	}
	view.lighting.OcclusionTexture = texture
	return nil
}
