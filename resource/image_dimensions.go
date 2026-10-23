package resource

import (
	"encoding/binary"
	"fmt"
	"image"
	"io"
)

// These readers only need dimensions; they do not register or import pixel
// decoders. The returned configs intentionally omit the color model.
func decodeGIFDimensions(reader io.Reader) (image.Config, error) {
	var scratch [768]byte
	if _, err := io.ReadFull(reader, scratch[:13]); err != nil {
		return image.Config{}, err
	}
	if string(scratch[:6]) != "GIF87a" && string(scratch[:6]) != "GIF89a" {
		return image.Config{}, image.ErrFormat
	}
	width := int(binary.LittleEndian.Uint16(scratch[6:8]))
	height := int(binary.LittleEndian.Uint16(scratch[8:10]))
	if fields := scratch[10]; fields&0x80 != 0 {
		// Verify that the global color table is present without allocating it.
		paletteSize := 3 * (1 << ((fields & 7) + 1))
		if _, err := io.ReadFull(reader, scratch[:paletteSize]); err != nil {
			return image.Config{}, err
		}
	}
	return image.Config{Width: width, Height: height}, nil
}

func decodeJPEGDimensions(reader io.Reader) (image.Config, error) {
	var scratch [512]byte
	if _, err := io.ReadFull(reader, scratch[:2]); err != nil {
		return image.Config{}, err
	}
	if scratch[0] != 0xff || scratch[1] != 0xd8 {
		return image.Config{}, image.ErrFormat
	}
	for {
		if _, err := io.ReadFull(reader, scratch[:2]); err != nil {
			return image.Config{}, err
		}
		if scratch[0] != 0xff {
			return image.Config{}, fmt.Errorf("jpeg: missing marker prefix")
		}
		for scratch[1] == 0xff {
			if _, err := io.ReadFull(reader, scratch[1:2]); err != nil {
				return image.Config{}, err
			}
		}
		marker := scratch[1]
		switch {
		case marker == 0xda || marker == 0xd9:
			return image.Config{}, fmt.Errorf("jpeg: missing frame header")
		case marker == 0x01 || marker >= 0xd0 && marker <= 0xd7:
			continue // Standalone markers do not carry a segment length.
		}
		if _, err := io.ReadFull(reader, scratch[:2]); err != nil {
			return image.Config{}, err
		}
		size := int(binary.BigEndian.Uint16(scratch[:2])) - 2
		if size < 0 {
			return image.Config{}, fmt.Errorf("jpeg: invalid segment length")
		}
		if marker == 0xc0 || marker == 0xc1 || marker == 0xc2 {
			if size != 9 && size != 15 && size != 18 {
				return image.Config{}, fmt.Errorf("jpeg: invalid frame header length")
			}
			if _, err := io.ReadFull(reader, scratch[:size]); err != nil {
				return image.Config{}, err
			}
			if scratch[0] != 8 || int(scratch[5])*3+6 != size {
				return image.Config{}, fmt.Errorf("jpeg: invalid frame header")
			}
			return image.Config{
				Width:  int(binary.BigEndian.Uint16(scratch[3:5])),
				Height: int(binary.BigEndian.Uint16(scratch[1:3])),
			}, nil
		}
		for size > 0 {
			n := min(size, len(scratch))
			if _, err := io.ReadFull(reader, scratch[:n]); err != nil {
				return image.Config{}, err
			}
			size -= n
		}
	}
}
