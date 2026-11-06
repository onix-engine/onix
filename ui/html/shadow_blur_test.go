package html

import (
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/render"
	"image"
	"image/color"
	"math"
	"testing"
)

func TestHTMLShadowR8MatchesRGBAAndReusesColor(t *testing.T) {
	for _, blur := range []float64{0, 1, 3, 7, 8, 24, 80} {
		for _, alpha := range []uint8{17, 137, 255} {
			radius := style.Radius{Horizontal: style.LengthPx(7), Vertical: style.LengthPx(7)}
			computed := &style.ComputedStyle{BorderRadius: style.Corners{TopLeft: radius, TopRight: radius, BottomRight: radius, BottomLeft: radius}, BoxShadow: []style.Shadow{{BlurRadius: style.LengthPx(blur), SpreadRadius: style.LengthPx(-2), OffsetX: style.LengthPx(2.5), OffsetY: style.LengthPx(-1.5), Color: style.Color{R: 37, G: 121, B: 213, A: alpha}}}}
			state := htmlRenderState{}
			node := &dom.Node{}
			texture, version := htmlBoxShadowTextureForMode(&state, node, computed, 0, 73, 65, 41, 33, 16, false)
			raster := state.Textures.entries[htmlTextureKey{node: node, variant: "shadow-0"}].shadow
			var want image.RGBA
			var scratch htmlFilterScratch
			rasterHTMLBoxShadow(&want, computed, computed.BoxShadow[0], 41, 33, 16, &scratch)
			if texture.Format() != render.TextureFormatR8 || len(raster.image.Pix) != 73*65 {
				t.Fatal("shadow is not single-channel")
			}
			for y := range 65 {
				for x := range 73 {
					if raster.image.Pix[y*73+x] != want.RGBAAt(x, y).A {
						t.Fatalf("blur=%g alpha=%d at %d,%d: got %d want %d", blur, alpha, x, y, raster.image.Pix[y*73+x], want.RGBAAt(x, y).A)
					}
				}
			}
			computed.BoxShadow[0].Color.R++
			if next, v := htmlBoxShadowTextureForMode(&state, node, computed, 0, 73, 65, 41, 33, 16, false); next != texture || v != version {
				t.Fatal("recoloring rebuilt coverage")
			}
			state.Textures.dispose()
		}
	}
}

func TestHTMLShadowCoverageBlurReusesBuffers(t *testing.T) {
	for _, sigma := range []float64{1, 3, 4, 12} {
		dst := image.NewAlpha(image.Rect(0, 0, 80, 72))
		var scratch htmlFilterScratch
		paint := func() {
			clear(dst.Pix)
			dst.SetAlpha(40, 36, color.Alpha{A: 255})
			htmlBlurShadowCoverage(dst, sigma, &scratch)
		}
		paint()
		if n := testing.AllocsPerRun(50, paint); n != 0 {
			t.Fatalf("sigma %g warm R8 blur allocated %g times", sigma, n)
		}
		clear(dst.Pix)
		htmlBlurShadowCoverage(dst, sigma, &scratch)
		for _, alpha := range dst.Pix {
			if alpha != 0 {
				t.Fatalf("sigma %g retained an earlier shadow in scratch", sigma)
			}
		}
	}
}

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
