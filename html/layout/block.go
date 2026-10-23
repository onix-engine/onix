package layout

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
)

func layoutBlockContents(box *Box, containing Rect, absoluteContaining Rect, face *font.Face, styles style.Map, ctx *layoutContext) {
	if ctx != nil && establishesFloatContext(box) {
		previous := ctx.floatContext
		if box.floats == nil {
			box.floats = &floatContext{}
		}
		box.floats.areas = box.floats.areas[:0]
		ctx.floatContext = box.floats
		defer func() { ctx.floatContext = previous }()
	}
	layoutContaining := containing
	if isAbsolutePositioned(box) {
		layoutContaining = absoluteContaining
	}
	box.Margin = resolveEdges(box.Style.Margin, layoutContaining.Width)
	box.Padding = resolveEdges(box.Style.Padding, layoutContaining.Width)
	box.Border = resolveBorderEdges(box.Style.BorderWidth, box.Style.BorderStyle, layoutContaining.Width)

	intrinsicWidth, intrinsicHeight := intrinsicImageSize(box, ctx)
	intrinsicWidth, intrinsicHeight = resolveImageSize(box.Node, box.Style, intrinsicWidth, intrinsicHeight, layoutContaining.Width, layoutContaining.Height)
	hasIntrinsicRatio := intrinsicWidth > 0 && intrinsicHeight > 0
	controlWidth, controlHeight := intrinsicControlSize(box, face, ctx, styles)
	if controlWidth > 0 {
		if box.Style.Width.Kind == style.ValueAuto && !isBorderBox(box.Style) {
			controlWidth -= horizontalBoxExtras(box.Padding, box.Border)
			if controlWidth <= 0 {
				controlWidth = 1
			}
		}
		intrinsicWidth = controlWidth
	}
	if controlHeight > 0 {
		if box.Style.Height.Kind == style.ValueAuto && !isBorderBox(box.Style) {
			controlHeight -= verticalBoxExtras(box.Padding, box.Border)
		}
		intrinsicHeight = controlHeight
	}

	contentWidth := resolveLength(box.Style.Width, layoutContaining.Width)
	if style.IsLength(box.Style.Width) && isBorderBox(box.Style) {
		contentWidth -= horizontalBoxExtras(box.Padding, box.Border)
	}
	if box.Style.Width.Kind == style.ValueAuto {
		if intrinsicWidth > 0 {
			contentWidth = intrinsicWidth
			if isBorderBox(box.Style) {
				contentWidth -= horizontalBoxExtras(box.Padding, box.Border)
			}
		} else {
			contentWidth = layoutContaining.Width - box.Margin.Left - box.Margin.Right - box.Border.Left - box.Border.Right - box.Padding.Left - box.Padding.Right
			if contentWidth < 0 {
				contentWidth = 0
			}
		}
	}
	if box.Style.Width.Kind == style.ValueAuto && style.IsLength(box.Style.Height) && hasIntrinsicRatio {
		contentHeight := resolveLength(box.Style.Height, layoutContaining.Height)
		if isBorderBox(box.Style) {
			contentHeight -= verticalBoxExtras(box.Padding, box.Border)
		}
		contentWidth = maxFloat(contentHeight, 0) * intrinsicWidth / intrinsicHeight
	}
	contentHeight := 0.0
	if isAbsolutePositioned(box) && box.Style.Height.Kind == style.ValueAuto && style.IsLength(box.Style.Top) && style.IsLength(box.Style.Bottom) {
		contentHeight = layoutContaining.Height - resolveLength(box.Style.Top, layoutContaining.Height) - resolveLength(box.Style.Bottom, layoutContaining.Height) - verticalBoxExtras(box.Padding, box.Border) - box.Margin.Top - box.Margin.Bottom
		contentHeight = maxFloat(contentHeight, 0)
	}
	if isAbsolutePositioned(box) && box.Style.Width.Kind == style.ValueAuto && style.IsLength(box.Style.Left) && style.IsLength(box.Style.Right) {
		contentWidth = layoutContaining.Width - resolveLength(box.Style.Left, layoutContaining.Width) - resolveLength(box.Style.Right, layoutContaining.Width) - horizontalBoxExtras(box.Padding, box.Border) - box.Margin.Left - box.Margin.Right
		contentWidth = maxFloat(contentWidth, 0)
	} else if isAbsolutePositioned(box) && box.Style.Width.Kind == style.ValueAuto && intrinsicWidth == 0 {
		// CSS 2.1 10.3.7: auto width with an auto horizontal inset uses
		// shrink-to-fit sizing, bounded by the space beside the definite inset.
		extras := horizontalBoxExtras(box.Padding, box.Border)
		available := layoutContaining.Width - box.Margin.Left - box.Margin.Right - extras
		if style.IsLength(box.Style.Left) {
			available -= resolveLength(box.Style.Left, layoutContaining.Width)
		} else if style.IsLength(box.Style.Right) {
			available -= resolveLength(box.Style.Right, layoutContaining.Width)
		}
		minimum, preferred := tableCellIntrinsicWidths(box, face, styles, ctx)
		contentWidth = maxFloat(0, min(max(minimum-extras, available), preferred-extras))
	}

	contentWidth = clampContentDimension(contentWidth, box.Style.MinWidth, box.Style.MaxWidth, layoutContaining.Width, horizontalBoxExtras(box.Padding, box.Border), isBorderBox(box.Style))

	// Resolve definite height before descendants use this box as their containing block.
	if style.IsLength(box.Style.Height) {
		contentHeight = contentDimension(box.Style.Height, layoutContaining.Height, verticalBoxExtras(box.Padding, box.Border), isBorderBox(box.Style))
	}
	if !isAbsolutePositioned(box) && !isFloated(box) && !isFlexContainer(box.Parent) && box.Style.Display.Keyword != "inline-block" && box.Style.Display.Keyword != "inline-flex" {
		free := maxFloat(layoutContaining.Width-contentWidth-horizontalBoxExtras(box.Padding, box.Border)-box.Margin.Left-box.Margin.Right, 0)
		leftAuto, rightAuto := box.Style.Margin.Left.Kind == style.ValueAuto, box.Style.Margin.Right.Kind == style.ValueAuto
		if leftAuto && rightAuto {
			box.Margin.Left, box.Margin.Right = free/2, free/2
		} else if leftAuto {
			box.Margin.Left = free
		} else if rightAuto {
			box.Margin.Right = free
		}
	}
	contentX := layoutContaining.X + box.Margin.Left + box.Border.Left + box.Padding.Left
	contentY := layoutContaining.Y + box.Margin.Top + box.Border.Top + box.Padding.Top
	if isAbsolutePositioned(box) {
		contentX = resolveAbsoluteX(box, layoutContaining, contentWidth)
		contentY = resolveAbsoluteY(box, layoutContaining)
	}

	box.Content = Rect{X: contentX, Y: contentY, Width: contentWidth, Height: contentHeight}
	if box.Style.Display.Kind == style.ValueKeyword && box.Style.Display.Keyword == "table" {
		layoutTableContainer(box, layoutContaining, absoluteContaining, face, styles, ctx)
		return
	}

	if isFlexContainer(box) {
		clear(box.TextFragments)
		box.TextFragments = box.TextFragments[:0]
		clear(box.ImageFragments)
		box.ImageFragments = box.ImageFragments[:0]
		clear(box.ControlFragments)
		box.ControlFragments = box.ControlFragments[:0]
		box.TextHeight = 0
		layoutFlexContainer(box, layoutContaining, absoluteContaining, face, styles, ctx)
		return
	}
	if isGridContainer(box) {
		clear(box.TextFragments)
		box.TextFragments = box.TextFragments[:0]
		clear(box.ImageFragments)
		box.ImageFragments = box.ImageFragments[:0]
		clear(box.ControlFragments)
		box.ControlFragments = box.ControlFragments[:0]
		box.TextHeight = 0
		layoutGridContainer(box, layoutContaining, absoluteContaining, face, styles, ctx)
		return
	}

	cursorY := box.Content.Y
	_, inlineBoxIndex := layoutInlineChildren(box, layoutContaining, absoluteContaining, face, styles, ctx)
	var reusedInlineBoxFragments []inlineBoxFragment
	if box.flow != nil {
		reusedInlineBoxFragments = box.flow.inlineBoxFragments
	}
	textFragments, imageFragments, controlFragments, inlineBoxFragments, textHeight, err := layoutText(TextLayoutOptions{
		Reuse:    box,
		Face:     face,
		StyleMap: styles,
		Width:    box.Content.Width,
		OriginX:  box.Content.X,
		OriginY:  box.Content.Y,
		ContentHeight: func() float64 {
			if box.Content.Height > 0 {
				return box.Content.Height
			}
			// An auto-height button grows with its flow content. Its intrinsic
			// single-line height must not center multiline text before sizing.
			if box.Node.Data == "button" {
				return 0
			}
			if isControlElementNode(box.Node) {
				if isBorderBox(box.Style) {
					return maxFloat(intrinsicHeight-verticalBoxExtras(box.Padding, box.Border), 0)
				}
				return intrinsicHeight
			}
			return 0
		}(),
		FlowHeight: func() float64 {
			if style.IsLength(box.Style.Height) || contentHeight > 0 {
				return box.Content.Height
			}
			return layoutContaining.Height
		}(),
		Node:               box.Node,
		Context:            ctx,
		InlineBoxes:        inlineBoxIndex,
		InlineBoxFragments: reusedInlineBoxFragments,
		FlowBoxes:          box.Children,
		AbsoluteContaining: descendantAbsoluteContaining(box, absoluteContaining),
	})
	if err != nil {
		if ctx != nil && ctx.err == nil {
			ctx.err = err
		}
	} else {
		textFragments = appendListMarker(textFragments, box, face, ctx)
		if ctx != nil && establishesFloatContext(box) {
			textHeight = maxFloat(textHeight, ctx.floatContext.clearance(box.Content.Y, "both")-box.Content.Y)
		}
		if box.Style.Display.Kind == style.ValueKeyword && box.Style.Display.Keyword == "list-item" && box.Style.ListStyleType != style.Keyword("none") {
			markerHeight := UsedLineHeight(box.Style, fontForStyle(face, box.Style, ctx))
			if markerHeight > textHeight {
				textHeight = markerHeight
			}
		}
		clearLayoutTail(box.TextFragments, len(textFragments))
		clearLayoutTail(box.ImageFragments, len(imageFragments))
		clearLayoutTail(box.ControlFragments, len(controlFragments))
		box.TextFragments = textFragments
		box.ImageFragments = imageFragments
		box.ControlFragments = controlFragments
		box.TextHeight = textHeight
		if box.flow != nil {
			box.flow.inlineBoxFragments = inlineBoxFragments
		}
		scrollTextInputFragmentsIntoView(box)
		for _, fragment := range inlineBoxFragments {
			translateBox(fragment.Box, fragment.Rect.X-fragment.Box.FlowMarginBox.X, fragment.Rect.Y-fragment.Box.FlowMarginBox.Y, ctx)
		}
		cursorY += textHeight
	}

	for _, child := range box.Children {
		if isAbsolutePositioned(child) {
			layoutBlock(child, box.Content, descendantAbsoluteContaining(box, absoluteContaining), face, styles, ctx)
		}
	}

	if style.IsLength(box.Style.Height) {
		box.Content.Height = resolveLength(box.Style.Height, layoutContaining.Height)
		if isBorderBox(box.Style) {
			box.Content.Height -= verticalBoxExtras(box.Padding, box.Border)
		}
	} else if hasIntrinsicRatio {
		box.Content.Height = intrinsicHeight
		if contentWidth != intrinsicWidth {
			box.Content.Height = contentWidth * intrinsicHeight / intrinsicWidth
		}
		if isBorderBox(box.Style) {
			box.Content.Height -= verticalBoxExtras(box.Padding, box.Border)
		}
	} else if intrinsicHeight > 0 {
		box.Content.Height = intrinsicHeight
		if isBorderBox(box.Style) {
			box.Content.Height -= verticalBoxExtras(box.Padding, box.Border)
		}
		if box.Node.Data == "button" {
			box.Content.Height = max(box.Content.Height, cursorY-box.Content.Y)
		}
	} else {
		if contentHeight > 0 {
			box.Content.Height = contentHeight
		} else {
			box.Content.Height = cursorY - box.Content.Y
		}
	}
	box.Content.Height = clampContentDimension(box.Content.Height, box.Style.MinHeight, box.Style.MaxHeight, layoutContaining.Height, verticalBoxExtras(box.Padding, box.Border), isBorderBox(box.Style))
	alignBottomPositionedChildren(box, absoluteContaining, ctx)
	if isControlElementNode(box.Node) {
		box.ImageFragments = layoutControlImageFragments(box.Node, box.Content, 0, styles, ctx, box.TextFragments)
	}

	box.FlowMarginBox = box.MarginBox()
	if isRelativePositioned(box) {
		translateBox(box, resolveRelativeOffsetX(box.Style, layoutContaining.Width), resolveRelativeOffsetY(box.Style, layoutContaining.Height), ctx)
	}
}

func appendListMarker(fragments []TextFragment, box *Box, face *font.Face, ctx *layoutContext) []TextFragment {
	if box == nil || box.Style == nil || box.Style.Display.Kind != style.ValueKeyword || box.Style.Display.Keyword != "list-item" {
		return fragments
	}
	ordinal := box.listOrdinal
	if !box.listOrdinalValid {
		ordinal = 1
	}
	if !box.markerValid || box.markerOrdinal != ordinal || box.markerType != box.Style.ListStyleType {
		box.markerText = listMarkerText(ordinal, box.Style.ListStyleType)
		box.markerOrdinal, box.markerType, box.markerValid = ordinal, box.Style.ListStyleType, true
	}
	marker := box.markerText
	if marker == "" {
		return fragments
	}
	markerFace := fontForStyle(face, box.Style, ctx)
	fontSize := box.Style.FontSize.LengthPx
	width := measureTextWidth(markerFace, marker, fontSize, box.Style.LetterSpacing.LengthPx, ctx)
	lineHeight := UsedLineHeight(box.Style, markerFace)
	if lineHeight <= 0 {
		lineHeight = box.Style.LineHeight.LengthPx
	}
	x := box.Content.X - width - 8
	fragment := TextFragment{Node: box.Node, Rect: Rect{X: x, Y: box.Content.Y, Width: width, Height: lineHeight}, Text: marker, Style: box.Style, Line: 0}
	if len(fragments) < cap(fragments) {
		fragment.Glyphs = fragments[:cap(fragments)][len(fragments)].Glyphs[:0]
	}
	if markerFace != nil {
		glyphs, _, err := shapeToken(markerFace, textToken{Text: marker}, fontSize, box.Style.LetterSpacing.LengthPx, ctx)
		if err == nil {
			baseline := box.Content.Y + float64(markerFace.Hhea.Ascent)*fontSize/float64(markerFace.Head.UnitsPerEm)
			for _, glyph := range glyphs {
				fragment.Glyphs = append(fragment.Glyphs, GlyphFragment{Rune: glyph.Rune, X: x + glyph.X, Y: baseline, Advance: glyph.Advance})
			}
		}
	}
	return append(fragments, fragment)
}

func listMarkerText(ordinal int, marker style.Value) string {
	if marker.Kind != style.ValueKeyword || marker.Keyword == "none" {
		return ""
	}
	switch marker.Keyword {
	case "disc":
		return "\u2022"
	case "circle":
		return "\u25e6"
	case "square":
		return "\u25aa"
	case "decimal-leading-zero":
		decimal := strconv.Itoa(ordinal)
		if ordinal > -10 && ordinal < 0 {
			decimal = "-0" + decimal[1:]
		} else if len(decimal) < 2 {
			decimal = "0" + decimal
		}
		return decimal + "."
	case "lower-alpha", "lower-latin":
		if ordinal <= 0 {
			return strconv.Itoa(ordinal) + "."
		}
		return alphaMarker(ordinal, false) + "."
	case "upper-alpha", "upper-latin":
		if ordinal <= 0 {
			return strconv.Itoa(ordinal) + "."
		}
		return alphaMarker(ordinal, true) + "."
	case "lower-roman":
		return romanMarker(ordinal, false) + "."
	case "upper-roman":
		return romanMarker(ordinal, true) + "."
	default:
		return strconv.Itoa(ordinal) + "."
	}
}

func alphaMarker(value int, upper bool) string {
	if value <= 0 {
		return ""
	}
	var result [16]byte
	index := len(result)
	for value > 0 {
		value--
		index--
		letter := byte('a' + value%26)
		if upper {
			letter -= 'a' - 'A'
		}
		result[index] = letter
		value /= 26
	}
	return string(result[index:])
}

func romanMarker(value int, upper bool) string {
	if value <= 0 || value > 3999 {
		return strconv.Itoa(value)
	}
	var result strings.Builder
	for _, item := range [...]struct {
		value int
		text  string
	}{{1000, "m"}, {900, "cm"}, {500, "d"}, {400, "cd"}, {100, "c"}, {90, "xc"}, {50, "l"}, {40, "xl"}, {10, "x"}, {9, "ix"}, {5, "v"}, {4, "iv"}, {1, "i"}} {
		for value >= item.value {
			result.WriteString(item.text)
			value -= item.value
		}
	}
	text := result.String()
	if upper {
		return strings.ToUpper(text)
	}
	return text
}

func layoutInlineChildren(box *Box, layoutContaining Rect, absoluteContaining Rect, face *font.Face, styles style.Map, ctx *layoutContext) ([]*Box, map[*dom.Node]*Box) {
	flow := box.flow
	if flow != nil {
		clear(flow.inlineBoxes)
		flow.inlineBoxes = flow.inlineBoxes[:0]
		clear(flow.inlineBoxIndex)
	}
	ordinal, step := 1, 1
	if box.Node != nil && box.Node.Data == "ol" {
		_, reversed := box.Node.Attr("reversed")
		if reversed {
			step, ordinal = -1, 0
			for _, child := range box.Node.Children {
				if child.Type == dom.ElementNode && child.Data == "li" {
					ordinal++
				}
			}
		}
		if raw, ok := box.Node.Attr("start"); ok {
			if value, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
				ordinal = value
			}
		}
	}
	for _, child := range box.Children {
		if child.Style.Display == style.Keyword("list-item") {
			if child.Node != nil && child.Node.Data == "li" {
				if raw, ok := child.Node.Attr("value"); ok {
					if value, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
						ordinal = value
					}
				}
			}
			if child.listOrdinalValid && child.listOrdinal != ordinal {
				child.reflow.valid = false
			}
			child.listOrdinal, child.listOrdinalValid = ordinal, true
			ordinal += step
		}
		if child.Style.Display.Kind != style.ValueKeyword || isAbsolutePositioned(child) || isFloated(child) {
			continue
		}
		switch child.Style.Display.Keyword {
		case "inline-flex", "inline-grid", "inline-block":
		default:
			continue
		}
		if flow == nil {
			flow = &flowScratch{inlineBoxIndex: make(map[*dom.Node]*Box)}
			box.flow = flow
		}
		contentWidth := intrinsicFlexContentWidth(child, face, ctx, styles)
		if style.IsLength(child.Style.Width) {
			contentWidth = resolveLength(child.Style.Width, box.Content.Width)
			if isBorderBox(child.Style) {
				contentWidth -= horizontalBoxExtras(resolveEdges(child.Style.Padding, box.Content.Width), resolveBorderEdges(child.Style.BorderWidth, child.Style.BorderStyle, box.Content.Width))
			}
			contentWidth = maxFloat(contentWidth, 0)
		}
		margin := resolveEdges(child.Style.Margin, box.Content.Width)
		padding := resolveEdges(child.Style.Padding, box.Content.Width)
		border := resolveBorderEdges(child.Style.BorderWidth, child.Style.BorderStyle, box.Content.Width)
		outerWidth := contentWidth + margin.Left + margin.Right + padding.Left + padding.Right + border.Left + border.Right
		containing := Rect{X: box.Content.X, Y: box.Content.Y, Width: maxFloat(outerWidth, 1), Height: layoutContaining.Height}
		layoutBlock(child, containing, absoluteContaining, face, styles, ctx)
		flow.inlineBoxes = append(flow.inlineBoxes, child)
		flow.inlineBoxIndex[child.Node] = child
	}
	if flow == nil {
		return nil, nil
	}
	return flow.inlineBoxes, flow.inlineBoxIndex
}

func alignBottomPositionedChildren(box *Box, absoluteContaining Rect, ctx *layoutContext) {
	if box == nil || len(box.Children) == 0 {
		return
	}
	containing := descendantAbsoluteContaining(box, absoluteContaining)
	for _, child := range box.Children {
		if !isBottomAnchoredAbsolutePositioned(child) {
			continue
		}
		targetY := resolveAbsoluteY(child, containing)
		translateBox(child, 0, targetY-child.Content.Y, ctx)
	}
}

func isBottomAnchoredAbsolutePositioned(box *Box) bool {
	return isAbsolutePositioned(box) && style.IsLength(box.Style.Bottom) && !style.IsLength(box.Style.Top)
}

func descendantAbsoluteContaining(box *Box, absoluteContaining Rect) Rect {
	if establishesAbsoluteContainingBlock(box) {
		return box.PaddingBox()
	}
	return absoluteContaining
}

func isFlexContainer(box *Box) bool {
	return box != nil && box.Style.Display.Kind == style.ValueKeyword && isFlexDisplay(box.Style.Display.Keyword)
}

func isGridContainer(box *Box) bool {
	return box != nil && box.Style.Display.Kind == style.ValueKeyword && isGridDisplay(box.Style.Display.Keyword)
}

func isRelativePositioned(box *Box) bool {
	return box != nil && box.Style.Position.Kind == style.ValueKeyword && box.Style.Position.Keyword == "relative"
}

func isAbsolutePositioned(box *Box) bool {
	return box != nil && box.Style.Position.Kind == style.ValueKeyword && box.Style.Position.Keyword == "absolute"
}

func establishesAbsoluteContainingBlock(box *Box) bool {
	if box == nil || box.Style.Position.Kind != style.ValueKeyword {
		return false
	}
	return box.Style.Position.Keyword != "static"
}

func resolveRelativeOffsetX(computed *style.ComputedStyle, base float64) float64 {
	if style.IsLength(computed.Left) {
		return resolveLength(computed.Left, base)
	}
	if style.IsLength(computed.Right) {
		return -resolveLength(computed.Right, base)
	}
	return 0
}

func resolveRelativeOffsetY(computed *style.ComputedStyle, base float64) float64 {
	if style.IsLength(computed.Top) {
		return resolveLength(computed.Top, base)
	}
	if style.IsLength(computed.Bottom) {
		return -resolveLength(computed.Bottom, base)
	}
	return 0
}

func resolveAbsoluteX(box *Box, containing Rect, contentWidth float64) float64 {
	baseLeft := containing.X + box.Margin.Left + box.Border.Left + box.Padding.Left
	if style.IsLength(box.Style.Left) {
		return containing.X + resolveLength(box.Style.Left, containing.Width) + box.Margin.Left + box.Border.Left + box.Padding.Left
	}
	if style.IsLength(box.Style.Right) {
		return containing.X + containing.Width - resolveLength(box.Style.Right, containing.Width) - box.Margin.Right - box.Border.Right - box.Padding.Right - contentWidth
	}
	return baseLeft
}

func resolveAbsoluteY(box *Box, containing Rect) float64 {
	baseTop := containing.Y + box.Margin.Top + box.Border.Top + box.Padding.Top
	if style.IsLength(box.Style.Top) {
		return containing.Y + resolveLength(box.Style.Top, containing.Height) + box.Margin.Top + box.Border.Top + box.Padding.Top
	}
	if style.IsLength(box.Style.Bottom) {
		height := box.Content.Height
		if style.IsLength(box.Style.Height) {
			height = clampContentDimension(contentDimension(box.Style.Height, containing.Height, verticalBoxExtras(box.Padding, box.Border), isBorderBox(box.Style)), box.Style.MinHeight, box.Style.MaxHeight, containing.Height, verticalBoxExtras(box.Padding, box.Border), isBorderBox(box.Style))
		}
		if height > 0 || style.IsLength(box.Style.Height) {
			return containing.Y + containing.Height - resolveLength(box.Style.Bottom, containing.Height) - box.Margin.Bottom - box.Border.Bottom - box.Padding.Bottom - height
		}
	}
	return baseTop
}

func translateBox(box *Box, dx, dy float64, ctx *layoutContext) {
	translateLayoutBox(box, dx, dy, false, ctx)
}

func translateLayoutBox(box *Box, dx, dy float64, flow bool, ctx *layoutContext) {
	if dx == 0 && dy == 0 || box == nil {
		return
	}

	if ctx != nil && ctx.retained != nil {
		ctx.retained.recordChanged(box)
	}
	box.InvalidatePaintHash()
	if flow {
		box.FlowMarginBox.X += dx
		box.FlowMarginBox.Y += dy
		box.reflow.containing.X += dx
		box.reflow.containing.Y += dy
	} else {
		// Flex/grid placement and ancestor offsets move the measured output,
		// not its inputs. Remember the displacement so a later identical measure
		// can restore its original position without laying out the subtree.
		box.reflow.placementX += dx
		box.reflow.placementY += dy
		if !box.reflow.placementReusable {
			box.reflow.valid = false
		}
	}
	box.Content.X += dx
	box.Content.Y += dy
	for i := range box.TextFragments {
		box.TextFragments[i].Rect.X += dx
		box.TextFragments[i].Rect.Y += dy
		for j := range box.TextFragments[i].Glyphs {
			box.TextFragments[i].Glyphs[j].X += dx
			box.TextFragments[i].Glyphs[j].Y += dy
		}
	}
	for i := range box.ImageFragments {
		box.ImageFragments[i].Rect.X += dx
		box.ImageFragments[i].Rect.Y += dy
	}
	for i := range box.ControlFragments {
		box.ControlFragments[i].Rect.X += dx
		box.ControlFragments[i].Rect.Y += dy
		box.ControlFragments[i].Content.X += dx
		box.ControlFragments[i].Content.Y += dy
		for j := range box.ControlFragments[i].TextFragments {
			box.ControlFragments[i].TextFragments[j].Rect.X += dx
			box.ControlFragments[i].TextFragments[j].Rect.Y += dy
			for k := range box.ControlFragments[i].TextFragments[j].Glyphs {
				box.ControlFragments[i].TextFragments[j].Glyphs[k].X += dx
				box.ControlFragments[i].TextFragments[j].Glyphs[k].Y += dy
			}
		}
		for j := range box.ControlFragments[i].ImageFragments {
			box.ControlFragments[i].ImageFragments[j].Rect.X += dx
			box.ControlFragments[i].ImageFragments[j].Rect.Y += dy
		}
	}
	for _, child := range box.Children {
		translateLayoutBox(child, dx, dy, flow, ctx)
	}
}

func resolveEdges(edges style.Edges, base float64) Edges {
	return Edges{
		Top:    resolveLength(edges.Top, base),
		Right:  resolveLength(edges.Right, base),
		Bottom: resolveLength(edges.Bottom, base),
		Left:   resolveLength(edges.Left, base),
	}
}

func resolveBorderEdges(widths, borderStyle style.Edges, base float64) Edges {
	edges := resolveEdges(widths, base)
	if !isPaintedBorderStyle(borderStyle.Top) {
		edges.Top = 0
	}
	if !isPaintedBorderStyle(borderStyle.Right) {
		edges.Right = 0
	}
	if !isPaintedBorderStyle(borderStyle.Bottom) {
		edges.Bottom = 0
	}
	if !isPaintedBorderStyle(borderStyle.Left) {
		edges.Left = 0
	}
	return edges
}

func resolveLength(value style.Value, base float64) float64 {
	return style.ResolveLength(value, base)
}

func clampContentDimension(value float64, minimum, maximum style.Value, base, extras float64, borderBox bool) float64 {
	if style.IsLength(minimum) {
		minimumValue := resolveLength(minimum, base)
		if borderBox {
			minimumValue -= extras
		}
		value = maxFloat(value, minimumValue)
	}
	if style.IsLength(maximum) {
		maximumValue := resolveLength(maximum, base)
		if borderBox {
			maximumValue -= extras
		}
		value = minFloat(value, maximumValue)
	}
	return maxFloat(value, 0)
}

func contentDimension(value style.Value, base, extras float64, borderBox bool) float64 {
	resolved := resolveLength(value, base)
	if borderBox {
		resolved -= extras
	}
	return maxFloat(resolved, 0)
}

func borderBoxDimension(value style.Value, base, extras float64, borderBox bool) float64 {
	resolved := resolveLength(value, base)
	if !borderBox {
		resolved += extras
	}
	return maxFloat(resolved, 0)
}

func isBorderBox(computed *style.ComputedStyle) bool {
	return computed.BoxSizing.Kind == style.ValueKeyword && computed.BoxSizing.Keyword == "border-box"
}

func horizontalBoxExtras(padding, border Edges) float64 {
	return padding.Left + padding.Right + border.Left + border.Right
}

func verticalBoxExtras(padding, border Edges) float64 {
	return padding.Top + padding.Bottom + border.Top + border.Bottom
}

func computedHorizontalBoxExtras(computed *style.ComputedStyle, base float64) float64 {
	return horizontalBoxExtras(resolveEdges(computed.Padding, base), resolveBorderEdges(computed.BorderWidth, computed.BorderStyle, base))
}

func computedVerticalBoxExtras(computed *style.ComputedStyle, base float64) float64 {
	return verticalBoxExtras(resolveEdges(computed.Padding, base), resolveBorderEdges(computed.BorderWidth, computed.BorderStyle, base))
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func isPaintedBorderStyle(value style.Value) bool {
	return value.Kind == style.ValueKeyword && value.Keyword == "solid"
}

func intrinsicImageSize(box *Box, ctx *layoutContext) (float64, float64) {
	if box == nil || box.Node == nil || box.Node.Type != dom.ElementNode || box.Node.Data != "img" {
		return 0, 0
	}
	src, ok := box.Node.Attr("src")
	if !ok || src == "" {
		return 0, 0
	}
	width, height, ok := resourceImageSize(ctx, src)
	if !ok {
		return 0, 0
	}
	return width, height
}

const (
	textInputCaretScrollPadding = 12.0
)

func intrinsicControlSize(box *Box, face *font.Face, ctx *layoutContext, styles style.Map) (float64, float64) {
	if box == nil || box.Node == nil || box.Node.Type != dom.ElementNode {
		return 0, 0
	}
	face = fontForStyle(face, box.Style, ctx)

	fontSize := box.Style.FontSize.LengthPx
	if fontSize <= 0 {
		fontSize = 16
	}
	lineHeight := UsedLineHeight(box.Style, face)
	paddingWidth := box.Padding.Left + box.Padding.Right
	paddingHeight := box.Padding.Top + box.Padding.Bottom
	borderWidth := box.Border.Left + box.Border.Right
	borderHeight := box.Border.Top + box.Border.Bottom

	switch box.Node.Data {
	case "button":
		textWidth := measureTextWidth(face, strings.TrimSpace(descendantText(box.Node)), fontSize, box.Style.LetterSpacing.LengthPx, ctx)
		imageWidth, imageHeight, imageCount := intrinsicControlImageMetrics(box.Node, styles, ctx)
		if imageCount > 0 {
			textWidth += imageWidth
			if textWidth > imageWidth {
				textWidth += resolveLength(box.Style.ColumnGap, 0)
			}
		}
		if imageHeight > lineHeight {
			lineHeight = imageHeight
		}
		return textWidth + paddingWidth + borderWidth, lineHeight + paddingHeight + borderHeight
	case "input":
		if isHiddenInputNode(box.Node) {
			return 0, 0
		}
		if isChoiceInputNode(box.Node) {
			return 13, 13
		}
		text, _ := syntheticControlText(box.Node)
		if isButtonInputNode(box.Node) {
			textWidth := measureTextWidth(face, text.Text, fontSize, box.Style.LetterSpacing.LengthPx, ctx)
			return textWidth + paddingWidth + borderWidth, lineHeight + paddingHeight + borderHeight
		}
		return 0, lineHeight + paddingHeight + borderHeight
	case "select":
		textWidth := intrinsicSelectTextWidth(box.Node, face, fontSize, box.Style.LetterSpacing.LengthPx, ctx)
		return textWidth + paddingWidth + borderWidth + 12, lineHeight + paddingHeight + borderHeight
	case "textarea":
		cols := controlIntAttr(box.Node, "cols", 20)
		rows := controlIntAttr(box.Node, "rows", 2)
		chWidth := measureTextWidth(face, "0", fontSize, box.Style.LetterSpacing.LengthPx, ctx)
		if chWidth <= 0 {
			chWidth = fontSize * 0.6
		}
		return float64(cols)*chWidth + paddingWidth + borderWidth, float64(rows)*lineHeight + paddingHeight + borderHeight
	default:
		return 0, 0
	}
}

func intrinsicSelectTextWidth(node *dom.Node, face *font.Face, fontSize, letterSpacing float64, ctx *layoutContext) float64 {
	width := 0.0
	for _, child := range node.Children {
		if child == nil || child.Type != dom.ElementNode {
			continue
		}
		if child.Data == "option" {
			width = max(width, measureTextWidth(face, strings.TrimSpace(descendantText(child)), fontSize, letterSpacing, ctx))
		} else {
			width = max(width, intrinsicSelectTextWidth(child, face, fontSize, letterSpacing, ctx))
		}
	}
	return width
}

func intrinsicControlImageMetrics(node *dom.Node, styles style.Map, ctx *layoutContext) (width, height float64, count int) {
	if node == nil {
		return 0, 0, 0
	}
	for _, child := range node.Children {
		if child == nil || child.Type != dom.ElementNode {
			continue
		}
		computed, ok := styles.Get(child)
		if !ok {
			untitled := style.ComputedStyle{}
			computed = &untitled
		}
		if computed.Display.Kind == style.ValueKeyword && computed.Display.Keyword == "none" {
			continue
		}
		if child.Data == "img" {
			imageWidth, imageHeight := resolveInlineImageSize(child, computed, ctx)
			if imageWidth <= 0 || imageHeight <= 0 {
				continue
			}
			width += imageWidth
			if imageHeight > height {
				height = imageHeight
			}
			count++
			continue
		}
		if child.Data == "button" || child.Data == "input" || child.Data == "select" {
			continue
		}
		childWidth, childHeight, childCount := intrinsicControlImageMetrics(child, styles, ctx)
		width += childWidth
		if childHeight > height {
			height = childHeight
		}
		count += childCount
	}
	return width, height, count
}

func controlIntAttr(node *dom.Node, name string, fallback int) int {
	if node == nil {
		return fallback
	}
	raw, ok := node.Attr(name)
	if !ok {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func scrollTextInputFragmentsIntoView(box *Box) {
	if box == nil {
		return
	}
	box.TextFragments = scrollTextInputTextFragmentsIntoView(box.Node, box.Content, box.TextFragments)
}

func scrollTextInputTextFragmentsIntoView(node *dom.Node, content Rect, fragments []TextFragment) []TextFragment {
	if !isTextInputNode(node) || len(fragments) == 0 || content.Width <= 0 || !inputHasValue(node) {
		return fragments
	}
	caret := inputCaretIndex(node, textFragmentRuneCount(fragments))
	caretX, ok := CaretXForTextFragments(fragments, caret)
	if !ok {
		return fragments
	}
	visibleLeft := content.X
	visibleRight := content.X + content.Width - 1
	padding := textInputCaretScrollPadding
	if maxPadding := content.Width / 4; padding > maxPadding {
		padding = maxPadding
	}
	shift := 0.0
	if caretX > visibleRight {
		shift = caretX - (visibleRight - padding)
	} else if caretX < visibleLeft {
		shift = caretX - (visibleLeft + padding)
	}
	if shift == 0 {
		return fragments
	}
	for i := range fragments {
		fragments[i].Rect.X -= shift
		for j := range fragments[i].Glyphs {
			fragments[i].Glyphs[j].X -= shift
		}
	}
	return fragments
}

func inputHasValue(node *dom.Node) bool {
	if node == nil {
		return false
	}
	value, ok := node.Attr("value")
	return ok && value != ""
}

func inputCaretIndex(node *dom.Node, fallback int) int {
	if node == nil {
		return fallback
	}
	raw, ok := node.Attr("data-caret")
	if !ok {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	if value < 0 {
		return 0
	}
	if value > fallback {
		return fallback
	}
	return value
}

func textFragmentRuneCount(fragments []TextFragment) int {
	count := 0
	for _, fragment := range fragments {
		if len(fragment.Glyphs) > 0 {
			count += len(fragment.Glyphs)
			continue
		}
		count += utf8.RuneCountInString(fragment.Text)
	}
	return count
}

// Release removed fragment references without dropping the live prefix.
func clearLayoutTail[T any](previous []T, live int) {
	if live < len(previous) {
		clear(previous[live:])
	}
}
