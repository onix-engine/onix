package project

import "github.com/diiyw/onix/scene/render3d"

type FogSpec struct {
	Mode       render3d.FogMode `json:"mode"`
	Color      string           `json:"color"`
	Brightness float32          `json:"brightness"`
	Start      float32          `json:"start"`
	End        float32          `json:"end"`
	Density    float32          `json:"density"`
}

func NewFogSpec() *FogSpec {
	s := render3d.NewFogSettings()
	return &FogSpec{Mode: s.Mode, Color: "#aabed2ff", Brightness: s.Brightness, Start: s.Start, End: s.End, Density: s.Density}
}

func (s *FogSpec) Settings() (render3d.FogSettings, error) {
	if s == nil {
		return render3d.FogSettings{}, nil
	}
	color, err := parseColorValue(s.Color, 1)
	if err != nil {
		return render3d.FogSettings{}, err
	}
	settings := render3d.FogSettings{Enabled: true, Mode: s.Mode, Color: color, Brightness: s.Brightness, Start: s.Start, End: s.End, Density: s.Density}
	return settings, settings.Validate()
}

func (s *Camera3DSpec) renderSettings() (render3d.Camera, error) {
	fog, err := s.Fog.Settings()
	if err != nil {
		return render3d.Camera{}, err
	}
	return render3d.Camera{Fog: fog, ToneMapping: s.ToneMapping, Quality: s.Quality, FXAA: s.FXAA, Exposure: s.Exposure, SSAO: s.SSAO.Settings(), SSR: s.SSR.Settings()}, nil
}
