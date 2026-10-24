package spine

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/diiyw/onix/graphics"
	graphicspng "github.com/diiyw/onix/graphics/codec/png"
)

func TestDecodePreprocessedSpineUsesCompiledMipmapFlag(t *testing.T) {
	skeleton := []byte(`{
		"skeleton":{"width":1,"height":1},
		"bones":[{"name":"root"}],
		"slots":[{"name":"slot","bone":"root","attachment":"region"}],
		"skins":{"default":{"slot":{"region":{"width":1,"height":1}}}}
	}`)
	atlas := []byte("page.png\nsize: 1,1\nformat: RGBA8888\nfilter: MipMapLinearLinear,Linear\nrepeat: none\nregion\nrotate: false\nxy: 0,0\nsize: 1,1\norig: 1,1\noffset: 0,0\nindex: -1\n")
	compiled, err := Preprocess(skeleton, atlas)
	if err != nil {
		t.Fatal(err)
	}
	var texture bytes.Buffer
	if err := png.Encode(&texture, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	resource, err := Decode(compiled, nil, texture.Bytes(), "page.png", graphics.TextureOptions{PremultiplyAlpha: true, Usage: graphics.ResourceUsageCPU}, graphics.ImageDecoderSet[graphics.NoImageDecoder, graphics.NoImageDecoder, graphicspng.Decoder, graphics.NoImageDecoder, graphics.NoImageDecoder]{})
	if err != nil {
		t.Fatal(err)
	}
	defer resource.Texture.Dispose()
	if !resource.Texture.Mipmapped() {
		t.Fatal("compiled atlas mipmap flag was not applied to the texture")
	}
	if !resource.Texture.PremultipliedAlpha() {
		t.Fatal("texture options were not preserved")
	}
}
