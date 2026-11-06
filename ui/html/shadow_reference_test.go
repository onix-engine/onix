package html

import (
	"github.com/diiyw/onix/html/style"
	"image"
	"image/color"
	"math"
	"slices"
)

func rasterHTMLBoxShadow(dst *image.RGBA, computed *style.ComputedStyle, shadow style.Shadow, width, height, padding float64, scratch *htmlFilterScratch) {
	rasterHTMLBoxShadowMask(dst, computed, shadow, width, height, padding)
	blur := max(style.ResolveLength(shadow.BlurRadius, min(width, height)), 0)
	htmlBlurShadowMask(dst, blur*.5, scratch)
	radii := prepareHTMLRasterCorners(computed, width, height)
	offsetX, offsetY := style.ResolveLength(shadow.OffsetX, width), style.ResolveLength(shadow.OffsetY, height)
	for y := 0; y < dst.Rect.Dy(); y++ {
		for x := 0; x < dst.Rect.Dx(); x++ {
			if htmlRoundedRasterPixel(radii, float64(x)+.5-padding+offsetX, float64(y)+.5-padding+offsetY, width, height) {
				dst.Pix[dst.PixOffset(x, y)+3] = 0
			}
		}
	}
}

func rasterHTMLBoxShadowMask(dst *image.RGBA, computed *style.ComputedStyle, shadow style.Shadow, width, height, padding float64) {
	w, h := max(1, int(math.Ceil(width+padding*2))), max(1, int(math.Ceil(height+padding*2)))
	length := w * h * 4
	if cap(dst.Pix) < length {
		dst.Pix = make([]byte, length)
	} else {
		dst.Pix = dst.Pix[:length]
		clear(dst.Pix)
	}
	dst.Stride, dst.Rect = w*4, image.Rect(0, 0, w, h)
	spread := style.ResolveLength(shadow.SpreadRadius, min(width, height))
	innerWidth, innerHeight := width+2*spread, height+2*spread
	radii := prepareHTMLRasterCorners(computed, width, height)
	expanded := radii
	for index := range expanded {
		expanded[index].x = htmlShadowSpreadRadius(expanded[index].x, spread)
		expanded[index].y = htmlShadowSpreadRadius(expanded[index].y, spread)
	}
	expanded = normalizeHTMLRasterCorners(expanded, innerWidth, innerHeight)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px, py := float64(x)+.5-padding+spread, float64(y)+.5-padding+spread
			if htmlRoundedRasterPixel(expanded, px, py, innerWidth, innerHeight) {
				dst.SetRGBA(x, y, color.RGBA(shadow.Color))
			}
		}
	}
}

// htmlBlurShadowMask approximates the Gaussian with three box filters, as
// permitted by CSS Backgrounds for box-shadow. A shadow has a constant color,
// so only its alpha needs filtering. Sliding sums make the cost independent
// of the blur radius, unlike a convolution over every neighboring pixel.
func htmlBlurShadowMask(dst *image.RGBA, sigma float64, scratch *htmlFilterScratch) {
	if sigma <= 0 || dst.Rect.Empty() {
		return
	}
	// Tiny kernels are cheap and retain the exact subpixel blur profile.
	if sigma < 4 {
		htmlGaussianBlur(dst, sigma, scratch)
		return
	}
	w, h := dst.Rect.Dx(), dst.Rect.Dy()
	count := w * h
	scratch.mask = slices.Grow(scratch.mask[:0], count)[:count]
	scratch.maskLine = slices.Grow(scratch.maskLine[:0], max(w, h))[:max(w, h)]
	var red, green, blue byte
	for y := range h {
		for x := range w {
			pixel := y*dst.Stride + x*4
			alpha := dst.Pix[pixel+3]
			scratch.mask[y*w+x] = float64(alpha)
			if alpha != 0 {
				red, green, blue = dst.Pix[pixel], dst.Pix[pixel+1], dst.Pix[pixel+2]
			}
		}
	}
	// Choose odd box widths whose combined variance matches sigma squared.
	lower := int(math.Floor(math.Sqrt(4*sigma*sigma + 1)))
	if lower%2 == 0 {
		lower--
	}
	lower = max(lower, 1)
	lowerCount := int(math.Round((12*sigma*sigma - float64(3*lower*lower+12*lower+9)) / float64(-4*lower-4)))
	for pass := range 3 {
		radius := (lower - 1) / 2
		if pass >= lowerCount {
			radius++
		}
		htmlBoxBlurMask(scratch.mask, scratch.maskLine, w, h, radius, false)
		htmlBoxBlurMask(scratch.mask, scratch.maskLine, w, h, radius, true)
	}
	for y := range h {
		for x := range w {
			pixel := y*dst.Stride + x*4
			dst.Pix[pixel], dst.Pix[pixel+1], dst.Pix[pixel+2] = red, green, blue
			dst.Pix[pixel+3] = uint8(math.Round(min(255, max(0, scratch.mask[y*w+x]))))
		}
	}
}
