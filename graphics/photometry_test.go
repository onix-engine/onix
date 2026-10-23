package graphics

import (
	"math"
	"testing"
)

func TestHDRPPhotometricConversions(t *testing.T) {
	for _, test := range []struct {
		unit                         LightUnit
		value, angle, distance, want float32
	}{
		{LightLumen, 4 * math.Pi, 4 * math.Pi, 1, 1},
		{LightCandela, 100, 4 * math.Pi, 1, 100},
		{LightLux, 25, 4 * math.Pi, 2, 100},
		{LightEV100, 0, 4 * math.Pi, 1, 0.125},
		{LightEV100, -3, 4 * math.Pi, 1, 0.015625},
	} {
		got := PunctualCandela(test.value, test.unit, test.angle, test.distance)
		if math.Abs(float64(got-test.want)) > 1e-5 {
			t.Fatalf("%s: %g != %g", test.unit, got, test.want)
		}
		back := IntensityFromPhysical(got, test.unit, test.angle, test.distance)
		if math.Abs(float64(back-test.value)) > 1e-4 {
			t.Fatalf("%s roundtrip: %g != %g", test.unit, back, test.value)
		}
	}
	if got := AreaNits(math.Pi*2, LightLumen, math.Pi*2); got != 1 {
		t.Fatal(got)
	}
	if err := ValidateLightIntensity(-1, LightEV100, LightEV100); err != nil {
		t.Fatal(err)
	}
	for _, value := range []float32{-1, float32(math.NaN()), float32(math.Inf(1))} {
		if ValidateLightIntensity(value, LightLumen, LightLumen) == nil {
			t.Fatal("invalid intensity accepted")
		}
	}
}

func TestPhysicalExposureAndEmission(t *testing.T) {
	e := NewExposure()
	e.Mode = ExposurePhysical
	// f/16, 1/125 s, ISO100 => 32000, calibrated exposure 1/38400.
	if math.Abs(float64(e.Multiplier()-1.0/38400)) > 1e-10 {
		t.Fatal(e.Multiplier())
	}
	base := e.Multiplier()
	e.Compensation = 1
	if e.Multiplier() != base*2 {
		t.Fatal("one EV must double exposure")
	}
	e.Mode = ExposureFixed
	e.EV100 = 0
	e.Compensation = 0
	if math.Abs(float64(e.Multiplier()-1.0/1.2)) > 1e-7 {
		t.Fatal("explicit zero EV was treated as default")
	}
	material := NewStandardMaterial(White)
	if material.Material3D().IOR != 1.5 || material.Material3D().EmissiveStrength != 1 {
		t.Fatal("invalid PBR defaults")
	}
	material.EmissiveStrength = 64
	if err := material.SetEmission(White, 3, LightEV100); err != nil {
		t.Fatal(err)
	}
	if material.Emissive.R != 1 || material.Material3D().Emissive.G != 1 || material.EmissiveStrength != 1 {
		t.Fatal("emission did not preserve nits")
	}
	if a := testing.AllocsPerRun(100, func() { _ = e.Multiplier(); _ = material.SetEmission(White, 100, LightNits) }); a != 0 {
		t.Fatalf("photometry allocated %g", a)
	}
}
