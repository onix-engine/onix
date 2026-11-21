package style

import "strings"

type preparedValue struct {
	motion *motionInputs
	value  Value
}

// Validate each declaration once. Cache a computed value only when it is
// independent of its element's font, viewport and filter definitions.
func prepareDeclaration(name, raw string, important bool) (preparedDeclaration, bool) {
	order := cascadePropertyIndex[name]
	entry := preparedDeclaration{order: order, value: raw, important: important}
	id := order.id
	if id == propertyInvalid || raw == "" {
		return entry, false
	}
	if isCSSWideKeyword(raw) {
		return entry, true
	}
	ctx := parseContext{FontSize: 16, ParentFontSize: 16, RootFontSize: 16}
	computed := defaultStyle(nil)
	err := applyProperty(&computed, id, raw, nil, ctx)
	if err != nil {
		// SVG filters resolve against the document at cascade time.
		return entry, id == propertyFilter && strings.Contains(strings.ToLower(raw), "url(")
	}
	if id.isTransition() || id.isAnimation() {
		inputs := computed.motion.motionInputs
		entry.parsed = &preparedValue{motion: &inputs}
		return entry, true
	}
	switch id {
	case propertyWidth, propertyHeight, propertyMinWidth, propertyMinHeight, propertyMaxWidth, propertyMaxHeight,
		propertyTop, propertyRight, propertyBottom, propertyLeft, propertyFlexBasis, propertyRowGap, propertyColumnGap,
		propertyTextIndent, propertyLetterSpacing, propertyTabSize:
		value := strings.ToLower(raw)
		if strings.Contains(value, "(") || strings.HasSuffix(value, "em") || strings.HasSuffix(value, "vw") || strings.HasSuffix(value, "vh") {
			return entry, true
		}
	}
	if field := compiledValueField(&computed, id); field != nil {
		entry.parsed = &preparedValue{value: *field}
	}
	return entry, true
}

// These longhands each accept one keyword and write one computed Value.
var propertyKeywords = [propertyCount][]string{
	propertyDisplay:            {"none", "contents", "block", "inline", "inline-block", "flow-root", "flex", "inline-flex", "grid", "inline-grid", "list-item", "table", "inline-table", "table-caption", "table-column", "table-column-group", "table-row", "table-row-group", "table-header-group", "table-footer-group", "table-cell"},
	propertyFloat:              {"none", "left", "right"},
	propertyClear:              {"none", "left", "right", "both"},
	propertyTableLayout:        {"auto", "fixed"},
	propertyVerticalAlign:      {"baseline", "top", "middle", "bottom"},
	propertyListStyleType:      {"disc", "circle", "square", "decimal", "decimal-leading-zero", "lower-alpha", "upper-alpha", "lower-latin", "upper-latin", "lower-roman", "upper-roman", "none"},
	propertyVisibility:         {"visible", "hidden", "collapse"},
	propertyAppearance:         {"auto", "none"},
	propertyBoxSizing:          {"content-box", "border-box"},
	propertyFlexDirection:      {"row", "row-reverse", "column", "column-reverse"},
	propertyFlexWrap:           {"nowrap", "wrap", "wrap-reverse"},
	propertyJustifyContent:     {"flex-start", "flex-end", "start", "end", "center", "space-between", "space-around", "space-evenly"},
	propertyJustifyItems:       {"stretch", "start", "end", "center"},
	propertyJustifySelf:        {"auto", "stretch", "start", "end", "center"},
	propertyAlignItems:         {"stretch", "flex-start", "flex-end", "start", "end", "center", "baseline"},
	propertyAlignSelf:          {"auto", "stretch", "flex-start", "flex-end", "start", "end", "center", "baseline"},
	propertyAlignContent:       {"stretch", "flex-start", "flex-end", "start", "end", "center", "space-between", "space-around", "space-evenly"},
	propertyContainerType:      {"normal", "inline-size", "size"},
	propertyOverflowX:          {"visible", "hidden", "clip", "auto", "scroll"},
	propertyOverflowY:          {"visible", "hidden", "clip", "auto", "scroll"},
	propertyTextAlign:          {"left", "right", "center"},
	propertyTextDecorationLine: {"none", "line-through"},
	propertyWordBreak:          {"normal", "break-all", "keep-all"},
	propertyOverflowWrap:       {"normal", "break-word", "anywhere"},
	propertyWordWrap:           {"normal", "break-word", "anywhere"},
	propertyTextOverflow:       {"clip", "ellipsis"},
	propertyTextBoxTrim:        {"none", "trim-start", "trim-end", "trim-both"},
	propertyWhiteSpace:         {"normal", "nowrap", "pre", "pre-wrap", "pre-line"},
	propertyObjectFit:          {"fill", "contain", "cover", "none", "scale-down"},
}

func compiledValueField(computed *ComputedStyle, property propertyID) *Value {
	switch property {
	case propertyDisplay:
		return &computed.Display
	case propertyTableLayout:
		return &computed.TableLayout
	case propertyVerticalAlign:
		return &computed.VerticalAlign
	case propertyAppearance:
		return &computed.Appearance
	case propertyVisibility:
		return &computed.Visibility
	case propertyPointerEvents:
		return &computed.PointerEvents
	case propertyBoxSizing:
		return &computed.BoxSizing
	case propertyWidth:
		return &computed.Width
	case propertyHeight:
		return &computed.Height
	case propertyMinWidth:
		return &computed.MinWidth
	case propertyMinHeight:
		return &computed.MinHeight
	case propertyMaxWidth:
		return &computed.MaxWidth
	case propertyMaxHeight:
		return &computed.MaxHeight
	case propertyTop:
		return &computed.Top
	case propertyRight:
		return &computed.Right
	case propertyBottom:
		return &computed.Bottom
	case propertyLeft:
		return &computed.Left
	case propertyPosition:
		return &computed.Position
	case propertyFloat:
		return &computed.Float
	case propertyClear:
		return &computed.Clear
	case propertyZIndex:
		return &computed.ZIndex
	case propertyOpacity:
		return &computed.Opacity
	case propertyFlexDirection:
		return &computed.FlexDirection
	case propertyFlexWrap:
		return &computed.FlexWrap
	case propertyFlexGrow:
		return &computed.FlexGrow
	case propertyFlexShrink:
		return &computed.FlexShrink
	case propertyFlexBasis:
		return &computed.FlexBasis
	case propertyRowGap:
		return &computed.RowGap
	case propertyColumnGap:
		return &computed.ColumnGap
	case propertyOrder:
		return &computed.Order
	case propertyJustifyContent:
		return &computed.JustifyContent
	case propertyJustifyItems:
		return &computed.JustifyItems
	case propertyJustifySelf:
		return &computed.JustifySelf
	case propertyAlignItems:
		return &computed.AlignItems
	case propertyAlignSelf:
		return &computed.AlignSelf
	case propertyAlignContent:
		return &computed.AlignContent
	case propertyColor:
		return &computed.Color
	case propertyPlaceholderColor:
		return &computed.PlaceholderColor
	case propertyFontFamily:
		return &computed.FontFamily
	case propertyTextAlign:
		return &computed.TextAlign
	case propertyTextBoxTrim:
		return &computed.TextBoxTrim
	case propertyTextBoxEdge:
		return &computed.TextBoxEdge
	case propertyTextDecorationLine:
		return &computed.TextDecorationLine
	case propertyTextIndent:
		return &computed.TextIndent
	case propertyLetterSpacing:
		return &computed.LetterSpacing
	case propertyTabSize:
		return &computed.TabSize
	case propertyWhiteSpace:
		return &computed.WhiteSpace
	case propertyWordBreak:
		return &computed.WordBreak
	case propertyOverflowWrap, propertyWordWrap:
		return &computed.OverflowWrap
	case propertyTextOverflow:
		return &computed.TextOverflow
	case propertyListStyleType:
		return &computed.ListStyleType
	case propertyOverflowX:
		return &computed.OverflowX
	case propertyOverflowY:
		return &computed.OverflowY
	case propertyScrollbarWidth:
		return &computed.ScrollbarWidth
	case propertyContainerType:
		return &computed.ContainerType
	case propertyObjectFit:
		return &computed.ObjectFit
	}
	return nil
}
