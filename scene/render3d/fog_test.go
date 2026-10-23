package render3d

import (
	"math"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

func TestFogSettingsValidationAndCameraIsolation(t *testing.T) {
	for _, change := range []func(*FogSettings){
		func(s *FogSettings) { s.End = s.Start },
		func(s *FogSettings) { s.Brightness = -1 },
		func(s *FogSettings) { s.Density = -1 },
		func(s *FogSettings) { s.Color.R = 2 },
		func(s *FogSettings) { s.Color.B = float32(math.NaN()) },
		func(s *FogSettings) { s.Start = float32(math.Inf(1)) },
		func(s *FogSettings) { s.Mode = "unknown" },
	} {
		s := NewFogSettings()
		change(&s)
		if s.Validate() == nil {
			t.Fatal("invalid fog accepted", s)
		}
	}
	s := NewFogSettings()
	s.Mode, s.Start, s.End, s.Density = FogExp2, 100, 0, 0
	if err := s.Validate(); err != nil {
		t.Fatal("exp2 must allow zero density and ignore end", err)
	}
	world := ecs.NewWorld()
	Register(world)
	state := NewWorld()
	state.lights = []extractedLight3D{{Light3D: render.Light3D{Kind: render.Light3DAmbient}}}
	b := ecs.NewBundle(1)
	fog := NewFogSettings()
	ecs.Put(&b, Camera{Fog: fog, ToneMapping: graphics.ToneMappingACES})
	first := world.Spawn(b)
	b = ecs.NewBundle(1)
	ecs.Put(&b, Camera{})
	second := world.Spawn(b)
	a, other := &renderView3D{}, &renderView3D{}
	a.frame.Camera.screenProjection = mgl32.Ident4()
	other.frame.Camera.screenProjection = mgl32.Ident4()
	if err := setupViewLighting3D(world, state, a, first); err != nil {
		t.Fatal(err)
	}
	if err := setupViewLighting3D(world, state, other, second); err != nil {
		t.Fatal(err)
	}
	color := fog.Color.Linear()
	if a.lighting.FogColor != ([3]float32{color.R * fog.Brightness, color.G * fog.Brightness, color.B * fog.Brightness}) || a.lighting.FogParams != ([4]float32{20, 120, .02, 1}) || a.lighting.ToneMapping != 1 {
		t.Fatal("fog did not convert sRGB/nits or curve selection")
	}
	if other.lighting.FogParams != ([4]float32{}) || other.lighting.ToneMapping != 0 {
		t.Fatal("camera fog/curve leaked")
	}
	settings := ecs.GetMut[Camera](world, first)
	if settings.Fog != fog {
		t.Fatal("fog authoring was modified")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if err := setupViewLighting3D(world, state, a, first); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("fog extraction allocated %g times", allocations)
	}
	settings.Fog.Enabled, settings.ToneMapping = false, ""
	if err := setupViewLighting3D(world, state, a, first); err != nil {
		t.Fatal(err)
	}
	if a.lighting.FogParams != ([4]float32{}) || a.lighting.FogColor != ([3]float32{}) || a.lighting.ToneMapping != 0 {
		t.Fatal("disabled effects retained camera data")
	}
}
