package html

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/html/layout"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/scene"
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
		computed.BackgroundImage == "" && len(computed.BackgroundLayers) == 0 &&
		computed.AnimatedPaint == 0 && !computed.AnimatedOpacity && computed.Opacity.Kind == style.ValueNumber
}

func spawnHTMLInlineBackgroundList(world *ecs.World, owner ecs.Entity, state *htmlRenderState, backgrounds []layout.InlineBackground, window htmlPaintWindow, stackIndex int, visible bool) {
	first := backgrounds[0]
	bounds := first.Rect
	for _, background := range backgrounds[1:] {
		if htmlPaintFragmentVisible(window, background.Rect, background.Style) {
			bounds = htmlUnionPaintBounds(bounds, background.Rect)
		}
	}
	bundle := beginHTMLEntity(world, owner, first.Node, htmlRoleInlineBackgroundList, "", 0, 10)
	layers := scene.SpriteLayers2D{}
	if bundle.entity != 0 {
		if previous := world.GetRef[scene.SpriteLayers2D](bundle.entity); previous != nil {
			layers = *previous
			layers.Layers = layers.Layers[:0]
		}
	}
	for _, background := range backgrounds {
		if !htmlPaintFragmentVisible(window, background.Rect, background.Style) {
			continue
		}
		r := background.Rect
		sprite := scene.NewSprite2D()
		sprite.Shape = scene.RoundedRectShape2D(htmlColor(background.Style.BackgroundColor), htmlOverflowClipRadius(background.Style, r.Width, r.Height))
		sprite.Width, sprite.Height = float32(r.Width), float32(r.Height)
		layers.Layers = append(layers.Layers, scene.SpriteLayer2D{
			Sprite: sprite, Transform: scene.SpriteLayerTransform2D{X: float32(r.X - bounds.X), Y: float32(r.Y - bounds.Y)},
			Anchor: scene.AnchorTopLeft, Visible: true,
		})
	}
	layers.InvalidateMatrices()
	sprite := scene.NewSprite2D()
	sprite.Alpha = float32(first.Style.Opacity.Number)
	(&bundle).putHTML(scene.UITransformFromXY(float32(bounds.X), float32(bounds.Y)))
	(&bundle).putHTML(scene.UIGlobalTransform{})
	(&bundle).putHTML(scene.Visibility{Visible: visible})
	(&bundle).putHTML(scene.AnchorTopLeft)
	(&bundle).putHTML(scene.UIStackIndex(stackIndex))
	(&bundle).putHTML(scene.Parent{Entity: owner})
	(&bundle).putHTML(sprite)
	(&bundle).putHTML(layers)
	(&bundle).putHTML(scene.Size2D{Width: float32(bounds.Width), Height: float32(bounds.Height)})
	(&bundle).putHTML(htmlChild{Owner: owner, Node: first.Node})
	entity := bundle.finish()
	markHTMLChildNode(world, entity, first.Node)
	state.Children = append(state.Children, entity)
}
