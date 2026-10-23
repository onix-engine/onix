package texcompress

import (
	"bytes"
	"testing"

	"github.com/diiyw/onix/render"
)

func TestEncodeChainMatchesIndependentLevels(t *testing.T) {
	for _, format := range Formats() {
		t.Run(format.Name(), func(t *testing.T) {
			t.Parallel()
			for _, size := range [][2]int{{1, 1}, {1, 9}, {8, 1}, {7, 5}, {16, 32}} {
				width, height := size[0], size[1]
				pixels := gradientPixels(width, height)
				original := bytes.Clone(pixels)
				chain, levels, err := EncodeChain(format, pixels, width, height)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(pixels, original) {
					t.Fatal("encoder mutated source pixels")
				}
				var want []byte
				for range levels {
					level, err := EncodeLevel(format, pixels, width, height)
					if err != nil {
						t.Fatal(err)
					}
					want = append(want, level...)
					pixels = downsample(pixels, width, height, format.SRGB())
					width, height = max(1, width/2), max(1, height/2)
				}
				for range 3 {
					other, _, err := EncodeChain(format, gradientPixels(11, 13), 11, 13)
					if err != nil {
						t.Fatal(err)
					}
					clear(other)
				}
				if !bytes.Equal(chain, want) {
					t.Fatalf("%dx%d mip chain changed during buffer reuse", size[0], size[1])
				}
			}
		})
	}
}

func TestEncodeChainRejectsInvalidInput(t *testing.T) {
	for _, format := range Formats() {
		if _, _, err := EncodeChain(format, nil, 0, 4); err == nil {
			t.Fatalf("%s accepted zero width", format.Name())
		}
		if _, _, err := EncodeChain(format, make([]byte, 4), 4, 4); err == nil {
			t.Fatalf("%s accepted short pixels", format.Name())
		}
	}
	if _, _, err := EncodeChain(render.TextureFormatRGBA8, gradientPixels(4, 4), 4, 4); err == nil {
		t.Fatal("unsupported format was accepted")
	}
}
