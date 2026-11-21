package layout

import (
	"unicode/utf8"

	"github.com/onix-engine/onix/font"
	"github.com/onix-engine/onix/html/style"
	uinode "github.com/onix-engine/onix/scene/node"
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
	scratch           *boxScratch
	flex              *flexScratch
	grid              *gridScratch
	table             *tableScratch
	Style             *style.ComputedStyle
	Parent            *Box
	Node              *uinode.Node
	ControlFragments  []ControlFragment
	InlineBackgrounds []InlineBackground
	ImageFragments    []ImageFragment
	TextFragments     []TextFragment
	Children          []*Box
	reflow            boxLayoutCache
	// StableID identifies this box within its builder/cache lifetime. It survives
	// sibling insertions, reordering and reparenting; zero means unassigned.
	StableID      uint64
	Padding       Edges
	FlowMarginBox Rect
	// ScrollbarGutter is reserved between the padding edge and border.
	ScrollbarGutter   Edges
	Border            Edges
	Content           Rect
	Margin            Edges
	ScrollHeight      float64
	TextHeight        float64
	ScrollWidth       float64
	ownPaintHash      uint64
	hasFloats         bool
	ownPaintHashValid bool
}

// Ordinary text rows do not need float, inline-box, or intrinsic measurement
// workspace. Allocate these together only for boxes that use them.
type boxScratch struct {
	marker      *listMarkerCache
	layout      *layoutContext
	inlinePaint map[inlineBackgroundKey]int
	flow        *flowScratch
	viewport    *Rect
	floats      floatContext
}

func (box *Box) ensureScratch() *boxScratch {
	if box.scratch == nil {
		box.scratch = &boxScratch{}
	}
	return box.scratch
}

// Only list items retain numbering and formatted marker text. Ordinary boxes
// (including every table cell) need no marker payload.
type listMarkerCache struct {
	text                 string
	kind                 style.Value
	ordinal, textOrdinal int
	ordinalValid, valid  bool
}

func (box *Box) listMarker() *listMarkerCache {
	scratch := box.ensureScratch()
	if scratch.marker == nil {
		scratch.marker = &listMarkerCache{}
	}
	return scratch.marker
}

// Viewport returns the document viewport on the root box. Descendant boxes
// have no viewport payload and return a zero rectangle.
func (box *Box) Viewport() Rect {
	if box.scratch != nil && box.scratch.viewport != nil {
		return *box.scratch.viewport
	}
	return Rect{}
}

func (box *Box) setViewport(viewport Rect) {
	scratch := box.ensureScratch()
	if scratch.viewport == nil {
		scratch.viewport = &Rect{}
	}
	*scratch.viewport = viewport
}

type flowScratch struct {
	inlineBoxes        []*Box
	inlineBoxIndex     map[*uinode.Node]*Box
	inlineBoxFragments []inlineBoxFragment
}

type TextFragment struct {
	Node          *uinode.Node
	Style         *style.ComputedStyle
	Text          string
	Glyphs        []GlyphFragment
	Rect          Rect
	Line          int
	IsPlaceholder bool
}

// Color resolves the text paint from the current style, including after a restyle.
func (f *TextFragment) Color() style.Value {
	if f.IsPlaceholder {
		return f.Style.PlaceholderColor
	}
	return f.Style.Color
}

type GlyphFragment = font.PositionedGlyph

type ImageFragment struct {
	Node  *uinode.Node
	Style *style.ComputedStyle
	Rect  Rect
	Line  int
}

type ControlFragment struct {
	Node           *uinode.Node
	Style          *style.ComputedStyle
	TextFragments  []TextFragment
	ImageFragments []ImageFragment
	Rect           Rect
	Content        Rect
	Padding        Edges
	Border         Edges
	Line           int
}

func (b *Box) PaddingBox() Rect {
	rect := b.ScrollportBox()
	return expandRect(rect, b.ScrollbarGutter)
}

// ScrollportBox excludes classic scrollbar gutters from the padding box.
func (b *Box) ScrollportBox() Rect {
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
