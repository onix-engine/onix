package style

import "github.com/diiyw/onix/html/dom"

const defaultTextInputWidthPx = 140.0

func defaultStyle(node *dom.Node) ComputedStyle {
	display := Keyword("inline")
	if node == nil || node.Type == dom.DocumentNode || isBlockTag(node.Data) {
		display = Keyword("block")
	}

	computed := ComputedStyle{styleCore: styleCore{
		Display:                  display,
		TableLayout:              Keyword("auto"),
		BorderSpacingX:           LengthPx(0),
		BorderSpacingY:           LengthPx(0),
		VerticalAlign:            Keyword("baseline"),
		Visibility:               Keyword("visible"),
		PointerEvents:            Keyword("auto"),
		Appearance:               Keyword("auto"),
		BoxSizing:                Keyword("content-box"),
		Width:                    Auto(),
		Height:                   Auto(),
		MinWidth:                 Auto(),
		MinHeight:                Auto(),
		MaxWidth:                 Auto(),
		MaxHeight:                Auto(),
		BackgroundImage:          "",
		BackgroundSizeX:          Keyword("auto"),
		BackgroundSizeY:          Keyword("auto"),
		BackgroundPosX:           Number(0),
		BackgroundPosY:           Number(0),
		BackgroundPosXOrigin:     "left",
		BackgroundPosYOrigin:     "top",
		BackgroundRepeatX:        Keyword("repeat"),
		BackgroundRepeatY:        Keyword("repeat"),
		BackgroundOrigin:         "padding-box",
		BackgroundClip:           "border-box",
		BackgroundAttachment:     "scroll",
		Margin:                   uniformEdges(LengthPx(0)),
		Padding:                  uniformEdges(LengthPx(0)),
		BorderWidth:              uniformEdges(LengthPx(0)),
		BorderColor:              uniformEdges(currentColorValue()),
		BorderStyle:              uniformEdges(Keyword("none")),
		BorderRadius:             uniformCorners(LengthPx(0)),
		BackgroundColor:          ColorValue(Color{A: 0}),
		Color:                    ColorValue(Color{R: 0, G: 0, B: 0, A: 255}),
		PlaceholderColor:         ColorValue(Color{R: 117, G: 117, B: 117, A: 255}),
		TextAlign:                Keyword("left"),
		TextBoxTrim:              Keyword("none"),
		TextBoxEdge:              Keyword("auto"),
		TextDecorationLine:       Keyword("none"),
		TextIndent:               LengthPx(0),
		LetterSpacing:            LengthPx(0),
		TabSize:                  Number(8),
		FontSize:                 LengthPx(16),
		LineHeight:               LengthPx(19.2),
		LineHeightNormal:         true,
		FontFamily:               Keyword("sans-serif"),
		FontWeight:               Number(400),
		ListStyleType:            Keyword("disc"),
		WhiteSpace:               Keyword("normal"),
		WordBreak:                Keyword("normal"),
		OverflowWrap:             Keyword("normal"),
		TextOverflow:             Keyword("clip"),
		FlexDirection:            Keyword("row"),
		FlexWrap:                 Keyword("nowrap"),
		GridTemplateColumns:      "",
		GridTemplateRows:         "",
		GridColumnStart:          Auto(),
		GridColumnEnd:            Auto(),
		GridRowStart:             Auto(),
		GridRowEnd:               Auto(),
		JustifyContent:           Keyword("flex-start"),
		JustifyItems:             Keyword("stretch"),
		JustifySelf:              Keyword("auto"),
		AlignItems:               Keyword("stretch"),
		AlignSelf:                Keyword("auto"),
		AlignContent:             Keyword("stretch"),
		FlexGrow:                 Number(0),
		FlexShrink:               Number(1),
		FlexBasis:                Auto(),
		Order:                    Number(0),
		RowGap:                   LengthPx(0),
		ColumnGap:                LengthPx(0),
		ContainerType:            Keyword("normal"),
		ContainerName:            "",
		Position:                 Keyword("static"),
		Float:                    Keyword("none"),
		Clear:                    Keyword("none"),
		ZIndex:                   Keyword("auto"),
		Opacity:                  Number(1),
		TransformTranslateX:      LengthPx(0),
		TransformTranslateY:      LengthPx(0),
		TransformScaleX:          1,
		TransformScaleY:          1,
		TransformOriginX:         LengthPercent(.5),
		TransformOriginY:         LengthPercent(.5),
		TransformTransition:      TransformTransition{Easing: TransformEase},
		OverflowX:                Keyword("visible"),
		OverflowY:                Keyword("visible"),
		ScrollbarWidth:           Keyword("auto"),
		ScrollbarTrackColor:      Keyword("auto"),
		ScrollbarThumbColor:      Keyword("auto"),
		ScrollbarThumbHoverColor: Keyword("auto"),
		Top:                      Auto(),
		Right:                    Auto(),
		Bottom:                   Auto(),
		Left:                     Auto(),
		ObjectFit:                Keyword("fill"),
		ObjectPosX:               Number(0.5),
		ObjectPosY:               Number(0.5),
		ObjectPosXOrigin:         "left",
		ObjectPosYOrigin:         "top",
	}}
	if node != nil && node.Type == dom.ElementNode {
		switch node.Data {
		case "li":
			computed.Display = Keyword("list-item")
		case "table":
			computed.Display = Keyword("table")
			computed.BorderSpacingX, computed.BorderSpacingY = LengthPx(2), LengthPx(2)
		case "thead":
			computed.Display = Keyword("table-header-group")
		case "tbody":
			computed.Display = Keyword("table-row-group")
		case "tfoot":
			computed.Display = Keyword("table-footer-group")
		case "tr":
			computed.Display = Keyword("table-row")
		case "td", "th":
			computed.Display = Keyword("table-cell")
			computed.VerticalAlign = Keyword("middle")
		case "caption":
			computed.Display = Keyword("table-caption")
		case "ul", "menu":
			computed.Padding.Left = LengthPx(40)
		case "ol":
			computed.Padding.Left = LengthPx(40)
			computed.ListStyleType = Keyword("decimal")
		}
	}

	applyFormControlDefaults(node, &computed)
	applyHiddenAttributeDefault(node, &computed)
	return computed
}

func applyHiddenAttributeDefault(node *dom.Node, computed *ComputedStyle) {
	if node != nil && node.Type == dom.ElementNode {
		if _, hidden := node.Attr("hidden"); hidden {
			computed.Display = Keyword("none")
		}
	}
}

func isBlockTag(tag string) bool {
	switch tag {
	case "address", "article", "aside", "blockquote", "body", "center", "dd", "div", "dl", "dt", "fieldset", "figcaption", "figure", "footer", "form",
		"h1", "h2", "h3", "h4", "h5", "h6", "header", "hgroup", "html", "legend", "li", "main", "nav", "ol", "p", "pre", "section", "table", "ul":
		return true
	default:
		return false
	}
}

func applyFormControlDefaults(node *dom.Node, computed *ComputedStyle) {
	if node == nil || computed == nil || node.Type != dom.ElementNode {
		return
	}

	switch node.Data {
	case "button":
		computed.Display = Keyword("inline-block")
		computed.TextAlign = Keyword("center")
		computed.Padding = Edges{Top: LengthPx(2), Right: LengthPx(6), Bottom: LengthPx(2), Left: LengthPx(6)}
		computed.BorderWidth = uniformEdges(LengthPx(1))
		computed.BorderStyle = uniformEdges(Keyword("solid"))
		computed.BorderColor = uniformEdges(ColorValue(Color{R: 118, G: 118, B: 118, A: 255}))
		computed.BorderRadius = uniformCorners(LengthPx(4))
		computed.BackgroundColor = ColorValue(Color{R: 239, G: 239, B: 239, A: 255})
	case "input":
		computed.Display = Keyword("inline-block")
		computed.WhiteSpace = Keyword("nowrap")
		computed.Padding = Edges{Top: LengthPx(2), Right: LengthPx(6), Bottom: LengthPx(2), Left: LengthPx(6)}
		computed.BorderWidth = uniformEdges(LengthPx(1))
		computed.BorderStyle = uniformEdges(Keyword("solid"))
		computed.BorderColor = uniformEdges(ColorValue(Color{R: 118, G: 118, B: 118, A: 255}))
		computed.BackgroundColor = ColorValue(Color{R: 255, G: 255, B: 255, A: 255})
		if isHiddenInput(node) {
			computed.Display = Keyword("none")
			computed.Padding = uniformEdges(LengthPx(0))
			computed.BorderWidth = uniformEdges(LengthPx(0))
			computed.BorderStyle = uniformEdges(Keyword("none"))
			computed.BackgroundColor = ColorValue(Color{A: 0})
			break
		}
		if isButtonInput(node) {
			computed.TextAlign = Keyword("center")
			computed.BorderRadius = uniformCorners(LengthPx(4))
			computed.BackgroundColor = ColorValue(Color{R: 239, G: 239, B: 239, A: 255})
		}
		if isChoiceInput(node) {
			computed.Padding = uniformEdges(LengthPx(0))
			computed.Width = LengthPx(13)
			computed.Height = LengthPx(13)
			if isRadioInput(node) {
				computed.BorderRadius = uniformCorners(LengthPx(999))
			}
		} else if !isButtonInput(node) {
			computed.Width = LengthPx(defaultTextInputWidthPx)
		}
	case "select":
		computed.Display = Keyword("inline-block")
		computed.WhiteSpace = Keyword("nowrap")
		computed.Padding = Edges{Top: LengthPx(2), Right: LengthPx(20), Bottom: LengthPx(2), Left: LengthPx(6)}
		computed.BorderWidth = uniformEdges(LengthPx(1))
		computed.BorderStyle = uniformEdges(Keyword("solid"))
		computed.BorderColor = uniformEdges(ColorValue(Color{R: 118, G: 118, B: 118, A: 255}))
		computed.BackgroundColor = ColorValue(Color{R: 255, G: 255, B: 255, A: 255})
	case "textarea":
		computed.Display = Keyword("inline-block")
		computed.WhiteSpace = Keyword("pre-wrap")
		computed.Padding = Edges{Top: LengthPx(2), Right: LengthPx(6), Bottom: LengthPx(2), Left: LengthPx(6)}
		computed.BorderWidth = uniformEdges(LengthPx(1))
		computed.BorderStyle = uniformEdges(Keyword("solid"))
		computed.BorderColor = uniformEdges(ColorValue(Color{R: 118, G: 118, B: 118, A: 255}))
		computed.BackgroundColor = ColorValue(Color{R: 255, G: 255, B: 255, A: 255})
	}

	if isDisabledControl(node) {
		computed.Color = ColorValue(Color{R: 106, G: 106, B: 106, A: 255})
		computed.BorderColor = uniformEdges(ColorValue(Color{R: 180, G: 180, B: 180, A: 255}))
		computed.BackgroundColor = ColorValue(Color{R: 240, G: 240, B: 240, A: 255})
	}
}

func isCheckboxInput(node *dom.Node) bool {
	return isInputType(node, "checkbox")
}

func isRadioInput(node *dom.Node) bool {
	return isInputType(node, "radio")
}

func isButtonInput(node *dom.Node) bool {
	return isInputType(node, "button") || isInputType(node, "submit") || isInputType(node, "reset")
}

func isHiddenInput(node *dom.Node) bool {
	return isInputType(node, "hidden")
}

func isChoiceInput(node *dom.Node) bool {
	return isCheckboxInput(node) || isRadioInput(node)
}

func isInputType(node *dom.Node, inputType string) bool {
	return dom.HasInputType(node, inputType)
}

func isDisabledControl(node *dom.Node) bool {
	if node == nil || node.Type != dom.ElementNode {
		return false
	}
	if _, ok := node.Attr("disabled"); !ok {
		return false
	}
	switch node.Data {
	case "button", "input", "select", "textarea":
		return true
	default:
		return false
	}
}
