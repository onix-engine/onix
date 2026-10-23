package project

import (
	"fmt"

	"github.com/diiyw/onix/scene"
)

func validateModAnimationTargets(plan *ModPlan) error {
	content := make(map[string]ModComponents, len(plan.Elements)+len(plan.Slots)+1)
	content[""] = plan.Spec.Components
	for _, element := range plan.Elements {
		content[element.Spec.ID] = element.Spec.Components
	}
	for _, slot := range plan.Slots {
		content[slot.Name] = ModComponents{}
	}
	claims := make(map[scene.AnimationBinding]string)
	validate := func(owner string, c ModComponents) error {
		if c.Animation == nil {
			return nil
		}
		claim := func(binding scene.AnimationBinding) error {
			if binding.Path == "" {
				binding.Path = owner
			}
			target, exists := content[binding.Path]
			if !exists {
				return fmt.Errorf("animation %q: target %q is not private content", owner, binding.Path)
			}
			switch binding.Target {
			case scene.AnimationTargetSpriteTexture, scene.AnimationTargetSpriteAlpha:
				implicitSprite := target.Animation != nil && len(target.Animation.Clip.TextureTracks) > 0
				if target.Sprite == nil && !implicitSprite {
					return fmt.Errorf("animation target %q requires a sprite", binding.Path)
				}
			case scene.AnimationTargetTransformPositionZ, scene.AnimationTargetTransformScaleZ, scene.AnimationTargetTransformRotationX, scene.AnimationTargetTransformRotationY:
				if effectiveModSpace(plan.Spec.Space) != CoordinateSpaceWorld {
					return fmt.Errorf("animation target %s requires world space", binding.Target)
				}
			}
			if previous, exists := claims[binding]; exists && previous != owner {
				return fmt.Errorf("animation target %q %s is controlled by both %q and %q", binding.Path, binding.Target, previous, owner)
			}
			claims[binding] = owner
			return nil
		}
		if err := validateModTrackClaims(c.Animation.Clip.FloatTracks, claim); err != nil {
			return err
		}
		if err := validateModTrackClaims(c.Animation.Clip.TextureTracks, claim); err != nil {
			return err
		}
		return validateModTrackClaims(c.Animation.Clip.BoolTracks, claim)
	}
	if err := validate("", plan.Spec.Components); err != nil {
		return err
	}
	for _, element := range plan.Elements {
		if err := validate(element.Spec.ID, element.Spec.Components); err != nil {
			return err
		}
	}
	return nil
}

func validateModTrackClaims[T any](tracks []AnimationTrackSpec[T], claim func(scene.AnimationBinding) error) error {
	for _, track := range tracks {
		if err := claim(track.Binding); err != nil {
			return err
		}
	}
	return nil
}
