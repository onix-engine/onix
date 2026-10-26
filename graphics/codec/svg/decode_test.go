package svg

import (
	"testing"

	"github.com/diiyw/onix/graphics"
	onixsvg "github.com/diiyw/onix/images/svg"
)

type sizeRasterScale struct{}

func (sizeRasterScale) SVGTextureRasterScale(width, height int) int {
	if width*height > 500 {
		return 1
	}
	return 2
}

func TestScaledDecoderSelectsResolutionFromIntrinsicSize(t *testing.T) {
	for _, test := range []struct {
		name, source         string
		width, height, scale int
	}{
		{"small", `<svg width="12" height="8"><rect width="12" height="8"/></svg>`, 12, 8, 2},
		{"large", `<svg width="30" height="20"><rect width="30" height="20"/></svg>`, 30, 20, 1},
		{"viewbox", `<svg viewBox="0 0 30 20"><circle cx="15" cy="10" r="8"/></svg>`, 30, 20, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			texture, err := (ScaledDecoder[sizeRasterScale]{}).DecodeImage([]byte(test.source), graphics.TextureOptions{Usage: graphics.ResourceUsageCPU})
			if err != nil {
				t.Fatal(err)
			}
			defer texture.Dispose()
			if texture.Width != test.width*test.scale || texture.Height != test.height*test.scale ||
				texture.LogicalWidth() != test.width || texture.LogicalHeight() != test.height || !texture.Mipmapped() {
				t.Fatalf("physical=%dx%d logical=%dx%d mipmaps=%v", texture.Width, texture.Height, texture.LogicalWidth(), texture.LogicalHeight(), texture.Mipmapped())
			}
		})
	}
}

func TestDefaultDecoderRetainsResolutionAndFiltersMinification(t *testing.T) {
	texture, err := (Decoder{}).DecodeImage([]byte(`<svg width="16" height="12"><circle cx="8" cy="6" r="5"/></svg>`), graphics.TextureOptions{Usage: graphics.ResourceUsageCPU})
	if err != nil {
		t.Fatal(err)
	}
	defer texture.Dispose()
	if texture.Width != 32 || texture.Height != 24 || texture.LogicalWidth() != 16 || texture.LogicalHeight() != 12 {
		t.Fatalf("physical size %dx%d, logical size %dx%d", texture.Width, texture.Height, texture.LogicalWidth(), texture.LogicalHeight())
	}
	if !texture.Mipmapped() || texture.MipLevels() != 6 {
		t.Fatalf("SVG mip levels = %d, want 6", texture.MipLevels())
	}
}

func TestDecodeAtScaleKeepsLogicalSize(t *testing.T) {
	texture, err := DecodeAtScale([]byte(`<svg width="20" height="10"><rect width="20" height="10" fill="#fff"/></svg>`), 2, graphics.TextureOptions{Usage: graphics.ResourceUsageCPU})
	if err != nil {
		t.Fatalf("decode scaled svg: %v", err)
	}
	if texture.Width != 40 || texture.Height != 20 {
		t.Fatalf("physical texture size = %dx%d, want 40x20", texture.Width, texture.Height)
	}
	if texture.LogicalWidth() != 20 || texture.LogicalHeight() != 10 {
		t.Fatalf("logical texture size = %dx%d, want 20x10", texture.LogicalWidth(), texture.LogicalHeight())
	}
	if texture.Trimmed() {
		t.Fatal("scaled SVG must retain its full texture bounds")
	}
	if scaleX, scaleY := texture.RasterScale(); scaleX != 2 || scaleY != 2 {
		t.Fatalf("raster scale = %gx%g, want 2x2", scaleX, scaleY)
	}
}

func TestDecodeAtScaleRejectsInvalidScale(t *testing.T) {
	if _, err := DecodeAtScale([]byte(`<svg width="1" height="1"/>`), 0, graphics.TextureOptions{}); err == nil {
		t.Fatal("expected invalid raster scale to fail")
	}
}

func TestDecodeHonorsPremultiplyAlpha(t *testing.T) {
	texture, err := Decode([]byte(`<svg width="1" height="1"><rect width="1" height="1" fill="#c86432" fill-opacity="0.5"/></svg>`), onixsvg.Options{}, graphics.TextureOptions{PremultiplyAlpha: true, Usage: graphics.ResourceUsageCPU})
	if err != nil {
		t.Fatalf("decode SVG: %v", err)
	}
	defer texture.Dispose()
	if !texture.PremultipliedAlpha() {
		t.Fatal("SVG texture lost premultiplied-alpha metadata")
	}
}
