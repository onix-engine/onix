package texcompress

import "github.com/diiyw/onix/render"

const astcBlockBytes = 16

var astcVoidExtentHeader = [8]byte{0xFC, 0xFD, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}

// EncodeASTCChain encodes an RGBA8 mip chain as standard ASTC 4x4 blocks.
//
// The encoder uses ASTC's constant-color (void-extent) block form. Every block
// stores the average RGBA value of its 4x4 texel footprint, including edge
// texels replicated to fill a partial block. This form is intentionally small,
// deterministic, and can be decoded by all ASTC 4x4 implementations. Encoding
// allocates the returned payload and borrows an RGBA mip buffer from the pool.
func EncodeASTCChain(pixels []byte, width, height int, srgb bool) ([]byte, int, error) {
	format := render.TextureFormatASTC4x4
	if srgb {
		format = render.TextureFormatASTC4x4SRGB
	}
	return EncodeChain(format, pixels, width, height)
}

func encodeASTCLevel(dst, pixels []byte, width, height int) {
	blocksX, blocksY := (width+3)/4, (height+3)/4
	for blockY := 0; blockY < blocksY; blockY++ {
		for blockX := 0; blockX < blocksX; blockX++ {
			block := dst[(blockY*blocksX+blockX)*astcBlockBytes:]
			encodeASTCConstantBlock(block, pixels, width, height, blockX*4, blockY*4)
		}
	}
}

func encodeASTCConstantBlock(dst, pixels []byte, width, height, originX, originY int) {
	// ASTC constant-color UNORM blocks have a fixed 64-bit header. The first
	// two bytes encode block mode 0x1FC and the remaining six bytes are the
	// reserved void-extent fields (all ones).
	copy(dst, astcVoidExtentHeader[:])
	var sums [4]int
	for row := 0; row < 4; row++ {
		y := min(originY+row, height-1)
		for column := 0; column < 4; column++ {
			x := min(originX+column, width-1)
			pixel := pixels[(y*width+x)*4:]
			for channel := range 4 {
				sums[channel] += int(pixel[channel])
			}
		}
	}
	for channel, sum := range sums {
		// 8-bit to UNORM16 expansion is exact when multiplying by 257. The
		// rounded average avoids a systematic darkening of constant blocks.
		value := uint16((sum + 8) / 16)
		value *= 257
		dst[8+channel*2] = byte(value)
		dst[9+channel*2] = byte(value >> 8)
	}
}
