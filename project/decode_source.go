//go:build !onix_release

package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	pathpkg "path"
)

func decodeJSON[T any](data []byte, path string) (T, error) {
	var target T
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&target); err != nil {
		return target, fmt.Errorf("decode %s: %w", pathpkg.Base(path), err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return target, fmt.Errorf("decode %s: multiple JSON values are not allowed", pathpkg.Base(path))
		}
		return target, fmt.Errorf("decode %s: %w", pathpkg.Base(path), err)
	}
	return target, nil
}

func decodeStrictJSONValue(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func (s *Camera3DSpec) UnmarshalJSON(data []byte) error {
	type plain Camera3DSpec
	value := plain(NewCamera3DSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = Camera3DSpec(value)
	return nil
}

func (s *DirectionalLightSpec) UnmarshalJSON(data []byte) error {
	type plain DirectionalLightSpec
	value := plain(NewDirectionalLightSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = DirectionalLightSpec(value)
	return nil
}

func (s *ShadowSpec) UnmarshalJSON(data []byte) error {
	type plain ShadowSpec
	value := plain(*NewShadowSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = ShadowSpec(value)
	return nil
}

func (s *SSRSpec) UnmarshalJSON(data []byte) error {
	type plain SSRSpec
	value := plain(*NewSSRSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = SSRSpec(value)
	return nil
}

func (s *SSAOSpec) UnmarshalJSON(data []byte) error {
	type plain SSAOSpec
	value := plain(*NewSSAOSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = SSAOSpec(value)
	return nil
}

func (s *FogSpec) UnmarshalJSON(data []byte) error {
	type plain FogSpec
	value := plain(*NewFogSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = FogSpec(value)
	return nil
}

func (s *PunctualShadowSpec) UnmarshalJSON(data []byte) error {
	type plain PunctualShadowSpec
	value := plain(*NewPunctualShadowSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = PunctualShadowSpec(value)
	return nil
}

func (s *AmbientLightSpec) UnmarshalJSON(data []byte) error {
	type plain AmbientLightSpec
	value := plain(NewAmbientLightSpec())
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*s = AmbientLightSpec(value)
	return nil
}

func (s *PointLightSpec) UnmarshalJSON(data []byte) error {
	type plain PointLightSpec
	value := plain(NewPointLightSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = PointLightSpec(value)
	return nil
}

func (s *SpotLightSpec) UnmarshalJSON(data []byte) error {
	type plain SpotLightSpec
	value := plain(NewSpotLightSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = SpotLightSpec(value)
	return nil
}

func (s *SceneNodeSpec) UnmarshalJSON(data []byte) error {
	type plain SceneNodeSpec
	value := plain{ScaleX: 1, ScaleY: 1, ScaleZ: 1, Alpha: 1}
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = SceneNodeSpec(value)
	return nil
}

func (s *SpriteComponentSpec) UnmarshalJSON(data []byte) error {
	type plain SpriteComponentSpec
	var value plain
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = SpriteComponentSpec(value)
	return nil
}

func (s *AnimationComponentSpec) UnmarshalJSON(data []byte) error {
	type plain AnimationComponentSpec
	value := plain{Speed: 1}
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = AnimationComponentSpec(value)
	return nil
}

func (s *SpineComponentSpec) UnmarshalJSON(data []byte) error {
	type plain SpineComponentSpec
	value := plain{Loop: true, Speed: 1}
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = SpineComponentSpec(value)
	return nil
}

func (s *RigidBody2DSpec) UnmarshalJSON(data []byte) error {
	type plain RigidBody2DSpec
	value := plain(NewRigidBody2DSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = RigidBody2DSpec(value)
	return nil
}

func (s *CircleCollider2DSpec) UnmarshalJSON(data []byte) error {
	type plain CircleCollider2DSpec
	value := plain(NewCircleCollider2DSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = CircleCollider2DSpec(value)
	return nil
}

func (s *BoxCollider2DSpec) UnmarshalJSON(data []byte) error {
	type plain BoxCollider2DSpec
	value := plain(NewBoxCollider2DSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = BoxCollider2DSpec(value)
	return nil
}

func (s *SegmentCollider2DSpec) UnmarshalJSON(data []byte) error {
	type plain SegmentCollider2DSpec
	value := plain(NewSegmentCollider2DSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = SegmentCollider2DSpec(value)
	return nil
}

func (s *RigidBody3DSpec) UnmarshalJSON(data []byte) error {
	type plain RigidBody3DSpec
	value := plain(NewRigidBody3DSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = RigidBody3DSpec(value)
	return nil
}

func (s *TriangleMeshCollider3DSpec) UnmarshalJSON(data []byte) error {
	type plain TriangleMeshCollider3DSpec
	value := plain(NewTriangleMeshCollider3DSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = TriangleMeshCollider3DSpec(value)
	return nil
}

func (s *ConvexHullCollider3DSpec) UnmarshalJSON(data []byte) error {
	type plain ConvexHullCollider3DSpec
	value := plain(NewConvexHullCollider3DSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = ConvexHullCollider3DSpec(value)
	return nil
}

func (s *CompoundCollider3DSpec) UnmarshalJSON(data []byte) error {
	type plain CompoundCollider3DSpec
	value := plain(NewCompoundCollider3DSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = CompoundCollider3DSpec(value)
	return nil
}

func (s *AreaLightSpec) UnmarshalJSON(data []byte) error {
	type plain AreaLightSpec
	value := plain(NewAreaLightSpec())
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*s = AreaLightSpec(value)
	return nil
}

func (s *ModelMaterialSpec) UnmarshalJSON(data []byte) error {
	type alias ModelMaterialSpec
	var value struct {
		alias
		Metallic         *float32 `json:"metallic"`
		Roughness        *float32 `json:"roughness"`
		IOR              *float32 `json:"ior"`
		EmissiveStrength *float32 `json:"emissiveStrength"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*s = ModelMaterialSpec(value.alias)
	if value.Metallic != nil {
		s.Metallic, s.MetallicSet = *value.Metallic, true
	}
	if value.Roughness != nil {
		s.PerceptualRoughness, s.RoughnessSet = *value.Roughness, true
	}
	if value.IOR != nil {
		s.IOR, s.IORSet = *value.IOR, true
	}
	if value.EmissiveStrength != nil {
		s.EmissiveStrength, s.EmissiveStrengthSet = *value.EmissiveStrength, true
	}
	return nil
}

func (s ModelMaterialSpec) MarshalJSON() ([]byte, error) {
	type alias ModelMaterialSpec
	value := struct {
		alias
		Metallic         *float32 `json:"metallic,omitempty"`
		Roughness        *float32 `json:"roughness,omitempty"`
		IOR              *float32 `json:"ior,omitempty"`
		EmissiveStrength *float32 `json:"emissiveStrength,omitempty"`
	}{alias: alias(s)}
	if s.MetallicSet || s.Metallic != 0 {
		value.Metallic = &s.Metallic
	}
	if s.RoughnessSet || s.PerceptualRoughness != 0 {
		value.Roughness = &s.PerceptualRoughness
	}
	if s.IORSet || s.IOR != 0 {
		value.IOR = &s.IOR
	}
	if s.EmissiveStrengthSet || s.EmissiveStrength != 0 {
		value.EmissiveStrength = &s.EmissiveStrength
	}
	return json.Marshal(value)
}
