package render3d

import (
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestHDRPLightShapesAndUnits(t *testing.T) {
	spot := NewSpotLight()
	spot.Intensity = math.Pi
	if math.Abs(float64(spot.PhysicalIntensity()-1/(2*(1-float32(math.Cos(math.Pi/6)))))) > 1e-5 {
		t.Fatal("cone solid angle")
	}
	spot.Reflector = false
	if math.Abs(float64(spot.PhysicalIntensity()-.25)) > 1e-6 {
		t.Fatal("reflector disabled must use sphere solid angle")
	}
	spot.Reflector = true
	spot.Shape = SpotPyramid
	spot.AspectRatio = 1
	spot.OuterConeAngle = math.Pi / 2
	if math.Abs(float64(spot.SolidAngle()-2*math.Pi/3)) > 1e-6 {
		t.Fatal(spot.SolidAngle())
	}
	spot.Shape = SpotBox
	spot.Unit = graphics.LightLux
	spot.Intensity = 100
	if spot.PhysicalIntensity() != 100 || spot.Validate() != nil {
		t.Fatal("box is measured in lux")
	}
	spot.Unit = graphics.LightLumen
	if spot.Validate() == nil {
		t.Fatal("box accepted lumens")
	}
	for _, shape := range []AreaLightShape{AreaRectangle, AreaDisc, AreaTube} {
		l := NewAreaLight()
		l.Shape = shape
		l.Width = 2
		l.Height = 1
		l.Radius = 1
		l.Intensity = l.ProjectedArea()
		if l.Validate() != nil || l.Nits() != 1 {
			t.Fatalf("%s failed unit conversion", shape)
		}
		before := l.renderLight(mgl32.Ident4())
		after := l.renderLight(mgl32.Scale3D(2, 3, 4))
		if before != after {
			t.Fatal("transform scale changed authored light size")
		}
	}
	spot = NewSpotLight()
	record := spot.renderLight(mgl32.Ident4())
	if math.Abs(float64(record.Params[3]-float32(math.Cos(math.Pi/6)))) > 1e-6 {
		t.Fatal("outer angle must be a full angle")
	}
}

func spawnPhotometricTestLight[T any](world *ecs.World, light T) ecs.Entity {
	b := ecs.NewBundle(2)
	ecs.Put(&b, light)
	ecs.Put(&b, scene.GlobalTransform{Matrix: mgl32.Ident4()})
	return world.Spawn(b)
}

func TestMixedPhotometricExtractionAndExposureNoAllocations(t *testing.T) {
	world := ecs.NewWorld()
	Register(world)
	state := NewWorld()
	t.Cleanup(func() { _ = state.Dispose() })
	spawnPhotometricTestLight(world, NewPointLight())
	for _, shape := range []SpotLightShape{SpotCone, SpotPyramid, SpotBox} {
		l := NewSpotLight()
		l.Shape = shape
		if shape == SpotBox {
			l.Unit = graphics.LightLux
		}
		spawnPhotometricTestLight(world, l)
	}
	for _, shape := range []AreaLightShape{AreaRectangle, AreaDisc, AreaTube} {
		l := NewAreaLight()
		l.Shape = shape
		spawnPhotometricTestLight(world, l)
	}
	hidden := spawnPhotometricTestLight(world, NewAreaLight())
	_ = ecs.Insert(world, hidden, scene.Visibility{Visible: false})
	invalid := NewPointLight()
	invalid.Intensity = float32(math.NaN())
	invalidEntity := spawnPhotometricTestLight(world, invalid)
	extract := func() {
		state.lights = state.lights[:0]
		extractLocalLights[PointLight](world, state)
		extractLocalLights[SpotLight](world, state)
		extractLocalLights[AreaLight](world, state)
	}
	extract()
	if len(state.lights) != 7 {
		t.Fatal(state.lights)
	}
	for i, kind := range []render.Light3DKind{render.Light3DPoint, render.Light3DSpot, render.Light3DPyramid, render.Light3DBox, render.Light3DRectangle, render.Light3DDisc, render.Light3DTube} {
		if state.lights[i].Kind != kind {
			t.Fatal(state.lights)
		}
	}
	_ = ecs.Insert(world, invalidEntity, scene.Visibility{Visible: false})
	if a := testing.AllocsPerRun(100, extract); a != 0 {
		t.Fatalf("light extraction allocated %g", a)
	}
	camera := spawnPhotometricTestLight(world, Camera{Exposure: graphics.NewExposure()})
	_ = ecs.Insert(world, camera, Environment{Exposure: 2})
	view := state.viewFor(camera)
	view.frame.Mode = render.RenderViewLit
	if err := setupViewLighting3D(world, state, view, camera); err != nil {
		t.Fatal(err)
	}
	if view.lighting.Params[0] != graphics.NewExposure().Multiplier()*2 {
		t.Fatal("camera and environment exposure did not compose")
	}
}
