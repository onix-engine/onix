package webp

import (
	"bytes"

	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/images/webp"
)

type Decoder struct{}

func (Decoder) ImageExtension() string { return ".webp" }

func (Decoder) DecodeImage(data []byte, options graphics.TextureOptions) (*graphics.Texture, error) {
	image, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return graphics.TextureFromOwnedDecodedImage(image, options), nil
}
