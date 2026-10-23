// Package texcontainer describes the byte layout of the block-compressed
// texture container a build packs in place of a source image.
//
// It deliberately depends on nothing. A packaged project keeps the source path
// for a packed texture, so code that only has to recognize a container or read
// its dimensions must not link the rendering stack to do it.
package texcontainer

import "encoding/binary"

const (
	// MagicSize is the length of the container signature.
	MagicSize = 8
	// HeaderSize is the fixed prefix that precedes the payload.
	HeaderSize = 20
)

// Magic identifies the container.
var Magic = [MagicSize]byte{'O', 'N', 'I', 'X', 'T', 'E', 'X', '1'}

// Encoded reports whether a payload starts with the container signature.
func Encoded(data []byte) bool {
	return len(data) >= MagicSize && [MagicSize]byte(data[:MagicSize]) == Magic
}

// Dimensions reads the stored texel dimensions from a container header. It
// reports false when data is not a complete container header.
func Dimensions(data []byte) (width, height int, ok bool) {
	if len(data) < HeaderSize || !Encoded(data) {
		return 0, 0, false
	}
	width = int(binary.LittleEndian.Uint32(data[12:16]))
	height = int(binary.LittleEndian.Uint32(data[16:20]))
	return width, height, true
}
