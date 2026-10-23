package assets

import (
	"github.com/diiyw/onix/audio/codec"
	"github.com/diiyw/onix/font"
	fontwoff2 "github.com/diiyw/onix/font/codec/woff2"
	"github.com/diiyw/onix/graphics"
	graphicsgif "github.com/diiyw/onix/graphics/codec/gif"
	graphicsjpeg "github.com/diiyw/onix/graphics/codec/jpeg"
	graphicspng "github.com/diiyw/onix/graphics/codec/png"
	graphicswebp "github.com/diiyw/onix/graphics/codec/webp"
	windowpng "github.com/diiyw/onix/window/codec/png"
)

// DefaultResourceFeature is the standard decoder bundle used by projects
// whose resource packages use the built-in asset formats.
type DefaultResourceFeature = Feature[
	graphics.ImageDecoderSet[
		graphicsgif.Decoder,
		graphicsjpeg.Decoder,
		graphicspng.Decoder,
		graphicswebp.Decoder,
		graphics.NoImageDecoder,
	],
	font.DecoderSet[font.SFNTDecoder, fontwoff2.Decoder],
	windowpng.Decoder,
	codec.DefaultDecoder,
]
