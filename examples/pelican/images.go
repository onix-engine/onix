package main

import (
	"io/fs"

	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/graphics/codec/svg"
	"github.com/onix-engine/onix/module"
	"github.com/onix-engine/onix/project"
)

// Keep small, rotating sprites sharp while limiting background rasterization
// to intrinsic resolution. At 2x, this budget allows one million base texels.
type pelicanRasterScale struct{}

func (pelicanRasterScale) SVGTextureRasterScale(width, height int) int {
	if int64(width)*int64(height) > 250_000 {
		return 1
	}
	return 2
}

func pelicanModuleOptions(source fs.FS) module.Options {
	options := OnixModuleOptions(source)
	options.Resources = project.ImageFeature[graphics.ImageDecoderWith[OnixRasterImageDecoder, svg.ScaledDecoder[pelicanRasterScale]]]{}
	return options
}
