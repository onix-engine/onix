package html

import (
	"image"
	"math"
	"slices"
)

// The cached shadow stores one coverage byte per pixel. Tiny kernels use a
// ring of horizontal rows; broad kernels reuse the sliding box-filter storage.
func htmlBlurShadowCoverage(dst *image.Alpha, sigma float64, scratch *htmlFilterScratch) {
	if sigma <= 0 || dst.Rect.Empty() {
		return
	}
	w, h := dst.Rect.Dx(), dst.Rect.Dy()
	if sigma < 4 {
		radius := int(math.Ceil(3 * sigma))
		length := 2*radius + 1
		if scratch.sigma != sigma || len(scratch.kernel) != length {
			scratch.kernel = slices.Grow(scratch.kernel[:0], length)[:length]
			total := 0.0
			for i := range scratch.kernel {
				x := float64(i - radius)
				scratch.kernel[i] = math.Exp(-x * x / (2 * sigma * sigma))
				total += scratch.kernel[i]
			}
			for i := range scratch.kernel {
				scratch.kernel[i] /= total
			}
			scratch.sigma = sigma
		}
		rows := min(h, length)
		scratch.mask = slices.Grow(scratch.mask[:0], w*(rows+1))[:w*(rows+1)]
		output := scratch.mask[w*rows:]
		next := 0
		for y := range h {
			last := min(h-1, y+radius)
			for next <= last {
				row := scratch.mask[next%rows*w:][:w]
				for x := range w {
					value := 0.0
					for sx := max(0, x-radius); sx <= min(w-1, x+radius); sx++ {
						value += float64(dst.Pix[next*dst.Stride+sx]) / 255 * scratch.kernel[sx-x+radius]
					}
					row[x] = value
				}
				next++
			}
			clear(output)
			for sy := max(0, y-radius); sy <= last; sy++ {
				weight := scratch.kernel[sy-y+radius]
				for x, v := range scratch.mask[sy%rows*w:][:w] {
					output[x] += v * weight
				}
			}
			for x, v := range output {
				dst.Pix[y*dst.Stride+x] = uint8(math.Round(min(1, max(0, v)) * 255))
			}
		}
		return
	}
	scratch.mask = slices.Grow(scratch.mask[:0], w*h)[:w*h]
	scratch.maskLine = slices.Grow(scratch.maskLine[:0], max(w, h))[:max(w, h)]
	for y := range h {
		for x := range w {
			scratch.mask[y*w+x] = float64(dst.Pix[y*dst.Stride+x])
		}
	}
	lower := int(math.Floor(math.Sqrt(4*sigma*sigma + 1)))
	if lower%2 == 0 {
		lower--
	}
	lower = max(lower, 1)
	count := int(math.Round((12*sigma*sigma - float64(3*lower*lower+12*lower+9)) / float64(-4*lower-4)))
	for pass := range 3 {
		radius := (lower - 1) / 2
		if pass >= count {
			radius++
		}
		htmlBoxBlurMask(scratch.mask, scratch.maskLine, w, h, radius, false)
		htmlBoxBlurMask(scratch.mask, scratch.maskLine, w, h, radius, true)
	}
	for y := range h {
		for x := range w {
			dst.Pix[y*dst.Stride+x] = uint8(math.Round(min(255, max(0, scratch.mask[y*w+x]))))
		}
	}
}

// Copy one line before overwriting it. This preserves the sliding sum's
// operation order while avoiding a second full floating-point image.
func htmlBoxBlurMask(mask, lineBuffer []float64, w, h, radius int, vertical bool) {
	lines, length, step, lineStep := h, w, 1, w
	if vertical {
		lines, length, step, lineStep = w, h, w, 1
	}
	weight := 1 / float64(2*radius+1)
	for line := range lines {
		base := line * lineStep
		for index := range length {
			lineBuffer[index] = mask[base+index*step]
		}
		sum := 0.0
		for index := 0; index <= min(radius, length-1); index++ {
			sum += lineBuffer[index]
		}
		for index := range length {
			mask[base+index*step] = sum * weight
			if leaving := index - radius; leaving >= 0 {
				sum -= lineBuffer[leaving]
			}
			if entering := index + radius + 1; entering < length {
				sum += lineBuffer[entering]
			}
		}
	}
}
