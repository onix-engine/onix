package graphics

import (
	"image"
	"image/color"
	"slices"
	"testing"
)

func TestStraightRGBAKeepsStraightAlpha(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 128})

	dst := StraightRGBA8(src)
	got := dst.Pix[:4]
	if got[0] != 200 || got[1] != 100 || got[2] != 50 || got[3] != 128 {
		t.Fatalf("unexpected straight RGBA pixel: %v", got)
	}
}

func TestDecodedTexturePremultipliesAlpha(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 128})
	src.SetNRGBA(1, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 0})

	texture := decodedTexture(src, TextureOptions{PremultiplyAlpha: true, Usage: ResourceUsageCPU})
	defer texture.Dispose()
	if got, want := texture.pix, []byte{100, 50, 25, 128, 0, 0, 0, 0}; !slices.Equal(got, want) {
		t.Fatalf("premultiplied pixels = %v, want %v", got, want)
	}
	if !texture.PremultipliedAlpha() {
		t.Fatal("texture lost its premultiplied-alpha metadata")
	}
}

func TestDecodedRGBAHandlesNonZeroBounds(t *testing.T) {
	src := image.NewNRGBA(image.Rect(5, 7, 7, 8))
	src.SetNRGBA(5, 7, color.NRGBA{R: 10, G: 20, B: 30, A: 40})
	src.SetNRGBA(6, 7, color.NRGBA{R: 50, G: 60, B: 70, A: 80})

	dst := decodedRGBA(src, false)
	if got, want := dst.Bounds(), image.Rect(0, 0, 2, 1); got != want {
		t.Fatalf("bounds = %v, want %v", got, want)
	}
	if got, want := dst.Pix, []byte{10, 20, 30, 40, 50, 60, 70, 80}; !slices.Equal(got, want) {
		t.Fatalf("pixels = %v, want %v", got, want)
	}
}

func TestOwnedImagePixelsAreTransferredToTexture(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 2, 1))
	texture := createTextureByOwnedImageWithUsage(im, false, ResourceUsageCPU)
	defer texture.Dispose()

	if &texture.pix[0] != &im.Pix[0] {
		t.Fatal("owned image pixels were copied")
	}
}

func TestOwnedNRGBAPixelsAreTransferredWithoutChangingStraightAlpha(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	im.SetNRGBA(0, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 128})
	texture := TextureFromOwnedNRGBA(im, TextureOptions{Usage: ResourceUsageCPU})
	defer texture.Dispose()

	if &texture.pix[0] != &im.Pix[0] {
		t.Fatal("owned NRGBA image pixels were copied")
	}
	if got, want := texture.pix, []byte{200, 100, 50, 128}; !slices.Equal(got, want) {
		t.Fatalf("straight pixels = %v, want %v", got, want)
	}
	if texture.PremultipliedAlpha() {
		t.Fatal("straight texture marked as premultiplied")
	}
}

func TestOwnedNRGBAPixelsPremultiplyInPlace(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	im.SetNRGBA(0, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 128})
	texture := TextureFromOwnedNRGBA(im, TextureOptions{PremultiplyAlpha: true, Usage: ResourceUsageCPU})
	defer texture.Dispose()

	if got, want := texture.pix, []byte{100, 50, 25, 128}; !slices.Equal(got, want) {
		t.Fatalf("premultiplied pixels = %v, want %v", got, want)
	}
	if !texture.PremultipliedAlpha() {
		t.Fatal("premultiplied texture lost metadata")
	}
}

func TestOwnedRGBAPixelsBecomeStraightAlphaWhenRequested(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 1, 1))
	im.SetRGBA(0, 0, color.RGBA{R: 100, G: 50, B: 25, A: 128})
	texture := TextureFromOwnedImage(im, TextureOptions{Usage: ResourceUsageCPU})
	defer texture.Dispose()

	if got, want := texture.pix, []byte{199, 100, 50, 128}; !slices.Equal(got, want) {
		t.Fatalf("straight pixels = %v, want %v", got, want)
	}
	if texture.PremultipliedAlpha() {
		t.Fatal("straight texture marked as premultiplied")
	}
}
