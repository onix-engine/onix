package html

import (
	"image"
	"math"
	"slices"
)

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
	scratch.maskTemp = slices.Grow(scratch.maskTemp[:0], count)[:count]
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
		htmlBoxBlurMask(scratch.maskTemp, scratch.mask, w, h, radius, false)
		htmlBoxBlurMask(scratch.mask, scratch.maskTemp, w, h, radius, true)
	}
	for y := range h {
		for x := range w {
			pixel := y*dst.Stride + x*4
			dst.Pix[pixel], dst.Pix[pixel+1], dst.Pix[pixel+2] = red, green, blue
			dst.Pix[pixel+3] = uint8(math.Round(min(255, max(0, scratch.mask[y*w+x]))))
		}
	}
}

func htmlBoxBlurMask(dst, src []float64, w, h, radius int, vertical bool) {
	lines, length, step, lineStep := h, w, 1, w
	if vertical {
		lines, length, step, lineStep = w, h, w, 1
	}
	weight := 1 / float64(2*radius+1)
	for line := range lines {
		base := line * lineStep
		sum := 0.0
		for index := 0; index <= min(radius, length-1); index++ {
			sum += src[base+index*step]
		}
		for index := range length {
			dst[base+index*step] = sum * weight
			if leaving := index - radius; leaving >= 0 {
				sum -= src[base+leaving*step]
			}
			if entering := index + radius + 1; entering < length {
				sum += src[base+entering*step]
			}
		}
	}
}
