package layout

import (
	"strconv"
	"strings"

	"github.com/diiyw/onix/font"
	"github.com/diiyw/onix/internal/style"
)

type gridTrackKind uint8

const (
	gridTrackAuto gridTrackKind = iota
	gridTrackPixels
	gridTrackPercent
	gridTrackFraction
)

type gridTrack struct {
	kind   gridTrackKind
	value  float64
	size   float64
	offset float64
}

type gridItem struct {
	box                 *Box
	column, row         int
	columnSpan, rowSpan int
	columnSet, rowSet   bool
}

// Grid scratch belongs to one container so recursive layouts do not alias and
// repeat layouts retain their backing storage.
type gridScratch struct {
	columns, rows []gridTrack
	items         []gridItem
	occupied      []bool
}

func layoutGridContainer(box *Box, layoutContaining Rect, absoluteContaining Rect, face *font.Face, styles style.Map, ctx *layoutContext) {
	if box.grid == nil {
		box.grid = &gridScratch{}
	}
	scratch := box.grid
	columns := appendGridTracks(box.Style.GridTemplateColumns, scratch.columns[:0])
	if len(columns) == 0 {
		columns = append(columns, gridTrack{kind: gridTrackAuto})
	}
	rows := appendGridTracks(box.Style.GridTemplateRows, scratch.rows[:0])

	items := scratch.items[:0]
	for _, child := range box.Children {
		if isAbsolutePositioned(child) {
			continue
		}
		item := gridItem{box: child, columnSpan: 1, rowSpan: 1}
		columnStart, columnEnd := gridLine(child.Style.GridColumnStart), gridLine(child.Style.GridColumnEnd)
		rowStart, rowEnd := gridLine(child.Style.GridRowStart), gridLine(child.Style.GridRowEnd)
		if columnStart > 0 {
			item.column = columnStart - 1
			item.columnSet = true
		}
		if columnEnd > columnStart {
			item.columnSpan = columnEnd - columnStart
		}
		if rowStart > 0 {
			item.row = rowStart - 1
			item.rowSet = true
		}
		if rowEnd > rowStart {
			item.rowSpan = rowEnd - rowStart
		}
		items = append(items, item)
	}
	defer func() {
		clear(items)
		scratch.items = items[:0]
		scratch.columns = columns
		scratch.rows = rows
	}()

	columnCount := len(columns)
	rowCount := len(rows)
	if implicitRows := (len(items) + columnCount - 1) / columnCount; implicitRows > rowCount {
		rowCount = implicitRows
	}
	for _, item := range items {
		columnCount = maxInt(columnCount, item.column+item.columnSpan)
		rowCount = maxInt(rowCount, item.row+item.rowSpan)
	}
	if rowCount == 0 {
		rowCount = 1
	}
	columns = growGridTracks(columns, columnCount)
	rows = growGridTracks(rows, rowCount)
	scratch.occupied = growGridOccupied(scratch.occupied, len(columns)*len(rows))
	occupied := scratch.occupied[:len(columns)*len(rows)]
	clear(occupied)

	nextColumn, nextRow := 0, 0
	for index := range items {
		item := &items[index]
		column, row := item.column, item.row
		switch {
		case item.columnSet && item.rowSet:
		case item.columnSet:
			row = gridFindRow(occupied, len(columns), len(rows), column, item.columnSpan, item.rowSpan)
		case item.rowSet:
			column = gridFindColumn(occupied, len(columns), row, item.columnSpan, item.rowSpan)
		default:
			column, row = gridFindAutoCell(occupied, len(columns), len(rows), nextColumn, nextRow, item.columnSpan, item.rowSpan)
			nextColumn, nextRow = column+item.columnSpan, row
			if nextColumn >= len(columns) {
				nextColumn, nextRow = 0, row+1
			}
		}
		item.column, item.row = column, row
		gridOccupy(occupied, len(columns), column, row, item.columnSpan, item.rowSpan)
	}

	columnGap := resolveLength(box.Style.ColumnGap, box.Content.Width)
	rowGap := resolveLength(box.Style.RowGap, box.Content.Height)
	resolveGridTracks(columns, box.Content.Width, columnGap, items, true, face, styles, ctx)
	resolveGridTracks(rows, box.Content.Height, rowGap, items, false, face, styles, ctx)
	if !style.IsLength(box.Style.Height) {
		box.Content.Height = gridTracksSize(rows, rowGap)
		rowGap = resolveLength(box.Style.RowGap, box.Content.Height)
		resolveGridTracks(rows, box.Content.Height, rowGap, items, false, face, styles, ctx)
	}
	box.Content.Height = clampContentDimension(box.Content.Height, box.Style.MinHeight, box.Style.MaxHeight, layoutContaining.Height, verticalBoxExtras(box.Padding, box.Border), isBorderBox(box.Style))

	columnStart := gridContentOffset(box.Style.JustifyContent, box.Content.Width-positionGridTracks(columns, columnGap), len(columns), columnGap)
	if box.Style.AlignContent.Kind == style.ValueKeyword && box.Style.AlignContent.Keyword == "stretch" {
		stretchGridTracks(rows, box.Content.Height, rowGap)
	}
	rowStart := gridContentOffset(box.Style.AlignContent, box.Content.Height-positionGridTracks(rows, rowGap), len(rows), rowGap)
	absoluteContaining = descendantAbsoluteContaining(box, absoluteContaining)
	for _, item := range items {
		cell := Rect{
			X:      box.Content.X + columnStart + columns[item.column].offset,
			Y:      box.Content.Y + rowStart + rows[item.row].offset,
			Width:  gridTrackSpanSize(columns, item.column, item.columnSpan),
			Height: gridTrackSpanSize(rows, item.row, item.rowSpan),
		}
		layoutGridItem(item.box, box, cell, absoluteContaining, face, styles, ctx)
	}
	for _, child := range box.Children {
		if !isAbsolutePositioned(child) {
			continue
		}
		childContaining := Rect{X: box.Content.X, Y: box.Content.Y, Width: box.Content.Width, Height: box.Content.Height}
		layoutBlock(child, childContaining, absoluteContaining, face, styles, ctx)
	}
	alignBottomPositionedChildren(box, absoluteContaining, ctx)
	box.FlowMarginBox = box.MarginBox()
	if isRelativePositioned(box) {
		translateBox(box, resolveRelativeOffsetX(box.Style, layoutContaining.Width), resolveRelativeOffsetY(box.Style, layoutContaining.Height), ctx)
	}
}

// Auto tracks absorb free space for align-content:stretch. Fixed, percentage,
// and fractional tracks keep their resolved size; only auto tracks stretch.
func stretchGridTracks(tracks []gridTrack, available, gap float64) {
	if len(tracks) == 0 {
		return
	}
	auto := 0
	used := gap * float64(maxInt(len(tracks)-1, 0))
	for i := range tracks {
		used += tracks[i].size
		if tracks[i].kind == gridTrackAuto {
			auto++
		}
	}
	if auto == 0 {
		return
	}
	free := available - used
	if free <= 0 {
		return
	}
	step := free / float64(auto)
	for i := range tracks {
		if tracks[i].kind == gridTrackAuto {
			tracks[i].size += step
		}
	}
}

func appendGridTracks(input string, tracks []gridTrack) []gridTrack {
	for start := 0; start < len(input); {
		for start < len(input) && (input[start] == ' ' || input[start] == '\t' || input[start] == '\n' || input[start] == '\r') {
			start++
		}
		if start == len(input) {
			break
		}
		end, depth := start, 0
		for end < len(input) {
			switch input[end] {
			case '(':
				depth++
			case ')':
				if depth > 0 {
					depth--
				}
			case ' ', '\t', '\n', '\r':
				if depth == 0 {
					goto token
				}
			}
			end++
		}
	token:
		value := strings.TrimSpace(input[start:end])
		if strings.HasPrefix(strings.ToLower(value), "repeat(") && strings.HasSuffix(value, ")") {
			body := value[len("repeat(") : len(value)-1]
			if comma := strings.IndexByte(body, ','); comma > 0 {
				count, err := strconv.Atoi(strings.TrimSpace(body[:comma]))
				if err == nil && count > 0 {
					first := len(tracks)
					tracks = appendGridTracks(body[comma+1:], tracks)
					last := len(tracks)
					for repeat := 1; repeat < count; repeat++ {
						tracks = append(tracks, tracks[first:last]...)
					}
				}
			}
		} else if value != "" {
			tracks = append(tracks, parseGridTrack(value))
		}
		start = end + 1
	}
	return tracks
}

func parseGridTrack(input string) gridTrack {
	value := strings.ToLower(strings.TrimSpace(input))
	if value == "auto" {
		return gridTrack{kind: gridTrackAuto}
	}
	kind, suffix := gridTrackAuto, ""
	switch {
	case strings.HasSuffix(value, "px"):
		kind, suffix = gridTrackPixels, "px"
	case strings.HasSuffix(value, "%"):
		kind, suffix = gridTrackPercent, "%"
	case strings.HasSuffix(value, "fr"):
		kind, suffix = gridTrackFraction, "fr"
	default:
		return gridTrack{kind: gridTrackAuto}
	}
	number, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, suffix)), 64)
	if err == nil && number >= 0 {
		return gridTrack{kind: kind, value: number}
	}
	return gridTrack{kind: gridTrackAuto}
}

func growGridTracks(tracks []gridTrack, count int) []gridTrack {
	for len(tracks) < count {
		tracks = append(tracks, gridTrack{kind: gridTrackAuto})
	}
	return tracks
}

func growGridOccupied(occupied []bool, count int) []bool {
	if cap(occupied) < count {
		return make([]bool, count)
	}
	return occupied[:count]
}

func gridLine(value style.Value) int {
	if value.Kind != style.ValueNumber || value.Number <= 0 {
		return 0
	}
	return int(value.Number)
}

func gridFindRow(occupied []bool, columns, rows, column, columnSpan, rowSpan int) int {
	for row := 0; row+rowSpan <= rows; row++ {
		if gridCellFree(occupied, columns, column, row, columnSpan, rowSpan) {
			return row
		}
	}
	return maxInt(rows-rowSpan, 0)
}

func gridFindColumn(occupied []bool, columns, row, columnSpan, rowSpan int) int {
	for column := 0; column+columnSpan <= columns; column++ {
		if gridCellFree(occupied, columns, column, row, columnSpan, rowSpan) {
			return column
		}
	}
	return maxInt(columns-columnSpan, 0)
}

func gridFindAutoCell(occupied []bool, columns, rows, startColumn, startRow, columnSpan, rowSpan int) (int, int) {
	for row := startRow; row+rowSpan <= rows; row++ {
		columnStart := 0
		if row == startRow {
			columnStart = startColumn
		}
		for column := columnStart; column+columnSpan <= columns; column++ {
			if gridCellFree(occupied, columns, column, row, columnSpan, rowSpan) {
				return column, row
			}
		}
	}
	return 0, maxInt(rows-rowSpan, 0)
}

func gridCellFree(occupied []bool, columns, column, row, columnSpan, rowSpan int) bool {
	for y := row; y < row+rowSpan; y++ {
		for x := column; x < column+columnSpan; x++ {
			if occupied[y*columns+x] {
				return false
			}
		}
	}
	return true
}

func gridOccupy(occupied []bool, columns, column, row, columnSpan, rowSpan int) {
	for y := row; y < row+rowSpan; y++ {
		for x := column; x < column+columnSpan; x++ {
			occupied[y*columns+x] = true
		}
	}
}

func resolveGridTracks(tracks []gridTrack, available, gap float64, items []gridItem, columns bool, face *font.Face, styles style.Map, ctx *layoutContext) {
	fractions := 0.0
	for index := range tracks {
		track := &tracks[index]
		track.size = 0
		switch track.kind {
		case gridTrackPixels:
			track.size = track.value
		case gridTrackPercent:
			track.size = available * track.value / 100
		case gridTrackFraction:
			fractions += track.value
		}
	}
	for _, item := range items {
		start, span := item.row, item.rowSpan
		if columns {
			start, span = item.column, item.columnSpan
		}
		if span != 1 || start < 0 || start >= len(tracks) || tracks[start].kind != gridTrackAuto {
			continue
		}
		tracks[start].size = maxFloat(tracks[start].size, gridPreferredOuterSize(item.box, columns, available, face, styles, ctx))
	}
	fixed := 0.0
	for _, track := range tracks {
		if track.kind != gridTrackFraction {
			fixed += track.size
		}
	}
	free := maxFloat(available-fixed-gap*float64(maxInt(len(tracks)-1, 0)), 0)
	if fractions > 0 {
		for index := range tracks {
			if tracks[index].kind == gridTrackFraction {
				tracks[index].size = free * tracks[index].value / fractions
			}
		}
	}
}

func gridPreferredOuterSize(box *Box, columns bool, available float64, face *font.Face, styles style.Map, ctx *layoutContext) float64 {
	if box == nil {
		return 0
	}
	value, extras := box.Style.Height, computedVerticalBoxExtras(box.Style, available)
	margin := resolveEdges(box.Style.Margin, available)
	marginSize := margin.Top + margin.Bottom
	if columns {
		value, extras = box.Style.Width, computedHorizontalBoxExtras(box.Style, available)
		marginSize = margin.Left + margin.Right
	}
	if style.IsLength(value) {
		return contentDimension(value, available, extras, isBorderBox(box.Style)) + extras + marginSize
	}
	if columns {
		return intrinsicFlexContentWidth(box, face, ctx, styles) + extras + marginSize
	}
	return 0
}

func gridTracksSize(tracks []gridTrack, gap float64) float64 {
	size := 0.0
	for _, track := range tracks {
		size += track.size
	}
	if len(tracks) > 1 {
		size += gap * float64(len(tracks)-1)
	}
	return size
}

func positionGridTracks(tracks []gridTrack, gap float64) float64 {
	offset, size := 0.0, 0.0
	for index := range tracks {
		tracks[index].offset = offset
		offset += tracks[index].size + gap
		size += tracks[index].size
	}
	return size + gap*float64(maxInt(len(tracks)-1, 0))
}

func gridTrackSpanSize(tracks []gridTrack, start, span int) float64 {
	if span == 1 {
		return tracks[start].size
	}
	last := &tracks[start+span-1]
	return last.offset + last.size - tracks[start].offset
}

func gridContentOffset(mode style.Value, free float64, count int, gap float64) float64 {
	if free <= 0 {
		return 0
	}
	switch mode.Keyword {
	case "flex-end", "end":
		return free
	case "center":
		return free / 2
	case "space-around":
		return free / float64(maxInt(count, 1)) / 2
	case "space-evenly":
		return free / float64(count+1)
	}
	return 0
}

func layoutGridItem(child, container *Box, cell, absoluteContaining Rect, face *font.Face, styles style.Map, ctx *layoutContext) {
	if child == nil {
		return
	}
	justify, align := gridItemAlignment(child.Style.JustifySelf, container.Style.JustifyItems), gridItemAlignment(child.Style.AlignSelf, container.Style.AlignItems)
	original := child.Style
	width, height := original.Width, original.Height
	changed := false
	if justify == "stretch" && original.Width.Kind == style.ValueAuto {
		width = gridStretchDimension(cell.Width, original, true)
		changed = true
	}
	if align == "stretch" && original.Height.Kind == style.ValueAuto {
		height = gridStretchDimension(cell.Height, original, false)
		changed = true
	}
	if changed {
		child.Style = child.measureStyle(&child.gridMeasureStyle, width, height)
	}
	layoutBlock(child, cell, absoluteContaining, face, styles, ctx)
	child.Style = original
	outer := child.FlowMarginBox
	dx := gridAlignedOffset(justify, maxFloat(cell.Width-outer.Width, 0))
	dy := gridAlignedOffset(align, maxFloat(cell.Height-outer.Height, 0))
	translateBox(child, cell.X+dx-outer.X, cell.Y+dy-outer.Y, ctx)
}

func gridItemAlignment(self, inherited style.Value) string {
	if self.Kind == style.ValueKeyword && self.Keyword != "auto" {
		return self.Keyword
	}
	if inherited.Kind == style.ValueKeyword {
		return inherited.Keyword
	}
	return "stretch"
}

func gridStretchDimension(size float64, computed *style.ComputedStyle, horizontal bool) style.Value {
	extras := computedVerticalBoxExtras(computed, size)
	if horizontal {
		extras = computedHorizontalBoxExtras(computed, size)
	}
	if isBorderBox(computed) {
		return style.LengthPx(maxFloat(size, 0))
	}
	return style.LengthPx(maxFloat(size-extras, 0))
}

func gridAlignedOffset(mode string, free float64) float64 {
	switch mode {
	case "end", "flex-end":
		return free
	case "center":
		return free / 2
	}
	return 0
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
