package graphics

import (
	"testing"

	"github.com/diiyw/onix/render"
)

func BenchmarkCompactTextureUpload(b *testing.B) {
	for _, format := range []render.TextureFormat{render.TextureFormatR8, render.TextureFormatRGBA8} {
		b.Run(format.Name(), func(b *testing.B) {
			driver := &textureTestDriver{}
			device, err := render.NewDevice(driver)
			if err != nil {
				b.Fatal(err)
			}
			defer device.Destroy()
			texture := CreateEmptyTexture(1024, 1024, format, ResourceUsageDefault)
			defer texture.Dispose()
			texture.pix = []byte{173}
			texture.RenderHandleFor(device)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if err := texture.destroyHandle(); err != nil {
					b.Fatal(err)
				}
				if !texture.RenderHandleFor(device).Valid() {
					b.Fatal(texture.UploadError())
				}
			}
		})
	}
}
