package scene

import (
	"math"
	"sort"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

type standaloneTextScratchTag struct{}

type standaloneTextScratch struct {
	layers []renderItem2D
}

func (t *Text2D) appendRenderItems(dst []renderItem2D, entity ecs.Entity, base mgl32.Mat4, clip clipRect2D, ui bool, sortIndex int) []renderItem2D {
	if t == nil || t.cache == nil || len(t.glyphs) == 0 {
		return dst
	}
	if t.InkOffsetY != 0 {
		base = base.Mul4(mgl32.Translate3D(0, t.InkOffsetY, 0))
	}
	order := renderOrder2D(entity)
	if t.shadowEnabled() {
		dst = t.appendTextRuns(dst, order, base.Mul4(mgl32.Translate3D(t.ShadowX, t.ShadowY, 0)), t.ShadowColor, clip, ui, sortIndex, false)
	}
	if t.outlineEnabled() {
		dst = t.appendTextRuns(dst, order, base, t.OutlineColor, clip, ui, sortIndex, true)
	}
	return t.appendStyledTextRuns(dst, order, base, clip, ui, sortIndex)
}

func (t *Text2D) appendStyledTextRuns(dst []renderItem2D, order uint64, transform mgl32.Mat4, clip clipRect2D, ui bool, sortIndex int) []renderItem2D {
	spanIndex := 0
	colorAt := func(index int) graphics.ColorRGBA {
		byteIndex := int(t.glyphs[index].byteIndex)
		for spanIndex < len(t.Spans) && t.Spans[spanIndex].End <= byteIndex {
			spanIndex++
		}
		if spanIndex < len(t.Spans) && t.Spans[spanIndex].Start <= byteIndex {
			return t.Spans[spanIndex].Color
		}
		return t.Color
	}
	for start := 0; start < len(t.glyphs); {
		atlas := t.glyphs[start].raster.Atlas
		color := colorAt(start)
		end := start + 1
		for end < len(t.glyphs) && t.glyphs[end].raster.Atlas == atlas && colorAt(end) == color {
			end++
		}
		if color.A > 0 {
			texture, ok := t.cache.Atlas(atlas)
			if ok && texture != nil {
				item := t.newTextRenderItem(order, start, end, transform, color, texture, clip, ui, sortIndex, false)
				item.lcdText = true
				item.sequence = uint32(len(dst))
				dst = append(dst, item)
			}
		}
		start = end
	}
	return dst
}

func (t *Text2D) spanColor(byteIndex int) graphics.ColorRGBA {
	index := sort.Search(len(t.Spans), func(index int) bool { return t.Spans[index].End > byteIndex })
	if index < len(t.Spans) && t.Spans[index].Start <= byteIndex {
		return t.Spans[index].Color
	}
	return t.Color
}

func (t *Text2D) appendTextRuns(dst []renderItem2D, order uint64, transform mgl32.Mat4, clr graphics.ColorRGBA, clip clipRect2D, ui bool, sortIndex int, outlined bool) []renderItem2D {
	if clr.A <= 0 {
		return dst
	}
	atlasAt := func(index int) int {
		if outlined {
			return t.outlineGlyphs[index].Atlas
		}
		return t.glyphs[index].raster.Atlas
	}
	for start := 0; start < len(t.glyphs); {
		atlas := atlasAt(start)
		end := start + 1
		for end < len(t.glyphs) && atlasAt(end) == atlas {
			end++
		}
		texture, ok := t.cache.Atlas(atlas)
		if ok && texture != nil {
			item := t.newTextRenderItem(order, start, end, transform, clr, texture, clip, ui, sortIndex, outlined)
			item.sequence = uint32(len(dst))
			dst = append(dst, item)
		}
		start = end
	}
	return dst
}

func (t *Text2D) newTextRenderItem(order uint64, start, end int, transform mgl32.Mat4, clr graphics.ColorRGBA, texture *graphics.Texture, clip clipRect2D, ui bool, sortIndex int, outlined bool) renderItem2D {
	return renderItem2D{
		order:      order,
		depth:      transform[14],
		ui:         ui,
		sortIndex:  sortIndex,
		material:   textureMaterialKey(texture, nil),
		kind:       renderItemTextLayer2D,
		text:       t,
		texture:    texture,
		transform:  transform,
		color:      clr,
		glyphStart: start,
		glyphEnd:   end,
		outlined:   outlined,
		clip:       clip,
	}
}

func (item renderItem2D) drawText(c *graphics.Canvas2D) {
	width, height := c.Size()
	aligned := c.ProjectionMatrix() == mgl32.Ortho2D(0, width, height, 0) && c.ViewMatrix() == mgl32.Ident4() && item.ui && !item.outlined && item.transform[0] == 1 && item.transform[5] == 1 && item.transform[1] == 0 && item.transform[4] == 0
	paintTransform := item.transform
	lcd := c.BlendState() == render.PremultipliedAlphaBlend && aligned && item.lcdText && item.text.LCD && item.color.A > 0 && item.text.Alpha == 1
	if lcd {
		blend := render.LCDTextBlend
		blend.Constant = render.Color{R: item.color.R, G: item.color.G, B: item.color.B}
		c.SetBlendState(blend)
	}
	scale := float32(normalizeTextRasterScale(item.text.rasterScale))
	red, green, blue := uint8(math.Round(float64(item.color.R)*255)), uint8(math.Round(float64(item.color.G)*255)), uint8(math.Round(float64(item.color.B)*255))
	lcdColor, grayColor := font.LCDColorFromRGB(red, green, blue), font.GrayColorFromRGB(red, green, blue)
	for index := item.glyphStart; index < item.glyphEnd; index++ {
		glyph := &item.text.glyphs[index]
		source := glyph.raster
		if item.outlined {
			source = item.text.outlineGlyphs[index]
		}
		if !source.Visible() {
			continue
		}
		// Placement is stored in logical units while the atlas bitmap is stored at
		// the raster scale, so both offsets are shared through that divisor.
		baseOffsetX, baseOffsetY := glyph.raster.OffsetX/scale, glyph.raster.OffsetY/scale
		x := glyph.x + source.OffsetX/scale - baseOffsetX
		y := glyph.y + source.OffsetY/scale - baseOffsetY
		raster := source
		texture := item.texture
		if aligned {
			originX := (paintTransform[12] + glyph.x - baseOffsetX) * scale
			originY := (paintTransform[13] + glyph.y - baseOffsetY) * scale
			quarter := int(math.Floor(float64(originX)*4 + 0.5))
			if lcd {
				raster = item.text.cache.GlyphLCDColorAt(glyph.raster.Rune, uint8(quarter&3), lcdColor)
			} else {
				raster = item.text.cache.GlyphGrayAt(glyph.raster.Rune, uint8(quarter&3), 0, grayColor)
			}
			if atlas, ok := item.text.cache.Atlas(raster.Atlas); ok {
				texture = atlas
			}
			c.BindTexture(texture)
			x = (float32(quarter>>2)+raster.OffsetX)/scale - paintTransform[12]
			y = (float32(math.Floor(float64(originY)+0.5))+raster.OffsetY)/scale - paintTransform[13]
		}
		if lcd {
			coverageColor := graphics.White
			coverageColor.A = item.color.A
			item.text.drawGlyph(c, raster, paintTransform, x, y, texture, coverageColor, item.clip)
		} else {
			item.text.drawGlyph(c, raster, paintTransform, x, y, texture, item.color, item.clip)
		}
	}
	if aligned {
		item.text.cache.FlushUploads()
	}
	if lcd {
		c.BlendSeparate(render.PremultipliedAlphaBlend.RGB, render.PremultipliedAlphaBlend.Alpha)
	}
	if aligned {
		c.BindTexture(item.texture)
	}
}

func drawStandaloneTextLayers(c *graphics.Canvas2D, layers []renderItem2D) {
	material := renderMaterialKey{}
	materialSet := false
	for i := range layers {
		if !materialSet || material != layers[i].material {
			material = layers[i].material
			materialSet = true
			c.BindTexture(layers[i].texture)
		}
		layers[i].drawText(c)
	}
}
