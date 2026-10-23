package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/input"
	"github.com/diiyw/onix/project"
	"github.com/diiyw/onix/render"
	htmlui "github.com/diiyw/onix/ui/html"
)

func TestWorkspaceLayoutMatchesRenderedPanels(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	for _, size := range [][2]int{{1600, 960}, {960, 540}, {720, 300}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			canvas := graphics.NewCanvas2D(render.ActiveDevice(), size[0], size[1])
			t.Cleanup(graphics.SetCurrentContextForTesting(canvas))
			a.changed()
			if err := a.sync(&ecs.SystemContext{World: world}); err != nil {
				t.Fatal(err)
			}
			if err := htmlui.Sync(world); err != nil {
				t.Fatal(err)
			}
			layout := a.layout()
			for _, panel := range []struct {
				id   string
				want rect
			}{{"titlebar", layout.TopBar}, {"scene-panel", layout.Files}, {"viewport-panel", layout.Viewport}, {"content-browser", layout.Content}, {"inspector", layout.Inspector}} {
				owner := a.shell
				if panel.id == "titlebar" {
					owner = a.titlebar.entity
				}
				node, ok := htmlui.NodeByID(world, owner, panel.id)
				if !ok {
					t.Fatalf("missing %s", panel.id)
				}
				box, ok := node.Bounds()
				if panel.id == "titlebar" {
					box.X += layout.TopBar.X
					box.Y += layout.TopBar.Y
				}
				if got := (rect{X: box.X, Y: box.Y, W: box.Width, H: box.Height}); !ok || got != panel.want {
					t.Fatalf("%s: got %+v, layout %+v", panel.id, got, panel.want)
				}
			}
			if abs(layout.Viewport.X-layout.Files.X-layout.Files.W-editorPanelGap) > .001 || abs(layout.Inspector.X-layout.Viewport.X-layout.Viewport.W-editorPanelGap) > .001 || abs(layout.Content.Y-layout.Viewport.Y-layout.Viewport.H-editorPanelGap) > .001 {
				t.Fatalf("panel gutters are inconsistent: %+v", layout)
			}
			if layout.Files.X != editorPanelGap || layout.TopBar.X != editorPanelGap || layout.TopBar.Y != editorPanelGap || abs(layout.Root.W-layout.TopBar.X-layout.TopBar.W-editorPanelGap) > .001 || layout.Files.Y-layout.TopBar.Y-layout.TopBar.H != editorPanelGap || abs(layout.Root.W-layout.Inspector.X-layout.Inspector.W-editorPanelGap) > .001 || abs(layout.Root.H-layout.Content.Y-layout.Content.H-editorPanelGap) > .001 {
				t.Fatalf("outer panel spacing is inconsistent: %+v", layout)
			}
			if _, ok := htmlui.NodeByID(world, a.shell, "statusbar"); ok {
				t.Fatal("removed status bar is still mounted")
			}
			bounds := a.canvasBounds()
			if bounds.X <= layout.Viewport.X || bounds.Y <= layout.Viewport.Y || bounds.X+bounds.W >= layout.Viewport.X+layout.Viewport.W || bounds.Y+bounds.H >= layout.Viewport.Y+layout.Viewport.H {
				t.Fatalf("canvas overlaps its panel border: canvas %+v, panel %+v", bounds, layout.Viewport)
			}
		})
	}
}

func TestWorkspacePanelResizePreservesCanvasAndResourceLayout(t *testing.T) {
	a := workspaceEditorFixture(t)
	in := input.NewState()
	layout := a.layout()
	x, y := layout.Content.X+30, layout.Content.Y-editorPanelGap/2
	in.PressMouse(input.MouseLeft, float64(x), float64(y))
	if !a.updatePanelResize(in) {
		t.Fatal("resource divider did not start resizing")
	}
	in.EndFrame()
	in.MoveMouse(float64(x), float64(y-60))
	a.updatePanelResize(in)
	next := a.layout()
	if abs(next.Content.H-layout.Content.H-60) > .001 || abs(next.Content.Y-next.Viewport.Y-next.Viewport.H-editorPanelGap) > .001 || next.Content.X != next.Viewport.X {
		t.Fatalf("resource resize changed the gutter or left an overlap: %+v", next)
	}
	in.ReleaseMouse(input.MouseLeft, float64(x), float64(y-60))
	a.updatePanelResize(in)
	if a.panelResizeMode != "" {
		t.Fatal("resize did not end on release")
	}
}

func TestWorkspaceSidePanelResizePreservesGutters(t *testing.T) {
	for _, side := range []string{panelResizeLeftWidth, panelResizeRightWidth} {
		t.Run(side, func(t *testing.T) {
			a := workspaceEditorFixture(t)
			in := input.NewState()
			layout := a.layout()
			x := layout.Files.X + layout.Files.W + editorPanelGap/2
			delta := float32(40)
			if side == panelResizeRightWidth {
				x, delta = layout.Inspector.X-editorPanelGap/2, -delta
			}
			y := layout.Files.Y + 60
			in.PressMouse(input.MouseLeft, float64(x), float64(y))
			if !a.updatePanelResize(in) || a.panelResizeMode != side {
				t.Fatal("panel gutter did not start resizing")
			}
			in.EndFrame()
			in.MoveMouse(float64(x+delta), float64(y))
			a.updatePanelResize(in)
			next := a.layout()
			growth := next.Files.W - layout.Files.W
			if side == panelResizeRightWidth {
				growth = next.Inspector.W - layout.Inspector.W
			}
			if abs(growth-40) > .001 || abs(next.Viewport.X-next.Files.X-next.Files.W-editorPanelGap) > .001 || abs(next.Inspector.X-next.Viewport.X-next.Viewport.W-editorPanelGap) > .001 {
				t.Fatalf("panel resize lost width or gutters: %+v", next)
			}
		})
	}
}

func TestWorkspacePanelEdgeHoverAndDrag(t *testing.T) {
	a := workspaceEditorFixture(t)
	layout := a.layout()
	for _, edge := range []struct {
		id   string
		want rect
	}{{panelResizeLeftWidth, rect{X: layout.Files.X + layout.Files.W, Y: layout.Files.Y, W: editorPanelGap, H: layout.Files.H}},
		{panelResizeRightWidth, rect{X: layout.Inspector.X - editorPanelGap, Y: layout.Inspector.Y, W: editorPanelGap, H: layout.Inspector.H}},
		{panelResizeContentHeight, rect{X: layout.Viewport.X, Y: layout.Viewport.Y + layout.Viewport.H, W: layout.Viewport.W, H: editorPanelGap}},
	} {
		if got := panelResizeRect(layout, edge.id); got != edge.want {
			t.Fatalf("%s: got %+v want %+v", edge.id, got, edge.want)
		}
	}
	in := input.NewState()
	edge := panelResizeRect(layout, panelResizeLeftWidth)
	in.MoveMouse(float64(edge.X+edge.W/2), float64(edge.Y+60))
	a.updatePanelResize(in)
	if a.panelResizeHover != panelResizeLeftWidth {
		t.Fatalf("edge hover = %q", a.panelResizeHover)
	}
	if source := workspaceShellTest(t, a); !strings.Contains(source, `class="outliner edge-resize-right"`) {
		t.Fatal("hover did not highlight the panel edge")
	}
	in.EndFrame()
	in.PressMouse(input.MouseLeft, float64(edge.X+edge.W/2), float64(edge.Y+60))
	if !a.updatePanelResize(in) || a.panelResizeMode != panelResizeLeftWidth {
		t.Fatalf("edge press did not start resizing, mode=%q", a.panelResizeMode)
	}
	in.EndFrame()
	in.MoveMouse(float64(edge.X+edge.W/2+40), float64(edge.Y+60))
	a.updatePanelResize(in)
	if moved := panelResizeRect(a.layout(), panelResizeLeftWidth); abs(moved.X-edge.X-40) > .001 {
		t.Fatalf("held drag did not move the edge: %+v", moved)
	}
	if source := workspaceShellTest(t, a); !strings.Contains(source, `class="outliner edge-resize-right"`) {
		t.Fatal("drag did not keep the panel edge highlighted")
	}
	in.EndFrame()
	in.MoveMouse(float64(layout.Viewport.X+100), float64(layout.Viewport.Y+100))
	if !a.updatePanelResize(in) || a.panelResizeMode != panelResizeLeftWidth {
		t.Fatal("drag ended while moving away from the edge")
	}
	in.EndFrame()
	in.ReleaseMouse(input.MouseLeft, float64(layout.Viewport.X+100), float64(layout.Viewport.Y+100))
	if !a.updatePanelResize(in) || a.panelResizeMode != "" {
		t.Fatal("release did not end the drag")
	}
	in.EndFrame()
	in.MoveMouse(float64(layout.Inspector.X-1), float64(layout.Files.Y+60))
	a.updatePanelResize(in)
	if a.panelResizeHover != panelResizeRightWidth {
		t.Fatalf("hover did not follow the pointer, got %q", a.panelResizeHover)
	}
	if source := workspaceShellTest(t, a); !strings.Contains(source, `class="panel inspector edge-resize-left"`) || strings.Contains(source, `class="outliner edge-resize-right"`) {
		t.Fatal("edge highlight did not follow the hovered panel")
	}
}

func TestWorkspaceWindowDragExcludesToolsAndClosePreservesEdits(t *testing.T) {
	a := workspaceEditorFixture(t)
	world := ecs.NewWorld()
	if err := a.sync(&ecs.SystemContext{World: world}); err != nil {
		t.Fatal(err)
	}
	if err := htmlui.Sync(world); err != nil {
		t.Fatal(err)
	}
	blank := float64((180 + a.width*.5 - 90) * .5)
	if !a.windowDragRegion(blank, 18) || a.windowDragRegion(45, 18) || a.windowDragRegion(float64(a.width/2), 18) || a.windowDragRegion(blank, 70) {
		for _, id := range []string{"menu-file", "menu-edit", "menu-debug", "menu-about", "top-run", "window-controls"} {
			node, _ := htmlui.NodeByID(world, a.titlebar.entity, id)
			box, _ := node.Bounds()
			t.Log(id, box)
		}
		t.Fatalf("window drag hits: blank=%t menu=%t tools=%t body=%t", a.windowDragRegion(blank, 18), a.windowDragRegion(45, 18), a.windowDragRegion(float64(a.width/2), 18), a.windowDragRegion(blank, 70))
	}
	closed := false
	a.requestClose = func() { closed = true }
	if err := a.Document.EditInstance("start", "Move", func(instance *project.ModInstanceSpec) error {
		instance.Placement.Position = []float32{2, 3}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	a.action("window-close")
	if closed || a.pendingAction != "close" {
		t.Fatal("window close discarded unsaved documents")
	}
	a.action("discard-pending")
	if !closed || !a.closeConfirmed {
		t.Fatal("explicit discard did not complete window close")
	}
}
