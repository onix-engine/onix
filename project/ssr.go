package project

import "github.com/diiyw/onix/scene/render3d"

// A nil camera SSR spec disables screen-space reflection targets.
type SSRSpec struct {
	MaxDistance  float32 `json:"maxDistance"`
	Thickness    float32 `json:"thickness"`
	Strength     float32 `json:"strength"`
	MaxRoughness float32 `json:"maxRoughness"`
	EdgeFade     float32 `json:"edgeFade"`
	Steps        int     `json:"steps"`
}

func NewSSRSpec() *SSRSpec {
	s := render3d.NewSSRSettings()
	return &SSRSpec{MaxDistance: s.MaxDistance, Thickness: s.Thickness, Strength: s.Strength, MaxRoughness: s.MaxRoughness, EdgeFade: s.EdgeFade, Steps: s.Steps}
}

func (s *SSRSpec) Settings() render3d.SSRSettings {
	if s == nil {
		return render3d.SSRSettings{}
	}
	return render3d.SSRSettings{Enabled: true, MaxDistance: s.MaxDistance, Thickness: s.Thickness, Strength: s.Strength, MaxRoughness: s.MaxRoughness, EdgeFade: s.EdgeFade, Steps: s.Steps}
}
