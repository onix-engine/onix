package html

import (
	_ "embed"
	"image"
	"sync"

	"github.com/onix-engine/onix/graphics"
)

//go:generate go run resources/icons/generate.go

// The built-in lucide:chevron-down is always rasterized at its intrinsic size.
// Embed its pixels so a fixed control decoration needs no runtime SVG decoder.
//
//go:embed resources/icons/chevron-down.rgba
var htmlSelectIconPixels []byte

var htmlSelectIcon struct {
	texture *graphics.Texture
	sync.Once
}

func defaultHTMLSelectIcon() *graphics.Texture {
	htmlSelectIcon.Do(func() {
		const size = 16
		img := &image.NRGBA{Pix: htmlSelectIconPixels, Stride: size * 4, Rect: image.Rect(0, 0, size, size)}
		htmlSelectIcon.texture = graphics.TextureFromOwnedNRGBA(img, graphics.TextureOptions{Usage: graphics.ResourceUsageDefault})
	})
	return htmlSelectIcon.texture
}
