package style

func inherit(parent *ComputedStyle, current *ComputedStyle, specified *cascadedValues) {
	if parent == nil {
		return
	}

	// Explicit pointer-events values override this in applyProperty; inherit
	// and unset retain the inherited value.
	current.Visibility = parent.Visibility
	current.PointerEvents = parent.PointerEvents
	current.Cursor = parent.Cursor
	// CSS Cascade 4 §7: inherit/unset cascade to the parent's computed value,
	// so treat them as unspecified here and let applySpecifiedValues skip the
	// declaration. initial/revert stay out of the copy so the element default
	// from defaultStyle survives when applySpecifiedValues resolves them.
	if inheritsFromParent(specified, propertyFont) {
		current.FontSize, current.FontFamily, current.FontWeight = parent.FontSize, parent.FontFamily, parent.FontWeight
		current.FontStyle = parent.FontStyle
		current.LineHeight, current.LineHeightNormal = parent.LineHeight, parent.LineHeightNormal
	}
	if inheritsFromParent(specified, propertyColor) {
		current.Color = parent.Color
	}
	if inheritsFromParent(specified, propertyPlaceholderColor) {
		current.PlaceholderColor = parent.PlaceholderColor
	}
	if inheritsFromParent(specified, propertyTextAlign) {
		current.TextAlign = parent.TextAlign
	}
	if inheritsFromParent(specified, propertyTextBoxEdge) {
		current.TextBoxEdge = parent.TextBoxEdge
	}
	if inheritsFromParent(specified, propertyTextDecorationLine) {
		current.TextDecorationLine = parent.TextDecorationLine
	}
	if inheritsFromParent(specified, propertyTextIndent) {
		current.TextIndent = parent.TextIndent
	}
	if inheritsFromParent(specified, propertyLetterSpacing) {
		current.LetterSpacing = parent.LetterSpacing
	}
	if inheritsFromParent(specified, propertyFontSize) {
		current.FontSize = parent.FontSize
	}
	if inheritsFromParent(specified, propertyLineHeight) {
		current.LineHeight = parent.LineHeight
		current.LineHeightNormal = parent.LineHeightNormal
	}
	if inheritsFromParent(specified, propertyFontFamily) {
		current.FontFamily = parent.FontFamily
	}
	if inheritsFromParent(specified, propertyFontStyle) {
		current.FontStyle = parent.FontStyle
	}
	if inheritsFromParent(specified, propertyFontWeight) {
		current.FontWeight = parent.FontWeight
	}
	if inheritsFromParent(specified, propertyWhiteSpace) {
		current.WhiteSpace = parent.WhiteSpace
	}
	if inheritsFromParent(specified, propertyTabSize) {
		current.TabSize = parent.TabSize
	}
	if inheritsFromParent(specified, propertyWordBreak) {
		current.WordBreak = parent.WordBreak
	}
	if inheritsFromParent(specified, propertyOverflowWrap) {
		current.OverflowWrap = parent.OverflowWrap
	}
	if inheritsFromParent(specified, propertyTextOverflow) {
		current.TextOverflow = parent.TextOverflow
	}
	if inheritsFromParent(specified, propertyListStyleType) {
		current.ListStyleType = parent.ListStyleType
	}
	if inheritsFromParent(specified, propertyBorderSpacing) {
		current.BorderSpacingX, current.BorderSpacingY = parent.BorderSpacingX, parent.BorderSpacingY
	}
}

// inheritsFromParent reports whether an inherited property takes its computed
// value from the parent: either nothing cascaded, or inherit/unset.
func inheritsFromParent(specified *cascadedValues, property propertyID) bool {
	value, ok := specified.get(property)
	if !ok {
		return true
	}
	return isWideKeywordOf(value.value, "inherit") || isWideKeywordOf(value.value, "unset")
}
