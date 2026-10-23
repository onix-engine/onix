package layout

import (
	"unicode/utf8"

	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

type Rect struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

type Edges struct {
	Top    float64
	Right  float64
	Bottom float64
	Left   float64
}

type Box struct {
	reflow            boxLayoutCache
	flex              *flexScratch
	grid              *gridScratch
	table             *tableScratch
	flow              *flowScratch
	flexMeasureStyle  *style.ComputedStyle
	gridMeasureStyle  *style.ComputedStyle
	tableMeasureStyle *style.ComputedStyle
	markerText        string
	markerOrdinal     int
	markerType        style.Value
	markerValid       bool
	listOrdinal       int
	listOrdinalValid  bool
	floats            *floatContext
	hasFloats         bool
	intrinsicScratch  *layoutContext
	StableID          string
	Node              *dom.Node
	Parent            *Box
	Viewport          Rect
	Style             *style.ComputedStyle
	Content           Rect
	Padding           Edges
	Border            Edges
	Margin            Edges
	FlowMarginBox     Rect
	TextFragments     []TextFragment
	ImageFragments    []ImageFragment
	ControlFragments  []ControlFragment
	TextHeight        float64
	Children          []*Box

	ownPaintHash      uint64
	ownPaintHashValid bool
}

type flowScratch struct {
	inlineBoxes        []*Box
	inlineBoxIndex     map[*dom.Node]*Box
	inlineBoxFragments []inlineBoxFragment
}

type TextFragment struct {
	Node   *dom.Node
	Rect   Rect
	Text   string
	Style  *style.ComputedStyle
	Line   int
	Glyphs []GlyphFragment
}

type GlyphFragment struct {
	Rune    rune
	X       float64
	Y       float64
	Advance float64
}

type ImageFragment struct {
	Node  *dom.Node
	Rect  Rect
	Style *style.ComputedStyle
	Line  int
}

type ControlFragment struct {
	Node           *dom.Node
	Rect           Rect
	Content        Rect
	Padding        Edges
	Border         Edges
	Style          *style.ComputedStyle
	Line           int
	TextFragments  []TextFragment
	ImageFragments []ImageFragment
}

func (b *Box) PaddingBox() Rect {
	return expandRect(b.Content, b.Padding)
}

func (b *Box) BorderBox() Rect {
	return expandRect(b.PaddingBox(), b.Border)
}

func (b *Box) MarginBox() Rect {
	return expandRect(b.BorderBox(), b.Margin)
}

func (b *Box) InvalidatePaintHash() {
	if b == nil {
		return
	}
	b.ownPaintHash = 0
	b.ownPaintHashValid = false
}

func (b *Box) CachedPaintHash() (uint64, bool) {
	if b == nil || !b.ownPaintHashValid {
		return 0, false
	}
	return b.ownPaintHash, true
}

func (b *Box) SetCachedPaintHash(hash uint64) {
	if b == nil {
		return
	}
	b.ownPaintHash = hash
	b.ownPaintHashValid = true
}

func expandRect(rect Rect, edges Edges) Rect {
	return Rect{
		X:      rect.X - edges.Left,
		Y:      rect.Y - edges.Top,
		Width:  rect.Width + edges.Left + edges.Right,
		Height: rect.Height + edges.Top + edges.Bottom,
	}
}

func CaretXForTextFragments(fragments []TextFragment, caret int) (float64, bool) {
	if len(fragments) == 0 {
		return 0, false
	}
	if caret < 0 {
		caret = 0
	}
	index := 0
	for _, fragment := range fragments {
		start := fragment.Rect.X
		if caret <= index {
			if len(fragment.Glyphs) > 0 {
				left, _ := glyphCaretBounds(fragment, 0, 0)
				return left, true
			}
			return start, true
		}
		advance := 0.0
		for glyphIndex, glyph := range fragment.Glyphs {
			left, right := glyphCaretBounds(fragment, glyphIndex, advance)
			if caret <= index {
				return left, true
			}
			index++
			if caret <= index {
				return right, true
			}
			advance += glyph.Advance
		}
		if len(fragment.Glyphs) == 0 {
			runeCount := utf8.RuneCountInString(fragment.Text)
			if runeCount > 0 {
				step := fragment.Rect.Width / float64(runeCount)
				for range runeCount {
					if caret <= index {
						return start + advance, true
					}
					advance += step
					index++
				}
			}
		}
	}
	last := fragments[len(fragments)-1]
	return last.Rect.X + last.Rect.Width, true
}

func CaretIndexForTextFragments(fragments []TextFragment, x float64) (int, bool) {
	if len(fragments) == 0 {
		return 0, false
	}
	index := 0
	for _, fragment := range fragments {
		advance := 0.0
		start := fragment.Rect.X
		if x <= start {
			return index, true
		}
		for glyphIndex, glyph := range fragment.Glyphs {
			left, right := glyphCaretBounds(fragment, glyphIndex, advance)
			if x < left {
				return index, true
			}
			mid := left + (right-left)/2
			if x < mid {
				return index, true
			}
			if x < right {
				return index + 1, true
			}
			advance += glyph.Advance
			index++
		}
		if len(fragment.Glyphs) == 0 {
			runeCount := utf8.RuneCountInString(fragment.Text)
			if runeCount > 0 {
				step := fragment.Rect.Width / float64(runeCount)
				for range runeCount {
					next := start + advance + step
					if x < start+advance+step/2 {
						return index, true
					}
					advance += step
					index++
					if x <= next {
						return index, true
					}
				}
			}
		}
	}
	return index, true
}

func glyphCaretBounds(fragment TextFragment, glyphIndex int, advance float64) (float64, float64) {
	glyph := fragment.Glyphs[glyphIndex]
	left := glyph.X
	right := fragment.Rect.X + advance + glyph.Advance
	if glyphIndex+1 < len(fragment.Glyphs) && fragment.Glyphs[glyphIndex+1].X > left {
		right = fragment.Glyphs[glyphIndex+1].X
	}
	if glyphIndex == len(fragment.Glyphs)-1 {
		right = fragment.Rect.X + fragment.Rect.Width
	}
	return left, right
}
