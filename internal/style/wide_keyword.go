package style

import (
	"slices"
	"strings"

	"github.com/diiyw/onix/internal/dom"
)

// cssWideKeywords are the value keywords from CSS Cascade 4 section 7 that
// stand in for a declaration's cascaded value instead of a property value.
var cssWideKeywords = [...]string{"inherit", "initial", "unset", "revert", "revert-layer"}

// inheritedProperties lists the properties whose unset value inherits from the
// parent. The list mirrors the properties copied by inherit().
var inheritedProperties = map[string]struct{}{
	"font":                 {},
	"font-size":            {},
	"font-family":          {},
	"font-weight":          {},
	"visibility":           {},
	"pointer-events":       {},
	"color":                {},
	"placeholder-color":    {},
	"text-align":           {},
	"text-box-edge":        {},
	"text-decoration-line": {},
	"text-indent":          {},
	"letter-spacing":       {},
	"tab-size":             {},
	"line-height":          {},
	"white-space":          {},
	"word-break":           {},
	"overflow-wrap":        {},
	"word-wrap":            {},
	"text-overflow":        {},
	"list-style-type":      {},
	"border-spacing":       {},
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

func isInheritedProperty(property string) bool {
	_, inherited := inheritedProperties[property]
	return inherited
}

// applyCSSWideKeyword resolves one winning CSS-wide declaration. Inherit is
// valid for every property, not only inherited properties; unset inherits only
// where the property is inherited and resets all other properties.
func applyCSSWideKeyword(style, parent *ComputedStyle, node *dom.Node, property, raw string) {
	inherits := isWideKeywordOf(raw, "inherit") || isInheritedProperty(property) && (isWideKeywordOf(raw, "unset") || isWideKeywordOf(raw, "revert") || isWideKeywordOf(raw, "revert-layer"))
	if inherits && parent != nil {
		copyCSSProperty(style, parent, property)
		return
	}
	resetCSSProperty(style, node, property)
}

func resetCSSProperty(style *ComputedStyle, node *dom.Node, property string) {
	if style == nil {
		return
	}
	defaults := defaultStyle(node)
	copyCSSProperty(style, &defaults, property)
}

// copyCSSProperty copies the computed value represented by one declaration.
// Keeping this as a switch avoids reflection in the cascade hot path and also
// makes shorthand behavior explicit.
func copyCSSProperty(style, parent *ComputedStyle, property string) {
	if style == nil || parent == nil {
		return
	}
	switch property {
	case "font":
		style.FontSize, style.FontFamily, style.FontWeight = parent.FontSize, parent.FontFamily, parent.FontWeight
		style.LineHeight, style.LineHeightNormal = parent.LineHeight, parent.LineHeightNormal
	case "display":
		style.Display = parent.Display
	case "table-layout":
		style.TableLayout = parent.TableLayout
	case "border-spacing":
		style.BorderSpacingX, style.BorderSpacingY = parent.BorderSpacingX, parent.BorderSpacingY
	case "vertical-align":
		style.VerticalAlign = parent.VerticalAlign
	case "grid-template-columns":
		style.GridTemplateColumns = parent.GridTemplateColumns
	case "grid-template-rows":
		style.GridTemplateRows = parent.GridTemplateRows
	case "grid-column":
		style.GridColumnStart, style.GridColumnEnd = parent.GridColumnStart, parent.GridColumnEnd
	case "grid-row":
		style.GridRowStart, style.GridRowEnd = parent.GridRowStart, parent.GridRowEnd
	case "grid-column-start":
		style.GridColumnStart = parent.GridColumnStart
	case "grid-column-end":
		style.GridColumnEnd = parent.GridColumnEnd
	case "grid-row-start":
		style.GridRowStart = parent.GridRowStart
	case "grid-row-end":
		style.GridRowEnd = parent.GridRowEnd
	case "visibility":
		style.Visibility = parent.Visibility
	case "pointer-events":
		style.PointerEvents = parent.PointerEvents
	case "appearance":
		style.Appearance = parent.Appearance
	case "box-sizing":
		style.BoxSizing = parent.BoxSizing
	case "flex":
		style.FlexGrow, style.FlexShrink, style.FlexBasis = parent.FlexGrow, parent.FlexShrink, parent.FlexBasis
	case "flex-flow":
		style.FlexDirection, style.FlexWrap = parent.FlexDirection, parent.FlexWrap
	case "flex-direction":
		style.FlexDirection = parent.FlexDirection
	case "flex-wrap":
		style.FlexWrap = parent.FlexWrap
	case "flex-grow":
		style.FlexGrow = parent.FlexGrow
	case "flex-shrink":
		style.FlexShrink = parent.FlexShrink
	case "flex-basis":
		style.FlexBasis = parent.FlexBasis
	case "justify-content":
		style.JustifyContent = parent.JustifyContent
	case "justify-items":
		style.JustifyItems = parent.JustifyItems
	case "justify-self":
		style.JustifySelf = parent.JustifySelf
	case "align-items":
		style.AlignItems = parent.AlignItems
	case "align-self":
		style.AlignSelf = parent.AlignSelf
	case "align-content":
		style.AlignContent = parent.AlignContent
	case "order":
		style.Order = parent.Order
	case "gap":
		style.RowGap, style.ColumnGap = parent.RowGap, parent.ColumnGap
	case "row-gap":
		style.RowGap = parent.RowGap
	case "column-gap":
		style.ColumnGap = parent.ColumnGap
	case "container":
		style.ContainerName, style.ContainerType = parent.ContainerName, parent.ContainerType
	case "container-type":
		style.ContainerType = parent.ContainerType
	case "container-name":
		style.ContainerName = parent.ContainerName
	case "position":
		style.Position = parent.Position
	case "float":
		style.Float = parent.Float
	case "clear":
		style.Clear = parent.Clear
	case "inset":
		style.Top, style.Right, style.Bottom, style.Left = parent.Top, parent.Right, parent.Bottom, parent.Left
	case "opacity":
		style.Opacity = parent.Opacity
	case "transform":
		style.TransformRotation = parent.TransformRotation
		style.TransformTranslateX = parent.TransformTranslateX
		style.TransformTranslateY = parent.TransformTranslateY
		style.TransformScaleX, style.TransformScaleY = parent.TransformScaleX, parent.TransformScaleY
		style.TransformSkewX = parent.TransformSkewX
		style.TransformTranslateCrossX, style.TransformTranslateCrossY = parent.TransformTranslateCrossX, parent.TransformTranslateCrossY
	case "transform-origin":
		style.TransformOriginX, style.TransformOriginY = parent.TransformOriginX, parent.TransformOriginY
	case "transition":
		style.TransformTransition = parent.TransformTransition
		style.OpacityTransition = parent.OpacityTransition
		style.ColorTransitions = parent.ColorTransitions
		style.TransitionRaw = parent.TransitionRaw
	case "transition-property", "transition-duration", "transition-timing-function", "transition-delay":
		index := slices.Index(transitionProperties[:], property)
		style.TransitionRaw[index] = parent.TransitionRaw[index]
		resolveTransitions(style)
	case "animation":
		style.AnimationRaw = parent.AnimationRaw
	case "animation-name", "animation-duration", "animation-timing-function", "animation-delay", "animation-iteration-count", "animation-direction", "animation-fill-mode", "animation-play-state":
		index := slices.Index(animationProperties[:], property)
		style.AnimationRaw[index] = parent.AnimationRaw[index]
	case "z-index":
		style.ZIndex = parent.ZIndex
	case "overflow":
		style.OverflowX, style.OverflowY = parent.OverflowX, parent.OverflowY
	case "overflow-x":
		style.OverflowX = parent.OverflowX
	case "overflow-y":
		style.OverflowY = parent.OverflowY
	case "scrollbar-width":
		style.ScrollbarWidth = parent.ScrollbarWidth
	case "scrollbar-color":
		style.ScrollbarTrackColor, style.ScrollbarThumbColor = parent.ScrollbarTrackColor, parent.ScrollbarThumbColor
	case "scrollbar-track-color":
		style.ScrollbarTrackColor = parent.ScrollbarTrackColor
	case "scrollbar-thumb-color":
		style.ScrollbarThumbColor = parent.ScrollbarThumbColor
	case "scrollbar-thumb-hover-color":
		style.ScrollbarThumbHoverColor = parent.ScrollbarThumbHoverColor
	case "width":
		style.Width = parent.Width
	case "height":
		style.Height = parent.Height
	case "min-width":
		style.MinWidth = parent.MinWidth
	case "min-height":
		style.MinHeight = parent.MinHeight
	case "max-width":
		style.MaxWidth = parent.MaxWidth
	case "max-height":
		style.MaxHeight = parent.MaxHeight
	case "margin":
		style.Margin = parent.Margin
	case "margin-top":
		style.Margin.Top = parent.Margin.Top
	case "margin-right":
		style.Margin.Right = parent.Margin.Right
	case "margin-bottom":
		style.Margin.Bottom = parent.Margin.Bottom
	case "margin-left":
		style.Margin.Left = parent.Margin.Left
	case "padding":
		style.Padding = parent.Padding
	case "padding-top":
		style.Padding.Top = parent.Padding.Top
	case "padding-right":
		style.Padding.Right = parent.Padding.Right
	case "padding-bottom":
		style.Padding.Bottom = parent.Padding.Bottom
	case "padding-left":
		style.Padding.Left = parent.Padding.Left
	case "border":
		style.BorderWidth, style.BorderStyle, style.BorderColor = parent.BorderWidth, parent.BorderStyle, parent.BorderColor
	case "border-top", "border-right", "border-bottom", "border-left":
		copyBorderSide(style, parent, property)
	case "border-width":
		style.BorderWidth = parent.BorderWidth
	case "border-style":
		style.BorderStyle = parent.BorderStyle
	case "border-color":
		style.BorderColor = parent.BorderColor
	case "border-top-style", "border-right-style", "border-bottom-style", "border-left-style":
		copyBorderStyleSide(style, parent, property)
	case "border-top-width", "border-right-width", "border-bottom-width", "border-left-width":
		copyBorderWidthSide(style, parent, property)
	case "border-top-color", "border-right-color", "border-bottom-color", "border-left-color":
		copyBorderColorSide(style, parent, property)
	case "border-radius":
		style.BorderRadius = parent.BorderRadius
	case "border-top-left-radius":
		style.BorderRadius.TopLeft = parent.BorderRadius.TopLeft
	case "border-top-right-radius":
		style.BorderRadius.TopRight = parent.BorderRadius.TopRight
	case "border-bottom-right-radius":
		style.BorderRadius.BottomRight = parent.BorderRadius.BottomRight
	case "border-bottom-left-radius":
		style.BorderRadius.BottomLeft = parent.BorderRadius.BottomLeft
	case "box-shadow":
		style.BoxShadow = slices.Clone(parent.BoxShadow)
	case "filter":
		style.Filters = slices.Clone(parent.Filters)
	case "background":
		copyBackground(style, parent)
	case "background-color":
		style.BackgroundColor = parent.BackgroundColor
	case "background-image":
		copyBackgroundImages(style, parent)
	case "background-position":
		copyBackgroundPositions(style, parent)
	case "background-repeat":
		copyBackgroundRepeats(style, parent)
	case "background-size":
		copyBackgroundSizes(style, parent)
	case "background-origin":
		copyBackgroundOrigins(style, parent)
	case "background-clip":
		copyBackgroundClips(style, parent)
	case "background-attachment":
		copyBackgroundAttachments(style, parent)
	case "color":
		style.Color = parent.Color
	case "placeholder-color":
		style.PlaceholderColor = parent.PlaceholderColor
	case "text-align":
		style.TextAlign = parent.TextAlign
	case "text-box-trim":
		style.TextBoxTrim = parent.TextBoxTrim
	case "text-box-edge":
		style.TextBoxEdge = parent.TextBoxEdge
	case "text-decoration-line":
		style.TextDecorationLine = parent.TextDecorationLine
	case "text-indent":
		style.TextIndent = parent.TextIndent
	case "letter-spacing":
		style.LetterSpacing = parent.LetterSpacing
	case "font-size":
		style.FontSize = parent.FontSize
	case "line-height":
		style.LineHeight, style.LineHeightNormal = parent.LineHeight, parent.LineHeightNormal
	case "font-family":
		style.FontFamily = parent.FontFamily
	case "font-weight":
		style.FontWeight = parent.FontWeight
	case "white-space":
		style.WhiteSpace = parent.WhiteSpace
	case "tab-size":
		style.TabSize = parent.TabSize
	case "word-break":
		style.WordBreak = parent.WordBreak
	case "overflow-wrap", "word-wrap":
		style.OverflowWrap = parent.OverflowWrap
	case "text-overflow":
		style.TextOverflow = parent.TextOverflow
	case "list-style-type":
		style.ListStyleType = parent.ListStyleType
	case "top":
		style.Top = parent.Top
	case "right":
		style.Right = parent.Right
	case "bottom":
		style.Bottom = parent.Bottom
	case "left":
		style.Left = parent.Left
	case "object-fit":
		style.ObjectFit = parent.ObjectFit
	case "object-position":
		style.ObjectPosX, style.ObjectPosY = parent.ObjectPosX, parent.ObjectPosY
		style.ObjectPosXOrigin, style.ObjectPosYOrigin = parent.ObjectPosXOrigin, parent.ObjectPosYOrigin
	}
}

func copyBackground(style, parent *ComputedStyle) {
	style.BackgroundLayers = slices.Clone(parent.BackgroundLayers)
	style.BackgroundImage = parent.BackgroundImage
	style.BackgroundSizeX, style.BackgroundSizeY = parent.BackgroundSizeX, parent.BackgroundSizeY
	style.BackgroundPosX, style.BackgroundPosY = parent.BackgroundPosX, parent.BackgroundPosY
	style.BackgroundPosXOrigin, style.BackgroundPosYOrigin = parent.BackgroundPosXOrigin, parent.BackgroundPosYOrigin
	style.BackgroundRepeatX, style.BackgroundRepeatY = parent.BackgroundRepeatX, parent.BackgroundRepeatY
	style.BackgroundOrigin, style.BackgroundClip, style.BackgroundAttachment = parent.BackgroundOrigin, parent.BackgroundClip, parent.BackgroundAttachment
	style.BackgroundColor = parent.BackgroundColor
}

func copyBackgroundImages(style, parent *ComputedStyle) {
	if len(parent.BackgroundLayers) == 0 {
		style.BackgroundLayers = nil
		style.BackgroundImage = ""
		return
	}
	if len(style.BackgroundLayers) < len(parent.BackgroundLayers) {
		ensureBackgroundLayerCount(style, len(parent.BackgroundLayers))
	}
	style.BackgroundLayers = style.BackgroundLayers[:len(parent.BackgroundLayers)]
	for index := range style.BackgroundLayers {
		style.BackgroundLayers[index].Image = parent.BackgroundLayers[index].Image
	}
	style.BackgroundImage = parent.BackgroundImage
}

func copyBackgroundPositions(style, parent *ComputedStyle) {
	copyBackgroundLayerField(style, parent, func(target *BackgroundLayer, source BackgroundLayer) {
		target.PosX, target.PosY = source.PosX, source.PosY
		target.PosXOrigin, target.PosYOrigin = source.PosXOrigin, source.PosYOrigin
	})
	if len(style.BackgroundLayers) > 0 {
		first := style.BackgroundLayers[0]
		style.BackgroundPosX, style.BackgroundPosY = first.PosX, first.PosY
		style.BackgroundPosXOrigin, style.BackgroundPosYOrigin = first.PosXOrigin, first.PosYOrigin
	} else {
		style.BackgroundPosX, style.BackgroundPosY = parent.BackgroundPosX, parent.BackgroundPosY
		style.BackgroundPosXOrigin, style.BackgroundPosYOrigin = parent.BackgroundPosXOrigin, parent.BackgroundPosYOrigin
	}
}

func copyBackgroundRepeats(style, parent *ComputedStyle) {
	copyBackgroundLayerField(style, parent, func(target *BackgroundLayer, source BackgroundLayer) {
		target.RepeatX, target.RepeatY = source.RepeatX, source.RepeatY
	})
	if len(style.BackgroundLayers) > 0 {
		first := style.BackgroundLayers[0]
		style.BackgroundRepeatX, style.BackgroundRepeatY = first.RepeatX, first.RepeatY
	} else {
		style.BackgroundRepeatX, style.BackgroundRepeatY = parent.BackgroundRepeatX, parent.BackgroundRepeatY
	}
}

func copyBackgroundSizes(style, parent *ComputedStyle) {
	copyBackgroundLayerField(style, parent, func(target *BackgroundLayer, source BackgroundLayer) {
		target.SizeX, target.SizeY = source.SizeX, source.SizeY
	})
	if len(style.BackgroundLayers) > 0 {
		first := style.BackgroundLayers[0]
		style.BackgroundSizeX, style.BackgroundSizeY = first.SizeX, first.SizeY
	} else {
		style.BackgroundSizeX, style.BackgroundSizeY = parent.BackgroundSizeX, parent.BackgroundSizeY
	}
}

func copyBackgroundOrigins(style, parent *ComputedStyle) {
	copyBackgroundLayerField(style, parent, func(target *BackgroundLayer, source BackgroundLayer) {
		target.Origin = source.Origin
	})
	if len(style.BackgroundLayers) > 0 {
		style.BackgroundOrigin = style.BackgroundLayers[0].Origin
	} else {
		style.BackgroundOrigin = parent.BackgroundOrigin
	}
}

func copyBackgroundClips(style, parent *ComputedStyle) {
	copyBackgroundLayerField(style, parent, func(target *BackgroundLayer, source BackgroundLayer) {
		target.Clip = source.Clip
	})
	if len(style.BackgroundLayers) > 0 {
		style.BackgroundClip = style.BackgroundLayers[0].Clip
	} else {
		style.BackgroundClip = parent.BackgroundClip
	}
}

func copyBackgroundAttachments(style, parent *ComputedStyle) {
	copyBackgroundLayerField(style, parent, func(target *BackgroundLayer, source BackgroundLayer) {
		target.Attachment = source.Attachment
	})
	if len(style.BackgroundLayers) > 0 {
		style.BackgroundAttachment = style.BackgroundLayers[0].Attachment
	} else {
		style.BackgroundAttachment = parent.BackgroundAttachment
	}
}

func copyBackgroundLayerField(style, parent *ComputedStyle, copyField func(*BackgroundLayer, BackgroundLayer)) {
	count := len(style.BackgroundLayers)
	if count < len(parent.BackgroundLayers) {
		count = len(parent.BackgroundLayers)
		ensureBackgroundLayerCount(style, count)
	}
	if count == 0 {
		return
	}
	for index := 0; index < count; index++ {
		source := backgroundLayerAt(parent, index)
		copyField(&style.BackgroundLayers[index], source)
	}
}

func backgroundLayerAt(style *ComputedStyle, index int) BackgroundLayer {
	if len(style.BackgroundLayers) > 0 {
		if index >= len(style.BackgroundLayers) {
			index = len(style.BackgroundLayers) - 1
		}
		return style.BackgroundLayers[index]
	}
	return BackgroundLayer{
		Image:      style.BackgroundImage,
		SizeX:      style.BackgroundSizeX,
		SizeY:      style.BackgroundSizeY,
		PosX:       style.BackgroundPosX,
		PosY:       style.BackgroundPosY,
		PosXOrigin: style.BackgroundPosXOrigin,
		PosYOrigin: style.BackgroundPosYOrigin,
		RepeatX:    style.BackgroundRepeatX,
		RepeatY:    style.BackgroundRepeatY,
		Origin:     style.BackgroundOrigin,
		Clip:       style.BackgroundClip,
		Attachment: style.BackgroundAttachment,
	}
}

func copyBorderSide(style, parent *ComputedStyle, property string) {
	switch property {
	case "border-top":
		style.BorderWidth.Top, style.BorderStyle.Top, style.BorderColor.Top = parent.BorderWidth.Top, parent.BorderStyle.Top, parent.BorderColor.Top
	case "border-right":
		style.BorderWidth.Right, style.BorderStyle.Right, style.BorderColor.Right = parent.BorderWidth.Right, parent.BorderStyle.Right, parent.BorderColor.Right
	case "border-bottom":
		style.BorderWidth.Bottom, style.BorderStyle.Bottom, style.BorderColor.Bottom = parent.BorderWidth.Bottom, parent.BorderStyle.Bottom, parent.BorderColor.Bottom
	case "border-left":
		style.BorderWidth.Left, style.BorderStyle.Left, style.BorderColor.Left = parent.BorderWidth.Left, parent.BorderStyle.Left, parent.BorderColor.Left
	}
}

func copyBorderStyleSide(style, parent *ComputedStyle, property string) {
	switch property {
	case "border-top-style":
		style.BorderStyle.Top = parent.BorderStyle.Top
	case "border-right-style":
		style.BorderStyle.Right = parent.BorderStyle.Right
	case "border-bottom-style":
		style.BorderStyle.Bottom = parent.BorderStyle.Bottom
	case "border-left-style":
		style.BorderStyle.Left = parent.BorderStyle.Left
	}
}

func copyBorderWidthSide(style, parent *ComputedStyle, property string) {
	switch property {
	case "border-top-width":
		style.BorderWidth.Top = parent.BorderWidth.Top
	case "border-right-width":
		style.BorderWidth.Right = parent.BorderWidth.Right
	case "border-bottom-width":
		style.BorderWidth.Bottom = parent.BorderWidth.Bottom
	case "border-left-width":
		style.BorderWidth.Left = parent.BorderWidth.Left
	}
}

func copyBorderColorSide(style, parent *ComputedStyle, property string) {
	switch property {
	case "border-top-color":
		style.BorderColor.Top = parent.BorderColor.Top
	case "border-right-color":
		style.BorderColor.Right = parent.BorderColor.Right
	case "border-bottom-color":
		style.BorderColor.Bottom = parent.BorderColor.Bottom
	case "border-left-color":
		style.BorderColor.Left = parent.BorderColor.Left
	}
}
