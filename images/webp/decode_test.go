package webp

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"testing"
)

func TestDecodeWebPVariants(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		width     int
		height    int
		imageType any
	}{
		{name: "vp8", path: "testdata/vp8-lossy.webp", width: 150, height: 100, imageType: &image.YCbCr{}},
		{name: "vp8l", path: "testdata/vp8l-lossless.webp", width: 75, height: 100, imageType: &image.NRGBA{}},
		{name: "vp8x-alpha", path: "testdata/vp8x-alpha.webp", width: 400, height: 301, imageType: &image.NYCbCrA{}},
		{name: "animated", path: "testdata/animated-lossless.webp", width: 8, height: 6, imageType: &image.NRGBA{}},
		{name: "animated-alpha-vp8", path: "testdata/animated-alpha-lossy.webp", width: 16, height: 12, imageType: &image.NRGBA{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, err := os.ReadFile(test.path)
			if err != nil {
				t.Fatal(err)
			}
			config, err := DecodeConfig(bytes.NewReader(data))
			if err != nil || config.Width != test.width || config.Height != test.height {
				t.Fatalf("config=%#v err=%v", config, err)
			}
			decoded, err := Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Bounds() != image.Rect(0, 0, test.width, test.height) {
				t.Fatalf("bounds = %v", decoded.Bounds())
			}
			switch test.imageType.(type) {
			case *image.YCbCr:
				if _, ok := decoded.(*image.YCbCr); !ok {
					t.Fatalf("decoded type = %T", decoded)
				}
			case *image.NRGBA:
				if _, ok := decoded.(*image.NRGBA); !ok {
					t.Fatalf("decoded type = %T", decoded)
				}
			case *image.NYCbCrA:
				alpha, ok := decoded.(*image.NYCbCrA)
				if !ok {
					t.Fatalf("decoded type = %T", decoded)
				}
				minimum, maximum := uint8(255), uint8(0)
				for _, value := range alpha.A {
					minimum = min(minimum, value)
					maximum = max(maximum, value)
				}
				if minimum != 0 || maximum != 255 {
					t.Fatalf("alpha range = %d..%d", minimum, maximum)
				}
			}

			registered, format, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if format != "webp" || registered.Bounds() != decoded.Bounds() {
				t.Fatalf("registered decode format=%q bounds=%v", format, registered.Bounds())
			}
		})
	}
}

func TestDecodeAnimatedAlphaVP8FirstFrame(t *testing.T) {
	data, err := os.ReadFile("testdata/animated-alpha-lossy.webp")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	outside := color.NRGBAModel.Convert(decoded.At(0, 0)).(color.NRGBA)
	inside := color.NRGBAModel.Convert(decoded.At(5, 5)).(color.NRGBA)
	if outside.A != 0 {
		t.Fatalf("outside pixel = %#v", outside)
	}
	if inside.R < 220 || inside.G > 40 || inside.B > 40 || inside.A < 120 || inside.A > 136 {
		t.Fatalf("inside pixel = %#v", inside)
	}
}

func TestDecodeAnimatedWebPFirstFrame(t *testing.T) {
	data, err := os.ReadFile("testdata/animated-lossless.webp")
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for point, want := range map[image.Point]color.NRGBA{
		{X: 0, Y: 0}: {},
		{X: 2, Y: 1}: {R: 255, A: 255},
		{X: 5, Y: 3}: {R: 255, A: 255},
		{X: 6, Y: 4}: {},
	} {
		got := color.NRGBAModel.Convert(decoded.At(point.X, point.Y)).(color.NRGBA)
		if got != want {
			t.Errorf("pixel %v = %#v, want %#v", point, got, want)
		}
	}
}

func TestDecodeAnimationFrameRejectsMalformedChunks(t *testing.T) {
	for name, chunks := range map[string][]byte{
		"unknown":       make([]byte, 8),
		"alpha only":    []byte("ALPH\x00\x00\x00\x00"),
		"trailing byte": []byte("VP8L\x00\x00\x00\x00\x00"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeAnimationFrame(chunks, 1, 1); err == nil {
				t.Fatal("malformed animation frame unexpectedly decoded")
			}
		})
	}
}

func TestDecodeWebPRejectsMalformedInput(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty": nil,
		"riff":  []byte("RIFF\x04\x00\x00\x00WEBP"),
		"other": []byte("not a webp image"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(bytes.NewReader(data)); err == nil {
				t.Fatal("malformed WebP unexpectedly decoded")
			}
		})
	}
}
