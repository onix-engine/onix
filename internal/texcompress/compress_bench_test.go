package texcompress

import (
	"testing"

	"github.com/diiyw/onix/render"
)

func BenchmarkEncodeChain(b *testing.B) {
	const width, height = 256, 256
	pixels := gradientPixels(width, height)
	for _, format := range []render.TextureFormat{render.TextureFormatBC1SRGB, render.TextureFormatBC5, render.TextureFormatASTC4x4SRGB} {
		b.Run(format.Name(), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, _, err := EncodeChain(format, pixels, width, height); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
