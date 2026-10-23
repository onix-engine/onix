//go:build !onix_release

package project

import (
	"fmt"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
)

func (importer *gltfImporter) appendMaterialSurface(value *compiledModelMaterial, material gltfMaterial) error {
	s := &value.Surface
	*s = compiledMaterialSurface{IOR: 1.5, EmissiveStrength: 1, NormalScale: 1, OcclusionStrength: 1, SpecularFactor: 1, ClearcoatNormalScale: 1, SpecularColor: [3]float32{1, 1, 1}, AlphaCutoff: 0.5, DoubleSided: material.DoubleSided, Unlit: material.Extensions.Unlit != nil}
	if ior := material.Extensions.IOR; ior != nil && ior.IOR != nil {
		s.IOR = *ior.IOR
	}
	if emission := material.Extensions.EmissiveStrength; emission != nil && emission.Strength != nil {
		s.EmissiveStrength = *emission.Strength
	}
	switch material.AlphaMode {
	case "", "OPAQUE":
	case "MASK":
		s.AlphaMode = graphics.AlphaMask
	case "BLEND":
		s.AlphaMode = graphics.AlphaBlend
	default:
		return fmt.Errorf("invalid alphaMode %q", material.AlphaMode)
	}
	if material.AlphaCutoff != nil {
		s.AlphaCutoff = *material.AlphaCutoff
	}
	if material.NormalTexture != nil && material.NormalTexture.Scale != nil {
		s.NormalScale = *material.NormalTexture.Scale
	}
	if material.OcclusionTexture != nil && material.OcclusionTexture.Strength != nil {
		s.OcclusionStrength = *material.OcclusionTexture.Strength
	}
	refs := [render.MeshSurfaceTextureSlots]*gltfTextureRef{material.OcclusionTexture, material.EmissiveTexture}
	if specular := material.Extensions.Specular; specular != nil {
		if specular.Factor != nil {
			s.SpecularFactor = *specular.Factor
		}
		if specular.Color != nil {
			if len(*specular.Color) != 3 {
				return fmt.Errorf("specular color factor must contain three values")
			}
			copy(s.SpecularColor[:], *specular.Color)
		}
		refs[2], refs[3] = specular.Texture, specular.ColorTexture
	}
	if s.Unlit && (material.Extensions.Specular != nil || material.Extensions.Clearcoat != nil || material.Extensions.Sheen != nil || material.Extensions.Anisotropy != nil || material.Extensions.IOR != nil || material.Extensions.EmissiveStrength != nil) {
		return fmt.Errorf("PBR material extensions cannot be combined with unlit")
	}
	if coat := material.Extensions.Clearcoat; coat != nil {
		if coat.Factor != nil {
			s.ClearcoatFactor = *coat.Factor
		}
		if coat.Roughness != nil {
			s.ClearcoatRoughness = *coat.Roughness
		}
		if coat.NormalTexture != nil && coat.NormalTexture.Scale != nil {
			s.ClearcoatNormalScale = *coat.NormalTexture.Scale
		}
		refs[4], refs[5], refs[6] = coat.Texture, coat.RoughnessTexture, coat.NormalTexture
	}
	if sheen := material.Extensions.Sheen; sheen != nil {
		if sheen.Color != nil {
			if len(*sheen.Color) != 3 {
				return fmt.Errorf("sheen color factor must contain three values")
			}
			copy(s.SheenColor[:], *sheen.Color)
		}
		if sheen.Roughness != nil {
			s.SheenRoughness = *sheen.Roughness
		}
		refs[7], refs[8] = sheen.ColorTexture, sheen.RoughnessTexture
	}
	if anisotropy := material.Extensions.Anisotropy; anisotropy != nil {
		if anisotropy.Strength != nil {
			s.AnisotropyStrength = *anisotropy.Strength
		}
		if anisotropy.Rotation != nil {
			s.AnisotropyRotation = *anisotropy.Rotation
		}
		refs[9] = anisotropy.Texture
	}
	if err := s.validate(); err != nil {
		return err
	}
	for _, ref := range [...]*gltfTextureRef{material.PBR.BaseColorTexture, material.NormalTexture, material.PBR.MetallicRoughnessTexture} {
		if ref != nil && ref.TexCoord != 0 {
			return fmt.Errorf("TEXCOORD_%d is not supported", ref.TexCoord)
		}
	}
	for slot, ref := range refs {
		if ref == nil {
			continue
		}
		if ref.TexCoord != 0 {
			return fmt.Errorf("TEXCOORD_%d is not supported", ref.TexCoord)
		}
		t := &s.Textures[slot]
		var err error
		t.Sampling, err = importer.textureSampling(ref.Index)
		if err != nil {
			return err
		}
		t.Sampling.SRGB = slot == 1 || slot == 3 || slot == 7
		t.Path, t.Data, t.Mime, err = importer.textureReference(ref.Index)
		if err != nil {
			return err
		}
	}
	return nil
}
