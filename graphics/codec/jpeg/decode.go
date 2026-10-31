package jpeg

import (
	"bytes"
	stdjpeg "image/jpeg"

	"github.com/diiyw/onix/graphics"
)

type Decoder struct{}

func (Decoder) ImageExtension() string { return ".jpg" }

func (Decoder) DecodeImage(data []byte, options graphics.TextureOptions) (*graphics.Texture, error) {
	image, err := stdjpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return graphics.TextureFromOwnedDecodedImage(image, options), nil
}
