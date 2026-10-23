package svg

import (
	"image"
	"image/color"
	"slices"
	"testing"

	onixsvg "github.com/diiyw/onix/images/svg"
)

func TestTransparentColorsKeepFilteredContourBrightness(t *testing.T) {
	img, err := onixsvg.RenderString(`<svg width="8" height="8"><circle cx="4" cy="4" r="1.5" fill="#c86432"/></svg>`, onixsvg.Options{})
	if err != nil {
		t.Fatal(err)
	}
	before := slices.Clone(img.Pix)
	extendTransparentColors(img)
	for offset := 0; offset < len(img.Pix); offset += 4 {
		if img.Pix[offset+3] != before[offset+3] {
			t.Fatal("color extension changed coverage")
		}
		if !slices.Equal(img.Pix[offset:offset+3], []byte{200, 100, 50}) {
			t.Fatalf("pixel %d retains transparent black: %v", offset/4, img.Pix[offset:offset+4])
		}
	}
	// A 2x2 box is the first mip reduction. Its straight RGB must retain the
	// contour color even where only part of the footprint is covered.
	var red, alpha int
	for y := 2; y < 4; y++ {
		for x := 2; x < 4; x++ {
			pixel := img.NRGBAAt(x, y)
			red += int(pixel.R)
			alpha += int(pixel.A)
		}
	}
	if red != 4*200 || alpha <= 0 || alpha >= 4*255 {
		t.Fatalf("filtered contour red = %g, alpha = %g", float64(red)/4, float64(alpha)/4)
	}
}

func TestTransparentColorsPreserveVisiblePaints(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 5, 3))
	left := color.NRGBA{R: 200, A: 128}
	right := color.NRGBA{B: 100, A: 255}
	img.SetNRGBA(0, 1, left)
	img.SetNRGBA(4, 1, right)
	extendTransparentColors(img)
	if img.NRGBAAt(0, 1) != left || img.NRGBAAt(4, 1) != right {
		t.Fatal("visible paint changed")
	}
	if got := img.NRGBAAt(1, 0); got != (color.NRGBA{R: 200}) {
		t.Fatalf("left transparent color = %v", got)
	}
	if got := img.NRGBAAt(3, 2); got != (color.NRGBA{B: 100}) {
		t.Fatalf("right transparent color = %v", got)
	}
}

func TestTransparentColorsSkipUniformAlphaWithoutAllocating(t *testing.T) {
	for _, alpha := range []uint8{0, 255} {
		img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		for offset := 3; offset < len(img.Pix); offset += 4 {
			img.Pix[offset] = alpha
		}
		if allocations := testing.AllocsPerRun(10, func() { extendTransparentColors(img) }); allocations != 0 {
			t.Fatalf("uniform alpha %d allocated %g times", alpha, allocations)
		}
	}
}
