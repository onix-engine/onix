package html

import (
	"bytes"
	"testing"

	"github.com/diiyw/onix/images/svg"
)

func TestHTMLSelectIndicatorCanBeTintedByCSSColor(t *testing.T) {
	icon, err := svg.Render(bytes.NewReader(htmlSelectIconData), svg.Options{})
	if err != nil {
		t.Fatal(err)
	}
	visible := false
	for y := range icon.Bounds().Dy() {
		for x := range icon.Bounds().Dx() {
			pixel := icon.NRGBAAt(x, y)
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
