package html

import (
	"image"
	"image/color"
	"math"
	"slices"
	"strconv"

	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/graphics"
	"github.com/onix-engine/onix/html/dom"
	"github.com/onix-engine/onix/html/style"
)

type htmlRasterGradient struct {
	image     string
	stops     []htmlGradientStop
	direction htmlGradientDirection
}

func htmlPaintRasterScale(world *ecs.World) float64 {
	return max(graphics.CanvasFor(world).RasterScale(), 1)
}

func htmlNeedsRasterPaint(computed *style.ComputedStyle) bool {
	if computed == nil {
		return false
	}
	if htmlHasGradientBackground(computed) {
		return true
	}
	radii := computed.BorderRadius
	if computed.AnimatedPaint&style.AnimatedBorders != 0 && (radii.TopLeft.Horizontal != style.LengthPx(0) || radii.TopLeft.Vertical != style.LengthPx(0)) {
		return true
	}
	return radii.TopLeft != radii.TopRight || radii.TopLeft != radii.BottomRight || radii.TopLeft != radii.BottomLeft
}

func htmlPaintTextureFor(state *htmlRenderState, node *dom.Node, computed *style.ComputedStyle, width, height float32) *graphics.Texture {
	if state == nil || node == nil || computed == nil || width <= 0 || height <= 0 {
		return nil
	}
	scale := max(state.Key.RasterScale, 1)
	w, h := max(1, int(math.Ceil(float64(width)*scale))), max(1, int(math.Ceil(float64(height)*scale)))
	key := htmlTextureKey{node: node, variant: "background"}
	raster := state.Textures.paint(key)
	if raster == nil {
		raster = &htmlPaintRaster{}
	}
	colors := [5]style.Color{computed.BackgroundColor.Color, computed.BorderColor.Top.Color, computed.BorderColor.Right.Color, computed.BorderColor.Bottom.Color, computed.BorderColor.Left.Color}
	if raster.style != computed || raster.colors != colors || raster.logicalWidth != width || raster.logicalHeight != height || raster.width != w || raster.height != h {
		if raster.texture != nil && (raster.width != w || raster.height != h) {
			raster.image.Pix = nil
		}
		raster.gradients = prepareHTMLRasterGradients(raster.gradients, computed)
		rasterHTMLBoxPaintPrepared(&raster.image, computed, float64(width), float64(height), scale, raster.gradients)
		raster.style, raster.width, raster.height, raster.colors = computed, w, h, colors
		raster.logicalWidth, raster.logicalHeight = width, height
		if raster.texture == nil || !raster.texture.ValidFor(nil) || raster.texture.Width != w || raster.texture.Height != h {
			raster.texture = graphics.CreateTextureBySharedImage(&raster.image, false, graphics.ResourceUsageDefault)
			if raster.texture == nil {
				return nil
			}
			raster.texture.SetLogicalSize(int(math.Ceil(float64(width))), int(math.Ceil(float64(height))))
			state.Textures.add(key, raster.texture).paint = raster
		} else {
			raster.texture.UpdateRegions([]graphics.TextureRegionUpdate{{Width: w, Height: h, Pixels: raster.image.Pix}})
		}
	}
	return raster.texture
}

func prepareHTMLRasterGradients(reuse []htmlRasterGradient, computed *style.ComputedStyle) []htmlRasterGradient {
	count := computed.BackgroundLayerCount()
	if count == 1 && computed.Background.Image == "" {
		count = 0
	}
	previous := len(reuse)
	if count < previous {
		clear(reuse[count:])
	}
	reuse = slices.Grow(reuse, max(count-previous, 0))[:count]
	for index := range reuse {
		source := computed.BackgroundLayer(index).Image
		if index < previous && reuse[index].image == source {
			continue
		}
		stops, direction, ok := parseHTMLLinearGradient(source)
		if !ok {
			stops = nil
		}
		reuse[index] = htmlRasterGradient{image: source, stops: stops, direction: direction}
	}
	return reuse
}

func rasterHTMLBoxPaintPrepared(dst *image.RGBA, computed *style.ComputedStyle, width, height, scale float64, layers []htmlRasterGradient) {
	w, h := max(1, int(math.Ceil(width*scale))), max(1, int(math.Ceil(height*scale)))
	length := w * h * 4
	if cap(dst.Pix) < length {
		dst.Pix = make([]byte, length)
	} else {
		dst.Pix = dst.Pix[:length]
		clear(dst.Pix)
	}
	dst.Stride, dst.Rect = w*4, image.Rect(0, 0, w, h)
	background := color.RGBA(computed.BackgroundColor.Color)
	radii := prepareHTMLRasterCorners(computed, width, height)
	border := prepareHTMLRasterBorder(computed, radii, width, height)
	pixelWidth, pixelHeight := width/float64(w), height/float64(h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			px := (float64(x) + 0.5) * pixelWidth
			py := (float64(y) + 0.5) * pixelHeight
			coverage := htmlRoundedRasterCoverage(radii, px, py, width, height, pixelWidth, pixelHeight)
			if coverage == 0 {
				if border.visible {
					value := htmlRGBAOver(border.colorAt(px, py, width, height), background)
					value.A = 0
					dst.SetRGBA(x, y, value)
				}
				continue
			}
			value := background
			for index := len(layers) - 1; index >= 0; index-- {
				layer := &layers[index]
				if len(layer.stops) < 2 {
					continue
				}
				progress := gradientProgress(px, py, layer.direction.X, layer.direction.Y, width, height)
				value = htmlRGBAOver(htmlGradientColorAt(layer.stops, progress), value)
			}
			value = border.paint(px, py, width, height, pixelWidth, pixelHeight, coverage, value)
			value.A = uint8(math.Round(float64(value.A) * coverage))
			// Straight-alpha textures are filtered before the shader premultiplies
			// them. Retain the border's RGB in transparent texels to avoid dark halos.
			if value.A == 0 && border.visible {
				value = border.colorAt(px, py, width, height)
				value.A = 0
			}
			dst.SetRGBA(x, y, value)
		}
	}
}

// Only pixels crossed by a corner arc need sampling. Interior and exterior
// pixels retain the fast path, and the retained paint buffer is reused.
func htmlRoundedRasterCoverage(radii htmlRasterCorners, x, y, width, height, pixelWidth, pixelHeight float64) float64 {
	halfW, halfH := pixelWidth*.5, pixelHeight*.5
	if width <= 0 || height <= 0 {
		return 0
	}
	// The inset border contour can cross a pixel along a straight edge too.
	coveredWidth := max(0, min(x+halfW, width)-max(x-halfW, 0))
	coveredHeight := max(0, min(y+halfH, height)-max(y-halfH, 0))
	rectCoverage := coveredWidth * coveredHeight / (pixelWidth * pixelHeight)
	if rectCoverage == 0 {
		return 0
	}
	for index, radius := range radii {
		if radius.x <= 0 || radius.y <= 0 {
			continue
		}
		cx, cy := radius.x, radius.y
		dx, dy := cx-x, cy-y
		if index == 1 || index == 2 {
			cx = width - radius.x
			dx = x - cx
		}
		if index == 2 || index == 3 {
			cy = height - radius.y
			dy = y - cy
		}
		if dx+halfW <= 0 || dy+halfH <= 0 {
			continue
		}
		nearX, nearY := max(0, dx-halfW)/radius.x, max(0, dy-halfH)/radius.y
		if nearX*nearX+nearY*nearY >= 1 {
			return 0
		}
		farX, farY := max(0, dx+halfW)/radius.x, max(0, dy+halfH)/radius.y
		if farX*farX+farY*farY <= 1 {
			continue
		}
		const samples = 4
		covered := 0
		for sy := range samples {
			for sx := range samples {
				px := x + (float64(sx)+.5-float64(samples)/2)*pixelWidth/float64(samples)
				py := y + (float64(sy)+.5-float64(samples)/2)*pixelHeight/float64(samples)
				if htmlRoundedRasterPixel(radii, px, py, width, height) {
					covered++
				}
			}
		}
		return float64(covered) / float64(samples*samples)
	}
	return min(rectCoverage, 1)
}

func htmlGradientColorAt(stops []htmlGradientStop, progress float64) color.RGBA {
	progress = max(0, min(1, progress))
	segment := len(stops) - 2
	for index := 0; index+1 < len(stops); index++ {
		if progress <= stops[index+1].Position {
			segment = index
			break
		}
	}
	left, right := stops[segment], stops[segment+1]
	local := (progress - left.Position) / max(right.Position-left.Position, 1e-9)
	return imageRGBAInterpolate(left.Color, right.Color, max(0, min(1, local)))
}

func htmlRGBAOver(source, destination color.RGBA) color.RGBA {
	sa, da := float64(source.A)/255, float64(destination.A)/255
	outA := sa + da*(1-sa)
	if outA <= 0 {
		return color.RGBA{}
	}
	blend := func(s, d uint8) uint8 {
		return uint8(math.Round((float64(s)*sa + float64(d)*da*(1-sa)) / outA))
	}
	return color.RGBA{R: blend(source.R, destination.R), G: blend(source.G, destination.G), B: blend(source.B, destination.B), A: uint8(math.Round(outA * 255))}
}

type htmlRasterBorder struct {
	widths      [4]float64 // top, right, bottom, left
	colors      [4]color.RGBA
	innerRadii  htmlRasterCorners
	innerWidth  float64
	innerHeight float64
	visible     bool
}

func prepareHTMLRasterBorder(computed *style.ComputedStyle, radii htmlRasterCorners, width, height float64) htmlRasterBorder {
	border := htmlRasterBorder{}
	for index, edge := range [...]struct{ width, style, color style.Value }{
		{computed.BorderWidth.Top, computed.BorderStyle.Top, computed.BorderColor.Top},
		{computed.BorderWidth.Right, computed.BorderStyle.Right, computed.BorderColor.Right},
		{computed.BorderWidth.Bottom, computed.BorderStyle.Bottom, computed.BorderColor.Bottom},
		{computed.BorderWidth.Left, computed.BorderStyle.Left, computed.BorderColor.Left},
	} {
		if edge.style.Keyword == "none" || edge.style.Keyword == "hidden" {
			continue
		}
		border.widths[index] = max(style.ResolveLength(edge.width, width), 0)
		if isHTMLSolidBorder(edge.style) {
			border.colors[index] = color.RGBA(edge.color.Color)
			border.visible = border.visible || (border.widths[index] > 0 && border.colors[index].A > 0)
		}
	}
	top, right, bottom, left := border.widths[0], border.widths[1], border.widths[2], border.widths[3]
	border.innerWidth, border.innerHeight = width-left-right, height-top-bottom
	// CSS padding-edge radii subtract each adjoining border's thickness from
	// the corresponding axis of the normalized outer radius.
	border.innerRadii = htmlRasterCorners{
		{max(radii[0].x-left, 0), max(radii[0].y-top, 0)},
		{max(radii[1].x-right, 0), max(radii[1].y-top, 0)},
		{max(radii[2].x-right, 0), max(radii[2].y-bottom, 0)},
		{max(radii[3].x-left, 0), max(radii[3].y-bottom, 0)},
	}
	return border
}

func (border *htmlRasterBorder) paint(x, y, width, height, pixelWidth, pixelHeight, outerCoverage float64, background color.RGBA) color.RGBA {
	if !border.visible {
		return background
	}
	innerCoverage := htmlRoundedRasterCoverage(border.innerRadii, x-border.widths[3], y-border.widths[0], border.innerWidth, border.innerHeight, pixelWidth, pixelHeight)
	coverage := max(outerCoverage-innerCoverage, 0)
	if coverage == 0 {
		return background
	}
	value := border.colorAt(x, y, width, height)
	// Outer coverage is applied to the composite by the caller.
	value.A = uint8(math.Round(float64(value.A) * coverage / outerCoverage))
	return htmlRGBAOver(value, background)
}

func (border *htmlRasterBorder) colorAt(x, y, width, height float64) color.RGBA {
	// Distances relative to border thickness split adjoining colors along
	// their corner join, including when one adjoining side has zero width.
	side, nearest := 0, math.Inf(1)
	for index, distance := range [...]float64{y, width - x, height - y, x} {
		if border.widths[index] > 0 {
			if distance /= border.widths[index]; distance < nearest {
				side, nearest = index, distance
			}
		}
	}
	return border.colors[side]
}

func htmlBoxShadowTextureFor(state *htmlRenderState, node *dom.Node, computed *style.ComputedStyle, shadowIndex int, width, height, boxWidth, boxHeight, padding float32) *graphics.Texture {
	texture, _ := htmlBoxShadowTextureForMode(state, node, computed, shadowIndex, width, height, boxWidth, boxHeight, padding, false)
	return texture
}

type htmlShadowKey struct {
	corners                        htmlRasterCorners
	offsetX, offsetY, blur, spread float64
	alpha                          uint8
}

type htmlShadowRaster struct {
	texture                     *graphics.Texture
	image                       image.Alpha
	key                         htmlShadowKey
	width, height               int
	logicalWidth, logicalHeight float32
	padding                     float32
	version                     uint64
}

func htmlBoxShadowTextureForMode(state *htmlRenderState, node *dom.Node, computed *style.ComputedStyle, shadowIndex int, width, height, boxWidth, boxHeight, padding float32, maskOnly bool) (*graphics.Texture, uint64) {
	if state == nil || node == nil || computed == nil || shadowIndex < 0 || shadowIndex >= len(computed.BoxShadow) {
		return nil, 0
	}
	w, h := max(1, int(math.Ceil(float64(width)))), max(1, int(math.Ceil(float64(height))))
	variant := "shadow-" + strconv.Itoa(shadowIndex)
	if maskOnly {
		variant = "shadow-mask-" + strconv.Itoa(shadowIndex)
	}
	key := htmlTextureKey{node: node, variant: variant}
	var raster *htmlShadowRaster
	if entry := state.Textures.entries[key]; entry != nil {
		raster = entry.shadow
	}
	if raster == nil {
		raster = &htmlShadowRaster{}
	}
	shadow := computed.BoxShadow[shadowIndex]
	// Retain only resolved coverage data; color belongs to the sprite.
	coverage := htmlShadowKey{
		corners: prepareHTMLRasterCorners(computed, float64(boxWidth), float64(boxHeight)),
		spread:  style.ResolveLength(shadow.SpreadRadius, min(float64(boxWidth), float64(boxHeight))),
		alpha:   shadow.Color.A,
	}
	if !maskOnly {
		coverage.offsetX = style.ResolveLength(shadow.OffsetX, float64(boxWidth))
		coverage.offsetY = style.ResolveLength(shadow.OffsetY, float64(boxHeight))
		coverage.blur = max(style.ResolveLength(shadow.BlurRadius, min(float64(boxWidth), float64(boxHeight))), 0) * .5
	}
	if raster.version == 0 || raster.key != coverage || raster.padding != padding || raster.width != w || raster.height != h || raster.logicalWidth != boxWidth || raster.logicalHeight != boxHeight {
		if raster.texture != nil && (raster.width != w || raster.height != h) {
			raster.image.Pix = nil
		}
		rasterHTMLShadowCoverage(&raster.image, coverage, float64(boxWidth), float64(boxHeight), float64(padding))
		if !maskOnly {
			scratch := acquireHTMLRasterScratch()
			htmlBlurShadowCoverage(&raster.image, coverage.blur, &scratch.htmlFilterScratch)
			releaseHTMLRasterScratch(scratch)
			for y := range h {
				for x := range w {
					if htmlRoundedRasterPixel(coverage.corners, float64(x)+.5-float64(padding)+coverage.offsetX, float64(y)+.5-float64(padding)+coverage.offsetY, float64(boxWidth), float64(boxHeight)) {
						raster.image.Pix[y*raster.image.Stride+x] = 0
					}
				}
			}
		}
		raster.width, raster.height = w, h
		raster.logicalWidth, raster.logicalHeight = boxWidth, boxHeight
		raster.key, raster.padding = coverage, padding
		raster.version++
		if raster.texture == nil || !raster.texture.ValidFor(nil) || raster.texture.Width != w || raster.texture.Height != h {
			raster.texture = graphics.CreateTextureR8(w, h, raster.image.Pix, false, graphics.ResourceUsageDefault)
			state.Textures.add(key, raster.texture).shadow = raster
		} else {
			raster.texture.UpdateRegions([]graphics.TextureRegionUpdate{{Width: w, Height: h, Pixels: raster.image.Pix}})
		}
	}
	return raster.texture, raster.version
}

func rasterHTMLShadowCoverage(dst *image.Alpha, coverage htmlShadowKey, width, height, padding float64) {
	w, h := max(1, int(math.Ceil(width+padding*2))), max(1, int(math.Ceil(height+padding*2)))
	length := w * h
	if cap(dst.Pix) < length {
		dst.Pix = make([]byte, length)
	} else {
		dst.Pix = dst.Pix[:length]
		clear(dst.Pix)
	}
	dst.Stride, dst.Rect = w, image.Rect(0, 0, w, h)
	spread := coverage.spread
	innerWidth, innerHeight := width+2*spread, height+2*spread
	expanded := coverage.corners
	for index := range expanded {
		expanded[index].x = htmlShadowSpreadRadius(expanded[index].x, spread)
		expanded[index].y = htmlShadowSpreadRadius(expanded[index].y, spread)
	}
	expanded = normalizeHTMLRasterCorners(expanded, innerWidth, innerHeight)
	for y := range h {
		for x := range w {
			if htmlRoundedRasterPixel(expanded, float64(x)+.5-padding+spread, float64(y)+.5-padding+spread, innerWidth, innerHeight) {
				dst.Pix[y*w+x] = coverage.alpha
			}
		}
	}
}

func htmlShadowSpreadRadius(radius, spread float64) float64 {
	if spread > 0 && radius < spread {
		return radius + spread*(1-math.Pow(1-radius/spread, 3))
	}
	return max(radius+spread, 0)
}
