package font

import (
	"cmp"
	"image"
	"math"
	"slices"
)

type GlyphBitmap struct {
	Image   *image.Alpha
	Bounds  image.Rectangle
	Advance float64
	Scale   float64
}

type glyphBitmapCacheKey struct {
	glyphID        uint16
	sizeBits       uint64
	phaseX, phaseY uint8
}

// These CPU masks can be regenerated and are separate from live atlas glyphs.
// Bound bytes and entries, including zero-area glyphs and old sizes/phases.
const (
	glyphBitmapCacheByteLimit  = 4 << 20
	glyphBitmapCacheEntryLimit = 1024
)

// cacheBitmap runs under cacheMu. Eviction only drops the cache's reference;
// masks already returned to callers remain valid and are never overwritten.
func (f *Face) cacheBitmap(key glyphBitmapCacheKey, bitmap *GlyphBitmap) *GlyphBitmap {
	if cached, ok := f.glyphBitmapCache[key]; ok {
		return cached
	}
	size := len(bitmap.Image.Pix)
	if size > glyphBitmapCacheByteLimit {
		return bitmap
	}
	if f.bitmapOrder == nil {
		f.bitmapOrder = make([]glyphBitmapCacheKey, glyphBitmapCacheEntryLimit)
	}
	for len(f.glyphBitmapCache) >= glyphBitmapCacheEntryLimit || f.bitmapBytes+size > glyphBitmapCacheByteLimit {
		old := f.bitmapOrder[f.bitmapHead]
		f.bitmapBytes -= len(f.glyphBitmapCache[old].Image.Pix)
		delete(f.glyphBitmapCache, old)
		f.bitmapOrder[f.bitmapHead] = glyphBitmapCacheKey{}
		f.bitmapHead = (f.bitmapHead + 1) % len(f.bitmapOrder)
	}
	index := (f.bitmapHead + len(f.glyphBitmapCache)) % len(f.bitmapOrder)
	f.bitmapOrder[index] = key
	f.glyphBitmapCache[key] = bitmap
	f.bitmapBytes += size
	return bitmap
}

func (f *Face) RasterizeGlyph(glyphID uint16, pixelsPerEm float64) (*GlyphBitmap, error) {
	return f.RasterizeGlyphAt(glyphID, pixelsPerEm, 0, 0)
}

// RasterizeGlyphAt samples a glyph at a quarter-pixel device-space origin.
// The sixteen possible phases bound cache growth independently of positions.
func (f *Face) RasterizeGlyphAt(glyphID uint16, pixelsPerEm float64, phaseX, phaseY uint8) (*GlyphBitmap, error) {
	if f == nil {
		return nil, nil
	}
	if pixelsPerEm <= 0 {
		pixelsPerEm = 16
	}
	phaseX, phaseY = phaseX&3, phaseY&3
	shiftX, shiftY := float64(phaseX)/4, float64(phaseY)/4
	key := glyphBitmapCacheKey{glyphID: glyphID, sizeBits: math.Float64bits(pixelsPerEm), phaseX: phaseX, phaseY: phaseY}
	f.cacheMu.RLock()
	if bitmap, ok := f.glyphBitmapCache[key]; ok {
		f.cacheMu.RUnlock()
		return bitmap, nil
	}
	f.cacheMu.RUnlock()

	outline, err := f.Outline(glyphID)
	if err != nil {
		return nil, err
	}
	metrics, err := f.HorizontalMetrics(glyphID)
	if err != nil {
		return nil, err
	}
	if f.Head.UnitsPerEm == 0 {
		return nil, nil
	}
	scale := pixelsPerEm / float64(f.Head.UnitsPerEm)
	bounds := image.Rect(
		int(math.Floor(float64(outline.Bounds.XMin)*scale+shiftX)),
		int(math.Floor(float64(outline.Bounds.YMin)*scale-shiftY)),
		int(math.Ceil(float64(outline.Bounds.XMax)*scale+shiftX)),
		int(math.Ceil(float64(outline.Bounds.YMax)*scale-shiftY)),
	)
	if bounds.Empty() {
		bitmap := &GlyphBitmap{Image: image.NewAlpha(image.Rect(0, 0, 0, 0)), Bounds: bounds, Advance: float64(metrics.AdvanceWidth) * scale, Scale: scale}
		f.cacheMu.Lock()
		f.ensureCachesLocked()
		bitmap = f.cacheBitmap(key, bitmap)
		f.cacheMu.Unlock()
		return bitmap, nil
	}

	img := image.NewAlpha(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	scaledContours := scaleOutlineAt(outline.Contours, scale, bounds, shiftX, shiftY)
	paintCoverage(img, scaledContours)

	bitmap := &GlyphBitmap{Image: img, Bounds: bounds, Advance: float64(metrics.AdvanceWidth) * scale, Scale: scale}
	f.cacheMu.Lock()
	f.ensureCachesLocked()
	bitmap = f.cacheBitmap(key, bitmap)
	f.cacheMu.Unlock()
	return bitmap, nil
}

func scaleOutline(contours []OutlineContour, scale float64, bounds image.Rectangle) []OutlineContour {
	return scaleOutlineAt(contours, scale, bounds, 0, 0)
}
func scaleOutlineAt(contours []OutlineContour, scale float64, bounds image.Rectangle, shiftX, shiftY float64) []OutlineContour {
	scaled := make([]OutlineContour, 0, len(contours))
	for _, contour := range contours {
		mapped := make(OutlineContour, len(contour))
		for i, point := range contour {
			mapped[i] = OutlinePoint{
				X: point.X*scale + shiftX - float64(bounds.Min.X),
				Y: float64(bounds.Max.Y) - point.Y*scale + shiftY,
			}
		}
		scaled = append(scaled, mapped)
	}
	return scaled
}

func paintCoverage(img *image.Alpha, contours []OutlineContour) {
	if img == nil || img.Bounds().Empty() {
		return
	}
	// Integrate horizontal span coverage instead of counting four x samples.
	// Sixteen vertical scanlines bound the remaining curve-integration error
	// while retaining all 256 alpha values even for thin vertical stems.
	const scanlines = 16
	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	var local [256]coverageIntersection
	intersections := local[:0]
	var localCoverage [256]float64
	coverage := localCoverage[:]
	if width > len(coverage) {
		coverage = make([]float64, width)
	}
	coverage = coverage[:width]
	for y := range height {
		clear(coverage)
		for sample := range scanlines {
			var winding int
			intersections, winding = coverageIntersections(intersections[:0], float64(y)+(float64(sample)+0.5)/scanlines, contours)
			slices.SortFunc(intersections, func(a, b coverageIntersection) int { return cmp.Compare(a.x, b.x) })
			previous := 0.0
			for _, crossing := range intersections {
				end := max(0, min(float64(width), crossing.x))
				if winding != 0 && end > previous {
					for x := int(previous); x < min(width, int(math.Ceil(end))); x++ {
						coverage[x] += min(end, float64(x+1)) - max(previous, float64(x))
					}
				}
				winding -= crossing.winding
				previous = end
			}
		}
		row := img.Pix[y*img.Stride : y*img.Stride+width]
		for x, value := range coverage {
			row[x] = uint8(math.Round(min(1, value/scanlines) * 255))
		}
	}

}

type coverageIntersection struct {
	x       float64
	winding int
}

func coverageIntersections(dst []coverageIntersection, y float64, contours []OutlineContour) ([]coverageIntersection, int) {
	winding := 0
	for _, contour := range contours {
		for i, j := 0, len(contour)-1; i < len(contour); j, i = i, i+1 {
			p0, p1 := contour[i], contour[j]
			delta := 0
			switch {
			case p0.Y <= y && p1.Y > y:
				delta = 1
			case p0.Y > y && p1.Y <= y:
				delta = -1
			default:
				continue
			}
			x := p0.X + (y-p0.Y)*(p1.X-p0.X)/(p1.Y-p0.Y)
			dst = append(dst, coverageIntersection{x: x, winding: delta})
			winding += delta
		}
	}
	return dst, winding
}

func pointInContours(x, y float64, contours []OutlineContour) bool {
	winding := 0
	for _, contour := range contours {
		winding += contourWindingNumber(x, y, contour)
	}
	return winding != 0
}

func contourWindingNumber(x, y float64, contour OutlineContour) int {
	winding := 0
	for i, j := 0, len(contour)-1; i < len(contour); j, i = i, i+1 {
		pi := contour[i]
		pj := contour[j]
		if pi.Y <= y {
			if pj.Y > y && isLeft(pi, pj, x, y) > 0 {
				winding++
			}
			continue
		}
		if pj.Y <= y && isLeft(pi, pj, x, y) < 0 {
			winding--
		}
	}
	return winding
}

func isLeft(p0, p1 OutlinePoint, x, y float64) float64 {
	return (p1.X-p0.X)*(y-p0.Y) - (x-p0.X)*(p1.Y-p0.Y)
}
