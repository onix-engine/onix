package config

import (
	"bytes"
	"image"
	"image/color"
	"os"
	"testing"
)

func TestDecodeVariants(t *testing.T) {
	for _, test := range []struct {
		file          string
		width, height int
		model         color.Model
	}{
		{"vp8-lossy.webp", 150, 100, color.YCbCrModel},
		{"vp8l-lossless.webp", 75, 100, color.NRGBAModel},
		{"vp8x-alpha.webp", 400, 301, color.NYCbCrAModel},
		{"animated-lossless.webp", 8, 6, color.NYCbCrAModel},
		{"animated-alpha-lossy.webp", 16, 12, color.NYCbCrAModel},
	} {
		t.Run(test.file, func(t *testing.T) {
			data, err := os.ReadFile("../testdata/" + test.file)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Decode(bytes.NewReader(data))
			want := image.Config{ColorModel: test.model, Width: test.width, Height: test.height}
			if err != nil || got != want {
				t.Fatalf("config = %#v, error = %v; want %#v", got, err, want)
			}
			// Metadata queries must not retain Decode through the image registry.
			if _, _, err := image.Decode(bytes.NewReader(data)); err != image.ErrFormat {
				t.Fatalf("config package registered a pixel decoder: %v", err)
			}
		})
	}
}

func TestDecodeRejectsMalformedHeaders(t *testing.T) {
	data, err := os.ReadFile("../testdata/vp8x-alpha.webp")
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{0, 4, 12, 20, 29} {
		if _, err := Decode(bytes.NewReader(data[:size])); err == nil {
			t.Fatalf("accepted truncated %d-byte header", size)
		}
	}
	for _, offset := range []int{0, 8, 16} {
		invalid := bytes.Clone(data)
		invalid[offset]++
		if _, err := Decode(bytes.NewReader(invalid)); err == nil {
			t.Fatalf("accepted malformed header at offset %d", offset)
		}
	}
	for _, id := range []string{"ALPH", "ANIM", "ANMF"} {
		invalid := bytes.Clone(data)
		copy(invalid[12:16], id)
		if _, err := Decode(bytes.NewReader(invalid)); err == nil {
			t.Fatalf("accepted %s chunk without VP8X", id)
		}
	}
}
