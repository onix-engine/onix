package project

import (
	"github.com/onix-engine/onix/ecs"
	"github.com/onix-engine/onix/scene"
)

// Build sprite and animation values once for creation, authored updates and
// the small 2D profiles. The caller chooses how to install the typed values.
func (b *Bundle) spriteAnimationForNode(node sceneSpawnNode) (sprite scene.Sprite2D, animator scene.Animator, hasSprite bool, err error) {
	spec := node.Spec
	requiresSprite := false
	if node.HasAnimation {
		animator, requiresSprite, err = b.ecsAnimationForSpec(spec, &node)
		if err != nil {
			return
		}
	}
	hasSprite = node.HasSprite || requiresSprite
	if node.HasSprite {
		sprite, err = b.ecsSpriteForSpec(spec)
		if err != nil {
			return
		}
		if requiresSprite {
			if texture := animator.CurrentTexture(); texture != nil {
				sprite.Texture, sprite.Slice = texture, nil
				if spec.Width <= 0 {
					sprite.Width = float32(texture.LogicalWidth())
				}
				if spec.Height <= 0 {
					sprite.Height = float32(texture.LogicalHeight())
				}
			}
		}
	} else if requiresSprite {
		sprite = scene.Sprite2DFromTexture(animator.CurrentTexture())
		sprite.Alpha = spec.Alpha
		if spec.Width > 0 {
			sprite.Width = float32(spec.Width)
		}
		if spec.Height > 0 {
			sprite.Height = float32(spec.Height)
		}
	}
	return
}

func (b *Bundle) putSpriteAnimationComponents(components *ecs.Bundle, node sceneSpawnNode) error {
	if !node.HasSprite && !node.HasAnimation {
		return nil
	}
	sprite, animator, hasSprite, err := b.spriteAnimationForNode(node)
	if err != nil {
		return err
	}
	if hasSprite {
		components.Put(sprite)
	}
	if node.HasAnimation {
		components.Put(animator)
	}
	return nil
}
