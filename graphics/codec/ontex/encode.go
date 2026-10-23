package ontex

import (
	"fmt"

	"github.com/diiyw/onix/internal/texcompress"
	"github.com/diiyw/onix/render"
)

// EncodeOptions selects how source pixels are compressed.
type EncodeOptions struct {
	// Format requests one compressed format. Leave it zero to let Encode choose
	// between BC1 and BC3 based on whether the source has transparency.
	Format render.TextureFormat
	// SRGB marks colour textures. It selects the sRGB sampler variant so the
	// device decodes gamma the way the source expects.
	SRGB bool
	// Premultiplied records that source colour is already scaled by alpha. The
	// payload bakes it in, so the runtime cannot infer it later.
	Premultiplied bool
	// Mipmaps generates the full chain down to 1x1. Without it the container
	// carries a single level and the texture aliases when minified.
	Mipmaps bool
}

// SelectFormat reports the format Encode picks when Format is left zero: BC3
// whenever the source has transparency, because BC1 has no alpha channel, and
// BC1 otherwise.
func SelectFormat(pixels []byte, srgb bool) render.TextureFormat {
	opaque := texcompress.Opaque(pixels)
	switch {
	case !opaque && srgb:
		return render.TextureFormatBC3SRGB
	case !opaque:
		return render.TextureFormatBC3
	case srgb:
		return render.TextureFormatBC1SRGB
	default:
		return render.TextureFormatBC1
	}
}

// Encode compresses RGBA8 pixels into an ONIXTEX1 container.
func Encode(pixels []byte, width, height int, options EncodeOptions) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("ontex: invalid source size %dx%d", width, height)
	}
	if len(pixels) != width*height*4 {
		return nil, fmt.Errorf("ontex: source %dx%d needs %d bytes, got %d",
			width, height, width*height*4, len(pixels))
	}
	format := options.Format
	if format == 0 {
		format = SelectFormat(pixels, options.SRGB)
	}
	if !format.Compressed() || !texcompress.Supported(format) {
		return nil, fmt.Errorf("ontex: %s has no block-compressed encoder", format.Name())
	}
	levels := 1
	var data []byte
	var err error
	if options.Mipmaps {
		data, levels, err = texcompress.EncodeChain(format, pixels, width, height)
	} else {
		data, err = texcompress.EncodeLevel(format, pixels, width, height)
	}
	if err != nil {
		return nil, err
	}
	return Marshal(Image{
		Format:        format,
		Width:         width,
		Height:        height,
		Levels:        levels,
		Premultiplied: options.Premultiplied,
		Data:          data,
	})
}
