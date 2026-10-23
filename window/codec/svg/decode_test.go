package svg

import "testing"

type testRasterSize struct{}

func (testRasterSize) SVGIconSize() (int, int) { return 12, 8 }

func TestDecoderRasterizesSVGAtSelectedSize(t *testing.T) {
	image, err := (Decoder[testRasterSize]{}).DecodeIcon([]byte(`<svg viewBox="0 0 3 2"><rect width="3" height="2" fill="red"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	if image.Bounds().Dx() != 12 || image.Bounds().Dy() != 8 {
		t.Fatalf("decoded icon size = %dx%d, want 12x8", image.Bounds().Dx(), image.Bounds().Dy())
	}
}
