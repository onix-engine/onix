package window

import (
	"fmt"
	"image"
)

// IconDecoder decodes a window icon selected by an application resource
// feature. Concrete image formats live outside the window core package.
type IconDecoder interface {
	DecodeIcon([]byte) (image.Image, error)
}

type iconDecoderSupport struct {
	decoder IconDecoder
}

// SetIconDecoder configures the decoder used by SetIcon.
func (support *iconDecoderSupport) SetIconDecoder(decoder IconDecoder) {
	if support != nil {
		support.decoder = decoder
	}
}

func (support *iconDecoderSupport) decodeIcon(data []byte) (image.Image, error) {
	if support == nil || support.decoder == nil {
		return nil, fmt.Errorf("window icon decoder is not configured")
	}
	return support.decoder.DecodeIcon(data)
}
