package layout

import (
	"math"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/html/style"
)

type floatArea struct {
	rect  Rect
	right bool
}

type floatContext struct{ areas []floatArea }

func isFloated(box *Box) bool {
	return box != nil && box.Style.Float.Kind == style.ValueKeyword && box.Style.Float.Keyword != "none" && !isAbsolutePositioned(box) && !isFlexContainer(box.Parent) && !isGridContainer(box.Parent)
}

func establishesFloatContext(box *Box) bool {
	if box.Parent == nil || isFloated(box) || isAbsolutePositioned(box) {
		return true
	}
	switch box.Style.Display.Keyword {
	case "flow-root", "inline-block", "flex", "inline-flex", "grid", "inline-grid", "table", "table-cell":
		return true
	}
	return box.Style.OverflowX.Keyword != "visible" && box.Style.OverflowX.Keyword != "clip" || box.Style.OverflowY.Keyword != "visible" && box.Style.OverflowY.Keyword != "clip"
}

func (floats *floatContext) bounds(x, y, width, height float64) (float64, float64, float64) {
	left, right, next := x, x+width, math.Inf(1)
	if floats == nil {
		return left, maxFloat(right-left, 0), next
	}
	for _, area := range floats.areas {
		r := area.rect
		if r.Y+r.Height <= y || r.Y >= y+maxFloat(height, .01) || r.X+r.Width <= x || r.X >= x+width {
			continue
		}
		if area.right {
			right = minFloat(right, r.X)
		} else {
			left = maxFloat(left, r.X+r.Width)
		}
		next = minFloat(next, r.Y+r.Height)
	}
	return left, maxFloat(right-left, 0), next
}

func (floats *floatContext) clearance(y float64, side string) float64 {
	if floats == nil || side == "none" {
		return y
	}
	for _, area := range floats.areas {
		if side == "both" || side == "right" && area.right || side == "left" && !area.right {
			y = maxFloat(y, area.rect.Y+area.rect.Height)
		}
	}
	return y
}

func layoutFloat(box *Box, originX, width, y, flowHeight, inlineWidth, inlineHeight float64, absolute Rect, face *font.Face, styles style.Map, ctx *layoutContext) {
	floats := ctx.floatContext
	y = floats.clearance(y, box.Style.Clear.Keyword)
	if len(floats.areas) > 0 {
		y = maxFloat(y, floats.areas[len(floats.areas)-1].rect.Y)
	}
	margin := resolveEdges(box.Style.Margin, width)
	extras := computedHorizontalBoxExtras(box.Style, width) + margin.Left + margin.Right
	outerWidth := 0.0
	if style.IsLength(box.Style.Width) {
		outerWidth = contentDimension(box.Style.Width, width, extras-margin.Left-margin.Right, isBorderBox(box.Style)) + extras
	} else {
		minimum, preferred := tableCellIntrinsicWidths(box, face, styles, ctx)
		_, available, _ := floats.bounds(originX, y, width, inlineHeight)
		outerWidth = min(max(minimum+margin.Left+margin.Right, available-inlineWidth), preferred+margin.Left+margin.Right)
	}
	left, available, next := floats.bounds(originX, y, width, inlineHeight)
	if outerWidth+inlineWidth > available && inlineWidth > 0 {
		y += inlineHeight
	}
	for {
		left, available, next = floats.bounds(originX, y, width, inlineHeight)
		if outerWidth <= available || math.IsInf(next, 1) {
			break
		}
		y = next
	}
	x := left
	if box.Style.Float.Keyword == "right" {
		x += available - outerWidth
	}
	layoutBlock(box, Rect{X: x, Y: y, Width: maxFloat(outerWidth, 0), Height: flowHeight}, absolute, face, styles, ctx)
	floats.areas = append(floats.areas, floatArea{rect: box.FlowMarginBox, right: box.Style.Float.Keyword == "right"})
}
