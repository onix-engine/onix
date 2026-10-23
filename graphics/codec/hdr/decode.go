// Package hdr decodes Radiance RGBE panoramas without reducing their dynamic
// range. It is separate from the ordinary, eight-bit scene image decoders.
package hdr

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/diiyw/onix/graphics"
	"github.com/go-gl/mathgl/mgl32"
)

// Panorama owns top-left, row-major RGBE pixels (four bytes per pixel).
// Sampling returns linear RGB; stored exposure/color corrections are already
// applied to the pixels and are not applied again. RGB primaries are preserved.
type Panorama struct {
	width, height int
	pixels        []byte
}

func (p *Panorama) Size() (int, int) { return p.width, p.height }

// At returns the unclipped linear value, or black outside the image.
func (p *Panorama) At(x, y int) mgl32.Vec3 {
	if x < 0 || y < 0 || x >= p.width || y >= p.height {
		return mgl32.Vec3{}
	}
	c := p.pixels[(y*p.width+x)*4:][:4]
	if c[3] == 0 {
		return mgl32.Vec3{}
	}
	f := float32(math.Ldexp(1, int(c[3])-136))
	return mgl32.Vec3{(float32(c[0]) + .5) * f, (float32(c[1]) + .5) * f, (float32(c[2]) + .5) * f}
}

// Radiance bilinearly samples a unit direction in the engine's equirectangular
// convention. Longitude wraps, latitude clamps, and sampling allocates nothing.
func (p *Panorama) Radiance(n mgl32.Vec3) mgl32.Vec3 {
	return graphics.SamplePanorama(p, n)
}

// Decode accepts flat RGBE and component-RLE scanlines in all eight Radiance
// orientations. XYZE and obsolete pixel-run encoding are not supported.
// Image storage is bounded to 64 million pixels; the input is not retained.
func Decode(data []byte) (*Panorama, error) {
	line, data, ok := takeLine(data)
	if !ok || string(line) != "#?RADIANCE" && string(line) != "#?RGBE" {
		return nil, fmt.Errorf("hdr: missing Radiance signature")
	}
	format := false
	for {
		line, data, ok = takeLine(data)
		if !ok {
			return nil, fmt.Errorf("hdr: truncated header")
		}
		if len(line) == 0 {
			break
		}
		if value, found := strings.CutPrefix(string(line), "FORMAT="); found {
			if format || strings.TrimSpace(value) != "32-bit_rle_rgbe" {
				return nil, fmt.Errorf("hdr: expected one FORMAT=32-bit_rle_rgbe")
			}
			format = true
		}
	}
	if !format {
		return nil, fmt.Errorf("hdr: missing RGBE format")
	}
	line, data, ok = takeLine(data)
	fields := strings.Fields(string(line))
	if !ok || len(fields) != 4 || !validAxis(fields[0]) || !validAxis(fields[2]) || fields[0][1] == fields[2][1] {
		return nil, fmt.Errorf("hdr: invalid resolution")
	}
	rows, errRows := strconv.Atoi(fields[1])
	columns, errColumns := strconv.Atoi(fields[3])
	if errRows != nil || errColumns != nil || rows <= 0 || columns <= 0 || rows > 16384 || columns > 16384 || rows > (1<<26)/columns {
		return nil, fmt.Errorf("hdr: invalid or excessive dimensions")
	}
	// Check a lower bound on encoded storage before allocating the image.
	minimum := rows * columns * 4
	if columns >= 8 && columns <= 32767 {
		minimum = rows * (4 + 8*((columns+126)/127))
	}
	if len(data) < minimum {
		return nil, fmt.Errorf("hdr: truncated pixel data")
	}
	p := &Panorama{width: columns, height: rows, pixels: make([]byte, rows*columns*4)}
	if fields[0][1] == 'X' {
		p.width, p.height = rows, columns
	}
	scanline := make([]byte, columns*4)
	for row := range rows {
		var err error
		data, err = decodeScanline(data, scanline)
		if err != nil {
			return nil, fmt.Errorf("hdr: scanline %d: %w", row, err)
		}
		for column := range columns {
			a, b := axisPosition(fields[0], row, rows), axisPosition(fields[2], column, columns)
			x, y := b, a
			if fields[0][1] == 'X' {
				x, y = a, b
			}
			copy(p.pixels[(y*p.width+x)*4:][:4], scanline[column*4:][:4])
		}
	}
	return p, nil
}

func takeLine(data []byte) ([]byte, []byte, bool) {
	i := bytes.IndexByte(data, '\n')
	if i < 0 {
		return nil, data, false
	}
	return bytes.TrimSuffix(data[:i], []byte{'\r'}), data[i+1:], true
}

func validAxis(axis string) bool {
	return len(axis) == 2 && (axis[0] == '+' || axis[0] == '-') && (axis[1] == 'X' || axis[1] == 'Y')
}

func axisPosition(axis string, index, count int) int {
	if axis == "-X" || axis == "+Y" {
		return count - 1 - index
	}
	return index
}

func decodeScanline(data, dst []byte) ([]byte, error) {
	width := len(dst) / 4
	if len(data) < 4 {
		return nil, fmt.Errorf("truncated pixels")
	}
	if width < 8 || width > 32767 || data[0] != 2 || data[1] != 2 || data[2]&128 != 0 {
		if len(data) < len(dst) {
			return nil, fmt.Errorf("truncated flat pixels")
		}
		for i := 0; i < len(dst); i += 4 {
			if data[i] == 1 && data[i+1] == 1 && data[i+2] == 1 {
				return nil, fmt.Errorf("obsolete pixel-run encoding is unsupported")
			}
		}
		copy(dst, data)
		return data[len(dst):], nil
	}
	if int(data[2])<<8|int(data[3]) != width {
		return nil, fmt.Errorf("RLE width mismatch")
	}
	data = data[4:]
	for channel := range 4 {
		for x := 0; x < width; {
			if len(data) < 2 {
				return nil, fmt.Errorf("truncated RLE packet")
			}
			code := int(data[0])
			data = data[1:]
			count := code
			if code > 128 {
				count -= 128
			}
			if count == 0 || count > width-x {
				return nil, fmt.Errorf("invalid RLE packet length")
			}
			if code > 128 {
				for i := range count {
					dst[(x+i)*4+channel] = data[0]
				}
				data = data[1:]
			} else {
				if len(data) < count {
					return nil, fmt.Errorf("truncated RLE literal")
				}
				for i, value := range data[:count] {
					dst[(x+i)*4+channel] = value
				}
				data = data[count:]
			}
			x += count
		}
	}
	return data, nil
}
