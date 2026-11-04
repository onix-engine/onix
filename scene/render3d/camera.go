package render3d

import (
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
)

// Camera adds 3D output and lighting settings to the common scene.Camera.
// Register enables extraction and input routing for this component.
type Camera struct {
	Target      *render.RenderTarget
	ToneMapping graphics.ToneMapping
	Quality     Quality
	Exposure    graphics.Exposure
	Fog         FogSettings
	SSAO        SSAOSettings
	SSR         SSRSettings
	DepthClear  float32
	FXAA        bool
	ViewMode    render.RenderViewMode
}
