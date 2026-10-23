package editor

import (
	"math"
)

const (
	defaultViewportWidth       = 1600
	defaultViewportHeight      = 960
	editorTopBarHeight         = 38
	editorFieldHeight          = float32(28)
	editorCompactControlHeight = float32(26)
	editorSidebarTabHeight     = float32(34)
	editorPanelMinWidth        = float32(240)
	editorPanelMinHeight       = float32(120)
	editorPanelGap             = float32(6)
	editorPanelCornerRadius    = float32(8)
	defaultTargetFPS           = 30
	panelResizeLeftWidth       = "left-width"
	panelResizeRightWidth      = "right-width"
	panelResizeContentHeight   = "content-height"
)

type editorLayout struct {
	Root      rect
	TopBar    rect
	Files     rect
	Tree      rect
	Viewport  rect
	Inspector rect
	Content   rect
}

type rect struct {
	X float32
	Y float32
	W float32
	H float32
}

type recentProjectRow struct {
	Path       string
	Rect       rect
	RemoveRect rect
}

func computeEditorLayout(root rect, leftWidth, rightWidth, contentHeight float32) editorLayout {
	topH := float32(editorTopBarHeight)
	bodyW := max(0, root.W-editorPanelGap*4)
	leftW := min(float32(300), bodyW*0.28)
	rightW := min(float32(380), bodyW*0.28)
	if leftWidth > 0 {
		minWidth, maxWidth := panelWidthBounds(root.W, rightW)
		leftW = clamp(leftWidth, minWidth, maxWidth)
	}
	if rightWidth > 0 {
		minWidth, maxWidth := panelWidthBounds(root.W, leftW)
		rightW = clamp(rightWidth, minWidth, maxWidth)
	}
	bodyTop := topH + editorPanelGap*2
	bodyH := max(0, root.H-bodyTop-editorPanelGap)
	centerH := max(0, bodyH-editorPanelGap)
	contentH := min(centerH, min(float32(280), max(float32(170), bodyH*0.28)))
	if contentHeight > 0 {
		minHeight, maxHeight := contentHeightBounds(centerH)
		contentH = clamp(contentHeight, minHeight, maxHeight)
	}
	mainH := max(0, centerH-contentH)
	centerW := max(0, bodyW-leftW-rightW)
	centerX := editorPanelGap*2 + leftW
	treeTop := bodyTop + editorSidebarTabHeight
	contentTop := bodyTop + mainH + editorPanelGap
	return editorLayout{
		Root:      root,
		TopBar:    rect{X: editorPanelGap, Y: editorPanelGap, W: max(0, root.W-editorPanelGap*2), H: topH},
		Files:     rect{X: editorPanelGap, Y: bodyTop, W: leftW, H: bodyH},
		Tree:      rect{X: editorPanelGap, Y: treeTop, W: leftW, H: max(0, bodyH-editorSidebarTabHeight)},
		Viewport:  rect{X: centerX, Y: bodyTop, W: centerW, H: mainH},
		Inspector: rect{X: centerX + centerW + editorPanelGap, Y: bodyTop, W: rightW, H: bodyH},
		Content:   rect{X: centerX, Y: contentTop, W: centerW, H: contentH},
	}
}

func panelWidthBounds(rootWidth, otherWidth float32) (float32, float32) {
	rootWidth = max(0, rootWidth-editorPanelGap*4)
	minWidth := min(editorPanelMinWidth, rootWidth*0.28)
	maxWidth := rootWidth - otherWidth - min(editorPanelMinWidth, rootWidth*0.3)
	if maxWidth < minWidth {
		minWidth = maxWidth
	}
	return minWidth, maxWidth
}

func contentHeightBounds(bodyHeight float32) (float32, float32) {
	minHeight := min(bodyHeight, editorPanelMinHeight)
	maxHeight := bodyHeight
	if bodyHeight >= editorPanelMinHeight*2 {
		maxHeight = bodyHeight - editorPanelMinHeight
	}
	if maxHeight < minHeight {
		minHeight = maxHeight
	}
	return minHeight, maxHeight
}

func approximateTextWidth(value string, size float32) float32 {
	return float32(len([]rune(value))) * size * 0.58
}

func buttonTextWidth(label string, icon bool) float32 {
	width := approximateTextWidth(label, 14) + 24
	if icon {
		width += editorButtonIconSize + 8
	}
	return float32(math.Ceil(float64(width)))
}

func clamp[T ~float32 | ~float64](value, low, high T) T {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func abs[T ~float32 | ~float64](value T) T {
	if value < 0 {
		return -value
	}
	return value
}
