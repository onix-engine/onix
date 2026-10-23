package render3d

import (
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
)

// Camera adds 3D output and lighting settings to the common scene.Camera.
// Register enables extraction and input routing for this component.
type Camera struct {
	Fog         FogSettings
	ToneMapping graphics.ToneMapping
	Quality     Quality
	FXAA        bool
	SSAO        SSAOSettings
	SSR         SSRSettings
	Exposure    graphics.Exposure
	DepthClear  float32
	Target      *render.RenderTarget
	ViewMode    render.RenderViewMode
}
