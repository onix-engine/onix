package editor

import (
	"path"
	"strings"

	"github.com/diiyw/onix/audio/codec"
	"github.com/diiyw/onix/font"
	fontwoff2 "github.com/diiyw/onix/font/codec/woff2"
	"github.com/diiyw/onix/graphics"
	graphicsgif "github.com/diiyw/onix/graphics/codec/gif"
	graphicsjpeg "github.com/diiyw/onix/graphics/codec/jpeg"
	"github.com/diiyw/onix/graphics/codec/ontex"
	graphicspng "github.com/diiyw/onix/graphics/codec/png"
	graphicssvg "github.com/diiyw/onix/graphics/codec/svg"
	graphicswebp "github.com/diiyw/onix/graphics/codec/webp"
	onixsvg "github.com/diiyw/onix/images/svg"
	projectassets "github.com/diiyw/onix/project/assets"
	windowpng "github.com/diiyw/onix/window/codec/png"
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
