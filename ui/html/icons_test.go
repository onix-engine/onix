package html

import (
	"bytes"
	_ "embed"
	"testing"

	"github.com/onix-engine/onix/images/svg"
)

//go:embed resources/icons/chevron-down.svg
var htmlSelectIconData []byte

func TestHTMLSelectIndicatorCanBeTintedByCSSColor(t *testing.T) {
	icon, err := svg.Render(bytes.NewReader(htmlSelectIconData), svg.Options{})
	if err != nil {
		t.Fatal(err)
	}
	texture := defaultHTMLSelectIcon()
	if texture.Width != icon.Rect.Dx() || texture.Height != icon.Rect.Dy() || len(htmlSelectIconPixels) != len(icon.Pix) {
		t.Fatal("baked indicator dimensions differ from the source SVG")
	}
	visible := false
	for y := range icon.Bounds().Dy() {
		for x := range icon.Bounds().Dx() {
			pixel := icon.NRGBAAt(x, y)
			i := icon.PixOffset(x, y)
			rgba := htmlSelectIconPixels[i : i+4]
			if rgba[0] != 255 || rgba[1] != 255 || rgba[2] != 255 || rgba[3] != pixel.A {
				t.Fatalf("baked indicator at (%d,%d) = %v, want white with alpha %d", x, y, rgba, pixel.A)
			}
			if pixel.A == 0 {
				continue
			}
			visible = true
			if pixel.R != 255 || pixel.G != 255 || pixel.B != 255 {
				t.Fatalf("indicator pixel %v is not a white tint mask; CSS color would be darkened", pixel)
			}
		}
	}
	if !visible {
		t.Fatal("indicator has no visible pixels")
	}
}
