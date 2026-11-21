package style

import (
	"slices"
	"strings"

	uinode "github.com/onix-engine/onix/scene/node"
)

// cssWideKeywords are the value keywords from CSS Cascade 4 section 7 that
// stand in for a declaration's cascaded value instead of a property value.
var cssWideKeywords = [...]string{"inherit", "initial", "unset", "revert", "revert-layer"}

// inheritedProperties lists the properties whose unset value inherits from the
// parent. The list mirrors the properties copied by inherit().
var inheritedProperties = [propertyCount]bool{
	propertyFont:               true,
	propertyFontSize:           true,
	propertyFontFamily:         true,
	propertyFontWeight:         true,
	propertyFontStyle:          true,
	propertyVisibility:         true,
	propertyPointerEvents:      true,
	propertyCursor:             true,
	propertyColor:              true,
	propertyPlaceholderColor:   true,
	propertyTextAlign:          true,
	propertyTextBoxEdge:        true,
	propertyTextDecorationLine: true,
	propertyTextIndent:         true,
	propertyLetterSpacing:      true,
	propertyTabSize:            true,
	propertyLineHeight:         true,
	propertyWhiteSpace:         true,
	propertyWordBreak:          true,
	propertyOverflowWrap:       true,
	propertyWordWrap:           true,
	propertyTextOverflow:       true,
	propertyListStyleType:      true,
	propertyBorderSpacing:      true,
}

func isCSSWideKeyword(value string) bool {
	value = strings.TrimSpace(value)
	for _, keyword := range cssWideKeywords {
		if strings.EqualFold(value, keyword) {
			return true
		}
	}
	return false
}

func isWideKeywordOf(value, keyword string) bool {
	return strings.EqualFold(strings.TrimSpace(value), keyword)
}

func isInheritedProperty(property propertyID) bool {
	return inheritedProperties[property]
}

// applyCSSWideKeyword resolves one winning CSS-wide declaration. Inherit is
// valid for every property, not only inherited properties; unset inherits only
// where the property is inherited and resets all other properties.
func applyCSSWideKeyword(style, parent *ComputedStyle, node *uinode.Node, property propertyID, raw string) {
	inherits := isWideKeywordOf(raw, "inherit") || isInheritedProperty(property) && (isWideKeywordOf(raw, "unset") || isWideKeywordOf(raw, "revert") || isWideKeywordOf(raw, "revert-layer"))
	if inherits && parent != nil {
		copyCSSProperty(style, parent, property)
		return
	}
	resetCSSProperty(style, node, property)
}

func resetCSSProperty(style *ComputedStyle, node *uinode.Node, property propertyID) {
	if style == nil {
		return
	}
	defaults := defaultStyle(node)
	copyCSSProperty(style, &defaults, property)
}

// copyCSSProperty copies the computed value represented by one declaration.
// Keeping this as a switch avoids reflection in the cascade hot path and also
// makes shorthand behavior explicit.
func copyCSSProperty(style, parent *ComputedStyle, property propertyID) {
	if style == nil || parent == nil {
		return
	}
	if field := compiledValueField(style, property); field != nil {
		*field = *compiledValueField(parent, property)
		return
	}
	switch property {
	case propertyFont:
		style.FontSize, style.FontFamily, style.FontWeight = parent.FontSize, parent.FontFamily, parent.FontWeight
		style.FontStyle = parent.FontStyle
		style.LineHeight, style.LineHeightNormal = parent.LineHeight, parent.LineHeightNormal
	case propertyBorderSpacing:
		style.BorderSpacingX, style.BorderSpacingY = parent.BorderSpacingX, parent.BorderSpacingY
	case propertyGridTemplateColumns:
		style.GridTemplateColumns = parent.GridTemplateColumns
	case propertyGridTemplateRows:
		style.GridTemplateRows = parent.GridTemplateRows
	case propertyGridColumn:
		style.GridColumnStart, style.GridColumnEnd = parent.GridColumnStart, parent.GridColumnEnd
	case propertyGridRow:
		style.GridRowStart, style.GridRowEnd = parent.GridRowStart, parent.GridRowEnd
	case propertyGridColumnStart:
		style.GridColumnStart = parent.GridColumnStart
	case propertyGridColumnEnd:
		style.GridColumnEnd = parent.GridColumnEnd
	case propertyGridRowStart:
		style.GridRowStart = parent.GridRowStart
	case propertyGridRowEnd:
		style.GridRowEnd = parent.GridRowEnd
	case propertyCursor:
		style.Cursor = parent.Cursor
	case propertyFlex:
		style.FlexGrow, style.FlexShrink, style.FlexBasis = parent.FlexGrow, parent.FlexShrink, parent.FlexBasis
	case propertyFlexFlow:
		style.FlexDirection, style.FlexWrap = parent.FlexDirection, parent.FlexWrap
	case propertyGap:
		style.RowGap, style.ColumnGap = parent.RowGap, parent.ColumnGap
	case propertyContainer:
		style.ContainerName, style.ContainerType = parent.ContainerName, parent.ContainerType
	case propertyContainerName:
		style.ContainerName = parent.ContainerName
	case propertyInset:
		style.Top, style.Right, style.Bottom, style.Left = parent.Top, parent.Right, parent.Bottom, parent.Left
	case propertyTransform:
		style.TransformRotation = parent.TransformRotation
		style.TransformTranslateX = parent.TransformTranslateX
		style.TransformTranslateY = parent.TransformTranslateY
		style.TransformScaleX, style.TransformScaleY = parent.TransformScaleX, parent.TransformScaleY
		style.TransformSkewX = parent.TransformSkewX
		style.TransformTranslateCrossX, style.TransformTranslateCrossY = parent.TransformTranslateCrossX, parent.TransformTranslateCrossY
	case propertyTransformOrigin:
		style.TransformOriginX, style.TransformOriginY = parent.TransformOriginX, parent.TransformOriginY
	case propertyTransition:
		style.mutableMotion().transition = parent.transitionValues()
	case propertyTransitionProperty, propertyTransitionDuration, propertyTransitionTimingFunction, propertyTransitionDelay:
		index := property - propertyTransitionProperty
		style.mutableMotion().transition[index] = parent.transitionValues()[index]
	case propertyAnimation:
		style.mutableMotion().animation = parent.animationValues()
	case propertyAnimationName, propertyAnimationDuration, propertyAnimationTimingFunction, propertyAnimationDelay, propertyAnimationIterationCount, propertyAnimationDirection, propertyAnimationFillMode, propertyAnimationPlayState:
		index := property - propertyAnimationName
		style.mutableMotion().animation[index] = parent.animationValues()[index]
	case propertyOverflow:
		style.OverflowX, style.OverflowY = parent.OverflowX, parent.OverflowY
	case propertyScrollbarColor:
		style.ScrollbarTrackColor, style.ScrollbarThumbColor = parent.ScrollbarTrackColor, parent.ScrollbarThumbColor
	case propertyScrollbarTrackColor:
		style.ScrollbarTrackColor = parent.ScrollbarTrackColor
	case propertyScrollbarThumbColor:
		style.ScrollbarThumbColor = parent.ScrollbarThumbColor
	case propertyScrollbarThumbHoverColor:
		style.ScrollbarThumbHoverColor = parent.ScrollbarThumbHoverColor
	case propertyMargin:
		style.Margin = parent.Margin
	case propertyMarginTop:
		style.Margin.Top = parent.Margin.Top
	case propertyMarginRight:
		style.Margin.Right = parent.Margin.Right
	case propertyMarginBottom:
		style.Margin.Bottom = parent.Margin.Bottom
	case propertyMarginLeft:
		style.Margin.Left = parent.Margin.Left
	case propertyPadding:
		style.Padding = parent.Padding
	case propertyPaddingTop:
		style.Padding.Top = parent.Padding.Top
	case propertyPaddingRight:
		style.Padding.Right = parent.Padding.Right
	case propertyPaddingBottom:
		style.Padding.Bottom = parent.Padding.Bottom
	case propertyPaddingLeft:
		style.Padding.Left = parent.Padding.Left
	case propertyBorder:
		style.BorderWidth, style.BorderStyle, style.BorderColor = parent.BorderWidth, parent.BorderStyle, parent.BorderColor
	case propertyBorderTop, propertyBorderRight, propertyBorderBottom, propertyBorderLeft:
		copyBorderSide(style, parent, property)
	case propertyBorderWidth:
		style.BorderWidth = parent.BorderWidth
	case propertyBorderStyle:
		style.BorderStyle = parent.BorderStyle
	case propertyBorderColor:
		style.BorderColor = parent.BorderColor
	case propertyBorderTopStyle, propertyBorderRightStyle, propertyBorderBottomStyle, propertyBorderLeftStyle:
		copyBorderStyleSide(style, parent, property)
	case propertyBorderTopWidth, propertyBorderRightWidth, propertyBorderBottomWidth, propertyBorderLeftWidth:
		copyBorderWidthSide(style, parent, property)
	case propertyBorderTopColor, propertyBorderRightColor, propertyBorderBottomColor, propertyBorderLeftColor:
		copyBorderColorSide(style, parent, property)
	case propertyBorderRadius:
		style.BorderRadius = parent.BorderRadius
	case propertyBorderTopLeftRadius:
		style.BorderRadius.TopLeft = parent.BorderRadius.TopLeft
	case propertyBorderTopRightRadius:
		style.BorderRadius.TopRight = parent.BorderRadius.TopRight
	case propertyBorderBottomRightRadius:
		style.BorderRadius.BottomRight = parent.BorderRadius.BottomRight
	case propertyBorderBottomLeftRadius:
		style.BorderRadius.BottomLeft = parent.BorderRadius.BottomLeft
	case propertyBoxShadow:
		style.BoxShadow = slices.Clone(parent.BoxShadow)
	case propertyFilter:
		style.Filters = slices.Clone(parent.Filters)
	case propertyBackground:
		copyBackground(style, parent)
	case propertyBackgroundColor:
		style.BackgroundColor = parent.BackgroundColor
	case propertyBackgroundImage:
		copyBackgroundImages(style, parent)
	case propertyBackgroundPosition, propertyBackgroundRepeat, propertyBackgroundSize, propertyBackgroundOrigin, propertyBackgroundClip, propertyBackgroundAttachment:
		copyBackgroundLayerField(style, parent, property)
	case propertyFontSize:
		style.FontSize = parent.FontSize
	case propertyLineHeight:
		style.LineHeight, style.LineHeightNormal = parent.LineHeight, parent.LineHeightNormal
	case propertyFontStyle:
		style.FontStyle = parent.FontStyle
	case propertyFontWeight:
		style.FontWeight = parent.FontWeight
	case propertyObjectPosition:
		style.ObjectPosX, style.ObjectPosY = parent.ObjectPosX, parent.ObjectPosY
		style.ObjectPosXOrigin, style.ObjectPosYOrigin = parent.ObjectPosXOrigin, parent.ObjectPosYOrigin
	}
}

func copyBackground(style, parent *ComputedStyle) {
	style.Background = parent.Background
	style.ExtraBackgroundLayers = slices.Clone(parent.ExtraBackgroundLayers)
	style.BackgroundColor = parent.BackgroundColor
}

func copyBackgroundImages(style, parent *ComputedStyle) {
	style.Background.Image = parent.Background.Image
	if len(parent.ExtraBackgroundLayers) == 0 {
		style.ExtraBackgroundLayers = nil
		return
	}
	ensureBackgroundLayerCount(style, parent.BackgroundLayerCount())
	style.ExtraBackgroundLayers = style.ExtraBackgroundLayers[:len(parent.ExtraBackgroundLayers)]
	for index := range style.ExtraBackgroundLayers {
		style.ExtraBackgroundLayers[index].Image = parent.ExtraBackgroundLayers[index].Image
	}
}

func copyBackgroundLayerField(style, parent *ComputedStyle, property propertyID) {
	ensureBackgroundLayerCount(style, parent.BackgroundLayerCount())
	for index := range style.BackgroundLayerCount() {
		source := parent.BackgroundLayer(min(index, parent.BackgroundLayerCount()-1))
		target := style.BackgroundLayer(index)
		switch property {
		case propertyBackgroundPosition:
			target.PosX, target.PosY = source.PosX, source.PosY
			target.PosXOrigin, target.PosYOrigin = source.PosXOrigin, source.PosYOrigin
		case propertyBackgroundRepeat:
			target.RepeatX, target.RepeatY = source.RepeatX, source.RepeatY
		case propertyBackgroundSize:
			target.SizeX, target.SizeY = source.SizeX, source.SizeY
		case propertyBackgroundOrigin:
			target.Origin = source.Origin
		case propertyBackgroundClip:
			target.Clip = source.Clip
		case propertyBackgroundAttachment:
			target.Attachment = source.Attachment
		}
	}
}

func copyBorderSide(style, parent *ComputedStyle, property propertyID) {
	switch property {
	case propertyBorderTop:
		style.BorderWidth.Top, style.BorderStyle.Top, style.BorderColor.Top = parent.BorderWidth.Top, parent.BorderStyle.Top, parent.BorderColor.Top
	case propertyBorderRight:
		style.BorderWidth.Right, style.BorderStyle.Right, style.BorderColor.Right = parent.BorderWidth.Right, parent.BorderStyle.Right, parent.BorderColor.Right
	case propertyBorderBottom:
		style.BorderWidth.Bottom, style.BorderStyle.Bottom, style.BorderColor.Bottom = parent.BorderWidth.Bottom, parent.BorderStyle.Bottom, parent.BorderColor.Bottom
	case propertyBorderLeft:
		style.BorderWidth.Left, style.BorderStyle.Left, style.BorderColor.Left = parent.BorderWidth.Left, parent.BorderStyle.Left, parent.BorderColor.Left
	}
}

func copyBorderStyleSide(style, parent *ComputedStyle, property propertyID) {
	switch property {
	case propertyBorderTopStyle:
		style.BorderStyle.Top = parent.BorderStyle.Top
	case propertyBorderRightStyle:
		style.BorderStyle.Right = parent.BorderStyle.Right
	case propertyBorderBottomStyle:
		style.BorderStyle.Bottom = parent.BorderStyle.Bottom
	case propertyBorderLeftStyle:
		style.BorderStyle.Left = parent.BorderStyle.Left
	}
}

func copyBorderWidthSide(style, parent *ComputedStyle, property propertyID) {
	switch property {
	case propertyBorderTopWidth:
		style.BorderWidth.Top = parent.BorderWidth.Top
	case propertyBorderRightWidth:
		style.BorderWidth.Right = parent.BorderWidth.Right
	case propertyBorderBottomWidth:
		style.BorderWidth.Bottom = parent.BorderWidth.Bottom
	case propertyBorderLeftWidth:
		style.BorderWidth.Left = parent.BorderWidth.Left
	}
}

func copyBorderColorSide(style, parent *ComputedStyle, property propertyID) {
	switch property {
	case propertyBorderTopColor:
		style.BorderColor.Top = parent.BorderColor.Top
	case propertyBorderRightColor:
		style.BorderColor.Right = parent.BorderColor.Right
	case propertyBorderBottomColor:
		style.BorderColor.Bottom = parent.BorderColor.Bottom
	case propertyBorderLeftColor:
		style.BorderColor.Left = parent.BorderColor.Left
	}
}
