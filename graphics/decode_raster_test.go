package graphics_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/diiyw/onix/graphics"
	graphicspng "github.com/diiyw/onix/graphics/codec/png"
	graphicswebp "github.com/diiyw/onix/graphics/codec/webp"
)

func TestDecodeRasterCreatesTexture(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 3, 2))
	im.Set(1, 1, color.RGBA{R: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, im); err != nil {
		t.Fatal(err)
	}
	texture, err := graphicspng.Decode(encoded.Bytes(), graphics.TextureOptions{Usage: graphics.ResourceUsageCPU})
	if err != nil {
		t.Fatal(err)
	}
	defer texture.Dispose()
	if texture.Width != 3 || texture.Height != 2 || !texture.Valid() {
		t.Fatalf("texture = %#v", texture)
	}
}

func TestDecodeRasterCreatesTextureFromWebPWithAlpha(t *testing.T) {
	data, err := os.ReadFile("../images/webp/testdata/vp8x-alpha.webp")
	if err != nil {
		t.Fatal(err)
	}
	texture, err := graphicswebp.Decoder{}.DecodeImage(data, graphics.TextureOptions{Usage: graphics.ResourceUsageCPU})
	if err != nil {
		t.Fatal(err)
	}
	defer texture.Dispose()
	if texture.Width != 400 || texture.Height != 301 {
		t.Fatalf("texture dimensions=%dx%d", texture.Width, texture.Height)
	}
}
