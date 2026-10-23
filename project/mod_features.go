package project

import "slices"

// RuntimeFeatures includes explicitly selected systems and systems required by
// used content. Unreferenced definitions do not add features to the executable.
func (p *ProjectPlan) RuntimeFeatures() []RuntimeFeatureID {
	features := make(map[RuntimeFeatureID]bool, len(p.Spec.Engine.Features))
	for _, f := range p.Spec.Engine.Features {
		features[f] = true
	}
	used := make([]bool, len(p.Mods))
	for _, s := range p.Scenes {
		for _, i := range s.Instances {
			used[i.Mod] = true
		}
	}
	content := func(c ModComponents) {
		if c.Sprite != nil || c.Trail != nil || c.Material != nil || c.Shader != nil && c.Model == nil || c.Text != nil || c.HTML != nil || c.Emitter != nil || c.Tilemap != nil || c.Camera2D != nil {
			features[FeatureRender2D] = true
		}
		if c.Input != nil && c.Input.Pointer {
			features[FeatureInput2D] = true
		}
		if c.Animation != nil {
			features[FeatureAnimations2D] = true
		}
		if c.Trail != nil {
			features[FeatureSpriteTrails2D] = true
		}
		if c.HTML != nil {
			features[FeatureUIHTML] = true
		}
		if c.Emitter != nil {
			features[FeatureParticles2D] = true
		}
		if c.Tilemap != nil {
			features[FeatureTilemaps2D] = true
		}
		if c.Model != nil || c.Camera != nil || c.AmbientLight != nil || c.DirectionalLight != nil || c.PointLight != nil || c.SpotLight != nil || c.AreaLight != nil {
			features[FeatureRender3D] = true
		}
		if c.RigidBody2D != nil {
			features[FeaturePhysics2D] = true
		}
		if c.Spine != nil {
			features[FeatureSpine2D] = true
			features[FeatureRender2D] = true
		}
		if c.RigidBody3D != nil {
			features[FeaturePhysics3D] = true
		}
	}
	for i, m := range p.Mods {
		if used[i] {
			for _, slot := range m.Slots {
				if slot.Type == VisualValue {
					features[FeatureRender2D] = true
				}
			}
			content(m.Spec.Components)
			for _, e := range m.Elements {
				content(e.Spec.Components)
			}
		}
	}
	result := make([]RuntimeFeatureID, 0, len(features))
	for f := range features {
		result = append(result, f)
	}
	slices.Sort(result)
	return result
}
