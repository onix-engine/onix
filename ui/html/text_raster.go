package html

import (
	"image"
	"image/color"
	"math"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/internal/layout"
	"github.com/diiyw/onix/internal/style"
)

func htmlVisibilityHash(computed *style.ComputedStyle) uint64 {
	if htmlStyleVisible(computed) {
		return 1
	}
	return 0
}

func htmlTextRasterHash(box *layout.Box) uint64 {
	hash := uint64(14695981039346656037)
	for _, fragment := range box.TextFragments {
		computed := fragment.Style
		if computed == nil {
			continue
		}
		c := computed.Color.Color
		for _, value := range [...]uint64{htmlVisibilityHash(computed), uint64(c.R)<<24 | uint64(c.G)<<16 | uint64(c.B)<<8 | uint64(c.A), math.Float64bits(computed.FontSize.LengthPx), math.Float64bits(computed.FontWeight.Number)} {
			hash = (hash ^ value) * 1099511628211
		}
		for index := range len(computed.TextDecorationLine.Keyword) {
			hash = (hash ^ uint64(computed.TextDecorationLine.Keyword[index])) * 1099511628211
		}
	}
	return hash
}

func rasterHTMLText(dst *image.RGBA, box *layout.Box, regular, bold *font.Face, originX, originY float64) {
	for _, fragment := range box.TextFragments {
		computed := fragment.Style
		if !htmlStyleVisible(computed) || computed.Color.Color.A == 0 {
			continue
		}
		face := regular
		if bold != nil && computed.FontWeight.Number >= 600 {
			face = bold
		}
		if face == nil || face.Head.UnitsPerEm == 0 {
			continue
		}
		size := float64(fragmentFontSize(computed))
		c := color.RGBA(computed.Color.Color)
		for _, glyph := range fragment.Glyphs {
			glyphFace, id := face.ResolveGlyph(glyph.Rune)
			scale := size / float64(glyphFace.Head.UnitsPerEm)
			metrics, err := glyphFace.HorizontalMetrics(id)
			if err != nil {
				continue
			}
			// Layout's X includes the side bearing; the glyph bitmap is relative
			// to the pen, with its own outline bounds and flipped font Y axis.
			x := glyph.X - float64(metrics.LeftSideBearing)*scale - originX
			y := glyph.Y - originY
			ix, iy := math.Floor(x), math.Floor(y)
			bitmap, err := glyphFace.RasterizeGlyphAt(id, size, uint8((x-ix)*4), uint8((y-iy)*4))
			if err != nil || bitmap == nil || bitmap.Image == nil {
				continue
			}
			left, top := int(ix)+bitmap.Bounds.Min.X, int(iy)-bitmap.Bounds.Max.Y
			for sy := range bitmap.Image.Rect.Dy() {
				dy := top + sy
				if dy < dst.Rect.Min.Y || dy >= dst.Rect.Max.Y {
					continue
				}
				row := bitmap.Image.Pix[sy*bitmap.Image.Stride:][:bitmap.Image.Rect.Dx()]
				for sx, coverage := range row {
					dx := left + sx
					if coverage == 0 || dx < dst.Rect.Min.X || dx >= dst.Rect.Max.X {
						continue
					}
					value := c
					value.A = uint8(math.Round(float64(c.A) * float64(coverage) / 255))
					dst.SetRGBA(dx, dy, htmlRGBAOver(value, dst.RGBAAt(dx, dy)))
				}
			}
		}
		if computed.TextDecorationLine == style.Keyword("line-through") {
			thickness := min(max(size/14, 1), fragment.Rect.Height)
			top := fragment.Rect.Y + (fragment.Rect.Height-thickness)/2 - originY
			r := image.Rect(int(math.Floor(fragment.Rect.X-originX)), int(math.Floor(top)), int(math.Ceil(fragment.Rect.X+fragment.Rect.Width-originX)), int(math.Ceil(top+thickness))).Intersect(dst.Rect)
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					dst.SetRGBA(x, y, htmlRGBAOver(c, dst.RGBAAt(x, y)))
				}
			}
		}
	}
}
