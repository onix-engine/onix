package layout

import "github.com/onix-engine/onix/html/style"

func flexBaselineAligned(box *Box, align string) bool {
	return align == "baseline" && box.Style.Margin.Top.Kind != style.ValueAuto && box.Style.Margin.Bottom.Kind != style.ValueAuto
}

func firstBoxBaseline(box *Box) float64 {
	if isFlexContainer(box) && box.flex != nil {
		// Flexbox 8.5: the first line exports its participating items' shared
		// baseline, or the first item's baseline when none participate.
		return box.Content.Y + box.flex.baseline
	}
	for _, fragment := range box.TextFragments {
		if len(fragment.Glyphs) != 0 {
			return fragment.Glyphs[0].Y
		}
	}
	for _, child := range box.Children {
		if !isAbsolutePositioned(child) && !isFloated(child) {
			return firstBoxBaseline(child)
		}
	}
	border := box.BorderBox()
	// Replaced elements without a text baseline use their border edge.
	return border.Y + border.Height
}

func lastBoxBaseline(box *Box) float64 {
	border := box.BorderBox()
	baseline := border.Y + border.Height
	if box.Style.OverflowX.Keyword != "visible" || box.Style.OverflowY.Keyword != "visible" {
		return baseline
	}
	found := false
	for _, fragment := range box.TextFragments {
		if len(fragment.Glyphs) != 0 {
			baseline, found = fragment.Glyphs[0].Y, true
		}
	}
	for _, child := range box.Children {
		if !isAbsolutePositioned(child) && !isFloated(child) {
			candidate := lastBoxBaseline(child)
			if !found || candidate > baseline {
				baseline = candidate
			}
			found = true
		}
	}
	return baseline
}
