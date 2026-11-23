// Package glyphdraw copies and composites premultiplied glyph pixels into separate RGBA surfaces.
package glyphdraw

import (
	"image"
	"image/color"
)

// clip keeps the source pixel corresponding to the first visible destination pixel.
func clip(dst, r, src image.Rectangle, sp image.Point) (image.Rectangle, image.Point) {
	offset := sp.Sub(r.Min)
	r = r.Intersect(dst).Intersect(src.Sub(offset))
	return r, r.Min.Add(offset)
}

// Copy copies rows between separate RGBA glyph surfaces.
func Copy(dst *image.RGBA, r image.Rectangle, src *image.RGBA, sp image.Point) {
	r, sp = clip(dst.Rect, r, src.Rect, sp)
	width := 4 * r.Dx()
	d := dst.PixOffset(r.Min.X, r.Min.Y)
	s := src.PixOffset(sp.X, sp.Y)
	for range r.Dy() {
		copy(dst.Pix[d:d+width], src.Pix[s:s+width])
		d += dst.Stride
		s += src.Stride
	}
}

// MaskOver paints one solid palette color through an outline glyph's coverage.
func MaskOver(dst *image.RGBA, r image.Rectangle, src color.RGBA, mask *image.Alpha) {
	r, mp := clip(dst.Rect, r, mask.Rect, image.Point{})
	d := dst.PixOffset(r.Min.X, r.Min.Y)
	m := mask.PixOffset(mp.X, mp.Y)
	for range r.Dy() {
		row := dst.Pix[d : d+4*r.Dx()]
		for x, coverage := range mask.Pix[m : m+r.Dx()] {
			if coverage == 0 {
				continue
			}
			ma := uint32(coverage) * 0x101
			rest := 0xffff - uint32(src.A)*ma/255
			p := (*[4]byte)(row[4*x:])
			p[0] = blend(p[0], src.R, rest, ma)
			p[1] = blend(p[1], src.G, rest, ma)
			p[2] = blend(p[2], src.B, rest, ma)
			p[3] = blend(p[3], src.A, rest, ma)
		}
		d += dst.Stride
		m += mask.Stride
	}
}

// ColorOver paints one color glyph with a uniform opacity.
func ColorOver(dst *image.RGBA, r image.Rectangle, src *image.RGBA, opacity uint8) {
	r, sp := clip(dst.Rect, r, src.Rect, image.Point{})
	d := dst.PixOffset(r.Min.X, r.Min.Y)
	s := src.PixOffset(sp.X, sp.Y)
	mask := uint32(opacity) * 0x101
	for range r.Dy() {
		to := dst.Pix[d : d+4*r.Dx()]
		from := src.Pix[s : s+len(to)]
		for x := 0; x < len(to); x += 4 {
			p := (*[4]byte)(from[x:])
			q := (*[4]byte)(to[x:])
			rest := 0xffff - uint32(p[3])*mask/255
			q[0] = blend(q[0], p[0], rest, mask)
			q[1] = blend(q[1], p[1], rest, mask)
			q[2] = blend(q[2], p[2], rest, mask)
			q[3] = blend(q[3], p[3], rest, mask)
		}
		d += dst.Stride
		s += src.Stride
	}
}

// Match image/draw's 16-bit source-over rounding. The 8-bit source and
// destination channels share a factor of 257, so it cancels from the divisor.
func blend(dst, src uint8, remaining, mask uint32) uint8 {
	return uint8((uint32(dst)*remaining + uint32(src)*mask) / (255 * 256))
}
