// Package png provides the opt-in PNG window icon decoder.
package png

import (
	"bytes"
	"image"

	onixpng "github.com/onix-engine/onix/images/png"
)

type Decoder struct{}

func (Decoder) DecodeIcon(data []byte) (image.Image, error) {
	return onixpng.Decode(bytes.NewReader(data))
}
