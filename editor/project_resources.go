package editor

import (
	"path"
	"strings"

	"github.com/onix-engine/onix/audio/codec"
	"github.com/onix-engine/onix/font"
	fontwoff2 "github.com/onix-engine/onix/font/codec/woff2"
	"github.com/onix-engine/onix/graphics"
	graphicsgif "github.com/onix-engine/onix/graphics/codec/gif"
	graphicsjpeg "github.com/onix-engine/onix/graphics/codec/jpeg"
	"github.com/onix-engine/onix/graphics/codec/ontex"
	graphicspng "github.com/onix-engine/onix/graphics/codec/png"
	graphicssvg "github.com/onix-engine/onix/graphics/codec/svg"
	graphicswebp "github.com/onix-engine/onix/graphics/codec/webp"
	onixsvg "github.com/onix-engine/onix/images/svg"
	projectassets "github.com/onix-engine/onix/project/assets"
	windowpng "github.com/onix-engine/onix/window/codec/png"
)

type editorResourceFeature = projectassets.Feature[
	editorImageDecoder,
	font.DecoderSet[font.SFNTDecoder, fontwoff2.Decoder],
	windowpng.Decoder,
	codec.DefaultDecoder,
]

type editorImageDecoder struct{}

func (editorImageDecoder) DecodeImage(data []byte, imagePath string, options graphics.TextureOptions) (*graphics.Texture, error) {
	if strings.EqualFold(path.Ext(imagePath), ".svg") {
		return graphicssvg.Decode(data, onixsvg.Options{}, options)
	}
	return (graphics.ImageDecoderSet[
		graphicsgif.Decoder,
		graphicsjpeg.Decoder,
		graphicspng.Decoder,
		graphicswebp.Decoder,
		ontex.Decoder,
	]{}).DecodeImage(data, imagePath, options)
}
