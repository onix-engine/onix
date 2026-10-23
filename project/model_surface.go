package project

import (
	"fmt"

	"github.com/diiyw/onix/graphics"
)

func (s compiledMaterialSurface) validate() error {
	if !finiteFloat32(s.IOR) || (s.IOR != 0 && s.IOR < 1) {
		return fmt.Errorf("IOR must be finite and either zero or at least 1")
	}
	if !finiteFloat32(s.EmissiveStrength) || s.EmissiveStrength < 0 {
		return fmt.Errorf("emissive strength must be finite and non-negative")
	}
	for _, v := range [...]float32{s.OcclusionStrength, s.SpecularFactor, s.ClearcoatFactor, s.ClearcoatRoughness, s.SheenRoughness, s.AnisotropyStrength} {
		if !finiteFloat32(v) || v < 0 || v > 1 {
			return fmt.Errorf("surface factors must be finite and between 0 and 1")
		}
	}
	for _, v := range s.SpecularColor {
		if !finiteFloat32(v) || v < 0 {
			return fmt.Errorf("specular color must be finite and non-negative")
		}
	}
	for _, v := range s.SheenColor {
		if !finiteFloat32(v) || v < 0 || v > 1 {
			return fmt.Errorf("sheen color must be finite and between 0 and 1")
		}
	}
	if !finiteFloat32(s.ClearcoatNormalScale) || !finiteFloat32(s.AnisotropyRotation) {
		return fmt.Errorf("invalid clearcoat normal scale or anisotropy rotation")
	}
	if !finiteFloat32(s.NormalScale) || !finiteFloat32(s.AlphaCutoff) || s.AlphaCutoff < 0 || s.AlphaMode > graphics.AlphaBlend {
		return fmt.Errorf("invalid normal scale, alpha cutoff or alpha mode")
	}
	return nil
}

func (r *Resources) loadMaterialSurface(material *graphics.StandardMaterial, source compiledMaterialSurface, owned *[]*graphics.Texture) error {
	material.AlphaMode, material.AlphaCutoff = source.AlphaMode, source.AlphaCutoff
	material.IOR, material.EmissiveStrength = source.IOR, source.EmissiveStrength
	material.Surface = graphics.MaterialSurface{
		NormalScale: source.NormalScale, OcclusionStrength: source.OcclusionStrength,
		SpecularFactor: source.SpecularFactor, SpecularColor: graphics.ColorRGBA{R: source.SpecularColor[0], G: source.SpecularColor[1], B: source.SpecularColor[2], A: 1},
		Unlit: source.Unlit, DoubleSided: source.DoubleSided,
		ClearcoatFactor: source.ClearcoatFactor, ClearcoatRoughness: source.ClearcoatRoughness, ClearcoatNormalScale: source.ClearcoatNormalScale,
		SheenColor: graphics.ColorRGBA{R: source.SheenColor[0], G: source.SheenColor[1], B: source.SheenColor[2], A: 1}, SheenRoughness: source.SheenRoughness,
		AnisotropyStrength: source.AnisotropyStrength, AnisotropyRotation: source.AnisotropyRotation,
	}

	textures := [...]**graphics.Texture{&material.Surface.OcclusionTexture, &material.Surface.EmissiveTexture, &material.Surface.SpecularTexture, &material.Surface.SpecularColorTexture, &material.Surface.ClearcoatTexture, &material.Surface.ClearcoatRoughnessTexture, &material.Surface.ClearcoatNormalTexture, &material.Surface.SheenColorTexture, &material.Surface.SheenRoughnessTexture, &material.Surface.AnisotropyTexture}
	for slot, target := range textures {
		t := source.Textures[slot]
		if t.Path == "" && len(t.Data) == 0 {
			continue
		}
		var err error
		*target, err = r.loadImportedTexture(t.Path, t.Data, t.Mime, t.Sampling)
		if err != nil {
			return err
		}
		if owned != nil && len(t.Data) != 0 {
			*owned = append(*owned, *target)
		}
	}
	return nil
}
