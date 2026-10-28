package editor

import (
	"runtime"

	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/window"
	"github.com/go-gl/mathgl/mgl32"
)

func (a *ModuleEditor) renderWindowBorder(context *ecs.SystemContext) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	c := graphics.CanvasFor(context.World)
	if c == nil {
		return nil
	}
	width, height := c.Size()
	c.ResetViewMatrix()
	c.ResetProjectionMatrix()
	c.DrawShapeMatrices(mgl32.Scale3D(width, height, 1), mgl32.Ident3(), width, height,
		render.ShapeRoundedRect, graphics.ColorRGBA{}, editorTheme.panelBorder, float32(window.DefaultCornerRadius), 1, 1)
	return nil
}
