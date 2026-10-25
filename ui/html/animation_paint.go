package html

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/html/dom"
	"github.com/diiyw/onix/html/style"
	"github.com/diiyw/onix/scene"
)

func htmlAnimationColor(color style.AnimationColor) graphics.ColorRGBA {
	r, g, b, a := color.RGBA()
	return graphics.ColorRGBA{R: float32(r), G: float32(g), B: float32(b), A: float32(a)}
}

func applyHTMLAnimatedPaint(world *ecs.World, state *htmlRenderState, entity ecs.Entity, node *dom.Node, child *htmlChild, foreground style.AnimationColor) {
	computed := state.Styles[node]
	if computed == nil || computed.AnimatedPaint == 0 {
		return
	}
	var value style.AnimationValue
	animation := state.Animations[node]
	if animation != nil {
		value = animation.Current
	} else {
		value = style.BaseAnimationValue(computed)
	}
	if clip := ecs.GetRef[scene.RoundedClip2D](world, entity); (clip != nil) && child.role == htmlRoleOverflow {
		background := htmlAnimationColor(value.Colors[style.AnimationBackground].Resolve(foreground))
		if clip.MaskColor != background {
			mutable := ecs.GetMut[scene.RoundedClip2D](world, entity)
			mutable.MaskColor = background
		}
	}
	color := htmlAnimationColor(foreground)
	if text := ecs.GetRef[scene.Text2D](world, entity); text != nil {
		placeholder := false
		if link := ecs.GetRef[ControlText](world, entity); link != nil {
			if control := ecs.GetRef[Control](world, link.Control); control != nil {
				placeholder = control.Value == "" && control.Placeholder != ""
			}
		}
		if placeholder {
			color = htmlColor(computed.PlaceholderColor)
		}
		if text.Color != color {
			mutable := ecs.GetMut[scene.Text2D](world, entity)
			mutable.Color = color
		}
	}
	if indicator := ecs.GetRef[scene.UIImage2D](world, entity); (indicator != nil) && child.role == htmlRoleIndicator && indicator.Tint != color {
		mutable := ecs.GetMut[scene.UIImage2D](world, entity)
		mutable.Tint = color
	}
	sprite := ecs.GetRef[scene.Sprite2D](world, entity)
	if sprite == nil || ecs.Has[htmlOpacitySurface](world, entity) {
		return
	}
	fill, border := sprite.Shape.Fill, sprite.Shape.Border
	switch child.role {
	case htmlRoleDecoration, htmlRoleCheckbox:
		fill = color
	case htmlRoleBorderTop, htmlRoleBorderRight, htmlRoleBorderBottom, htmlRoleBorderLeft:
		index := style.AnimationBorderTop + int(child.role-htmlRoleBorderTop)
		fill = htmlAnimationColor(value.Colors[index].Resolve(foreground))
	case htmlRoleSurface, htmlRoleInlineBackground, htmlRoleControl, htmlRoleCanvas:
		if htmlNeedsRasterPaint(computed) {
			if animation == nil || computed.AnimatedPaint&(style.AnimatedBackgroundColor|style.AnimatedBorders) == 0 {
				return
			}
			if animation.PaintStyle == nil {
				animation.PaintStyle = new(style.ComputedStyle)
			}
			paint := animation.PaintStyle
			if animation.PaintSource != computed {
				*paint = *computed
				animation.PaintSource = computed
				if raster := state.Textures.paint[htmlTextureKey{node: node, variant: "background"}]; raster != nil {
					raster.style = nil
				}
			}
			paint.BackgroundColor = style.ColorValue(value.Colors[style.AnimationBackground].Resolve(foreground).Color())
			paint.BorderColor.Top = style.ColorValue(value.Colors[style.AnimationBorderTop].Resolve(foreground).Color())
			paint.BorderColor.Right = style.ColorValue(value.Colors[style.AnimationBorderRight].Resolve(foreground).Color())
			paint.BorderColor.Bottom = style.ColorValue(value.Colors[style.AnimationBorderBottom].Resolve(foreground).Color())
			paint.BorderColor.Left = style.ColorValue(value.Colors[style.AnimationBorderLeft].Resolve(foreground).Color())
			if texture := htmlPaintTextureFor(state, node, paint, sprite.Width, sprite.Height); texture != nil {
				mutable := ecs.GetMut[scene.Sprite2D](world, entity)
				mutable.Texture, mutable.Tint, mutable.Shape = texture, graphics.White, scene.Shape2D{}
				state.Textures.bind(entity, texture)
			}
			return
		}
		fill = htmlAnimationColor(value.Colors[style.AnimationBackground].Resolve(foreground))
		if child.role == htmlRoleSurface {
			if box := state.BoxesByNode[node]; box != nil && (htmlPropagatesCanvasBackground(box) || htmlOverflowBackgroundCovered(box)) {
				fill = graphics.ColorRGBA{}
			}
		}
		if sprite.Shape.BorderWidth > 0 {
			border = htmlAnimationColor(value.Colors[style.AnimationBorderTop].Resolve(foreground))
		}
	default:
		return
	}
	if sprite.Shape.Fill != fill || sprite.Shape.Border != border {
		mutable := ecs.GetMut[scene.Sprite2D](world, entity)
		mutable.Shape.Fill, mutable.Shape.Border = fill, border
	}
}
