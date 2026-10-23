package render3d

import (
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

type AmbientLight struct {
	Color      graphics.ColorRGBA
	Brightness float32
}

func NewAmbientLight() *AmbientLight {
	return &AmbientLight{Color: graphics.White, Brightness: 800}
}

type DirectionalLight struct {
	Shadows     ShadowSettings
	Color       graphics.ColorRGBA
	Illuminance float32
}

// ShadowSettings configures directional shadows. A single map uses Radius;
// 2–4 cascades fit camera frustum slices out to Distance. Depth adds coverage
// for off-camera casters. Resolution is the requested size of each atlas tile.
type ShadowSettings struct {
	Enabled      bool
	Resolution   int
	Radius       float32
	Depth        float32
	Bias         float32
	NormalBias   float32
	Cascades     int
	Distance     float32
	SplitLambda  float32
	CascadeBlend float32
}

func NewShadowSettings() ShadowSettings {
	return ShadowSettings{Enabled: true, Resolution: 2048, Radius: 45, Depth: 160, Bias: 0.0003, NormalBias: 0.035, Cascades: 1, Distance: 120, SplitLambda: .6, CascadeBlend: .1}
}

// PunctualShadowSettings configures point and spot shadow projections in meters.
type PunctualShadowSettings struct {
	Enabled    bool
	Resolution int
	Near       float32
	Bias       float32
	NormalBias float32
}

func NewPunctualShadowSettings() PunctualShadowSettings {
	return PunctualShadowSettings{Enabled: true, Resolution: 512, Near: .05, Bias: .0003, NormalBias: .01}
}

// Environment is attached to a camera. Atlas contains eight lighting tiles;
// Panorama is an optional original-resolution background. Both are loaded
// as linear RGBA8 without mipmaps or premultiplication. Exposure applies to
// lit output; Rotation applies to both panorama and atlas.
type Environment struct {
	Atlas *graphics.Texture
	// Panorama displays the original RGBE image independently of the IBL atlas.
	Panorama *graphics.Texture
	Encoding render.EnvironmentEncoding
	// Sky replaces only the visible background; Atlas still supplies lighting.
	Sky        *graphics.SkyRadiance
	Intensity  float32
	Rotation   float32
	Exposure   float32
	Background bool
}

// ViewLighting optionally replaces scene lights for a single camera, without
// modifying entities in the scene. The zero value uses the scene's lights.
type ViewLighting struct {
	Override         bool
	DisableShadows   bool
	ShadowResolution int // zero keeps the light's resolution
	Ambient          AmbientLight
	Sun              DirectionalLight
	Direction        mgl32.Vec3
}

// GroundPlane adds a shadow-receiving ground to one view. It is excluded from
// picking and shadow casting. Size <= 0 disables it.
type GroundPlane struct {
	Center mgl32.Vec3
	Size   float32
	Color  graphics.ColorRGBA
}

// NoShadow excludes first-person meshes and effects from both shadow casting
// and receiving. Ordinary meshes cast and receive shadows by default.
type NoShadow struct{}

func NewDirectionalLight() DirectionalLight {
	return DirectionalLight{Color: graphics.White, Illuminance: 10000}
}
