package project

import (
	"fmt"
	iofs "io/fs"
	"math"
	pathpkg "path"
	"strings"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
)

type compiledModelResource struct {
	Vertices     []render.Vertex3D
	Indices      []uint32
	Submeshes    []graphics.Submesh
	Nodes        []ModelNode
	Skin         []compiledModelSkinVertex
	Materials    []compiledModelMaterial
	MorphTargets []compiledModelMorphTarget
	Joints       []compiledModelJoint
	Animations   []compiledModelAnimation
}

type compiledModelSkinVertex struct {
	Joints  [4]uint32
	Weights [4]float32
	Active  bool
}

// ModelAnimation returns a compiled ECS clip exported by a glTF/FBX model.
// The clip is immutable and can be shared by multiple Animator components.
func (b *Bundle) ModelAnimation(modelID, animationName string) (*scene.AnimationClip, error) {
	value, _, err := b.modelResourceForSpec(SceneNodeSpec{ID: modelID, Components: SceneComponents{Model: &ResourceComponentSpec{Resource: modelID}}})
	if err != nil {
		return nil, err
	}
	for _, animation := range value.asset.Animations {
		if animation.Name == animationName {
			return compileModelAnimation(animation)
		}
	}
	return nil, fmt.Errorf("model %q animation %q not found", modelID, animationName)
}

func compileModelAnimation(source compiledModelAnimation) (*scene.AnimationClip, error) {
	clip := &scene.AnimationClip{Duration: source.Duration}
	for _, track := range source.Tracks {
		components := int(track.Components)
		if components != 1 && components != 3 && components != 4 {
			return nil, fmt.Errorf("model animation track %q has invalid component count", track.Target)
		}
		componentCount := min(components, 3)
		componentStart := 0
		if components == 1 {
			componentCount = 1
			componentStart = int(track.Component)
			if componentStart >= 3 {
				return nil, fmt.Errorf("model animation track %q has invalid component index", track.Target)
			}
		}
		for component := 0; component < componentCount; component++ {
			valueComponent := component
			if components == 1 {
				valueComponent = 0
			}
			target := scene.AnimationTargetTransformPositionX
			switch track.Target {
			case "transform.translation":
				target = [...]scene.AnimationTarget{scene.AnimationTargetTransformPositionX, scene.AnimationTargetTransformPositionY, scene.AnimationTargetTransformPositionZ}[componentStart+component]
			case "transform.scale":
				target = [...]scene.AnimationTarget{scene.AnimationTargetTransformScaleX, scene.AnimationTargetTransformScaleY, scene.AnimationTargetTransformScaleZ}[componentStart+component]
			case "transform.rotation":
				target = [...]scene.AnimationTarget{scene.AnimationTargetTransformRotationX, scene.AnimationTargetTransformRotationY, scene.AnimationTargetTransformRotationZ}[componentStart+component]
			default:
				return nil, fmt.Errorf("model animation target %q is unsupported", track.Target)
			}
			tween := scene.AnimationLinear
			if track.Interpolation == "STEP" {
				tween = scene.AnimationStep
			} else if track.Interpolation == "CUBICSPLINE" {
				tween = scene.AnimationCubicSpline
			}
			keys := make([]scene.AnimationKeyframe[float32], len(track.Keyframes))
			for index, key := range track.Keyframes {
				value := key.Value[valueComponent]
				inTangent := key.InTangent[valueComponent]
				outTangent := key.OutTangent[valueComponent]
				if track.Target == "transform.rotation" && components == 4 {
					euler := modelQuaternionEuler(key.Value)
					value = euler[component]
					inTangent, outTangent = 0, 0
					if index > 0 {
						value = unwrapModelAngle(keys[index-1].Value, value)
					}
				}
				keys[index] = scene.AnimationKeyframe[float32]{Time: key.Time, Value: value, InTangent: inTangent, OutTangent: outTangent, Tween: tween}
				if track.Target == "transform.rotation" && components == 4 && tween == scene.AnimationCubicSpline {
					keys[index].QuaternionSpline = scene.AnimationQuaternionSpline{Value: key.Value, InTangent: key.InTangent, OutTangent: key.OutTangent, Component: uint8(component + 1)}
				}
			}
			clip.FloatTracks = append(clip.FloatTracks, scene.AnimationTrack[float32]{Binding: scene.AnimationBinding{Path: track.Path, Target: target}, Keyframes: keys})
		}
	}
	if err := clip.Prepare(); err != nil {
		return nil, err
	}
	return clip, nil
}

func modelQuaternionEuler(value [4]float32) [3]float32 {
	x, y, z, w := float64(value[0]), float64(value[1]), float64(value[2]), float64(value[3])
	siny := 2 * (w*y - z*x)
	if siny > 1 {
		siny = 1
	}
	if siny < -1 {
		siny = -1
	}
	return [3]float32{float32(math.Atan2(2*(w*x+y*z), 1-2*(x*x+y*y))), float32(math.Asin(siny)), float32(math.Atan2(2*(w*z+x*y), 1-2*(y*y+z*z)))}
}

func unwrapModelAngle(reference, value float32) float32 {
	delta := value - reference
	for delta > math.Pi {
		value -= 2 * math.Pi
		delta -= 2 * math.Pi
	}
	for delta < -math.Pi {
		value += 2 * math.Pi
		delta += 2 * math.Pi
	}
	return value
}

type compiledModelMaterial struct {
	Surface                   compiledMaterialSurface
	BaseColorSampling         graphics.TextureOptions
	NormalSampling            graphics.TextureOptions
	MetallicRoughnessSampling graphics.TextureOptions
	Name                      string
	BaseColor                 [4]float32
	Emissive                  [3]float32
	Metallic                  float32
	Roughness                 float32
	BaseColorTexture          string
	BaseColorTextureData      []byte
	BaseColorTextureMime      string
	NormalTexture             string
	NormalTextureData         []byte
	NormalTextureMime         string
	MetallicRoughness         string
	MetallicRoughnessData     []byte
	MetallicRoughnessMime     string
}

type compiledMaterialTexture struct {
	Path     string
	Data     []byte
	Mime     string
	Sampling graphics.TextureOptions
}

type compiledMaterialSurface struct {
	IOR                  float32
	EmissiveStrength     float32
	NormalScale          float32
	OcclusionStrength    float32
	SpecularFactor       float32
	SpecularColor        [3]float32
	ClearcoatFactor      float32
	ClearcoatRoughness   float32
	ClearcoatNormalScale float32
	SheenColor           [3]float32
	SheenRoughness       float32
	AnisotropyStrength   float32
	AnisotropyRotation   float32
	Textures             [render.MeshSurfaceTextureSlots]compiledMaterialTexture
	AlphaMode            graphics.AlphaMode
	AlphaCutoff          float32
	Unlit                bool
	DoubleSided          bool
}

type compiledModelMorphTarget struct {
	Name          string
	PositionDelta [][3]float32
	NormalDelta   [][3]float32
	TangentDelta  [][3]float32
}

type compiledModelJoint struct {
	Name        string
	Node        int32
	Parent      int32
	InverseBind [16]float32
}

type compiledModelAnimation struct {
	Name     string
	Duration float32
	Tracks   []compiledModelAnimationTrack
}

type compiledModelAnimationTrack struct {
	Path          string
	Target        string
	Components    uint8
	Component     uint8
	Interpolation string
	Keyframes     []compiledModelAnimationKeyframe
}

type compiledModelAnimationKeyframe struct {
	Time       float32
	Value      [4]float32
	InTangent  [4]float32
	OutTangent [4]float32
}

func (resource compiledModelResource) validate() error {
	if len(resource.Vertices) == 0 {
		return fmt.Errorf("model has no vertices")
	}
	if len(resource.Indices) == 0 || len(resource.Indices)%3 != 0 {
		return fmt.Errorf("model index count must be a non-zero multiple of three")
	}
	var indexEnd uint64
	for index, part := range resource.Submeshes {
		if uint64(part.FirstIndex) != indexEnd || part.IndexCount == 0 || part.IndexCount%3 != 0 {
			return fmt.Errorf("submesh %d has an invalid index range", index)
		}
		indexEnd += uint64(part.IndexCount)
		if indexEnd > uint64(len(resource.Indices)) || uint64(part.Material) > uint64(len(resource.Materials)) || part.Node < -1 || int(part.Node) >= len(resource.Nodes) {
			return fmt.Errorf("submesh %d references invalid geometry, material or node", index)
		}
	}
	if len(resource.Submeshes) != 0 && indexEnd != uint64(len(resource.Indices)) {
		return fmt.Errorf("submeshes do not cover the model indices")
	}
	var inlineVisited [128]uint8
	visited := inlineVisited[:]
	if len(resource.Nodes) > len(visited) {
		visited = make([]uint8, len(resource.Nodes))
	}
	for index, node := range resource.Nodes {
		if node.Parent < -1 || int(node.Parent) >= len(resource.Nodes) {
			return fmt.Errorf("model node %d parent is out of range", index)
		}
		current := int32(index)
		for current >= 0 {
			if int(current) >= len(resource.Nodes) || visited[current] == 1 {
				return fmt.Errorf("model node %d hierarchy is invalid", index)
			}
			if visited[current] == 2 {
				break
			}
			visited[current] = 1
			current = resource.Nodes[current].Parent
		}
		for current = int32(index); current >= 0 && visited[current] == 1; current = resource.Nodes[current].Parent {
			visited[current] = 2
		}
		for component := range node.Local {
			if !finiteFloat32(node.Local[component]) || !finiteFloat32(node.World[component]) {
				return fmt.Errorf("model node %d transform is not finite", index)
			}
		}
	}
	for index, vertex := range resource.Vertices {
		for component, value := range vertex.Position {
			if !finiteFloat32(value) {
				return fmt.Errorf("vertex %d position component %d is not finite", index, component)
			}
		}
		for component, value := range vertex.Normal {
			if !finiteFloat32(value) {
				return fmt.Errorf("vertex %d normal component %d is not finite", index, component)
			}
		}
		for component, value := range vertex.Tangent {
			if !finiteFloat32(value) {
				return fmt.Errorf("vertex %d tangent component %d is not finite", index, component)
			}
		}
		if w := vertex.Tangent[3]; w != 0 && w != -1 && w != 1 {
			return fmt.Errorf("vertex %d has invalid tangent handedness", index)
		}
		for component, value := range vertex.UV {
			if !finiteFloat32(value) {
				return fmt.Errorf("vertex %d UV component %d is not finite", index, component)
			}
		}
	}
	if len(resource.Skin) != 0 && len(resource.Skin) != len(resource.Vertices) {
		return fmt.Errorf("skin vertex count %d does not match vertex count %d", len(resource.Skin), len(resource.Vertices))
	}
	for vertex, skin := range resource.Skin {
		if !skin.Active {
			continue
		}
		total := float32(0)
		for influence, weight := range skin.Weights {
			if !finiteFloat32(weight) || weight < 0 || weight > 1 {
				return fmt.Errorf("skin vertex %d influence %d has invalid weight", vertex, influence)
			}
			if weight > 0 && uint64(skin.Joints[influence]) >= uint64(len(resource.Joints)) {
				return fmt.Errorf("skin vertex %d influence %d joint is out of range", vertex, influence)
			}
			total += weight
		}
		if math.Abs(float64(total-1)) > 0.01 {
			return fmt.Errorf("skin vertex %d weights sum to %g", vertex, total)
		}
	}
	for index, joint := range resource.Joints {
		if len(resource.Nodes) != 0 && (joint.Node < 0 || int(joint.Node) >= len(resource.Nodes)) {
			return fmt.Errorf("joint %d node is out of range", index)
		}
	}
	for materialIndex, material := range resource.Materials {
		if err := material.Surface.validate(); err != nil {
			return fmt.Errorf("material %d: %w", materialIndex, err)
		}
		for component, value := range material.BaseColor {
			if !finiteFloat32(value) {
				return fmt.Errorf("material %d base color component %d is not finite", materialIndex, component)
			}
		}
		for component, value := range material.Emissive {
			if !finiteFloat32(value) {
				return fmt.Errorf("material %d emissive component %d is not finite", materialIndex, component)
			}
		}
		if !finiteFloat32(material.Metallic) || !finiteFloat32(material.Roughness) || material.Metallic < 0 || material.Metallic > 1 || material.Roughness < 0 || material.Roughness > 1 {
			return fmt.Errorf("material %d has invalid metallic/roughness", materialIndex)
		}
	}
	for targetIndex, target := range resource.MorphTargets {
		if len(target.PositionDelta) != len(resource.Vertices) || (len(target.NormalDelta) != 0 && len(target.NormalDelta) != len(resource.Vertices)) || (len(target.TangentDelta) != 0 && len(target.TangentDelta) != len(resource.Vertices)) {
			return fmt.Errorf("morph target %d delta counts do not match vertex count", targetIndex)
		}
		for deltaKind, values := range map[string][][3]float32{"position": target.PositionDelta, "normal": target.NormalDelta, "tangent": target.TangentDelta} {
			for vertex, value := range values {
				for component, item := range value {
					if !finiteFloat32(item) {
						return fmt.Errorf("morph target %d %s delta vertex %d component %d is not finite", targetIndex, deltaKind, vertex, component)
					}
				}
			}
		}
	}
	for animationIndex, animation := range resource.Animations {
		for trackIndex, track := range animation.Tracks {
			if track.Components == 0 || track.Components > 4 {
				return fmt.Errorf("animation %d track %d has invalid component count", animationIndex, trackIndex)
			}
			if track.Interpolation != "LINEAR" && track.Interpolation != "STEP" && track.Interpolation != "CUBICSPLINE" {
				return fmt.Errorf("animation %d track %d has invalid interpolation %q", animationIndex, trackIndex, track.Interpolation)
			}
			for keyIndex, key := range track.Keyframes {
				if !finiteFloat32(key.Time) || keyIndex > 0 && key.Time < track.Keyframes[keyIndex-1].Time {
					return fmt.Errorf("animation %d track %d keyframe times are invalid", animationIndex, trackIndex)
				}
				for component := range int(track.Components) {
					if !finiteFloat32(key.Value[component]) || !finiteFloat32(key.InTangent[component]) || !finiteFloat32(key.OutTangent[component]) {
						return fmt.Errorf("animation %d track %d keyframe %d component %d is not finite", animationIndex, trackIndex, keyIndex, component)
					}
				}
			}
		}
	}
	for offset, index := range resource.Indices {
		if uint64(index) >= uint64(len(resource.Vertices)) {
			return fmt.Errorf("index %d at offset %d exceeds %d vertices", index, offset, len(resource.Vertices))
		}
	}
	return nil
}

func finiteFloat32(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

func (r *Resources) loadModelResource(value string) (*graphics.Mesh, error) {
	_, mesh, err := r.loadModelAsset(value)
	return mesh, err
}

func (r *Resources) loadImportedTexture(path string, data []byte, mime string, options graphics.TextureOptions) (*graphics.Texture, error) {
	options.Usage = graphics.ResourceUsageGPU
	if len(data) == 0 {
		if path == "" {
			return nil, nil
		}
		if _, err := NormalizeResourcePath(path); err != nil {
			return nil, fmt.Errorf("resolve imported image %q: %w", path, err)
		}
		texture, _, err := r.textureFromPath(path, options)
		if err != nil {
			return nil, fmt.Errorf("load imported image %q: %w", path, err)
		}
		return texture, nil
	}
	if r == nil || r.imageDecoder == nil {
		return nil, fmt.Errorf("embedded model texture requires an injected image decoder")
	}
	extension := ".bin"
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "image/png":
		extension = ".png"
	case "image/jpeg", "image/jpg":
		extension = ".jpg"
	case "image/webp":
		extension = ".webp"
	case "image/gif":
		extension = ".gif"
	case "image/svg+xml":
		extension = ".svg"
	}
	return r.imageDecoder.DecodeImage(data, "model-embedded"+extension, options)
}

func (r *Resources) loadModelAsset(value string) (compiledModelResource, *graphics.Mesh, error) {
	if r == nil || r.source == nil {
		return compiledModelResource{}, nil, fmt.Errorf("project resources are nil")
	}
	normalized, err := NormalizeResourcePath(value)
	if err != nil {
		return compiledModelResource{}, nil, err
	}
	resolved, err := ResolveResourcePath(r.baseDir, normalized)
	if err != nil {
		return compiledModelResource{}, nil, err
	}
	blob, err := openProjectBlob(r.source, resolved)
	if err != nil {
		return compiledModelResource{}, nil, fmt.Errorf("read model resource %q: %w", value, err)
	}
	defer blob.Close()
	data, err := blob.Bytes()
	if err != nil {
		return compiledModelResource{}, nil, fmt.Errorf("read model resource %q: %w", value, err)
	}
	resource, err := decodeModelResource(r.source, resolved, data)
	if err != nil {
		return compiledModelResource{}, nil, fmt.Errorf("model resource %q: %w", value, err)
	}
	if err := resource.validate(); err != nil {
		return compiledModelResource{}, nil, fmt.Errorf("model resource %q: %w", value, err)
	}
	mesh, err := graphics.NewMeshOwned(resource.Vertices, resource.Indices)
	if err != nil {
		return compiledModelResource{}, nil, err
	}
	mesh.Submeshes = resource.Submeshes
	mesh.Skin = make([]graphics.SkinVertex, len(resource.Skin))
	for index, skin := range resource.Skin {
		mesh.Skin[index] = graphics.SkinVertex{Joints: skin.Joints, Weights: skin.Weights, Active: skin.Active}
	}
	mesh.MorphTargets = make([]graphics.MorphTarget, len(resource.MorphTargets))
	for index, target := range resource.MorphTargets {
		mesh.MorphTargets[index] = graphics.MorphTarget{Name: target.Name, PositionDelta: target.PositionDelta, NormalDelta: target.NormalDelta, TangentDelta: target.TangentDelta}
	}
	return resource, mesh, nil
}

type modelBufferSource struct {
	source iofs.FS
	path   string
	data   []byte
}

func (source modelBufferSource) readRelative(uri string) ([]byte, error) {
	resolved, err := ResolveResourcePath(pathpkg.Dir(source.path), uri)
	if err != nil {
		return nil, err
	}
	return readProjectFile(source.source, resolved)
}

func modelPathExtension(path string) string {
	return strings.ToLower(pathpkg.Ext(path))
}
