package webp

import (
	"bytes"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/images/webp"
)

type Decoder struct{}

func (Decoder) ImageExtension() string { return ".webp" }

func (Decoder) DecodeImage(data []byte, options graphics.TextureOptions) (*graphics.Texture, error) {
	image, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return graphics.TextureFromImage(image, options), nil
}
