package render3d

import (
	"fmt"
	"math"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
)

type FogMode string

const (
	FogLinear FogMode = "linear"
	FogExp2   FogMode = "exp2"
)

// FogSettings blends scene radiance toward an sRGB color scaled by Brightness
// in nits before exposure. Distances are meters; Density is inverse meters.
// Sky, UI and custom fragment stages retain their own atmosphere behavior.
type FogSettings struct {
	Enabled    bool
	Mode       FogMode
	Color      graphics.ColorRGBA
	Brightness float32
	Start      float32
	End        float32
	Density    float32
}

func NewFogSettings() FogSettings {
	return FogSettings{Enabled: true, Mode: FogLinear, Color: graphics.ColorFromRGB(170, 190, 210), Brightness: 800, Start: 20, End: 120, Density: .02}
}

func (s FogSettings) Validate() error {
	if !s.Enabled {
		return nil
	}
	for _, value := range [...]float32{s.Brightness, s.Start, s.End, s.Density, s.Color.R, s.Color.G, s.Color.B, s.Color.A} {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) || value < 0 {
			return fmt.Errorf("fog settings must be finite and non-negative")
		}
	}
	if max(s.Color.R, s.Color.G, s.Color.B, s.Color.A) > 1 {
		return fmt.Errorf("fog color must be in [0,1]")
	}
	switch s.Mode {
	case FogLinear:
		if s.End <= s.Start {
			return fmt.Errorf("linear fog requires end > start")
		}
	case FogExp2:
	default:
		return fmt.Errorf("fog mode must be linear or exp2")
	}
	return nil
}

func (s FogSettings) writeLighting(lighting *render.MeshLighting) {
	if !s.Enabled {
		return
	}
	color := s.Color.Linear()
	lighting.FogColor = [3]float32{color.R * s.Brightness, color.G * s.Brightness, color.B * s.Brightness}
	mode := float32(1)
	if s.Mode == FogExp2 {
		mode = 2
	}
	lighting.FogParams = [4]float32{s.Start, s.End, s.Density, mode}
}
