package project

import (
	"github.com/diiyw/onix/ecs"
	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene"
)

func modContentSize(c ModComponents) (width, height float32) {
	if len(c.Size) == 2 {
		width, height = c.Size[0], c.Size[1]
	}
	if c.Sprite != nil && len(c.Sprite.Size) == 2 {
		width, height = c.Sprite.Size[0], c.Sprite.Size[1]
	}
	return
}

func (r *Resources) AppendModContent2D(b *ecs.Bundle, c ModComponents) error {
	width, height := float32(0), float32(0)
	if len(c.Size) == 2 {
		width, height = c.Size[0], c.Size[1]
		b.Put(scene.Size2D{Width: width, Height: height})
	}
	if s := c.Sprite; s != nil {
		tint := graphics.White
		color := s.Color
		if s.Texture != nil {
			color = s.Texture.Color
		}
		if color != "" {
			var err error
			tint, err = ParseColor(color, 1)
			if err != nil {
				return err
			}
		}
		value := scene.NewSprite2D()
		if s.Texture != nil && (s.Texture.Image != "" || s.Texture.Atlas != "") {
			texture, err := r.texture(s.Texture)
			if err != nil {
				return err
			}
			value = scene.Sprite2DFromTexture(texture)
			if slice := s.Texture.Slice; slice != nil {
				if err := slice.Validate(texture.LogicalWidth(), texture.LogicalHeight()); err != nil {
					return err
				}
				value.Slice = &scene.NineSlice2D{Left: float32(slice.Left), Right: float32(slice.Right), Top: float32(slice.Top), Bottom: float32(slice.Bottom)}
			}
		} else {
			value.Shape = scene.RoundedRectShape2D(tint, 0)
		}
		if c.Shader != nil && c.Shader.HasSource() {
			if value.Texture == nil {
				value.Texture = graphics.CreateTextureByColor(tint)
				value.Shape = scene.Shape2D{}
			}
		}
		if len(s.Size) == 2 {
			width, height = s.Size[0], s.Size[1]
		}
		if width > 0 {
			value.Width = width
		}
		if height > 0 {
			value.Height = height
		}
		if s.Alpha != nil {
			value.Alpha = *s.Alpha
		}
		b.Put(value)
	}
	if spec := c.Material; spec != nil {
		material := &ModMaterial2D{Custom: spec.Custom}
		b.Put(scene.MeshMaterial2D[ModMaterial2D]{Material: material})
	}
	if err := r.modShader(b, c.Shader, c.Model != nil); err != nil {
		return err
	}
	if t := c.Text; t != nil {
		value := scene.NewText2D(t.Value)
		if t.Font != "" {
			font, err := r.Font(t.Font)
			if err != nil {
				return err
			}
			value.Font = font
		}
		if t.FontSize > 0 {
			value.FontSize = t.FontSize
		}
		if err := configureTextLayout(&value, t, "module text"); err != nil {
			return err
		}
		value.OutlineWidth, value.MaxWidth = t.OutlineWidth, t.MaxWidth
		if t.Color != "" {
			color, err := ParseColor(t.Color, 1)
			if err != nil {
				return err
			}
			value.Color = color
		}
		if t.OutlineColor != "" {
			color, err := ParseColor(t.OutlineColor, 1)
			if err != nil {
				return err
			}
			value.OutlineColor = color
		}
		if t.Shadow != nil {
			color, err := ParseColor(t.Shadow.Color, 1)
			if err != nil {
				return err
			}
			value.ShadowX, value.ShadowY, value.ShadowColor = t.Shadow.OffsetX, t.Shadow.OffsetY, color
		}
		b.Put(value)
	}
	if spec := c.Camera2D; spec != nil {
		camera := scene.NewCamera()
		camera.Order, camera.Viewport = spec.Order, spec.Viewport
		if spec.Clear != nil {
			camera.Clear = *spec.Clear
		}
		if spec.ClearColor != "" {
			color, err := ParseColor(spec.ClearColor, 1)
			if err != nil {
				return err
			}
			camera.ClearColor = color
		}
		projection := scene.OrthographicProjection2D()
		if spec.Scale > 0 {
			projection.Orthographic.Scale = spec.Scale
		}
		b.Put(camera)
		b.Put(scene.Camera2D{})
		b.Put(projection)
	}
	return nil
}

func (r *Resources) AppendModContentAnimation(b *ecs.Bundle, c ModComponents) error {
	width, height := modContentSize(c)
	if a := c.Animation; a != nil {
		clip := r.modClips[a]
		if clip == nil {
			var err error
			clip, err = r.compileAnimationClipSpec(a.Clip, "module animation")
			if err != nil {
				return err
			}
			if r.modClips == nil {
				r.modClips = make(map[*AnimationComponentSpec]*scene.AnimationClip)
			}
			r.modClips[a] = clip
		}
		animator := scene.NewAnimator(clip, a.Clip.Loop)
		if a.Speed > 0 {
			animator.Speed = a.Speed
		}
		if c.Sprite == nil && len(clip.TextureTracks) > 0 {
			sprite := scene.Sprite2DFromTexture(animator.CurrentTexture())
			if width > 0 {
				sprite.Width = width
			}
			if height > 0 {
				sprite.Height = height
			}
			b.Put(sprite)
		}
		b.Put(animator)
	}
	return nil
}
