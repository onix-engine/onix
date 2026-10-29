package render3d

import (
	"fmt"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/render"
)

// Quality caps authored effects. It never enables an effect that the
// camera or light has disabled. A world resource supplies the default tier;
// a non-empty Camera.Quality overrides it for that camera.
type Quality string

const (
	QualityCustom Quality = ""
	QualityLow    Quality = "low"
	QualityMedium Quality = "medium"
	QualityHigh   Quality = "high"
)

type qualityBudget3D struct {
	lights, shadowMaps, shadowResolution, cascades int
	ssaoSamples                                    int
	ssaoScale                                      float32
	ssrSteps                                       int
}

func (q Quality) Validate() error {
	switch q {
	case QualityCustom, QualityLow, QualityMedium, QualityHigh:
		return nil
	default:
		return fmt.Errorf("3D quality must be empty, low, medium or high")
	}
}

func (q Quality) budget() qualityBudget3D {
	switch q {
	case QualityLow:
		return qualityBudget3D{lights: 8, shadowMaps: 4, shadowResolution: 512, cascades: 1}
	case QualityMedium:
		return qualityBudget3D{lights: 12, shadowMaps: 8, shadowResolution: 1024, cascades: 2, ssaoSamples: 16, ssaoScale: .5}
	case QualityHigh:
		return qualityBudget3D{lights: 16, shadowMaps: 16, shadowResolution: 2048, cascades: 4, ssaoSamples: 32, ssaoScale: 1, ssrSteps: 64}
	default:
		return qualityBudget3D{lights: render.MaxMeshLights, shadowMaps: render.MaxShadowMaps, shadowResolution: 8192, cascades: 4, ssaoSamples: 32, ssaoScale: 1, ssrSteps: 128}
	}
}

func effectiveQuality3D(world *ecs.World, camera Camera) (qualityBudget3D, error) {
	q := camera.Quality
	if q == QualityCustom {
		if global, ok := world.GetResource[Quality](); ok {
			q = global
		}
	}
	if err := q.Validate(); err != nil {
		return qualityBudget3D{}, err
	}
	return q.budget(), nil
}

func (b qualityBudget3D) apply(ssao SSAOSettings, ssr SSRSettings) (SSAOSettings, SSRSettings) {
	ssao.Enabled = ssao.Enabled && b.ssaoSamples > 0
	ssao.Samples = min(ssao.Samples, b.ssaoSamples)
	ssao.ResolutionScale = min(ssao.ResolutionScale, b.ssaoScale)
	ssr.Enabled = ssr.Enabled && b.ssrSteps > 0
	ssr.Steps = min(ssr.Steps, b.ssrSteps)
	return ssao, ssr
}
