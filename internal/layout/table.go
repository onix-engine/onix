package layout

import (
	"slices"
	"strconv"
	"strings"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/internal/style"
)

type tableColumn struct {
	minimum, preferred, width, x float64
	definite                     bool
}

type tableCell struct {
	box                              *Box
	row, column, rowSpan, columnSpan int
}

type tableRow struct {
	box       *Box
	height, y float64
	groupEnd  int
}

type tableScratch struct {
	rows     []tableRow
	cells    []tableCell
	columns  []tableColumn
	occupied []int
}

func layoutTableContainer(box *Box, containing, absolute Rect, face *font.Face, styles style.Map, ctx *layoutContext) {
	if box.table == nil {
		box.table = &tableScratch{}
	}
	scratch := box.table
	clear(scratch.rows)
	scratch.rows = scratch.rows[:0]
	clear(scratch.cells)
	scratch.cells = scratch.cells[:0]
	for _, child := range box.Children {
		if child.Style.Display.Keyword == "table-row" {
			scratch.rows = append(scratch.rows, tableRow{box: child})
		} else if isTableRowGroup(child) {
			for _, row := range child.Children {
				if row.Style.Display.Keyword == "table-row" {
					scratch.rows = append(scratch.rows, tableRow{box: row})
				}
			}
		}
	}
	slices.SortStableFunc(scratch.rows, func(left, right tableRow) int { return tableRowOrder(left.box) - tableRowOrder(right.box) })
	groupEnd := len(scratch.rows)
	for index := len(scratch.rows) - 1; index >= 0; index-- {
		if index == len(scratch.rows)-1 || scratch.rows[index].box.Parent != scratch.rows[index+1].box.Parent {
			groupEnd = index + 1
		}
		scratch.rows[index].groupEnd = groupEnd
	}
	clear(scratch.occupied)
	scratch.occupied = scratch.occupied[:0]
	columnCount := 0
	for rowIndex, row := range scratch.rows {
		column := 0
		for _, child := range row.box.Children {
			if child.Style.Display.Keyword != "table-cell" || isAbsolutePositioned(child) {
				continue
			}
			for column < len(scratch.occupied) && scratch.occupied[column] > rowIndex {
				column++
			}
			columnSpan := tableSpan(child, "colspan", 1000)
			rowSpan := tableSpan(child, "rowspan", min(65534, row.groupEnd-rowIndex))
			end := column + columnSpan
			if len(scratch.occupied) < end {
				old := len(scratch.occupied)
				scratch.occupied = slices.Grow(scratch.occupied, end-old)[:end]
				clear(scratch.occupied[old:])
			}
			for i := column; i < end; i++ {
				scratch.occupied[i] = rowIndex + rowSpan
			}
			scratch.cells = append(scratch.cells, tableCell{box: child, row: rowIndex, column: column, rowSpan: rowSpan, columnSpan: columnSpan})
			column = end
		}
		columnCount = max(columnCount, column)
	}
	scratch.columns = slices.Grow(scratch.columns[:0], columnCount)[:columnCount]
	clear(scratch.columns)
	spacingX := resolveLength(box.Style.BorderSpacingX, 0)
	spacingY := resolveLength(box.Style.BorderSpacingY, 0)
	fixed := box.Style.TableLayout.Keyword == "fixed" && style.IsLength(box.Style.Width)
	available := maxFloat(box.Content.Width-spacingX*float64(columnCount+1), 0)
	for _, cell := range scratch.cells {
		if fixed && cell.row != 0 {
			continue
		}
		minimum, preferred := 0.0, 0.0
		if !fixed {
			minimum, preferred = tableCellIntrinsicWidths(cell.box, face, styles, ctx)
		}
		definite := style.IsLength(cell.box.Style.Width)
		if definite {
			width := borderBoxDimension(cell.box.Style.Width, available, computedHorizontalBoxExtras(cell.box.Style, available), isBorderBox(cell.box.Style))
			minimum, preferred = max(minimum, width), max(preferred, width)
		}
		minimum = maxFloat(minimum-spacingX*float64(cell.columnSpan-1), 0)
		preferred = maxFloat(preferred-spacingX*float64(cell.columnSpan-1), 0)
		currentMin, currentPreferred := 0.0, 0.0
		for _, column := range scratch.columns[cell.column : cell.column+cell.columnSpan] {
			currentMin += column.minimum
			currentPreferred += column.preferred
		}
		for i := cell.column; i < cell.column+cell.columnSpan; i++ {
			column := &scratch.columns[i]
			column.minimum += maxFloat(minimum-currentMin, 0) / float64(cell.columnSpan)
			column.preferred += maxFloat(preferred-currentPreferred, 0) / float64(cell.columnSpan)
			column.definite = column.definite || definite
		}
	}
	minimum, preferred := 0.0, 0.0
	for _, column := range scratch.columns {
		minimum += column.minimum
		preferred += max(column.preferred, column.minimum)
	}
	if !fixed && box.Style.Width.Kind == style.ValueAuto {
		available = max(minimum, min(available, preferred))
		box.Content.Width = available + spacingX*float64(columnCount+1)
	} else if !fixed {
		available = max(available, minimum)
		box.Content.Width = available + spacingX*float64(columnCount+1)
	}
	resolveTableColumns(scratch.columns, available, minimum, preferred, fixed)
	x := box.Content.X + spacingX
	for index := range scratch.columns {
		column := &scratch.columns[index]
		column.x = x
		x += column.width + spacingX
	}
	abs := descendantAbsoluteContaining(box, absolute)
	y := box.Content.Y
	for _, caption := range box.Children {
		if caption.Style.Display.Keyword == "table-caption" {
			layoutBlock(caption, Rect{X: box.Content.X, Y: y, Width: box.Content.Width, Height: containing.Height}, abs, face, styles, ctx)
			y += caption.MarginBox().Height
		}
	}
	y += spacingY
	for _, cell := range scratch.cells {
		first, last := scratch.columns[cell.column], scratch.columns[cell.column+cell.columnSpan-1]
		x, width := first.x, last.x+last.width-first.x
		layoutTableCell(cell.box, Rect{X: x, Y: y, Width: width, Height: containing.Height}, abs, face, styles, ctx)
		if cell.rowSpan == 1 {
			scratch.rows[cell.row].height = max(scratch.rows[cell.row].height, cell.box.BorderBox().Height)
		}
	}
	for _, cell := range scratch.cells {
		if cell.rowSpan <= 1 {
			continue
		}
		height := spacingY * float64(cell.rowSpan-1)
		for _, row := range scratch.rows[cell.row : cell.row+cell.rowSpan] {
			height += row.height
		}
		additional := maxFloat(cell.box.BorderBox().Height-height, 0) / float64(cell.rowSpan)
		for i := cell.row; i < cell.row+cell.rowSpan; i++ {
			scratch.rows[i].height += additional
		}
	}
	var group *Box
	var groupRect Rect
	for i := range scratch.rows {
		row := &scratch.rows[i]
		if style.IsLength(row.box.Style.Height) {
			row.height = max(row.height, resolveLength(row.box.Style.Height, containing.Height))
		}
		row.y = y
		setTableRect(row.box, Rect{X: box.Content.X + spacingX, Y: y, Width: maxFloat(box.Content.Width-2*spacingX, 0), Height: row.height}, ctx)
		parent := row.box.Parent
		if parent != group {
			if group != nil {
				setTableRect(group, groupRect, ctx)
			}
			group = nil
			if parent != nil && isTableRowGroup(parent) {
				group = parent
				groupRect = row.box.Content
			}
		} else if group != nil {
			groupRect.Height = y + row.height - groupRect.Y
		}
		y += row.height + spacingY
	}
	if group != nil {
		setTableRect(group, groupRect, ctx)
	}
	for _, cell := range scratch.cells {
		height := spacingY * float64(cell.rowSpan-1)
		for _, row := range scratch.rows[cell.row : cell.row+cell.rowSpan] {
			height += row.height
		}
		child := cell.box
		translateBox(child, 0, scratch.rows[cell.row].y-child.BorderBox().Y, ctx)
		extra := maxFloat(height-child.BorderBox().Height, 0)
		offset := 0.0
		switch child.Style.VerticalAlign.Keyword {
		case "middle":
			offset = extra / 2
		case "bottom":
			offset = extra
		}
		if offset != 0 {
			translateTableCellContent(child, offset, ctx)
		}
		child.Content.Height += extra
		child.FlowMarginBox = child.MarginBox()
		// Row stretching changes geometry outside layoutBlock's cached inputs.
		child.reflow.valid = false
	}
	if !style.IsLength(box.Style.Height) {
		box.Content.Height = maxFloat(y-box.Content.Y, 0)
	}
	box.Content.Height = clampContentDimension(box.Content.Height, box.Style.MinHeight, box.Style.MaxHeight, containing.Height, verticalBoxExtras(box.Padding, box.Border), isBorderBox(box.Style))
	box.FlowMarginBox = box.MarginBox()
	clear(box.TextFragments)
	box.TextFragments = box.TextFragments[:0]
	clear(box.ImageFragments)
	box.ImageFragments = box.ImageFragments[:0]
	clear(box.ControlFragments)
	box.ControlFragments = box.ControlFragments[:0]
	if isRelativePositioned(box) {
		translateBox(box, resolveRelativeOffsetX(box.Style, containing.Width), resolveRelativeOffsetY(box.Style, containing.Height), ctx)
	}
}

func resolveTableColumns(columns []tableColumn, available, minimum, preferred float64, fixed bool) {
	if len(columns) == 0 {
		return
	}
	if fixed {
		used, unspecified := 0.0, 0
		for i := range columns {
			if columns[i].definite {
				columns[i].width = columns[i].minimum
				used += columns[i].width
			} else {
				unspecified++
			}
		}
		for i := range columns {
			if !columns[i].definite {
				columns[i].width = maxFloat(available-used, 0) / float64(max(unspecified, 1))
			} else if unspecified == 0 {
				columns[i].width += maxFloat(available-used, 0) / float64(len(columns))
			}
		}
		return
	}
	for i := range columns {
		column := &columns[i]
		column.width = column.minimum
		if preferred > minimum {
			column.width += maxFloat(min(available, preferred)-minimum, 0) * (max(column.preferred, column.minimum) - column.minimum) / (preferred - minimum)
		}
		if available > preferred {
			column.width += (available - preferred) / float64(len(columns))
		}
	}
}

func layoutTableCell(box *Box, containing, absolute Rect, face *font.Face, styles style.Map, ctx *layoutContext) {
	original := box.Style
	if box.tableMeasureStyle == nil {
		box.tableMeasureStyle = new(style.ComputedStyle)
	}
	*box.tableMeasureStyle = *original
	box.tableMeasureStyle.Margin = style.Edges{}
	box.tableMeasureStyle.Width = style.LengthPx(maxFloat(containing.Width-computedHorizontalBoxExtras(original, containing.Width), 0))
	box.tableMeasureStyle.BoxSizing = style.Keyword("content-box")
	box.Style = box.tableMeasureStyle
	layoutBlock(box, containing, absolute, face, styles, ctx)
	box.Style = original
}

func tableCellIntrinsicWidths(box *Box, face *font.Face, styles style.Map, ctx *layoutContext) (float64, float64) {
	if isFlexContainer(box) {
		return flexIntrinsicWidths(box, face, styles, ctx)
	}
	if box.intrinsicScratch == nil {
		box.intrinsicScratch = newLayoutContext()
	}
	if ctx != nil {
		box.intrinsicScratch.boldFace = ctx.boldFace
	}
	ctx = box.intrinsicScratch
	minimum, preferred, line := 0.0, 0.0, 0.0
	for _, segment := range collectInlineSegmentsWithIndex(box.Node, styles, ctx, nil) {
		if segment.Kind == inlineSegmentImage {
			width, _ := resolveInlineImageSize(segment.Node, segment.Style, ctx)
			minimum, line = max(minimum, width), line+width
			continue
		}
		if segment.Kind == inlineSegmentControl {
			if item, ok := newControlLineItem(segment.Node, segment.Style, face, ctx, styles, 0); ok {
				minimum, line = max(minimum, item.Width), line+item.Width
			}
			continue
		}
		textFace := fontForStyle(face, segment.Style, ctx)
		for _, token := range tokenizeStyledTextInto(ctx, segment.Text, whiteSpaceMode(segment.Style, false), line == 0, segment.Style.WordBreak.Keyword == "break-all") {
			if token.IsBreak {
				preferred, line = max(preferred, line), 0
				continue
			}
			if token.IsTab {
				// Preserved tabs advance from the current line position, including
				// preceding syntax spans, just as they do during inline layout.
				_, spaceWidth, _ := shapeToken(textFace, textToken{Text: " "}, segment.Style.FontSize.LengthPx, segment.Style.LetterSpacing.LengthPx, ctx)
				line += tabAdvance(line, tabStopWidth(segment.Style.TabSize, spaceWidth))
				continue
			}
			_, width, _ := shapeToken(textFace, token, segment.Style.FontSize.LengthPx, segment.Style.LetterSpacing.LengthPx, ctx)
			line += width
			if !token.IsSpace {
				minimum = max(minimum, width)
			}
		}
	}
	preferred = max(preferred, line)
	if !whiteSpaceAllowsWrap(box.Style.WhiteSpace.Keyword) {
		minimum = preferred
	}
	// Inline collection stops at layout containers. Include their intrinsic
	// widths so a nested block (for example a tooltip's pre) also sizes its parent.
	for _, child := range box.Children {
		if isAbsolutePositioned(child) || !isLayoutContainerDisplay(child.Style.Display.Keyword) {
			continue
		}
		childMin, childPreferred := 0.0, 0.0
		if style.IsLength(child.Style.Width) {
			extra := computedHorizontalBoxExtras(child.Style, 0)
			childMin = contentDimension(child.Style.Width, 0, extra, isBorderBox(child.Style)) + extra
			childPreferred = childMin
		} else {
			childMin, childPreferred = tableCellIntrinsicWidths(child, face, styles, ctx)
		}
		margin := resolveEdges(child.Style.Margin, 0)
		minimum = max(minimum, childMin+margin.Left+margin.Right)
		preferred = max(preferred, childPreferred+margin.Left+margin.Right)
	}
	extras := computedHorizontalBoxExtras(box.Style, 0)
	return minimum + extras, max(minimum, preferred) + extras
}

func setTableRect(box *Box, rect Rect, ctx *layoutContext) {
	if box.Content != rect && ctx != nil && ctx.retained != nil {
		ctx.retained.recordChanged(box)
	}
	box.Content, box.FlowMarginBox = rect, rect
	box.InvalidatePaintHash()
}

func translateTableCellContent(box *Box, dy float64, ctx *layoutContext) {
	box.Content.Y -= dy
	translateBox(box, 0, dy, ctx)
}

func isTableRowGroup(box *Box) bool {
	switch box.Style.Display.Keyword {
	case "table-row-group", "table-header-group", "table-footer-group":
		return true
	}
	return false
}

func tableRowOrder(box *Box) int {
	if box.Parent != nil {
		switch box.Parent.Style.Display.Keyword {
		case "table-header-group":
			return -1
		case "table-footer-group":
			return 1
		}
	}
	return 0
}

func tableSpan(cell *Box, name string, limit int) int {
	if cell == nil || cell.Node == nil {
		return 1
	}
	value, ok := cell.Node.Attr(name)
	if !ok {
		return 1
	}
	span, err := strconv.Atoi(strings.TrimSpace(value))
	if err == nil && span == 0 && name == "rowspan" {
		return limit
	}
	if err != nil || span < 1 {
		return 1
	}
	return min(span, limit)
}
