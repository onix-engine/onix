package spine

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/diiyw/onix/graphics"
	graphicspng "github.com/diiyw/onix/graphics/codec/png"
)

func TestDecodeUsesAtlasMinFilter(t *testing.T) {
	skeleton := []byte(`{
		"bones":[{"name":"root"}],
		"slots":[{"name":"slot","bone":"root","attachment":"region"}],
		"skins":{"default":{"slot":{"region":{"width":1,"height":1}}}}
	}`)
	var texture bytes.Buffer
	if err := png.Encode(&texture, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		filter    string
		mipmapped bool
	}{
		{name: "linear", filter: "Linear,Linear"},
		{name: "mipmap", filter: "MipMapLinearLinear,Linear", mipmapped: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			atlas := []byte("page.png\nsize: 1,1\nformat: RGBA8888\nfilter: " + test.filter + "\nrepeat: none\nregion\nrotate: false\nxy: 0,0\nsize: 1,1\norig: 1,1\noffset: 0,0\nindex: -1\n")
			resource, err := Decode(skeleton, atlas, texture.Bytes(), "page.png", graphics.TextureOptions{Mipmap: !test.mipmapped, PremultiplyAlpha: true, Usage: graphics.ResourceUsageCPU}, pngImageDecoder())
			if err != nil {
				t.Fatal(err)
			}
			if got := resource.Texture.Mipmapped(); got != test.mipmapped {
				t.Fatalf("texture mipmapped = %v, want %v", got, test.mipmapped)
			}
			if !resource.Texture.PremultipliedAlpha() {
				t.Fatal("texture lost premultiplied alpha option")
			}
		})
	}
}

func pngImageDecoder() graphics.ImageDecoder {
	return graphics.ImageDecoderSet[graphics.NoImageDecoder, graphics.NoImageDecoder, graphicspng.Decoder, graphics.NoImageDecoder, graphics.NoImageDecoder]{}
}
