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
	if err := decodeStrictJSONValue(data, &target); err != nil {
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

// Decode into the default value so a malformed field cannot partially update dst.
func decodeStrictInto[T any](data []byte, dst *T, value T) error {
	if err := decodeStrictJSONValue(data, &value); err != nil {
		return err
	}
	*dst = value
	return nil
}

func (m *gameManifest) UnmarshalJSON(data []byte) error {
	type plain gameManifest
	var fields struct {
		Engine *struct {
			TextureCompression *bool `json:"textureCompression"`
			EngineConfig
		} `json:"engine"`
		plain
	}
	if err := decodeStrictJSONValue(data, &fields); err != nil {
		return err
	}
	if fields.Engine != nil {
		fields.plain.Engine = fields.Engine.EngineConfig
		compression := fields.Engine.TextureCompression
		fields.plain.Engine.TextureCompression = compression == nil || *compression
	}
	*m = gameManifest(fields.plain)
	return nil
}

func (s *Camera3DSpec) UnmarshalJSON(data []byte) error {
	type plain Camera3DSpec
	return decodeStrictInto(data, (*plain)(s), plain(NewCamera3DSpec()))
}

func (s *DirectionalLightSpec) UnmarshalJSON(data []byte) error {
	type plain DirectionalLightSpec
	return decodeStrictInto(data, (*plain)(s), plain(NewDirectionalLightSpec()))
}

func (s *ShadowSpec) UnmarshalJSON(data []byte) error {
	type plain ShadowSpec
	return decodeStrictInto(data, (*plain)(s), plain(*NewShadowSpec()))
}

func (s *SSRSpec) UnmarshalJSON(data []byte) error {
	type plain SSRSpec
	return decodeStrictInto(data, (*plain)(s), plain(*NewSSRSpec()))
}

func (s *SSAOSpec) UnmarshalJSON(data []byte) error {
	type plain SSAOSpec
	return decodeStrictInto(data, (*plain)(s), plain(*NewSSAOSpec()))
}

func (s *FogSpec) UnmarshalJSON(data []byte) error {
	type plain FogSpec
	return decodeStrictInto(data, (*plain)(s), plain(*NewFogSpec()))
}

func (s *PunctualShadowSpec) UnmarshalJSON(data []byte) error {
	type plain PunctualShadowSpec
	return decodeStrictInto(data, (*plain)(s), plain(*NewPunctualShadowSpec()))
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
	return decodeStrictInto(data, (*plain)(s), plain(NewPointLightSpec()))
}

func (s *SpotLightSpec) UnmarshalJSON(data []byte) error {
	type plain SpotLightSpec
	return decodeStrictInto(data, (*plain)(s), plain(NewSpotLightSpec()))
}

func (s *SceneNodeSpec) UnmarshalJSON(data []byte) error {
	type plain SceneNodeSpec
	return decodeStrictInto(data, (*plain)(s), plain{ScaleX: 1, ScaleY: 1, ScaleZ: 1, Alpha: 1})
}

func (s *SpriteComponentSpec) UnmarshalJSON(data []byte) error {
	type plain SpriteComponentSpec
	return decodeStrictInto(data, (*plain)(s), plain{})
}

func (s *AnimationComponentSpec) UnmarshalJSON(data []byte) error {
	type plain AnimationComponentSpec
	return decodeStrictInto(data, (*plain)(s), plain{Speed: 1})
}

func (s *SpineComponentSpec) UnmarshalJSON(data []byte) error {
	type plain SpineComponentSpec
	return decodeStrictInto(data, (*plain)(s), plain{Loop: true, Speed: 1})
}

func (s *RigidBody2DSpec) UnmarshalJSON(data []byte) error {
	type plain RigidBody2DSpec
	return decodeStrictInto(data, (*plain)(s), plain(NewRigidBody2DSpec()))
}

func (s *CircleCollider2DSpec) UnmarshalJSON(data []byte) error {
	type plain CircleCollider2DSpec
	return decodeStrictInto(data, (*plain)(s), plain(NewCircleCollider2DSpec()))
}

func (s *BoxCollider2DSpec) UnmarshalJSON(data []byte) error {
	type plain BoxCollider2DSpec
	return decodeStrictInto(data, (*plain)(s), plain(NewBoxCollider2DSpec()))
}

func (s *SegmentCollider2DSpec) UnmarshalJSON(data []byte) error {
	type plain SegmentCollider2DSpec
	return decodeStrictInto(data, (*plain)(s), plain(NewSegmentCollider2DSpec()))
}

func (s *RigidBody3DSpec) UnmarshalJSON(data []byte) error {
	type plain RigidBody3DSpec
	return decodeStrictInto(data, (*plain)(s), plain(NewRigidBody3DSpec()))
}

func (s *TriangleMeshCollider3DSpec) UnmarshalJSON(data []byte) error {
	type plain TriangleMeshCollider3DSpec
	return decodeStrictInto(data, (*plain)(s), plain(NewTriangleMeshCollider3DSpec()))
}

func (s *ConvexHullCollider3DSpec) UnmarshalJSON(data []byte) error {
	type plain ConvexHullCollider3DSpec
	return decodeStrictInto(data, (*plain)(s), plain(NewConvexHullCollider3DSpec()))
}

func (s *CompoundCollider3DSpec) UnmarshalJSON(data []byte) error {
	type plain CompoundCollider3DSpec
	return decodeStrictInto(data, (*plain)(s), plain(NewCompoundCollider3DSpec()))
}

func (s *AreaLightSpec) UnmarshalJSON(data []byte) error {
	type plain AreaLightSpec
	return decodeStrictInto(data, (*plain)(s), plain(NewAreaLightSpec()))
}

func (s *ModelMaterialSpec) UnmarshalJSON(data []byte) error {
	type alias ModelMaterialSpec
	var value struct {
		Metallic         *float32 `json:"metallic"`
		Roughness        *float32 `json:"roughness"`
		IOR              *float32 `json:"ior"`
		EmissiveStrength *float32 `json:"emissiveStrength"`
		alias
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
		Metallic         *float32 `json:"metallic,omitempty"`
		Roughness        *float32 `json:"roughness,omitempty"`
		IOR              *float32 `json:"ior,omitempty"`
		EmissiveStrength *float32 `json:"emissiveStrength,omitempty"`
		alias
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
