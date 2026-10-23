package layout

import (
	"math"
	"slices"
	"strings"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/internal/dom"
	"github.com/diiyw/onix/internal/style"
)

type flexConfig struct {
	isRow            bool
	reverseMain      bool
	reverseCross     bool
	wrap             bool
	justifyContent   string
	alignItems       string
	alignContent     string
	mainGap          float64
	crossGap         float64
	hasDefiniteMain  bool
	hasDefiniteCross bool
	mainSize         float64
	crossSize        float64
}

type flexItem struct {
	baseSize, minMain, maxMain, violation float64
	frozen                                bool
	box                                   *Box
	mainSize                              float64
	outerMain                             float64
	outerCross                            float64
	mainExtras                            float64
	crossExtras                           float64
	grow                                  float64
	shrink                                float64
	alignSelf                             string
	measured                              bool
	measureMain                           float64
	measureCross                          float64
	measureAutoCross                      bool
}

type flexLine struct {
	items       []flexItem
	mainUsed    float64
	crossSize   float64
	autoMargins int
}

// Each container owns its scratch so recursive flex measurements never alias.
// Pointer-bearing buffers are cleared after use to release detached children.
type flexScratch struct {
	items     []flexItem
	lines     []flexLine
	positions []float64
	basis     []flexBasisMeasurement
}

// Auto basis needs only the intrinsic height, not the subtree's final geometry.
// Keep it separately: stretch/final sizing may replace that geometry before an
// outer flex container asks for the same intrinsic height again in this pass.
type flexBasisMeasurement struct {
	pass                 uint64
	containing, absolute Rect
	width, height        style.Value
	result               float64
}

func layoutFlexContainer(box *Box, layoutContaining Rect, absoluteContaining Rect, face *font.Face, styles style.Map, ctx *layoutContext) {
	if style.IsLength(box.Style.Height) {
		box.Content.Height = resolveLength(box.Style.Height, layoutContaining.Height)
		if isBorderBox(box.Style) {
			box.Content.Height -= verticalBoxExtras(box.Padding, box.Border)
		}
	}
	config := resolveFlexConfig(box)
	if box.flex == nil {
		box.flex = &flexScratch{}
	}
	scratch := box.flex
	items := slices.Grow(scratch.items[:0], len(box.Children))
	defer func() {
		clear(items)
		scratch.items = items[:0]
		clear(scratch.lines)
		scratch.lines = scratch.lines[:0]
	}()
	for _, child := range box.Children {
		if isAbsolutePositioned(child) {
			continue
		}
		shrink := maxFloat(resolveNumber(child.Style.FlexShrink), 0)
		// Preserve the intrinsic width of nowrap content so flex sizing cannot
		// compress glyphs or other unbreakable inline items into each other.
		if flexItemPreservesNoWrapIntrinsicSize(child, config) {
			shrink = 0
		}
		item := flexItem{
			box:         child,
			mainExtras:  flexMainExtras(child.Style, config, box.Content.Width),
			crossExtras: flexCrossExtras(child.Style, config, box.Content.Width),
			grow:        maxFloat(resolveNumber(child.Style.FlexGrow), 0),
			shrink:      shrink,
			alignSelf:   resolveAlignSelf(box, child),
		}
		item.mainSize = resolveFlexBaseSize(&item, box, config, absoluteContaining, face, styles, ctx)
		item.baseSize = item.mainSize
		minSize, maxSize := child.Style.MinWidth, child.Style.MaxWidth
		base, extras := box.Content.Width, computedHorizontalBoxExtras(child.Style, box.Content.Width)
		if !config.isRow {
			minSize, maxSize = child.Style.MinHeight, child.Style.MaxHeight
			base, extras = box.Content.Height, computedVerticalBoxExtras(child.Style, box.Content.Width)
		}
		item.minMain = clampContentDimension(0, minSize, maxSize, base, extras, isBorderBox(child.Style))
		item.maxMain = clampContentDimension(math.Inf(1), minSize, maxSize, base, extras, isBorderBox(child.Style))
		item.mainSize = min(max(item.baseSize, item.minMain), item.maxMain)
		item.outerMain = item.mainSize + item.mainExtras
		items = append(items, item)
	}

	lines := buildFlexLines(items, config, scratch.lines[:0])
	scratch.lines = lines
	for i := range lines {
		resolveFlexibleLine(&lines[i], config)
		measureFlexLine(&lines[i], box, config, absoluteContaining, face, styles, ctx)
	}

	mainExtent, crossExtent := flexContentExtents(lines, config)
	// An absolutely positioned flex container with both block-axis insets has
	// a definite used height even when its CSS height is auto. Preserve that
	// stretch size instead of replacing it with the intrinsic line extent.
	definiteHeight := style.IsLength(box.Style.Height) ||
		(isAbsolutePositioned(box) && style.IsLength(box.Style.Top) && style.IsLength(box.Style.Bottom))
	if config.isRow {
		if definiteHeight {
			box.Content.Height = contentDimension(box.Style.Height, layoutContaining.Height, verticalBoxExtras(box.Padding, box.Border), isBorderBox(box.Style))
			if box.Style.Height.Kind == style.ValueAuto {
				box.Content.Height = maxFloat(layoutContaining.Height-resolveLength(box.Style.Top, layoutContaining.Height)-resolveLength(box.Style.Bottom, layoutContaining.Height)-verticalBoxExtras(box.Padding, box.Border)-box.Margin.Top-box.Margin.Bottom, 0)
			}
		} else {
			box.Content.Height = crossExtent
		}
		config.crossSize = box.Content.Height
	} else {
		if definiteHeight {
			box.Content.Height = contentDimension(box.Style.Height, layoutContaining.Height, verticalBoxExtras(box.Padding, box.Border), isBorderBox(box.Style))
			if box.Style.Height.Kind == style.ValueAuto {
				box.Content.Height = maxFloat(layoutContaining.Height-resolveLength(box.Style.Top, layoutContaining.Height)-resolveLength(box.Style.Bottom, layoutContaining.Height)-verticalBoxExtras(box.Padding, box.Border)-box.Margin.Top-box.Margin.Bottom, 0)
			}
		} else {
			box.Content.Height = mainExtent
		}
		config.mainSize = box.Content.Height
	}
	box.Content.Height = clampContentDimension(box.Content.Height, box.Style.MinHeight, box.Style.MaxHeight, layoutContaining.Height, verticalBoxExtras(box.Padding, box.Border), isBorderBox(box.Style))
	if config.isRow {
		config.crossSize = box.Content.Height
	} else {
		config.mainSize = box.Content.Height
	}

	linePositions := resolveFlexLinePositions(lines, box, config, scratch.positions[:0])
	scratch.positions = linePositions
	for lineIndex := range lines {
		layoutFlexLine(lines[lineIndex], linePositions[lineIndex], box, config, absoluteContaining, face, styles, ctx)
	}

	absoluteContaining = descendantAbsoluteContaining(box, absoluteContaining)
	for _, child := range box.Children {
		if !isAbsolutePositioned(child) {
			continue
		}
		childContaining := Rect{X: box.Content.X, Y: box.Content.Y, Width: box.Content.Width, Height: box.Content.Height}
		layoutBlock(child, childContaining, absoluteContaining, face, styles, ctx)
	}

	alignBottomPositionedChildren(box, absoluteContaining, ctx)

	box.FlowMarginBox = box.MarginBox()
	if isRelativePositioned(box) {
		translateBox(box, resolveRelativeOffsetX(box.Style, layoutContaining.Width), resolveRelativeOffsetY(box.Style, layoutContaining.Height), ctx)
	}
}

func flexItemPreservesNoWrapIntrinsicSize(child *Box, config flexConfig) bool {
	if child == nil || child.Style.WhiteSpace.Kind != style.ValueKeyword || child.Style.WhiteSpace.Keyword != "nowrap" {
		return false
	}
	minimum, overflow := child.Style.MinWidth, child.Style.OverflowX
	if !config.isRow {
		minimum, overflow = child.Style.MinHeight, child.Style.OverflowY
	}
	return minimum.Kind == style.ValueAuto && overflow.Kind == style.ValueKeyword && overflow.Keyword == "visible"
}

func resolveFlexConfig(box *Box) flexConfig {
	direction := box.Style.FlexDirection.Keyword
	wrap := box.Style.FlexWrap.Keyword
	config := flexConfig{
		isRow:            direction == "row" || direction == "row-reverse",
		reverseMain:      direction == "row-reverse" || direction == "column-reverse",
		reverseCross:     wrap == "wrap-reverse",
		wrap:             wrap != "nowrap",
		justifyContent:   box.Style.JustifyContent.Keyword,
		alignItems:       box.Style.AlignItems.Keyword,
		alignContent:     box.Style.AlignContent.Keyword,
		hasDefiniteMain:  true,
		mainSize:         box.Content.Width,
		crossSize:        box.Content.Height,
		hasDefiniteCross: style.IsLength(box.Style.Height) || style.IsLength(box.Style.MinHeight) || (isAbsolutePositioned(box) && style.IsLength(box.Style.Top) && style.IsLength(box.Style.Bottom)),
		mainGap:          resolveLength(box.Style.ColumnGap, box.Content.Width),
		crossGap:         resolveLength(box.Style.RowGap, box.Content.Width),
	}
	if !config.isRow {
		config.hasDefiniteMain = style.IsLength(box.Style.Height)
		config.mainSize = box.Content.Height
		config.crossSize = box.Content.Width
		config.hasDefiniteCross = true
		config.mainGap = resolveLength(box.Style.RowGap, box.Content.Width)
		config.crossGap = resolveLength(box.Style.ColumnGap, box.Content.Width)
	}
	return config
}

func buildFlexLines(items []flexItem, config flexConfig, lines []flexLine) []flexLine {
	if len(items) == 0 {
		return lines
	}
	lineStart := 0
	used := 0.0
	for index := range items {
		item := &items[index]
		additional := item.outerMain
		if index > lineStart {
			additional += config.mainGap
		}
		if config.wrap && config.hasDefiniteMain && index > lineStart && used+additional > config.mainSize {
			lines = append(lines, flexLine{items: items[lineStart:index]})
			lineStart = index
			used = 0
			additional = item.outerMain
		}
		used += additional
	}
	lines = append(lines, flexLine{items: items[lineStart:]})
	return lines
}

// CSS Flexbox §9.7: freeze constrained items, then redistribute the remainder.
func resolveFlexibleLine(line *flexLine, config flexConfig) {
	hypotheticalUsed := config.mainGap * float64(max(len(line.items)-1, 0))
	for i := range line.items {
		item := &line.items[i]
		hypotheticalUsed += item.outerMain
		before, after := flexMainAutoMargins(item.box, config)
		if before {
			line.autoMargins++
		}
		if after {
			line.autoMargins++
		}
	}
	line.mainUsed = hypotheticalUsed
	if !config.hasDefiniteMain {
		return
	}
	growing := hypotheticalUsed < config.mainSize
	initialFree := config.mainSize - config.mainGap*float64(max(len(line.items)-1, 0))
	for i := range line.items {
		item := &line.items[i]
		factor := item.shrink
		if growing {
			factor = item.grow
		}
		item.frozen = factor == 0 || (growing && item.baseSize > item.mainSize) || (!growing && item.baseSize < item.mainSize)
		if !item.frozen {
			item.mainSize = item.baseSize
		}
		initialFree -= item.mainSize + item.mainExtras
	}
	for {
		free := config.mainSize - config.mainGap*float64(max(len(line.items)-1, 0))
		factors, weights, active := 0.0, 0.0, 0
		for i := range line.items {
			item := &line.items[i]
			size := item.mainSize
			if !item.frozen {
				size = item.baseSize
				active++
				if growing {
					factors += item.grow
					weights += item.grow
				} else {
					factors += item.shrink
					weights += item.shrink * item.baseSize
				}
			}
			free -= size + item.mainExtras
		}
		if active == 0 {
			break
		}
		if factors < 1 && math.Abs(initialFree*factors) < math.Abs(free) {
			free = initialFree * factors
		}
		violation := 0.0
		for i := range line.items {
			item := &line.items[i]
			if item.frozen {
				continue
			}
			weight := item.shrink * item.baseSize
			if growing {
				weight = item.grow
			}
			target := item.baseSize
			if weights > 0 {
				target += free * weight / weights
			}
			item.mainSize = min(max(target, item.minMain), item.maxMain)
			item.violation = item.mainSize - target
			violation += item.violation
		}
		for i := range line.items {
			item := &line.items[i]
			if violation == 0 || (violation > 0 && item.violation > 0) || (violation < 0 && item.violation < 0) {
				item.frozen = true
			}
		}
	}
	line.mainUsed = config.mainGap * float64(max(len(line.items)-1, 0))
	for i := range line.items {
		item := &line.items[i]
		item.outerMain = item.mainSize + item.mainExtras
		line.mainUsed += item.outerMain
	}
}

func measureFlexLine(line *flexLine, container *Box, config flexConfig, absoluteContaining Rect, face *font.Face, styles style.Map, ctx *layoutContext) {
	line.crossSize = 0
	for index := range line.items {
		item := &line.items[index]
		measureFlexItem(item, container, config, absoluteContaining, face, styles, ctx, math.NaN())
		if item.outerCross > line.crossSize {
			line.crossSize = item.outerCross
		}
	}
	updateFlexLineMainUsed(line, config)
	if line.crossSize < 0 {
		line.crossSize = 0
	}
}

func updateFlexLineMainUsed(line *flexLine, config flexConfig) {
	if line == nil {
		return
	}
	line.mainUsed = 0
	for _, item := range line.items {
		line.mainUsed += item.outerMain
	}
	if len(line.items) > 1 {
		line.mainUsed += config.mainGap * float64(len(line.items)-1)
	}
}

func measureFlexItem(item *flexItem, container *Box, config flexConfig, absoluteContaining Rect, face *font.Face, styles style.Map, ctx *layoutContext, forcedCrossInner float64) {
	mainSize := maxFloat(item.mainSize, 0)
	if item.measured && item.measureMain == mainSize && sameMeasuredCross(item, forcedCrossInner) {
		return
	}
	if item.measured && item.measureMain == mainSize && !math.IsNaN(forcedCrossInner) {
		cross := item.box.Style.Height
		if !config.isRow {
			cross = item.box.Style.Width
		}
		// Stretch cannot alter an explicit cross size. A column's auto width
		// already fills its definite containing width; assigning the same width
		// again also leaves percentage resolution and descendants unchanged.
		if cross.Kind != style.ValueAuto || !config.isRow && forcedCrossInner == item.box.Content.Width {
			return
		}
	}

	// Copy-on-write: box.Style is shared with the style map, so flex sizing
	// overrides must apply to a private copy for the duration of the measure.
	original := item.box.Style
	width, height := original.Width, original.Height
	if config.isRow {
		width = style.LengthPx(mainSize)
		if isBorderBox(original) {
			width = style.LengthPx(mainSize + computedHorizontalBoxExtras(original, container.Content.Width))
		}
		if !math.IsNaN(forcedCrossInner) && original.Height.Kind == style.ValueAuto {
			crossSize := maxFloat(forcedCrossInner, 0)
			if isBorderBox(original) {
				crossSize += computedVerticalBoxExtras(original, container.Content.Width)
			}
			height = style.LengthPx(crossSize)
		}
	} else {
		// Let an auto-sized item with no intrinsic content measure its normal
		// flow children before fixing the flex main size. This preserves the
		// child's natural height instead of collapsing the item to zero.
		if original.Height.Kind != style.ValueAuto || mainSize > 0 {
			height = style.LengthPx(mainSize)
			if isBorderBox(original) {
				height = style.LengthPx(mainSize + computedVerticalBoxExtras(original, container.Content.Width))
			}
		}
		if !math.IsNaN(forcedCrossInner) && original.Width.Kind == style.ValueAuto {
			crossSize := maxFloat(forcedCrossInner, 0)
			if isBorderBox(original) {
				crossSize += computedHorizontalBoxExtras(original, container.Content.Width)
			}
			width = style.LengthPx(crossSize)
		} else if math.IsNaN(forcedCrossInner) && original.Width.Kind == style.ValueAuto && item.alignSelf != "stretch" {
			crossSize := intrinsicFlexContentWidth(item.box, face, ctx, styles)
			if isBorderBox(original) {
				crossSize += computedHorizontalBoxExtras(original, container.Content.Width)
			}
			width = style.LengthPx(maxFloat(crossSize, 0))
		}
	}
	item.box.Style = item.box.measureStyle(&item.box.flexMeasureStyle, width, height)
	containing := Rect{X: container.Content.X, Y: container.Content.Y, Width: container.Content.Width, Height: container.Content.Height}
	layoutBlock(item.box, containing, absoluteContaining, face, styles, ctx)
	item.box.Style = original
	item.outerMain = flexOuterMain(item.box, config)
	item.outerCross = flexOuterCross(item.box, config)
	item.mainSize = maxFloat(flexContentMain(item.box, config), 0)
	item.mainExtras = item.outerMain - item.mainSize
	item.crossExtras = item.outerCross - flexContentCross(item.box, config)
	item.measured = true
	item.measureMain = mainSize
	item.measureCross = forcedCrossInner
	item.measureAutoCross = math.IsNaN(forcedCrossInner)
}

func resolveFlexBaseSize(item *flexItem, container *Box, config flexConfig, absoluteContaining Rect, face *font.Face, styles style.Map, ctx *layoutContext) float64 {
	computed := item.box.Style
	base, extras, size := container.Content.Width, computedHorizontalBoxExtras(computed, container.Content.Width), computed.Width
	if !config.isRow {
		base, extras, size = container.Content.Height, computedVerticalBoxExtras(computed, container.Content.Width), computed.Height
	}
	if style.IsLength(computed.FlexBasis) {
		size = computed.FlexBasis
	}
	// The flex base size is measured without min/max constraints; the
	// hypothetical size used for line breaking is clamped by the caller.
	if style.IsLength(size) {
		return contentDimension(size, base, extras, isBorderBox(computed))
	}
	if config.isRow {
		return intrinsicFlexContentWidth(item.box, face, ctx, styles)
	}

	// A column's auto basis is the laid-out content height at its available
	// width, including wrapped flex lines and normal-flow block children.
	original := item.box.Style
	if original.Width.Kind == style.ValueAuto && item.alignSelf != "stretch" {
		width := intrinsicFlexContentWidth(item.box, face, ctx, styles)
		if isBorderBox(original) {
			width += computedHorizontalBoxExtras(original, container.Content.Width)
		}
		// Copy-on-write: box.Style is shared with the style map, so the auto
		// basis must not leak into the node's computed style.
		item.box.Style = item.box.measureStyle(&item.box.flexMeasureStyle, style.LengthPx(width), original.Height)
	}
	containing := Rect{X: container.Content.X, Y: container.Content.Y, Width: container.Content.Width, Height: container.Content.Height}
	absolute := descendantAbsoluteContaining(container, absoluteContaining)
	var scratch *flexScratch
	if ctx != nil && ctx.retained != nil && !item.box.hasFloats && (ctx.floatContext == nil || len(ctx.floatContext.areas) == 0) {
		if item.box.flex == nil {
			item.box.flex = &flexScratch{}
		}
		scratch = item.box.flex
		if len(scratch.basis) > 0 && scratch.basis[0].pass != ctx.retained.pass {
			scratch.basis = scratch.basis[:0]
		}
		for _, basis := range scratch.basis {
			if basis.containing == containing && basis.absolute == absolute && basis.width == item.box.Style.Width && basis.height == item.box.Style.Height {
				item.box.Style = original
				return basis.result
			}
		}
	}
	layoutBlock(item.box, containing, absolute, face, styles, ctx)
	result := clampContentDimension(item.box.Content.Height, original.MinHeight, original.MaxHeight, container.Content.Height, computedVerticalBoxExtras(original, container.Content.Width), isBorderBox(original))
	if scratch != nil && ctx.err == nil {
		scratch.basis = append(scratch.basis, flexBasisMeasurement{pass: ctx.retained.pass, containing: containing, absolute: absolute, width: item.box.Style.Width, height: item.box.Style.Height, result: result})
	}
	item.box.Style = original
	return result
}

// Flex items contribute along the container's axes, unlike vertically stacked
// blocks. Out-of-flow children contribute neither widths nor gaps (Flexbox §4.1,
// §9.9). Percentages resolve against zero during intrinsic width measurement.
func flexIntrinsicWidths(box *Box, face *font.Face, styles style.Map, ctx *layoutContext) (float64, float64) {
	row := box.Style.FlexDirection.Keyword == "row" || box.Style.FlexDirection.Keyword == "row-reverse"
	wrap := box.Style.FlexWrap.Keyword != "nowrap"
	minimum, preferred, count := 0.0, 0.0, 0
	gap := resolveLength(box.Style.ColumnGap, 0)
	for _, child := range box.Children {
		if isAbsolutePositioned(child) {
			continue
		}
		extras := computedHorizontalBoxExtras(child.Style, 0)
		childMin, childPreferred := 0.0, 0.0
		if style.IsLength(child.Style.Width) {
			childMin = contentDimension(child.Style.Width, 0, extras, isBorderBox(child.Style)) + extras
			childPreferred = childMin
		} else {
			childMin, childPreferred = tableCellIntrinsicWidths(child, face, styles, ctx)
		}
		childMin = clampContentDimension(childMin-extras, child.Style.MinWidth, child.Style.MaxWidth, 0, extras, isBorderBox(child.Style)) + extras
		childPreferred = clampContentDimension(childPreferred-extras, child.Style.MinWidth, child.Style.MaxWidth, 0, extras, isBorderBox(child.Style)) + extras
		margin := resolveEdges(child.Style.Margin, 0)
		childMin += margin.Left + margin.Right
		childPreferred += margin.Left + margin.Right
		if row {
			if count > 0 {
				preferred += gap
				if !wrap {
					minimum += gap
				}
			}
			preferred += childPreferred
			if !wrap {
				minimum += childMin
			} else {
				minimum = max(minimum, childMin)
			}
		} else {
			minimum = max(minimum, childMin)
			preferred = max(preferred, childPreferred)
		}
		count++
	}
	extras := computedHorizontalBoxExtras(box.Style, 0)
	return minimum + extras, max(minimum, preferred) + extras
}

func intrinsicFlexContentWidth(box *Box, face *font.Face, ctx *layoutContext, styles style.Map) float64 {
	if box == nil {
		return 0
	}
	if style.IsLength(box.Style.Width) {
		return contentDimension(box.Style.Width, 0, computedHorizontalBoxExtras(box.Style, 0), isBorderBox(box.Style))
	}
	// A control that is itself a flex container sizes from its flex children;
	// native control metrics apply only to controls with their native display.
	if box.Node != nil && box.Node.Type == dom.ElementNode && !isFlexDisplay(box.Style.Display.Keyword) {
		if width, _ := intrinsicControlSize(box, face, ctx, styles); width > 0 {
			extra := box.Padding.Left + box.Padding.Right + box.Border.Left + box.Border.Right
			return maxFloat(width-extra, 0)
		}
		if box.Node.Data == "img" {
			if width, _ := resolveInlineImageSize(box.Node, box.Style, ctx); width > 0 {
				return maxFloat(width-box.Padding.Left-box.Padding.Right-box.Border.Left-box.Border.Right, 0)
			}
		}
	}
	if isFlexDisplay(box.Style.Display.Keyword) {
		row := box.Style.FlexDirection.Keyword == "row" || box.Style.FlexDirection.Keyword == "row-reverse"
		extent := 0.0
		inFlowCount := 0
		for _, child := range box.Children {
			// Absolutely positioned children do not contribute to intrinsic flex
			// sizing or gaps (CSS Flexbox §4.1).
			if isAbsolutePositioned(child) {
				continue
			}
			childWidth := intrinsicFlexContentWidth(child, face, ctx, styles)
			childEdges := resolveEdges(child.Style.Padding, box.Content.Height)
			border := resolveBorderEdges(child.Style.BorderWidth, child.Style.BorderStyle, box.Content.Height)
			margin := resolveEdges(child.Style.Margin, box.Content.Width)
			childWidth += childEdges.Left + childEdges.Right + border.Left + border.Right + margin.Left + margin.Right
			inFlowCount++
			if row {
				if inFlowCount > 1 {
					extent += resolveLength(box.Style.ColumnGap, box.Content.Width)
				}
				extent += childWidth
				continue
			}
			if childWidth > extent {
				extent = childWidth
			}
		}
		return extent
	}
	text := descendantText(box.Node)
	mode := whiteSpaceMode(box.Style, false)
	// Preformatted flex items need their leading whitespace in the intrinsic
	// width; trimming it makes indented code lines too narrow and pre-wrap then
	// breaks otherwise short lines at the first token boundary.
	if mode != "pre" && mode != "pre-wrap" {
		text = strings.TrimSpace(text)
	}
	face = fontForStyle(face, box.Style, ctx)
	if text == "" || face == nil {
		return 0
	}
	fontSize := box.Style.FontSize.LengthPx
	if fontSize <= 0 {
		fontSize = 16
	}
	return measureIntrinsicTextWidth(face, text, fontSize, box.Style.LetterSpacing.LengthPx, mode, box.Style.TabSize, ctx)
}

func sameMeasuredCross(item *flexItem, forcedCrossInner float64) bool {
	if item == nil {
		return false
	}
	if item.measureAutoCross || math.IsNaN(forcedCrossInner) {
		return item.measureAutoCross && math.IsNaN(forcedCrossInner)
	}
	return item.measureCross == forcedCrossInner
}

func flexContentExtents(lines []flexLine, config flexConfig) (float64, float64) {
	mainExtent := 0.0
	crossExtent := 0.0
	for _, line := range lines {
		if line.mainUsed > mainExtent {
			mainExtent = line.mainUsed
		}
		crossExtent += line.crossSize
	}
	if len(lines) > 1 {
		crossExtent += config.crossGap * float64(len(lines)-1)
	}
	if !config.isRow {
		mainExtent = 0
		for _, line := range lines {
			if line.mainUsed > mainExtent {
				mainExtent = line.mainUsed
			}
		}
		if len(lines) == 1 {
			mainExtent = lines[0].mainUsed
		}
	}
	return mainExtent, crossExtent
}

func resolveFlexLinePositions(lines []flexLine, box *Box, config flexConfig, positions []float64) []float64 {
	if len(lines) == 0 {
		return positions
	}
	positions = slices.Grow(positions, len(lines))[:len(lines)]
	containerCross := config.crossSize
	if !config.isRow {
		containerCross = box.Content.Width
	}
	if len(lines) == 1 && config.hasDefiniteCross {
		lines[0].crossSize = containerCross
		positions[0] = 0
		return positions
	}
	totalCross := 0.0
	for _, line := range lines {
		totalCross += line.crossSize
	}
	if len(lines) > 1 {
		totalCross += config.crossGap * float64(len(lines)-1)
	}
	start, gap, stretch := distributeSpace(logicalFlexAlignment(config.alignContent, config.reverseCross), maxFloat(containerCross-totalCross, 0), len(lines), config.crossGap, config.hasDefiniteCross && len(lines) > 1)
	accumulated := 0.0
	for i, line := range lines {
		lineSize := line.crossSize + stretch
		lines[i].crossSize = lineSize
		if !config.reverseCross {
			positions[i] = start + accumulated
		} else {
			positions[i] = containerCross - start - accumulated - lineSize
		}
		accumulated += lineSize + gap
	}
	return positions
}

func layoutFlexLine(line flexLine, lineCrossStart float64, container *Box, config flexConfig, absoluteContaining Rect, face *font.Face, styles style.Map, ctx *layoutContext) {
	if len(line.items) == 0 {
		return
	}
	containerMain := config.mainSize
	if !config.hasDefiniteMain {
		containerMain = line.mainUsed
	}
	freeSpace := maxFloat(containerMain-line.mainUsed, 0)
	autoMargin := 0.0
	if line.autoMargins > 0 {
		autoMargin = freeSpace / float64(line.autoMargins)
		freeSpace = 0
	}
	mainStart, gap, _ := distributeSpace(logicalFlexAlignment(config.justifyContent, config.reverseMain), freeSpace, len(line.items), config.mainGap, false)
	mainOrigin := flexMainOrigin(container.Content, config)
	crossOrigin := flexCrossOrigin(container.Content, config)
	progress := 0.0
	for index := range line.items {
		item := &line.items[index]
		align := item.alignSelf
		forcedCross := math.NaN()
		crossBefore, crossAfter := item.box.Style.Margin.Top, item.box.Style.Margin.Bottom
		if !config.isRow {
			crossBefore, crossAfter = item.box.Style.Margin.Left, item.box.Style.Margin.Right
		}
		beforeCrossAuto, afterCrossAuto := crossBefore.Kind == style.ValueAuto, crossAfter.Kind == style.ValueAuto
		if align == "stretch" && !beforeCrossAuto && !afterCrossAuto {
			forcedCross = line.crossSize - item.crossExtras
			measureFlexItem(item, container, config, absoluteContaining, face, styles, ctx, forcedCross)
		}
		crossFree := maxFloat(line.crossSize-item.outerCross, 0)
		crossOffset := alignedOffset(logicalFlexAlignment(align, false), crossFree)
		if config.reverseCross && (align == "flex-start" || align == "flex-end") {
			crossOffset = crossFree - crossOffset
		}
		// Physical cross-axis auto margins consume free space before alignment.
		if beforeCrossAuto || afterCrossAuto {
			crossOffset = 0
			if beforeCrossAuto {
				crossOffset = crossFree
				if afterCrossAuto {
					crossOffset /= 2
				}
			}
		}
		beforeAuto, afterAuto := flexMainAutoMargins(item.box, config)
		if beforeAuto {
			progress += autoMargin
		}
		var outerMainStart float64
		if !config.reverseMain {
			outerMainStart = mainOrigin + mainStart + progress
		} else {
			outerMainStart = mainOrigin + containerMain - mainStart - progress - item.outerMain
		}
		outerCrossStart := crossOrigin + lineCrossStart + crossOffset
		if config.isRow {
			translateBox(item.box, outerMainStart-item.box.FlowMarginBox.X, outerCrossStart-item.box.FlowMarginBox.Y, ctx)
		} else {
			translateBox(item.box, outerCrossStart-item.box.FlowMarginBox.X, outerMainStart-item.box.FlowMarginBox.Y, ctx)
		}
		progress += item.outerMain + gap
		if afterAuto {
			progress += autoMargin
		}
	}
}

func distributeSpace(mode string, freeSpace float64, count int, baseGap float64, allowStretch bool) (float64, float64, float64) {
	if count <= 0 {
		return 0, baseGap, 0
	}
	gap := baseGap
	switch mode {
	case "flex-end":
		return freeSpace, gap, 0
	case "center":
		return freeSpace / 2, gap, 0
	case "space-between":
		if count > 1 {
			gap += freeSpace / float64(count-1)
		}
		return 0, gap, 0
	case "space-around":
		extra := freeSpace / float64(count)
		gap += extra
		return extra / 2, gap, 0
	case "space-evenly":
		extra := freeSpace / float64(count+1)
		gap += extra
		return extra, gap, 0
	case "stretch":
		if allowStretch {
			return 0, gap, freeSpace / float64(count)
		}
	}
	return 0, gap, 0
}

func alignedOffset(mode string, freeSpace float64) float64 {
	switch mode {
	case "flex-end":
		return freeSpace
	case "center":
		return freeSpace / 2
	default:
		return 0
	}
}

func resolveAlignSelf(container *Box, child *Box) string {
	if child.Style.AlignSelf.Kind == style.ValueKeyword && child.Style.AlignSelf.Keyword != "auto" {
		return child.Style.AlignSelf.Keyword
	}
	if container.Style.AlignItems.Kind == style.ValueKeyword {
		return container.Style.AlignItems.Keyword
	}
	return "stretch"
}

func flexMainOrigin(rect Rect, config flexConfig) float64 {
	if config.isRow {
		return rect.X
	}
	return rect.Y
}

func flexCrossOrigin(rect Rect, config flexConfig) float64 {
	if config.isRow {
		return rect.Y
	}
	return rect.X
}

func flexMainExtras(computed *style.ComputedStyle, config flexConfig, base float64) float64 {
	margin := resolveEdges(computed.Margin, base)
	padding := resolveEdges(computed.Padding, base)
	border := resolveBorderEdges(computed.BorderWidth, computed.BorderStyle, base)
	if config.isRow {
		return margin.Left + margin.Right + padding.Left + padding.Right + border.Left + border.Right
	}
	return margin.Top + margin.Bottom + padding.Top + padding.Bottom + border.Top + border.Bottom
}

func flexCrossExtras(computed *style.ComputedStyle, config flexConfig, base float64) float64 {
	margin := resolveEdges(computed.Margin, base)
	padding := resolveEdges(computed.Padding, base)
	border := resolveBorderEdges(computed.BorderWidth, computed.BorderStyle, base)
	if config.isRow {
		return margin.Top + margin.Bottom + padding.Top + padding.Bottom + border.Top + border.Bottom
	}
	return margin.Left + margin.Right + padding.Left + padding.Right + border.Left + border.Right
}

func flexOuterMain(box *Box, config flexConfig) float64 {
	if config.isRow {
		return box.FlowMarginBox.Width
	}
	return box.FlowMarginBox.Height
}

func flexOuterCross(box *Box, config flexConfig) float64 {
	if config.isRow {
		return box.FlowMarginBox.Height
	}
	return box.FlowMarginBox.Width
}

func flexContentMain(box *Box, config flexConfig) float64 {
	if config.isRow {
		return box.Content.Width
	}
	return box.Content.Height
}

func flexContentCross(box *Box, config flexConfig) float64 {
	if config.isRow {
		return box.Content.Height
	}
	return box.Content.Width
}

func resolveNumber(value style.Value) float64 {
	if value.Kind != style.ValueNumber {
		return 0
	}
	return value.Number
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

// measureStyle returns the style a measure pass reads. The forced size lives on
// a private copy because fragments retain its address, so only reuse the shared
// map style when the sizes it would carry are already identical.
func (box *Box) measureStyle(slot **style.ComputedStyle, width, height style.Value) *style.ComputedStyle {
	if box.Style == nil || box.Style.Width == width && box.Style.Height == height {
		return box.Style
	}
	if *slot == nil {
		*slot = &style.ComputedStyle{}
	}
	**slot = *box.Style
	(*slot).Width, (*slot).Height = width, height
	return *slot
}

// start/end follow the physical start in the currently supported horizontal LTR
// writing mode; flex-start/flex-end follow the potentially reversed flex axis.
func logicalFlexAlignment(mode string, reverse bool) string {
	switch mode {
	case "start":
		if reverse {
			return "flex-end"
		}
		return "flex-start"
	case "end":
		if reverse {
			return "flex-start"
		}
		return "flex-end"
	default:
		return mode
	}
}

func flexMainAutoMargins(box *Box, config flexConfig) (bool, bool) {
	before, after := box.Style.Margin.Left, box.Style.Margin.Right
	if !config.isRow {
		before, after = box.Style.Margin.Top, box.Style.Margin.Bottom
	}
	if config.reverseMain {
		before, after = after, before
	}
	return before.Kind == style.ValueAuto, after.Kind == style.ValueAuto
}
