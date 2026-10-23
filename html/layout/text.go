package layout

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/internal/reuse"
	"github.com/diiyw/onix/resource"
)

type TextLayoutOptions struct {
	Reuse              *Box
	Face               *font.Face
	StyleMap           style.Map
	Width              float64
	OriginX            float64
	OriginY            float64
	ContentHeight      float64
	FlowHeight         float64
	Node               *dom.Node
	Context            *layoutContext
	InlineBoxes        map[*dom.Node]*Box
	InlineBoxFragments []inlineBoxFragment
	FlowBoxes          []*Box
	AbsoluteContaining Rect
}

type layoutContext struct {
	floatContext   *floatContext
	lineItems      []lineItem
	retained       *Cache
	uncached       int
	boldFace       *font.Face
	imageSizes     map[string]imageSizeResult
	inlineSegments []inlineSegment
	textTokens     []textToken
	controlTokens  []controlToken
	textScratch    []textLayoutScratch
	textDepth      int
	err            error
}

type textLayoutScratch struct {
	lineItems      []lineItem
	inlineSegments []inlineSegment
	textTokens     []textToken
	controlTokens  []controlToken
}

type textShapeResult struct {
	glyphs []shapedGlyph
	width  float64
	bytes  int
}

type imageSizeResult struct {
	width  float64
	height float64
	valid  bool
}

func newLayoutContext() *layoutContext {
	return &layoutContext{}
}

var layoutContextPool = sync.Pool{New: func() any { return newLayoutContext() }}

func releaseLayoutContext(ctx *layoutContext) {
	ctx.reset()
	layoutContextPool.Put(ctx)
}

func (ctx *layoutContext) reset() {
	ctx.lineItems = reuse.Reset(ctx.lineItems, 4096)
	ctx.inlineSegments = reuse.Reset(ctx.inlineSegments, 4096)
	ctx.textTokens = reuse.Reset(ctx.textTokens, 4096)
	ctx.controlTokens = reuse.Reset(ctx.controlTokens, 4096)
	if cap(ctx.textScratch) > 32 {
		ctx.textScratch = nil
	} else {
		for index := range ctx.textScratch {
			ctx.textScratch[index].reset()
		}
	}
	if len(ctx.imageSizes) > 256 {
		ctx.imageSizes = nil
	} else {
		clear(ctx.imageSizes)
	}
	ctx.floatContext, ctx.retained, ctx.boldFace, ctx.err = nil, nil, nil, nil
	ctx.uncached, ctx.textDepth = 0, 0
}

func (scratch *textLayoutScratch) reset() {
	// Bound each cached buffer; large documents should not inflate the pool.
	const maxCapacity = 4096
	scratch.lineItems = reuse.Reset(scratch.lineItems, maxCapacity)
	scratch.inlineSegments = reuse.Reset(scratch.inlineSegments, maxCapacity)
	scratch.textTokens = reuse.Reset(scratch.textTokens, maxCapacity)
	scratch.controlTokens = reuse.Reset(scratch.controlTokens, maxCapacity)
}

// UsedLineHeight resolves the used value of line-height for a shaped face.
// The computed style stores a 1.2 fallback for `normal`, but Chromium derives
// the normal value from the selected face metrics instead of that fallback.
func UsedLineHeight(computed *style.ComputedStyle, face *font.Face) float64 {
	if computed == nil {
		return 0
	}
	fontSize := computed.FontSize.LengthPx
	if fontSize <= 0 {
		fontSize = 16
	}
	customFamily := computed.FontFamily.Kind != style.ValueKeyword || !isGenericFontFamily(computed.FontFamily.Keyword)
	if computed.LineHeightNormal && customFamily && face != nil && face.Head.UnitsPerEm > 0 {
		descent := float64(face.Hhea.Descent)
		if descent > 0 {
			descent = 0
		}
		natural := float64(face.Hhea.Ascent) - descent + float64(face.Hhea.LineGap)
		if natural > 0 {
			return natural * (fontSize / float64(face.Head.UnitsPerEm))
		}
	}
	if computed.LineHeight.Kind == style.ValueLengthPx && computed.LineHeight.LengthPx > 0 {
		return computed.LineHeight.LengthPx
	}
	return fontSize * 1.2
}

func isGenericFontFamily(keyword string) bool {
	// Onix projects use `ui` as the registered name of their UI face; it is not
	// one of the CSS generic family keywords.
	switch keyword {
	case "sans-serif", "serif", "monospace", "system-ui", "cursive", "fantasy":
		return true
	default:
		return false
	}
}

func layoutText(options TextLayoutOptions) ([]TextFragment, []ImageFragment, []ControlFragment, []inlineBoxFragment, float64, error) {
	if options.Node == nil {
		return nil, nil, nil, nil, 0, nil
	}
	if options.Context != nil {
		ctx := options.Context
		parent := textLayoutScratch{
			lineItems:      ctx.lineItems,
			inlineSegments: ctx.inlineSegments,
			textTokens:     ctx.textTokens,
			controlTokens:  ctx.controlTokens,
		}
		depth := ctx.textDepth
		if depth == len(ctx.textScratch) {
			ctx.textScratch = append(ctx.textScratch, textLayoutScratch{})
		}
		scratch := ctx.textScratch[depth]
		ctx.lineItems = scratch.lineItems
		ctx.inlineSegments = scratch.inlineSegments
		ctx.textTokens = scratch.textTokens
		ctx.controlTokens = scratch.controlTokens
		ctx.textDepth++
		defer func() {
			ctx.textDepth--
			ctx.textScratch[depth] = textLayoutScratch{
				lineItems:      ctx.lineItems,
				inlineSegments: ctx.inlineSegments,
				textTokens:     ctx.textTokens,
				controlTokens:  ctx.controlTokens,
			}
			ctx.lineItems = parent.lineItems
			ctx.inlineSegments = parent.inlineSegments
			ctx.textTokens = parent.textTokens
			ctx.controlTokens = parent.controlTokens
		}()
	}

	containerStyle, _ := options.StyleMap.Get(options.Node)
	firstLineIndent := resolveLength(containerStyle.TextIndent, options.Width)
	if firstLineIndent < 0 {
		firstLineIndent = 0
	}

	segments := collectInlineSegmentsWithIndex(options.Node, options.StyleMap, options.Context, options.InlineBoxes)
	if len(segments) == 0 && len(options.FlowBoxes) == 0 {
		clear(options.InlineBoxFragments)
		return nil, nil, nil, nil, 0, nil
	}

	line := 0
	penY := options.OriginY
	maxY := options.OriginY
	var fragments []TextFragment
	var imageFragments []ImageFragment
	var controlFragments []ControlFragment
	if options.Reuse != nil {
		fragments = options.Reuse.TextFragments[:0]
		imageFragments = options.Reuse.ImageFragments[:0]
		controlFragments = options.Reuse.ControlFragments[:0]
	}
	inlineBoxFragments := options.InlineBoxFragments[:0]
	prevEndedWithSpace := true
	currentWidth := firstLineIndent
	flowBoxIndex := 0
	previousMarginBottom := 0.0
	lineX, lineWidth := options.OriginX, options.Width
	lineHeightBasis := maxFloat(UsedLineHeight(containerStyle, options.Face), 1)
	updateLineBounds := func(height float64) {
		if options.Context != nil {
			lineX, lineWidth, _ = options.Context.floatContext.bounds(options.OriginX, penY, options.Width, height)
		}
	}
	updateLineBounds(lineHeightBasis)
	var currentLine []lineItem
	lineItemsUsed := 0
	trimStart, trimEnd := textBoxTrimming(containerStyle, fontForStyle(options.Face, containerStyle, options.Context))
	lastLine := false
	if options.Context != nil {
		currentLine = options.Context.lineItems[:0]
		defer func() {
			// A pooled buffer can be much larger than this paragraph. Include
			// flushed lines, but leave its already-zero spare capacity alone.
			clear(currentLine[:max(lineItemsUsed, len(currentLine))])
			options.Context.lineItems = currentLine[:0]
		}()
	}

	flushLine := func() {
		if len(currentLine) == 0 {
			return
		}
		// text-overflow: ellipsis truncates nowrap content that exceeds the
		// line box (CSS Overflow 3 §7), matching Chromium's single-line behavior.
		if containerStyle.TextOverflow.Keyword == "ellipsis" && currentWidth > lineWidth+0.01 {
			lineItemsUsed = max(lineItemsUsed, len(currentLine))
			for index := len(currentLine) - 1; index >= 0; index-- {
				item := &currentLine[index]
				if item.Kind != inlineSegmentText {
					break
				}
				start := currentWidth - item.Width
				if start >= lineWidth && index > 0 {
					currentWidth = start
					currentLine = currentLine[:index]
					continue
				}
				face := fontForStyle(options.Face, item.Style, options.Context)
				ellipsis, ellipsisWidth, err := shapeToken(face, textToken{Text: "…"}, item.Style.FontSize.LengthPx, item.Style.LetterSpacing.LengthPx, options.Context)
				if err != nil || face == nil {
					break
				}
				available := lineWidth - start - ellipsisWidth
				if available < 0 && index > 0 {
					currentWidth = start
					currentLine = currentLine[:index]
					continue
				}
				fit := 0
				if len(item.Glyphs) != 0 && (index == 0 || item.Glyphs[0].Advance <= available+0.01) {
					fit = fittingGlyphCount(item.Glyphs, maxFloat(available, 0))
				}
				item.Glyphs = item.Glyphs[:fit]
				item.Ellipsis = ellipsis
				item.EllipsisX = glyphRunWidth(item.Glyphs)
				item.Width = item.EllipsisX + ellipsisWidth
				currentWidth = start + item.Width
				break
			}
		}
		baseline := 0.0
		lineHeight := 0.0
		for _, item := range currentLine {
			baseline = math.Max(baseline, item.Ascent)
			lineHeight = math.Max(lineHeight, item.Ascent+item.Descent)
		}
		updateLineBounds(lineHeight)
		startTrim, endTrim := 0.0, 0.0
		if line == 0 {
			startTrim = trimStart
		}
		if lastLine {
			endTrim = trimEnd
		}
		baseline -= startTrim
		cursorX := lineX
		switch containerStyle.TextAlign.Keyword {
		case "center":
			cursorX += (lineWidth - currentWidth) / 2
		case "right", "end":
			cursorX += lineWidth - currentWidth
		}
		if line == 0 && firstLineIndent > 0 {
			cursorX += firstLineIndent
		}
		for _, item := range currentLine {
			top := penY + baseline - item.Ascent
			switch item.Kind {
			case inlineSegmentBox:
				inlineBoxFragments = append(inlineBoxFragments, inlineBoxFragment{
					Box:  item.Box,
					Rect: Rect{X: cursorX, Y: top, Width: item.Width, Height: item.Height},
				})
			case inlineSegmentImage:
				imageFragments = append(imageFragments, ImageFragment{
					Node:  item.Node,
					Style: item.Style,
					Line:  line,
					Rect:  Rect{X: cursorX, Y: top, Width: item.Width, Height: item.Height},
				})
			case inlineSegmentText:
				fragment := TextFragment{
					Node:  item.Node,
					Text:  item.Text,
					Style: item.Style,
					Line:  line,
					Rect:  Rect{X: cursorX, Y: top, Width: item.Width, Height: item.Ascent + item.Descent},
				}
				if len(fragments) < cap(fragments) {
					previous := fragments[:cap(fragments)][len(fragments)]
					fragment.Glyphs = previous.Glyphs[:0]
					if len(item.Ellipsis) != 0 {
						fragment.Text = ellipsisRunText(item.Glyphs, previous.Text)
					}
				} else if len(item.Ellipsis) != 0 {
					fragment.Text = ellipsisRunText(item.Glyphs, "")
				}
				for _, glyph := range item.Glyphs {
					fragment.Glyphs = append(fragment.Glyphs, GlyphFragment{
						Rune:    glyph.Rune,
						X:       cursorX + glyph.X,
						Y:       top + item.Ascent,
						Advance: glyph.Advance,
					})
				}
				if len(item.Ellipsis) != 0 {
					for _, glyph := range item.Ellipsis {
						fragment.Glyphs = append(fragment.Glyphs, GlyphFragment{
							Rune: glyph.Rune, X: cursorX + item.EllipsisX + glyph.X,
							Y: top + item.Ascent, Advance: glyph.Advance,
						})
					}
				}
				fragments = append(fragments, fragment)
			case inlineSegmentControl:
				borderBox := Rect{
					X:      cursorX + item.Margin.Left,
					Y:      top + item.Margin.Top,
					Width:  maxFloat(0, item.Width-item.Margin.Left-item.Margin.Right),
					Height: maxFloat(0, item.Height-item.Margin.Top-item.Margin.Bottom),
				}
				contentRect := Rect{
					X:      borderBox.X + item.Border.Left + item.Padding.Left,
					Y:      borderBox.Y + item.Border.Top + item.Padding.Top,
					Width:  maxFloat(0, borderBox.Width-item.Border.Left-item.Border.Right-item.Padding.Left-item.Padding.Right),
					Height: maxFloat(0, borderBox.Height-item.Border.Top-item.Border.Bottom-item.Padding.Top-item.Padding.Bottom),
				}
				var reusedText []TextFragment
				if len(controlFragments) < cap(controlFragments) {
					reusedText = controlFragments[:cap(controlFragments)][len(controlFragments)].TextFragments
				}
				textFragments := layoutControlTextFragments(item.Node, item.Style, contentRect, line, options.Face, options.Context, options.StyleMap, reusedText)
				controlFragments = append(controlFragments, ControlFragment{
					Node:           item.Node,
					Rect:           borderBox,
					Content:        contentRect,
					Padding:        item.Padding,
					Border:         item.Border,
					Style:          item.Style,
					Line:           line,
					TextFragments:  textFragments,
					ImageFragments: layoutControlImageFragments(item.Node, contentRect, line, options.StyleMap, options.Context, textFragments),
				})
			}
			cursorX += item.Width
		}
		penY += lineHeight - startTrim - endTrim
		maxY = math.Max(maxY, penY)
		line++
		currentWidth = 0
		lineItemsUsed = max(lineItemsUsed, len(currentLine))
		currentLine = currentLine[:0]
		previousMarginBottom = 0
		updateLineBounds(lineHeightBasis)
	}
	fitFloatLine := func(width, height float64) {
		updateLineBounds(height)
		if len(currentLine) != 0 || options.Context == nil {
			return
		}
		for {
			_, available, next := options.Context.floatContext.bounds(options.OriginX, penY, options.Width, height)
			if width <= available || math.IsInf(next, 1) {
				break
			}
			penY = next
		}
		updateLineBounds(height)
	}
	for _, segment := range segments {
		if segment.Node != nil {
			for child := nextTextFlowBox(options.FlowBoxes, options.InlineBoxes, &flowBoxIndex, segment.Node.Order); child != nil; child = nextTextFlowBox(options.FlowBoxes, options.InlineBoxes, &flowBoxIndex, segment.Node.Order) {
				if !isFloated(child) {
					flushLine()
				}
				layoutTextFlowBox(child, options.OriginX, options.Width, options.FlowHeight, options.AbsoluteContaining, options.Face, options.StyleMap, options.Context, &penY, &maxY, &previousMarginBottom, currentWidth, lineHeightBasis)
				updateLineBounds(lineHeightBasis)
			}
		}
		if segment.Kind == inlineSegmentBox {
			bounds := segment.Box.FlowMarginBox
			if bounds.Width <= 0 || bounds.Height <= 0 {
				continue
			}
			fitFloatLine(bounds.Width, bounds.Height)
			if currentWidth > 0 && currentWidth+bounds.Width > lineWidth {
				flushLine()
				fitFloatLine(bounds.Width, bounds.Height)
			}
			currentLine = append(currentLine, lineItem{Kind: inlineSegmentBox, Node: segment.Node, Style: segment.Style, Box: segment.Box, Width: bounds.Width, Height: bounds.Height, Ascent: bounds.Height})
			currentWidth += bounds.Width
			prevEndedWithSpace = false
			continue
		}
		if segment.Kind == inlineSegmentImage {
			width, height := resolveInlineImageSize(segment.Node, segment.Style, options.Context)
			if width <= 0 || height <= 0 {
				continue
			}
			fitFloatLine(width, height)
			if currentWidth > 0 && currentWidth+width > lineWidth {
				flushLine()
				fitFloatLine(width, height)
			}
			currentLine = append(currentLine, lineItem{Kind: inlineSegmentImage, Node: segment.Node, Style: segment.Style, Width: width, Height: height, Ascent: height, Descent: 0})
			currentWidth += width
			prevEndedWithSpace = false
			continue
		}
		if segment.Kind == inlineSegmentControl {
			item, ok := newControlLineItem(segment.Node, segment.Style, options.Face, options.Context, options.StyleMap, options.Width)
			if !ok {
				continue
			}
			fitFloatLine(item.Width, item.Height)
			if currentWidth > 0 && currentWidth+item.Width > lineWidth {
				flushLine()
				fitFloatLine(item.Width, item.Height)
			}
			currentLine = append(currentLine, item)
			currentWidth += item.Width
			prevEndedWithSpace = false
			continue
		}

		face := fontForStyle(options.Face, segment.Style, options.Context)
		if face == nil {
			continue
		}
		fontSize := segment.Style.FontSize.LengthPx
		if fontSize <= 0 {
			fontSize = 16
		}
		segmentLineHeight := UsedLineHeight(segment.Style, face)
		fontAscent := float64(face.Hhea.Ascent) * (fontSize / float64(face.Head.UnitsPerEm))
		fontDescent := math.Abs(float64(face.Hhea.Descent)) * (fontSize / float64(face.Head.UnitsPerEm))
		// CSS line boxes retain half-leading even when the requested line-height
		// is smaller than the font's natural metrics.
		leading := segmentLineHeight - (fontAscent + fontDescent)
		ascent := fontAscent + leading/2
		descent := fontDescent + leading/2
		if segment.Node != nil && segment.Node.Type == dom.ElementNode && segment.Node.Data == "select" {
			// A select is a single-line control even when its current value
			// contains CJK text. Keep it as one visual so the widget state sync
			// cannot duplicate the full label across rune fragments.
			glyphs, width, err := shapeToken(face, textToken{Text: segment.Text}, fontSize, segment.Style.LetterSpacing.LengthPx, options.Context)
			if err != nil {
				return nil, nil, nil, nil, 0, err
			}
			if width == 0 {
				continue
			}
			currentLine = append(currentLine, lineItem{Kind: inlineSegmentText, Node: segment.Node, Style: segment.Style, Text: segment.Text, Glyphs: glyphs, Width: width, Height: segmentLineHeight, Ascent: ascent, Descent: descent})
			currentWidth += width
			prevEndedWithSpace = false
			continue
		}
		mode := whiteSpaceMode(segment.Style, segment.PreserveWhitespace)
		breakAll := segment.Style.WordBreak.Keyword == "break-all"
		breakWord := segment.Style.OverflowWrap.Keyword == "break-word" || segment.Style.OverflowWrap.Keyword == "anywhere"
		tokens := tokenizeStyledTextInto(options.Context, segment.Text, mode, prevEndedWithSpace, breakAll)
		allowsWrap := whiteSpaceAllowsWrap(mode)
		for _, token := range tokens {
			if token.IsBreak {
				if len(currentLine) == 0 {
					penY += segmentLineHeight
					maxY = math.Max(maxY, penY)
					line++
					currentWidth = 0
					prevEndedWithSpace = true
					previousMarginBottom = 0
					continue
				}
				flushLine()
				prevEndedWithSpace = true
				continue
			}
			if token.IsTab {
				_, spaceWidth, err := shapeToken(face, textToken{Text: " "}, fontSize, segment.Style.LetterSpacing.LengthPx, options.Context)
				if err != nil {
					return nil, nil, nil, nil, 0, err
				}
				interval := tabStopWidth(segment.Style.TabSize, spaceWidth)
				width := tabAdvance(currentWidth, interval)
				if allowsWrap && currentWidth > 0 && currentWidth+width > lineWidth+0.01 {
					flushLine()
					width = tabAdvance(currentWidth, interval)
				}
				if width > 0 {
					currentLine = append(currentLine, lineItem{
						Kind: inlineSegmentText, Node: segment.Node, Style: segment.Style, Width: width,
						Height: segmentLineHeight, Ascent: ascent, Descent: descent,
					})
					currentWidth += width
				}
				prevEndedWithSpace = true
				continue
			}
			glyphs, width, err := shapeToken(face, token, fontSize, segment.Style.LetterSpacing.LengthPx, options.Context)
			if err != nil {
				return nil, nil, nil, nil, 0, err
			}
			if width == 0 {
				prevEndedWithSpace = token.IsSpace
				continue
			}
			if allowsWrap {
				fitFloatLine(width, segmentLineHeight)
			}
			if allowsWrap && currentWidth > 0 && currentWidth+width > lineWidth+0.01 && !token.IsSpace {
				flushLine()
				fitFloatLine(width, segmentLineHeight)
			}
			// overflow-wrap: break-word — hard-break tokens that cannot fit a
			// line of their own (CSS Text §5.5).
			if allowsWrap && breakWord && !token.IsSpace && width > lineWidth+0.01 && len(glyphs) > 1 {
				for len(glyphs) > 0 {
					if currentWidth > 0 && currentWidth+glyphRunWidth(glyphs) > lineWidth+0.01 {
						flushLine()
						continue
					}
					fit := fittingGlyphCount(glyphs, lineWidth-currentWidth)
					if fit <= 0 {
						fit = 1
					}
					run := glyphs[:fit]
					shift := run[0].X
					runGlyphs := make([]shapedGlyph, len(run))
					for index, glyph := range run {
						runGlyphs[index] = glyph
						runGlyphs[index].X -= shift
					}
					currentLine = append(currentLine, lineItem{Kind: inlineSegmentText, Node: segment.Node, Style: segment.Style, Text: glyphRunText(run), Glyphs: runGlyphs, Width: glyphRunWidth(run), Height: segmentLineHeight, Ascent: ascent, Descent: descent})
					currentWidth += glyphRunWidth(run)
					glyphs = glyphs[fit:]
					if len(glyphs) > 0 {
						flushLine()
					}
				}
				prevEndedWithSpace = false
				continue
			}
			currentLine = append(currentLine, lineItem{Kind: inlineSegmentText, Node: segment.Node, Style: segment.Style, Text: token.Text, Glyphs: glyphs, Width: width, Height: segmentLineHeight, Ascent: ascent, Descent: descent})
			currentWidth += width
			prevEndedWithSpace = token.IsSpace
		}
	}
	lastLine = true
	flushLine()
	for child := nextTextFlowBox(options.FlowBoxes, options.InlineBoxes, &flowBoxIndex, ^uint32(0)); child != nil; child = nextTextFlowBox(options.FlowBoxes, options.InlineBoxes, &flowBoxIndex, ^uint32(0)) {
		layoutTextFlowBox(child, options.OriginX, options.Width, options.FlowHeight, options.AbsoluteContaining, options.Face, options.StyleMap, options.Context, &penY, &maxY, &previousMarginBottom, 0, lineHeightBasis)
	}
	centerControlText := options.Node.Data == "button" || options.Node.Data == "input" || options.Node.Data == "select"
	if centerControlText && len(fragments) > 0 && options.ContentHeight > 0 {
		lineHeight := penY - options.OriginY
		// Center single-line controls even when the font's line box overflows
		// the content box or the control has block display.
		offset := (options.ContentHeight - lineHeight) * 0.5
		for index := range fragments {
			fragments[index].Rect.Y += offset
			for glyph := range fragments[index].Glyphs {
				fragments[index].Glyphs[glyph].Y += offset
			}
		}
	}

	if len(fragments) == 0 {
		if len(imageFragments) == 0 && len(controlFragments) == 0 && len(inlineBoxFragments) == 0 && maxY == options.OriginY {
			clear(options.InlineBoxFragments)
			return nil, nil, nil, nil, 0, nil
		}
	}
	if maxY == options.OriginY {
		if len(fragments) > 0 {
			last := fragments[len(fragments)-1]
			maxY = last.Rect.Y + last.Rect.Height
		} else if len(controlFragments) > 0 {
			last := controlFragments[len(controlFragments)-1]
			maxY = last.Rect.Y + last.Rect.Height
		} else if len(imageFragments) > 0 {
			last := imageFragments[len(imageFragments)-1]
			maxY = last.Rect.Y + last.Rect.Height
		} else {
			last := inlineBoxFragments[len(inlineBoxFragments)-1]
			maxY = last.Rect.Y + last.Rect.Height
		}
	}
	clearLayoutTail(options.InlineBoxFragments, len(inlineBoxFragments))
	return fragments, imageFragments, controlFragments, inlineBoxFragments, maxY - options.OriginY, nil
}

func nextTextFlowBox(boxes []*Box, inlineBoxes map[*dom.Node]*Box, index *int, before uint32) *Box {
	for *index < len(boxes) {
		child := boxes[*index]
		if child == nil || inlineBoxForNode(inlineBoxes, child.Node) != nil || isAbsolutePositioned(child) {
			*index++
			continue
		}
		if child.Node != nil && child.Node.Order >= before {
			return nil
		}
		*index++
		return child
	}
	return nil
}

func layoutTextFlowBox(child *Box, originX, width, flowHeight float64, absoluteContaining Rect, face *font.Face, styles style.Map, ctx *layoutContext, penY, maxY, previousMarginBottom *float64, inlineWidth, inlineHeight float64) {
	if isFloated(child) && ctx != nil && ctx.floatContext != nil {
		layoutFloat(child, originX, width, *penY, flowHeight, inlineWidth, inlineHeight, absoluteContaining, face, styles, ctx)
		return
	}
	if ctx != nil {
		cleared := ctx.floatContext.clearance(*penY, child.Style.Clear.Keyword)
		if cleared != *penY {
			*previousMarginBottom = 0
			*penY = cleared
		}
		if ctx.floatContext != nil && len(ctx.floatContext.areas) > 0 {
			ctx.uncached++
			defer func() { ctx.uncached-- }()
		}
	}
	childMarginTop := resolveLength(child.Style.Margin.Top, width)
	if child.Style.Margin.Top.Kind == style.ValueAuto {
		childMarginTop = 0
	}
	collapse := min(*previousMarginBottom, childMarginTop)
	containing := Rect{X: originX, Y: *penY - collapse, Width: width, Height: flowHeight}
	if ctx != nil && ctx.floatContext != nil && len(ctx.floatContext.areas) > 0 && establishesFloatContext(child) {
		minimum, _ := tableCellIntrinsicWidths(child, face, styles, ctx)
		if style.IsLength(child.Style.Width) {
			minimum = borderBoxDimension(child.Style.Width, width, computedHorizontalBoxExtras(child.Style, width), isBorderBox(child.Style))
		}
		for {
			left, available, next := ctx.floatContext.bounds(originX, containing.Y, width, maxFloat(inlineHeight, 1))
			if minimum <= available || math.IsInf(next, 1) {
				containing.X, containing.Width = left, available
				break
			}
			containing.Y = next
		}
	}
	layoutBlock(child, containing, absoluteContaining, face, styles, ctx)
	*penY = child.FlowMarginBox.Y + child.FlowMarginBox.Height
	*maxY = math.Max(*maxY, *penY)
	*previousMarginBottom = child.Margin.Bottom
}

type inlineBoxFragment struct {
	Box  *Box
	Rect Rect
}

type lineItem struct {
	Kind      inlineSegmentKind
	Node      *dom.Node
	Style     *style.ComputedStyle
	Box       *Box
	Text      string
	Glyphs    []shapedGlyph
	Ellipsis  []shapedGlyph // Read-only shaped cache entry, emitted after the prefix.
	EllipsisX float64
	Width     float64
	Height    float64
	Ascent    float64
	Descent   float64
	Margin    Edges
	Padding   Edges
	Border    Edges
}

type inlineSegmentKind int

const (
	inlineSegmentText inlineSegmentKind = iota
	inlineSegmentImage
	inlineSegmentControl
	inlineSegmentBox
)

type inlineSegment struct {
	Node               *dom.Node
	Style              *style.ComputedStyle
	Box                *Box
	Kind               inlineSegmentKind
	Text               string
	PreserveWhitespace bool
}

func collectInlineSegments(node *dom.Node, styles style.Map) []inlineSegment {
	var segments []inlineSegment
	appendInlineSegments(&segments, node, styles, nil)
	return segments
}

func collectInlineSegmentsWithContext(node *dom.Node, styles style.Map, ctx *layoutContext, inlineBoxes []*Box) []inlineSegment {
	return collectInlineSegmentsWithIndex(node, styles, ctx, inlineBoxIndexFor(inlineBoxes))
}

func collectInlineSegmentsWithIndex(node *dom.Node, styles style.Map, ctx *layoutContext, index map[*dom.Node]*Box) []inlineSegment {
	if ctx == nil {
		var segments []inlineSegment
		appendInlineRootSegments(&segments, node, styles, index)
		return segments
	}
	ctx.inlineSegments = ctx.inlineSegments[:0]
	appendInlineRootSegments(&ctx.inlineSegments, node, styles, index)
	return ctx.inlineSegments
}

func appendInlineRootSegments(segments *[]inlineSegment, node *dom.Node, styles style.Map, inlineBoxes map[*dom.Node]*Box) {
	// Buttons contain styled flow content; unlike input/select labels their
	// descendants retain independent paint styles, including visibility.
	if node != nil && node.Type == dom.ElementNode && node.Data == "button" {
		for _, child := range node.Children {
			collectSegmentsInto(segments, child, styles, inlineBoxes)
		}
		return
	}
	computed, ok := styles.Get(node)
	if !ok || !shouldInlineRenderControl(node, computed) {
		appendInlineSegments(segments, node, styles, inlineBoxes)
		return
	}
	control, ok := syntheticControlText(node)
	if !ok {
		return
	}
	if control.IsPlaceholder {
		placeholder := *computed
		placeholder.Color = placeholder.PlaceholderColor
		computed = &placeholder
	}
	*segments = append(*segments, inlineSegment{Node: node, Style: computed, Kind: inlineSegmentText, Text: control.Text, PreserveWhitespace: control.PreserveWhitespace})
}

// inlineBoxIndexFor maps nodes to their atomic inline boxes; a nil index (no
// boxes) supports the same lookups as an empty one.
func inlineBoxIndexFor(boxes []*Box) map[*dom.Node]*Box {
	if len(boxes) == 0 {
		return nil
	}
	index := make(map[*dom.Node]*Box, len(boxes))
	for _, box := range boxes {
		index[box.Node] = box
	}
	return index
}

func appendInlineSegments(segments *[]inlineSegment, node *dom.Node, styles style.Map, inlineBoxes map[*dom.Node]*Box) {
	if node == nil {
		return
	}
	if computed, ok := styles.Get(node); ok {
		if isDisplayNone(computed) {
			return
		}
		if computed.Display.Kind == style.ValueKeyword && computed.Display.Keyword == "contents" {
			for _, child := range node.Children {
				appendInlineSegments(segments, child, styles, inlineBoxes)
			}
			return
		}
	}
	if box := inlineBoxForNode(inlineBoxes, node); box != nil {
		computed, _ := styles.Get(node)
		*segments = append(*segments, inlineSegment{Node: node, Style: computed, Box: box, Kind: inlineSegmentBox})
		return
	}
	if node.Type == dom.TextNode {
		computed, ok := styles.Get(node)
		if ok {
			*segments = append(*segments, inlineSegment{Node: node, Style: computed, Kind: inlineSegmentText, Text: node.Data})
		}
		return
	}
	if computed, ok := styles.Get(node); ok && shouldInlineRenderControl(node, computed) {
		*segments = append(*segments, inlineSegment{Node: node, Style: computed, Kind: inlineSegmentControl})
		return
	}
	if control, ok := syntheticControlText(node); ok {
		computed, hasStyle := styles.Get(node)
		if !hasStyle {
			return
		}
		if control.IsPlaceholder {
			placeholder := *computed
			placeholder.Color = placeholder.PlaceholderColor
			computed = &placeholder
		}
		*segments = append(*segments, inlineSegment{Node: node, Style: computed, Kind: inlineSegmentText, Text: control.Text, PreserveWhitespace: control.PreserveWhitespace})
		return
	}
	for _, child := range node.Children {
		collectSegmentsInto(segments, child, styles, inlineBoxes)
	}
}

func collectSegmentsInto(segments *[]inlineSegment, node *dom.Node, styles style.Map, inlineBoxes map[*dom.Node]*Box) {
	if node == nil {
		return
	}
	computed, ok := styles.Get(node)
	if !ok {
		return
	}
	if isDisplayNone(computed) {
		return
	}
	if computed.Display.Kind == style.ValueKeyword && computed.Display.Keyword == "contents" {
		for _, child := range node.Children {
			collectSegmentsInto(segments, child, styles, inlineBoxes)
		}
		return
	}
	if box := inlineBoxForNode(inlineBoxes, node); box != nil {
		*segments = append(*segments, inlineSegment{Node: node, Style: computed, Box: box, Kind: inlineSegmentBox})
		return
	}
	if node.Type == dom.TextNode {
		*segments = append(*segments, inlineSegment{Node: node, Style: computed, Kind: inlineSegmentText, Text: node.Data})
		return
	}
	if node.Type == dom.ElementNode && node.Data == "img" && !isLayoutContainerDisplay(computed.Display.Keyword) {
		*segments = append(*segments, inlineSegment{Node: node, Style: computed, Kind: inlineSegmentImage})
		return
	}
	if shouldInlineRenderControl(node, computed) {
		*segments = append(*segments, inlineSegment{Node: node, Style: computed, Kind: inlineSegmentControl})
		return
	}
	if node.Type == dom.ElementNode && isLayoutContainerDisplay(computed.Display.Keyword) {
		return
	}
	for _, child := range node.Children {
		collectSegmentsInto(segments, child, styles, inlineBoxes)
	}
}

func inlineBoxForNode(boxes map[*dom.Node]*Box, node *dom.Node) *Box {
	return boxes[node]
}

type controlText struct {
	Text               string
	IsPlaceholder      bool
	PreserveWhitespace bool
}

func syntheticControlText(node *dom.Node) (controlText, bool) {
	if node == nil || node.Type != dom.ElementNode {
		return controlText{}, false
	}

	switch node.Data {
	case "button":
		return controlText{Text: strings.TrimSpace(descendantText(node))}, true
	case "input":
		if isCheckboxInputNode(node) {
			if _, checked := node.Attr("checked"); checked {
				return controlText{Text: "✓"}, true
			}
			return controlText{}, true
		}
		if isRadioInputNode(node) {
			return controlText{}, true
		}
		if isHiddenInputNode(node) {
			return controlText{}, true
		}
		if isButtonInputNode(node) {
			if value, ok := node.Attr("value"); ok && strings.TrimSpace(value) != "" {
				return controlText{Text: value}, true
			}
			if isInputTypeNode(node, "submit") {
				return controlText{Text: "Submit"}, true
			}
			if isInputTypeNode(node, "reset") {
				return controlText{Text: "Reset"}, true
			}
			return controlText{}, true
		}
		if isPasswordInputNode(node) {
			if value, ok := node.Attr("value"); ok && value != "" {
				return controlText{Text: strings.Repeat("•", utf8.RuneCountInString(value))}, true
			}
			if placeholder, ok := node.Attr("placeholder"); ok && placeholder != "" {
				return controlText{Text: placeholder, IsPlaceholder: true}, true
			}
			return controlText{}, true
		}
		if value, ok := node.Attr("value"); ok && value != "" {
			return controlText{Text: value, PreserveWhitespace: true}, true
		}
		if placeholder, ok := node.Attr("placeholder"); ok && placeholder != "" {
			return controlText{Text: placeholder, IsPlaceholder: true}, true
		}
		return controlText{}, true
	case "select":
		return controlText{Text: selectedOptionText(node)}, true
	case "textarea":
		text := descendantText(node)
		if text != "" {
			return controlText{Text: text, PreserveWhitespace: true}, true
		}
		if placeholder, ok := node.Attr("placeholder"); ok && placeholder != "" {
			return controlText{Text: placeholder, IsPlaceholder: true, PreserveWhitespace: true}, true
		}
		return controlText{}, true
	default:
		return controlText{}, false
	}
}

func isControlElementNode(node *dom.Node) bool {
	if node == nil || node.Type != dom.ElementNode {
		return false
	}
	switch node.Data {
	case "button", "input", "select", "textarea":
		return true
	default:
		return false
	}
}

func layoutControlImageFragments(node *dom.Node, content Rect, line int, styles style.Map, ctx *layoutContext, textFragments []TextFragment) []ImageFragment {
	if node == nil || content.Width <= 0 || content.Height <= 0 {
		return nil
	}
	images := appendControlImageFragments(nil, node, styles, ctx)
	computed, _ := styles.Get(node)
	isFlex := computed.Display.Kind == style.ValueKeyword && isFlexDisplay(computed.Display.Keyword)
	if len(images) == 0 && !isFlex {
		return nil
	}
	gap := 0.0
	if isFlex {
		gap = resolveLength(computed.ColumnGap, content.Width)
		if gap < 0 {
			gap = 0
		}
	}
	totalTextWidth := 0.0
	for _, fragment := range textFragments {
		totalTextWidth += fragment.Rect.Width
	}
	totalWidth := totalTextWidth
	if len(images) > 0 {
		if len(images) > 1 {
			totalWidth += gap * float64(len(images)-1)
		}
		if totalTextWidth > 0 {
			totalWidth += gap
		}
		for _, image := range images {
			totalWidth += image.Rect.Width
		}
	}
	maxHeight := 0.0
	for _, image := range images {
		if image.Rect.Height > maxHeight {
			maxHeight = image.Rect.Height
		}
	}
	startX := content.X
	if isFlex {
		switch computed.JustifyContent.Keyword {
		case "center":
			startX += maxFloat(content.Width-totalWidth, 0) * .5
		case "flex-end":
			startX += maxFloat(content.Width-totalWidth, 0)
		}
	} else if len(textFragments) > 0 {
		startX = textFragments[0].Rect.X - gap
		for _, image := range images {
			startX -= image.Rect.Width
		}
		if startX < content.X {
			startX = content.X
		}
	}
	startY := content.Y + maxFloat(content.Height-maxHeight, 0)*.5
	textX := startX
	for index := range images {
		images[index].Rect.X = startX
		images[index].Rect.Y = startY + (maxHeight-images[index].Rect.Height)*.5
		images[index].Line = line
		startX += images[index].Rect.Width + gap
	}
	if isFlex && len(textFragments) > 0 {
		textX = startX
		if len(images) == 0 {
			textX = startX
		}
		shiftTextFragments(textFragments, textX-textFragments[0].Rect.X)
	}
	return images
}

func shiftTextFragments(fragments []TextFragment, dx float64) {
	if dx == 0 {
		return
	}
	for index := range fragments {
		fragments[index].Rect.X += dx
		for glyphIndex := range fragments[index].Glyphs {
			fragments[index].Glyphs[glyphIndex].X += dx
		}
	}
}

func appendControlImageFragments(images []ImageFragment, node *dom.Node, styles style.Map, ctx *layoutContext) []ImageFragment {
	if node == nil {
		return images
	}
	for _, child := range node.Children {
		if child == nil || child.Type != dom.ElementNode {
			continue
		}
		computed, ok := styles.Get(child)
		if !ok || computed.Display.Kind == style.ValueKeyword && computed.Display.Keyword == "none" {
			continue
		}
		if child.Data == "img" {
			width, height := resolveInlineImageSize(child, computed, ctx)
			if width > 0 && height > 0 {
				images = append(images, ImageFragment{Node: child, Style: computed, Rect: Rect{Width: width, Height: height}})
			}
			continue
		}
		if child.Data == "button" || child.Data == "input" || child.Data == "select" {
			continue
		}
		images = appendControlImageFragments(images, child, styles, ctx)
	}
	return images
}

func shouldInlineRenderControl(node *dom.Node, computed *style.ComputedStyle) bool {
	if node == nil || node.Type != dom.ElementNode {
		return false
	}
	switch node.Data {
	case "button", "input", "select", "textarea":
		// Block-level controls are boxes; inline-level ones render as atomic
		// inline widgets (CSS 2.2 §9.2.2 inline-level content).
		return !isBlockLevelDisplay(computed.Display.Keyword)
	default:
		return false
	}
}

func isDisplayNone(computed *style.ComputedStyle) bool {
	return computed.Display.Kind == style.ValueKeyword && computed.Display.Keyword == "none"
}

func newControlLineItem(node *dom.Node, computed *style.ComputedStyle, face *font.Face, ctx *layoutContext, styles style.Map, base float64) (lineItem, bool) {
	temp := &Box{
		Node:    node,
		Style:   computed,
		Margin:  resolveEdges(computed.Margin, base),
		Padding: resolveEdges(computed.Padding, base),
		Border:  resolveBorderEdges(computed.BorderWidth, computed.BorderStyle, base),
	}
	borderBoxWidth, borderBoxHeight := intrinsicControlSize(temp, face, ctx, styles)
	if style.IsLength(computed.Width) {
		borderBoxWidth = borderBoxDimension(computed.Width, base, horizontalBoxExtras(temp.Padding, temp.Border), isBorderBox(computed))
	}
	if style.IsLength(computed.Height) {
		borderBoxHeight = borderBoxDimension(computed.Height, base, verticalBoxExtras(temp.Padding, temp.Border), isBorderBox(computed))
	}
	if borderBoxWidth <= 0 || borderBoxHeight <= 0 {
		return lineItem{}, false
	}
	outerWidth := temp.Margin.Left + borderBoxWidth + temp.Margin.Right
	outerHeight := temp.Margin.Top + borderBoxHeight + temp.Margin.Bottom
	return lineItem{
		Kind:    inlineSegmentControl,
		Node:    node,
		Style:   computed,
		Width:   outerWidth,
		Height:  outerHeight,
		Ascent:  temp.Margin.Top + borderBoxHeight,
		Descent: temp.Margin.Bottom,
		Margin:  temp.Margin,
		Padding: temp.Padding,
		Border:  temp.Border,
	}, true
}

type controlToken struct {
	text   string
	glyphs []shapedGlyph
	width  float64
}

func layoutControlTextFragments(node *dom.Node, computed *style.ComputedStyle, content Rect, line int, face *font.Face, ctx *layoutContext, styles style.Map, reuse []TextFragment) []TextFragment {
	if node != nil && node.Data == "button" {
		retained := Box{TextFragments: reuse}
		fragments, _, _, _, _, err := layoutText(TextLayoutOptions{
			Reuse: &retained, Node: node, Face: face, Context: ctx, StyleMap: styles,
			Width: content.Width, OriginX: content.X, OriginY: content.Y, ContentHeight: content.Height,
		})
		if err != nil && ctx != nil {
			ctx.err = err
		}
		for index := range fragments {
			fragments[index].Line += line
		}
		return fragments
	}
	face = fontForStyle(face, computed, ctx)
	if face == nil || content.Width <= 0 || content.Height <= 0 {
		return nil
	}
	text, ok := syntheticControlText(node)
	if !ok || strings.TrimSpace(text.Text) == "" {
		return nil
	}
	fragmentStyle := computed
	if text.IsPlaceholder {
		placeholder := *computed
		placeholder.Color = placeholder.PlaceholderColor
		fragmentStyle = &placeholder
	}
	fontSize := fragmentStyle.FontSize.LengthPx
	if fontSize <= 0 {
		fontSize = 16
	}
	lineHeight := UsedLineHeight(fragmentStyle, face)
	fontAscent := float64(face.Hhea.Ascent) * (fontSize / float64(face.Head.UnitsPerEm))
	fontDescent := math.Abs(float64(face.Hhea.Descent)) * (fontSize / float64(face.Head.UnitsPerEm))
	leading := lineHeight - (fontAscent + fontDescent)
	ascent := fontAscent + leading/2
	descent := fontDescent + leading/2
	textTop := content.Y
	if node != nil && node.Data != "textarea" {
		textTop += (content.Height - lineHeight) / 2
	}
	var shaped []controlToken
	if ctx != nil {
		shaped = ctx.controlTokens[:0]
		defer func() {
			clear(shaped)
			ctx.controlTokens = shaped[:0]
		}()
	}
	totalWidth := 0.0
	availableWidth := content.Width
	indent := resolveLength(fragmentStyle.TextIndent, content.Width)
	if indent < 0 {
		indent = 0
	}
	if indent > availableWidth {
		indent = availableWidth
	}
	availableWidth -= indent
	if node != nil && node.Data == "select" {
		// A select displays one current value. CJK tokenization normally splits
		// each rune into a separate fragment, but keeping those fragments would
		// let the widget sync replace every one with the complete label.
		glyphs, width, err := shapeToken(face, textToken{Text: text.Text}, fontSize, fragmentStyle.LetterSpacing.LengthPx, ctx)
		if err != nil || width == 0 {
			return nil
		}
		shaped = append(shaped, controlToken{text: text.Text, glyphs: glyphs, width: width})
		totalWidth = width
	} else {
		mode := whiteSpaceMode(fragmentStyle, text.PreserveWhitespace)
		breakAll := fragmentStyle.WordBreak.Keyword == "break-all"
		tokens := tokenizeStyledTextInto(ctx, text.Text, mode, false, breakAll)
		allowsWrap := whiteSpaceAllowsWrap(mode)
		lineWidth := 0.0
		for _, token := range tokens {
			if token.IsBreak {
				if !allowsWrap {
					break
				}
				lineWidth = 0
				continue
			}
			if token.IsTab {
				_, spaceWidth, err := shapeToken(face, textToken{Text: " "}, fontSize, fragmentStyle.LetterSpacing.LengthPx, ctx)
				if err != nil {
					return nil
				}
				width := tabAdvance(lineWidth, tabStopWidth(fragmentStyle.TabSize, spaceWidth))
				if width > 0 {
					shaped = append(shaped, controlToken{width: width})
					totalWidth += width
					lineWidth += width
				}
				continue
			}
			glyphs, width, err := shapeToken(face, token, fontSize, fragmentStyle.LetterSpacing.LengthPx, ctx)
			if err != nil || width == 0 {
				continue
			}
			if allowsWrap && len(shaped) > 0 && totalWidth+width > availableWidth && !token.IsSpace {
				break
			}
			shaped = append(shaped, controlToken{text: token.Text, glyphs: glyphs, width: width})
			totalWidth += width
			lineWidth += width
		}
	}
	cursorX := content.X + indent
	if computed.TextAlign.Kind == style.ValueKeyword {
		switch computed.TextAlign.Keyword {
		case "center":
			offset := (availableWidth - totalWidth) / 2
			if offset > 0 {
				cursorX += offset
			}
		case "right":
			offset := availableWidth - totalWidth
			if offset > 0 {
				cursorX += offset
			}
		}
	}
	fragments := reuse[:0]
	for _, token := range shaped {
		fragment := TextFragment{
			Node:  node,
			Text:  token.text,
			Style: fragmentStyle,
			Line:  line,
			Rect:  Rect{X: cursorX, Y: textTop, Width: token.width, Height: ascent + descent},
		}
		if len(fragments) < cap(fragments) {
			fragment.Glyphs = fragments[:cap(fragments)][len(fragments)].Glyphs[:0]
		}
		for _, glyph := range token.glyphs {
			fragment.Glyphs = append(fragment.Glyphs, GlyphFragment{
				Rune:    glyph.Rune,
				X:       cursorX + glyph.X,
				Y:       textTop + ascent,
				Advance: glyph.Advance,
			})
		}
		fragments = append(fragments, fragment)
		cursorX += token.width
	}
	clearLayoutTail(reuse, len(fragments))
	return scrollTextInputTextFragmentsIntoView(node, content, fragments)
}

func selectedOptionText(node *dom.Node) string {
	if selected := selectedOption(node); selected != nil {
		return strings.TrimSpace(descendantText(selected))
	}
	return ""
}

// Render live selectedness, including an empty or disabled selected option.
// The DOM owns default selection and dirtyness; layout must not pick a fallback.
func selectedOption(node *dom.Node) *dom.Node {
	if node == nil {
		return nil
	}
	for _, child := range node.Children {
		if child == nil || child.Type != dom.ElementNode {
			continue
		}
		if child.Data == "option" {
			if dom.IsChecked(child) {
				return child
			}
			continue
		}
		if selected := selectedOption(child); selected != nil {
			return selected
		}
	}
	return nil
}

func descendantText(node *dom.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == dom.TextNode {
		return node.Data
	}
	var first string
	var builder strings.Builder
	appendDescendantText(&builder, &first, node)
	if builder.Len() == 0 {
		return first
	}
	return builder.String()
}

func appendDescendantText(builder *strings.Builder, first *string, node *dom.Node) {
	if node == nil {
		return
	}
	if node.Type == dom.TextNode {
		if node.Data == "" {
			return
		}
		// Borrow the first immutable text string, even through nested wrappers.
		// Allocate owned storage only when another nonempty text node appears.
		if *first == "" {
			*first = node.Data
			return
		}
		if builder.Len() == 0 {
			builder.Grow(len(*first) + len(node.Data))
			builder.WriteString(*first)
		}
		builder.WriteString(node.Data)
		return
	}
	for _, child := range node.Children {
		appendDescendantText(builder, first, child)
	}
}

func isCheckboxInputNode(node *dom.Node) bool {
	return isInputTypeNode(node, "checkbox")
}

func isRadioInputNode(node *dom.Node) bool {
	return isInputTypeNode(node, "radio")
}

func isPasswordInputNode(node *dom.Node) bool {
	return isInputTypeNode(node, "password")
}

func isButtonInputNode(node *dom.Node) bool {
	return isInputTypeNode(node, "button") || isInputTypeNode(node, "submit") || isInputTypeNode(node, "reset")
}

func isHiddenInputNode(node *dom.Node) bool {
	return isInputTypeNode(node, "hidden")
}

func isChoiceInputNode(node *dom.Node) bool {
	return isCheckboxInputNode(node) || isRadioInputNode(node)
}

func isTextInputNode(node *dom.Node) bool {
	if node == nil || node.Type != dom.ElementNode || node.Data != "input" {
		return false
	}
	return !isChoiceInputNode(node) && !isButtonInputNode(node) && !isHiddenInputNode(node)
}

func isInputTypeNode(node *dom.Node, inputType string) bool {
	return dom.HasInputType(node, inputType)
}

type textToken struct {
	Text    string
	IsSpace bool
	IsBreak bool
	IsTab   bool
}

func tokenizeText(input string, suppressLeadingSpace bool) []textToken {
	return appendNormalTextTokens(make([]textToken, 0, 4), input, 0, len(input), suppressLeadingSpace, false)
}

func tokenizePreformattedText(input string) []textToken {
	return appendPreformattedTextTokens(make([]textToken, 0, 4), input)
}

func appendPreformattedTextTokens(tokens []textToken, input string) []textToken {
	tokens = tokens[:0]
	textStart := -1
	flushText := func(end int) {
		if textStart < 0 {
			return
		}
		tokens = append(tokens, textToken{Text: input[textStart:end]})
		textStart = -1
	}
	for offset, r := range input {
		switch r {
		case '\r':
			flushText(offset)
			continue
		case '\n':
			flushText(offset)
			tokens = append(tokens, textToken{IsBreak: true})
		case '\t':
			flushText(offset)
			tokens = append(tokens, textToken{IsTab: true})
		case ' ':
			flushText(offset)
			tokens = append(tokens, textToken{Text: " ", IsSpace: true})
		default:
			if textStart < 0 {
				textStart = offset
			}
		}
	}
	flushText(len(input))
	return tokens
}

func tokenizePreLineText(input string, suppressLeadingSpace bool) []textToken {
	return appendPreLineTextTokens(make([]textToken, 0, 4), input, suppressLeadingSpace, false)
}

func appendPreLineTextTokens(tokens []textToken, input string, suppressLeadingSpace bool, breakAll bool) []textToken {
	tokens = tokens[:0]
	lineStart := 0
	for offset := 0; offset <= len(input); {
		lineEnd := offset
		for lineEnd < len(input) {
			if input[lineEnd] == '\n' || (input[lineEnd] == '\r' && lineEnd+1 < len(input) && input[lineEnd+1] == '\n') {
				break
			}
			lineEnd++
		}
		tokens = appendNormalTextTokens(tokens, input, lineStart, lineEnd, suppressLeadingSpace, breakAll)
		if lineEnd == len(input) {
			break
		}
		tokens = append(tokens, textToken{IsBreak: true})
		suppressLeadingSpace = true
		if input[lineEnd] == '\r' && lineEnd+1 < len(input) && input[lineEnd+1] == '\n' {
			offset = lineEnd + 2
		} else {
			offset = lineEnd + 1
		}
		lineStart = offset
	}
	return tokens
}

func appendNormalTextTokens(tokens []textToken, input string, start, end int, suppressLeadingSpace bool, breakAll bool) []textToken {
	spacePending := suppressLeadingSpace
	tokenStart := len(tokens)
	wordStart := -1
	flushWord := func(offset int) {
		if wordStart >= 0 {
			tokens = append(tokens, textToken{Text: input[wordStart:offset]})
			wordStart = -1
		}
	}
	for offset := start; offset < end; {
		r, size := utf8.DecodeRuneInString(input[offset:end])
		next := offset + size
		switch {
		case unicode.IsSpace(r):
			flushWord(offset)
			spacePending = true
		case breakAll || isCJK(r):
			flushWord(offset)
			if spacePending {
				if len(tokens) > tokenStart {
					tokens = append(tokens, textToken{Text: " ", IsSpace: true})
				}
				spacePending = false
			}
			tokens = append(tokens, textToken{Text: input[offset:next]})
		default:
			if spacePending {
				flushWord(offset)
				if len(tokens) > tokenStart {
					tokens = append(tokens, textToken{Text: " ", IsSpace: true})
				}
				spacePending = false
			}
			if wordStart < 0 {
				wordStart = offset
			}
		}
		offset = next
	}
	flushWord(end)
	return tokens
}

func tokenizeStyledTextInto(ctx *layoutContext, input, mode string, suppressLeadingSpace bool, breakAll bool) []textToken {
	var tokens []textToken
	if ctx != nil {
		ctx.textTokens = ctx.textTokens[:0]
		tokens = ctx.textTokens
	}
	switch mode {
	case "pre", "pre-wrap":
		result := appendPreformattedTextTokens(tokens, input)
		if ctx != nil {
			ctx.textTokens = result
		}
		return result
	case "pre-line":
		result := appendPreLineTextTokens(tokens, input, suppressLeadingSpace, breakAll)
		if ctx != nil {
			ctx.textTokens = result
		}
		return result
	default:
		result := appendNormalTextTokens(tokens, input, 0, len(input), suppressLeadingSpace, breakAll)
		if ctx != nil {
			ctx.textTokens = result
		}
		return result
	}
}

func whiteSpaceMode(computed *style.ComputedStyle, preserveWhitespace bool) string {
	if computed.WhiteSpace.Kind == style.ValueKeyword && computed.WhiteSpace.Keyword != "" {
		if preserveWhitespace && (computed.WhiteSpace.Keyword == "normal" || computed.WhiteSpace.Keyword == "nowrap") {
			return "pre"
		}
		return computed.WhiteSpace.Keyword
	}
	if preserveWhitespace {
		return "pre-wrap"
	}
	return "normal"
}

func glyphRunWidth(glyphs []shapedGlyph) float64 {
	width := 0.0
	for _, glyph := range glyphs {
		width += glyph.Advance
	}
	return width
}

func glyphRunText(glyphs []shapedGlyph) string {
	var builder strings.Builder
	for _, glyph := range glyphs {
		builder.WriteRune(glyph.Rune)
	}
	return builder.String()
}

func ellipsisRunText(glyphs []shapedGlyph, previous string) string {
	remaining := previous
	bytes := len("…")
	for _, glyph := range glyphs {
		r, size := utf8.DecodeRuneInString(remaining)
		if len(remaining) != 0 && r == glyph.Rune {
			remaining = remaining[size:]
		} else {
			remaining = ""
		}
		bytes += utf8.RuneLen(glyph.Rune)
	}
	if remaining == "…" {
		return previous
	}
	var builder strings.Builder
	builder.Grow(bytes)
	for _, glyph := range glyphs {
		builder.WriteRune(glyph.Rune)
	}
	builder.WriteRune('…')
	return builder.String()
}

// fittingGlyphCount returns how many leading glyphs fit within available
// advance space; at least one glyph is always admitted.
func fittingGlyphCount(glyphs []shapedGlyph, available float64) int {
	advance := 0.0
	count := 0
	for _, glyph := range glyphs {
		if count > 0 && advance+glyph.Advance > available+0.01 {
			break
		}
		advance += glyph.Advance
		count++
	}
	return count
}

func whiteSpaceAllowsWrap(mode string) bool {
	return mode != "nowrap" && mode != "pre"
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana)
}

type shapedGlyph struct {
	Rune    rune
	X       float64
	Advance float64
}

// Shaping depends only on font identity, token text, and font size, so the
// results are cached process-wide: layout rebuilds (typing, hover changes)
// would otherwise re-shape the same tokens every frame. Faces are identified
// by pointer, and the cache holds that pointer strongly: metric patching (for
// synthesized bold) makes content-based keys unsafe, and pinning prevents a
// closed face from aliasing a new one at the same address. Only short tokens
// are retained; cache keys clone their text so a token cannot pin a whole
// source-file backing string.
type textShapeKey struct {
	face              *font.Face
	text              string
	fontSizeBits      uint64
	letterSpacingBits uint64
}

const (
	textShapeCacheLimit      = 8192
	textShapeCacheBytesLimit = 4 << 20
	textShapeCacheTokenLimit = 512
)

var textShapeCache = struct {
	sync.Mutex
	entries map[textShapeKey]textShapeResult
	bytes   int
}{entries: make(map[textShapeKey]textShapeResult, 256)}

func shapeToken(face *font.Face, token textToken, fontSize, letterSpacing float64, ctx *layoutContext) ([]shapedGlyph, float64, error) {
	if face == nil || token.Text == "" {
		return nil, 0, nil
	}
	cacheable := len(token.Text) <= textShapeCacheTokenLimit
	var key textShapeKey
	if cacheable {
		key = textShapeKey{face: face, text: token.Text, fontSizeBits: math.Float64bits(fontSize), letterSpacingBits: math.Float64bits(letterSpacing)}
		textShapeCache.Lock()
		result, ok := textShapeCache.entries[key]
		textShapeCache.Unlock()
		if ok {
			return result.glyphs, result.width, nil
		}
	}
	advance := 0.0
	glyphs := make([]shapedGlyph, 0, utf8.RuneCountInString(token.Text))
	var previous uint16
	var previousFace *font.Face
	for _, r := range token.Text {
		glyphFace, glyphID := face.ResolveGlyph(r)
		scale := fontSize / float64(glyphFace.Head.UnitsPerEm)
		if previousFace == glyphFace {
			adjustment := float64(glyphFace.GlyphKerning(previous, glyphID)) * scale
			advance += adjustment
			glyphs[len(glyphs)-1].Advance += adjustment
		}
		previous = glyphID
		previousFace = glyphFace
		metrics, err := glyphFace.HorizontalMetrics(glyphID)
		if err != nil {
			return nil, 0, fmt.Errorf("metrics for %q: %w", r, err)
		}
		glyphAdvance := float64(metrics.AdvanceWidth) * scale
		glyphs = append(glyphs, shapedGlyph{Rune: r, X: advance + float64(metrics.LeftSideBearing)*scale, Advance: glyphAdvance})
		advance += glyphAdvance + letterSpacing
	}
	if !cacheable {
		return glyphs, advance, nil
	}
	cacheKey := key
	cacheKey.text = strings.Clone(token.Text)
	entry := textShapeResult{glyphs: glyphs, width: advance, bytes: len(cacheKey.text) + len(glyphs)*32}
	if entry.bytes > textShapeCacheBytesLimit {
		return glyphs, advance, nil
	}
	textShapeCache.Lock()
	if previous, ok := textShapeCache.entries[cacheKey]; ok {
		textShapeCache.bytes -= previous.bytes
	}
	if len(textShapeCache.entries) >= textShapeCacheLimit || textShapeCache.bytes+entry.bytes > textShapeCacheBytesLimit {
		textShapeCache.entries = make(map[textShapeKey]textShapeResult, 256)
		textShapeCache.bytes = 0
	}
	textShapeCache.entries[cacheKey] = entry
	textShapeCache.bytes += entry.bytes
	textShapeCache.Unlock()
	return glyphs, advance, nil
}

func fontForStyle(regular *font.Face, value *style.ComputedStyle, ctx *layoutContext) *font.Face {
	if ctx != nil && ctx.boldFace != nil && value.FontWeight.Kind == style.ValueNumber && value.FontWeight.Number >= 600 {
		return ctx.boldFace
	}
	return regular
}

func measureTextWidth(face *font.Face, text string, fontSize, letterSpacing float64, ctx *layoutContext) float64 {
	if face == nil || fontSize <= 0 || text == "" {
		return 0
	}
	if !strings.ContainsAny(text, "\r\n\t") {
		_, width, err := shapeToken(face, textToken{Text: text}, fontSize, letterSpacing, ctx)
		if err != nil {
			return 0
		}
		return width
	}
	width := 0.0
	for _, token := range tokenizeStyledTextInto(ctx, text, "normal", false, false) {
		_, tokenWidth, err := shapeToken(face, token, fontSize, letterSpacing, ctx)
		if err != nil {
			return 0
		}
		width += tokenWidth
	}
	return width
}

func measureIntrinsicTextWidth(face *font.Face, text string, fontSize float64, letterSpacing float64, mode string, tabSize style.Value, ctx *layoutContext) float64 {
	if mode != "pre" && mode != "pre-wrap" {
		return measureTextWidth(face, text, fontSize, letterSpacing, ctx)
	}

	maxWidth := 0.0
	lineWidth := 0.0
	for _, token := range tokenizeStyledTextInto(ctx, text, mode, false, false) {
		if token.IsBreak {
			if lineWidth > maxWidth {
				maxWidth = lineWidth
			}
			lineWidth = 0
			continue
		}
		if token.IsTab {
			_, spaceWidth, err := shapeToken(face, textToken{Text: " "}, fontSize, letterSpacing, ctx)
			if err != nil {
				return 0
			}
			lineWidth += tabAdvance(lineWidth, tabStopWidth(tabSize, spaceWidth))
			continue
		}
		_, width, err := shapeToken(face, token, fontSize, letterSpacing, ctx)
		if err != nil {
			return 0
		}
		lineWidth += width
	}
	if lineWidth > maxWidth {
		maxWidth = lineWidth
	}
	return maxWidth
}

func tabStopWidth(tabSize style.Value, spaceWidth float64) float64 {
	switch tabSize.Kind {
	case style.ValueNumber:
		return max(tabSize.Number*spaceWidth, 0)
	case style.ValueLengthPx:
		return max(tabSize.LengthPx, 0)
	case style.ValueLengthCalc:
		return max(tabSize.LengthPx, 0)
	default:
		return 8 * spaceWidth
	}
}

func tabAdvance(position, interval float64) float64 {
	if interval <= 0 {
		return 0
	}
	nextStop := math.Floor(max(position, 0)/interval+1e-12) + 1
	return max(nextStop*interval-position, 0)
}

func resolveInlineImageSize(node *dom.Node, computed *style.ComputedStyle, ctx *layoutContext) (float64, float64) {
	if node == nil || node.Type != dom.ElementNode || node.Data != "img" {
		return 0, 0
	}
	src, ok := node.Attr("src")
	if !ok || strings.TrimSpace(src) == "" {
		return 0, 0
	}
	intrinsicWidth, intrinsicHeight, ok := resourceImageSize(ctx, src)
	if !ok {
		// HTML image providers can resolve assets outside the engine resource
		// source. Explicit CSS or HTML dimensions are enough to lay out those
		// images before the provider supplies the texture.
		return resolveImageSize(node, computed, 0, 0, 0, 0)
	}
	width, height := resolveImageSize(node, computed, intrinsicWidth, intrinsicHeight, intrinsicWidth, intrinsicHeight)
	return width, height
}

func resourceImageSize(ctx *layoutContext, source string) (float64, float64, bool) {
	if ctx != nil {
		if size, ok := ctx.imageSizes[source]; ok {
			return size.width, size.height, size.valid
		}
	}
	width, height, err := resource.Size(source)
	result := imageSizeResult{width: width, height: height, valid: err == nil}
	if ctx != nil {
		if ctx.imageSizes == nil {
			ctx.imageSizes = make(map[string]imageSizeResult)
		}
		ctx.imageSizes[source] = result
	}
	return result.width, result.height, result.valid
}

func resolveImageSize(node *dom.Node, computed *style.ComputedStyle, intrinsicWidth, intrinsicHeight, widthBase, heightBase float64) (float64, float64) {
	width := resolveImageDimensionAttr(node, "width")
	height := resolveImageDimensionAttr(node, "height")
	if style.IsLength(computed.Width) {
		width = resolveLength(computed.Width, widthBase)
	}
	if style.IsLength(computed.Height) {
		height = resolveLength(computed.Height, heightBase)
	}
	if width <= 0 {
		width = intrinsicWidth
	}
	if height <= 0 {
		height = intrinsicHeight
	}
	if intrinsicWidth > 0 && intrinsicHeight > 0 {
		if style.IsLength(computed.Width) && !style.IsLength(computed.Height) {
			height = width * intrinsicHeight / intrinsicWidth
		}
		if style.IsLength(computed.Height) && !style.IsLength(computed.Width) {
			width = height * intrinsicWidth / intrinsicHeight
		}
		if width <= 0 {
			width = height * intrinsicWidth / intrinsicHeight
		}
		if height <= 0 {
			height = width * intrinsicHeight / intrinsicWidth
		}
	}
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	return width, height
}

func resolveImageDimensionAttr(node *dom.Node, name string) float64 {
	if node == nil {
		return 0
	}
	raw, ok := node.Attr(name)
	if !ok {
		return 0
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}
