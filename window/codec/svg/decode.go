// Package svg provides SVG window icon decoding at a statically selected size.
package svg

import (
	"bytes"
	"fmt"
	"image"

	onixsvg "github.com/diiyw/onix/images/svg"
)

type RasterSize interface {
	SVGIconSize() (width, height int)
}

// Size128 selects the standard desktop window icon resolution.
type Size128 struct{}

func (Size128) SVGIconSize() (int, int) { return 128, 128 }

type Decoder[S RasterSize] struct{}

func (Decoder[S]) DecodeIcon(data []byte) (image.Image, error) {
	var size S
	width, height := size.SVGIconSize()
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("svg window icon size must be positive, got %dx%d", width, height)
	}
	return onixsvg.Render(bytes.NewReader(data), onixsvg.Options{Width: width, Height: height})
}
