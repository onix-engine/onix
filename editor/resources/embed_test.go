package resources

import (
	"bytes"
	"image/color"
	"testing"

	"github.com/diiyw/onix/images/svg"
)

func TestLogoResourcesMatchTheirDeclaredSizes(t *testing.T) {
	for _, size := range []LogoSize{Logo16, Logo32, Logo48, Logo64, Logo128, Logo512, Logo1024} {
		texture := LogoTexture(size)
		if texture.Width != int(size) || texture.Height != int(size) {
			t.Fatalf("logo %d has size %dx%d", size, texture.Width, texture.Height)
		}
		if !texture.PremultipliedAlpha() {
			t.Fatalf("logo %d is not configured for premultiplied-alpha rendering", size)
		}
	}
}

func TestLogoSVGUsesReferencedDefinitionsAndClipPaths(t *testing.T) {
	data, err := LogoData()
	if err != nil {
		t.Fatal(err)
	}
	image, err := svg.Render(bytes.NewReader(data), svg.Options{Width: 128, Height: 128})
	if err != nil {
		t.Fatalf("render logo SVG: %v", err)
	}
	background := color.NRGBA{R: 52, G: 56, B: 63, A: 255}
	if pixel := image.NRGBAAt(0, 0); pixel.A != 0 {
		t.Errorf("logo SVG background is not transparent: alpha=%d", pixel.A)
	}
	for _, point := range [...]struct{ x, y int }{{75, 55}, {50, 65}, {82, 41}} {
		if pixel := image.NRGBAAt(point.x, point.y); pixel == background {
			t.Errorf("logo SVG did not render artwork at %d,%d", point.x, point.y)
		}
	}
}
