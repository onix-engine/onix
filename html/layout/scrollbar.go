package layout

import (
	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/html/style"
)

// ScrollbarSize is shared by layout and painting so the gutter matches its bar.
func ScrollbarSize(computed *style.ComputedStyle) float64 {
	value := computed.ScrollbarWidth
	if value.Kind == style.ValueLengthPx {
		return max(value.LengthPx, 0)
	}
	if value.Kind == style.ValueKeyword {
		switch value.Keyword {
		case "thin":
			return 10
		case "none":
			return 0
		}
	}
	return 15
}

func scrollsOverflow(value style.Value) bool {
	return value == style.Keyword("auto") || value == style.Keyword("scroll")
}

func clipsOverflow(value style.Value) bool {
	return scrollsOverflow(value) || value == style.Keyword("hidden") || value == style.Keyword("clip")
}

func clampScrollContentHeight(box *Box, base float64) float64 {
	gutter := box.ScrollbarGutter.Bottom
	return max(clampContentDimension(box.Content.Height+gutter, box.Style.MinHeight, box.Style.MaxHeight, base, verticalBoxExtras(box.Padding, box.Border), isBorderBox(box.Style))-gutter, 0)
}

func hasDefiniteScrollHeight(box *Box) bool {
	return style.IsLength(box.Style.Height) || isAbsolutePositioned(box) && style.IsLength(box.Style.Top) && style.IsLength(box.Style.Bottom)
}

func layoutBlockScrollbars(box *Box, containing, absolute Rect, face *font.Face, styles style.Map, ctx *layoutContext) {
	box.ScrollbarGutter = Edges{}
	box.ScrollWidth, box.ScrollHeight = 0, 0
	classic := ctx != nil && ctx.reserveScrollbarSpace
	bar := 0.0
	if classic {
		bar = ScrollbarSize(box.Style)
		if box.Style.OverflowY == style.Keyword("scroll") {
			box.ScrollbarGutter.Right = bar
		}
		if box.Style.OverflowX == style.Keyword("scroll") {
			box.ScrollbarGutter.Bottom = bar
		}
	}
	for {
		layoutBlockContents(box, containing, absolute, face, styles, ctx)
		if !classic {
			return
		}
		updateScrollExtent(box)
		r := box.ScrollportBox()
		next := box.ScrollbarGutter
		if bar > 0 && box.ScrollWidth > r.Width && scrollsOverflow(box.Style.OverflowX) {
			next.Bottom = bar
			if hasDefiniteScrollHeight(box) {
				next.Bottom = min(bar, box.Content.Height+box.ScrollbarGutter.Bottom)
			}
		}
		if bar > 0 && box.ScrollHeight > r.Height && scrollsOverflow(box.Style.OverflowY) {
			next.Right = min(bar, box.Content.Width+box.ScrollbarGutter.Right)
		}
		if next == box.ScrollbarGutter || ctx.err != nil {
			return
		}
		// Gutters only grow within a reflow. At most two relayouts are
		// needed, including overflow introduced by the other axis's gutter.
		box.ScrollbarGutter = next
	}
}

// Descendant extents are retained with their boxes. Inspect direct children
// and fragments together, without rescanning nested subtrees for each scroller.
func updateScrollExtent(box *Box) {
	r := box.ScrollportBox()
	width, height := r.Width, max(r.Height, box.Padding.Top+box.TextHeight+box.Padding.Bottom)
	for _, child := range box.Children {
		border := child.BorderBox()
		childWidth, childHeight := border.Width, border.Height
		if !clipsOverflow(child.Style.OverflowX) {
			childWidth = max(childWidth, child.Border.Left+child.ScrollWidth)
		}
		if !clipsOverflow(child.Style.OverflowY) {
			childHeight = max(childHeight, child.Border.Top+child.ScrollHeight)
		}
		width = max(width, border.X-r.X+childWidth)
		height = max(height, border.Y-r.Y+childHeight)
	}
	for _, fragment := range box.TextFragments {
		width = max(width, fragment.Rect.X-r.X+fragment.Rect.Width)
		height = max(height, fragment.Rect.Y-r.Y+fragment.Rect.Height)
	}
	for _, fragment := range box.ImageFragments {
		width = max(width, fragment.Rect.X-r.X+fragment.Rect.Width)
		height = max(height, fragment.Rect.Y-r.Y+fragment.Rect.Height)
	}
	for _, fragment := range box.ControlFragments {
		width = max(width, fragment.Rect.X-r.X+fragment.Rect.Width)
		height = max(height, fragment.Rect.Y-r.Y+fragment.Rect.Height)
	}
	box.ScrollWidth, box.ScrollHeight = width, height
}
