package ontex

import (
	"bytes"

	"github.com/diiyw/onix/graphics"
)

// Decoder plugs the ONIXTEX1 container into a graphics.ImageDecoderSet.
type Decoder struct{}

func (Decoder) ImageExtension() string { return Extension }

// ImageContainerMagic claims containers by their leading bytes. A build packs a
// compressed texture in place of the source image, so a container can sit
// behind any extension the source used.
func (Decoder) ImageContainerMagic() []byte { return magic[:] }

func (Decoder) DecodeImage(data []byte, options graphics.TextureOptions) (*graphics.Texture, error) {
	return Decode(data, options)
}

// Decode validates a container and owns its payload until explicit GPU upload.
// Resource package views may be released as soon as decoding returns.
func Decode(data []byte, options graphics.TextureOptions) (*graphics.Texture, error) {
	image, err := Unmarshal(data)
	if err != nil {
		return nil, err
	}
	pixels := bytes.Clone(image.Data)
	return graphics.TextureFromCompressed(graphics.CompressedTextureDescriptor{
		// The build packs one payload per image, so a sampler that wants the
		// other colour interpretation reuses the same blocks.
		Format:        image.Format.SRGBVariant(options.SRGB),
		Width:         image.Width,
		Height:        image.Height,
		Levels:        image.Levels,
		Mipmap:        options.Mipmap,
		Premultiplied: image.Premultiplied,
		WrapU:         options.WrapU,
		WrapV:         options.WrapV,
	}, pixels)
}
