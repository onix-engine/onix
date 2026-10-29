package html

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/scene"
)

func spawnHTMLInlineBackgrounds(world *ecs.World, owner ecs.Entity, state *htmlRenderState, box *layout.Box, window htmlPaintWindow, control ecs.Entity, controlRect layout.Rect, stackIndex int, visible bool) {
	for _, background := range box.InlineBackgrounds {
		if !htmlPaintFragmentVisible(window, background.Rect, background.Style) {
			continue
		}
		r := background.Rect
		shape := scene.RoundedRectShape2D(htmlColor(background.Style.BackgroundColor), htmlOverflowClipRadius(background.Style, r.Width, r.Height))
		entity := spawnHTMLSurface(world, owner, background.Node, htmlRoleInlineBackground, float32(r.X), float32(r.Y), float32(r.Width), float32(r.Height), shape, stackIndex, visible)
		markHTMLChildNode(world, entity, background.Node)
		_ = world.Insert(entity, htmlElementSurface{})
		if control != 0 {
			attachHTMLImageToControl(world, entity, control, controlRect)
		}
		applyHTMLAnimatedVisual(world, state, entity, background.Node)
		state.Children = append(state.Children, entity)
	}
}
