package html

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func TestHTMLShadowMaskBlurMatchesGaussian(t *testing.T) {
	for _, sigma := range []float64{.5, 1.5, 2, 4, 12, 24} {
		got := image.NewRGBA(image.Rect(0, 0, 97, 89))
		want := image.NewRGBA(got.Rect)
		ink := color.RGBA{R: 37, G: 121, B: 213, A: 180}
		for y := 25; y < 64; y++ {
			for x := 27; x < 70; x++ {
				got.SetRGBA(x, y, ink)
				want.SetRGBA(x, y, ink)
			}
		}
		var fast, reference htmlFilterScratch
		htmlBlurShadowMask(got, sigma, &fast)
		htmlGaussianBlur(want, sigma, &reference)
		for y := range got.Rect.Dy() {
			for x := range got.Rect.Dx() {
				actual, expected := got.RGBAAt(x, y), want.RGBAAt(x, y)
				if difference := math.Abs(float64(actual.A) - float64(expected.A)); difference > 4 {
					t.Fatalf("sigma %g at %d,%d: alpha %d, Gaussian %d", sigma, x, y, actual.A, expected.A)
				}
				if actual.A != 0 && (actual.R != ink.R || actual.G != ink.G || actual.B != ink.B) {
					t.Fatalf("sigma %g changed shadow color: %v", sigma, actual)
				}
			}
		}
	}
}

func TestHTMLShadowMaskBlurReusesBuffers(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 80, 72))
	var scratch htmlFilterScratch
	paint := func() {
		clear(dst.Pix)
		dst.SetRGBA(40, 36, color.RGBA{R: 255, A: 255})
		htmlBlurShadowMask(dst, 8, &scratch)
	}
	paint()
	if allocations := testing.AllocsPerRun(50, paint); allocations != 0 {
		t.Fatalf("warm shadow mask blur allocations = %g, want 0", allocations)
	}
	clear(dst.Pix)
	htmlBlurShadowMask(dst, 8, &scratch)
	for index := 3; index < len(dst.Pix); index += 4 {
		if dst.Pix[index] != 0 {
			t.Fatal("reused scratch retained a previous shadow")
		}
	}
}
