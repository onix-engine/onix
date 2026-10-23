package project

import "github.com/diiyw/onix/scene/render3d"

// A nil camera SSAO spec disables the depth prepass.
type SSAOSpec struct {
	Radius          float32 `json:"radius"`
	Bias            float32 `json:"bias"`
	Strength        float32 `json:"strength"`
	ResolutionScale float32 `json:"resolutionScale"`
	Samples         int     `json:"samples"`
}

func NewSSAOSpec() *SSAOSpec {
	s := render3d.NewSSAOSettings()
	return &SSAOSpec{Radius: s.Radius, Bias: s.Bias, Strength: s.Strength, ResolutionScale: s.ResolutionScale, Samples: s.Samples}
}

func (s *SSAOSpec) Settings() render3d.SSAOSettings {
	if s == nil {
		return render3d.SSAOSettings{}
	}
	return render3d.SSAOSettings{Enabled: true, Radius: s.Radius, Bias: s.Bias, Strength: s.Strength, ResolutionScale: s.ResolutionScale, Samples: s.Samples}
}
