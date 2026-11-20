package html

import (
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/html/layout"
	"github.com/onix-engine/onix/html/style"
	"github.com/onix-engine/onix/scene"
)

func spawnHTMLInlineBackgrounds(world *ecs.World, owner ecs.Entity, state *htmlRenderState, box *layout.Box, window htmlPaintWindow, control ecs.Entity, controlRect layout.Rect, stackIndex int, visible bool) {
	for index := 0; index < len(box.InlineBackgrounds); {
		background := box.InlineBackgrounds[index]
		index++
		if !htmlPaintFragmentVisible(window, background.Rect, background.Style) {
			continue
		}
		if control == 0 && htmlInlineBackgroundListEligible(background) {
			end := index
			for end < len(box.InlineBackgrounds) && box.InlineBackgrounds[end].Node == background.Node && box.InlineBackgrounds[end].Style == background.Style && htmlInlineBackgroundListEligible(box.InlineBackgrounds[end]) {
				end++
			}
			if end-index >= 1 {
				spawnHTMLInlineBackgroundList(world, owner, state, box.InlineBackgrounds[index-1:end], window, stackIndex, visible)
				index = end
				continue
			}
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

func htmlInlineBackgroundListEligible(background layout.InlineBackground) bool {
	return background.Node != nil && htmlInlineBackgroundListStyleEligible(background.Style)
}

func htmlInlineBackgroundListStyleEligible(computed *style.ComputedStyle) bool {
	return computed != nil && computed.BackgroundColor.Color.A > 0 &&
		computed.Background.Image == "" && len(computed.ExtraBackgroundLayers) == 0 &&
		computed.AnimatedPaint == 0 && !computed.AnimatedOpacity && computed.Opacity.Kind == style.ValueNumber
}

type htmlInlinePaintScratch struct{}

func spawnHTMLInlineBackgroundList(world *ecs.World, owner ecs.Entity, state *htmlRenderState, backgrounds []layout.InlineBackground, window htmlPaintWindow, stackIndex int, visible bool) {
	first := backgrounds[0]
	origin := first.Rect
	bounds := origin
	scratch := world.FrameScratch[htmlInlinePaintScratch, scene.UIShapeFragment2D]()
	scratch.Values = scratch.Values[:0]
	for _, background := range backgrounds {
		if !htmlPaintFragmentVisible(window, background.Rect, background.Style) {
			continue
		}
		r := background.Rect
		bounds = htmlUnionPaintBounds(bounds, r)
		scratch.Values = append(scratch.Values, scene.UIShapeFragment2D{
			Shape: scene.RoundedRectShape2D(htmlColor(background.Style.BackgroundColor), htmlOverflowClipRadius(background.Style, r.Width, r.Height)),
			X:     float32(r.X - origin.X), Y: float32(r.Y - origin.Y), Width: float32(r.Width), Height: float32(r.Height),
		})
	}
	sprite := scene.NewSprite2D()
	sprite.Width, sprite.Height = float32(bounds.Width), float32(bounds.Height)
	sprite.Alpha = float32(first.Style.Opacity.Number)
	bundle := beginHTMLEntity(world, owner, first.Node, htmlRoleInlineBackgroundList, "", 0, 8)
	putHTMLSurface(&bundle, owner, float32(origin.X), float32(origin.Y), sprite, stackIndex, visible)
	entity := bundle.finish()
	scene.SetUIShapeFragments2D(world, entity, scratch.Values)
	scratch.ReleaseUnused()
	state.Children = append(state.Children, entity)
}
