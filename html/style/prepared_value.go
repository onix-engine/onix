package style

import "strings"

type preparedValue struct {
	property string
	value    Value
	motion   *motionInputs
}

func preparePropertyValue(property, raw string) *preparedValue {
	if isCSSWideKeyword(raw) {
		return nil
	}
	computed := defaultStyle(nil)
	if strings.HasPrefix(property, "transition") || strings.HasPrefix(property, "animation") {
		if err := applyProperty(&computed, property, raw, nil, parseContext{}); err != nil || computed.motion == nil {
			return nil
		}
		inputs := computed.motion.motionInputs
		return &preparedValue{property: property, motion: &inputs}
	}
	field := compiledValueField(&computed, property)
	if field == nil {
		return nil
	}
	// Lengths in relative units and expressions keep their node/font/viewport
	// context. Only literal px, percentage, zero and keywords are compiled.
	switch property {
	case "width", "height", "min-width", "min-height", "max-width", "max-height", "top", "right", "bottom", "left", "flex-basis", "row-gap", "column-gap", "text-indent", "letter-spacing", "tab-size":
		value := strings.ToLower(raw)
		if strings.Contains(value, "(") || strings.HasSuffix(value, "em") || strings.HasSuffix(value, "vw") || strings.HasSuffix(value, "vh") {
			return nil
		}
	}
	if err := applyProperty(&computed, property, raw, nil, parseContext{FontSize: 16, ParentFontSize: 16, RootFontSize: 16}); err != nil {
		return nil
	}
	return &preparedValue{property: property, value: *field}
}

func compiledValueField(computed *ComputedStyle, property string) *Value {
	switch property {
	case "display":
		return &computed.Display
	case "table-layout":
		return &computed.TableLayout
	case "vertical-align":
		return &computed.VerticalAlign
	case "appearance":
		return &computed.Appearance
	case "visibility":
		return &computed.Visibility
	case "pointer-events":
		return &computed.PointerEvents
	case "box-sizing":
		return &computed.BoxSizing
	case "width":
		return &computed.Width
	case "height":
		return &computed.Height
	case "min-width":
		return &computed.MinWidth
	case "min-height":
		return &computed.MinHeight
	case "max-width":
		return &computed.MaxWidth
	case "max-height":
		return &computed.MaxHeight
	case "top":
		return &computed.Top
	case "right":
		return &computed.Right
	case "bottom":
		return &computed.Bottom
	case "left":
		return &computed.Left
	case "position":
		return &computed.Position
	case "float":
		return &computed.Float
	case "clear":
		return &computed.Clear
	case "z-index":
		return &computed.ZIndex
	case "opacity":
		return &computed.Opacity
	case "flex-direction":
		return &computed.FlexDirection
	case "flex-wrap":
		return &computed.FlexWrap
	case "flex-grow":
		return &computed.FlexGrow
	case "flex-shrink":
		return &computed.FlexShrink
	case "flex-basis":
		return &computed.FlexBasis
	case "row-gap":
		return &computed.RowGap
	case "column-gap":
		return &computed.ColumnGap
	case "order":
		return &computed.Order
	case "justify-content":
		return &computed.JustifyContent
	case "justify-items":
		return &computed.JustifyItems
	case "justify-self":
		return &computed.JustifySelf
	case "align-items":
		return &computed.AlignItems
	case "align-self":
		return &computed.AlignSelf
	case "align-content":
		return &computed.AlignContent
	case "color":
		return &computed.Color
	case "placeholder-color":
		return &computed.PlaceholderColor
	case "font-family":
		return &computed.FontFamily
	case "text-align":
		return &computed.TextAlign
	case "text-box-trim":
		return &computed.TextBoxTrim
	case "text-box-edge":
		return &computed.TextBoxEdge
	case "text-decoration-line":
		return &computed.TextDecorationLine
	case "text-indent":
		return &computed.TextIndent
	case "letter-spacing":
		return &computed.LetterSpacing
	case "tab-size":
		return &computed.TabSize
	case "white-space":
		return &computed.WhiteSpace
	case "word-break":
		return &computed.WordBreak
	case "overflow-wrap", "word-wrap":
		return &computed.OverflowWrap
	case "text-overflow":
		return &computed.TextOverflow
	case "list-style-type":
		return &computed.ListStyleType
	case "overflow-x":
		return &computed.OverflowX
	case "overflow-y":
		return &computed.OverflowY
	case "scrollbar-width":
		return &computed.ScrollbarWidth
	case "container-type":
		return &computed.ContainerType
	case "object-fit":
		return &computed.ObjectFit
	}
	return nil
}
