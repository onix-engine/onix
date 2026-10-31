package html

import (
	"image"
	"image/color"
	"math"
	"slices"

	"github.com/diiyw/onix/html/style"
)

type htmlFilterScratch struct {
	rows     [][4]float64
	kernel   []float64
	sigma    float64
	matrices [][20]float64
	shadow   image.RGBA
	mask     []float64
	maskLine []float64
}

func htmlFilterBounds(filters []style.Filter, minX, minY, maxX, maxY float64) (float64, float64, float64, float64) {
	for _, filter := range filters {
		switch filter.Kind {
		case style.FilterBlur:
			pad := math.Ceil(max(filter.Amount, 0) * 3)
			minX, minY, maxX, maxY = minX-pad, minY-pad, maxX+pad, maxY+pad
		case style.FilterDropShadow:
			pad := math.Ceil(max(style.ResolveLength(filter.Shadow.BlurRadius, 0), 0) * 3)
			x, y := style.ResolveLength(filter.Shadow.OffsetX, 0), style.ResolveLength(filter.Shadow.OffsetY, 0)
			minX, minY, maxX, maxY = min(minX, minX+x-pad), min(minY, minY+y-pad), max(maxX, maxX+x+pad), max(maxY, maxY+y+pad)
		}
	}
	return minX, minY, maxX, maxY
}

func applyHTMLFilters(dst *image.RGBA, filters []style.Filter, scratch *htmlFilterScratch) {
	for index := 0; index < len(filters); {
		filter := filters[index]
		switch filter.Kind {
		case style.FilterBlur:
			htmlGaussianBlur(dst, filter.Amount, scratch)
			index++
		case style.FilterDropShadow:
			htmlRasterDropShadow(dst, filter.Shadow, scratch)
			index++
		default:
			// Traverse pixels once for each run of color functions, preserving
			// the clamp after every function required by the filter pipeline.
			scratch.matrices = scratch.matrices[:0]
			for index < len(filters) && filters[index].Kind != style.FilterBlur && filters[index].Kind != style.FilterDropShadow {
				scratch.matrices = append(scratch.matrices, htmlFilterMatrix(filters[index]))
				index++
			}
			for y := dst.Rect.Min.Y; y < dst.Rect.Max.Y; y++ {
				row := dst.Pix[dst.PixOffset(dst.Rect.Min.X, y):][:dst.Rect.Dx()*4]
				for offset := 0; offset < len(row); offset += 4 {
					value := [4]float64{float64(row[offset]) / 255, float64(row[offset+1]) / 255, float64(row[offset+2]) / 255, float64(row[offset+3]) / 255}
					for _, matrix := range scratch.matrices {
						var out [4]float64
						for channel := range out {
							m := matrix[channel*5 : channel*5+5]
							out[channel] = max(0, min(1, m[0]*value[0]+m[1]*value[1]+m[2]*value[2]+m[3]*value[3]+m[4]))
						}
						value = out
					}
					for channel := range value {
						row[offset+channel] = uint8(math.Round(value[channel] * 255))
					}
				}
			}
		}
	}
}

func htmlFilterMatrix(filter style.Filter) [20]float64 {
	m := [20]float64{1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 1, 0}
	a := max(filter.Amount, 0)
	switch filter.Kind {
	case style.FilterColorMatrix:
		return filter.Matrix
	case style.FilterBrightness:
		m[0], m[6], m[12] = a, a, a
	case style.FilterContrast:
		m[0], m[6], m[12] = a, a, a
		m[4], m[9], m[14] = (1-a)/2, (1-a)/2, (1-a)/2
	case style.FilterOpacity:
		m[18] = min(a, 1)
	case style.FilterInvert:
		a = min(a, 1)
		m[0], m[6], m[12] = 1-2*a, 1-2*a, 1-2*a
		m[4], m[9], m[14] = a, a, a
	case style.FilterGrayscale:
		a = 1 - min(a, 1)
		fallthrough
	case style.FilterSaturate:
		m[0], m[1], m[2] = .213+.787*a, .715-.715*a, .072-.072*a
		m[5], m[6], m[7] = .213-.213*a, .715+.285*a, .072-.072*a
		m[10], m[11], m[12] = .213-.213*a, .715-.715*a, .072+.928*a
	case style.FilterSepia:
		a = min(a, 1)
		m[0], m[1], m[2] = 1-.607*a, .769*a, .189*a
		m[5], m[6], m[7] = .349*a, 1-.314*a, .168*a
		m[10], m[11], m[12] = .272*a, .534*a, 1-.869*a
	case style.FilterHueRotate:
		s, c := math.Sincos(filter.Amount * math.Pi / 180)
		m[0], m[1], m[2] = .213+.787*c-.213*s, .715-.715*c-.715*s, .072-.072*c+.928*s
		m[5], m[6], m[7] = .213-.213*c+.143*s, .715+.285*c+.140*s, .072-.072*c-.283*s
		m[10], m[11], m[12] = .213-.213*c-.787*s, .715-.715*c+.715*s, .072+.928*c+.072*s
	}
	return m
}

func htmlGaussianBlur(dst *image.RGBA, sigma float64, scratch *htmlFilterScratch) {
	if sigma <= 0 || dst.Rect.Empty() {
		return
	}
	radius := int(math.Ceil(3 * sigma))
	length := 2*radius + 1
	if scratch.sigma != sigma || len(scratch.kernel) != length {
		scratch.kernel = slices.Grow(scratch.kernel[:0], length)[:length]
		total := 0.0
		for index := range scratch.kernel {
			x := float64(index - radius)
			scratch.kernel[index] = math.Exp(-x * x / (2 * sigma * sigma))
			total += scratch.kernel[index]
		}
		for index := range scratch.kernel {
			scratch.kernel[index] /= total
		}
		scratch.sigma = sigma
	}
	w, h := dst.Rect.Dx(), dst.Rect.Dy()
	rowCount := min(h, length)
	scratch.rows = slices.Grow(scratch.rows[:0], w*(rowCount+1))[:w*(rowCount+1)]
	output := scratch.rows[w*rowCount:]
	// Convolve premultiplied color to keep transparent texels from creating
	// dark fringes. Keep only the horizontal rows needed by the vertical
	// kernel, reading each source row before the in-place output reaches it.
	nextRow := 0
	for y := range h {
		lastRow := min(h-1, y+radius)
		for nextRow <= lastRow {
			row := scratch.rows[nextRow%rowCount*w:][:w]
			for x := range w {
				var value [4]float64
				for sx := max(0, x-radius); sx <= min(w-1, x+radius); sx++ {
					pixel := dst.PixOffset(sx+dst.Rect.Min.X, nextRow+dst.Rect.Min.Y)
					alpha := float64(dst.Pix[pixel+3]) / 255
					weight := scratch.kernel[sx-x+radius]
					for channel := 0; channel < 3; channel++ {
						value[channel] += float64(dst.Pix[pixel+channel]) * alpha * weight
					}
					value[3] += alpha * weight
				}
				row[x] = value
			}
			nextRow++
		}
		firstRow := max(0, y-radius)
		clear(output)
		// Accumulate contiguous rows so both the input and output stay local,
		// preserving each pixel's original order of floating-point operations.
		for sy := firstRow; sy <= lastRow; sy++ {
			weight := scratch.kernel[sy-y+radius]
			row := scratch.rows[sy%rowCount*w:][:w]
			for x, value := range row {
				pixel := &output[x]
				pixel[0] += value[0] * weight
				pixel[1] += value[1] * weight
				pixel[2] += value[2] * weight
				pixel[3] += value[3] * weight
			}
		}
		for x, value := range output {
			pixel := dst.PixOffset(x+dst.Rect.Min.X, y+dst.Rect.Min.Y)
			if value[3] > 0 {
				for channel := 0; channel < 3; channel++ {
					dst.Pix[pixel+channel] = uint8(math.Round(min(255, value[channel]/value[3])))
				}
			} else {
				clear(dst.Pix[pixel : pixel+3])
			}
			dst.Pix[pixel+3] = uint8(math.Round(value[3] * 255))
		}
	}
}

func htmlRasterDropShadow(dst *image.RGBA, shadow style.Shadow, scratch *htmlFilterScratch) {
	length := len(dst.Pix)
	scratch.shadow.Pix = slices.Grow(scratch.shadow.Pix[:0], length)[:length]
	scratch.shadow.Stride, scratch.shadow.Rect = dst.Stride, dst.Rect
	for offset := 0; offset < length; offset += 4 {
		scratch.shadow.Pix[offset], scratch.shadow.Pix[offset+1], scratch.shadow.Pix[offset+2] = shadow.Color.R, shadow.Color.G, shadow.Color.B
		scratch.shadow.Pix[offset+3] = uint8(math.Round(float64(dst.Pix[offset+3]) * float64(shadow.Color.A) / 255))
	}
	htmlGaussianBlur(&scratch.shadow, style.ResolveLength(shadow.BlurRadius, 0), scratch)
	x, y := style.ResolveLength(shadow.OffsetX, 0), style.ResolveLength(shadow.OffsetY, 0)
	for dy := dst.Rect.Min.Y; dy < dst.Rect.Max.Y; dy++ {
		for dx := dst.Rect.Min.X; dx < dst.Rect.Max.X; dx++ {
			// Sample the shifted mask so fractional CSS offsets remain visible.
			sx, sy := float64(dx)-x, float64(dy)-y
			left, top := int(math.Floor(sx)), int(math.Floor(sy))
			fx, fy := sx-float64(left), sy-float64(top)
			alpha := 0.0
			for yy := 0; yy < 2; yy++ {
				for xx := 0; xx < 2; xx++ {
					point := image.Pt(left+xx, top+yy)
					if !point.In(dst.Rect) {
						continue
					}
					wx, wy := 1-fx, 1-fy
					if xx == 1 {
						wx = fx
					}
					if yy == 1 {
						wy = fy
					}
					alpha += float64(scratch.shadow.Pix[scratch.shadow.PixOffset(point.X, point.Y)+3]) * wx * wy
				}
			}
			pixel := dst.PixOffset(dx, dy)
			// The shifted mask is independent of dst. Each source pixel is
			// consumed once, so compositing needs no full source snapshot.
			source := color.RGBA{R: dst.Pix[pixel], G: dst.Pix[pixel+1], B: dst.Pix[pixel+2], A: dst.Pix[pixel+3]}
			background := color.RGBA{R: shadow.Color.R, G: shadow.Color.G, B: shadow.Color.B, A: uint8(math.Round(alpha))}
			dst.SetRGBA(dx, dy, htmlRGBAOver(source, background))
		}
	}
}
