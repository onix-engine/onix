package assets

import (
	"github.com/onix-engine/onix/audio/codec"
	"github.com/onix-engine/onix/font"
	fontwoff2 "github.com/onix-engine/onix/font/codec/woff2"
	"github.com/onix-engine/onix/graphics"
	graphicsgif "github.com/onix-engine/onix/graphics/codec/gif"
	graphicsjpeg "github.com/onix-engine/onix/graphics/codec/jpeg"
	graphicspng "github.com/onix-engine/onix/graphics/codec/png"
	graphicswebp "github.com/onix-engine/onix/graphics/codec/webp"
	windowpng "github.com/onix-engine/onix/window/codec/png"
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
