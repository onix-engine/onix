package render3d

import (
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestRenderQualityBudgetsInheritanceAndEffects(t *testing.T) {
	world := ecs.NewWorld()
	Register(world)
	world.InsertResource(QualityLow)
	budget, err := effectiveQuality3D(world, Camera{})
	if err != nil || budget.lights != 8 || budget.shadowMaps != 4 {
		t.Fatal("world quality was not inherited", budget, err)
	}
	ssao, ssr := NewSSAOSettings(), NewSSRSettings()
	ssao.Samples, ssao.ResolutionScale, ssr.Steps = 32, 1, 128
	for _, tier := range []Quality{QualityLow, QualityMedium, QualityHigh} {
		budget, err = effectiveQuality3D(world, Camera{Quality: tier})
		if err != nil {
			t.Fatal(err)
		}
		a, r := budget.apply(ssao, ssr)
		switch tier {
		case QualityLow:
			if a.Enabled || r.Enabled || budget.cascades != 1 || budget.shadowResolution != 512 {
				t.Fatal("low budget", budget, a, r)
			}
		case QualityMedium:
			if !a.Enabled || a.Samples != 16 || a.ResolutionScale != .5 || r.Enabled || budget.lights != 12 || budget.shadowMaps != 8 {
				t.Fatal("medium budget", budget, a, r)
			}
		case QualityHigh:
			if !a.Enabled || !r.Enabled || r.Steps != 64 || budget.shadowResolution != 2048 {
				t.Fatal("high budget", budget, a, r)
			}
		}
		a, r = budget.apply(SSAOSettings{}, SSRSettings{})
		if a.Enabled || r.Enabled {
			t.Fatal("quality enabled an unauthored effect")
		}
	}
	if ssao.Samples != 32 || ssao.ResolutionScale != 1 || ssr.Steps != 128 {
		t.Fatal("authoring settings were modified")
	}
	if _, err := effectiveQuality3D(world, Camera{Quality: "ultra"}); err == nil {
		t.Fatal("invalid camera quality accepted")
	}
	world.InsertResource(Quality("ultra"))
	if _, err := effectiveQuality3D(world, Camera{}); err == nil {
		t.Fatal("invalid world quality accepted")
	}
}

func TestRenderQualityShadowBudgetAndValidation(t *testing.T) {
	view := &renderView3D{budget: QualityLow.budget()}
	view.frame.Camera.screenProjection = scene.NewPerspectiveProjection().Matrix(1)
	view.frame.Camera.viewProjection = view.frame.Camera.screenProjection
	view.frame.Camera.position = mgl32.Vec3{}
	view.lights = []render.Light3D{
		{Kind: render.Light3DDirectional, Direction: [4]float32{0, -1, 0}},
		{Kind: render.Light3DPoint, Params: [4]float32{10, 100}},
		NewSpotLight().renderLight(mgl32.Ident4()),
	}
	var settings [render.MaxMeshLights]extractedShadowLight3D
	settings[0].directional = NewShadowSettings()
	settings[0].directional.Cascades = 4
	settings[0].directional.Radius = 0 // valid fitted cascades remain fitted when capped to one
	settings[1].punctual, settings[2].punctual = NewPunctualShadowSettings(), NewPunctualShadowSettings()
	if err := setupShadowAtlas3D(view, settings, false, 2048); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disposeViewTargets3D(view) })
	if view.shadow.count != 2 || view.lighting.LightShadows[0][1] != 1 || view.lighting.LightShadows[1][0] != 0 || view.lighting.LightShadows[2][1] != 1 {
		t.Fatal("low tier split a point shadow or failed to cap cascades")
	}
	for index, pass := range view.shadow.views[:view.shadow.count] {
		if pass.frame.Viewport.Width != 512 {
			t.Fatal("quality did not cap the override resolution")
		}
		for _, v := range pass.frame.Camera.viewProjection {
			if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
				t.Fatalf("shadow %d has a non-finite projection", index)
			}
		}
	}
	settings[0].directional.Cascades = 5
	if err := setupShadowAtlas3D(view, settings, false, 0); err == nil {
		t.Fatal("quality concealed invalid authored cascades")
	}
	settings[0].directional.Cascades = 4
	settings[1].punctual.Resolution = 5000
	if err := setupShadowAtlas3D(view, settings, false, 0); err == nil {
		t.Fatal("budget concealed invalid resolution on a skipped point shadow")
	}
	if err := setupShadowAtlas3D(view, settings, true, 0); err != nil {
		t.Fatal(err)
	}
	if view.lighting.LightShadows[0][0] != 0 || view.shadowActive {
		t.Fatal("disabled shadows retained stale bindings")
	}
}
