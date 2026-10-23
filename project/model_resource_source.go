//go:build !onix_release

package project

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	iofs "io/fs"
	"math"
	"net/url"
	pathpkg "path"
	"strings"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/typedbin"
	"github.com/diiyw/onix/render"
	"github.com/go-gl/mathgl/mgl32"
)

const (
	gltfComponentUnsignedByte  = 5121
	gltfComponentUnsignedShort = 5123
	gltfComponentUnsignedInt   = 5125
	gltfComponentFloat         = 5126
	gltfPrimitiveTriangles     = 4
)

type gltfDocument struct {
	Samplers []gltfSampler `json:"samplers"`
	Asset    struct {
		Version string `json:"version"`
	} `json:"asset"`
	ExtensionsRequired []string         `json:"extensionsRequired"`
	Scene              *int             `json:"scene"`
	Scenes             []gltfScene      `json:"scenes"`
	Nodes              []gltfNode       `json:"nodes"`
	Meshes             []gltfMesh       `json:"meshes"`
	Materials          []gltfMaterial   `json:"materials"`
	Textures           []gltfTexture    `json:"textures"`
	Images             []gltfImage      `json:"images"`
	Animations         []gltfAnimation  `json:"animations"`
	Skins              []gltfSkin       `json:"skins"`
	Accessors          []gltfAccessor   `json:"accessors"`
	BufferViews        []gltfBufferView `json:"bufferViews"`
	Buffers            []gltfBuffer     `json:"buffers"`
}

type gltfScene struct {
	Nodes []int `json:"nodes"`
}

type gltfNode struct {
	Name        string    `json:"name"`
	Mesh        *int      `json:"mesh"`
	Skin        *int      `json:"skin"`
	Children    []int     `json:"children"`
	Matrix      []float32 `json:"matrix"`
	Translation []float32 `json:"translation"`
	Rotation    []float32 `json:"rotation"`
	Scale       []float32 `json:"scale"`
}

type gltfMesh struct {
	Primitives []gltfPrimitive `json:"primitives"`
}

type gltfPrimitive struct {
	Attributes map[string]int   `json:"attributes"`
	Indices    *int             `json:"indices"`
	Mode       *uint32          `json:"mode"`
	Targets    []map[string]int `json:"targets"`
	Material   *int             `json:"material"`
}

type gltfMaterial struct {
	AlphaMode        string          `json:"alphaMode"`
	AlphaCutoff      *float32        `json:"alphaCutoff"`
	DoubleSided      bool            `json:"doubleSided"`
	OcclusionTexture *gltfTextureRef `json:"occlusionTexture"`
	EmissiveTexture  *gltfTextureRef `json:"emissiveTexture"`
	Extensions       struct {
		Unlit *struct{} `json:"KHR_materials_unlit"`
		IOR   *struct {
			IOR *float32 `json:"ior"`
		} `json:"KHR_materials_ior"`
		EmissiveStrength *struct {
			Strength *float32 `json:"emissiveStrength"`
		} `json:"KHR_materials_emissive_strength"`
		Specular *struct {
			Factor       *float32        `json:"specularFactor"`
			Color        *[]float32      `json:"specularColorFactor"`
			Texture      *gltfTextureRef `json:"specularTexture"`
			ColorTexture *gltfTextureRef `json:"specularColorTexture"`
		} `json:"KHR_materials_specular"`
		Clearcoat *struct {
			Factor           *float32        `json:"clearcoatFactor"`
			Roughness        *float32        `json:"clearcoatRoughnessFactor"`
			Texture          *gltfTextureRef `json:"clearcoatTexture"`
			RoughnessTexture *gltfTextureRef `json:"clearcoatRoughnessTexture"`
			NormalTexture    *gltfTextureRef `json:"clearcoatNormalTexture"`
		} `json:"KHR_materials_clearcoat"`
		Sheen *struct {
			Color            *[]float32      `json:"sheenColorFactor"`
			Roughness        *float32        `json:"sheenRoughnessFactor"`
			ColorTexture     *gltfTextureRef `json:"sheenColorTexture"`
			RoughnessTexture *gltfTextureRef `json:"sheenRoughnessTexture"`
		} `json:"KHR_materials_sheen"`
		Anisotropy *struct {
			Strength *float32        `json:"anisotropyStrength"`
			Rotation *float32        `json:"anisotropyRotation"`
			Texture  *gltfTextureRef `json:"anisotropyTexture"`
		} `json:"KHR_materials_anisotropy"`
	} `json:"extensions"`
	Name string `json:"name"`
	PBR  struct {
		BaseColorFactor          *[]float32      `json:"baseColorFactor"`
		BaseColorTexture         *gltfTextureRef `json:"baseColorTexture"`
		MetallicFactor           *float32        `json:"metallicFactor"`
		RoughnessFactor          *float32        `json:"roughnessFactor"`
		MetallicRoughnessTexture *gltfTextureRef `json:"metallicRoughnessTexture"`
	} `json:"pbrMetallicRoughness"`
	EmissiveFactor [3]float32      `json:"emissiveFactor"`
	NormalTexture  *gltfTextureRef `json:"normalTexture"`
}

type gltfTextureRef struct {
	Index    int      `json:"index"`
	TexCoord int      `json:"texCoord"`
	Scale    *float32 `json:"scale"`
	Strength *float32 `json:"strength"`
}
type gltfTexture struct {
	Sampler *int `json:"sampler"`
	Source  *int `json:"source"`
}
type gltfSampler struct {
	WrapS     uint32 `json:"wrapS"`
	WrapT     uint32 `json:"wrapT"`
	MinFilter uint32 `json:"minFilter"`
	MagFilter uint32 `json:"magFilter"`
}

func (importer *gltfImporter) textureSampling(index int) (graphics.TextureOptions, error) {
	result := graphics.TextureOptions{WrapU: render.TextureAddressRepeat, WrapV: render.TextureAddressRepeat, Mipmap: true}
	if index < 0 || index >= len(importer.document.Textures) {
		return result, fmt.Errorf("texture %d is out of range", index)
	}
	sampler := importer.document.Textures[index].Sampler
	if sampler == nil {
		return result, nil
	}
	if *sampler < 0 || *sampler >= len(importer.document.Samplers) {
		return result, fmt.Errorf("texture %d sampler is out of range", index)
	}
	source := importer.document.Samplers[*sampler]
	convert := func(value uint32) (render.TextureAddressMode, error) {
		switch value {
		case 0, 10497:
			return render.TextureAddressRepeat, nil
		case 33071:
			return render.TextureAddressClamp, nil
		case 33648:
			return render.TextureAddressMirror, nil
		}
		return 0, fmt.Errorf("unsupported glTF texture wrap %d", value)
	}
	var err error
	result.WrapU, err = convert(source.WrapS)
	if err != nil {
		return result, err
	}
	result.WrapV, err = convert(source.WrapT)
	if err != nil {
		return result, err
	}
	result.Mipmap = source.MinFilter == 0 || source.MinFilter >= 9984 && source.MinFilter <= 9987
	return result, nil
}

type gltfImage struct {
	URI        string `json:"uri"`
	BufferView *int   `json:"bufferView"`
	MimeType   string `json:"mimeType"`
}

type gltfAnimation struct {
	Name     string                 `json:"name"`
	Samplers []gltfAnimationSampler `json:"samplers"`
	Channels []gltfAnimationChannel `json:"channels"`
}
type gltfAnimationSampler struct {
	Input         int    `json:"input"`
	Output        int    `json:"output"`
	Interpolation string `json:"interpolation"`
}
type gltfAnimationChannel struct {
	Sampler int                 `json:"sampler"`
	Target  gltfAnimationTarget `json:"target"`
}
type gltfAnimationTarget struct {
	Node *int   `json:"node"`
	Path string `json:"path"`
}
type gltfSkin struct {
	Name                string `json:"name"`
	Joints              []int  `json:"joints"`
	InverseBindMatrices *int   `json:"inverseBindMatrices"`
}

type gltfAccessor struct {
	BufferView    *int            `json:"bufferView"`
	ByteOffset    uint64          `json:"byteOffset"`
	ComponentType uint32          `json:"componentType"`
	Normalized    bool            `json:"normalized"`
	Count         uint64          `json:"count"`
	Type          string          `json:"type"`
	Sparse        json.RawMessage `json:"sparse"`
}

type gltfBufferView struct {
	Buffer     int    `json:"buffer"`
	ByteOffset uint64 `json:"byteOffset"`
	ByteLength uint64 `json:"byteLength"`
	ByteStride uint64 `json:"byteStride"`
}

type gltfBuffer struct {
	URI        string `json:"uri"`
	ByteLength uint64 `json:"byteLength"`
}

type gltfImporter struct {
	document         gltfDocument
	buffers          [][]byte
	sourcePath       string
	result           compiledModelResource
	skinJointOffsets []int
}

type gltfAccessorView struct {
	data          []byte
	count         int
	stride        int
	componentSize int
	componentType uint32
	normalized    bool
}

func decodeModelResource(source iofs.FS, path string, data []byte) (compiledModelResource, error) {
	if typedbin.IsEncoded(data) {
		return typedbin.Unmarshal[compiledModelResource](data)
	}
	switch modelPathExtension(path) {
	case ".gltf", ".glb":
		return importGLTF(modelBufferSource{source: source, path: path, data: data})
	case ".fbx":
		return importFBX(data)
	default:
		return compiledModelResource{}, fmt.Errorf("unsupported model format %q", modelPathExtension(path))
	}
}

func importGLTF(source modelBufferSource) (compiledModelResource, error) {
	jsonData, binaryChunk, err := splitGLTF(source.path, source.data)
	if err != nil {
		return compiledModelResource{}, err
	}
	var document gltfDocument
	if err := json.Unmarshal(jsonData, &document); err != nil {
		return compiledModelResource{}, fmt.Errorf("decode glTF JSON: %w", err)
	}
	if document.Asset.Version != "2.0" {
		return compiledModelResource{}, fmt.Errorf("glTF asset.version must be 2.0")
	}
	for _, extension := range document.ExtensionsRequired {
		switch extension {
		case "KHR_materials_unlit", "KHR_materials_specular", "KHR_materials_clearcoat", "KHR_materials_sheen", "KHR_materials_anisotropy", "KHR_materials_ior", "KHR_materials_emissive_strength":
		default:
			return compiledModelResource{}, fmt.Errorf("glTF required extension is unsupported: %s", extension)
		}
	}
	buffers, err := loadGLTFBuffers(source, document.Buffers, binaryChunk)
	if err != nil {
		return compiledModelResource{}, err
	}
	importer := gltfImporter{document: document, buffers: buffers, sourcePath: source.path}
	if err := importer.appendNodes(); err != nil {
		return compiledModelResource{}, err
	}
	if err := importer.appendMaterials(); err != nil {
		return compiledModelResource{}, err
	}
	if err := importer.appendSkins(); err != nil {
		return compiledModelResource{}, err
	}
	if err := importer.appendScene(); err != nil {
		return compiledModelResource{}, err
	}
	if err := importer.appendAnimations(); err != nil {
		return compiledModelResource{}, err
	}
	if err := importer.result.validate(); err != nil {
		return compiledModelResource{}, err
	}
	return importer.result, nil
}

func (importer *gltfImporter) appendMaterials() error {
	if len(importer.document.Materials) == 0 {
		return nil
	}
	importer.result.Materials = make([]compiledModelMaterial, len(importer.document.Materials))
	for index, material := range importer.document.Materials {
		value := compiledModelMaterial{Name: material.Name, BaseColor: [4]float32{1, 1, 1, 1}, Metallic: 1, Roughness: 1}
		if err := importer.appendMaterialSurface(&value, material); err != nil {
			return fmt.Errorf("glTF material %d: %w", index, err)
		}
		if material.PBR.BaseColorFactor != nil {
			if len(*material.PBR.BaseColorFactor) != 4 {
				return fmt.Errorf("glTF material %d base color factor must contain four values", index)
			}
			copy(value.BaseColor[:], *material.PBR.BaseColorFactor)
		}
		value.Emissive = material.EmissiveFactor
		if material.PBR.MetallicFactor != nil {
			value.Metallic = *material.PBR.MetallicFactor
		}
		if material.PBR.RoughnessFactor != nil {
			value.Roughness = *material.PBR.RoughnessFactor
		}
		if material.PBR.BaseColorTexture != nil {
			sampling, err := importer.textureSampling(material.PBR.BaseColorTexture.Index)
			if err != nil {
				return err
			}
			value.BaseColorSampling = sampling
			value.BaseColorSampling.SRGB = true

			path, data, mime, err := importer.textureReference(material.PBR.BaseColorTexture.Index)
			if err != nil {
				return fmt.Errorf("glTF material %d base color texture: %w", index, err)
			}
			value.BaseColorTexture, value.BaseColorTextureData, value.BaseColorTextureMime = path, data, mime
		}
		if material.NormalTexture != nil {
			sampling, err := importer.textureSampling(material.NormalTexture.Index)
			if err != nil {
				return err
			}
			value.NormalSampling = sampling

			path, data, mime, err := importer.textureReference(material.NormalTexture.Index)
			if err != nil {
				return fmt.Errorf("glTF material %d normal texture: %w", index, err)
			}
			value.NormalTexture, value.NormalTextureData, value.NormalTextureMime = path, data, mime
		}
		if material.PBR.MetallicRoughnessTexture != nil {
			sampling, err := importer.textureSampling(material.PBR.MetallicRoughnessTexture.Index)
			if err != nil {
				return err
			}
			value.MetallicRoughnessSampling = sampling

			path, data, mime, err := importer.textureReference(material.PBR.MetallicRoughnessTexture.Index)
			if err != nil {
				return fmt.Errorf("glTF material %d metallic roughness texture: %w", index, err)
			}
			value.MetallicRoughness, value.MetallicRoughnessData, value.MetallicRoughnessMime = path, data, mime
		}
		for _, component := range value.BaseColor {
			if !finiteFloat32(component) {
				return fmt.Errorf("glTF material %d base color is not finite", index)
			}
		}
		for _, component := range value.Emissive {
			if !finiteFloat32(component) {
				return fmt.Errorf("glTF material %d emissive is not finite", index)
			}
		}
		if !finiteFloat32(value.Metallic) || !finiteFloat32(value.Roughness) || value.Metallic < 0 || value.Metallic > 1 || value.Roughness < 0 || value.Roughness > 1 {
			return fmt.Errorf("glTF material %d metallic/roughness is invalid", index)
		}
		importer.result.Materials[index] = value
	}
	return nil
}

func (importer *gltfImporter) textureURI(index int) string {
	path, _, _, _ := importer.textureReference(index)
	return path
}

func (importer *gltfImporter) textureReference(index int) (string, []byte, string, error) {
	if index < 0 || index >= len(importer.document.Textures) {
		return "", nil, "", fmt.Errorf("texture %d is out of range", index)
	}
	texture := importer.document.Textures[index]
	if texture.Source == nil || *texture.Source < 0 || *texture.Source >= len(importer.document.Images) {
		return "", nil, "", fmt.Errorf("texture %d source is invalid", index)
	}
	image := importer.document.Images[*texture.Source]
	if image.URI != "" {
		if strings.HasPrefix(image.URI, "data:") {
			comma := strings.IndexByte(image.URI, ',')
			if comma < 0 {
				return "", nil, "", fmt.Errorf("image %d data URI has no payload", *texture.Source)
			}
			header, payload := image.URI[:comma], image.URI[comma+1:]
			mime := strings.TrimPrefix(strings.SplitN(header, ";", 2)[0], "data:")
			if strings.Contains(header, ";base64") {
				data, err := base64.StdEncoding.DecodeString(payload)
				if err != nil {
					return "", nil, "", fmt.Errorf("decode image %d data URI: %w", *texture.Source, err)
				}
				return "", data, mime, nil
			}
			decoded, err := url.PathUnescape(payload)
			if err != nil {
				return "", nil, "", fmt.Errorf("decode image %d data URI: %w", *texture.Source, err)
			}
			return "", []byte(decoded), mime, nil
		}
		resolved, err := ResolveResourcePath(".", pathpkg.Join(pathpkg.Dir(importer.sourcePath), image.URI))
		if err != nil {
			return "", nil, "", err
		}
		return resolved, nil, image.MimeType, nil
	}
	if image.BufferView == nil || *image.BufferView < 0 || *image.BufferView >= len(importer.document.BufferViews) {
		return "", nil, "", fmt.Errorf("image %d has no URI or valid bufferView", *texture.Source)
	}
	view := importer.document.BufferViews[*image.BufferView]
	if view.Buffer < 0 || view.Buffer >= len(importer.buffers) || view.ByteOffset > uint64(len(importer.buffers[view.Buffer])) || view.ByteLength > uint64(len(importer.buffers[view.Buffer]))-view.ByteOffset {
		return "", nil, "", fmt.Errorf("image %d bufferView exceeds buffer", *texture.Source)
	}
	start := int(view.ByteOffset)
	end := start + int(view.ByteLength)
	return "", append([]byte(nil), importer.buffers[view.Buffer][start:end]...), image.MimeType, nil
}

func splitGLTF(path string, data []byte) ([]byte, []byte, error) {
	if modelPathExtension(path) == ".gltf" {
		return data, nil, nil
	}
	if len(data) < 12 || string(data[:4]) != "glTF" {
		return nil, nil, fmt.Errorf("invalid GLB header")
	}
	if binary.LittleEndian.Uint32(data[4:8]) != 2 {
		return nil, nil, fmt.Errorf("GLB version must be 2")
	}
	if declared := binary.LittleEndian.Uint32(data[8:12]); uint64(declared) != uint64(len(data)) {
		return nil, nil, fmt.Errorf("GLB length %d does not match %d bytes", declared, len(data))
	}
	var jsonData, binaryChunk []byte
	for offset := 12; offset < len(data); {
		if len(data)-offset < 8 {
			return nil, nil, fmt.Errorf("truncated GLB chunk header")
		}
		length := int(binary.LittleEndian.Uint32(data[offset : offset+4]))
		chunkType := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		offset += 8
		if length < 0 || length > len(data)-offset {
			return nil, nil, fmt.Errorf("GLB chunk exceeds file length")
		}
		chunk := data[offset : offset+length]
		offset += length
		switch chunkType {
		case 0x4e4f534a:
			if jsonData != nil || binaryChunk != nil {
				return nil, nil, fmt.Errorf("GLB JSON chunk must be first and unique")
			}
			jsonData = chunk
		case 0x004e4942:
			if jsonData == nil || binaryChunk != nil {
				return nil, nil, fmt.Errorf("GLB BIN chunk must follow JSON and be unique")
			}
			binaryChunk = chunk
		}
	}
	if jsonData == nil {
		return nil, nil, fmt.Errorf("GLB has no JSON chunk")
	}
	return jsonData, binaryChunk, nil
}

func loadGLTFBuffers(source modelBufferSource, specs []gltfBuffer, binaryChunk []byte) ([][]byte, error) {
	buffers := make([][]byte, len(specs))
	usedBinaryChunk := false
	for index, spec := range specs {
		var data []byte
		var err error
		switch {
		case spec.URI != "":
			data, err = loadGLTFBufferURI(source, spec.URI)
		case binaryChunk != nil && !usedBinaryChunk:
			data, usedBinaryChunk = binaryChunk, true
		default:
			err = fmt.Errorf("buffer has no URI or GLB BIN chunk")
		}
		if err != nil {
			return nil, fmt.Errorf("glTF buffer %d: %w", index, err)
		}
		if spec.ByteLength > uint64(len(data)) {
			return nil, fmt.Errorf("glTF buffer %d declares %d bytes but has %d", index, spec.ByteLength, len(data))
		}
		buffers[index] = data[:int(spec.ByteLength)]
	}
	if binaryChunk != nil && !usedBinaryChunk {
		return nil, fmt.Errorf("GLB BIN chunk is not referenced by a buffer")
	}
	return buffers, nil
}

func loadGLTFBufferURI(source modelBufferSource, uri string) ([]byte, error) {
	if strings.HasPrefix(uri, "data:") {
		comma := strings.IndexByte(uri, ',')
		if comma < 0 || uri[:comma] != "data:application/octet-stream;base64" && uri[:comma] != "data:application/gltf-buffer;base64" {
			return nil, fmt.Errorf("buffer data URI must use base64")
		}
		data, err := base64.StdEncoding.DecodeString(uri[comma+1:])
		if err != nil {
			return nil, fmt.Errorf("decode buffer data URI: %w", err)
		}
		return data, nil
	}
	parsed, err := url.Parse(uri)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || strings.HasPrefix(uri, "//") {
		return nil, fmt.Errorf("buffer URI %q must reference a relative file", uri)
	}
	return source.readRelative(parsed.Path)
}

func (importer *gltfImporter) appendNodes() error {
	if _, err := validateGLTFNodeGraph(importer.document.Nodes); err != nil {
		return err
	}
	nodes := make([]ModelNode, len(importer.document.Nodes))
	names := make(map[string]int, len(nodes))
	for _, node := range importer.document.Nodes {
		names[node.Name]++
	}
	for index, node := range importer.document.Nodes {
		local, err := gltfNodeMatrix(node)
		if err != nil {
			return fmt.Errorf("glTF node %d: %w", index, err)
		}
		// Include the node index in every binding when names are ambiguous.
		path := node.Name
		if path == "" || names[path] > 1 || strings.HasPrefix(path, "node-") {
			path = fmt.Sprintf("node-%d", index)
		}
		nodes[index] = ModelNode{Name: node.Name, Path: path, Parent: -1, Local: [16]float32(local)}
	}
	for index, node := range importer.document.Nodes {
		for _, child := range node.Children {
			nodes[child].Parent = int32(index)
		}
	}
	resolved := make([]bool, len(nodes))
	var resolve func(int) mgl32.Mat4
	resolve = func(index int) mgl32.Mat4 {
		if resolved[index] {
			return mgl32.Mat4(nodes[index].World)
		}
		matrix := mgl32.Mat4(nodes[index].Local)
		if parent := nodes[index].Parent; parent >= 0 {
			matrix = resolve(int(parent)).Mul4(matrix)
		}
		nodes[index].World, resolved[index] = [16]float32(matrix), true
		return matrix
	}
	for index := range nodes {
		matrix := resolve(index)
		if determinant := matrix.Det(); !finiteFloat32(determinant) || math.Abs(float64(determinant)) < 1e-12 {
			return fmt.Errorf("glTF node %d transform is singular", index)
		}
	}
	importer.result.Nodes = nodes
	return nil
}

func (importer *gltfImporter) appendScene() error {
	if len(importer.document.Nodes) == 0 {
		if importer.document.Scene != nil && len(importer.document.Scenes) == 0 {
			return fmt.Errorf("glTF default scene is set but scenes are empty")
		}
		if len(importer.document.Scenes) != 0 {
			scene := 0
			if importer.document.Scene != nil {
				scene = *importer.document.Scene
			}
			if scene < 0 || scene >= len(importer.document.Scenes) {
				return fmt.Errorf("glTF scene %d is out of range", scene)
			}
			if len(importer.document.Scenes[scene].Nodes) != 0 {
				return fmt.Errorf("glTF scene references nodes but nodes are empty")
			}
			return nil
		}
		for mesh := range importer.document.Meshes {
			if err := importer.appendMesh(mesh, mgl32.Ident4(), -1); err != nil {
				return err
			}
		}
		return nil
	}
	parents, err := validateGLTFNodeGraph(importer.document.Nodes)
	if err != nil {
		return err
	}
	var roots []int
	if len(importer.document.Scenes) != 0 {
		scene := 0
		if importer.document.Scene != nil {
			scene = *importer.document.Scene
		}
		if scene < 0 || scene >= len(importer.document.Scenes) {
			return fmt.Errorf("glTF scene %d is out of range", scene)
		}
		roots = importer.document.Scenes[scene].Nodes
	} else {
		roots = make([]int, 0, len(parents))
		for node, parentCount := range parents {
			if parentCount == 0 {
				roots = append(roots, node)
			}
		}
	}
	state := make([]uint8, len(importer.document.Nodes))
	for _, root := range roots {
		if root < 0 || root >= len(importer.document.Nodes) {
			return fmt.Errorf("glTF scene node %d is out of range", root)
		}
		if err := importer.appendNode(root, mgl32.Ident4(), state); err != nil {
			return err
		}
	}
	return nil
}

func validateGLTFNodeGraph(nodes []gltfNode) ([]int, error) {
	parents := make([]int, len(nodes))
	for nodeIndex, node := range nodes {
		for _, child := range node.Children {
			if child < 0 || child >= len(nodes) {
				return nil, fmt.Errorf("glTF node %d child %d is out of range", nodeIndex, child)
			}
			parents[child]++
			if parents[child] > 1 {
				return nil, fmt.Errorf("glTF node %d has multiple parents", child)
			}
		}
	}
	state := make([]uint8, len(nodes))
	var visit func(int) error
	visit = func(index int) error {
		switch state[index] {
		case 1:
			return fmt.Errorf("glTF node graph contains a cycle at node %d", index)
		case 2:
			return nil
		}
		state[index] = 1
		for _, child := range nodes[index].Children {
			if err := visit(child); err != nil {
				return err
			}
		}
		state[index] = 2
		return nil
	}
	for index := range nodes {
		if err := visit(index); err != nil {
			return nil, err
		}
	}
	return parents, nil
}

func (importer *gltfImporter) appendNode(index int, parent mgl32.Mat4, state []uint8) error {
	switch state[index] {
	case 1:
		return fmt.Errorf("glTF node graph contains a cycle at node %d", index)
	case 2:
		return fmt.Errorf("glTF node %d is referenced more than once", index)
	}
	state[index] = 1
	node := importer.document.Nodes[index]
	local, err := gltfNodeMatrix(node)
	if err != nil {
		return fmt.Errorf("glTF node %d: %w", index, err)
	}
	transform := parent.Mul4(local)
	if node.Mesh != nil {
		skin := -1
		if node.Skin != nil {
			skin = *node.Skin
			if skin < 0 || skin >= len(importer.document.Skins) {
				return fmt.Errorf("glTF node %d skin %d is out of range", index, skin)
			}
		}
		firstPart := len(importer.result.Submeshes)
		if err := importer.appendMesh(*node.Mesh, transform, skin); err != nil {
			return fmt.Errorf("glTF node %d: %w", index, err)
		}
		if skin < 0 {
			for part := firstPart; part < len(importer.result.Submeshes); part++ {
				importer.result.Submeshes[part].Node = int32(index)
			}
		}
	}
	for _, child := range node.Children {
		if err := importer.appendNode(child, transform, state); err != nil {
			return err
		}
	}
	state[index] = 2
	return nil
}

func gltfNodeMatrix(node gltfNode) (mgl32.Mat4, error) {
	if len(node.Matrix) != 0 {
		if len(node.Matrix) != 16 {
			return mgl32.Mat4{}, fmt.Errorf("matrix must contain 16 values")
		}
		if len(node.Translation) != 0 || len(node.Rotation) != 0 || len(node.Scale) != 0 {
			return mgl32.Mat4{}, fmt.Errorf("matrix cannot be combined with TRS")
		}
		matrix := mgl32.Mat4(node.Matrix)
		if err := validateAffineMatrix(matrix); err != nil {
			return mgl32.Mat4{}, err
		}
		return matrix, nil
	}
	translation := mgl32.Vec3{}
	if len(node.Translation) != 0 {
		if len(node.Translation) != 3 {
			return mgl32.Mat4{}, fmt.Errorf("translation must contain 3 values")
		}
		translation = mgl32.Vec3(node.Translation)
	}
	rotation := mgl32.QuatIdent()
	if len(node.Rotation) != 0 {
		if len(node.Rotation) != 4 {
			return mgl32.Mat4{}, fmt.Errorf("rotation must contain 4 values")
		}
		rotation = mgl32.Quat{V: mgl32.Vec3(node.Rotation[:3]), W: node.Rotation[3]}
		if length := rotation.Len(); !finiteFloat32(length) || length < 1e-8 {
			return mgl32.Mat4{}, fmt.Errorf("rotation quaternion is invalid")
		}
		rotation = rotation.Normalize()
	}
	scale := mgl32.Vec3{1, 1, 1}
	if len(node.Scale) != 0 {
		if len(node.Scale) != 3 {
			return mgl32.Mat4{}, fmt.Errorf("scale must contain 3 values")
		}
		scale = mgl32.Vec3(node.Scale)
	}
	matrix := mgl32.Translate3D(translation[0], translation[1], translation[2]).Mul4(rotation.Mat4()).Mul4(mgl32.Scale3D(scale[0], scale[1], scale[2]))
	if err := validateAffineMatrix(matrix); err != nil {
		return mgl32.Mat4{}, err
	}
	return matrix, nil
}

func validateAffineMatrix(matrix mgl32.Mat4) error {
	for index, value := range matrix {
		if !finiteFloat32(value) {
			return fmt.Errorf("matrix component %d is not finite", index)
		}
	}
	if matrix[3] != 0 || matrix[7] != 0 || matrix[11] != 0 || matrix[15] != 1 {
		return fmt.Errorf("node matrix must be affine")
	}
	return nil
}

func (importer *gltfImporter) appendMesh(index int, transform mgl32.Mat4, skin int) error {
	if index < 0 || index >= len(importer.document.Meshes) {
		return fmt.Errorf("mesh %d is out of range", index)
	}
	mesh := importer.document.Meshes[index]
	if len(mesh.Primitives) == 0 {
		return fmt.Errorf("mesh %d has no primitives", index)
	}
	for primitiveIndex, primitive := range mesh.Primitives {
		if err := importer.appendPrimitive(primitive, transform, skin); err != nil {
			return fmt.Errorf("mesh %d primitive %d: %w", index, primitiveIndex, err)
		}
	}
	return nil
}

func (importer *gltfImporter) appendPrimitive(primitive gltfPrimitive, transform mgl32.Mat4, skinIndex int) error {
	// Skin matrices map source vertices into model space; the mesh node's
	// transform must not be baked into those vertices a second time.
	if skinIndex >= 0 {
		transform = mgl32.Ident4()
	}
	mode := uint32(gltfPrimitiveTriangles)
	if primitive.Mode != nil {
		mode = *primitive.Mode
	}
	if mode != gltfPrimitiveTriangles {
		return fmt.Errorf("primitive mode %d is unsupported; only TRIANGLES is accepted", mode)
	}
	if primitive.Material != nil && (*primitive.Material < 0 || *primitive.Material >= len(importer.result.Materials)) {
		return fmt.Errorf("material %d is out of range", *primitive.Material)
	}
	positionAccessor, ok := primitive.Attributes["POSITION"]
	if !ok {
		return fmt.Errorf("POSITION accessor is required")
	}
	positions, err := importer.accessorView(positionAccessor, "VEC3")
	if err != nil {
		return fmt.Errorf("POSITION: %w", err)
	}
	if positions.componentType != gltfComponentFloat || positions.normalized {
		return fmt.Errorf("POSITION must use non-normalized FLOAT components")
	}
	if uint64(len(importer.result.Vertices))+uint64(positions.count) > uint64(^uint32(0))+1 {
		return fmt.Errorf("vertex count exceeds uint32 index capacity")
	}
	base := len(importer.result.Vertices)
	importer.result.Vertices = append(importer.result.Vertices, make([]render.Vertex3D, positions.count)...)
	for index := range importer.result.MorphTargets {
		importer.result.MorphTargets[index].PositionDelta = append(importer.result.MorphTargets[index].PositionDelta, make([][3]float32, positions.count)...)
		if len(importer.result.MorphTargets[index].NormalDelta) != 0 {
			importer.result.MorphTargets[index].NormalDelta = append(importer.result.MorphTargets[index].NormalDelta, make([][3]float32, positions.count)...)
		}
		if len(importer.result.MorphTargets[index].TangentDelta) != 0 {
			importer.result.MorphTargets[index].TangentDelta = append(importer.result.MorphTargets[index].TangentDelta, make([][3]float32, positions.count)...)
		}
	}
	tangentAccessor, hasTangents := primitive.Attributes["TANGENT"]
	jointAccessor, hasJoints := primitive.Attributes["JOINTS_0"]
	weightAccessor, hasWeights := primitive.Attributes["WEIGHTS_0"]
	if hasJoints != hasWeights || (hasJoints && skinIndex < 0) || (skinIndex >= 0 && !hasJoints) {
		return fmt.Errorf("JOINTS_0 and WEIGHTS_0 must both be present on a skinned primitive")
	}
	if len(importer.result.Skin) != 0 || hasJoints {
		if len(importer.result.Skin) == 0 {
			importer.result.Skin = make([]compiledModelSkinVertex, base)
		}
		importer.result.Skin = append(importer.result.Skin, make([]compiledModelSkinVertex, positions.count)...)
	}
	if err := positions.copyFloats(3, func(vertex, component int, value float32) {
		importer.result.Vertices[base+vertex].Position[component] = value
	}); err != nil {
		return fmt.Errorf("POSITION: %w", err)
	}

	hasNormals := false
	if normalAccessor, exists := primitive.Attributes["NORMAL"]; exists {
		normals, viewErr := importer.accessorView(normalAccessor, "VEC3")
		if viewErr != nil {
			return fmt.Errorf("NORMAL: %w", viewErr)
		}
		if normals.count != positions.count {
			return fmt.Errorf("NORMAL count %d does not match POSITION count %d", normals.count, positions.count)
		}
		if normals.componentType != gltfComponentFloat || normals.normalized {
			return fmt.Errorf("NORMAL must use non-normalized FLOAT components")
		}
		if err := normals.copyFloats(3, func(vertex, component int, value float32) {
			importer.result.Vertices[base+vertex].Normal[component] = value
		}); err != nil {
			return fmt.Errorf("NORMAL: %w", err)
		}
		hasNormals = true
	}
	hasUV := false
	if uvAccessor, exists := primitive.Attributes["TEXCOORD_0"]; exists {
		uvs, viewErr := importer.accessorView(uvAccessor, "VEC2")
		if viewErr != nil {
			return fmt.Errorf("TEXCOORD_0: %w", viewErr)
		}
		if uvs.count != positions.count {
			return fmt.Errorf("TEXCOORD_0 count %d does not match POSITION count %d", uvs.count, positions.count)
		}
		if uvs.componentType == gltfComponentFloat && uvs.normalized || uvs.componentType != gltfComponentFloat && (uvs.componentType != gltfComponentUnsignedByte && uvs.componentType != gltfComponentUnsignedShort || !uvs.normalized) {
			return fmt.Errorf("TEXCOORD_0 must use FLOAT or normalized unsigned integer components")
		}
		if err := uvs.copyFloats(2, func(vertex, component int, value float32) {
			importer.result.Vertices[base+vertex].UV[component] = value
		}); err != nil {
			return fmt.Errorf("TEXCOORD_0: %w", err)
		}
		hasUV = true
	}
	if hasTangents {
		tangents, viewErr := importer.accessorView(tangentAccessor, "VEC4")
		if viewErr != nil {
			return fmt.Errorf("TANGENT: %w", viewErr)
		}
		if tangents.count != positions.count || tangents.componentType != gltfComponentFloat || tangents.normalized {
			return fmt.Errorf("TANGENT must contain one non-normalized FLOAT VEC4 per POSITION")
		}
		if err := tangents.copyFloats(4, func(vertex, component int, value float32) {
			importer.result.Vertices[base+vertex].Tangent[component] = value
		}); err != nil {
			return fmt.Errorf("TANGENT: %w", err)
		}
		for _, vertex := range importer.result.Vertices[base:] {
			t := vertex.Tangent
			if (t[3] != 1 && t[3] != -1) || t[0]*t[0]+t[1]*t[1]+t[2]*t[2] < 1e-8 {
				return fmt.Errorf("TANGENT must have a direction and handedness +1 or -1")
			}
		}
	}
	if hasJoints {
		joints, viewErr := importer.accessorView(jointAccessor, "VEC4")
		if viewErr != nil {
			return fmt.Errorf("JOINTS_0: %w", viewErr)
		}
		weights, viewErr := importer.accessorView(weightAccessor, "VEC4")
		if viewErr != nil {
			return fmt.Errorf("WEIGHTS_0: %w", viewErr)
		}
		if joints.count != positions.count || joints.normalized || joints.componentType != gltfComponentUnsignedByte && joints.componentType != gltfComponentUnsignedShort {
			return fmt.Errorf("JOINTS_0 must contain one non-normalized unsigned VEC4 per POSITION")
		}
		if weights.count != positions.count || weights.componentType == gltfComponentFloat && weights.normalized || weights.componentType != gltfComponentFloat && (weights.componentType != gltfComponentUnsignedByte && weights.componentType != gltfComponentUnsignedShort || !weights.normalized) {
			return fmt.Errorf("WEIGHTS_0 must contain one FLOAT or normalized unsigned VEC4 per POSITION")
		}
		jointCount := len(importer.document.Skins[skinIndex].Joints)
		jointOffset := importer.skinJointOffsets[skinIndex]
		if err := joints.copyUnsigned(4, func(vertex, influence int, value uint32) error {
			if uint64(value) >= uint64(jointCount) {
				return fmt.Errorf("joint %d at vertex %d influence %d exceeds %d skin joints", value, vertex, influence, jointCount)
			}
			importer.result.Skin[base+vertex].Joints[influence] = uint32(jointOffset) + value
			return nil
		}); err != nil {
			return fmt.Errorf("JOINTS_0: %w", err)
		}
		if err := weights.copyFloats(4, func(vertex, influence int, value float32) {
			importer.result.Skin[base+vertex].Weights[influence] = value
		}); err != nil {
			return fmt.Errorf("WEIGHTS_0: %w", err)
		}
		for vertex := range positions.count {
			value := &importer.result.Skin[base+vertex]
			value.Active = true
			total := value.Weights[0] + value.Weights[1] + value.Weights[2] + value.Weights[3]
			if math.Abs(float64(total-1)) > 0.01 {
				return fmt.Errorf("WEIGHTS_0 vertex %d weights sum to %g", vertex, total)
			}
		}
	}

	indexStart := len(importer.result.Indices)
	if primitive.Indices == nil {
		if positions.count%3 != 0 {
			return fmt.Errorf("non-indexed vertex count must be a multiple of three")
		}
		importer.result.Indices = append(importer.result.Indices, make([]uint32, positions.count)...)
		for index := range positions.count {
			importer.result.Indices[indexStart+index] = uint32(base + index)
		}
	} else {
		indices, viewErr := importer.accessorView(*primitive.Indices, "SCALAR")
		if viewErr != nil {
			return fmt.Errorf("indices: %w", viewErr)
		}
		if indices.normalized || indices.componentType != gltfComponentUnsignedByte && indices.componentType != gltfComponentUnsignedShort && indices.componentType != gltfComponentUnsignedInt {
			return fmt.Errorf("indices must use non-normalized unsigned integer components")
		}
		if indices.count == 0 || indices.count%3 != 0 {
			return fmt.Errorf("index count must be a non-zero multiple of three")
		}
		importer.result.Indices = append(importer.result.Indices, make([]uint32, indices.count)...)
		if err := indices.copyIndices(func(offset int, value uint32) error {
			if uint64(value) >= uint64(positions.count) {
				return fmt.Errorf("index %d at offset %d exceeds %d vertices", value, offset, positions.count)
			}
			importer.result.Indices[indexStart+offset] = uint32(base) + value
			return nil
		}); err != nil {
			return fmt.Errorf("indices: %w", err)
		}
	}
	material := len(importer.result.Materials)
	if primitive.Material != nil {
		material = *primitive.Material
	}
	importer.result.Submeshes = append(importer.result.Submeshes, graphics.Submesh{
		FirstIndex: uint32(indexStart), IndexCount: uint32(len(importer.result.Indices) - indexStart), Material: uint32(material), Node: -1,
	})
	if err := transformPrimitive(importer.result.Vertices[base:], importer.result.Indices[indexStart:], uint32(base), transform, hasNormals); err != nil {
		return err
	}
	if hasTangents {
		transformTangentPrimitive(importer.result.Vertices[base:], transform)
	} else if hasUV {
		graphics.GenerateTangents(importer.result.Vertices[base:], importer.result.Indices[indexStart:], uint32(base))
	}
	for targetIndex, target := range primitive.Targets {
		for len(importer.result.MorphTargets) <= targetIndex {
			importer.result.MorphTargets = append(importer.result.MorphTargets, compiledModelMorphTarget{
				Name:          fmt.Sprintf("morph-%d", len(importer.result.MorphTargets)),
				PositionDelta: make([][3]float32, len(importer.result.Vertices)),
			})
		}
		morph := &importer.result.MorphTargets[targetIndex]
		for semantic, accessor := range target {
			view, viewErr := importer.accessorView(accessor, "VEC3")
			if viewErr != nil {
				return fmt.Errorf("morph target %d %s accessor: %w", targetIndex, semantic, viewErr)
			}
			if view.count != positions.count || view.componentType != gltfComponentFloat || view.normalized {
				return fmt.Errorf("morph target %d %s accessor is invalid", targetIndex, semantic)
			}
			values := make([][3]float32, positions.count)
			if err := view.copyFloats(3, func(vertex, component int, value float32) { values[vertex][component] = value }); err != nil {
				return fmt.Errorf("morph target %d %s: %w", targetIndex, semantic, err)
			}
			transformMorphDelta(values, transform, semantic)
			switch semantic {
			case "POSITION":
				copy(morph.PositionDelta[base:base+positions.count], values)
			case "NORMAL":
				if len(morph.NormalDelta) == 0 {
					morph.NormalDelta = make([][3]float32, len(importer.result.Vertices))
				}
				copy(morph.NormalDelta[base:base+positions.count], values)
			case "TANGENT":
				if len(morph.TangentDelta) == 0 {
					morph.TangentDelta = make([][3]float32, len(importer.result.Vertices))
				}
				copy(morph.TangentDelta[base:base+positions.count], values)
			default:
				return fmt.Errorf("morph target semantic %q is unsupported", semantic)
			}
		}
	}
	return nil
}

func transformMorphDelta(values [][3]float32, transform mgl32.Mat4, semantic string) {
	matrix := transform
	if semantic == "NORMAL" {
		matrix = transform.Inv().Transpose()
	}
	for index, value := range values {
		result := matrix.Mul4x1(mgl32.Vec4{value[0], value[1], value[2], 0})
		values[index] = [3]float32{result[0], result[1], result[2]}
	}
}

func transformTangentPrimitive(vertices []render.Vertex3D, transform mgl32.Mat4) {
	flip := float32(1)
	if transform.Det() < 0 {
		flip = -1
	}
	for index := range vertices {
		t := vertices[index].Tangent
		value := transform.Mul4x1(mgl32.Vec4{t[0], t[1], t[2], 0}).Vec3()
		n := mgl32.Vec3(vertices[index].Normal)
		value = value.Sub(n.Mul(n.Dot(value)))
		if length := value.Len(); length > 1e-8 {
			value = value.Mul(1 / length)
		}
		vertices[index].Tangent = [4]float32{value[0], value[1], value[2], t[3] * flip}
	}
}

func (importer *gltfImporter) appendSkins() error {
	importer.skinJointOffsets = make([]int, len(importer.document.Skins))
	if len(importer.document.Skins) == 0 {
		return nil
	}
	// Store parent+1 so an unparented node needs no initialization pass.
	parents := make([]int, len(importer.document.Nodes))
	for parent, node := range importer.document.Nodes {
		for _, child := range node.Children {
			if parents[child] == 0 {
				parents[child] = parent + 1
			}
		}
	}
	for skinIndex, skin := range importer.document.Skins {
		importer.skinJointOffsets[skinIndex] = len(importer.result.Joints)
		if len(skin.Joints) == 0 {
			return fmt.Errorf("glTF skin %d has no joints", skinIndex)
		}
		var inverse [][16]float32
		if skin.InverseBindMatrices != nil {
			view, err := importer.accessorView(*skin.InverseBindMatrices, "MAT4")
			if err != nil || view.count != len(skin.Joints) || view.componentType != gltfComponentFloat || view.normalized {
				return fmt.Errorf("glTF skin %d inverse bind accessor is invalid", skinIndex)
			}
			inverse = make([][16]float32, len(skin.Joints))
			if err := view.copyFloats(16, func(index, component int, value float32) { inverse[index][component] = value }); err != nil {
				return fmt.Errorf("glTF skin %d inverse bind matrices: %w", skinIndex, err)
			}
		}
		jointLookup := make(map[int]int32, len(skin.Joints))
		for jointIndex, nodeIndex := range skin.Joints {
			if nodeIndex < 0 || nodeIndex >= len(importer.document.Nodes) {
				return fmt.Errorf("glTF skin %d joint %d is out of range", skinIndex, jointIndex)
			}
			if _, duplicate := jointLookup[nodeIndex]; duplicate {
				return fmt.Errorf("glTF skin %d contains duplicate joint node %d", skinIndex, nodeIndex)
			}
			jointLookup[nodeIndex] = int32(jointIndex)
		}
		for jointIndex, nodeIndex := range skin.Joints {
			parent := int32(-1)
			if value, exists := jointLookup[parents[nodeIndex]-1]; exists {
				parent = value
			}
			if parent >= 0 {
				parent += int32(importer.skinJointOffsets[skinIndex])
			}
			bind := [16]float32{}
			if inverse != nil {
				bind = inverse[jointIndex]
			} else {
				bind[0], bind[5], bind[10], bind[15] = 1, 1, 1, 1
			}
			for component, value := range bind {
				if !finiteFloat32(value) {
					return fmt.Errorf("glTF skin %d joint %d inverse bind component %d is not finite", skinIndex, jointIndex, component)
				}
			}
			name := importer.document.Nodes[nodeIndex].Name
			if name == "" {
				name = fmt.Sprintf("node-%d", nodeIndex)
			}
			importer.result.Joints = append(importer.result.Joints, compiledModelJoint{Name: name, Node: int32(nodeIndex), Parent: parent, InverseBind: bind})
		}
	}
	return nil
}

func (importer *gltfImporter) appendAnimations() error {
	for animationIndex, animation := range importer.document.Animations {
		compiled := compiledModelAnimation{Name: animation.Name}
		if compiled.Name == "" {
			compiled.Name = fmt.Sprintf("animation-%d", animationIndex)
		}
		for channelIndex, channel := range animation.Channels {
			if channel.Sampler < 0 || channel.Sampler >= len(animation.Samplers) || channel.Target.Node == nil || *channel.Target.Node < 0 || *channel.Target.Node >= len(importer.document.Nodes) {
				return fmt.Errorf("glTF animation %d channel %d is invalid", animationIndex, channelIndex)
			}
			sampler := animation.Samplers[channel.Sampler]
			interpolation := strings.ToUpper(strings.TrimSpace(sampler.Interpolation))
			if interpolation == "" {
				interpolation = "LINEAR"
			}
			if interpolation != "LINEAR" && interpolation != "STEP" && interpolation != "CUBICSPLINE" {
				return fmt.Errorf("glTF animation %d sampler %d interpolation %q is unsupported", animationIndex, channel.Sampler, sampler.Interpolation)
			}
			input, err := importer.accessorView(sampler.Input, "SCALAR")
			if err != nil || input.componentType != gltfComponentFloat || input.normalized {
				return fmt.Errorf("glTF animation %d input accessor is invalid", animationIndex)
			}
			outputType := "VEC3"
			components := 3
			switch channel.Target.Path {
			case "rotation":
				outputType, components = "VEC4", 4
			case "translation", "scale":
			default:
				return fmt.Errorf("glTF animation target %q is unsupported", channel.Target.Path)
			}
			output, err := importer.accessorView(sampler.Output, outputType)
			expectedOutputCount := input.count
			if interpolation == "CUBICSPLINE" {
				expectedOutputCount *= 3
			}
			if err != nil || output.componentType != gltfComponentFloat || output.normalized || output.count != expectedOutputCount {
				return fmt.Errorf("glTF animation %d output accessor is invalid", animationIndex)
			}
			var times []float32 = make([]float32, input.count)
			if err := input.copyFloats(1, func(index, _ int, value float32) { times[index] = value }); err != nil {
				return err
			}
			for index, time := range times {
				if !finiteFloat32(time) || (index > 0 && time < times[index-1]) {
					return fmt.Errorf("glTF animation %d channel %d input times are not finite and sorted", animationIndex, channelIndex)
				}
			}
			values := make([][4]float32, output.count)
			if err := output.copyFloats(components, func(index, component int, value float32) { values[index][component] = value }); err != nil {
				return err
			}
			path := importer.document.Nodes[*channel.Target.Node].Name
			if path == "" {
				path = fmt.Sprintf("node-%d", *channel.Target.Node)
			}
			if len(importer.result.Nodes) != 0 {
				path = importer.result.Nodes[*channel.Target.Node].Path
			}
			track := compiledModelAnimationTrack{Path: path, Target: "transform." + channel.Target.Path, Components: uint8(components), Interpolation: interpolation, Keyframes: make([]compiledModelAnimationKeyframe, len(times))}
			for index := range times {
				valueIndex := index
				key := compiledModelAnimationKeyframe{Time: times[index]}
				if interpolation == "CUBICSPLINE" {
					valueIndex = index*3 + 1
					key.InTangent = values[index*3]
					key.OutTangent = values[index*3+2]
				}
				key.Value = values[valueIndex]
				track.Keyframes[index] = key
				if times[index] > compiled.Duration {
					compiled.Duration = times[index]
				}
			}
			compiled.Tracks = append(compiled.Tracks, track)
		}
		if compiled.Duration <= 0 {
			return fmt.Errorf("glTF animation %d has no positive duration", animationIndex)
		}
		importer.result.Animations = append(importer.result.Animations, compiled)
	}
	return nil
}

func transformPrimitive(vertices []render.Vertex3D, indices []uint32, base uint32, transform mgl32.Mat4, hasNormals bool) error {
	determinant := transform.Det()
	if !finiteFloat32(determinant) || math.Abs(float64(determinant)) < 1e-12 {
		return fmt.Errorf("node transform is singular")
	}
	for index := range vertices {
		position := transform.Mul4x1(mgl32.Vec4{vertices[index].Position[0], vertices[index].Position[1], vertices[index].Position[2], 1})
		if position[3] != 1 {
			return fmt.Errorf("node transform produced a non-affine position")
		}
		vertices[index].Position = [3]float32{position[0], position[1], position[2]}
	}
	if determinant < 0 {
		for offset := 0; offset < len(indices); offset += 3 {
			indices[offset+1], indices[offset+2] = indices[offset+2], indices[offset+1]
		}
	}
	if !hasNormals {
		for offset := 0; offset < len(indices); offset += 3 {
			a := mgl32.Vec3(vertices[indices[offset]-base].Position)
			b := mgl32.Vec3(vertices[indices[offset+1]-base].Position)
			c := mgl32.Vec3(vertices[indices[offset+2]-base].Position)
			normal := b.Sub(a).Cross(c.Sub(a))
			for _, index := range indices[offset : offset+3] {
				current := mgl32.Vec3(vertices[index-base].Normal).Add(normal)
				vertices[index-base].Normal = [3]float32(current)
			}
		}
	} else {
		normalMatrix := transform.Inv().Transpose()
		for index := range vertices {
			normal := normalMatrix.Mul4x1(mgl32.Vec4{vertices[index].Normal[0], vertices[index].Normal[1], vertices[index].Normal[2], 0})
			vertices[index].Normal = [3]float32{normal[0], normal[1], normal[2]}
		}
	}
	for index := range vertices {
		normal := mgl32.Vec3(vertices[index].Normal)
		length := normal.Len()
		if !finiteFloat32(length) || length < 1e-8 {
			return fmt.Errorf("vertex %d has an invalid normal", index)
		}
		vertices[index].Normal = [3]float32(normal.Mul(1 / length))
	}
	return nil
}

func (importer *gltfImporter) accessorView(index int, wantType string) (gltfAccessorView, error) {
	if index < 0 || index >= len(importer.document.Accessors) {
		return gltfAccessorView{}, fmt.Errorf("accessor %d is out of range", index)
	}
	accessor := importer.document.Accessors[index]
	if accessor.Type != wantType {
		return gltfAccessorView{}, fmt.Errorf("accessor %d type is %q, want %q", index, accessor.Type, wantType)
	}
	if len(accessor.Sparse) != 0 && string(accessor.Sparse) != "null" {
		return gltfAccessorView{}, fmt.Errorf("sparse accessor %d is unsupported", index)
	}
	if accessor.BufferView == nil {
		return gltfAccessorView{}, fmt.Errorf("accessor %d has no bufferView", index)
	}
	if *accessor.BufferView < 0 || *accessor.BufferView >= len(importer.document.BufferViews) {
		return gltfAccessorView{}, fmt.Errorf("accessor %d bufferView is out of range", index)
	}
	view := importer.document.BufferViews[*accessor.BufferView]
	if view.Buffer < 0 || view.Buffer >= len(importer.buffers) {
		return gltfAccessorView{}, fmt.Errorf("bufferView %d buffer is out of range", *accessor.BufferView)
	}
	componentSize, err := gltfComponentSize(accessor.ComponentType)
	if err != nil {
		return gltfAccessorView{}, err
	}
	components := gltfTypeComponents(wantType)
	elementSize := componentSize * components
	stride := elementSize
	if view.ByteStride != 0 {
		if view.ByteStride > 252 || view.ByteStride%uint64(componentSize) != 0 || view.ByteStride < uint64(elementSize) {
			return gltfAccessorView{}, fmt.Errorf("bufferView byteStride %d is invalid", view.ByteStride)
		}
		stride = int(view.ByteStride)
	}
	if accessor.Count == 0 || accessor.Count > uint64(int(^uint(0)>>1)) {
		return gltfAccessorView{}, fmt.Errorf("accessor %d count %d is invalid", index, accessor.Count)
	}
	buffer := importer.buffers[view.Buffer]
	viewEnd, ok := addUint64(view.ByteOffset, view.ByteLength)
	if !ok || viewEnd > uint64(len(buffer)) {
		return gltfAccessorView{}, fmt.Errorf("bufferView %d exceeds buffer length", *accessor.BufferView)
	}
	start, ok := addUint64(view.ByteOffset, accessor.ByteOffset)
	if !ok || start%uint64(componentSize) != 0 || start > viewEnd {
		return gltfAccessorView{}, fmt.Errorf("accessor %d byteOffset is invalid", index)
	}
	lastOffset, ok := multiplyUint64(accessor.Count-1, uint64(stride))
	if !ok {
		return gltfAccessorView{}, fmt.Errorf("accessor %d byte range overflows", index)
	}
	lastEnd, ok := addUint64(lastOffset, uint64(elementSize))
	if !ok {
		return gltfAccessorView{}, fmt.Errorf("accessor %d byte range overflows", index)
	}
	end, ok := addUint64(start, lastEnd)
	if !ok || end > viewEnd {
		return gltfAccessorView{}, fmt.Errorf("accessor %d exceeds bufferView length", index)
	}
	return gltfAccessorView{
		data: buffer[int(start):int(end)], count: int(accessor.Count), stride: stride,
		componentSize: componentSize,
		componentType: accessor.ComponentType, normalized: accessor.Normalized,
	}, nil
}

func (view gltfAccessorView) copyFloats(components int, yield func(int, int, float32)) error {
	for element := range view.count {
		data := view.data[element*view.stride:]
		for component := range components {
			value, err := gltfFloat(data[component*view.componentSize:], view.componentType, view.normalized)
			if err != nil {
				return err
			}
			if !finiteFloat32(value) {
				return fmt.Errorf("element %d component %d is not finite", element, component)
			}
			yield(element, component, value)
		}
	}
	return nil
}

func (view gltfAccessorView) copyIndices(yield func(int, uint32) error) error {
	for element := range view.count {
		data := view.data[element*view.stride:]
		var value uint32
		switch view.componentType {
		case gltfComponentUnsignedByte:
			value = uint32(data[0])
		case gltfComponentUnsignedShort:
			value = uint32(binary.LittleEndian.Uint16(data))
		case gltfComponentUnsignedInt:
			value = binary.LittleEndian.Uint32(data)
		default:
			return fmt.Errorf("unsupported index component type %d", view.componentType)
		}
		if err := yield(element, value); err != nil {
			return err
		}
	}
	return nil
}

func (view gltfAccessorView) copyUnsigned(components int, yield func(int, int, uint32) error) error {
	for element := range view.count {
		data := view.data[element*view.stride:]
		for component := range components {
			offset := component * view.componentSize
			var value uint32
			switch view.componentType {
			case gltfComponentUnsignedByte:
				value = uint32(data[offset])
			case gltfComponentUnsignedShort:
				value = uint32(binary.LittleEndian.Uint16(data[offset:]))
			default:
				return fmt.Errorf("component type %d is not an unsigned joint index", view.componentType)
			}
			if err := yield(element, component, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func gltfFloat(data []byte, componentType uint32, normalized bool) (float32, error) {
	switch componentType {
	case gltfComponentFloat:
		return math.Float32frombits(binary.LittleEndian.Uint32(data)), nil
	case gltfComponentUnsignedByte:
		if normalized {
			return float32(data[0]) / 255, nil
		}
		return float32(data[0]), nil
	case gltfComponentUnsignedShort:
		value := binary.LittleEndian.Uint16(data)
		if normalized {
			return float32(value) / 65535, nil
		}
		return float32(value), nil
	default:
		return 0, fmt.Errorf("component type %d cannot be decoded as float", componentType)
	}
}

func gltfComponentSize(componentType uint32) (int, error) {
	switch componentType {
	case gltfComponentUnsignedByte:
		return 1, nil
	case gltfComponentUnsignedShort:
		return 2, nil
	case gltfComponentUnsignedInt, gltfComponentFloat:
		return 4, nil
	default:
		return 0, fmt.Errorf("unsupported accessor component type %d", componentType)
	}
}

func gltfTypeComponents(value string) int {
	switch value {
	case "SCALAR":
		return 1
	case "VEC2":
		return 2
	case "VEC3":
		return 3
	case "VEC4":
		return 4
	case "MAT4":
		return 16
	default:
		return 0
	}
}

func addUint64(left, right uint64) (uint64, bool) {
	result := left + right
	return result, result >= left
}

func multiplyUint64(left, right uint64) (uint64, bool) {
	if left == 0 || right <= ^uint64(0)/left {
		return left * right, true
	}
	return 0, false
}
