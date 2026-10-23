package layout

import "math"

func (b *Box) GeometryHash() uint64 {
	h := newGeometryHash()
	writeBoxGeometryHash(&h, b)
	return h.sum64()
}

func (b *Box) OwnGeometryHash() uint64 {
	h := newGeometryHash()
	writeBoxOwnGeometryHash(&h, b)
	return h.sum64()
}

type geometryHash struct {
	sum uint64
}

func newGeometryHash() geometryHash {
	return geometryHash{sum: 14695981039346656037}
}

func (h *geometryHash) sum64() uint64 {
	return h.sum
}

func (h *geometryHash) writeByte(value byte) {
	h.sum ^= uint64(value)
	h.sum *= 1099511628211
}

func (h *geometryHash) writeString(value string) {
	for i := 0; i < len(value); i++ {
		h.writeByte(value[i])
	}
}

func writeBoxGeometryHash(h *geometryHash, box *Box) {
	writeBoxOwnGeometryHash(h, box)
	if box == nil {
		return
	}
	writeUintHash(h, uint64(len(box.Children)))
	for _, child := range box.Children {
		writeBoxGeometryHash(h, child)
	}
}

func writeBoxOwnGeometryHash(h *geometryHash, box *Box) {
	if box == nil {
		writeStringHash(h, "<nil>")
		return
	}
	writeStringHash(h, box.StableID)
	writeRectHash(h, box.Viewport)
	writeRectHash(h, box.Content)
	writeEdgesHash(h, box.Padding)
	writeEdgesHash(h, box.Border)
	writeEdgesHash(h, box.Margin)
	writeRectHash(h, box.FlowMarginBox)
	writeFloatHash(h, box.TextHeight)
	writeUintHash(h, uint64(len(box.TextFragments)))
	for _, fragment := range box.TextFragments {
		writeRectHash(h, fragment.Rect)
		writeStringHash(h, fragment.Text)
		writeUintHash(h, uint64(fragment.Line))
		writeUintHash(h, uint64(len(fragment.Glyphs)))
		for _, glyph := range fragment.Glyphs {
			writeUintHash(h, uint64(glyph.Rune))
			writeFloatHash(h, glyph.X)
			writeFloatHash(h, glyph.Y)
			writeFloatHash(h, glyph.Advance)
		}
	}
	writeUintHash(h, uint64(len(box.ImageFragments)))
	for _, fragment := range box.ImageFragments {
		writeRectHash(h, fragment.Rect)
		writeUintHash(h, uint64(fragment.Line))
	}
	writeUintHash(h, uint64(len(box.ControlFragments)))
	for _, fragment := range box.ControlFragments {
		writeRectHash(h, fragment.Rect)
		writeRectHash(h, fragment.Content)
		writeEdgesHash(h, fragment.Padding)
		writeEdgesHash(h, fragment.Border)
		writeUintHash(h, uint64(fragment.Line))
		writeUintHash(h, uint64(len(fragment.ImageFragments)))
		for _, image := range fragment.ImageFragments {
			writeRectHash(h, image.Rect)
			writeUintHash(h, uint64(image.Line))
		}
	}
}

func writeRectHash(h *geometryHash, rect Rect) {
	writeFloatHash(h, rect.X)
	writeFloatHash(h, rect.Y)
	writeFloatHash(h, rect.Width)
	writeFloatHash(h, rect.Height)
}

func writeEdgesHash(h *geometryHash, edges Edges) {
	writeFloatHash(h, edges.Top)
	writeFloatHash(h, edges.Right)
	writeFloatHash(h, edges.Bottom)
	writeFloatHash(h, edges.Left)
}

func writeStringHash(h *geometryHash, value string) {
	writeUintHash(h, uint64(len(value)))
	h.writeString(value)
}

func writeFloatHash(h *geometryHash, value float64) {
	writeUintHash(h, math.Float64bits(value))
}

func writeUintHash(h *geometryHash, value uint64) {
	h.sum ^= value
	h.sum *= 1099511628211
}
