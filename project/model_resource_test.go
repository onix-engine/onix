//go:build !onix_release

package project

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/binzume/modelconv/fbx"
	"github.com/binzume/modelconv/geom"

	"github.com/diiyw/onix/graphics"
	graphicspng "github.com/diiyw/onix/graphics/codec/png"
	"github.com/diiyw/onix/internal/typedbin"
	enginephysics "github.com/diiyw/onix/physics"
	"github.com/diiyw/onix/render"
	"github.com/diiyw/onix/scene"
	"github.com/go-gl/mathgl/mgl32"
)

func TestImportGLTFExternalBufferAppliesNodeTransforms(t *testing.T) {
	buffer := testTriangleBuffer()
	document := fmt.Sprintf(`{
  "asset":{"version":"2.0"},
  "buffers":[{"uri":"triangle.bin","byteLength":%d}],
  "bufferViews":[{"buffer":0,"byteLength":%d}],
  "accessors":[{"bufferView":0,"componentType":5126,"count":3,"type":"VEC3"}],
  "meshes":[{"primitives":[{"attributes":{"POSITION":0}}]}],
  "nodes":[{"mesh":0},{"mesh":0,"translation":[2,0,0]}],
  "scenes":[{"nodes":[0,1]}],"scene":0
}`, len(buffer), len(buffer))
	source := fstest.MapFS{
		"models/triangle.gltf": {Data: []byte(document)},
		"models/triangle.bin":  {Data: buffer},
	}
	resource, err := decodeModelResource(source, "models/triangle.gltf", []byte(document))
	if err != nil {
		t.Fatalf("import glTF: %v", err)
	}
	if len(resource.Vertices) != 6 || len(resource.Indices) != 6 {
		t.Fatalf("merged geometry = %d vertices, %d indices", len(resource.Vertices), len(resource.Indices))
	}
	if got := resource.Vertices[4].Position; got != [3]float32{3, 0, 0} {
		t.Fatalf("translated vertex = %v", got)
	}
	for index, vertex := range resource.Vertices {
		if vertex.Normal != [3]float32{0, 0, 1} {
			t.Fatalf("generated normal %d = %v", index, vertex.Normal)
		}
	}
}

func TestImportGLBReadsNormalsUVsAndIndices(t *testing.T) {
	resource, err := decodeModelResource(fstest.MapFS{}, "models/triangle.glb", testTriangleGLB())
	if err != nil {
		t.Fatalf("import GLB: %v", err)
	}
	if len(resource.Vertices) != 3 || len(resource.Indices) != 3 {
		t.Fatalf("geometry = %+v", resource)
	}
	if resource.Vertices[1].Normal != [3]float32{0, 0, 1} || resource.Vertices[1].UV != [2]float32{1, 0} {
		t.Fatalf("vertex attributes = %+v", resource.Vertices[1])
	}
	if resource.Indices[2] != 2 {
		t.Fatalf("indices = %v", resource.Indices)
	}
}

func TestImportGLTFPreservesPBRMaterialTextureReferences(t *testing.T) {
	buffer := testTriangleBuffer()
	document := fmt.Sprintf(`{
  "asset":{"version":"2.0"},
  "buffers":[{"uri":"data:application/octet-stream;base64,%s","byteLength":36}],
  "bufferViews":[{"buffer":0,"byteLength":36}],
  "accessors":[{"bufferView":0,"componentType":5126,"count":3,"type":"VEC3"}],
  "images":[{"uri":"albedo.png"},{"uri":"normal.png"}],
  "textures":[{"source":0},{"source":1}],
  "materials":[{"name":"paint","pbrMetallicRoughness":{"baseColorFactor":[0.2,0.4,0.8,1],"baseColorTexture":{"index":0},"metallicFactor":0.7,"roughnessFactor":0.25},"normalTexture":{"index":1}}],
  "meshes":[{"primitives":[{"attributes":{"POSITION":0},"material":0}]}]
}`, base64.StdEncoding.EncodeToString(buffer))
	resource, err := decodeModelResource(fstest.MapFS{}, "resources/models/paint.gltf", []byte(document))
	if err != nil {
		t.Fatalf("import material glTF: %v", err)
	}
	if len(resource.Materials) != 1 {
		t.Fatalf("materials = %+v", resource.Materials)
	}
	material := resource.Materials[0]
	if material.Name != "paint" || material.BaseColor != [4]float32{0.2, 0.4, 0.8, 1} || material.Metallic != 0.7 || material.Roughness != 0.25 || material.BaseColorTexture != "resources/models/albedo.png" || material.NormalTexture != "resources/models/normal.png" {
		t.Fatalf("material = %+v", material)
	}
}

func TestLoadImportedTextureUsesResolvedGLTFPath(t *testing.T) {
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	const imagePath = "apps/game/resources/models/textures/albedo.png"
	resources := newResources()
	resources.source = fstest.MapFS{imagePath: {Data: imageData.Bytes()}}
	resources.baseDir = "apps/game"
	resources.imageDecoder = graphics.ImageDecoderSet[
		graphicspng.Decoder,
		graphics.NoImageDecoder,
		graphics.NoImageDecoder,
		graphics.NoImageDecoder,
		graphics.NoImageDecoder,
	]{}
	texture, err := resources.loadImportedTexture(imagePath, nil, "image/png", graphics.TextureOptions{})
	if err != nil {
		t.Fatalf("load resolved glTF texture: %v", err)
	}
	texture.Dispose()
}

func TestGLTFImporterLoadsDataURIAndBufferViewImages(t *testing.T) {
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(imageData.Bytes())
	dataURIDocument := gltfDocument{
		Images:    []gltfImage{{URI: "data:image/png;base64," + encoded}},
		Textures:  []gltfTexture{{Source: ptrInt(0)}},
		Materials: []gltfMaterial{{}},
	}
	dataURIDocument.Materials[0].PBR.BaseColorTexture = &gltfTextureRef{Index: 0}
	plainDataURIDocument := gltfDocument{
		Images:    []gltfImage{{URI: "data:image/png,hello%20world"}},
		Textures:  []gltfTexture{{Source: ptrInt(0)}},
		Materials: []gltfMaterial{{}},
	}
	plainDataURIDocument.Materials[0].PBR.BaseColorTexture = &gltfTextureRef{Index: 0}
	for _, test := range []struct {
		name  string
		doc   gltfDocument
		data  []byte
		check func(t *testing.T, material compiledModelMaterial)
	}{
		{
			name: "data URI",
			doc:  dataURIDocument,
			check: func(t *testing.T, material compiledModelMaterial) {
				if len(material.BaseColorTextureData) != imageData.Len() || material.BaseColorTextureMime != "image/png" {
					t.Fatalf("data URI texture = %d bytes, %q", len(material.BaseColorTextureData), material.BaseColorTextureMime)
				}
			},
		},
		{
			name: "plain data URI",
			doc:  plainDataURIDocument,
			check: func(t *testing.T, material compiledModelMaterial) {
				if string(material.BaseColorTextureData) != "hello world" || material.BaseColorTextureMime != "image/png" {
					t.Fatalf("plain data URI texture = %q, %q", material.BaseColorTextureData, material.BaseColorTextureMime)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			importer := gltfImporter{document: test.doc, buffers: [][]byte{test.data}}
			if err := importer.appendMaterials(); err != nil {
				t.Fatal(err)
			}
			test.check(t, importer.result.Materials[0])
		})
	}

	view := 0
	document := gltfDocument{
		Images:      []gltfImage{{BufferView: &view, MimeType: "image/png"}},
		Textures:    []gltfTexture{{Source: ptrInt(0)}},
		Materials:   []gltfMaterial{{}},
		BufferViews: []gltfBufferView{{Buffer: 0, ByteLength: uint64(imageData.Len())}},
	}
	document.Materials[0].PBR.BaseColorTexture = &gltfTextureRef{Index: 0}
	importer := gltfImporter{document: document, buffers: [][]byte{imageData.Bytes()}}
	if err := importer.appendMaterials(); err != nil {
		t.Fatal(err)
	}
	if got := importer.result.Materials[0].BaseColorTextureData; !bytes.Equal(got, imageData.Bytes()) {
		t.Fatalf("bufferView texture bytes do not match: %d vs %d", len(got), imageData.Len())
	}
}

func TestGLTFEmbeddedImageDataURIAndCubicSplineFixture(t *testing.T) {
	const fixture = "resources/fixtures/gltf/embedded_cubic.gltf"
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := decodeModelResource(os.DirFS("."), fixture, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(resource.Materials) != 1 || len(resource.Materials[0].BaseColorTextureData) == 0 || !bytes.Equal(resource.Materials[0].BaseColorTextureData, resource.Materials[0].NormalTextureData) {
		t.Fatalf("embedded textures = %+v", resource.Materials)
	}
	if resource.Materials[0].BaseColorTextureMime != "image/png" || resource.Materials[0].NormalTextureMime != "image/png" {
		t.Fatalf("embedded texture MIME types = %q, %q", resource.Materials[0].BaseColorTextureMime, resource.Materials[0].NormalTextureMime)
	}
	if _, err := png.Decode(bytes.NewReader(resource.Materials[0].BaseColorTextureData)); err != nil {
		t.Fatalf("decode fixture PNG: %v", err)
	}
	if len(resource.Animations) != 1 || resource.Animations[0].Tracks[0].Interpolation != "CUBICSPLINE" {
		t.Fatalf("fixture animations = %+v", resource.Animations)
	}
	clip, err := compileModelAnimation(resource.Animations[0])
	if err != nil {
		t.Fatal(err)
	}
	value, ok := scene.SampleAnimationTrack(clip.FloatTracks[0], .5, func(a, b, progress float32) float32 {
		return a + (b-a)*progress
	})
	if !ok || math.Abs(float64(value-.75)) > 1e-6 {
		t.Fatalf("fixture cubic sample = %g, %v", value, ok)
	}
}

func TestGLTFImporterLoadsMorphAndSkinVertexData(t *testing.T) {
	document := gltfDocument{Nodes: []gltfNode{{Name: "root", Children: []int{1}}, {Name: "child"}}, Skins: []gltfSkin{{Joints: []int{0, 1}}}}
	data := make([]byte, 0, 128)
	positions := addTestGLTFFloatAccessor(&document, &data, "VEC3", 3, 0, 0, 0, 1, 0, 0, 0, 1, 0)
	morph := addTestGLTFFloatAccessor(&document, &data, "VEC3", 3, 0, 0, 1, 0, 0, 1, 0, 0, 1)
	joints := addTestGLTFUnsignedAccessor(&document, &data, "VEC4", 3, gltfComponentUnsignedByte, []byte{0, 1, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0})
	weights := addTestGLTFFloatAccessor(&document, &data, "VEC4", 3, 0.5, 0.5, 0, 0, 1, 0, 0, 0, 0.25, 0.75, 0, 0)
	importer := gltfImporter{document: document, buffers: [][]byte{data}}
	if err := importer.appendSkins(); err != nil {
		t.Fatal(err)
	}
	primitive := gltfPrimitive{Attributes: map[string]int{"POSITION": positions, "JOINTS_0": joints, "WEIGHTS_0": weights}, Targets: []map[string]int{{"POSITION": morph}}}
	if err := importer.appendPrimitive(primitive, mgl32.Ident4(), 0); err != nil {
		t.Fatal(err)
	}
	if err := importer.result.validate(); err != nil {
		t.Fatal(err)
	}
	if len(importer.result.MorphTargets) != 1 || importer.result.MorphTargets[0].PositionDelta[2] != [3]float32{0, 0, 1} {
		t.Fatalf("morph targets = %+v", importer.result.MorphTargets)
	}
	if len(importer.result.Skin) != 3 || importer.result.Skin[0].Joints[0] != 0 || importer.result.Skin[0].Joints[1] != 1 || importer.result.Joints[1].Parent != 0 {
		t.Fatalf("skin = %+v, joints = %+v", importer.result.Skin, importer.result.Joints)
	}
}

func TestGLTFAnimationsCompileTranslationRotationScaleAndInterpolation(t *testing.T) {
	document := gltfDocument{Nodes: []gltfNode{{Name: "bone"}}}
	data := make([]byte, 0, 256)
	times := addTestGLTFFloatAccessor(&document, &data, "SCALAR", 2, 0, 1)
	translation := addTestGLTFFloatAccessor(&document, &data, "VEC3", 2, 0, 0, 0, 2, 4, 6)
	rotation := addTestGLTFFloatAccessor(&document, &data, "VEC4", 2, 0, 0, 0, 1, 0, 0, 1, 0)
	scale := addTestGLTFFloatAccessor(&document, &data, "VEC3", 2, 1, 1, 1, 2, 3, 4)
	node := 0
	document.Animations = []gltfAnimation{
		{Name: "move", Samplers: []gltfAnimationSampler{{Input: times, Output: translation, Interpolation: "STEP"}, {Input: times, Output: rotation}}, Channels: []gltfAnimationChannel{{Sampler: 0, Target: gltfAnimationTarget{Node: &node, Path: "translation"}}, {Sampler: 1, Target: gltfAnimationTarget{Node: &node, Path: "rotation"}}}},
		{Name: "scale", Samplers: []gltfAnimationSampler{{Input: times, Output: scale}}, Channels: []gltfAnimationChannel{{Sampler: 0, Target: gltfAnimationTarget{Node: &node, Path: "scale"}}}},
	}
	importer := gltfImporter{document: document, buffers: [][]byte{data}}
	if err := importer.appendAnimations(); err != nil {
		t.Fatal(err)
	}
	if len(importer.result.Animations) != 2 || importer.result.Animations[0].Duration != 1 || importer.result.Animations[1].Duration != 1 {
		t.Fatalf("animations = %+v", importer.result.Animations)
	}
	clip, err := compileModelAnimation(importer.result.Animations[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(clip.FloatTracks) != 6 || clip.FloatTracks[0].Keyframes[0].Tween != scene.AnimationStep || clip.FloatTracks[3].Binding.Target != scene.AnimationTargetTransformRotationX {
		t.Fatalf("compiled clip = %+v", clip)
	}
}

func TestGLTFCubicSplineAnimationUsesMiddleValues(t *testing.T) {
	document := gltfDocument{Nodes: []gltfNode{{Name: "bone"}}}
	data := make([]byte, 0, 256)
	times := addTestGLTFFloatAccessor(&document, &data, "SCALAR", 2, 0, 1)
	values := addTestGLTFFloatAccessor(&document, &data, "VEC3", 6,
		0, 0, 0, 1, 2, 3, 6, 0, 0,
		0, 0, 0, 4, 5, 6, 0, 0, 0,
	)
	node := 0
	document.Animations = []gltfAnimation{{Name: "cubic", Samplers: []gltfAnimationSampler{{Input: times, Output: values, Interpolation: "CUBICSPLINE"}}, Channels: []gltfAnimationChannel{{Sampler: 0, Target: gltfAnimationTarget{Node: &node, Path: "translation"}}}}}
	importer := gltfImporter{document: document, buffers: [][]byte{data}}
	if err := importer.appendAnimations(); err != nil {
		t.Fatal(err)
	}
	track := importer.result.Animations[0].Tracks[0]
	if len(track.Keyframes) != 2 || track.Keyframes[0].Value != [4]float32{1, 2, 3} || track.Keyframes[0].OutTangent != [4]float32{6, 0, 0} || track.Keyframes[1].Value != [4]float32{4, 5, 6} {
		t.Fatalf("cubic spline values = %+v", track.Keyframes)
	}
	clip, err := compileModelAnimation(importer.result.Animations[0])
	if err != nil || len(clip.FloatTracks) != 3 || clip.FloatTracks[0].Keyframes[0].Tween != scene.AnimationCubicSpline {
		t.Fatalf("compiled cubic spline clip = %+v, err=%v", clip, err)
	}
	value, ok := scene.SampleAnimationTrack(clip.FloatTracks[0], .5, func(a, b, progress float32) float32 { return a + (b-a)*progress })
	if !ok || math.Abs(float64(value-3.25)) > 1e-5 {
		t.Fatalf("compiled cubic spline midpoint = %g, %v", value, ok)
	}
}

func TestGLTFCubicSplineRotationNormalizesQuaternionBeforeEulerBinding(t *testing.T) {
	animation := compiledModelAnimation{Name: "rotate", Duration: 1, Tracks: []compiledModelAnimationTrack{{
		Path: "bone", Target: "transform.rotation", Components: 4, Interpolation: "CUBICSPLINE",
		Keyframes: []compiledModelAnimationKeyframe{
			{Time: 0, Value: [4]float32{0, 0, 0, 1}},
			{Time: 1, Value: [4]float32{0, 0, 1, 0}},
		},
	}}}
	clip, err := compileModelAnimation(animation)
	if err != nil {
		t.Fatal(err)
	}
	if len(clip.FloatTracks) != 3 {
		t.Fatalf("rotation tracks = %d", len(clip.FloatTracks))
	}
	value, ok := scene.SampleAnimationTrack(clip.FloatTracks[2], .5, func(a, b, progress float32) float32 { return a + (b-a)*progress })
	if !ok || math.Abs(float64(value-math.Pi/2)) > 1e-5 {
		t.Fatalf("cubic quaternion midpoint Z = %g, %v", value, ok)
	}
}

func TestImportFBXAppliesUnitsAndModelTransform(t *testing.T) {
	resource, err := decodeModelResource(fstest.MapFS{}, "models/triangle.fbx", testTriangleFBX(t))
	if err != nil {
		t.Fatalf("import FBX: %v", err)
	}
	if len(resource.Vertices) != 3 || len(resource.Indices) != 3 {
		t.Fatalf("geometry = %+v", resource)
	}
	if got := resource.Vertices[0].Position; got != [3]float32{1, 0, 0} {
		t.Fatalf("translated centimeter vertex = %v", got)
	}
	if got := resource.Vertices[1].Position; got != [3]float32{2, 0, 0} {
		t.Fatalf("scaled centimeter vertex = %v", got)
	}
	if resource.Vertices[0].Normal != [3]float32{0, 0, 1} {
		t.Fatalf("generated normal = %v", resource.Vertices[0].Normal)
	}
}

func TestImportedGLTFMeshIsCachedAndCooksCollision(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/resources.json", `{"models":[{"id":"triangle","path":"models/triangle.glb","collision":"trimesh"}]}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[]}`)
	writeTestFile(t, dir, "models/triangle.glb", string(testTriangleGLB()))
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Dispose()
	object := SceneNodeSpec{ID: "triangle", Components: SceneComponents{Model: &ResourceComponentSpec{Resource: "triangle"}}}
	first, supported, err := bundle.modelResourceForSpec(object)
	if err != nil || !supported || first.mesh == nil || !first.hasCollision {
		t.Fatalf("first load = %+v, supported=%v, err=%v", first, supported, err)
	}
	if first.collision.Mode != enginephysics.CollisionModeTriangleMesh {
		t.Fatalf("collision = %+v", first.collision)
	}
	second, _, err := bundle.modelResourceForSpec(object)
	if err != nil || second.mesh != first.mesh {
		t.Fatalf("cached mesh = %p, want %p, err=%v", second.mesh, first.mesh, err)
	}
	var cachedErr error
	allocations := testing.AllocsPerRun(100, func() {
		second, _, cachedErr = bundle.modelResourceForSpec(object)
	})
	if cachedErr != nil || second.mesh != first.mesh || allocations != 0 {
		t.Fatalf("cached model load = %p, allocations=%v, err=%v", second.mesh, allocations, cachedErr)
	}
}

func TestImportGLTFRejectsInvalidGeometry(t *testing.T) {
	tests := []struct {
		name   string
		buffer []byte
		mode   string
		want   string
	}{
		{name: "index range", buffer: testIndexedTriangleBuffer(3), want: "exceeds 3 vertices"},
		{name: "non-finite position", buffer: testNonFiniteTriangleBuffer(), want: "not finite"},
		{name: "topology", buffer: testIndexedTriangleBuffer(2), mode: `,"mode":1`, want: "only TRIANGLES"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := fmt.Sprintf(`{
  "asset":{"version":"2.0"},
  "buffers":[{"uri":"data.bin","byteLength":%d}],
  "bufferViews":[{"buffer":0,"byteLength":36},{"buffer":0,"byteOffset":36,"byteLength":6}],
  "accessors":[
    {"bufferView":0,"componentType":5126,"count":3,"type":"VEC3"},
    {"bufferView":1,"componentType":5123,"count":3,"type":"SCALAR"}
  ],
  "meshes":[{"primitives":[{"attributes":{"POSITION":0},"indices":1%s}]}]
}`, len(test.buffer), test.mode)
			source := fstest.MapFS{"models/data.bin": {Data: test.buffer}}
			_, err := decodeModelResource(source, "models/data.gltf", []byte(document))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("import error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestPreprocessManifestsCompilesGLTFModel(t *testing.T) {
	source := fstest.MapFS{
		"resources/resources.json": {Data: []byte(`{"models":[{"id":"triangle","path":"models/triangle.glb"}]}`)},
		"models/triangle.glb":      {Data: testTriangleGLB()},
	}
	files, err := PreprocessManifests(source, PreprocessOptions{})
	if err != nil {
		t.Fatalf("preprocess model: %v", err)
	}
	for _, file := range files {
		if file.Path != "models/triangle.glb" {
			continue
		}
		if !typedbin.IsEncoded(file.Data) {
			t.Fatal("model resource was not encoded as ONIXBIN1")
		}
		resource, decodeErr := typedbin.Unmarshal[compiledModelResource](file.Data)
		if decodeErr != nil || len(resource.Vertices) != 3 || len(resource.Indices) != 3 {
			t.Fatalf("compiled model = %+v, %v", resource, decodeErr)
		}
		return
	}
	t.Fatal("preprocessor omitted referenced model")
}

func TestPreprocessManifestsEncodesEmbeddedModelTextures(t *testing.T) {
	const fixture = "resources/fixtures/gltf/embedded_cubic.gltf"
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	source := fstest.MapFS{
		"resources/resources.json": {Data: []byte(`{"models":[{"id":"embedded","path":"resources/fixtures/gltf/embedded_cubic.gltf"}]}`)},
		fixture:                    {Data: data},
	}
	var srgb []bool
	files, err := PreprocessManifests(source, PreprocessOptions{TextureEncoder: func(data []byte, mime string, options graphics.TextureOptions) ([]byte, error) {
		if len(data) == 0 || mime != "image/png" {
			t.Fatalf("embedded input = %d bytes, MIME %q", len(data), mime)
		}
		srgb = append(srgb, options.SRGB)
		return []byte("ONIXTEX-test"), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(srgb) != 2 || !srgb[0] || srgb[1] {
		t.Fatalf("embedded texture color spaces = %v", srgb)
	}
	for _, file := range files {
		if file.Path != fixture {
			continue
		}
		resource, err := typedbin.Unmarshal[compiledModelResource](file.Data)
		if err != nil {
			t.Fatal(err)
		}
		material := resource.Materials[0]
		if string(material.BaseColorTextureData) != "ONIXTEX-test" || string(material.NormalTextureData) != "ONIXTEX-test" ||
			material.BaseColorTextureMime != "application/x-onixtex" || material.NormalTextureMime != "application/x-onixtex" {
			t.Fatalf("embedded textures were not encoded: %+v", material)
		}
		return
	}
	t.Fatal("preprocessor omitted embedded model")
}

func TestPreprocessManifestsEncodesExternalModelTextures(t *testing.T) {
	const modelPath = "resources/model.gltf"
	const texturePath = "resources/albedo.png"
	model := []byte(`{
  "asset":{"version":"2.0"},
  "images":[{"uri":"albedo.png","mimeType":"image/png"}],
  "textures":[{"source":0}],
  "materials":[{"pbrMetallicRoughness":{"baseColorTexture":{"index":0}}}],
  "meshes":[{"primitives":[{"attributes":{"POSITION":0},"material":0}]}],
  "buffers":[{"uri":"data:application/octet-stream;base64,AAAAAAAAAAAAAAAAAACAPwAAAAAAAAAAAAAAAAAAgD8AAAAA","byteLength":36}],
  "bufferViews":[{"buffer":0,"byteLength":36}],
  "accessors":[{"bufferView":0,"componentType":5126,"count":3,"type":"VEC3"}],
  "nodes":[{"mesh":0}],"scenes":[{"nodes":[0]}],"scene":0
}`)
	pngData := []byte("png-source")
	source := fstest.MapFS{
		"resources/resources.json": {Data: []byte(`{"models":[{"id":"external","path":"resources/model.gltf"}]}`)},
		modelPath:                  {Data: model},
		texturePath:                {Data: pngData},
	}
	var calls int
	files, err := PreprocessManifests(source, PreprocessOptions{TextureEncoder: func(data []byte, mime string, options graphics.TextureOptions) ([]byte, error) {
		calls++
		if !bytes.Equal(data, pngData) || mime != "image/png" || !options.SRGB {
			t.Fatalf("external texture input = %q, MIME %q, sRGB %v", data, mime, options.SRGB)
		}
		return []byte("ONIXTEX-external"), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("texture encoder called %d times, want 1", calls)
	}
	for _, file := range files {
		if file.Path != modelPath {
			continue
		}
		resource, err := typedbin.Unmarshal[compiledModelResource](file.Data)
		if err != nil {
			t.Fatal(err)
		}
		material := resource.Materials[0]
		if string(material.BaseColorTextureData) != "ONIXTEX-external" || material.BaseColorTexture != "" || material.BaseColorTextureMime != "application/x-onixtex" {
			t.Fatalf("external texture was not embedded: %+v", material)
		}
		return
	}
	t.Fatal("preprocessor omitted external model")
}

func TestPreprocessManifestsCompilesFBXModel(t *testing.T) {
	source := fstest.MapFS{
		"resources/resources.json": {Data: []byte(`{"models":[{"id":"triangle","path":"models/triangle.fbx"}]}`)},
		"models/triangle.fbx":      {Data: testTriangleFBX(t)},
	}
	files, err := PreprocessManifests(source, PreprocessOptions{})
	if err != nil {
		t.Fatalf("preprocess FBX: %v", err)
	}
	for _, file := range files {
		if file.Path == "models/triangle.fbx" && typedbin.IsEncoded(file.Data) {
			return
		}
	}
	t.Fatal("preprocessor omitted compiled FBX model")
}

func testTriangleBuffer() []byte {
	values := [...]float32{0, 0, 0, 1, 0, 0, 0, 1, 0}
	data := make([]byte, len(values)*4)
	for index, value := range values {
		binary.LittleEndian.PutUint32(data[index*4:], math.Float32bits(value))
	}
	return data
}

func addTestGLTFFloatAccessor(document *gltfDocument, data *[]byte, kind string, count int, values ...float32) int {
	start := len(*data)
	for _, value := range values {
		*data = binary.LittleEndian.AppendUint32(*data, math.Float32bits(value))
	}
	view := len(document.BufferViews)
	document.BufferViews = append(document.BufferViews, gltfBufferView{Buffer: 0, ByteOffset: uint64(start), ByteLength: uint64(len(*data) - start)})
	document.Accessors = append(document.Accessors, gltfAccessor{BufferView: &view, ComponentType: gltfComponentFloat, Count: uint64(count), Type: kind})
	return len(document.Accessors) - 1
}

func ptrInt(value int) *int { return &value }

func addTestGLTFUnsignedAccessor(document *gltfDocument, data *[]byte, kind string, count int, componentType uint32, values []byte) int {
	start := len(*data)
	*data = append(*data, values...)
	view := len(document.BufferViews)
	document.BufferViews = append(document.BufferViews, gltfBufferView{Buffer: 0, ByteOffset: uint64(start), ByteLength: uint64(len(values))})
	document.Accessors = append(document.Accessors, gltfAccessor{BufferView: &view, ComponentType: componentType, Count: uint64(count), Type: kind})
	return len(document.Accessors) - 1
}

func testIndexedTriangleBuffer(lastIndex uint16) []byte {
	data := append(testTriangleBuffer(), make([]byte, 6)...)
	binary.LittleEndian.PutUint16(data[36:], 0)
	binary.LittleEndian.PutUint16(data[38:], 1)
	binary.LittleEndian.PutUint16(data[40:], lastIndex)
	return data
}

func testNonFiniteTriangleBuffer() []byte {
	data := testIndexedTriangleBuffer(2)
	binary.LittleEndian.PutUint32(data[0:], math.Float32bits(float32(math.Inf(1))))
	return data
}

func testTriangleGLB() []byte {
	binaryChunk := make([]byte, 104)
	positions := [...][3]float32{{0, 0, 0}, {1, 0, 0}, {0, 1, 0}}
	uvs := [...]float32{0, 0, 1, 0, 0, 1}
	for vertex, position := range positions {
		for component, value := range position {
			binary.LittleEndian.PutUint32(binaryChunk[vertex*32+component*4:], math.Float32bits(value))
		}
		binary.LittleEndian.PutUint32(binaryChunk[vertex*32+20:], math.Float32bits(1))
		for component := range 2 {
			binary.LittleEndian.PutUint32(binaryChunk[vertex*32+24+component*4:], math.Float32bits(uvs[vertex*2+component]))
		}
	}
	binary.LittleEndian.PutUint16(binaryChunk[98:], 1)
	binary.LittleEndian.PutUint16(binaryChunk[100:], 2)
	jsonData := []byte(`{
  "asset":{"version":"2.0"},
  "buffers":[{"byteLength":102}],
  "bufferViews":[
	{"buffer":0,"byteLength":96,"byteStride":32},
	{"buffer":0,"byteOffset":96,"byteLength":6}
  ],
  "accessors":[
    {"bufferView":0,"componentType":5126,"count":3,"type":"VEC3"},
	{"bufferView":0,"byteOffset":12,"componentType":5126,"count":3,"type":"VEC3"},
	{"bufferView":0,"byteOffset":24,"componentType":5126,"count":3,"type":"VEC2"},
	{"bufferView":1,"componentType":5123,"count":3,"type":"SCALAR"}
  ],
  "meshes":[{"primitives":[{"attributes":{"POSITION":0,"NORMAL":1,"TEXCOORD_0":2},"indices":3}]}],
  "nodes":[{"mesh":0}],"scenes":[{"nodes":[0]}],"scene":0
}`)
	for len(jsonData)%4 != 0 {
		jsonData = append(jsonData, ' ')
	}
	data := make([]byte, 12+8+len(jsonData)+8+len(binaryChunk))
	copy(data[:4], "glTF")
	binary.LittleEndian.PutUint32(data[4:8], 2)
	binary.LittleEndian.PutUint32(data[8:12], uint32(len(data)))
	binary.LittleEndian.PutUint32(data[12:16], uint32(len(jsonData)))
	binary.LittleEndian.PutUint32(data[16:20], 0x4e4f534a)
	copy(data[20:], jsonData)
	offset := 20 + len(jsonData)
	binary.LittleEndian.PutUint32(data[offset:offset+4], uint32(len(binaryChunk)))
	binary.LittleEndian.PutUint32(data[offset+4:offset+8], 0x004e4942)
	copy(data[offset+8:], binaryChunk)
	return data
}

func testTriangleFBX(t *testing.T) []byte {
	t.Helper()
	document := fbx.NewDocument()
	geometry := fbx.NewGeometry("Triangle", []*geom.Vector3{
		geom.NewVector3(0, 0, 0),
		geom.NewVector3(100, 0, 0),
		geom.NewVector3(0, 100, 0),
	}, [][]int{{0, 1, 2}})
	model := fbx.NewModel("Triangle", "Mesh")
	model.SetTranslation(geom.NewVector3(100, 0, 0))
	document.AddObject(geometry)
	document.AddObject(model)
	document.AddConnection(model, geometry)
	document.AddConnection(document.Scene, model)
	var data bytes.Buffer
	if err := fbx.Write(&data, document); err != nil {
		t.Fatalf("write FBX fixture: %v", err)
	}
	return data.Bytes()
}

func TestGLTFSamplerAddressModes(t *testing.T) {
	importer := gltfImporter{document: gltfDocument{Textures: []gltfTexture{{}, {Sampler: ptrInt(0)}}, Samplers: []gltfSampler{{WrapS: 33071, WrapT: 33648, MinFilter: 9729}}}}
	defaults, err := importer.textureSampling(0)
	if err != nil {
		t.Fatal(err)
	}
	if defaults.WrapU != render.TextureAddressRepeat || defaults.WrapV != render.TextureAddressRepeat {
		t.Fatal("glTF default sampler must repeat")
	}
	specified, err := importer.textureSampling(1)
	if err != nil {
		t.Fatal(err)
	}
	if specified.WrapU != render.TextureAddressClamp || specified.WrapV != render.TextureAddressMirror || specified.Mipmap {
		t.Fatalf("sampler not preserved: %+v", specified)
	}
	importer.document.Samplers[0].WrapS = 123
	if _, err := importer.textureSampling(1); err == nil {
		t.Fatal("invalid wrap accepted")
	}
}
