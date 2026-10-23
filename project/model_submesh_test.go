//go:build !onix_release

package project

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/typedbin"
	"github.com/go-gl/mathgl/mgl32"
)

func TestGLTFSubmeshesAndNodeBindingsSurviveResourceCompilation(t *testing.T) {
	document := gltfDocument{
		Nodes: []gltfNode{
			{Name: "joint", Children: []int{1}, Translation: []float32{2, 0, 0}},
			{Name: "joint", Mesh: ptrInt(0), Translation: []float32{0, 3, 0}},
		},
		Materials: []gltfMaterial{{Name: "red"}, {Name: "blue"}},
	}
	document.Asset.Version = "2.0"
	red, blue := []float32{1, 0, 0, 1}, []float32{0, 0, 1, 1}
	document.Materials[0].PBR.BaseColorFactor = &red
	document.Materials[1].PBR.BaseColorFactor = &blue
	var data []byte
	positions := addTestGLTFFloatAccessor(&document, &data, "VEC3", 3, 0, 0, 0, 1, 0, 0, 0, 1, 0)
	times := addTestGLTFFloatAccessor(&document, &data, "SCALAR", 2, 0, 1)
	translations := addTestGLTFFloatAccessor(&document, &data, "VEC3", 2, 2, 0, 0, 4, 0, 0)
	document.Meshes = []gltfMesh{{Primitives: []gltfPrimitive{
		{Attributes: map[string]int{"POSITION": positions}, Material: ptrInt(0)},
		{Attributes: map[string]int{"POSITION": positions}, Material: ptrInt(1)},
		{Attributes: map[string]int{"POSITION": positions}},
	}}}
	document.Animations = []gltfAnimation{{Name: "move", Samplers: []gltfAnimationSampler{{Input: times, Output: translations}}, Channels: []gltfAnimationChannel{
		{Target: gltfAnimationTarget{Node: ptrInt(0), Path: "translation"}},
	}}}
	document.Buffers = []gltfBuffer{{URI: "mesh.bin", ByteLength: uint64(len(data))}}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	source := fstest.MapFS{
		"resources/resources.json": {Data: []byte(`{"models":[{"id":"model","path":"resources/model.gltf"}]}`)},
		"resources/model.gltf":     {Data: encoded},
		"resources/mesh.bin":       {Data: data},
	}
	asset, err := importGLTF(modelBufferSource{source: source, path: "resources/model.gltf", data: encoded})
	if err != nil {
		t.Fatal(err)
	}
	wantParts := []graphics.Submesh{
		{FirstIndex: 0, IndexCount: 3, Material: 0, Node: 1},
		{FirstIndex: 3, IndexCount: 3, Material: 1, Node: 1},
		{FirstIndex: 6, IndexCount: 3, Material: 2, Node: 1},
	}
	if !reflect.DeepEqual(asset.Submeshes, wantParts) || len(asset.Skin) != 0 {
		t.Fatalf("rigid primitives: %+v, skin vertices: %d", asset.Submeshes, len(asset.Skin))
	}
	if asset.Nodes[1].Parent != 0 || asset.Nodes[0].Path == asset.Nodes[1].Path || asset.Animations[0].Tracks[0].Path != asset.Nodes[0].Path {
		t.Fatalf("ambiguous node animation binding: %+v", asset.Nodes)
	}
	if got := asset.Vertices[0].Position; got != [3]float32{2, 3, 0} {
		t.Fatalf("initial rigid position = %v", got)
	}
	files, err := PreprocessManifests(source, PreprocessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Path != "resources/model.gltf" {
			continue
		}
		compiled, err := typedbin.Unmarshal[compiledModelResource](file.Data)
		if err != nil || !reflect.DeepEqual(compiled, asset) {
			t.Fatalf("compiled resource lost model data: %v", err)
		}
		if err := compiled.validate(); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Fatal("compiled model missing")
}

func TestGLTFSkinIgnoresMeshNodeTransform(t *testing.T) {
	document := gltfDocument{Nodes: []gltfNode{{Name: "joint"}}, Skins: []gltfSkin{{Joints: []int{0}}}}
	var data []byte
	positions := addTestGLTFFloatAccessor(&document, &data, "VEC3", 3, 0, 0, 0, 1, 0, 0, 0, 1, 0)
	joints := addTestGLTFUnsignedAccessor(&document, &data, "VEC4", 3, gltfComponentUnsignedByte, make([]byte, 12))
	weights := addTestGLTFFloatAccessor(&document, &data, "VEC4", 3, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0)
	importer := gltfImporter{document: document, buffers: [][]byte{data}}
	if err := importer.appendSkins(); err != nil {
		t.Fatal(err)
	}
	primitive := gltfPrimitive{Attributes: map[string]int{"POSITION": positions, "JOINTS_0": joints, "WEIGHTS_0": weights}}
	if err := importer.appendPrimitive(primitive, mgl32.Translate3D(20, 0, 0), 0); err != nil {
		t.Fatal(err)
	}
	if importer.result.Vertices[0].Position != [3]float32{} || importer.result.Submeshes[0].Node != -1 {
		t.Fatal("skinned geometry includes the mesh node transform")
	}
}

func TestSceneBindsEveryImportedMaterialAndDefaultSlot(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "resources/project.json", `{}`)
	writeTestFile(t, dir, "resources/scenes/main.scene.json", `{"nodes":[]}`)
	writeTestFile(t, dir, "resources/resources.json", `{"models":[{"id":"triangle","path":"resources/model.gltf","material":{"roughness":0.25}}]}`)
	writeTestFile(t, dir, "resources/model.gltf", `{
		"asset":{"version":"2.0"},"buffers":[{"uri":"mesh.bin","byteLength":36}],
		"bufferViews":[{"buffer":0,"byteLength":36}],"accessors":[{"bufferView":0,"componentType":5126,"count":3,"type":"VEC3"}],
		"materials":[{"pbrMetallicRoughness":{"baseColorFactor":[1,0,0,1]}},{"pbrMetallicRoughness":{"baseColorFactor":[0,0,1,1]}}],
		"meshes":[{"primitives":[{"attributes":{"POSITION":0},"material":0},{"attributes":{"POSITION":0},"material":1},{"attributes":{"POSITION":0}}]}]
	}`)
	writeTestFile(t, dir, "resources/mesh.bin", string(testTriangleBuffer()))
	bundle, err := loadTestProject(os.DirFS(dir), ".")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bundle.Dispose() })
	object := SceneNodeSpec{ID: "triangle", Components: SceneComponents{Model: &ResourceComponentSpec{Resource: "triangle"}}}
	mesh, material, ok, err := bundle.modelForSpec(object)
	if err != nil || !ok || len(material.Materials) != 3 || len(mesh.Submeshes) != 3 {
		t.Fatalf("model materials: %+v, %v", material, err)
	}
	for i, want := range []graphics.ColorRGBA{{R: 1, A: 1}, {B: 1, A: 1}, graphics.White} {
		got := material.Materials[i]
		if got.BaseColor != want || got.PerceptualRoughness != .25 || got.Metallic != 1 {
			t.Fatalf("material %d = %+v", i, got)
		}
	}
}
