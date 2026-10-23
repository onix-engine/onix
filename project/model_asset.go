package project

import (
	"fmt"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/scene/render3d"
	"github.com/go-gl/mathgl/mgl32"
)

// ModelJoint is an imported skin joint in model-local space.
type ModelJoint struct {
	Name        string
	Node        int32
	Parent      int32
	InverseBind [16]float32
}

// ModelNode retains the source node index, hierarchy and initial transforms.
// Path is the unambiguous animation binding, including for duplicate names.
type ModelNode struct {
	Name   string
	Path   string
	Parent int32
	Local  [16]float32
	World  [16]float32
}

// ModelNodes returns immutable node metadata owned by the model cache.
func (b *Bundle) ModelNodes(modelID string) ([]ModelNode, error) {
	value, _, err := b.modelResourceForSpec(SceneNodeSpec{ID: modelID, Components: SceneComponents{Model: &ResourceComponentSpec{Resource: modelID}}})
	return value.asset.Nodes, err
}

// ModelDeformation creates reusable runtime state sized for the model's morph
// targets, joints and rigid nodes. Initial matrices preserve the source pose.
func (b *Bundle) ModelDeformation(modelID string) (render3d.Deformation, error) {
	value, _, err := b.modelResourceForSpec(SceneNodeSpec{ID: modelID, Components: SceneComponents{Model: &ResourceComponentSpec{Resource: modelID}}})
	if err != nil {
		return render3d.Deformation{}, err
	}
	result := render3d.Deformation{
		MorphWeights:    make([]float32, len(value.asset.MorphTargets)),
		InverseBind:     make([]mgl32.Mat4, len(value.asset.Joints)),
		JointMatrices:   make([]mgl32.Mat4, len(value.asset.Joints)),
		NodeMatrices:    make([]mgl32.Mat4, len(value.asset.Nodes)),
		NodeInverseBind: make([]mgl32.Mat4, len(value.asset.Nodes)),
	}
	for index, joint := range value.asset.Joints {
		result.InverseBind[index] = mgl32.Mat4(joint.InverseBind)
		result.JointMatrices[index] = mgl32.Ident4()
		if len(value.asset.Nodes) != 0 {
			result.JointMatrices[index] = mgl32.Mat4(value.asset.Nodes[joint.Node].World).Mul4(result.InverseBind[index])
		}
	}
	for index, node := range value.asset.Nodes {
		result.NodeMatrices[index] = mgl32.Ident4()
		result.NodeInverseBind[index] = mgl32.Mat4(node.World).Inv()
	}
	return result, nil
}

// ModelMorphTarget is an imported morph target and its per-vertex deltas.
type ModelMorphTarget struct {
	Name          string
	PositionDelta [][3]float32
	NormalDelta   [][3]float32
	TangentDelta  [][3]float32
}

// ModelSkinVertex contains up to four joint influences for one model vertex.
type ModelSkinVertex struct {
	Joints  [4]uint32
	Weights [4]float32
	Active  bool
}

// ModelSkeleton returns the immutable skeleton metadata imported for a model.
func (b *Bundle) ModelSkeleton(modelID string) ([]ModelJoint, error) {
	value, _, err := b.modelResourceForSpec(SceneNodeSpec{ID: modelID, Components: SceneComponents{Model: &ResourceComponentSpec{Resource: modelID}}})
	if err != nil {
		return nil, err
	}
	result := make([]ModelJoint, len(value.asset.Joints))
	for index, joint := range value.asset.Joints {
		result[index] = ModelJoint{Name: joint.Name, Node: joint.Node, Parent: joint.Parent, InverseBind: joint.InverseBind}
	}
	return result, nil
}

// ModelMorphTargets returns imported morph data without copying its delta
// arrays. The model cache owns the returned slices for the bundle lifetime.
func (b *Bundle) ModelMorphTargets(modelID string) ([]ModelMorphTarget, error) {
	value, _, err := b.modelResourceForSpec(SceneNodeSpec{ID: modelID, Components: SceneComponents{Model: &ResourceComponentSpec{Resource: modelID}}})
	if err != nil {
		return nil, err
	}
	result := make([]ModelMorphTarget, len(value.asset.MorphTargets))
	for index, target := range value.asset.MorphTargets {
		result[index] = ModelMorphTarget{Name: target.Name, PositionDelta: target.PositionDelta, NormalDelta: target.NormalDelta, TangentDelta: target.TangentDelta}
	}
	return result, nil
}

// ModelSkinWeights returns the imported skin influences in mesh vertex order.
func (b *Bundle) ModelSkinWeights(modelID string) ([]ModelSkinVertex, error) {
	value, _, err := b.modelResourceForSpec(SceneNodeSpec{ID: modelID, Components: SceneComponents{Model: &ResourceComponentSpec{Resource: modelID}}})
	if err != nil {
		return nil, err
	}
	result := make([]ModelSkinVertex, len(value.asset.Skin))
	for index, skin := range value.asset.Skin {
		result[index] = ModelSkinVertex{Joints: skin.Joints, Weights: skin.Weights, Active: skin.Active}
	}
	return result, nil
}

// ModelMaterials returns imported PBR descriptors. Texture paths are resolved
// through the same resource cache used by authored model materials.
func (b *Bundle) ModelMaterials(modelID string) ([]graphics.StandardMaterial, error) {
	value, _, err := b.modelResourceForSpec(SceneNodeSpec{ID: modelID, Components: SceneComponents{Model: &ResourceComponentSpec{Resource: modelID}}})
	if err != nil {
		return nil, err
	}
	if len(value.asset.Materials) == 0 {
		return nil, fmt.Errorf("model %q has no imported materials", modelID)
	}
	result := make([]graphics.StandardMaterial, len(value.asset.Materials))
	for index, material := range value.asset.Materials {
		result[index] = graphics.StandardMaterial{BaseColor: graphics.ColorRGBA{R: material.BaseColor[0], G: material.BaseColor[1], B: material.BaseColor[2], A: material.BaseColor[3]}, Emissive: graphics.ColorRGBA{R: material.Emissive[0], G: material.Emissive[1], B: material.Emissive[2], A: 1}, Metallic: material.Metallic, PerceptualRoughness: material.Roughness}
		if err := b.resources.loadMaterialSurface(&result[index], material.Surface, nil); err != nil {
			return nil, err
		}
		if material.BaseColorTexture != "" || len(material.BaseColorTextureData) != 0 {
			result[index].BaseColorTexture, err = b.resources.loadImportedTexture(material.BaseColorTexture, material.BaseColorTextureData, material.BaseColorTextureMime, material.BaseColorSampling)
			if err != nil {
				return nil, err
			}
		}
		if material.NormalTexture != "" || len(material.NormalTextureData) != 0 {
			result[index].NormalTexture, err = b.resources.loadImportedTexture(material.NormalTexture, material.NormalTextureData, material.NormalTextureMime, material.NormalSampling)
			if err != nil {
				return nil, err
			}
		}
		if material.MetallicRoughness != "" || len(material.MetallicRoughnessData) != 0 {
			result[index].MetallicRoughnessTexture, err = b.resources.loadImportedTexture(material.MetallicRoughness, material.MetallicRoughnessData, material.MetallicRoughnessMime, material.MetallicRoughnessSampling)
			if err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}
