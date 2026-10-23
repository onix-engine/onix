package window

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"slices"
	"testing"
)

func TestLinuxIconPixelsKeepStraightAlpha(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	source.SetNRGBA(0, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 128})
	source.SetNRGBA(1, 0, color.NRGBA{R: 20, G: 40, B: 60, A: 255})

	var data bytes.Buffer
	if err := png.Encode(&data, source); err != nil {
		t.Fatalf("encode test icon: %v", err)
	}

	icon, err := loadIconNRGBA(data.Bytes(), testIconDecoder{})
	if err != nil {
		t.Fatalf("load test icon: %v", err)
	}
	if got, want := icon.Pix, source.Pix; !slices.Equal(got, want) {
		t.Fatalf("straight-alpha pixels = %v, want %v", got, want)
	}
	if got, want := nrgbaToARGB32(icon), []uint32{0x80c86432, 0xff14283c}; !slices.Equal(got, want) {
		t.Fatalf("X11 ARGB pixels = %#v, want %#v", got, want)
	}
}

type testIconDecoder struct{}

func (testIconDecoder) DecodeIcon(data []byte) (image.Image, error) {
	return png.Decode(bytes.NewReader(data))
}
