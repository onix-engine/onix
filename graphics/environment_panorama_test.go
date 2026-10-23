package graphics

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"
)

type testLinearPanorama struct {
	width, height int
	pixels        []mgl32.Vec3
}

func (p testLinearPanorama) Size() (int, int)       { return p.width, p.height }
func (p testLinearPanorama) At(x, y int) mgl32.Vec3 { return p.pixels[y*p.width+x] }

func testRGBERadiance(atlas []byte) mgl32.Vec3 {
	if atlas[3] == 0 {
		return mgl32.Vec3{}
	}
	scale := float32(math.Exp2(float64(int(atlas[3])-128))) / 255
	return mgl32.Vec3{float32(atlas[0]) * scale, float32(atlas[1]) * scale, float32(atlas[2]) * scale}
}

func TestPanoramaEnvironmentPreservesBackgroundResolutionAndDetail(t *testing.T) {
	p := testLinearPanorama{512, 256, make([]mgl32.Vec3, 512*256)}
	for y := range p.height {
		for x := range p.width {
			// Adjacent sub-atlas-resolution texels carry distinct red/blue details.
			value := mgl32.Vec3{1, .25, .125}
			if x%2 == 0 {
				value = mgl32.Vec3{.125, .25, 1}
			}
			p.pixels[y*p.width+x] = value
		}
	}
	images, err := BakePanoramaEnvironment(p, 32, 64)
	if err != nil {
		t.Fatal(err)
	}
	if images.Panorama.Bounds().Dx() != 512 || images.Panorama.Bounds().Dy() != 256 || images.Atlas.Bounds().Dx() != 32 {
		t.Fatal("background inherited atlas dimensions")
	}
	for x := 0; x < 8; x++ {
		pixel := images.Panorama.Pix[(128*512+x)*4:]
		if got := testRGBERadiance(pixel); got.Sub(p.At(x, 128)).Len() > .006 {
			t.Fatalf("source detail was filtered at %d: %v", x, got)
		}
	}
	// Opposing atlas borders duplicate the same longitude without blending
	// encoded exponents; the background itself retains every original texel.
	if allocations := testing.AllocsPerRun(100, func() { _ = SamplePanorama(p, mgl32.Vec3{1, 0, 0}) }); allocations != 0 {
		t.Fatal("panorama sampling allocated", allocations)
	}
}

func TestPanoramaDiffuseConvolutionMatchesLinearGradient(t *testing.T) {
	p := testLinearPanorama{128, 64, make([]mgl32.Vec3, 128*64)}
	for y := range p.height {
		for x := range p.width {
			n := environmentDirection((float32(x)+.5)/float32(p.width), (float32(y)+.5)/float32(p.height))
			value := 1 + .5*n.X()
			p.pixels[y*p.width+x] = mgl32.Vec3{value, value, value}
		}
	}
	images, err := BakePanoramaEnvironment(p, 32, 256)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range []int{1, 8, 16, 23, 30} {
		u, v := (float32(x)-.5)/30, (float32(8)-.5)/14
		n := environmentDirection(u, v)
		// Integral of (1 + .5 L.x) cos(theta)/pi over a hemisphere.
		want := 1 + n.X()/3
		got := testRGBERadiance(images.Atlas.Pix[((6*16+8)*32+x)*4:])[0]
		if math.Abs(float64(got-want)) > .035 {
			t.Fatalf("diffuse %d = %g, analytic %g", x, got, want)
		}
	}
}

func TestPanoramaPrefilterIntegratesSmallHDRLight(t *testing.T) {
	p := testLinearPanorama{256, 128, make([]mgl32.Vec3, 256*128)}
	p.pixels[64*256+128] = mgl32.Vec3{60000, 60000, 60000}
	images, err := BakePanoramaEnvironment(p, 64, 256)
	if err != nil {
		t.Fatal(err)
	}
	omega := (2 * math.Pi / 256) * (math.Cos(math.Pi*64/128) - math.Cos(math.Pi*65/128))
	expected := 60000 * omega / math.Pi
	for _, level := range []int{5, 6} {
		got := testRGBERadiance(images.Atlas.Pix[((level*32+16)*64+32)*4:])[0]
		// GGX at roughness 1 and Lambert both integrate this cosine-weighted light.
		if math.Abs(float64(got)-expected) > expected*.25 {
			t.Fatalf("small HDR light level %d = %g, analytic %g", level, got, expected)
		}
	}
	// Neighboring rough texels must vary smoothly rather than flicker from 0
	// to thousands depending on whether one ray happens to hit the source.
	for x := 27; x < 37; x++ {
		value := testRGBERadiance(images.Atlas.Pix[((6*32+16)*64+x)*4:])[0]
		if value < float32(expected*.5) || value > float32(expected*1.5) {
			t.Fatalf("diffuse HDR sparkle at %d: %g (peak %g)", x, value, expected)
		}
	}
}
