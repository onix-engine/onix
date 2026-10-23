package editor

import (
	"runtime"

	"github.com/diiyw/onix/input"
	htmlui "github.com/diiyw/onix/ui/html"
)

type workspaceWindowIconSize struct{}

func (workspaceWindowIconSize) SVGIconSize() (int, int) { return 128, 128 }

func (a *ModuleEditor) layout() editorLayout {
	return computeEditorLayout(rect{W: max(1, a.width), H: max(1, a.height)}, a.leftPanelWidth, a.rightPanelWidth, a.contentPanelHeight)
}

func (a *ModuleEditor) canvasBounds() rect {
	bounds := a.layout().Viewport
	bounds.X, bounds.Y = bounds.X+1, bounds.Y+1
	bounds.W, bounds.H = max(1, bounds.W-2), max(1, bounds.H-2)
	return bounds
}

func (a *ModuleEditor) windowDragRegion(x, y float64) bool {
	if y < float64(editorPanelGap) || y >= float64(editorTopBarHeight+editorPanelGap) || x < float64(editorPanelGap) || x >= float64(a.width-editorPanelGap) || a.world == nil || a.modalVisible() {
		return false
	}
	if runtime.GOOS == "darwin" && x < 80 {
		return false
	}
	x -= float64(editorPanelGap)
	y -= float64(editorPanelGap)
	for _, id := range [...]string{"menu-file", "menu-edit", "menu-debug", "menu-about", "top-run", "top-save", "top-reload", "top-frame", "top-dimension", "window-controls"} {
		if node, ok := htmlui.NodeByID(a.world, a.titlebar.entity, id); ok {
			if bounds, ok := node.Bounds(); ok && x >= float64(bounds.X) && x < float64(bounds.X+bounds.Width) && y >= float64(bounds.Y) && y < float64(bounds.Y+bounds.Height) {
				return false
			}
		}
	}
	return true
}

// panelResizeRect is the grab zone: the gutter flush against a panel's resizable edge.
func panelResizeRect(layout editorLayout, mode string) rect {
	body := layout.Files
	switch mode {
	case panelResizeLeftWidth:
		return rect{X: body.X + body.W, Y: body.Y, W: editorPanelGap, H: body.H}
	case panelResizeRightWidth:
		inspector := layout.Inspector
		return rect{X: inspector.X - editorPanelGap, Y: inspector.Y, W: editorPanelGap, H: inspector.H}
	}
	viewport := layout.Viewport
	return rect{X: viewport.X, Y: viewport.Y + viewport.H, W: viewport.W, H: editorPanelGap}
}

func panelResizeHit(layout editorLayout, mode string, pointer input.Pointer) bool {
	bounds := panelResizeRect(layout, mode)
	return float64(pointer.X) >= float64(bounds.X) && float64(pointer.X) < float64(bounds.X+bounds.W) &&
		float64(pointer.Y) >= float64(bounds.Y) && float64(pointer.Y) < float64(bounds.Y+bounds.H)
}

func (a *ModuleEditor) updatePanelResize(in *input.State) bool {
	pointer := in.Pointer()
	if a.panelResizeMode == "" {
		hover := ""
		if !a.modalVisible() && !a.workspaceMenuOpen() {
			for _, id := range [...]string{panelResizeLeftWidth, panelResizeRightWidth, panelResizeContentHeight} {
				if panelResizeHit(a.layout(), id, pointer) {
					hover = id
					break
				}
			}
		}
		if hover != a.panelResizeHover {
			a.panelResizeHover = hover
			a.changed()
		}
		if hover == "" {
			return false
		}
		if in.MousePressed(input.MouseLeft) {
			a.panelResizeMode = hover
			a.changed()
		}
		return true
	}
	layout := a.layout()
	if !in.MouseHeld(input.MouseLeft) {
		a.panelResizeMode = ""
		a.changed()
		return true
	}
	switch a.panelResizeMode {
	case panelResizeLeftWidth:
		low, high := panelWidthBounds(layout.Root.W, layout.Inspector.W)
		a.leftPanelWidth = clamp(pointer.X-layout.Files.X-editorPanelGap/2, low, high)
	case panelResizeRightWidth:
		low, high := panelWidthBounds(layout.Root.W, layout.Files.W)
		a.rightPanelWidth = clamp(layout.Root.W-editorPanelGap-pointer.X-editorPanelGap/2, low, high)
	case panelResizeContentHeight:
		low, high := contentHeightBounds(max(0, layout.Files.H-editorPanelGap))
		a.contentPanelHeight = clamp(layout.Files.Y+layout.Files.H-pointer.Y-editorPanelGap/2, low, high)
	}
	a.changed()
	return true
}
