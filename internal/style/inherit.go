package style

func inherit(parent *ComputedStyle, current *ComputedStyle, specified map[string]cascadedValue) {
	if parent == nil {
		return
	}

	// Explicit pointer-events values override this in applyProperty; inherit
	// and unset retain the inherited value.
	current.Visibility = parent.Visibility
	current.PointerEvents = parent.PointerEvents
	// CSS Cascade 4 §7: inherit/unset cascade to the parent's computed value,
	// so treat them as unspecified here and let applySpecifiedValues skip the
	// declaration. initial/revert stay out of the copy so the element default
	// from defaultStyle survives when applySpecifiedValues resolves them.
	if inheritsFromParent(specified, "font") {
		current.FontSize, current.FontFamily, current.FontWeight = parent.FontSize, parent.FontFamily, parent.FontWeight
		current.LineHeight, current.LineHeightNormal = parent.LineHeight, parent.LineHeightNormal
	}
	if inheritsFromParent(specified, "color") {
		current.Color = parent.Color
	}
	if inheritsFromParent(specified, "placeholder-color") {
		current.PlaceholderColor = parent.PlaceholderColor
	}
	if inheritsFromParent(specified, "text-align") {
		current.TextAlign = parent.TextAlign
	}
	if inheritsFromParent(specified, "text-box-edge") {
		current.TextBoxEdge = parent.TextBoxEdge
	}
	if inheritsFromParent(specified, "text-decoration-line") {
		current.TextDecorationLine = parent.TextDecorationLine
	}
	if inheritsFromParent(specified, "text-indent") {
		current.TextIndent = parent.TextIndent
	}
	if inheritsFromParent(specified, "letter-spacing") {
		current.LetterSpacing = parent.LetterSpacing
	}
	if inheritsFromParent(specified, "font-size") {
		current.FontSize = parent.FontSize
	}
	if inheritsFromParent(specified, "line-height") {
		current.LineHeight = parent.LineHeight
		current.LineHeightNormal = parent.LineHeightNormal
	}
	if inheritsFromParent(specified, "font-family") {
		current.FontFamily = parent.FontFamily
	}
	if inheritsFromParent(specified, "font-weight") {
		current.FontWeight = parent.FontWeight
	}
	if inheritsFromParent(specified, "white-space") {
		current.WhiteSpace = parent.WhiteSpace
	}
	if inheritsFromParent(specified, "tab-size") {
		current.TabSize = parent.TabSize
	}
	if inheritsFromParent(specified, "word-break") {
		current.WordBreak = parent.WordBreak
	}
	if inheritsFromParent(specified, "overflow-wrap") {
		current.OverflowWrap = parent.OverflowWrap
	}
	if inheritsFromParent(specified, "text-overflow") {
		current.TextOverflow = parent.TextOverflow
	}
	if inheritsFromParent(specified, "list-style-type") {
		current.ListStyleType = parent.ListStyleType
	}
	if inheritsFromParent(specified, "border-spacing") {
		current.BorderSpacingX, current.BorderSpacingY = parent.BorderSpacingX, parent.BorderSpacingY
	}
}

// inheritsFromParent reports whether an inherited property takes its computed
// value from the parent: either nothing cascaded, or inherit/unset.
func inheritsFromParent(specified map[string]cascadedValue, property string) bool {
	value, ok := specified[property]
	if !ok {
		return true
	}
	return isWideKeywordOf(value.value, "inherit") || isWideKeywordOf(value.value, "unset")
}
