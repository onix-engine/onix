package project

import (
	"fmt"
	"github.com/diiyw/onix/scene"
)

type SpriteTrailSpec struct {
	Lifetime    float32 `json:"lifetime,omitempty"`
	Interval    float32 `json:"interval,omitempty"`
	MinDistance float32 `json:"minDistance,omitempty"`
	AlphaStart  float32 `json:"alphaStart,omitempty"`
	AlphaEnd    float32 `json:"alphaEnd,omitempty"`
	MaxSamples  int     `json:"maxSamples,omitempty"`
}

func (s SpriteTrailSpec) component() scene.SpriteTrail2D {
	trail := scene.NewSpriteTrail2D()
	if s.Lifetime > 0 {
		trail.Lifetime = s.Lifetime
	}
	if s.Interval > 0 {
		trail.Interval = s.Interval
	}
	if s.MinDistance > 0 {
		trail.MinDistance = s.MinDistance
	}
	if s.AlphaStart > 0 {
		trail.AlphaStart = s.AlphaStart
	}
	trail.AlphaEnd = s.AlphaEnd
	if s.MaxSamples > 0 {
		trail.MaxSamples = s.MaxSamples
		trail.Reserve(s.MaxSamples)
	}
	return trail
}

func (s SpriteTrailSpec) validate() error {
	if !finiteModValues([]float32{s.Lifetime, s.Interval, s.MinDistance, s.AlphaStart, s.AlphaEnd}) {
		return fmt.Errorf("sprite trail values must be finite")
	}
	if s.Lifetime < 0 {
		return fmt.Errorf("lifetime must be non-negative")
	}
	if s.Interval < 0 {
		return fmt.Errorf("interval must be non-negative")
	}
	if s.MinDistance < 0 {
		return fmt.Errorf("minDistance must be non-negative")
	}
	if s.AlphaStart < 0 || s.AlphaStart > 1 {
		return fmt.Errorf("alphaStart must be between 0 and 1")
	}
	if s.AlphaEnd < 0 || s.AlphaEnd > 1 {
		return fmt.Errorf("alphaEnd must be between 0 and 1")
	}
	if s.MaxSamples < 0 {
		return fmt.Errorf("maxSamples must be non-negative")
	}
	return nil
}
