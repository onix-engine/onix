// Package texcompress implements pure-Go block compression for build-time
// texture preprocessing. Every encoder is deterministic and reuses scratch
// storage where possible, so a build stays reproducible and keeps a small heap.
//
// Encoders read straight or premultiplied RGBA8 pixels and never touch alpha
// unless the target format stores it. BC1 and BC3 read colour from the red,
// green, and blue channels; BC4 and BC5 read red and green, which is where
// normal and material maps keep their data. ASTC 4x4 stores all four channels
// in its standard constant-color block form.
package texcompress

import (
	"fmt"
	"sync"

	"github.com/diiyw/onix/internal/reuse"
	"github.com/diiyw/onix/render"
)

// Formats reports every format this package can encode.
func Formats() []render.TextureFormat {
	return []render.TextureFormat{
		render.TextureFormatBC1,
		render.TextureFormatBC1SRGB,
		render.TextureFormatBC3,
		render.TextureFormatBC3SRGB,
		render.TextureFormatBC4,
		render.TextureFormatBC5,
		render.TextureFormatASTC4x4,
		render.TextureFormatASTC4x4SRGB,
	}
}

// Supported reports whether format has an encoder in this package.
func Supported(format render.TextureFormat) bool {
	switch format {
	case render.TextureFormatBC1, render.TextureFormatBC1SRGB,
		render.TextureFormatBC3, render.TextureFormatBC3SRGB,
		render.TextureFormatBC4, render.TextureFormatBC5,
		render.TextureFormatASTC4x4, render.TextureFormatASTC4x4SRGB:
		return true
	default:
		return false
	}
}

// Opaque reports whether every texel of an RGBA8 level is fully opaque. BC1
// cannot store alpha, so callers use this to choose between BC1 and BC3.
func Opaque(pixels []byte) bool {
	for offset := 3; offset < len(pixels); offset += 4 {
		if pixels[offset] != 255 {
			return false
		}
	}
	return true
}

// EncodeLevel compresses one mip level of RGBA8 pixels.
func EncodeLevel(format render.TextureFormat, pixels []byte, width, height int) ([]byte, error) {
	if !Supported(format) {
		return nil, fmt.Errorf("texcompress: no encoder for %s", format.Name())
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("texcompress: invalid level size %dx%d", width, height)
	}
	if len(pixels) != width*height*4 {
		return nil, fmt.Errorf("texcompress: level %dx%d needs %d bytes, got %d",
			width, height, width*height*4, len(pixels))
	}
	dst := make([]byte, format.LevelSize(width, height))
	encodeLevelInto(format, dst, pixels, width, height)
	return dst, nil
}

func encodeLevelInto(format render.TextureFormat, dst, pixels []byte, width, height int) {
	if format == render.TextureFormatASTC4x4 || format == render.TextureFormatASTC4x4SRGB {
		encodeASTCLevel(dst, pixels, width, height)
		return
	}
	blocksX, blocksY := blockCount(width, height)
	blockBytes := format.Block().Bytes
	var scratch blockScratch
	offset := 0
	for blockY := range blocksY {
		for blockX := range blocksX {
			gatherBlock(&scratch, pixels, width*4, blockX*blockSize, blockY*blockSize, width, height)
			encodeBlock(format, dst[offset:offset+blockBytes], &scratch)
			offset += blockBytes
		}
	}
}

func encodeBlock(format render.TextureFormat, dst []byte, scratch *blockScratch) {
	switch format {
	case render.TextureFormatBC1, render.TextureFormatBC1SRGB:
		encodeBC1(dst, scratch, false)
	case render.TextureFormatBC3, render.TextureFormatBC3SRGB:
		// BC3 is an alpha block followed by a colour block, in that order. The
		// colour block is BC1 in four-colour mode, because index 3 has to be an
		// opaque interpolant: alpha comes from the other block.
		encodeBC4(dst[:bc4BlockBytes], scratch, 3)
		encodeBC1(dst[bc4BlockBytes:bc3BlockBytes], scratch, true)
	case render.TextureFormatBC4:
		encodeBC4(dst, scratch, 0)
	case render.TextureFormatBC5:
		encodeBC5(dst, scratch)
	}
}

// EncodeChain compresses a full mip chain from the largest level down to 1x1 and
// returns the concatenated payload together with its level count.
func EncodeChain(format render.TextureFormat, pixels []byte, width, height int) ([]byte, int, error) {
	if !Supported(format) {
		return nil, 0, fmt.Errorf("texcompress: no encoder for %s", format.Name())
	}
	levels := render.MipLevels(width, height)
	if levels == 0 {
		return nil, 0, fmt.Errorf("texcompress: invalid level size %dx%d", width, height)
	}
	if len(pixels) != width*height*4 {
		return nil, 0, fmt.Errorf("texcompress: level %dx%d needs %d bytes, got %d",
			width, height, width*height*4, len(pixels))
	}
	data := make([]byte, 0, format.ChainSize(width, height, levels))
	var mipScratch []byte
	if levels > 1 {
		buffer := mipScratchPool.Get().(*[]byte)
		*buffer = reuse.Resize(*buffer, max(1, width/2)*max(1, height/2)*4)
		mipScratch = *buffer
		defer releaseMipScratch(buffer)
	}
	srgb := format.SRGB()
	for level := range levels {
		start := len(data)
		data = data[:start+format.LevelSize(width, height)]
		encodeLevelInto(format, data[start:], pixels, width, height)
		if level+1 == levels {
			break
		}
		nextWidth, nextHeight := max(1, width/2), max(1, height/2)
		downsampleInto(mipScratch[:nextWidth*nextHeight*4], pixels, width, height, srgb)
		pixels = mipScratch[:nextWidth*nextHeight*4]
		width, height = nextWidth, nextHeight
	}
	return data, levels, nil
}

// Keep up to the first mip of a 2048x2048 texture per cached workspace.
const maxCachedMipBytes = 4 << 20

var mipScratchPool = sync.Pool{New: func() any { return new([]byte) }}

func releaseMipScratch(buffer *[]byte) {
	if cap(*buffer) > maxCachedMipBytes {
		*buffer = nil
	} else {
		*buffer = (*buffer)[:0]
	}
	mipScratchPool.Put(buffer)
}
