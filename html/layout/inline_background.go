package layout

import (
	"github.com/onix-engine/onix/html/style"
	uinode "github.com/onix-engine/onix/scene/node"
)

// Inline backgrounds follow the fragments on each line, not the containing
// block. One run includes nested syntax spans and intervening whitespace.
type InlineBackground struct {
	Node  *uinode.Node
	Style *style.ComputedStyle
	Rect  Rect
	Line  int
}

type inlineBackgroundKey struct {
	node *uinode.Node
	line int
}

func (b *Box) UpdateInlineBackgrounds(styles style.Map) {
	clear(b.InlineBackgrounds)
	b.InlineBackgrounds = b.InlineBackgrounds[:0]
	if b.scratch != nil {
		clear(b.scratch.inlinePaint)
	}
	for _, fragment := range b.TextFragments {
		node := fragment.Node
		if node != nil && node.Type == uinode.TextNode {
			node = node.Parent
		}
		b.appendInlineBackground(node, fragment.Rect, fragment.Line, styles)
	}
	for _, fragment := range b.ImageFragments {
		if fragment.Node != nil {
			b.appendInlineBackground(fragment.Node.Parent, fragment.Rect, fragment.Line, styles)
		}
	}
}

func (b *Box) appendInlineBackground(node *uinode.Node, rect Rect, line int, styles style.Map) {
	if node == nil || node == b.Node {
		return
	}
	computed := styles[node]
	if computed == nil || computed.Display != style.Keyword("inline") && computed.Display != style.Keyword("contents") {
		return
	}
	b.appendInlineBackground(node.Parent, rect, line, styles)
	if computed.Display == style.Keyword("contents") || computed.BackgroundColor.Color.A == 0 && computed.Background.Image == "" && computed.AnimatedPaint&style.AnimatedBackgroundColor == 0 || rect.Width <= 0 || rect.Height <= 0 {
		return
	}
	key := inlineBackgroundKey{node, line}
	scratch := b.ensureScratch()
	if index, ok := scratch.inlinePaint[key]; ok {
		previous := &b.InlineBackgrounds[index].Rect
		right, bottom := max(previous.X+previous.Width, rect.X+rect.Width), max(previous.Y+previous.Height, rect.Y+rect.Height)
		previous.X, previous.Y = min(previous.X, rect.X), min(previous.Y, rect.Y)
		previous.Width, previous.Height = right-previous.X, bottom-previous.Y
		return
	}
	if scratch.inlinePaint == nil {
		scratch.inlinePaint = make(map[inlineBackgroundKey]int)
	}
	scratch.inlinePaint[key] = len(b.InlineBackgrounds)
	b.InlineBackgrounds = append(b.InlineBackgrounds, InlineBackground{Node: node, Style: computed, Rect: rect, Line: line})
}
