package project

import (
	"fmt"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene/render3d"
)

type PointLightSpec struct {
	Shadows     *PunctualShadowSpec `json:"shadows,omitempty"`
	Color       string              `json:"color"`
	Intensity   float32             `json:"intensity"`
	Unit        graphics.LightUnit  `json:"unit"`
	LuxDistance float32             `json:"luxDistance"`
	Range       float32             `json:"range"`
	Radius      float32             `json:"radius"`
}

func NewPointLightSpec() PointLightSpec {
	l := render3d.NewPointLight()
	return PointLightSpec{Color: "#ffffffff", Intensity: l.Intensity, Unit: l.Unit, LuxDistance: l.LuxDistance, Range: l.Range, Radius: l.Radius}
}
func (s PointLightSpec) Light() (render3d.PointLight, error) {
	color, err := parseColorValue(s.Color, 1)
	if err != nil {
		return render3d.PointLight{}, fmt.Errorf("point light color: %w", err)
	}
	l := render3d.PointLight{Color: color.Linear(), Intensity: s.Intensity, Unit: s.Unit, LuxDistance: s.LuxDistance, Range: s.Range, Radius: s.Radius}
	if s.Shadows != nil {
		l.Shadows = s.Shadows.Settings()
	}
	return l, l.Validate()
}
func compileScenePointLight(spec SceneNodeSpec) (render3d.PointLight, error) {
	return spec.Components.PointLight.Light()
}

type SpotLightSpec struct {
	Shadows          *PunctualShadowSpec     `json:"shadows,omitempty"`
	Color            string                  `json:"color"`
	Intensity        float32                 `json:"intensity"`
	Unit             graphics.LightUnit      `json:"unit"`
	LuxDistance      float32                 `json:"luxDistance"`
	Range            float32                 `json:"range"`
	Radius           float32                 `json:"radius"`
	Shape            render3d.SpotLightShape `json:"shape"`
	InnerConeAngle   float32                 `json:"innerConeAngle"`
	OuterConeAngle   float32                 `json:"outerConeAngle"`
	AspectRatio      float32                 `json:"aspectRatio"`
	Width            float32                 `json:"width"`
	Height           float32                 `json:"height"`
	Reflector        bool                    `json:"reflector"`
	RangeAttenuation bool                    `json:"rangeAttenuation"`
}

func NewSpotLightSpec() SpotLightSpec {
	l := render3d.NewSpotLight()
	return SpotLightSpec{Color: "#ffffffff", Intensity: l.Intensity, Unit: l.Unit, LuxDistance: l.LuxDistance, Range: l.Range, Radius: l.Radius, Shape: l.Shape, InnerConeAngle: l.InnerConeAngle, OuterConeAngle: l.OuterConeAngle, AspectRatio: l.AspectRatio, Width: l.Width, Height: l.Height, Reflector: l.Reflector, RangeAttenuation: l.RangeAttenuation}
}
func (s SpotLightSpec) Light() (render3d.SpotLight, error) {
	color, err := parseColorValue(s.Color, 1)
	if err != nil {
		return render3d.SpotLight{}, fmt.Errorf("spot light color: %w", err)
	}
	l := render3d.SpotLight{Color: color.Linear(), Intensity: s.Intensity, Unit: s.Unit, LuxDistance: s.LuxDistance, Range: s.Range, Radius: s.Radius, Shape: s.Shape, InnerConeAngle: s.InnerConeAngle, OuterConeAngle: s.OuterConeAngle, AspectRatio: s.AspectRatio, Width: s.Width, Height: s.Height, Reflector: s.Reflector, RangeAttenuation: s.RangeAttenuation}
	if s.Shadows != nil {
		l.Shadows = s.Shadows.Settings()
	}
	return l, l.Validate()
}
func compileSceneSpotLight(spec SceneNodeSpec) (render3d.SpotLight, error) {
	return spec.Components.SpotLight.Light()
}

type PunctualShadowSpec struct {
	Resolution int     `json:"resolution"`
	Near       float32 `json:"near"`
	Bias       float32 `json:"bias"`
	NormalBias float32 `json:"normalBias"`
}

func (s PunctualShadowSpec) Settings() render3d.PunctualShadowSettings {
	return render3d.PunctualShadowSettings{Enabled: true, Resolution: s.Resolution, Near: s.Near, Bias: s.Bias, NormalBias: s.NormalBias}
}

func NewPunctualShadowSpec() *PunctualShadowSpec {
	s := render3d.NewPunctualShadowSettings()
	return &PunctualShadowSpec{Resolution: s.Resolution, Near: s.Near, Bias: s.Bias, NormalBias: s.NormalBias}
}

type AreaLightSpec struct {
	Color     string                  `json:"color"`
	Intensity float32                 `json:"intensity"`
	Unit      graphics.LightUnit      `json:"unit"`
	Shape     render3d.AreaLightShape `json:"shape"`
	Range     float32                 `json:"range"`
	Width     float32                 `json:"width"`
	Height    float32                 `json:"height"`
	Radius    float32                 `json:"radius"`
}

func NewAreaLightSpec() AreaLightSpec {
	l := render3d.NewAreaLight()
	return AreaLightSpec{Color: "#ffffffff", Intensity: l.Intensity, Unit: l.Unit, Shape: l.Shape, Range: l.Range, Width: l.Width, Height: l.Height, Radius: l.Radius}
}
func (s AreaLightSpec) Light() (render3d.AreaLight, error) {
	color, err := parseColorValue(s.Color, 1)
	if err != nil {
		return render3d.AreaLight{}, fmt.Errorf("area light color: %w", err)
	}
	l := render3d.AreaLight{Color: color.Linear(), Intensity: s.Intensity, Unit: s.Unit, Shape: s.Shape, Range: s.Range, Width: s.Width, Height: s.Height, Radius: s.Radius}
	return l, l.Validate()
}
func compileSceneAreaLight(spec SceneNodeSpec) (render3d.AreaLight, error) {
	return spec.Components.AreaLight.Light()
}
