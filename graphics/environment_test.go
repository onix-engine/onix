package graphics

import (
	"image"
	"math"
	"testing"

	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

type constantRadiance mgl32.Vec3

func (c constantRadiance) Radiance(mgl32.Vec3) mgl32.Vec3 { return mgl32.Vec3(c) }

func TestEnvironmentConvolutionPreservesConstantRadiance(t *testing.T) {
	want := mgl32.Vec3{.25, 1, 3}
	atlas, err := BakeEnvironment(constantRadiance(want), 32, 64)
	if err != nil {
		t.Fatal(err)
	}
	for level := 0; level < 7; level++ {
		for y := 0; y < 16; y++ {
			for x := 0; x < 32; x++ {
				c := atlas.NRGBAAt(x, level*16+y)
				for channel, value := range [3]byte{c.R, c.G, c.B} {
					decoded := float32(value) / 255 * float32(c.A) / 255 * 16
					if math.Abs(float64(decoded-want[channel])) > .02 {
						t.Fatalf("level %d: %v != %v", level, decoded, want[channel])
					}
				}
			}
		}
	}
	// A smooth, front-facing unit reflector should retain nearly all energy.
	ab := integrateEnvironmentBRDF(1, .045, 256)
	if ab[0] < .98 || ab[0] > 1.01 || ab[1] > .01 {
		t.Fatalf("integrated BRDF = %v", ab)
	}
}

func TestSRGBTexturePreservesAlphaAndDataMapFormat(t *testing.T) {
	input := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	copy(input.Pix, []byte{64, 128, 192, 127})
	color := TextureFromImage(input, TextureOptions{SRGB: true, Usage: ResourceUsageCPU})
	data := TextureFromImage(input, TextureOptions{Usage: ResourceUsageCPU})
	if color.Format() != render.TextureFormatSRGBA8 || data.Format() != render.TextureFormatRGBA8 {
		t.Fatal("color and data textures must have different GPU transfer functions")
	}
	if color.pix[3] != 127 || data.pix[3] != 127 {
		t.Fatal("alpha was converted or premultiplied")
	}
	for i := 0; i < 256; i++ {
		value := float32(i) / 255
		if math.Abs(float64(LinearToSRGB(SRGBToLinear(value))-value)) > 0.00001 {
			t.Fatalf("sRGB roundtrip failed for %d", i)
		}
	}
}

type contrastedRadiance struct{}

func (contrastedRadiance) Radiance(n mgl32.Vec3) mgl32.Vec3 {
	if n.X() > .5 {
		return mgl32.Vec3{65000, 50000, 30000}
	}
	return mgl32.Vec3{.17, .14, .29}
}

func TestHDREnvironmentPreservesDimColorsBesideBrightLights(t *testing.T) {
	atlas, err := BakeHDREnvironment(contrastedRadiance{}, 32, 16)
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct {
		x, y int
		want mgl32.Vec3
	}{{1, 8, mgl32.Vec3{.17, .14, .29}}, {16, 8, mgl32.Vec3{65000, 50000, 30000}}} {
		c := atlas.NRGBAAt(sample.x, sample.y)
		scale := float32(math.Exp2(float64(int(c.A)-128))) / 255
		for channel, value := range [3]byte{c.R, c.G, c.B} {
			decoded := float32(value) * scale
			if math.Abs(float64(decoded-sample.want[channel])) > float64(sample.want[channel])*.01 {
				t.Fatalf("RGBE sample %d,%d channel %d = %g, want %g", sample.x, sample.y, channel, decoded, sample.want[channel])
			}
		}
	}
	if atlas.NRGBAAt(16, 7*16+8).A != 255 {
		t.Fatal("BRDF tile must remain plain RGBA8")
	}
}
