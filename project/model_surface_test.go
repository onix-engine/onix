//go:build !onix_release

package project

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"testing/fstest"

	"github.com/diiyw/onix/graphics"
	"github.com/diiyw/onix/internal/typedbin"
)

func advancedSurfaceGLTF() string {
	return fmt.Sprintf(`{"asset":{"version":"2.0"},"extensionsRequired":["KHR_materials_specular","KHR_materials_clearcoat","KHR_materials_sheen","KHR_materials_anisotropy","KHR_materials_ior","KHR_materials_emissive_strength"],
	"buffers":[{"uri":"data:application/octet-stream;base64,%s","byteLength":36}],
	"bufferViews":[{"buffer":0,"byteLength":36}],"accessors":[{"bufferView":0,"componentType":5126,"count":3,"type":"VEC3"}],
	"images":[{"uri":"color.png"},{"uri":"data.png"}],"textures":[{"source":0},{"source":1}],
	"materials":[{"pbrMetallicRoughness":{"baseColorTexture":{"index":0}},"normalTexture":{"index":1,"scale":0.3},
	"occlusionTexture":{"index":1,"strength":0.4},"emissiveTexture":{"index":0},"alphaMode":"MASK","alphaCutoff":0.2,"doubleSided":true,
	"extensions":{"KHR_materials_specular":{"specularFactor":0.6,"specularColorFactor":[2,0.5,1],"specularTexture":{"index":1},"specularColorTexture":{"index":0}},
    "KHR_materials_clearcoat":{"clearcoatFactor":0.8,"clearcoatRoughnessFactor":0.2,"clearcoatTexture":{"index":1},"clearcoatRoughnessTexture":{"index":1},"clearcoatNormalTexture":{"index":1,"scale":0.7}},
    "KHR_materials_sheen":{"sheenColorFactor":[0.2,0.3,0.4],"sheenRoughnessFactor":0.6,"sheenColorTexture":{"index":0},"sheenRoughnessTexture":{"index":1}},
    "KHR_materials_anisotropy":{"anisotropyStrength":0.9,"anisotropyRotation":1.2,"anisotropyTexture":{"index":1}},
    "KHR_materials_ior":{"ior":2.42},"KHR_materials_emissive_strength":{"emissiveStrength":64}}}],
	"meshes":[{"primitives":[{"attributes":{"POSITION":0},"material":0}]}]}`, base64.StdEncoding.EncodeToString(testTriangleBuffer()))

}

func TestGLTFSurfaceSurvivesBinaryPreprocessing(t *testing.T) {
	document := advancedSurfaceGLTF()
	model, err := decodeModelResource(fstest.MapFS{}, "resources/model.gltf", []byte(document))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := typedbin.Marshal(model)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := typedbin.Unmarshal[compiledModelResource](encoded)
	if err != nil {
		t.Fatal(err)
	}
	m := decoded.Materials[0]
	s := m.Surface
	if s.IOR != 2.42 || s.EmissiveStrength != 64 {
		t.Fatalf("IOR/emission lost in binary preprocessing: %+v", s)
	}
	if s.NormalScale != .3 || s.OcclusionStrength != .4 || s.SpecularFactor != .6 || s.SpecularColor != [3]float32{2, .5, 1} || s.AlphaMode != graphics.AlphaMask || s.AlphaCutoff != .2 || !s.DoubleSided {
		t.Fatalf("surface lost in import/preprocessing: %+v", s)
	}
	if s.ClearcoatFactor != .8 || s.ClearcoatRoughness != .2 || s.ClearcoatNormalScale != .7 || s.SheenColor != [3]float32{.2, .3, .4} || s.SheenRoughness != .6 || s.AnisotropyStrength != .9 || s.AnisotropyRotation != 1.2 {
		t.Fatalf("advanced surface lost in binary preprocessing: %+v", s)
	}
	if !m.BaseColorSampling.SRGB || m.NormalSampling.SRGB {
		t.Fatal("incorrect base/normal transfer functions")
	}
	for slot, texture := range s.Textures {
		if texture.Path == "" || texture.Sampling.SRGB != (slot == 1 || slot == 3 || slot == 7) {
			t.Fatalf("texture slot %d lost its path or color space", slot)
		}
	}
}

func TestGLTFSurfaceRejectsInvalidFactors(t *testing.T) {
	for _, source := range []string{
		`{"extensions":{"KHR_materials_specular":{"specularColorFactor":[1,1]}}}`,
		`{"extensions":{"KHR_materials_specular":{"specularColorFactor":[1,1,1,1]}}}`,
		`{"extensions":{"KHR_materials_specular":{"specularFactor":1.1}}}`,
		`{"occlusionTexture":{"index":0,"strength":-1}}`,
		`{"normalTexture":{"index":0,"texCoord":1}}`,
		`{"alphaMode":"UNKNOWN"}`,
		`{"extensions":{"KHR_materials_clearcoat":{"clearcoatFactor":-0.1}}}`,
		`{"extensions":{"KHR_materials_clearcoat":{"clearcoatRoughnessFactor":1.1}}}`,
		`{"extensions":{"KHR_materials_sheen":{"sheenColorFactor":[1,1]}}}`,
		`{"extensions":{"KHR_materials_sheen":{"sheenColorFactor":[1,2,0]}}}`,
		`{"extensions":{"KHR_materials_sheen":{"sheenRoughnessFactor":-1}}}`,
		`{"extensions":{"KHR_materials_anisotropy":{"anisotropyStrength":2}}}`,
		`{"extensions":{"KHR_materials_anisotropy":{"anisotropyTexture":{"index":0,"texCoord":1}}}}`,
		`{"extensions":{"KHR_materials_unlit":{},"KHR_materials_clearcoat":{}}}`,
		`{"extensions":{"KHR_materials_unlit":{},"KHR_materials_sheen":{}}}`,
		`{"extensions":{"KHR_materials_unlit":{},"KHR_materials_anisotropy":{}}}`,
		`{"extensions":{"KHR_materials_ior":{"ior":0.5}}}`,
		`{"extensions":{"KHR_materials_ior":{"ior":-1}}}`,
		`{"extensions":{"KHR_materials_emissive_strength":{"emissiveStrength":-1}}}`,
		`{"extensions":{"KHR_materials_unlit":{},"KHR_materials_ior":{}}}`,
		`{"extensions":{"KHR_materials_unlit":{},"KHR_materials_emissive_strength":{}}}`,
		`{"extensions":{"KHR_materials_unlit":{},"KHR_materials_specular":{}}}`,
	} {
		var material gltfMaterial
		if err := json.Unmarshal([]byte(source), &material); err != nil {
			t.Fatal(err)
		}
		if err := (&gltfImporter{}).appendMaterialSurface(&compiledModelMaterial{}, material); err == nil {
			t.Fatalf("accepted invalid surface %s", source)
		}
	}
	for _, surface := range []compiledMaterialSurface{
		{NormalScale: float32(math.NaN())},
		{SpecularFactor: 2},
		{AlphaMode: graphics.AlphaMode(3)},
		{SpecularColor: [3]float32{-1}},
		{AnisotropyRotation: float32(math.Inf(1))},
		{ClearcoatNormalScale: float32(math.NaN())},
		{SheenColor: [3]float32{2}},
		{IOR: .5},
		{IOR: float32(math.Inf(1))},
		{EmissiveStrength: -1},
		{EmissiveStrength: float32(math.NaN())},
	} {
		if err := surface.validate(); err == nil {
			t.Fatalf("accepted invalid binary surface %+v", surface)
		}
	}
}

func TestGLTFAdvancedSurfaceDefaultsAndRuntimeFactors(t *testing.T) {
	var source compiledModelMaterial
	if err := (&gltfImporter{}).appendMaterialSurface(&source, gltfMaterial{}); err != nil {
		t.Fatal(err)
	}
	s := source.Surface
	if s.IOR != 1.5 || s.EmissiveStrength != 1 {
		t.Fatal(s)
	}
	s.IOR, s.EmissiveStrength = 2.42, 64
	if s.ClearcoatFactor != 0 || s.SheenColor != [3]float32{} || s.AnisotropyStrength != 0 || s.ClearcoatNormalScale != 1 {
		t.Fatal(s)
	}
	s.ClearcoatFactor, s.ClearcoatRoughness, s.ClearcoatNormalScale = .4, .2, .7
	s.SheenColor, s.SheenRoughness = [3]float32{.2, .3, .4}, .6
	s.AnisotropyStrength, s.AnisotropyRotation = .8, -7
	var material graphics.StandardMaterial
	if err := (&Resources{}).loadMaterialSurface(&material, s, nil); err != nil {
		t.Fatal(err)
	}
	actual := material.Surface
	if material.IOR != 2.42 || material.EmissiveStrength != 64 {
		t.Fatal(material)
	}
	if actual.ClearcoatFactor != .4 || actual.ClearcoatRoughness != .2 || actual.ClearcoatNormalScale != .7 || actual.SheenColor.R != .2 || actual.SheenRoughness != .6 || actual.AnisotropyStrength != .8 || actual.AnisotropyRotation != -7 {
		t.Fatal(actual)
	}
}

func TestPreprocessAdvancedSurfaceTexturesSharesEncodedImages(t *testing.T) {
	source := fstest.MapFS{
		"resources/resources.json": {Data: []byte(`{"models":[{"id":"layers","path":"resources/model.gltf"}]}`)},
		"resources/model.gltf":     {Data: []byte(advancedSurfaceGLTF())},
		"resources/color.png":      {Data: []byte("color")},
		"resources/data.png":       {Data: []byte("data")},
	}
	calls := 0
	files, err := PreprocessManifests(source, PreprocessOptions{TextureEncoder: func(data []byte, mime string, options graphics.TextureOptions) ([]byte, error) {
		calls++
		if (string(data) == "color") != options.SRGB {
			t.Fatal("incorrect surface color space", string(data), options)
		}
		return []byte(fmt.Sprintf("encoded-srgb-%t", options.SRGB)), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("encoder called %d times for two shared images", calls)
	}
	for _, file := range files {
		if file.Path != "resources/model.gltf" {
			continue
		}
		model, err := typedbin.Unmarshal[compiledModelResource](file.Data)
		if err != nil {
			t.Fatal(err)
		}
		for slot, texture := range model.Materials[0].Surface.Textures {
			want := fmt.Sprintf("encoded-srgb-%t", slot == 1 || slot == 3 || slot == 7)
			if texture.Path != "" || texture.Mime != "application/x-onixtex" || string(texture.Data) != want {
				t.Fatalf("slot %d was not encoded correctly: %+v", slot, texture)
			}
		}
		return
	}
	t.Fatal("compiled model was omitted")
}
