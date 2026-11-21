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
	lower := int(math.Floor(math.Sqrt(4*sigma*sigma + 1)))
	if lower%2 == 0 {
		lower--
	}
	lower = max(lower, 1)
	count := int(math.Round((12*sigma*sigma - float64(3*lower*lower+12*lower+9)) / float64(-4*lower-4)))
	var radii [3]int
	for pass := range radii {
		radii[pass] = (lower - 1) / 2
		if pass >= count {
			radii[pass]++
		}
	}
	// Each pass needs one row of sums and a window of horizontally blurred rows.
	// For very broad kernels a full image takes less space.
	if h >= 7 && radii[2] <= (h-7)/6 {
		rows := 2*(radii[0]+radii[1]+radii[2]) + 7
		scratch.mask = slices.Grow(scratch.mask[:0], rows*w)[:rows*w]
		streamShadowBlur(dst, radii, scratch.mask)
		return
	}
	scratch.mask = slices.Grow(scratch.mask[:0], w*h)[:w*h]
	lineSize := max(w, min(w, htmlShadowBlurColumns)*h)
	scratch.maskLine = slices.Grow(scratch.maskLine[:0], lineSize)[:lineSize]
	for y := range h {
		for x := range w {
			scratch.mask[y*w+x] = float64(dst.Pix[y*dst.Stride+x])
		}
	}
	for _, radius := range radii {
		htmlBoxBlurMask(scratch.mask, scratch.maskLine, w, h, radius, false)
		htmlBoxBlurColumns(scratch.mask, scratch.maskLine, w, h, radius)
	}
	for y := range h {
		for x := range w {
			dst.Pix[y*dst.Stride+x] = uint8(math.Round(min(255, max(0, scratch.mask[y*w+x]))))
		}
	}
}

// A pass consumes one input row at a time. Its output is delayed by radius
// rows so the running vertical sum includes both sides of the output pixel.
type shadowBoxPass struct {
	window []float64
	sums   []float64
	radius int
	weight float64
}

func streamShadowBlur(dst *image.Alpha, radii [3]int, storage []float64) {
	w, h := dst.Rect.Dx(), dst.Rect.Dy()
	row, storage := storage[:w], storage[w:]
	var passes [3]shadowBoxPass
	for i, radius := range radii {
		size := (2*radius + 1) * w
		passes[i] = shadowBoxPass{window: storage[:size], sums: storage[size : size+w], radius: radius, weight: 1 / float64(2*radius+1)}
		clear(passes[i].sums)
		storage = storage[size+w:]
	}
	for input := 0; input < h+radii[0]+radii[1]+radii[2]; input++ {
		if input < h {
			for x, alpha := range dst.Pix[input*dst.Stride:][:w] {
				row[x] = float64(alpha)
			}
		}
		y := input
		for i := range passes {
			y = passes[i].push(row, y, h)
			if y < 0 {
				break
			}
		}
		if y >= 0 && y < h {
			for x, v := range row {
				dst.Pix[y*dst.Stride+x] = uint8(math.Round(min(255, max(0, v))))
			}
		}
	}
}

// Replace row with this pass's output. Negative output rows fill the window;
// input rows beyond the bottom advance the remaining sums with zero padding.
func (pass *shadowBoxPass) push(row []float64, y, height int) int {
	width, diameter := len(row), 2*pass.radius+1
	if y < height {
		target := pass.window[(y%diameter)*width:][:width]
		sum := 0.0
		for _, v := range row[:min(pass.radius+1, width)] {
			sum += v
		}
		for x := range row {
			target[x] = sum * pass.weight
			pass.sums[x] += target[x]
			if x >= pass.radius {
				sum -= row[x-pass.radius]
			}
			if next := x + pass.radius + 1; next < width {
				sum += row[next]
			}
		}
	}
	output := y - pass.radius
	if output >= 0 && output < height {
		for x, sum := range pass.sums {
			row[x] = sum * pass.weight
		}
		if leaving := output - pass.radius; leaving >= 0 {
			for x, v := range pass.window[(leaving%diameter)*width:][:width] {
				pass.sums[x] -= v
			}
		}
	}
	return output
}

const htmlShadowBlurColumns = 16

// Process adjacent columns together so each pass reads and writes contiguous
// pixels. Preserve the addition and subtraction order within each column.
func htmlBoxBlurColumns(mask, buffer []float64, width, height, radius int) {
	weight := 1 / float64(2*radius+1)
	for left := 0; left < width; left += htmlShadowBlurColumns {
		columns := min(htmlShadowBlurColumns, width-left)
		var sums [htmlShadowBlurColumns]float64
		for y := range height {
			copy(buffer[y*columns:][:columns], mask[y*width+left:][:columns])
		}
		for y := 0; y <= min(radius, height-1); y++ {
			for x, value := range buffer[y*columns:][:columns] {
				sums[x] += value
			}
		}
		for y := range height {
			row := mask[y*width+left:][:columns]
			for x := range row {
				row[x] = sums[x] * weight
			}
			if leaving := y - radius; leaving >= 0 {
				for x, value := range buffer[leaving*columns:][:columns] {
					sums[x] -= value
				}
			}
			if entering := y + radius + 1; entering < height {
				for x, value := range buffer[entering*columns:][:columns] {
					sums[x] += value
				}
			}
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
