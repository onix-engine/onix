package gif

import (
	"bytes"
	stdgif "image/gif"

	"github.com/diiyw/onix/graphics"
)

type Decoder struct{}

func (Decoder) ImageExtension() string { return ".gif" }

func (Decoder) DecodeImage(data []byte, options graphics.TextureOptions) (*graphics.Texture, error) {
	image, err := stdgif.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return graphics.TextureFromImage(image, options), nil
}
