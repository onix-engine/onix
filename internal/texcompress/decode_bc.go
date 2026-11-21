package texcompress

import (
	"encoding/binary"

	"github.com/onix-engine/onix/render"
)

// DecodeBCLevel expands one BC1 RGB or BC3 level, clipping partial edge
// blocks to its logical dimensions. Both buffers have been sized by the caller.
func DecodeBCLevel(dst, src []byte, width, height int, format render.TextureFormat) {
	stride := format.Block().Bytes
	for y := 0; y < height; y += 4 {
		for x := 0; x < width; x += 4 {
			block := src[:stride]
			src = src[stride:]
			color := block[stride-8:]
			first, second := binary.LittleEndian.Uint16(color), binary.LittleEndian.Uint16(color[2:])
			palette := bc1Palette(first, second)
			if first <= second {
				for c := range 3 {
					a, b := int(palette[0][c]), int(palette[1][c])
					if stride == 16 {
						palette[2][c], palette[3][c] = byte((2*a+b)/3), byte((a+2*b)/3)
					} else {
						palette[2][c] = byte((a + b) / 2)
					}
				}
			}
			colors := binary.LittleEndian.Uint32(color[4:])
			var alpha [8]byte
			var selectors uint64
			if stride == 16 {
				alpha = bc4Palette(block[0], block[1])
				selectors = binary.LittleEndian.Uint64(block) >> 16
			}
			for row := range min(4, height-y) {
				for col := range min(4, width-x) {
					i := row*4 + col
					rgba := palette[(colors>>(2*i))&3]
					rgba[3] = 255
					if stride == 16 {
						rgba[3] = alpha[(selectors>>(3*i))&7]
					}
					copy(dst[((y+row)*width+x+col)*4:], rgba[:])
				}
			}
		}
	}
}
