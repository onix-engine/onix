package graphics

import (
	"testing"

	"github.com/diiyw/onix/render"
)

// A packed texture reaches the backend as one flat payload plus a level count.
// If the count were wrong the device would sample a chain that is not there,
// and if the payload were retained on the CPU the whole point of packing it
// would be lost, so both are asserted here rather than assumed.
func TestTextureFromCompressedReportsChainToDriver(t *testing.T) {
	driver := &canvasTestDriver{}
	installCanvasTestDriver(t, driver)

	width, height := 40, 24
	levels := render.MipLevels(width, height)
	pixels := make([]byte, render.TextureFormatBC1.ChainSize(width, height, levels))
	texture, err := TextureFromCompressed(CompressedTextureDescriptor{
		Format: render.TextureFormatBC1,
		Width:  width,
		Height: height,
		Levels: levels,
		Mipmap: true,
	}, pixels)
	if err != nil {
		t.Fatalf("adopt compressed texture: %v", err)
	}
	defer texture.Dispose()
	if len(driver.createdTextures) != 0 {
		t.Fatal("CPU construction called the render driver")
	}
	if !texture.RenderHandle().Valid() {
		t.Fatal("explicit upload failed", texture.UploadError())
	}

	if len(driver.createdTextures) != 1 {
		t.Fatalf("driver created %d textures, want 1", len(driver.createdTextures))
	}
	descriptor := driver.createdTextures[0]
	if descriptor.Format != render.TextureFormatBC1 {
		t.Fatalf("driver received format %s, want bc1", descriptor.Format.Name())
	}
	if descriptor.Levels != levels {
		t.Fatalf("driver received %d levels, want %d", descriptor.Levels, levels)
	}
	if descriptor.Filter != render.TextureFilterLinearMipmapLinear {
		t.Fatalf("driver received filter %v, want a mipmap filter for a multi-level chain", descriptor.Filter)
	}
	if driver.createdBytes[0] != len(pixels) {
		t.Fatalf("driver received %d bytes, want %d", driver.createdBytes[0], len(pixels))
	}
	// The compressed payload is the whole chain, so the device can sample it
	// without a CPU copy. Keeping one would defeat the memory saving.
	if texture.pix != nil {
		t.Fatal("compressed texture kept its CPU payload after upload")
	}
	if !texture.Format().Compressed() {
		t.Fatalf("texture reports format %s, want a compressed one", texture.Format().Name())
	}
}

func TestTextureFromCompressedRejectsMismatchedPayload(t *testing.T) {
	width, height := 16, 16
	levels := render.MipLevels(width, height)
	exact := render.TextureFormatBC3.ChainSize(width, height, levels)
	for _, testCase := range []struct {
		name   string
		format render.TextureFormat
		levels int
		length int
	}{
		{"short payload", render.TextureFormatBC3, levels, exact - 1},
		{"long payload", render.TextureFormatBC3, levels, exact + 1},
		{"one level of a chain", render.TextureFormatBC3, levels, render.TextureFormatBC3.LevelSize(width, height)},
		{"deeper than the dimensions allow", render.TextureFormatBC3, levels + 1, exact},
		{"zero levels", render.TextureFormatBC3, 0, exact},
		{"uncompressed format", render.TextureFormatRGBA8, levels, exact},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := TextureFromCompressed(CompressedTextureDescriptor{
				Format: testCase.format,
				Width:  width,
				Height: height,
				Levels: testCase.levels,
			}, make([]byte, testCase.length))
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
