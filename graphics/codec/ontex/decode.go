package ontex

import (
	"bytes"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
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

// Decode validates a container and hands its payload to the device.
//
// The payload normally arrives as a view into a resource package. A live
// render device consumes it synchronously and can borrow that view; when no
// device exists yet, take ownership of a copy for the deferred upload.
func Decode(data []byte, options graphics.TextureOptions) (*graphics.Texture, error) {
	image, err := Unmarshal(data)
	if err != nil {
		return nil, err
	}
	pixels := image.Data
	if render.Current() == nil {
		// Without a device TextureFromCompressed retains the payload for a later
		// upload, so it cannot keep borrowing the resource package view.
		pixels = bytes.Clone(pixels)
	}
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
