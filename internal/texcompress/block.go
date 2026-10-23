package texcompress

// blockSize is the texel edge length shared by every BC format.
const blockSize = 4

// blockScratch stages one 4x4 RGBA8 window so encoders read contiguous rows
// instead of chasing the source stride. One scratch buffer is reused across a
// whole level, so encoding allocates only the destination payload.
type blockScratch [blockSize * blockSize * 4]byte

// blockCount reports how many 4x4 blocks cover a level. Levels that are not a
// whole number of blocks round up, matching every native API.
func blockCount(width, height int) (int, int) {
	return (width + blockSize - 1) / blockSize, (height + blockSize - 1) / blockSize
}

// gatherBlock copies the 4x4 window whose top-left texel is (originX, originY)
// into scratch. Coordinates outside the level replicate the nearest edge texel:
// padding with transparent black would bleed dark fringes into the mip chain.
func gatherBlock(scratch *blockScratch, pixels []byte, stride, originX, originY, width, height int) {
	if originX >= 0 && originY >= 0 && originX+blockSize <= width && originY+blockSize <= height {
		for row := range blockSize {
			source := (originY+row)*stride + originX*4
			target := row * blockSize * 4
			copy(scratch[target:target+blockSize*4], pixels[source:source+blockSize*4])
		}
		return
	}
	for row := range blockSize {
		sourceY := min(max(originY+row, 0), height-1)
		for column := range blockSize {
			sourceX := min(max(originX+column, 0), width-1)
			source := sourceY*stride + sourceX*4
			target := (row*blockSize + column) * 4
			copy(scratch[target:target+4], pixels[source:source+4])
		}
	}
}

// pack565 quantizes an 8-bit colour into a BC1 endpoint.
func pack565(r, g, b uint8) uint16 {
	return uint16(r>>3)<<11 | uint16(g>>2)<<5 | uint16(b>>3)
}

// unpack565 expands a 5-6-5 endpoint back to 8-bit by replicating its high bits,
// which is what the sampler does when it reads a BC1 endpoint.
func unpack565(value uint16) (uint8, uint8, uint8) {
	r := uint8(value >> 11)
	g := uint8(value >> 5 & 0x3f)
	b := uint8(value & 0x1f)
	return r<<3 | r>>2, g<<2 | g>>4, b<<3 | b>>2
}
