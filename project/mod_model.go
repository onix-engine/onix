package project

import (
	"fmt"
	"github.com/diiyw/onix/graphics"
	"strings"
)

// ModelRig reads a model already loaded by this module's private content.
func (r *Resources) ModelRig(name string) (*ModelRig, error) {
	model, ok, err := r.modModels.get(name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("private model %q is not loaded", name)
	}
	return newModelRig(model.asset)
}

func (r *Resources) loadModModel(model ModelSpec) (modelResource, error) {
	var mesh *graphics.Mesh
	var asset compiledModelResource
	var err error
	if model.Mesh == nil {
		modelPath := strings.TrimSpace(model.Path)
		if strings.TrimSpace(model.CollisionPath) != "" {
			if modelPath != "" {
				normalized, normalizeErr := NormalizeResourcePath(modelPath)
				if normalizeErr != nil {
					return modelResource{}, normalizeErr
				}
				resolved, resolveErr := ResolveResourcePath(r.baseDir, normalized)
				if resolveErr != nil {
					return modelResource{}, resolveErr
				}
				exists, existsErr := projectFileExists(r.source, resolved)
				if existsErr != nil {
					return modelResource{}, existsErr
				}
				if exists {
					asset, mesh, err = r.loadModelAsset(modelPath)
					if err != nil {
						return modelResource{}, err
					}
				}
			}
		} else {
			asset, mesh, err = r.loadModelAsset(modelPath)
			if err != nil {
				return modelResource{}, err
			}
		}
	} else {
		mesh, err = model.Mesh.build()
		if err != nil {
			return modelResource{}, err
		}
	}
	if model.AnimationPath != "" {
		animations, err := r.loadModelAnimations(model.AnimationPath, asset.Nodes)
		if err != nil {
			if mesh != nil {
				_ = mesh.Dispose()
			}
			return modelResource{}, fmt.Errorf("model %q animations: %w", model.ID, err)
		}
		asset.Animations = append(asset.Animations, animations...)
	}
	result := modelResource{mesh: mesh, materials: make([]graphics.StandardMaterial, len(asset.Materials)+1)}
	result.material = &result.materials[0]
	keepMesh := false
	defer func() {
		if !keepMesh {
			_ = result.dispose()
		}
	}()
	baseColor := graphics.White
	if model.Material.BaseColor != "" {
		baseColor, err = r.ParseColor(model.Material.BaseColor, 1)
		if err != nil {
			return modelResource{}, fmt.Errorf("baseColor: %w", err)
		}
	}
	for materialIndex := range result.materials {
		material := &result.materials[materialIndex]
		*material = *graphics.NewStandardMaterial(baseColor.Linear())
		if len(asset.Submeshes) != 0 {
			material.Metallic, material.PerceptualRoughness = 1, 1
		}
		if shaderID := strings.TrimSpace(model.Material.Shader); shaderID != "" {
			material.Shader, err = r.MeshShader(shaderID)
			if err != nil {
				return modelResource{}, err
			}
		}
		if model.Material.MetallicSet || model.Material.Metallic != 0 {
			material.Metallic = model.Material.Metallic
		}
		if model.Material.PerceptualRoughness > 0 || model.Material.RoughnessSet {
			material.PerceptualRoughness = model.Material.PerceptualRoughness
		}
		if model.Material.Emissive != "" {
			material.Emissive, err = r.ParseColor(model.Material.Emissive, 1)
			material.Emissive = material.Emissive.Linear()
			if err != nil {
				return modelResource{}, fmt.Errorf("emissive: %w", err)
			}
		}
		if materialIndex < len(asset.Materials) {
			imported := asset.Materials[materialIndex]
			if err := r.loadMaterialSurface(material, imported.Surface, &result.ownedTextures); err != nil {
				return modelResource{}, err
			}
			if model.Material.BaseColor == "" {
				material.BaseColor = graphics.ColorRGBA{R: imported.BaseColor[0], G: imported.BaseColor[1], B: imported.BaseColor[2], A: imported.BaseColor[3]}
			}
			if model.Material.Emissive == "" {
				material.Emissive = graphics.ColorRGBA{R: imported.Emissive[0], G: imported.Emissive[1], B: imported.Emissive[2], A: 1}
			}
			if !model.Material.MetallicSet && model.Material.Metallic == 0 {
				material.Metallic = imported.Metallic
			}
			if !model.Material.RoughnessSet && model.Material.PerceptualRoughness == 0 {
				material.PerceptualRoughness = imported.Roughness
			}
			if (imported.BaseColorTexture != "" || len(imported.BaseColorTextureData) != 0) && model.Material.BaseColor == "" {
				material.BaseColorTexture, err = r.loadImportedTexture(imported.BaseColorTexture, imported.BaseColorTextureData, imported.BaseColorTextureMime, imported.BaseColorSampling)
				if err != nil {
					return modelResource{}, fmt.Errorf("material baseColorTexture: %w", err)
				}
				if len(imported.BaseColorTextureData) != 0 {
					result.ownedTextures = append(result.ownedTextures, material.BaseColorTexture)
				}
			}
			if imported.NormalTexture != "" || len(imported.NormalTextureData) != 0 {
				material.NormalTexture, err = r.loadImportedTexture(imported.NormalTexture, imported.NormalTextureData, imported.NormalTextureMime, imported.NormalSampling)
				if err != nil {
					return modelResource{}, fmt.Errorf("material normalTexture: %w", err)
				}
				if len(imported.NormalTextureData) != 0 {
					result.ownedTextures = append(result.ownedTextures, material.NormalTexture)
				}
			}
			if imported.MetallicRoughness != "" || len(imported.MetallicRoughnessData) != 0 {
				material.MetallicRoughnessTexture, err = r.loadImportedTexture(imported.MetallicRoughness, imported.MetallicRoughnessData, imported.MetallicRoughnessMime, imported.MetallicRoughnessSampling)
				if err != nil {
					return modelResource{}, fmt.Errorf("material metallicRoughnessTexture: %w", err)
				}
				if len(imported.MetallicRoughnessData) != 0 {
					result.ownedTextures = append(result.ownedTextures, material.MetallicRoughnessTexture)
				}
			}
		}
		if model.Material.IORSet || model.Material.IOR != 0 {
			material.IOR = model.Material.IOR
		}
		if model.Material.EmissiveStrengthSet || model.Material.EmissiveStrength != 0 {
			material.EmissiveStrength = model.Material.EmissiveStrength
		}
	}
	result.asset = asset
	if collisionPath := strings.TrimSpace(model.CollisionPath); collisionPath != "" {
		result.collision, err = r.loadCollisionResource(collisionPath, model.Collision)
		if err != nil {
			return modelResource{}, fmt.Errorf("collisionPath: %w", err)
		}
		result.hasCollision = true
	} else if model.Collision != "" {
		if mesh == nil {
			return modelResource{}, fmt.Errorf("collision requires a mesh or collisionPath")
		}
		result.collision, err = model.CookCollision(mesh)
		if err != nil {
			return modelResource{}, fmt.Errorf("collision: %w", err)
		}
		result.hasCollision = true
	}
	keepMesh = true
	return result, nil
}
