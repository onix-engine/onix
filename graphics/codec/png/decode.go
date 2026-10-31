package png

import (
	"bytes"

	"github.com/diiyw/onix/graphics"
	onixpng "github.com/diiyw/onix/images/png"
)

type Decoder struct{}

func (Decoder) ImageExtension() string { return ".png" }

func (Decoder) DecodeImage(data []byte, options graphics.TextureOptions) (*graphics.Texture, error) {
	return Decode(data, options)
}

func Decode(data []byte, options graphics.TextureOptions) (*graphics.Texture, error) {
	image, err := onixpng.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return graphics.TextureFromOwnedDecodedImage(image, options), nil
}
