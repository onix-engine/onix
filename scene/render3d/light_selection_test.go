package render3d

import (
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestViewLightSelectionVisibilityPriorityLayersAndReuse(t *testing.T) {
	projection := scene.NewPerspectiveProjection().Matrix(1)
	view := &renderView3D{}
	view.frame.Camera.screenProjection = projection
	candidates := make([]extractedLight3D, 0, 32)
	add := func(id ecs.Entity, kind render.Light3DKind, x, z, strength float32, layers scene.RenderLayers) {
		candidates = append(candidates, extractedLight3D{entity: id, layers: layers,
			Light3D: render.Light3D{Kind: kind, PositionDirection: [4]float32{x, 0, z, 1}, Color: [4]float32{1, 1, 1, 1}, Params: [4]float32{.1, strength}},
			shadows: extractedShadowLight3D{punctual: PunctualShadowSettings{Bias: float32(id)}},
		})
	}
	add(1, render.Light3DAmbient, 0, 0, 0, scene.RenderLayersNone())
	add(2, render.Light3DDirectional, 0, 0, 0, scene.RenderLayers{})
	for id := ecs.Entity(3); id < 25; id++ {
		add(id, render.Light3DPoint, 0, -float32(id), 1, scene.RenderLayers{})
	}
	add(25, render.Light3DPoint, 0, -2, math.MaxFloat32, scene.RenderLayers{}) // after the old global limit
	add(26, render.Light3DPoint, 100, -2, 10000, scene.RenderLayers{})
	add(27, render.Light3DPoint, 0, 2, 10000, scene.RenderLayers{})
	add(28, render.Light3DPoint, 0, -2, 10000, scene.RenderLayer(1))
	add(29, render.Light3DPoint, 0, -1, 0, scene.RenderLayers{})
	shadows := selectViewLights3D(view, candidates, scene.RenderLayers{}, 16)
	if len(view.lights) != 16 || view.lights[0].Kind != render.Light3DAmbient || view.lights[1].Kind != render.Light3DDirectional || shadows[2].punctual.Bias != 25 {
		t.Fatal("late nearby light did not displace distant lights", shadows)
	}
	for _, shadow := range shadows {
		if shadow.punctual.Bias >= 26 {
			t.Fatal("invisible, masked or disabled light was selected", shadow)
		}
	}
	for i, light := range view.lights {
		id := ecs.Entity(shadows[i].punctual.Bias)
		if light != candidates[id-1].Light3D {
			t.Fatal("shadow settings no longer match the selected light")
		}
	}
	other := &renderView3D{}
	other.frame.Camera.screenProjection = projection.Mul4(mgl32.HomogRotate3DY(float32(math.Pi)))
	selected := selectViewLights3D(other, candidates, scene.RenderLayers{}, 8)
	if len(other.lights) != 3 || selected[2].punctual.Bias != 27 || view.lights[2].PositionDirection[2] != -2 {
		t.Fatal("opposite cameras did not select independent lights")
	}
	selected = selectViewLights3D(other, candidates, scene.RenderLayer(1), 8)
	if len(other.lights) != 1 || other.lights[0].Kind != render.Light3DAmbient {
		t.Fatal("light layers were not isolated")
	}
	other.frame.Camera.screenProjection = projection
	selected = selectViewLights3D(other, candidates, scene.RenderLayer(1), 8)
	if len(other.lights) != 2 || selected[1].punctual.Bias != 28 {
		t.Fatal("camera did not select the light in its layer")
	}
	if allocations := testing.AllocsPerRun(100, func() { selectViewLights3D(view, candidates, scene.RenderLayers{}, 16) }); allocations != 0 {
		t.Fatalf("light selection allocated %g times", allocations)
	}
}

func TestExtractLightsRetainsAllCandidates(t *testing.T) {
	world := ecs.NewWorld()
	Register(world)
	for range 32 {
		b := ecs.NewBundle(2)
		b.Put(NewPointLight())
		b.Put(scene.GlobalTransform{Matrix: mgl32.Ident4()})
		world.Spawn(b)
	}
	ctx := &ecs.SystemContext{World: world}
	if err := extractRender3D(ctx); err != nil {
		t.Fatal(err)
	}
	state := WorldFor(world)
	if len(state.lights) != 33 { // ambient + every point light
		t.Fatalf("global extraction retained %d candidates", len(state.lights))
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if err := extractRender3D(ctx); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("light extraction allocated %g times", allocations)
	}
}
